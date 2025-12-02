package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"sync"
	"time"
)

// NodeConfig defines how to reach a backend node through cloudflared.
type NodeConfig struct {
	Hostname  string
	LocalPort int
}

// nodeConfigs maps incoming SNI hostnames to backend nodes.
// Update this map to match your environment.
var nodeConfigs = map[string]NodeConfig{
	"tcpproxyspectrum.ratio1.link":  {Hostname: "2c6c395cdbd9.ratio1.link", LocalPort: 15001},
	"service.customer2.example.com": {Hostname: "node2.internal.example.com", LocalPort: 15002},
}

const (
	listenAddr       = ":19000"
	idleTimeout      = 300 * time.Second
	startupTimeout   = 15 * time.Second
	readHelloTimeout = 10 * time.Second
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if len(nodeConfigs) == 0 {
		log.Fatalf("node configuration map is empty")
	}

	manager := NewNodeManager(nodeConfigs, idleTimeout, startupTimeout)
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

func handleConnection(conn net.Conn, manager *NodeManager) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	log.Printf("Incoming connection from %s", remote)

	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	sni, prelude, tlsInitial, sawPGSSLRequest, err := extractSNI(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		log.Printf("SNI extraction failed for %s: %v (returning success placeholder)", remote, err)
		_, _ = conn.Write([]byte("OK\n"))
		return
	}

	log.Printf("Connection %s SNI=%s", remote, sni)

	localPort, err := manager.GetOrStart(sni)
	if err != nil {
		log.Printf("failed to prepare tunnel for %s: %v", sni, err)
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

	log.Printf("Forwarding %s -> %s (backend %s)", remote, sni, backendAddr)

	// Send PROXY + optional PostgreSQL SSLRequest first so we can observe the backend's SSL response,
	// then stream the TLS ClientHello once the server has answered.
	if len(prelude) > 0 {
		if _, err := io.Copy(backendConn, bytes.NewReader(prelude)); err != nil {
			log.Printf("failed to forward prelude bytes to backend for %s: %v", sni, err)
			return
		}
	}

	var backendReader io.Reader = backendConn
	if sawPGSSLRequest {
		prefix, err := consumeBackendPostgresSSLResponse(backendConn)
		if err != nil {
			log.Printf("warning: failed to consume backend Postgres SSL response for %s: %v", sni, err)
		}
		if len(prefix) > 0 {
			backendReader = io.MultiReader(bytes.NewReader(prefix), backendConn)
		}
	}

	// Now deliver the TLS ClientHello (and any buffered bytes) to the backend before switching to streaming.
	if len(tlsInitial) > 0 {
		if _, err := io.Copy(backendConn, bytes.NewReader(tlsInitial)); err != nil {
			log.Printf("failed to forward TLS initial bytes to backend for %s: %v", sni, err)
			return
		}
	}

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

// extractSNI reads the first TLS record (handling optional PROXY and PostgreSQL SSLRequest),
// extracts the SNI hostname, and returns the raw bytes that were consumed so they can be
// replayed to the backend.
func extractSNI(conn net.Conn) (string, []byte, []byte, bool, error) {
	reader := bufio.NewReader(conn)
	var prelude bytes.Buffer    // everything before TLS (PROXY/SSLRequest)
	var tlsInitial bytes.Buffer // TLS record(s) consumed for SNI parsing

	if err := maybeConsumeProxyHeader(reader, &prelude); err != nil {
		return "", prelude.Bytes(), tlsInitial.Bytes(), false, err
	}

	sawPGSSLRequest, err := maybeHandlePostgresSSLRequest(reader, &prelude, conn)
	if err != nil {
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, err
	}

	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, fmt.Errorf("reading TLS header: %w", err)
	}
	tlsInitial.Write(header)

	if header[0] != 0x16 { // TLS Handshake
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, errors.New("not a TLS handshake record")
	}

	length := int(header[3])<<8 | int(header[4])
	if length <= 0 || length > 1<<15 {
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, fmt.Errorf("invalid TLS record length %d", length)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, fmt.Errorf("reading TLS body: %w", err)
	}
	tlsInitial.Write(body)

	sni, err := parseClientHelloForSNI(body)
	if err != nil {
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, err
	}
	if sni == "" {
		return "", prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, errors.New("no SNI present")
	}

	// Preserve any bytes bufio.Reader has already pulled from the socket so the
	// backend sees an unbroken stream.
	if buffered := reader.Buffered(); buffered > 0 {
		extra := make([]byte, buffered)
		if _, err := io.ReadFull(reader, extra); err == nil {
			tlsInitial.Write(extra)
		}
	}

	return sni, prelude.Bytes(), tlsInitial.Bytes(), sawPGSSLRequest, nil
}

func maybeHandlePostgresSSLRequest(r *bufio.Reader, consumed *bytes.Buffer, conn net.Conn) (bool, error) {
	const sslRequestLen = 8

	peek, err := r.Peek(sslRequestLen)
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) || errors.Is(err, io.EOF) {
			return false, nil
		}
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			// Let the caller hit the TLS read timeout instead.
			return false, nil
		}
		return false, fmt.Errorf("peek postgres SSLRequest: %w", err)
	}
	if len(peek) < sslRequestLen {
		return false, nil
	}

	length := binary.BigEndian.Uint32(peek[0:4])
	magic := binary.BigEndian.Uint32(peek[4:8])
	if length != 8 || magic != 80877103 {
		log.Printf("No PostgreSQL SSLRequest detected (length=%d, magic=%d)", length, magic)
		return false, nil
	}

	log.Printf("PostgreSQL SSLRequest detected; responding with acceptance")
	req := make([]byte, sslRequestLen)
	if _, err := io.ReadFull(r, req); err != nil {
		return true, fmt.Errorf("read postgres SSLRequest: %w", err)
	}
	consumed.Write(req)

	if _, err := conn.Write([]byte{'S'}); err != nil {
		return true, fmt.Errorf("write postgres SSL response: %w", err)
	}
	// Give the client a fresh window to send the subsequent TLS ClientHello.
	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	return true, nil
}

