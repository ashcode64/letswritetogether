#!/usr/bin/env bash
# Bring up colorbook + Mendr, then register routes from .env.mendr / .env.
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
# Export for compose variable substitution and for register.sh
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

echo "==> waiting for mendr-gateway healthy"
tries=0
until "${COMPOSE[@]}" ps mendr-gateway 2>/dev/null | grep -qi healthy; do
  tries=$((tries + 1))
  if (( tries > 60 )); then
    echo "WARN: mendr-gateway not healthy yet; registering anyway" >&2
    break
  fi
  sleep 2
done

# Compose already runs mendr-register; also run on host so operators see logs
# and so registration works even if the one-shot container image pull fails.
echo "==> registering with control plane"
bash "$ROOT/mendr/scripts/register.sh"

echo "==> deploy complete"
"${COMPOSE[@]}" ps
