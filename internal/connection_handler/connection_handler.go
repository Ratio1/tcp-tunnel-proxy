package connectionhandler

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	cloudflaredmanager "tcp-tunnel-proxy/internal/cloudflared_manager"
	"time"
)

// handleConnection drives a single client flow: extract SNI, prepare tunnel, and proxy bytes.
func HandleConnection(conn net.Conn, manager *cloudflaredmanager.NodeManager, readHelloTimeout time.Duration) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	log.Printf("Incoming connection %s", remote)

	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	sni, buffers, sawPGSSLRequest, err := extractSNI(conn, readHelloTimeout)
	if buffers != nil {
		defer func() {
			putInitialBuffers(buffers)
		}()
	}
	if err != nil {
		_ = conn.SetReadDeadline(time.Time{})
		log.Printf("SNI extraction failed for %s: %v (closing connection)", remote, err)
		if tlsErr := sendTLSAlert(conn, alertUnrecognizedName); tlsErr != nil {
			log.Printf("failed to send TLS alert to %s: %v", remote, tlsErr)
		}
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetReadDeadline(time.Time{})

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
		prefix, err := consumeBackendPostgresSSLResponse(backendConn, readHelloTimeout)
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
