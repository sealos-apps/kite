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

get_sealos_config() {
  local key=$1
  kubectl get configmap sealos-config -n sealos-system -o "jsonpath={.data.${key}}"
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

sealos_jwt_secret="$(get_sealos_config jwtInternal || true)"
sealos_cloud_domain="$(get_sealos_config cloudDomain || true)"

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
)

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

info "Installing chart charts/kite into namespace ${NAMESPACE}"
helm upgrade -i "${RELEASE_NAME}" -n "${NAMESPACE}" --create-namespace charts/kite \
  "${helm_set_args[@]}" \
  "${helm_opts_arr[@]}" \
  --wait
