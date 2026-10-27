#!/bin/sh
# Entrypoint for the mendr-register Compose one-shot service.
set -eu

if [ "${MENDR_AUTO_REGISTER:-true}" = "false" ] || [ "${MENDR_AUTO_REGISTER:-true}" = "0" ]; then
  echo "MENDR_AUTO_REGISTER disabled — exiting"
  exit 0
fi

echo "==> installing curl/bash for registration"
apk add --no-cache curl bash >/dev/null

export MENDR_OPENAPI_DIR="${MENDR_OPENAPI_DIR:-/mendr/openapi}"
export MENDR_ENV_LOADED=1

exec bash /mendr/scripts/register.sh
