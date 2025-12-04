package cloudflaredmanager

import (
	"net"
	"testing"
)

func TestPortPoolReserveAndRelease(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	pool := newPortPool(port, port)

	first, err := pool.reserve()
	if err != nil {
		t.Fatalf("first reserve failed: %v", err)
	}
	if first != port {
		t.Fatalf("expected reserved port %d, got %d", port, first)
	}

	if _, err := pool.reserve(); err == nil {
		t.Fatalf("expected exhaustion error when reserving twice")
	}

	pool.release(first)

	second, err := pool.reserve()
	if err != nil {
		t.Fatalf("reserve after release failed: %v", err)
	}
	if second != port {
		t.Fatalf("expected to reuse port %d after release, got %d", port, second)
	}
}
