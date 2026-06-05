## What This App Does

-   Port-based TCP proxy for Cloudflare Access tunnels.
-   The process listens on every configured public port and asks tunnel manager `get_tcp_route(public_port)` for the origin hostname, trying the local tunnel manager URL before the public fallback URL.
-   Successful route lookups are cached in memory for the process lifetime.
-   Local ports for cloudflared are dynamically reserved from `LOCAL_PORT_RANGE_START`-`LOCAL_PORT_RANGE_END`.

## Connection Handling

-   No protocol parsing is used for routing. Connections are forwarded as raw TCP.
-   If route lookup fails for the accepted public port, the connection is closed.
-   Full-duplex forwarding uses `io.Copy` in both directions after the local `cloudflared access tcp` process is ready.

## Files / Structure

-   `cmd/tcp_tunnel_proxy/main.go`: entrypoint; sets logging, opens one listener per public port, and hands each connection to the connection handler.
-   `configs/config.go`: environment-driven listen range, local/public tunnel manager URLs, timeouts, logging, and local cloudflared port range.
-   `internal/route_provider/route_provider.go`: swappable helper for resolving public port to origin hostname through tunnel manager.
-   `internal/connection_handler/connection_handler.go`: per-connection route lookup, tunnel preparation, and raw TCP proxying.
-   `internal/cloudflared_manager/cloudflared_manager.go`: cloudflared lifecycle, hostname-keyed reuse, restart on failure, idle teardown, refcounting, readiness wait, and port pool management.
-   `internal/cloudflared_manager/hostnames.go`: origin hostname validation helpers.
-   `README.md`: high-level overview, quick start, configuration, and behavior notes.

## Operational Notes

-   Requires `cloudflared` on PATH. Startup wait is `STARTUP_TIMEOUT`; idle teardown uses `IDLE_TIMEOUT`.
-   Restart logic: if cloudflared exits while refcount > 0, manager attempts restart.
-   Route deletion or port reuse can require proxy restart until route invalidation exists.

## Development Practices

-   Add or update automated tests for every new function or feature; keep coverage for route lookup, raw relay behavior, port management, and tunnel lifecycle helpers in sync with changes.

## TODO / Follow-ups

-   Add route invalidation or disk-backed reads if tunnel manager lookup becomes insufficient.
-   Consider adding observability/metrics if needed.
