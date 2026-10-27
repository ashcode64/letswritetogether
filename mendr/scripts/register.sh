#!/usr/bin/env bash
# Idempotent Mendr CP registration for colorbook dual-host ingress.
# Loads .env.mendr / .env from the repo root when present.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OPENAPI_DIR="${MENDR_OPENAPI_DIR:-$ROOT/mendr/openapi}"

load_env_file() {
  local f="$1"
  [[ -f "$f" ]] || return 0
  echo "==> loading $f"
  set -a
  # shellcheck disable=SC1090
  source <(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' -e 's/\r$//' "$f" | grep -E '^[A-Za-z_][A-Za-z0-9_]*=')
  set +a
}

# Prefer explicit env already in the shell; then .env.mendr; then .env
if [[ -z "${MENDR_ENV_LOADED:-}" ]]; then
  load_env_file "$ROOT/.env.mendr"
  load_env_file "$ROOT/.env"
  export MENDR_ENV_LOADED=1
fi

if [[ "${MENDR_AUTO_REGISTER:-true}" == "false" || "${MENDR_AUTO_REGISTER:-true}" == "0" ]]; then
  echo "MENDR_AUTO_REGISTER disabled — skipping registration"
  exit 0
fi

# Register may run on the host while the edge uses host.docker.internal.
resolve_cp_url() {
  local url="${MENDR_REGISTER_CONTROL_PLANE_URL:-${MENDR_CP_URL:-${MENDR_CONTROL_PLANE_URL:-http://127.0.0.1:8095}}}"
  url="${url%/}"
  if [[ ! -f /.dockerenv ]] && [[ "$url" == *"host.docker.internal"* ]]; then
    url="${url//host.docker.internal/127.0.0.1}"
  fi
  printf '%s' "$url"
}

CP="$(resolve_cp_url)"
TENANT="${MENDR_TENANT_ID:-00000000-0000-0000-0000-000000000001}"
PUBLIC_HOST="${PUBLIC_HOST:-localhost}"
PUBLIC_HOST_PROD="${PUBLIC_HOST_PROD:-sharedpage.app}"
WAIT_SEC="${MENDR_REGISTER_WAIT_SEC:-120}"

auth_args=()
if [[ -n "${GATEWAY_INTERNAL_API_KEY:-}" ]]; then
  auth_args+=(-H "Authorization: Bearer ${GATEWAY_INTERNAL_API_KEY}")
  auth_args+=(-H "X-Api-Key: ${GATEWAY_INTERNAL_API_KEY}")
fi
auth_args+=(-H "X-Tenant-Id: ${TENANT}")

curl_cp() {
  curl -sS --connect-timeout 5 --max-time 60 "${auth_args[@]}" "$@"
}

wait_for_cp() {
  echo "==> waiting for CP at $CP (up to ${WAIT_SEC}s)"
  local i=0
  while (( i < WAIT_SEC )); do
    if curl_cp -o /dev/null -w '' "$CP/health" 2>/dev/null \
      || curl_cp -o /dev/null -w '' "$CP/actuator/health" 2>/dev/null \
      || curl_cp -o /dev/null -w '' "$CP/api/services" 2>/dev/null; then
      echo "==> CP reachable"
      return 0
    fi
    sleep 2
    i=$((i + 2))
  done
  echo "ERROR: control plane not reachable at $CP" >&2
  exit 1
}

echo "==> CP: $CP"
echo "==> OpenAPI: $OPENAPI_DIR"
wait_for_cp

register_service() {
  local name="$1" url="${2:-}"
  local body
  if [[ -n "$url" ]]; then
    body=$(printf '{"name":"%s","baseUrl":"%s","healthEndpoint":"/healthz"}' "$name" "$url")
  else
    body=$(printf '{"name":"%s","description":"synthetic ingress source"}' "$name")
  fi
  echo "register $name"
  curl_cp -H "Content-Type: application/json" -d "$body" "$CP/api/services" >/dev/null || true
}

upsert_instance() {
  local name="$1" url="$2"
  echo "instance $name -> $url"
  curl_cp -H "Content-Type: application/json" \
    -d "$(printf '{"baseUrl":"%s","weight":100}' "$url")" \
    "$CP/api/services/$name/instances" >/dev/null || true
}

import_openapi() {
  local file="$1"
  [[ -f "$file" ]] || { echo "ERROR: missing $file" >&2; exit 1; }
  echo "dry-run $(basename "$file")"
  local dry
  dry=$(curl_cp -H "Content-Type: application/x-yaml" --data-binary @"$file" \
    "$CP/api/services/import-openapi/dry-run" || true)
  echo "$dry" | head -c 400
  echo
  case "$(basename "$file")" in
    submission.yaml|voting.yaml|canvas.yaml|archive.yaml|realtime.yaml)
      if ! grep -q 'x-mendr-host: colorbook-internal' "$file"; then
        echo "ERROR: $file missing x-mendr-host: colorbook-internal" >&2
        exit 1
      fi
      ;;
    gateway-localhost.yaml)
      if ! grep -q "x-mendr-host: ${PUBLIC_HOST}" "$file" && ! grep -q 'x-mendr-host: localhost' "$file"; then
        echo "ERROR: $file missing public x-mendr-host" >&2
        exit 1
      fi
      ;;
    gateway-sharedpage.yaml)
      if ! grep -q "x-mendr-host: ${PUBLIC_HOST_PROD}" "$file" && ! grep -q 'x-mendr-host: sharedpage.app' "$file"; then
        echo "ERROR: $file missing prod public x-mendr-host" >&2
        exit 1
      fi
      ;;
  esac
  echo "import $(basename "$file")"
  curl_cp -H "Content-Type: application/x-yaml" --data-binary @"$file" \
    "$CP/api/services/import-openapi" >/dev/null
}

