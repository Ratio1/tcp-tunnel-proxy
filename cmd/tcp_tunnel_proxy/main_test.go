package main

import (
	"net"
	"testing"

	"tcp-tunnel-proxy/configs"
)

func TestListenAddr(t *testing.T) {
	got := listenAddr("127.0.0.1", 30001)
	if got != "127.0.0.1:30001" {
		t.Fatalf("listenAddr = %q, want 127.0.0.1:30001", got)
	}
}

func TestOpenPublicListenersSmallRange(t *testing.T) {
	port := freePort(t)
	listeners, err := openPublicListeners(configs.Config{
		ListenHost:           "127.0.0.1",
		PublicPortRangeStart: port,
		PublicPortRangeEnd:   port,
	})
	if err != nil {
		t.Fatalf("openPublicListeners returned error: %v", err)
	}
	defer closeListeners(listeners)

	if len(listeners) != 1 {
		t.Fatalf("listeners len = %d, want 1", len(listeners))
	}
	if listeners[0].port != port {
		t.Fatalf("listener port = %d, want %d", listeners[0].port, port)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
