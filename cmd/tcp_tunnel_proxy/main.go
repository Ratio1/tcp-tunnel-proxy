package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"tcp-tunnel-proxy/configs"
	cloudflaredmanager "tcp-tunnel-proxy/internal/cloudflared_manager"
	connectionhandler "tcp-tunnel-proxy/internal/connection_handler"
	"tcp-tunnel-proxy/internal/logging"
	routeprovider "tcp-tunnel-proxy/internal/route_provider"
)

type portListener struct {
	port     int
	listener net.Listener
}

const healthResponse = "success\n"

func main() {
	cfg, err := configs.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	logging.Setup(cfg.LogFormat)
	logger := logging.New("main")

	manager, err := cloudflaredmanager.NewNodeManager(cloudflaredmanager.Config{
		IdleTimeout:         cfg.IdleTimeout,
		StartupTimeout:      cfg.StartupTimeout,
		LocalPortRangeStart: cfg.LocalPortRangeStart,
		LocalPortRangeEnd:   cfg.LocalPortRangeEnd,
		RestartBackoff:      cfg.RestartBackoff,
		MaxRestarts:         cfg.MaxRestarts,
	})
	if err != nil {
		log.Fatalf("failed to construct node manager: %v", err)
	}

	routes, err := routeprovider.NewHTTPProvider(cfg.TunnelManagerBaseURL, cfg.RouteLookupTimeout)
	if err != nil {
		log.Fatalf("failed to construct route provider: %v", err)
	}

	healthListener, err := openHealthListener(cfg)
	if err != nil {
		log.Fatalf("failed to open health listener: %v", err)
	}

	listeners, err := openPublicListeners(cfg)
	if err != nil {
		_ = healthListener.listener.Close()
		log.Fatalf("failed to open public listeners: %v", err)
	}
	logger.Infof("Listening on %s for health checks", healthListener.listener.Addr().String())
	for _, pl := range listeners {
		logger.Infof("Listening on %s for public TCP port %d", pl.listener.Addr().String(), pl.port)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var acceptWG sync.WaitGroup
	var handlerWG sync.WaitGroup
	var shutdownOnce sync.Once
	shutdown := func(reason string) {
		shutdownOnce.Do(func() {
			logger.Infof("Shutting down: %s", reason)
			cancel()
			_ = healthListener.listener.Close()
			for _, pl := range listeners {
				_ = pl.listener.Close()
			}
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		shutdown("received signal")
	}()

	acceptWG.Add(1)
	go func() {
		defer acceptWG.Done()
		acceptHealthLoop(ctx, healthListener, shutdown, logger)
	}()

	for _, pl := range listeners {
		acceptWG.Add(1)
		go func(pl portListener) {
			defer acceptWG.Done()
			acceptLoop(ctx, pl, routes, manager, &handlerWG, shutdown, logger)
		}(pl)
	}

	acceptWG.Wait()
	shutdown("accept loops exited")
	handlerWG.Wait()
	manager.Shutdown(context.Background())
}

func openHealthListener(cfg configs.Config) (portListener, error) {
	ln, err := net.Listen("tcp", listenAddr(cfg.ListenHost, cfg.HealthPort))
	if err != nil {
		return portListener{}, fmt.Errorf("listen on health port %d: %w", cfg.HealthPort, err)
	}
	return portListener{port: cfg.HealthPort, listener: ln}, nil
}

func openPublicListeners(cfg configs.Config) ([]portListener, error) {
	listeners := make([]portListener, 0, cfg.PublicPortRangeEnd-cfg.PublicPortRangeStart+1)
	for port := cfg.PublicPortRangeStart; port <= cfg.PublicPortRangeEnd; port++ {
		ln, err := net.Listen("tcp", listenAddr(cfg.ListenHost, port))
		if err != nil {
			closeListeners(listeners)
			return nil, fmt.Errorf("listen on public port %d: %w", port, err)
		}
		listeners = append(listeners, portListener{port: port, listener: ln})
	}
	return listeners, nil
}

func closeListeners(listeners []portListener) {
	for _, pl := range listeners {
		_ = pl.listener.Close()
	}
}

func listenAddr(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func acceptHealthLoop(ctx context.Context, pl portListener, shutdown func(string), logger *logging.Logger) {
	for {
		conn, err := pl.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				logger.Errorf("health accept timeout on port %d: %v", pl.port, err)
				continue
			}
			logger.Errorf("health listener error on port %d: %v", pl.port, err)
			shutdown(fmt.Sprintf("health listener error on port %d", pl.port))
			return
		}
		go handleHealthConnection(conn, logger)
	}
}

func handleHealthConnection(conn net.Conn, logger *logging.Logger) {
	defer conn.Close()
	if _, err := io.WriteString(conn, healthResponse); err != nil {
		logger.Errorf("health response write failed for %s: %v", conn.RemoteAddr().String(), err)
	}
}

func acceptLoop(ctx context.Context, pl portListener, routes connectionhandler.RouteProvider, manager connectionhandler.TunnelManager, handlerWG *sync.WaitGroup, shutdown func(string), logger *logging.Logger) {
	for {
		conn, err := pl.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				logger.Errorf("accept timeout on port %d: %v", pl.port, err)
				continue
			}
			logger.Errorf("listener error on port %d: %v", pl.port, err)
			shutdown(fmt.Sprintf("listener error on public port %d", pl.port))
			return
		}
		handlerWG.Add(1)
		go func(conn net.Conn) {
			defer handlerWG.Done()
			connectionhandler.HandleConnection(ctx, conn, pl.port, routes, manager, logging.New("connection"))
		}(conn)
	}
}
