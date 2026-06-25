#!/usr/bin/env bash
set -euo pipefail

timestamp() {
  date +"%Y-%m-%d %T"
}

info() {
  local flag
  flag="$(timestamp)"
  echo -e "\033[36m INFO [$flag] >> $* \033[0m"
}

warn() {
  local flag
  flag="$(timestamp)"
  echo -e "\033[33m WARN [$flag] >> $* \033[0m"
}

error() {
  local flag
  flag="$(timestamp)"
  echo -e "\033[31m ERROR [$flag] >> $* \033[0m"
  exit 1
}

RELEASE_NAME="${RELEASE_NAME:-kite}"
NAMESPACE="${NAMESPACE:-kite-system}"
HELM_OPTS="${HELM_OPTS:-}"
ENABLE_APP="${ENABLE_APP:-true}"
STRICT_SECRET_REUSE="${STRICT_SECRET_REUSE:-true}"
APP_NAME="${APP_NAME:-kite}"
CHART_DIR="${CHART_DIR:-charts/kite}"
TOOLS_FILE="${TOOLS_FILE:-/root/.sealos/cloud/scripts/tools.sh}"
VALUES_DIR="${VALUES_DIR:-/root/.sealos/cloud/values/apps/${APP_NAME}}"
DEFAULT_VALUES_FILE="${CHART_DIR}/${APP_NAME}-values.yaml"
GLOBAL_VALUES_FILE="/root/.sealos/cloud/values/global.yaml"

if [ -f /root/.sealos/cloud/scripts/tools.sh ]; then
  # shellcheck source=/dev/null
  source /root/.sealos/cloud/scripts/tools.sh
elif [ -f "${TOOLS_FILE}" ]; then
  # shellcheck source=/dev/null
  source "${TOOLS_FILE}"
else
  warn "Tools file ${TOOLS_FILE} not found, using local fallback readers"
fi

read_config_value() {
  local key=$1
  local value=""

  if declare -f get_cm_value >/dev/null 2>&1; then
    value="$(get_cm_value sealos-system sealos-config "${key}" 1 0 2>/dev/null || true)"
  fi

  if [ -z "${value}" ]; then
    value="$(kubectl get configmap sealos-config -n sealos-system -o "jsonpath={.data.${key}}" 2>/dev/null || true)"
  fi

  printf '%s' "${value}"
}

read_yaml_value() {
  local path_expr=$1
  local value=""

  if declare -f read_yaml_file_path >/dev/null 2>&1; then
    value="$(read_yaml_file_path "${path_expr}" 2>/dev/null || true)"
  fi

  printf '%s' "${value}"
}

read_tls_reject_unauthorized() {
  local value=""
  local cert_mode=""

  if declare -f read_cert_tls_reject_unauthorized >/dev/null 2>&1; then
    value="$(read_cert_tls_reject_unauthorized 2>/dev/null || true)"
  fi

  if [ -n "${value}" ]; then
    printf '%s' "${value}"
    return 0
  fi

  cert_mode="$(kubectl get configmap cert-config -n sealos-system -o "jsonpath={.data.CERT_MODE}" 2>/dev/null || true)"
  case "${cert_mode}" in
    https|acme)
      printf '0'
      ;;
    *)
      printf '1'
      ;;
  esac
}

