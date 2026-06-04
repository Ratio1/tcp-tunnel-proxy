package main

import (
	"io"
	"net"
	"testing"

	"tcp-tunnel-proxy/configs"
	"tcp-tunnel-proxy/internal/logging"
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

func TestOpenHealthListener(t *testing.T) {
	port := freePort(t)
	pl, err := openHealthListener(configs.Config{
		ListenHost: "127.0.0.1",
		HealthPort: port,
	})
	if err != nil {
		t.Fatalf("openHealthListener returned error: %v", err)
	}
	defer pl.listener.Close()

	if pl.port != port {
		t.Fatalf("health listener port = %d, want %d", pl.port, port)
	}
}

func TestHealthConnectionReturnsSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleHealthConnection(server, logging.New("test"))
	}()

	got, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("failed to read health response: %v", err)
	}
	if string(got) != healthResponse {
		t.Fatalf("health response = %q, want %q", string(got), healthResponse)
	}
	<-done
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
