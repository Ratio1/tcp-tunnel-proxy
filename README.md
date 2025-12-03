# TCP Tunnel Proxy (Cloudflare Access)

Dynamic TCP routing oracle that accepts public TCP connections, extracts TLS SNI, starts a Cloudflare Access TCP tunnel on demand to a private backend, and forwards raw bytes end-to-end. Supports arbitrary TCP protocols.

## Features

-   Listens on `:19000` (change in `listenAddr`).
-   SNI-based routing with deterministic hostname derivation (`cft-` + SNI with the first dot replaced by `-`); rejects/short-circuits if SNI missing (currently returns `OK` placeholder on failure).
-   On-demand `cloudflared access tcp` per backend with refcounts and idle shutdown.
-   Full-duplex raw TCP piping with initial bytes replayed.

## Quick Start

1. Ensure `cloudflared` is installed and on PATH.
2. Ensure your Cloudflare Access/DNS hostnames follow the rule `cft-<SNI with the first "." replaced by "-">` (e.g., SNI `db-123.ratio1.link` → tunnel hostname `cft-db-123.ratio1.link`).
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
-   Hostname derivation rule lives in `deriveTunnelHostname` (prefix `cft-`, replace the first dot with `-`); adjust if you need a different mapping pattern.

## Caveats / TODO

-   Temporary SNI failure handling sends `OK\n` instead of rejecting; switch to reject for production.
-   No persistence/log rotation; relies on stdout logging.
