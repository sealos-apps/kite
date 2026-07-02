#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  mirror-helm-chart-images.sh --chart <chart-ref> --registry <offline-registry> [options]

Examples:
  scripts/mirror-helm-chart-images.sh \
    --chart oci://hub.192.168.0.62.nip.io/kite-helm/nginx \
    --version 25.0.12 \
    --registry hub.192.168.0.62.nip.io

The script renders the chart twice:
  1. with the provided values as-is to discover source workload images
  2. with global.imageRegistry=<offline-registry> to discover target images

It then copies each source image to the rendered offline target image.

Requirements: helm, python3, crane.
EOF
}

CHART=""
VERSION=""
REGISTRY=""
NAMESPACE="default"
RELEASE_NAME="offline-preview"
VALUES_FILE=""
DRY_RUN="false"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --chart)
      CHART="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --registry)
      REGISTRY="${2:-}"
      shift 2
      ;;
    --namespace)
      NAMESPACE="${2:-}"
      shift 2
      ;;
    --release-name)
      RELEASE_NAME="${2:-}"
      shift 2
      ;;
    --values)
      VALUES_FILE="${2:-}"
      shift 2
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ -n "$CHART" ] || { echo "--chart is required" >&2; exit 2; }
[ -n "$REGISTRY" ] || { echo "--registry is required" >&2; exit 2; }

for bin in helm python3 crane; do
  command -v "$bin" >/dev/null 2>&1 || {
    echo "required command not found: $bin" >&2
    exit 127
  }
done

REGISTRY="${REGISTRY#http://}"
REGISTRY="${REGISTRY#https://}"
REGISTRY="${REGISTRY%/}"

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/kite-chart-images.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT

source_rendered="$tmp_dir/source.yaml"
target_rendered="$tmp_dir/target.yaml"
target_defaults="$tmp_dir/target-defaults.yaml"
copy_plan="$tmp_dir/copy-plan.tsv"

cat > "$target_defaults" <<EOF
global:
  imageRegistry: "$REGISTRY"
  security:
    allowInsecureImages: true
EOF

chart_helm_args=(
  template "$RELEASE_NAME" "$CHART"
  --namespace "$NAMESPACE"
)

if [ -n "$VERSION" ]; then
  chart_helm_args+=(--version "$VERSION")
fi

source_helm_args=("${chart_helm_args[@]}")
target_helm_args=("${chart_helm_args[@]}" -f "$target_defaults")
if [ -n "$VALUES_FILE" ]; then
  source_helm_args+=(-f "$VALUES_FILE")
  target_helm_args+=(-f "$VALUES_FILE")
fi

helm "${source_helm_args[@]}" > "$source_rendered"
helm "${target_helm_args[@]}" > "$target_rendered"

python3 - "$source_rendered" "$target_rendered" "$NAMESPACE" > "$copy_plan" <<'PY'
import sys
from pathlib import Path
import yaml

source_path = Path(sys.argv[1])
target_path = Path(sys.argv[2])
default_namespace = sys.argv[3]

def podspec_images(resource_key, spec):
    if not isinstance(spec, dict):
        return
    for key in ("initContainers", "containers", "ephemeralContainers"):
        for index, container in enumerate(spec.get(key) or []):
            if not isinstance(container, dict) or not container.get("image"):
                continue
            container_name = container.get("name") or str(index)
            identity = resource_key + (key, container_name)
            yield identity, str(container["image"])

def resource_key(doc):
    api_version = doc.get("apiVersion") or ""
    kind = doc.get("kind") or ""
    metadata = doc.get("metadata") or {}
    namespace = metadata.get("namespace") or default_namespace
    name = metadata.get("name") or ""
    return (api_version, kind, namespace, name)

def rendered_images(path):
    images = {}
    for doc_index, doc in enumerate(yaml.safe_load_all(path.read_text())):
        if not isinstance(doc, dict):
            continue
        kind = (doc.get("kind") or "").lower()
        key = resource_key(doc)
        if not key[1] or not key[3]:
            continue
        spec = doc.get("spec") or {}
        records = []
        if kind == "pod":
            records = list(podspec_images(key, spec))
        elif kind in {"deployment", "statefulset", "daemonset", "replicaset", "replicationcontroller", "job"}:
            records = list(podspec_images(key, ((spec.get("template") or {}).get("spec") or {})))
        elif kind == "cronjob":
            records = list(podspec_images(key, (((spec.get("jobTemplate") or {}).get("spec") or {}).get("template") or {}).get("spec") or {}))
        for identity, image in records:
            if identity in images:
                raise SystemExit(f"duplicate workload container identity in {path}: {identity}")
            images[identity] = image
    return images

source_images = rendered_images(source_path)
target_images = rendered_images(target_path)

if not target_images:
    print("# no workload images rendered")
    raise SystemExit(0)

source_keys = set(source_images)
target_keys = set(target_images)
if source_keys != target_keys:
    missing = sorted(source_keys - target_keys)
    added = sorted(target_keys - source_keys)
    if missing:
        print("source workload containers missing from target render:", file=sys.stderr)
        for key in missing:
            print("  " + "/".join(key), file=sys.stderr)
    if added:
        print("target workload containers missing from source render:", file=sys.stderr)
        for key in added:
            print("  " + "/".join(key), file=sys.stderr)
    raise SystemExit(1)

seen = set()
for key in sorted(source_keys):
    source = source_images[key]
    target = target_images[key]
    item = (source, target)
    if item in seen:
        continue
    seen.add(item)
    print(f"{source}\t{target}")
PY

if grep -q '^# no workload images rendered$' "$copy_plan"; then
  echo "no workload images rendered"
  exit 0
fi

if [ ! -s "$copy_plan" ]; then
  echo "no workload images rendered"
  exit 0
fi

sort -u "$copy_plan" -o "$copy_plan"

echo "Rendered image copy plan:"
while IFS="$(printf '\t')" read -r source target; do
  [ -n "$source" ] || continue
  [ -n "$target" ] || continue
  case "$target" in
    "$REGISTRY"/*)
      ;;
    *)
      echo "image does not use $REGISTRY after rendering: $target" >&2
      echo "the chart may not support global.imageRegistry; provide --values with chart-specific image overrides" >&2
      exit 1
      ;;
  esac
  if [ "$DRY_RUN" = "true" ]; then
    echo "would copy $source -> $target"
  else
    echo "copy $source -> $target"
    crane copy "$source" "$target"
  fi
done < "$copy_plan"
