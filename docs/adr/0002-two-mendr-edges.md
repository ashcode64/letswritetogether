# ADR 0002: Two Mendr edges with separate tenants

## Status

Accepted

## Context

ADR 0001 replaced both Caddys with **one** Mendr edge and two ingress hosts. That relies on control-plane route tables as the firewall: a mistaken public OpenAPI import could expose `/internal` on the internet Host. Mendr also syncs the **entire tenant snapshot** to every edge of a tenant, so two edges under one tenant would still put private routes on the public edge.

## Decision

Run **two** Mendr data-plane containers:

| Edge | Ports | Tenant | Routes |
|------|-------|--------|--------|
| `mendr-gateway-public` | `:80` (local); unpublished behind tunnel in prod | PUBLIC (`…0001`) | `sharedpage-gateway` only (`localhost`, `sharedpage.app`) |
| `mendr-gateway-private` | none (Docker network only) | PRIVATE (`…0002`) | mesh + `Host: sharedpage-internal` |

`PROXY_URL=http://mendr-gateway-private:80` and `PROXY_HOST=sharedpage-internal`. Registry service names use the `sharedpage-` prefix.

Latency is unchanged versus dual-Caddy / dual-host-one-edge (same two Mendr hops).

## Consequences

- Operators must mint **two** edge API keys and create the private tenant on the CP.
- Registration is split (`MENDR_SIDE=public|private`).
- Public edge crash does not take down orch→tally via the private edge (and vice versa).
- Slightly more compose/ops surface (two Redis, two sync clients).
