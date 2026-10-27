# ADR 0001: Replace both Caddys with Mendr dual-host ingress

## Status

Accepted

## Context

Colorbook used two Caddy containers:

- **Public** (`edge`) — `:80` → `gateway:8080` only; no `/internal`
- **Private** (`proxy`) — unpublished path router for submission, voting, canvas, archive, realtime

Mendr data plane provides transparent HTTP ingress synced from the control plane (OpenAPI / host identity). We want one edge in the call path for both browser and service-to-service REST.

## Decision

Use **one** Mendr data-plane gateway with **two ingress hosts**:

| Host | Role |
|------|------|
| `localhost` / `sharedpage.app` | Public → `colorbook-gateway` only |
| `colorbook-internal` | Private mesh (old `Caddyfile.proxy`) |

`PROXY_URL` points at Mendr. Gateway and orchestrator set `Host: colorbook-internal` on reverse-proxy / outbound calls.

Host identity maps each hostname to a source service (`colorbook-public`, `colorbook-public-prod`, or `colorbook-gateway`). Explicit OpenAPI routes are required; Mendr does not catch-all.

Envelope-only rewrite is deferred. Cloudflare Tunnel targets `http://mendr-gateway:80`.

## Consequences

- Control plane must be reachable for route sync; edge shares Docker DNS with Go services.
- Registration scripts must set `x-mendr-host`, instances, and ingress-host-identity or traffic fails with `401 IDENTITY_UNRESOLVED` / `404 ROUTE_NOT_FOUND`.
- Caddy is removed entirely; Mendr is the only public and private HTTP path.
