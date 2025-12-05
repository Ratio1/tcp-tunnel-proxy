package cloudflaredmanager

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"

	"tcp-tunnel-proxy/internal/logging"
)

type portPool struct {
	mu    sync.Mutex
	start int
	end   int
	used  map[int]bool
}

func newPortPool(start, end int) *portPool {
	return &portPool{
		start: start,
		end:   end,
		used:  make(map[int]bool),
	}
}

func (p *portPool) reserve() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for port := p.start; port <= p.end; port++ {
		if p.used[port] {
			continue
		}
		if !isPortAvailable(port) {
			continue
		}
		p.used[port] = true
		return port, nil
	}
	return 0, fmt.Errorf("no free ports in range %d-%d", p.start, p.end)
}

func (p *portPool) release(port int) {
	if port == 0 {
		return
	}
	p.mu.Lock()
	delete(p.used, port)
	p.mu.Unlock()
}

// NodeManager tracks cloudflared tunnels per backend hostname and manages lifecycles.
type NodeManager struct {
	mu             sync.Mutex
	nodes          map[string]*nodeState // keyed by backend hostname
	idleTimeout    time.Duration
	startupTimeout time.Duration
	ports          *portPool
	closed         bool
	restartBackoff time.Duration
	maxRestarts    int
	logger         *logging.Logger
}

type nodeState struct {
	hostname  string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	refCount  int
	idleTimer *time.Timer
	ready     chan struct{}
	startErr  error
	port      int
	restarts  int
}

// Config holds tunable settings for the node manager.
type Config struct {
	IdleTimeout    time.Duration
	StartupTimeout time.Duration
	PortRangeStart int
	PortRangeEnd   int
	RestartBackoff time.Duration
	MaxRestarts    int
}

// NewNodeManager constructs a manager using the provided configuration, then applies overrides.
func NewNodeManager(cfg Config) (*NodeManager, error) {
	if cfg.PortRangeStart <= 0 || cfg.PortRangeEnd < cfg.PortRangeStart {
		return nil, fmt.Errorf("invalid port pool range %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd)
	}
	if cfg.IdleTimeout <= 0 || cfg.StartupTimeout <= 0 {
		return nil, fmt.Errorf("timeouts must be positive")
	}
	if cfg.RestartBackoff <= 0 {
		cfg.RestartBackoff = 2 * time.Second
	}
	if cfg.MaxRestarts <= 0 {
		cfg.MaxRestarts = 3
	}

	return &NodeManager{
		nodes:          make(map[string]*nodeState),
		idleTimeout:    cfg.IdleTimeout,
		startupTimeout: cfg.StartupTimeout,
		ports:          newPortPool(cfg.PortRangeStart, cfg.PortRangeEnd),
		restartBackoff: cfg.RestartBackoff,
		maxRestarts:    cfg.MaxRestarts,
		logger:         logging.New("node_manager"),
	}, nil
}

// GetOrStart ensures a tunnel for the given SNI is running and returns its local port.
func (m *NodeManager) GetOrStart(sni string) (int, error) {
	hostname, err := deriveValidatedTunnelHostname(sni)
	if err != nil {
		return 0, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return 0, fmt.Errorf("node manager shutting down")
	}
	st, ok := m.nodes[hostname]
	if !ok {
		st = &nodeState{hostname: hostname}
		m.nodes[hostname] = st
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
			go m.launchTunnel(st, ready)
		}
	}
	m.mu.Unlock()

	if ready != nil {
		<-ready
	}

	m.mu.Lock()
	err = st.startErr
	port := st.port
	m.mu.Unlock()

	if err != nil {
		m.Release(sni)
		return 0, err
	}
	if port == 0 {
		m.Release(sni)
		return 0, fmt.Errorf("no port assigned for %s", hostname)
	}
	return port, nil
}

// Release decrements the refcount for a node and schedules tunnel teardown if idle.
func (m *NodeManager) Release(sni string) {
	hostname, err := deriveValidatedTunnelHostname(sni)
	if err != nil {
		return
	}

	m.mu.Lock()
	st, ok := m.nodes[hostname]
	if !ok {
		m.mu.Unlock()
		return
	}

	if st.refCount > 0 {
		st.refCount--
	}

	if st.refCount == 0 && st.idleTimer == nil {
		st.idleTimer = time.AfterFunc(m.idleTimeout, func() {
			m.stopNode(hostname, false)
		})
	}
	m.mu.Unlock()
}

