## What This App Does

-   Dynamic TCP routing oracle on `:19000`. Uses TLS SNI to choose a backend, spins up `cloudflared access tcp` on demand, and pipes raw TCP bytes end-to-end.
-   Tunnels are keyed by a derived backend hostname: prefix `cft-` to the incoming SNI and replace its first `.` with `-` (e.g., `db-123.ratio1.link` → `cft-db-123.ratio1.link`). No static map or external lookup is used.
-   Local ports for cloudflared are **dynamically** reserved from a pool (`portRangeStart`–`portRangeEnd`); no per-node static port in config anymore.

## Connection Handling

-   Supports PROXY protocol v1/v2: headers are consumed and replayed to backend.
-   PostgreSQL support: detects SSLRequest prelude, replies `S` so client sends TLS ClientHello, then consumes backend’s SSL response before piping. SNI is parsed from the ClientHello.
-   If SNI extraction fails, current temporary behavior writes `OK\n` and closes (meant for debugging; flip to hard reject for production).
-   Full-duplex forwarding via `io.Copy`; initial bytes (PROXY/SSLRequest/TLS record plus any buffered data) are replayed to backend before streaming.

## Files / Structure

-   `cmd/tcp_tunnel_proxy/main.go`: entrypoint; sets logging, builds the node manager from config defaults, listens on `ListenAddr`, and hands each connection to the connection handler.
-   `configs/config.go`: default listen address, timeouts, and dynamic port range; stub for env-driven overrides.
-   `internal/connection_handler/connection_handler.go`: per-connection flow—extract SNI, prepare tunnel, replay prelude bytes, and proxy streams.
-   `internal/connection_handler/sni.go`: PROXY v1/v2 handling, PostgreSQL SSLRequest negotiation, TLS ClientHello parsing for SNI, and buffer pooling (+ tests in `connection_handler_test.go`, `sni_test.go`).
-   `internal/cloudflared_manager/cloudflared_manager.go`: cloudflared lifecycle (start, restart on failure, idle teardown), refcounting, readiness wait, and port pool management.
-   `internal/cloudflared_manager/hostnames.go`: hostname derivation/validation helpers for the `cft-<sni>` rule (+ tests in `hostnames_test.go`, `portpool_test.go`).
-   `README.md`: high-level overview, quick start, and behavior notes.

## Operational Notes

-   Everything should be optimized to handle many connections, be fast, and have very small overhead time.
-   Requires `cloudflared` on PATH. Startup wait is `startupTimeout`; idle teardown uses `idleTimeout`.
-   Restart logic: if cloudflared exits while refcount > 0, manager attempts restart.

## Development Practices

-   Add or update automated tests for every new function or feature; keep coverage for SNI parsing, port management, and tunnel lifecycle helpers in sync with changes.

## TODO / Follow-ups

-   Replace the temporary SNI-failure `OK` response with proper rejection once debugging is done.
-   Expose configuration (timeouts/derivation rule/port range) via env or flags; wire `LoadConfigENV`.
-   Consider adding observability/metrics if needed.
