# TCP Tunnel Proxy (Cloudflare Access)

Port-based TCP proxy for Cloudflare Access tunnels. It listens on the configured public port range, asks the tunnel manager for the origin hostname assigned to the accepted port, starts or reuses `cloudflared access tcp`, and forwards raw TCP bytes end-to-end.

## Features

-   Listens on every public port in `PUBLIC_PORT_RANGE_START`-`PUBLIC_PORT_RANGE_END` (`30000`-`30499` by default).
-   Resolves routes through tunnel manager `get_tcp_route(public_port)`.
-   Caches successful route lookups in memory for the process lifetime.
-   Starts one local `cloudflared access tcp --hostname <origin>` process per origin hostname.
-   Full-duplex raw TCP piping with no TLS, SNI, PROXY, or protocol pre-parsing.

## Quick Start

1. Ensure `cloudflared` is installed and on PATH.
2. Ensure tunnel manager is running and has ChainStore TCP route entries for the public ports.
3. Build/run:
    ```sh
    go run ./cmd/tcp_tunnel_proxy
    # or
    go build -o tcp-tunnel-proxy ./cmd/tcp_tunnel_proxy && ./tcp-tunnel-proxy
    ```
4. Clients connect directly to the allocated public endpoint, for example `tcp.ratio1.link:30001`.

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
2. Create `/etc/systemd/system/tcp-tunnel-proxy.service` and place environment overrides directly in the unit:
    ```ini
    [Unit]
    Description=TCP Tunnel Proxy
    After=network-online.target
    Wants=network-online.target

    [Service]
    ExecStart=/usr/local/bin/tcp-tunnel-proxy
    Environment=LISTEN_HOST=
    Environment=PUBLIC_PORT_RANGE_START=30000
    Environment=PUBLIC_PORT_RANGE_END=30499
    Environment=TUNNEL_MANAGER_BASE_URL=https://1f8b266e9dbf.ratio1.link
    Environment=LOCAL_PORT_RANGE_START=20000
    Environment=LOCAL_PORT_RANGE_END=20100
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

-   Route source of truth: tunnel manager owns the ChainStore port registry. This proxy only calls `get_tcp_route(public_port)`.
-   Cache behavior: successful port lookups are cached indefinitely for this process. Route deletion or port reuse can require a proxy restart until invalidation exists.
-   Cloudflared lifecycle: starts on first connection per origin hostname, waits for local port readiness, increments refcounts, and tears down idle tunnels after `IDLE_TIMEOUT`.
-   Crashes: if cloudflared exits while connections are active, the manager attempts restart.

## Environment Variables

-   `LISTEN_HOST`: host/interface to bind (empty means all interfaces).
-   `PUBLIC_PORT_RANGE_START` / `PUBLIC_PORT_RANGE_END`: public ports to listen on.
-   `TUNNEL_MANAGER_BASE_URL`: tunnel manager API base URL.
-   `ROUTE_LOOKUP_TIMEOUT`: timeout for `get_tcp_route` requests (default `5s`).
-   `IDLE_TIMEOUT`: duration before idle tunnels are torn down (default `300s`).
-   `STARTUP_TIMEOUT`: how long to wait for `cloudflared` to become ready (default `15s`).
-   `LOCAL_PORT_RANGE_START` / `LOCAL_PORT_RANGE_END`: dynamic local port pool for `cloudflared`.
-   `LOG_FORMAT`: `plain` (default) or `json` logging.
-   `RESTART_BACKOFF`: base delay between restart attempts when cloudflared exits (default `2s`).
-   `MAX_RESTARTS`: maximum restart attempts while connections are active (default `3`).

## Caveats / TODO

-   No persistence/log rotation; relies on stdout logging.
-   No route invalidation yet; successful lookups remain cached until process restart.
