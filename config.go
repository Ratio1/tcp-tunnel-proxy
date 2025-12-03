package main

import "time"

const (
	listenAddr       = ":19000"
	idleTimeout      = 300 * time.Second
	startupTimeout   = 15 * time.Second
	readHelloTimeout = 10 * time.Second
	portRangeStart   = 20000
	portRangeEnd     = 20100
)
