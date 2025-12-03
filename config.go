package main

import "time"

// NodeConfig defines how to reach a backend node through cloudflared.
type NodeConfig struct {
	Hostname string
}

// nodeConfigs maps incoming SNI hostnames to backend nodes.
// Update this map to match your environment.
var nodeConfigs = map[string]NodeConfig{
	"tcpproxyspectrum.ratio1.link":  {Hostname: "2c6c395cdbd9.ratio1.link"},
	"service.customer2.example.com": {Hostname: "node2.internal.example.com"},
}

const (
	listenAddr       = ":19000"
	idleTimeout      = 300 * time.Second
	startupTimeout   = 15 * time.Second
	readHelloTimeout = 10 * time.Second
	portRangeStart   = 20000
	portRangeEnd     = 20100
)
