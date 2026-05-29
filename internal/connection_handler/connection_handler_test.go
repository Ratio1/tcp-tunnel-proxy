package connectionhandler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"tcp-tunnel-proxy/internal/logging"
)

type partialWriter struct {
	limit     int
	wrote     []byte
	failAfter int
	calls     int
}

func (p *partialWriter) Write(b []byte) (int, error) {
	p.calls++
	if p.failAfter > 0 && p.calls > p.failAfter {
		return 0, errors.New("forced failure")
	}
	n := p.limit
	if n <= 0 || n > len(b) {
		n = len(b)
	}
	p.wrote = append(p.wrote, b[:n]...)
	return n, nil
}

func TestWriteAllHandlesPartialWrites(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), 1024)
	writer := &partialWriter{limit: 128}

	if err := writeAll(writer, payload); err != nil {
		t.Fatalf("writeAll returned error: %v", err)
	}
	if !bytes.Equal(writer.wrote, payload) {
		t.Fatalf("writeAll wrote %d bytes, want %d", len(writer.wrote), len(payload))
	}
	if writer.calls <= 1 {
		t.Fatalf("expected multiple writes, got %d", writer.calls)
	}
}

func TestWriteAllPropagatesErrors(t *testing.T) {
	payload := []byte("hello world")
	writer := &partialWriter{limit: 2, failAfter: 1}

	err := writeAll(writer, payload)
	if err == nil {
		t.Fatalf("expected error on second write")
	}
}

type stubRouteProvider struct {
	hostname string
	err      error
	calls    int
}

func (s *stubRouteProvider) GetHostname(ctx context.Context, publicPort int) (string, error) {
	s.calls++
	return s.hostname, s.err
}

type stubTunnelManager struct {
	port     int
	err      error
	started  []string
	released []string
	mu       sync.Mutex
}

func (s *stubTunnelManager) GetOrStart(hostname string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = append(s.started, hostname)
	return s.port, s.err
}

func (s *stubTunnelManager) Release(hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released = append(s.released, hostname)
}

func TestHandleConnectionClosesWhenRouteMissing(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	routes := &stubRouteProvider{err: errors.New("missing route")}
	manager := &stubTunnelManager{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleConnection(server, 30001, routes, manager, logging.New("test"))
	}()

	buf := make([]byte, 1)
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(buf); err == nil {
		t.Fatalf("expected connection to close without data")
	}
	<-done

	if routes.calls != 1 {
		t.Fatalf("route calls = %d, want 1", routes.calls)
	}
	if len(manager.started) != 0 {
		t.Fatalf("manager should not be called when route lookup fails")
	}
}

func TestHandleConnectionRelaysRawBytes(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen failed: %v", err)
	}
	defer backend.Close()
	backendPort := backend.Addr().(*net.TCPAddr).Port

	backendDone := make(chan struct{})
	go func() {
		defer close(backendDone)
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		payload := make([]byte, 4)
		if _, err := io.ReadFull(conn, payload); err != nil {
			t.Errorf("backend read failed: %v", err)
			return
		}
		if string(payload) != "ping" {
			t.Errorf("backend received %q, want ping", payload)
			return
		}
		if _, err := conn.Write([]byte("pong")); err != nil {
			t.Errorf("backend write failed: %v", err)
		}
	}()

	client, server := net.Pipe()
	routes := &stubRouteProvider{hostname: "origin.ratio1.link"}
	manager := &stubTunnelManager{port: backendPort}
	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleConnection(server, 30001, routes, manager, logging.New("test"))
	}()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
	response := make([]byte, 4)
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(client, response); err != nil {
		t.Fatalf("client read failed: %v", err)
	}
	if string(response) != "pong" {
		t.Fatalf("client received %q, want pong", response)
	}
	_ = client.Close()
	<-done
	<-backendDone

	if len(manager.started) != 1 || manager.started[0] != "origin.ratio1.link" {
		t.Fatalf("manager started = %#v, want origin.ratio1.link", manager.started)
	}
	if len(manager.released) != 1 || manager.released[0] != "origin.ratio1.link" {
		t.Fatalf("manager released = %#v, want origin.ratio1.link", manager.released)
	}
}
