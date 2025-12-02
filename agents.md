# Agents Handoff

## Current State

-   Go TCP routing oracle that listens on `:19000`.
-   Extracts SNI from TLS ClientHello (after optional PROXY protocol headers and optional PostgreSQL SSLRequest prelude).
-   Maps SNI to `nodeConfigs` and spins up `cloudflared access tcp --hostname <node> --url localhost:<port>` on demand with refcounts and idle teardown.
-   Forwards raw TCP bytes between client and the local cloudflared port; full-duplex via `io.Copy`.
-   Temporary behavior: if SNI extraction fails, the handler writes `OK\n` and closes instead of rejecting.
-   PROXY protocol v1/v2 headers are consumed and replayed to the backend.

## Key Files

-   `main.go`: all logic (listener, SNI parsing, Postgres SSLRequest handling, node manager, forwarding).
-   `go.mod`: module name `tcp-tunnel-proxy`.

## Config to Edit

-   `nodeConfigs` map in `main.go` controls SNI → `{Hostname, LocalPort}`.
-   Timeouts: `idleTimeout`, `startupTimeout`, `readHelloTimeout` constants.

## Gotchas / Notes

-   Cloudflared must be on PATH.
-   NodeManager restarts cloudflared if it exits while connections are active; idles out after `idleTimeout`.
-   Keep an eye on the temporary SNI-failure `OK` response if production behavior should reject instead.