func (m *NodeManager) launchTunnel(st *nodeState, ready chan struct{}) {
	hostname := st.hostname
	m.mu.Lock()
	port := st.port
	m.mu.Unlock()

	if port == 0 {
		var err error
		port, err = m.ports.reserve()
		if err != nil {
			m.logger.Errorf("port reservation failed for %s: %v", hostname, err)
			m.mu.Lock()
			st.startErr = err
			if st.ready == ready {
				close(ready)
				st.ready = nil
			}
			m.mu.Unlock()
			return
		}
		m.mu.Lock()
		st.port = port
		m.mu.Unlock()
	}

	m.logger.Infof("Starting cloudflared for %s on %d", hostname, port)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "cloudflared", "access", "tcp", "--hostname", hostname, "--url", fmt.Sprintf("localhost:%d", port))

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		m.logger.Errorf("cloudflared start failed for %s: %v", hostname, err)
		m.mu.Lock()
		st.startErr = err
		st.cmd = nil
		st.cancel = nil
		st.port = 0
		if st.ready == ready {
			close(ready)
			st.ready = nil
		}
		m.mu.Unlock()
		m.ports.release(port)
		cancel()
		return
	}

	go streamPipe(m.logger, stdout, fmt.Sprintf("[%s][cloudflared][stdout]", hostname))
	go streamPipe(m.logger, stderr, fmt.Sprintf("[%s][cloudflared][stderr]", hostname))

	m.mu.Lock()
	st.cmd = cmd
	st.cancel = cancel
	st.startErr = nil
	m.mu.Unlock()

	err := waitForPort(ctx, "127.0.0.1", port, m.startupTimeout)
	if err != nil {
		m.logger.Errorf("cloudflared not ready for %s: %v", hostname, err)
		cancel()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		m.mu.Lock()
		st.cmd = nil
		st.cancel = nil
		st.startErr = err
		st.port = 0
		if st.ready == ready {
			close(ready)
			st.ready = nil
		}
		m.mu.Unlock()
		m.ports.release(port)
		return
	}

	m.mu.Lock()
	st.startErr = nil
	st.restarts = 0
	m.mu.Unlock()
	close(ready)

	go func() {
		err := cmd.Wait()
		cancel()
		m.logger.Errorf("cloudflared exited for %s: %v", hostname, err)
		m.handleProcessExit(st, err)
	}()
}

func (m *NodeManager) handleProcessExit(st *nodeState, err error) {
	hostname := st.hostname
	m.mu.Lock()
	active := st.refCount
	st.cmd = nil
	st.cancel = nil
	st.ready = nil
	st.startErr = fmt.Errorf("tunnel exited: %v", err)
	st.restarts++
	restarts := st.restarts
	m.mu.Unlock()

	if active > 0 && restarts <= m.maxRestarts {
		backoff := time.Duration(restarts) * m.restartBackoff
		m.logger.Infof("Restarting cloudflared for %s (active=%d, attempt=%d, backoff=%s)", hostname, active, restarts, backoff)
		m.mu.Lock()
		if st.ready == nil && st.cmd == nil {
			st.ready = make(chan struct{})
			ready := st.ready
			m.mu.Unlock()
			time.AfterFunc(backoff, func() {
				m.launchTunnel(st, ready)
			})
		} else {
			m.mu.Unlock()
		}
	} else if active > 0 {
		m.logger.Errorf("Max restart attempts reached for %s; not restarting", hostname)
	}
}

func (m *NodeManager) stopNode(hostname string, force bool) {
	m.mu.Lock()
	st, ok := m.nodes[hostname]
	if !ok {
		m.mu.Unlock()
		return
	}
	if st.refCount > 0 && !force {
		m.mu.Unlock()
		return
	}
	cmd := st.cmd
	cancel := st.cancel
	port := st.port
	st.cmd = nil
	st.cancel = nil
	st.ready = nil
	st.startErr = fmt.Errorf("tunnel stopped")
	st.idleTimer = nil
	st.port = 0
	m.mu.Unlock()

	m.logger.Infof("Stopping cloudflared for %s (idle=%v)", hostname, force)
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_, _ = cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	if port != 0 {
		m.ports.release(port)
	}
}

// Shutdown stops accepting new tunnels and tears down all running nodes.
func (m *NodeManager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	hostnames := make([]string, 0, len(m.nodes))
	for h := range m.nodes {
		hostnames = append(hostnames, h)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, h := range hostnames {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			m.stopNode(host, true)
		}(h)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
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

func streamPipe(logger *logging.Logger, r io.ReadCloser, prefix string) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		logger.Infof("%s %s", prefix, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Errorf("%s stream error: %v", prefix, err)
	}
}

func isPortAvailable(port int) bool {
	if port <= 0 {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