func consumeBackendPostgresSSLResponse(conn net.Conn) ([]byte, error) {
	var buf [1]byte

	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	n, err := conn.Read(buf[:])
	_ = conn.SetReadDeadline(time.Time{})

	if n == 0 {
		return nil, err
	}
	if buf[0] == 'S' {
		log.Printf("Backend Postgres SSL response: accepted TLS (S)")
		return nil, err
	}

	log.Printf("Backend Postgres first byte after SSLRequest: 0x%02x (%q)", buf[0], buf[0])
	return buf[:1], err
}

func maybeConsumeProxyHeader(r *bufio.Reader, consumed *bytes.Buffer) error {
	const proxyV2Len = 12
	sig, err := r.Peek(proxyV2Len)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			// Timed out waiting for data; proceed so TLS read reports the timeout instead.
			return nil
		}
		return fmt.Errorf("peek proxy header: %w", err)
	}
	// PROXY protocol v1 (text)
	if bytes.HasPrefix(sig, []byte("PROXY ")) {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read proxy v1 header: %w", err)
		}
		if len(line) > 107 { // spec limit plus CRLF
			return errors.New("proxy v1 header too long")
		}
		consumed.WriteString(line)
		return nil
	}

	// PROXY protocol v2 (binary)
	proxyV2Sig := []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a}
	if len(sig) >= proxyV2Len && bytes.Equal(sig[:proxyV2Len], proxyV2Sig) {
		hdr := make([]byte, 16)
		if _, err := io.ReadFull(r, hdr); err != nil {
			return fmt.Errorf("read proxy v2 header: %w", err)
		}
		consumed.Write(hdr)
		addrLen := int(binary.BigEndian.Uint16(hdr[14:16]))
		if addrLen > 0 {
			addr := make([]byte, addrLen)
			if _, err := io.ReadFull(r, addr); err != nil {
				return fmt.Errorf("read proxy v2 address block: %w", err)
			}
			consumed.Write(addr)
		}
		return nil
	}
	return nil
}

