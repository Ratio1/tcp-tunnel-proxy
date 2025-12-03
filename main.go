package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	manager := NewNodeManager(idleTimeout, startupTimeout, portRangeStart, portRangeEnd)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddr, err)
	}
	log.Printf("Routing oracle listening on %s", listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConnection(conn, manager)
	}
}

// handleConnection drives a single client flow: extract SNI, prepare tunnel, and proxy bytes.
func handleConnection(conn net.Conn, manager *NodeManager) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	log.Printf("Incoming connection %s", remote)

	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	sni, buffers, sawPGSSLRequest, err := extractSNI(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if buffers != nil {
		defer func() {
			putInitialBuffers(buffers)
		}()
	}
	if err != nil {
		log.Printf("SNI extraction failed for %s: %v (returning placeholder)", remote, err)
		_, _ = conn.Write([]byte("OK\n"))
		return
	}

	log.Printf("Resolved %s as SNI=%s", remote, sni)

	localPort, err := manager.GetOrStart(sni)
	if err != nil {
		log.Printf("tunnel prep failed for %s: %v", sni, err)
		return
	}
	defer manager.Release(sni)

	backendAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
	backendConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		log.Printf("failed to dial backend %s for %s: %v", backendAddr, sni, err)
		return
	}
	defer backendConn.Close()

	// Send PROXY + optional PostgreSQL SSLRequest first so we can observe the backend's SSL response,
	// then stream the TLS ClientHello once the server has answered.
	if len(buffers.prelude) > 0 {
		if err := writeAll(backendConn, buffers.prelude); err != nil {
			log.Printf("failed to forward prelude bytes to backend for %s: %v", sni, err)
			return
		}
	}

	var backendReader io.Reader = backendConn
	if sawPGSSLRequest {
		prefix, err := consumeBackendPostgresSSLResponse(backendConn)
		if err != nil {
			log.Printf("backend Postgres SSL response read failed for %s: %v", sni, err)
		}
		if len(prefix) > 0 {
			backendReader = io.MultiReader(bytes.NewReader(prefix), backendConn)
		}
	}

	// Now deliver the TLS ClientHello (and any buffered bytes) to the backend before switching to streaming.
	if len(buffers.tlsInitial) > 0 {
		if err := writeAll(backendConn, buffers.tlsInitial); err != nil {
			log.Printf("failed to forward TLS initial bytes to backend for %s: %v", sni, err)
			return
		}
	}
	putInitialBuffers(buffers)
	buffers = nil

	log.Printf("Proxying %s -> %s via %s", remote, sni, backendAddr)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(backendConn, conn)
		if tcp, ok := backendConn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseRead()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, backendReader)
		if tcp, ok := backendConn.(*net.TCPConn); ok {
			_ = tcp.CloseRead()
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()

	wg.Wait()
	log.Printf("Connection closed for %s (%s)", remote, sni)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}
