# Mendr onboarding runbook

Prerequisites: control plane reachable, `GATEWAY_EDGE_API_KEY` issued, colorbook compose network includes `mendr-gateway`, OpenAPI files under `mendr/openapi/`.

## 1. Deploy (compose + auto-register)

```bash
cp .env.mendr.example .env.mendr
# fill GATEWAY_EDGE_API_KEY and MENDR_CONTROL_PLANE_URL

./mendr/scripts/deploy.sh          # or: .\mendr\scripts\deploy.ps1
```

`deploy.*` loads `.env.mendr`, runs `docker compose up --build -d`, then `register.*`.

Compose also starts a one-shot `mendr-register` service (Alpine + `mendr/scripts/docker-register.sh`) after `mendr-gateway` is healthy. Set `MENDR_AUTO_REGISTER=false` to skip.

Re-register without recreating the stack:

```bash
docker compose --env-file .env.mendr run --rm mendr-register
# or
./mendr/scripts/register.sh
```

Confirm edge logs show sync with `ingress` and a rising route version.

## 2. What registration does

`register.sh` / `register.ps1` (driven by `.env.mendr`):

1. Wait until the control plane answers
2. Register services (including synthetic `colorbook-public` / `colorbook-public-prod`)
3. Upsert instances (`http://gateway:8080`, …)
4. Dry-run then import each OpenAPI file
5. Upsert ingress-host-identity for `PUBLIC_HOST`, `PUBLIC_HOST_PROD`, `colorbook-internal`

## 3. Verify sync

- Edge container logs: snapshot version bump after import
- Optional: CP dashboard → services / routes for this tenant

## 4. Smoke tests

```bash
# Public page
curl -sS -H "Host: localhost" http://127.0.0.1/ | head

# Public must not serve internal
curl -sS -o /dev/null -w "%{http_code}\n" -H "Host: localhost" \
  http://127.0.0.1/internal/v1/canvas/append
# expect 404 / NO_MATCH / ROUTE_NOT_FOUND

# Private canvas (may be 401 from Go JWT — proves Mendr routed)
curl -sS -D- -H "Host: colorbook-internal" \
  http://127.0.0.1/v1/canvas/today -o NUL

# SSE (leave running briefly)
curl -sS -N -H "Host: localhost" http://127.0.0.1/v1/events
```

Browser: open `http://localhost/`, mint session, submit and vote in a minute, confirm SSE updates.

Orchestrator path: wait for minute close (tally → append or skip) and UTC seal if testing overnight.

## 5. Identity proof (optional)

Temporarily clear host identity or set `MENDR_HOST_IDENTITY_FALLBACK=false` and omit `X-Mendr-Key` → expect **401 IDENTITY_UNRESOLVED**. Restore identity before production traffic.

## Manual CP curls (reference)

```http
POST /api/services
{"name":"colorbook-gateway","baseUrl":"http://gateway:8080"}

POST /api/services/colorbook-gateway/instances
{"baseUrl":"http://gateway:8080","weight":100}

POST /api/services/import-openapi
Content-Type: application/x-yaml
(body = mendr/openapi/gateway-localhost.yaml)

POST /api/services/ingress-host-identity
{"host":"localhost","sourceService":"colorbook-public"}
```