func parseClientHelloForSNI(record []byte) (string, error) {
	if len(record) < 4 {
		return "", errors.New("TLS record too short for handshake")
	}
	if record[0] != 0x01 {
		return "", errors.New("first handshake message is not ClientHello")
	}

	handshakeLen := int(record[1])<<16 | int(record[2])<<8 | int(record[3])
	if handshakeLen+4 > len(record) {
		return "", errors.New("truncated ClientHello")
	}
	data := record[4 : 4+handshakeLen]
	offset := 0

	if len(data) < 34 {
		return "", errors.New("ClientHello too short")
	}
	offset += 2  // version
	offset += 32 // random

	if offset >= len(data) {
		return "", errors.New("malformed ClientHello (session id length missing)")
	}
	sidLen := int(data[offset])
	offset++
	if offset+sidLen > len(data) {
		return "", errors.New("malformed ClientHello (session id)")
	}
	offset += sidLen

	if offset+2 > len(data) {
		return "", errors.New("malformed ClientHello (cipher suites length)")
	}
	csLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	if offset+csLen > len(data) {
		return "", errors.New("malformed ClientHello (cipher suites)")
	}
	offset += csLen

	if offset >= len(data) {
		return "", errors.New("malformed ClientHello (compression length)")
	}
	compLen := int(data[offset])
	offset++
	if offset+compLen > len(data) {
		return "", errors.New("malformed ClientHello (compression methods)")
	}
	offset += compLen

	if offset+2 > len(data) {
		return "", errors.New("ClientHello missing extensions length")
	}
	extLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	if offset+extLen > len(data) {
		return "", errors.New("ClientHello extensions truncated")
	}
	exts := data[offset : offset+extLen]

	for len(exts) >= 4 {
		extType := int(exts[0])<<8 | int(exts[1])
		extDataLen := int(exts[2])<<8 | int(exts[3])
		exts = exts[4:]
		if extDataLen > len(exts) {
			return "", errors.New("extension length overflow")
		}
		extData := exts[:extDataLen]
		exts = exts[extDataLen:]

		if extType == 0 { // server_name
			if len(extData) < 2 {
				return "", errors.New("SNI extension too short")
			}
			listLen := int(extData[0])<<8 | int(extData[1])
			if listLen+2 > len(extData) {
				return "", errors.New("SNI list length invalid")
			}
			names := extData[2 : 2+listLen]
			for len(names) >= 3 {
				nameType := names[0]
				nameLen := int(names[1])<<8 | int(names[2])
				names = names[3:]
				if nameLen > len(names) {
					return "", errors.New("SNI name length invalid")
				}
				name := string(names[:nameLen])
				names = names[nameLen:]
				if nameType == 0 {
					return name, nil
				}
			}
			return "", errors.New("SNI extension present but no host name found")
		}
	}

	return "", errors.New("SNI not found in ClientHello")
}

// NodeManager tracks cloudflared tunnels per node and manages lifecycles.
type NodeManager struct {
	mu             sync.Mutex
	config         map[string]NodeConfig
	nodes          map[string]*nodeState
	idleTimeout    time.Duration
	startupTimeout time.Duration
}

type nodeState struct {
	cfg       NodeConfig
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	refCount  int
	idleTimer *time.Timer
	ready     chan struct{}
	startErr  error
}

// NewNodeManager constructs a manager with the provided node mapping and timeouts.
func NewNodeManager(cfg map[string]NodeConfig, idle, startup time.Duration) *NodeManager {
	return &NodeManager{
		config:         cfg,
		nodes:          make(map[string]*nodeState),
		idleTimeout:    idle,
		startupTimeout: startup,
	}
}

// GetOrStart ensures a tunnel for the given SNI is running and returns its local port.
func (m *NodeManager) GetOrStart(sni string) (int, error) {
	cfg, ok := m.config[sni]
	if !ok {
		return 0, fmt.Errorf("no backend configured for %s", sni)
	}

	m.mu.Lock()
	st, ok := m.nodes[sni]
	if !ok {
		st = &nodeState{cfg: cfg}
		m.nodes[sni] = st
	}
	st.refCount++

	if st.idleTimer != nil {
		st.idleTimer.Stop()
		st.idleTimer = nil
	}

	ready := st.ready
	if st.cmd == nil || st.cmd.Process == nil || st.cmd.ProcessState != nil {
		if ready == nil {
			ready = make(chan struct{})
			st.ready = ready
			go m.launchTunnel(sni, st, ready)
		}
	}
	m.mu.Unlock()

	if ready != nil {
		<-ready
	}

	m.mu.Lock()
	err := st.startErr
	m.mu.Unlock()

	if err != nil {
		m.Release(sni)
		return 0, err
	}
	return cfg.LocalPort, nil
}

