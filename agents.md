## What This App Does

-   Dynamic TCP routing oracle on `:19000`. Uses TLS SNI to choose a backend, spins up `cloudflared access tcp` on demand, and pipes raw TCP bytes end-to-end.
-   Tunnels are keyed by backend hostname (from `NodeConfig`); SNI → backend mapping is in `nodeConfigs`. Multiple SNI entries can point at the same backend hostname and will share a tunnel.
-   Local ports for cloudflared are **dynamically** reserved from a pool (`portRangeStart`–`portRangeEnd`); no per-node static port in config anymore.

## Connection Handling

-   Supports PROXY protocol v1/v2: headers are consumed and replayed to backend.
-   PostgreSQL support: detects SSLRequest prelude, replies `S` so client sends TLS ClientHello, then consumes backend’s SSL response before piping. SNI is parsed from the ClientHello.
-   If SNI extraction fails, current temporary behavior writes `OK\n` and closes (meant for debugging; flip to hard reject for production).
-   Full-duplex forwarding via `io.Copy`; initial bytes (PROXY/SSLRequest/TLS record plus any buffered data) are replayed to backend before streaming.

## Files / Structure

-   `config.go`: constants (listen address, timeouts, port pool) and `nodeConfigs` map (`SNI -> NodeConfig{Hostname}`).
-   `sni.go`: PROXY/SSLRequest handling, TLS ClientHello parsing for SNI, backend Postgres SSL response consumption.
-   `node_manager.go`: port pool, tunnel lifecycle (start/restart/idle shutdown), refcounting, readiness wait, cloudflared logging.
-   `main.go`: listener and per-connection flow.
-   `README.md`: high-level overview (note: mentions static local ports—outdated relative to dynamic port pool).

## Operational Notes

-   Requires `cloudflared` on PATH. Startup wait is `startupTimeout`; idle teardown uses `idleTimeout`.
-   Restart logic: if cloudflared exits while refcount > 0, manager attempts restart.

## TODO / Follow-ups

-   Replace the temporary SNI-failure `OK` response with proper rejection once debugging is done.
-   Update `README.md` to reflect dynamic port pool and current file layout.
-   Consider exposing configuration via file/env instead of in-code map; add observability/metrics if needed.
