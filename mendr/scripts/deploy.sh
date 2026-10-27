#!/usr/bin/env bash
# Bring up sharedpage + dual Mendr edges, then register both tenants.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ENV_FILE=""
for cand in .env.mendr .env; do
  if [[ -f "$ROOT/$cand" ]]; then
    ENV_FILE="$cand"
    break
  fi
done

if [[ -z "$ENV_FILE" ]]; then
  echo "No .env.mendr or .env found. Copy .env.mendr.example first:" >&2
  echo "  cp .env.mendr.example .env.mendr" >&2
  exit 1
fi

echo "==> using env file: $ENV_FILE"
set -a
# shellcheck disable=SC1090
source <(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' -e 's/\r$//' "$ROOT/$ENV_FILE" | grep -E '^[A-Za-z_][A-Za-z0-9_]*=')
set +a
export MENDR_ENV_LOADED=1

COMPOSE=(docker compose --env-file "$ENV_FILE")
if [[ -f "$ROOT/docker-compose.prod.yml" && "${DEPLOY_PROD:-}" == "1" ]]; then
  COMPOSE+=(-f docker-compose.yml -f docker-compose.prod.yml)
fi

echo "==> docker compose up"
"${COMPOSE[@]}" up --build -d

wait_healthy() {
  local svc="$1"
  local tries=0
  until "${COMPOSE[@]}" ps "$svc" 2>/dev/null | grep -qi healthy; do
    tries=$((tries + 1))
    if (( tries > 60 )); then
      echo "WARN: $svc not healthy yet; continuing" >&2
      return 0
    fi
    sleep 2
  done
}

echo "==> waiting for Mendr edges"
wait_healthy mendr-gateway-public
wait_healthy mendr-gateway-private

echo "==> registering public + private tenants"
MENDR_SIDE=public bash "$ROOT/mendr/scripts/register.sh"
MENDR_SIDE=private bash "$ROOT/mendr/scripts/register.sh"

echo "==> deploy complete"
"${COMPOSE[@]}" ps
