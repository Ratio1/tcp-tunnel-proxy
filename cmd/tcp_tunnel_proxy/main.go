package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"tcp-tunnel-proxy/configs"
	cloudflaredmanager "tcp-tunnel-proxy/internal/cloudflared_manager"
	connectionhandler "tcp-tunnel-proxy/internal/connection_handler"
	"tcp-tunnel-proxy/internal/logging"
)

func main() {
	cfg, err := configs.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	logging.Setup(cfg.LogFormat)
	manager := cloudflaredmanager.NewNodeManager(cloudflaredmanager.Config{
		IdleTimeout:    cfg.IdleTimeout,
		StartupTimeout: cfg.StartupTimeout,
		PortRangeStart: cfg.PortRangeStart,
		PortRangeEnd:   cfg.PortRangeEnd,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", cfg.ListenAddr, err)
	}
	log.Printf("Routing oracle listening on %s", cfg.ListenAddr)

	var shutdownOnce sync.Once
	shutdown := func(reason string) {
		shutdownOnce.Do(func() {
			log.Printf("Shutting down: %s", reason)
			cancel()
			_ = ln.Close()
			manager.Shutdown(context.Background())
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		shutdown("received signal")
	}()

	var wg sync.WaitGroup

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if errors.Is(err, net.ErrClosed) {
				break
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				log.Printf("accept timeout: %v", err)
				continue
			}
			shutdown("listener error")
			break
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			connectionhandler.HandleConnection(c, manager, cfg.ReadHelloTimeout)
		}(conn)
	}

	wg.Wait()
	shutdown("accept loop exited")
}