read_kubeblocks_version() {
  local version=""

  version="$(read_yaml_value '.global.featureConfigs.database.kubeblocksVersion')"
  if [ -z "${version}" ] && [ -f "${GLOBAL_VALUES_FILE}" ]; then
    version="$(awk '
      /kubeblocksVersion:/ {
        value=$0
        sub(/^[^:]*:[[:space:]]*/, "", value)
        gsub(/["'\'' ]/, "", value)
        print value
        exit
      }
    ' "${GLOBAL_VALUES_FILE}")"
  fi

  printf '%s' "${version:-0.8.2}"
}

prepare_values_files() {
  mkdir -p "${VALUES_DIR}"

  if ! find "${VALUES_DIR}" -maxdepth 1 -type f -name '*-values.yaml' -print -quit | grep -q .; then
    warn "Cloud values directory /root/.sealos/cloud/values/apps/${APP_NAME}/ has no *-values.yaml, copying default ${DEFAULT_VALUES_FILE}"
    cp "${DEFAULT_VALUES_FILE}" "${VALUES_DIR}/${APP_NAME}-values.yaml"
  fi

  while IFS= read -r values_file; do
    [ -n "${values_file}" ] || continue
    info "Using additional Helm values from ${values_file}"
    values_args+=(-f "${values_file}")
  done < <(find "${VALUES_DIR}" -maxdepth 1 -type f -name '*-values.yaml' | sort)
}

decode_base64() {
  local raw=$1
  local decoded=""

  if decoded="$(printf '%s' "${raw}" | base64 --decode 2>/dev/null)"; then
    printf '%s' "${decoded}"
    return 0
  fi

  if decoded="$(printf '%s' "${raw}" | base64 -d 2>/dev/null)"; then
    printf '%s' "${decoded}"
    return 0
  fi

  return 1
}

get_secret_data() {
  local secret_name=$1
  local key=$2
  local encoded=""

  encoded="$(kubectl get secret "${secret_name}" -n "${NAMESPACE}" -o "jsonpath={.data.${key}}" 2>/dev/null || true)"
  [ -n "${encoded}" ] || return 1

  decode_base64 "${encoded}"
}

find_existing_kite_secret() {
  local name=""

  for name in "${RELEASE_NAME}-secret" "${RELEASE_NAME}-kite-secret"; do
    if kubectl get secret "${name}" -n "${NAMESPACE}" >/dev/null 2>&1; then
      echo "${name}"
      return 0
    fi
  done

  while IFS= read -r name; do
    [ -n "${name}" ] || continue
    if kubectl get secret "${name}" -n "${NAMESPACE}" >/dev/null 2>&1; then
      echo "${name}"
      return 0
    fi
  done < <(kubectl get secret -n "${NAMESPACE}" \
    -l "app.kubernetes.io/instance=${RELEASE_NAME},app.kubernetes.io/name=kite" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)

  return 1
}

is_existing_release() {
  helm status "${RELEASE_NAME}" -n "${NAMESPACE}" >/dev/null 2>&1
}

if declare -f read_jwt_internal >/dev/null 2>&1; then
  sealos_jwt_secret="$(read_jwt_internal 2>/dev/null || true)"
else
  sealos_jwt_secret="$(read_config_value jwtInternal)"
fi
sealos_cloud_domain="$(read_config_value cloudDomain)"
sealos_cloud_port="$(read_config_value cloudPort)"
sealos_http_port="$(read_config_value httpPort)"
sealos_disable_https="$(read_config_value disableHttps)"
sealos_cert_secret_name="$(read_config_value certSecretName)"
platform_tls_reject_unauthorized="$(read_tls_reject_unauthorized)"
kubeblocks_version="$(read_kubeblocks_version)"

if [ -z "${sealos_disable_https}" ] && declare -f global_http_disable_https >/dev/null 2>&1; then
  if global_http_disable_https; then
    sealos_disable_https="true"
  else
    sealos_disable_https="false"
  fi
fi

if [ -z "${sealos_cloud_port}" ]; then
  sealos_cloud_port="$(read_yaml_value '.global.http.httpsPort')"
fi
if [ -z "${sealos_http_port}" ]; then
  sealos_http_port="$(read_yaml_value '.global.http.httpPort')"
fi
if [ -z "${sealos_cert_secret_name}" ]; then
  sealos_cert_secret_name="$(read_yaml_value '.global.http.certSecretName')"
fi

[ -n "${sealos_jwt_secret}" ] || error "Failed to read sealos-config.data.jwtInternal"
[ -n "${sealos_cloud_domain}" ] || error "Failed to read sealos-config.data.cloudDomain"

jwt_secret=""
encrypt_key=""
jwt_secret_source="generated"
encrypt_key_source="generated"
existing_secret_name="$(find_existing_kite_secret || true)"
release_exists="false"
if is_existing_release; then
  release_exists="true"
fi

strict_reuse_enabled="false"
if [ "${STRICT_SECRET_REUSE}" = "true" ]; then
  strict_reuse_enabled="true"
fi

if [ "${release_exists}" = "true" ] && [ "${strict_reuse_enabled}" = "true" ] && [ -z "${existing_secret_name}" ]; then
  error "Existing release ${RELEASE_NAME} detected, but secret not found in namespace ${NAMESPACE}. Refuse to generate new keys when STRICT_SECRET_REUSE=true"
fi

if [ -n "${existing_secret_name}" ]; then
  info "Found existing secret ${existing_secret_name}, trying to reuse JWT_SECRET and KITE_ENCRYPT_KEY"
  jwt_secret="$(get_secret_data "${existing_secret_name}" "JWT_SECRET" || true)"
  encrypt_key="$(get_secret_data "${existing_secret_name}" "KITE_ENCRYPT_KEY" || true)"

  if [ -n "${jwt_secret}" ]; then
    jwt_secret_source="secret:${existing_secret_name}"
  fi
  if [ -n "${encrypt_key}" ]; then
    encrypt_key_source="secret:${existing_secret_name}"
  fi

  if [ "${strict_reuse_enabled}" = "true" ]; then
    [ -n "${jwt_secret}" ] || error "Missing JWT_SECRET in existing secret ${existing_secret_name} with STRICT_SECRET_REUSE=true"
    [ -n "${encrypt_key}" ] || error "Missing KITE_ENCRYPT_KEY in existing secret ${existing_secret_name} with STRICT_SECRET_REUSE=true"
  fi
fi

if [ -z "${jwt_secret}" ]; then
  warn "JWT_SECRET not found in existing secret, generating a new one"
  jwt_secret="$(openssl rand -hex 32)"
  jwt_secret_source="generated"
fi

if [ -z "${encrypt_key}" ]; then
  warn "KITE_ENCRYPT_KEY not found in existing secret, generating a new one"
  encrypt_key="$(openssl rand -hex 32)"
  encrypt_key_source="generated"
fi

info "Secret reuse summary: existing_secret=${existing_secret_name:-none}, jwt_source=${jwt_secret_source}, encrypt_source=${encrypt_key_source}, strict_reuse=${STRICT_SECRET_REUSE}"

helm_set_args=(
  --set-string "jwtSecret=${jwt_secret}"
  --set-string "encryptKey=${encrypt_key}"
  --set-string "sealos.jwtSecret=${sealos_jwt_secret}"
  --set-string "cloudDomain=${sealos_cloud_domain}"
  --set-string "cloudPort=${sealos_cloud_port:-443}"
  --set-string "httpPort=${sealos_http_port:-80}"
  --set-string "disableHttps=${sealos_disable_https:-false}"
  --set-string "certSecretName=${sealos_cert_secret_name:-wildcard-cert}"
  --set-string "platform.tlsRejectUnauthorized=${platform_tls_reject_unauthorized:-1}"
  --set-string "db.postgres.native.kubeblocksVersion=${kubeblocks_version:-0.8.2}"
)

if declare -f global_http_external_url >/dev/null 2>&1; then
  kite_external_url="$(global_http_external_url "kite.${sealos_cloud_domain}" 2>/dev/null || true)"
  [ -n "${kite_external_url}" ] && info "Kite external URL: ${kite_external_url}"
fi

if [ "${ENABLE_APP}" = "true" ]; then
  helm_set_args+=(--set "app.enabled=true")
fi

node_count="$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')"
if [ "${node_count}" = "1" ]; then
  warn "Single-node cluster detected, force app/database replicas to 1."
  helm_set_args+=(
    --set "replicaCount=1"
    --set "db.postgres.native.replicas=1"
  )
fi

helm_opts_arr=()
if [ -n "${HELM_OPTS}" ]; then
  # shellcheck disable=SC2206
  helm_opts_arr=(${HELM_OPTS})
fi

values_args=()
prepare_values_files

info "Installing chart ${CHART_DIR} into namespace ${NAMESPACE}"
helm upgrade -i "${RELEASE_NAME}" -n "${NAMESPACE}" --create-namespace "${CHART_DIR}" \
  "${values_args[@]}" \
  "${helm_set_args[@]}" \
  "${helm_opts_arr[@]}" \
  --wait
