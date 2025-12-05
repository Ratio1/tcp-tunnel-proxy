# TCP Tunnel Proxy (Cloudflare Access)

Dynamic TCP routing oracle that accepts public TCP connections, extracts TLS SNI, starts a Cloudflare Access TCP tunnel on demand to a private backend, and forwards raw bytes end-to-end. Supports arbitrary TCP protocols.

## Features

-   Listens on `:19000` (change in `listenAddr`).
-   SNI-based routing with deterministic hostname derivation (`cft-`); rejects/short-circuits if SNI missing (currently returns `OK` placeholder on failure).
-   On-demand `cloudflared access tcp` per backend with refcounts and idle shutdown.
-   Full-duplex raw TCP piping with initial bytes replayed.

## Quick Start

1. Ensure `cloudflared` is installed and on PATH.
2. Ensure your Cloudflare Access/DNS hostnames follow the rule `cft-<SNI>` (e.g., SNI `db-123.ratio1.link` → tunnel hostname `cft-db-123.ratio1.link`).
3. Build/run:
    ```sh
    go run .
    # or
    go build -o tcp-tunnel-proxy && ./tcp-tunnel-proxy cmd/tcp_tunnel_proxy/main.go
    ```
4. Clients connect over TLS with SNI set (e.g., `psql "postgres://service.customer1.example.com:19000/db?sslmode=require"`). Non-TLS/no-SNI currently get an `OK` and close (temporary behavior).

## Downloads

-   [Linux amd64](https://github.com/Ratio1/tcp-tunnel-proxy/releases/latest/download/tcp-tunnel-proxy-linux-amd64.tar.gz)
-   [Linux arm64](https://github.com/Ratio1/tcp-tunnel-proxy/releases/latest/download/tcp-tunnel-proxy-linux-arm64.tar.gz)

## Run as a systemd Service (Linux)

1. Download and install the release binary (pick the right arch):
    ```sh
    curl -L https://github.com/Ratio1/tcp-tunnel-proxy/releases/latest/download/tcp-tunnel-proxy-linux-amd64.tar.gz -o /tmp/tcp-tunnel-proxy.tar.gz
    sudo tar -xzf /tmp/tcp-tunnel-proxy.tar.gz -C /usr/local/bin
    sudo mv /usr/local/bin/tcp-tunnel-proxy-linux-amd64 /usr/local/bin/tcp-tunnel-proxy
    sudo chmod +x /usr/local/bin/tcp-tunnel-proxy
    ```
    Replace `amd64` with `arm64` if needed.
2. Create `/etc/systemd/system/tcp-tunnel-proxy.service` and place environment overrides directly in the unit (matches the variables in the section below):
    ```ini
    [Unit]
    Description=TCP Tunnel Proxy
    After=network-online.target
    Wants=network-online.target

    [Service]
    ExecStart=/usr/local/bin/tcp-tunnel-proxy
    Environment=LISTEN_ADDR=:19000
    Environment=PORT_RANGE_START=45000
    Environment=PORT_RANGE_END=46000
    Environment=LOG_FORMAT=plain
    Restart=on-failure
    RestartSec=2s
    LimitNOFILE=65536

    [Install]
    WantedBy=multi-user.target
    ```
3. Reload systemd and start:
    ```sh
    sudo systemctl daemon-reload
    sudo systemctl enable --now tcp-tunnel-proxy.service
    sudo systemctl status tcp-tunnel-proxy.service
    ```

## Behavior Notes

-   PROXY protocol: If a load balancer prepends PROXY v1/v2, it is consumed and forwarded to the backend.
-   PostgreSQL: SSLRequest (8-byte prelude) is accepted (`S`), then TLS ClientHello is parsed for SNI; backend’s `S` is consumed before piping.
-   Cloudflared lifecycle: starts on first connection per SNI, waits for local port readiness (`startupTimeout`), increments refcounts; when refcount hits zero, an idle timer (`idleTimeout`, default 300s) kills the tunnel.
-   Crashes: if cloudflared exits while connections are active, the manager attempts restart.

## Configuration

-   `listenAddr`, `idleTimeout`, `startupTimeout`, `readHelloTimeout` constants in `main.go`.
-   Hostname derivation rule lives in `deriveTunnelHostname` (prefix `cft-`); adjust if you need a different mapping pattern.

### Environment Variables

-   `LISTEN_ADDR`: address to listen on (e.g., `:19000`, `127.0.0.1:19000`).
-   `IDLE_TIMEOUT`: duration before idle tunnels are torn down (e.g., `300s`).
-   `STARTUP_TIMEOUT`: how long to wait for `cloudflared` to become ready (e.g., `15s`).
-   `READ_HELLO_TIMEOUT`: how long to wait for client TLS prelude/SNI (e.g., `10s`).
-   `PORT_RANGE_START` / `PORT_RANGE_END`: dynamic local port pool for `cloudflared`.
-   `LOG_FORMAT`: `plain` (default) or `json` logging.
-   `RESTART_BACKOFF`: base delay between restart attempts when cloudflared exits (default `2s`).
-   `MAX_RESTARTS`: maximum restart attempts while connections are active (default `3`).

## Caveats / TODO

-   Temporary SNI failure handling sends `OK\n` instead of rejecting; switch to reject for production.
-   No persistence/log rotation; relies on stdout logging.
