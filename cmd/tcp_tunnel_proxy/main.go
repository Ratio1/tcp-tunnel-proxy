package main

import (
	"log"
	"net"
	"tcp-tunnel-proxy/configs"
	cloudflaredmanager "tcp-tunnel-proxy/internal/cloudflared_manager"
	connectionhandler "tcp-tunnel-proxy/internal/connection_handler"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	configs.LoadConfigENV()
	manager := cloudflaredmanager.NewNodeManager()
	ln, err := net.Listen("tcp", configs.ListenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", configs.ListenAddr, err)
	}
	log.Printf("Routing oracle listening on %s", configs.ListenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go connectionhandler.HandleConnection(conn, manager)
	}
}