host_ident() {
  local host="$1" source="$2"
  echo "hostident $host -> $source"
  curl_cp -H "Content-Type: application/json" \
    -d "$(printf '{"host":"%s","sourceService":"%s"}' "$host" "$source")" \
    "$CP/api/services/ingress-host-identity" >/dev/null
}

register_service "colorbook-public"
register_service "colorbook-public-prod"
register_service "colorbook-gateway" "http://gateway:8080"
register_service "colorbook-submission" "http://submission:8080"
register_service "colorbook-voting" "http://voting:8080"
register_service "colorbook-canvas" "http://canvas:8080"
register_service "colorbook-archive" "http://archive:8080"
register_service "colorbook-realtime" "http://realtime:8080"

upsert_instance "colorbook-gateway" "http://gateway:8080"
upsert_instance "colorbook-submission" "http://submission:8080"
upsert_instance "colorbook-voting" "http://voting:8080"
upsert_instance "colorbook-canvas" "http://canvas:8080"
upsert_instance "colorbook-archive" "http://archive:8080"
upsert_instance "colorbook-realtime" "http://realtime:8080"

for f in \
  gateway-localhost.yaml \
  gateway-sharedpage.yaml \
  submission.yaml \
  voting.yaml \
  canvas.yaml \
  archive.yaml \
  realtime.yaml
do
  import_openapi "$OPENAPI_DIR/$f"
done

host_ident "$PUBLIC_HOST" "colorbook-public"
host_ident "$PUBLIC_HOST_PROD" "colorbook-public-prod"
host_ident "colorbook-internal" "colorbook-gateway"

echo
echo "==> Registration complete. After edge sync:"
cat <<EOF
curl -sS -H 'Host: ${PUBLIC_HOST}' http://127.0.0.1/ | head
curl -sS -o /dev/null -w '%{http_code}\\n' -H 'Host: ${PUBLIC_HOST}' http://127.0.0.1/internal/v1/canvas/append
curl -sS -D- -H 'Host: colorbook-internal' http://127.0.0.1/v1/canvas/today -o /dev/null
EOF
echo "Done."