// Release decrements the refcount for a node and schedules tunnel teardown if idle.
func (m *NodeManager) Release(sni string) {
	m.mu.Lock()
	st, ok := m.nodes[sni]
	if !ok {
		m.mu.Unlock()
		return
	}

	if st.refCount > 0 {
		st.refCount--
	}

	if st.refCount == 0 && st.idleTimer == nil {
		st.idleTimer = time.AfterFunc(m.idleTimeout, func() {
			m.stopNode(sni)
		})
	}
	m.mu.Unlock()
}

func (m *NodeManager) launchTunnel(sni string, st *nodeState, ready chan struct{}) {
	cfg := st.cfg
	log.Printf("Starting cloudflared for %s -> %s (local %d)", sni, cfg.Hostname, cfg.LocalPort)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "cloudflared", "access", "tcp", "--hostname", cfg.Hostname, "--url", fmt.Sprintf("localhost:%d", cfg.LocalPort))

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("failed to start cloudflared for %s: %v", sni, err)
		m.mu.Lock()
		st.startErr = err
		if st.ready == ready {
			close(ready)
			st.ready = nil
		}
		m.mu.Unlock()
		return
	}

	go streamPipe(stdout, fmt.Sprintf("[%s][cloudflared][stdout]", sni))
	go streamPipe(stderr, fmt.Sprintf("[%s][cloudflared][stderr]", sni))

	m.mu.Lock()
	st.cmd = cmd
	st.cancel = cancel
	st.startErr = nil
	m.mu.Unlock()

	err := waitForPort(ctx, "127.0.0.1", cfg.LocalPort, m.startupTimeout)
	if err != nil {
		log.Printf("cloudflared for %s did not become ready: %v", sni, err)
		cancel()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		m.mu.Lock()
		st.cmd = nil
		st.cancel = nil
		st.startErr = err
		if st.ready == ready {
			close(ready)
			st.ready = nil
		}
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	st.startErr = nil
	m.mu.Unlock()
	close(ready)

	go func() {
		err := cmd.Wait()
		cancel()
		log.Printf("cloudflared for %s exited: %v", sni, err)
		m.handleProcessExit(sni, st, err)
	}()
}

func (m *NodeManager) handleProcessExit(sni string, st *nodeState, err error) {
	m.mu.Lock()
	active := st.refCount
	st.cmd = nil
	st.cancel = nil
	st.ready = nil
	st.startErr = fmt.Errorf("tunnel exited: %v", err)
	m.mu.Unlock()

	if active > 0 {
		log.Printf("Restarting cloudflared for %s (active connections: %d)", sni, active)
		m.mu.Lock()
		if st.ready == nil && st.cmd == nil {
			st.ready = make(chan struct{})
			ready := st.ready
			m.mu.Unlock()
			go m.launchTunnel(sni, st, ready)
		} else {
			m.mu.Unlock()
		}
	}
}

func (m *NodeManager) stopNode(sni string) {
	m.mu.Lock()
	st, ok := m.nodes[sni]
	if !ok {
		m.mu.Unlock()
		return
	}
	if st.refCount > 0 {
		m.mu.Unlock()
		return
	}
	cmd := st.cmd
	cancel := st.cancel
	st.cmd = nil
	st.cancel = nil
	st.ready = nil
	st.startErr = fmt.Errorf("tunnel stopped")
	st.idleTimer = nil
	m.mu.Unlock()

	log.Printf("Stopping cloudflared for %s due to idleness", sni)
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
}

func waitForPort(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	target := fmt.Sprintf("%s:%d", host, port)
	for {
		dialer := net.Dialer{Timeout: 500 * time.Millisecond}
		conn, err := dialer.DialContext(ctx, "tcp", target)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s: %w", target, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func streamPipe(r io.ReadCloser, prefix string) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Printf("%s %s", prefix, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("%s stream error: %v", prefix, err)
	}
}
