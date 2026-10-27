# Mendr onboarding runbook

Prerequisites: control plane reachable; **two** tenants with edge API keys; OpenAPI under `mendr/openapi/`; `teammendr/themendr:latest` available for the host arch (arm64 on Oracle A1).

## 0. Control plane tenants

On the CP host (e.g. Netcup):

1. Keep default tenant `00000000-0000-0000-0000-000000000001` for the **public** edge.
2. Insert private tenant `00000000-0000-0000-0000-000000000002` into `tenants` if missing.
3. Mint a `mendr_…` API key for each tenant (admin `/api/internal/admin/api-keys` or SQL hash insert).
4. Put keys in Oracle `.env.mendr` as `GATEWAY_EDGE_API_KEY_PUBLIC` and `GATEWAY_EDGE_API_KEY_PRIVATE`.

## 1. Deploy

```bash
cp .env.mendr.example .env.mendr
# fill GATEWAY_EDGE_API_KEY_PUBLIC / _PRIVATE and MENDR_CONTROL_PLANE_URL

./mendr/scripts/deploy.sh          # or: .\mendr\scripts\deploy.ps1
```

`deploy.*` brings up both edges, then registers **public** and **private** sides.

Re-register without recreating the stack:

```bash
docker compose --env-file .env.mendr run --rm mendr-register-public
docker compose --env-file .env.mendr run --rm mendr-register-private
# or
MENDR_SIDE=public ./mendr/scripts/register.sh
MENDR_SIDE=private ./mendr/scripts/register.sh
```

## 2. What registration does

**Public** (`MENDR_SIDE=public`, tenant `…0001`):

1. Services: `sharedpage-public`, `sharedpage-public-prod`, `sharedpage-gateway`
2. Import `gateway-localhost.yaml`, `gateway-sharedpage.yaml`
3. Host identity: `localhost` → `sharedpage-public`, `sharedpage.app` → `sharedpage-public-prod`

**Private** (`MENDR_SIDE=private`, tenant `…0002`):

1. Services: `sharedpage-gateway` (identity), submission, voting, canvas, archive, realtime
2. Import mesh OpenAPI (`x-mendr-host: sharedpage-internal`)
3. Host identity: `sharedpage-internal` → `sharedpage-gateway`

## 3. Verify sync

```bash
# Public edge redis — only public hosts
docker compose --env-file .env.mendr exec mendr-edge-redis-public redis-cli KEYS 'mendr:hostident:*'
docker compose --env-file .env.mendr exec mendr-edge-redis-public redis-cli GET mendr:ingress:tables | head -c 400

# Private edge redis — only sharedpage-internal
docker compose --env-file .env.mendr exec mendr-edge-redis-private redis-cli KEYS 'mendr:hostident:*'
```

## 4. Smoke tests

```bash
# Public page
curl -sS -H "Host: sharedpage.app" http://127.0.0.1/ | head

# Public must not serve private Host / internal
curl -sS -o /dev/null -w "%{http_code}\n" -H "Host: sharedpage-internal" \
  http://127.0.0.1/v1/canvas/today
curl -sS -o /dev/null -w "%{http_code}\n" -H "Host: sharedpage.app" \
  http://127.0.0.1/internal/v1/canvas/append

# Private edge only inside Docker
docker compose --env-file .env.mendr exec gateway \
  wget -qO- --header='Host: sharedpage-internal' http://mendr-gateway-private/v1/canvas/today | head

# SSE
curl -sS -N -H "Host: sharedpage.app" http://127.0.0.1/v1/events
```

Browser: open the public site, mint session, submit and vote, confirm SSE updates.

## Manual CP curls (reference)

```http
POST /api/services
X-Tenant-Id: 00000000-0000-0000-0000-000000000001
X-Api-Key: <public mendr_ key>
{"name":"sharedpage-gateway","baseUrl":"http://gateway:8080"}

POST /api/services/ingress-host-identity
{"host":"sharedpage.app","sourceService":"sharedpage-public-prod"}
```
