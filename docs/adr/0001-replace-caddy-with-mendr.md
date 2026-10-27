# ADR 0001: Replace both Caddys with Mendr dual-host ingress

## Status

Superseded by [0002-two-mendr-edges.md](0002-two-mendr-edges.md)

## Context

Colorbook used two Caddy containers:

- **Public** (`edge`) — `:80` → `gateway:8080` only; no `/internal`
- **Private** (`proxy`) — unpublished path router for submission, voting, canvas, archive, realtime

Mendr data plane provides transparent HTTP ingress synced from the control plane (OpenAPI / host identity).

## Decision (historical)

First cut used **one** Mendr data-plane gateway with **two ingress hosts** (`localhost` / `sharedpage.app` vs private mesh host). That was later replaced by two edges and two tenants for complete isolation — see ADR 0002. Registry names are now `sharedpage-*` and the private Host is `sharedpage-internal`.

## Consequences

- See ADR 0002 for the current topology.
