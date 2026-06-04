package routeprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	cloudflaredmanager "tcp-tunnel-proxy/internal/cloudflared_manager"
)

type HTTPProvider struct {
	baseURL string
	client  *http.Client
	timeout time.Duration
	mu      sync.Mutex
	cache   map[int]string
}

func NewHTTPProvider(baseURL string, timeout time.Duration) (*HTTPProvider, error) {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid tunnel manager base URL %q", baseURL)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("route lookup timeout must be positive")
	}
	return &HTTPProvider{
		baseURL: normalized,
		client:  &http.Client{},
		timeout: timeout,
		cache:   make(map[int]string),
	}, nil
}

func (p *HTTPProvider) GetHostname(ctx context.Context, publicPort int) (string, error) {
	if publicPort <= 0 || publicPort > 65535 {
		return "", fmt.Errorf("invalid public port %d", publicPort)
	}

	p.mu.Lock()
	if hostname, ok := p.cache[publicPort]; ok {
		p.mu.Unlock()
		return hostname, nil
	}
	p.mu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	u, err := url.Parse(p.baseURL + "/get_tcp_route")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("public_port", strconv.Itoa(publicPort))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("tcp route lookup for port %d failed with status %d", publicPort, resp.StatusCode)
	}

	hostname, err := decodeHostname(resp)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	p.cache[publicPort] = hostname
	p.mu.Unlock()
	return hostname, nil
}

func decodeHostname(resp *http.Response) (string, error) {
	var raw any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}

	switch v := raw.(type) {
	case string:
		return requireHostname(v)
	case map[string]any:
		result, ok := v["result"].(string)
		if !ok {
			return "", fmt.Errorf("route lookup response missing string result")
		}
		return requireHostname(result)
	default:
		return "", fmt.Errorf("route lookup response must be a hostname string")
	}
}

func requireHostname(value string) (string, error) {
	hostname, err := cloudflaredmanager.NormalizeHostname(value)
	if err != nil {
		return "", fmt.Errorf("route lookup returned invalid hostname: %w", err)
	}
	return hostname, nil
}
