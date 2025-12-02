# TCP Tunnel Proxy (Cloudflare Access)

Dynamic TCP routing oracle that accepts public TCP connections, extracts TLS SNI, starts a Cloudflare Access TCP tunnel on demand to a private backend, and forwards raw bytes end-to-end. Supports arbitrary TCP protocols.

## Features

-   Listens on `:19000` (change in `listenAddr`).
-   SNI-based routing; rejects/short-circuits if SNI missing (currently returns `OK` placeholder on failure).
-   On-demand `cloudflared access tcp` per backend with refcounts and idle shutdown.
-   Full-duplex raw TCP piping with initial bytes replayed.

## Quick Start

1. Ensure `cloudflared` is installed and on PATH.
2. Edit `nodeConfigs` in `main.go` to map SNI hostnames to `{Hostname, LocalPort}` for your backends (local port is the `--url localhost:<port>` target for cloudflared).
3. Build/run:
    ```sh
    go run .
    # or
    go build -o tcp-tunnel-proxy && ./tcp-tunnel-proxy
    ```
4. Clients connect over TLS with SNI set (e.g., `psql "postgres://service.customer1.example.com:19000/db?sslmode=require"`). Non-TLS/no-SNI currently get an `OK` and close (temporary behavior).

## Behavior Notes

-   PROXY protocol: If a load balancer prepends PROXY v1/v2, it is consumed and forwarded to the backend.
-   PostgreSQL: SSLRequest (8-byte prelude) is accepted (`S`), then TLS ClientHello is parsed for SNI; backend’s `S` is consumed before piping.
-   Cloudflared lifecycle: starts on first connection per SNI, waits for local port readiness (`startupTimeout`), increments refcounts; when refcount hits zero, an idle timer (`idleTimeout`, default 300s) kills the tunnel.
-   Crashes: if cloudflared exits while connections are active, the manager attempts restart.

## Configuration

-   `listenAddr`, `idleTimeout`, `startupTimeout`, `readHelloTimeout` constants in `main.go`.
-   Update `nodeConfigs` for your hostnames and ports.

## Caveats / TODO

-   Temporary SNI failure handling sends `OK\n` instead of rejecting; switch to reject for production.
-   No persistence/log rotation; relies on stdout logging.
