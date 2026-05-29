package connectionhandler

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"tcp-tunnel-proxy/internal/logging"
)

type RouteProvider interface {
	GetHostname(ctx context.Context, publicPort int) (string, error)
}

type TunnelManager interface {
	GetOrStart(hostname string) (int, error)
	Release(hostname string)
}

// HandleConnection resolves the public port to an origin hostname, prepares cloudflared, and proxies raw TCP bytes.
func HandleConnection(conn net.Conn, publicPort int, routes RouteProvider, manager TunnelManager, logger *logging.Logger) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	logger.Infof("Incoming connection %s on public port %d", remote, publicPort)

	hostname, err := routes.GetHostname(context.Background(), publicPort)
	if err != nil {
		logger.Errorf("route lookup failed for public port %d from %s: %v", publicPort, remote, err)
		return
	}

	localPort, err := manager.GetOrStart(hostname)
	if err != nil {
		logger.Errorf("tunnel prep failed for %s: %v", hostname, err)
		return
	}
	defer manager.Release(hostname)

	backendAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
	backendConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		logger.Errorf("failed to dial backend %s for %s: %v", backendAddr, hostname, err)
		return
	}
	defer backendConn.Close()

	logger.Infof("Proxying %s -> %s via %s", remote, hostname, backendAddr)

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
		_, _ = io.Copy(conn, backendConn)
		if tcp, ok := backendConn.(*net.TCPConn); ok {
			_ = tcp.CloseRead()
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()

	wg.Wait()
	logger.Infof("Connection closed for %s (%s)", remote, hostname)
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
