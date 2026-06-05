package configs

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	unsetAllEnv(t)
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("expected no error for defaults, got %v", err)
	}

	if cfg.ListenHost != defaultListenHost {
		t.Fatalf("ListenHost: got %q, want %q", cfg.ListenHost, defaultListenHost)
	}
	if cfg.HealthPort != defaultHealthPort {
		t.Fatalf("HealthPort: got %d, want %d", cfg.HealthPort, defaultHealthPort)
	}
	if cfg.PublicPortRangeStart != defaultPublicPortRangeStart || cfg.PublicPortRangeEnd != defaultPublicPortRangeEnd {
		t.Fatalf("PublicPortRange: got %d-%d, want %d-%d", cfg.PublicPortRangeStart, cfg.PublicPortRangeEnd, defaultPublicPortRangeStart, defaultPublicPortRangeEnd)
	}
	if cfg.LocalTunnelManagerBaseURL != defaultLocalTunnelManagerBaseURL {
		t.Fatalf("LocalTunnelManagerBaseURL: got %q, want %q", cfg.LocalTunnelManagerBaseURL, defaultLocalTunnelManagerBaseURL)
	}
	if cfg.TunnelManagerBaseURL != defaultTunnelManagerBaseURL {
		t.Fatalf("TunnelManagerBaseURL: got %q, want %q", cfg.TunnelManagerBaseURL, defaultTunnelManagerBaseURL)
	}
	if cfg.RouteLookupTimeout != defaultRouteLookupTimeout {
		t.Fatalf("RouteLookupTimeout: got %v, want %v", cfg.RouteLookupTimeout, defaultRouteLookupTimeout)
	}
	if cfg.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout: got %v, want %v", cfg.IdleTimeout, defaultIdleTimeout)
	}
	if cfg.StartupTimeout != defaultStartupTimeout {
		t.Fatalf("StartupTimeout: got %v, want %v", cfg.StartupTimeout, defaultStartupTimeout)
	}
	if cfg.LocalPortRangeStart != defaultLocalPortRangeStart || cfg.LocalPortRangeEnd != defaultLocalPortRangeEnd {
		t.Fatalf("LocalPortRange: got %d-%d, want %d-%d", cfg.LocalPortRangeStart, cfg.LocalPortRangeEnd, defaultLocalPortRangeStart, defaultLocalPortRangeEnd)
	}
	if cfg.LogFormat != defaultLogFormat {
		t.Fatalf("LogFormat: got %q, want %q", cfg.LogFormat, defaultLogFormat)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envListenHost, "127.0.0.1")
	t.Setenv(envHealthPort, "29998")
	t.Setenv(envPublicPortRangeStart, "35000")
	t.Setenv(envPublicPortRangeEnd, "35010")
	t.Setenv(envLocalTunnelManagerBaseURL, "http://127.0.0.1:31033")
	t.Setenv(envTunnelManagerBaseURL, "https://tunnel-manager.example.com")
	t.Setenv(envRouteLookupTimeout, "4s")
	t.Setenv(envIdleTimeout, "42s")
	t.Setenv(envStartupTimeout, "5s")
	t.Setenv(envLocalPortRangeStart, "25000")
	t.Setenv(envLocalPortRangeEnd, "25010")
	t.Setenv(envLogFormat, "json")
	t.Setenv(envRestartBackoff, "1s")
	t.Setenv(envMaxRestarts, "5")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("expected no error for valid overrides, got %v", err)
	}

	if cfg.ListenHost != "127.0.0.1" {
		t.Fatalf("ListenHost override failed, got %q", cfg.ListenHost)
	}
	if cfg.HealthPort != 29998 {
		t.Fatalf("HealthPort override failed, got %d", cfg.HealthPort)
	}
	if cfg.PublicPortRangeStart != 35000 || cfg.PublicPortRangeEnd != 35010 {
		t.Fatalf("PublicPortRange override failed, got %d-%d", cfg.PublicPortRangeStart, cfg.PublicPortRangeEnd)
	}
	if cfg.LocalTunnelManagerBaseURL != "http://127.0.0.1:31033" {
		t.Fatalf("LocalTunnelManagerBaseURL override failed, got %q", cfg.LocalTunnelManagerBaseURL)
	}
	if cfg.TunnelManagerBaseURL != "https://tunnel-manager.example.com" {
		t.Fatalf("TunnelManagerBaseURL override failed, got %q", cfg.TunnelManagerBaseURL)
	}
	if cfg.RouteLookupTimeout != 4*time.Second {
		t.Fatalf("RouteLookupTimeout override failed, got %v", cfg.RouteLookupTimeout)
	}
	if cfg.IdleTimeout != 42*time.Second {
		t.Fatalf("IdleTimeout override failed, got %v", cfg.IdleTimeout)
	}
	if cfg.StartupTimeout != 5*time.Second {
		t.Fatalf("StartupTimeout override failed, got %v", cfg.StartupTimeout)
	}
	if cfg.LocalPortRangeStart != 25000 || cfg.LocalPortRangeEnd != 25010 {
		t.Fatalf("LocalPortRange override failed, got %d-%d", cfg.LocalPortRangeStart, cfg.LocalPortRangeEnd)
	}
	if cfg.LogFormat != "json" {
		t.Fatalf("LogFormat override failed, got %q", cfg.LogFormat)
	}
	if cfg.RestartBackoff != time.Second {
		t.Fatalf("RestartBackoff override failed, got %v", cfg.RestartBackoff)
	}
	if cfg.MaxRestarts != 5 {
		t.Fatalf("MaxRestarts override failed, got %d", cfg.MaxRestarts)
	}
}

func TestLoadConfigInvalidValues(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envListenHost, "bad host")
	t.Setenv(envHealthPort, "65536")
	t.Setenv(envPublicPortRangeStart, "40000")
	t.Setenv(envPublicPortRangeEnd, "30000")
	t.Setenv(envTunnelManagerBaseURL, "")
	t.Setenv(envRouteLookupTimeout, "-1s")
	t.Setenv(envIdleTimeout, "bogus")
	t.Setenv(envStartupTimeout, "0s")
	t.Setenv(envLocalPortRangeStart, "30000")
	t.Setenv(envLocalPortRangeEnd, "20000")
	t.Setenv(envLogFormat, "xml")
	t.Setenv(envRestartBackoff, "-1s")
	t.Setenv(envMaxRestarts, "0")

	cfg, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid env values")
	}

	if cfg.ListenHost != defaultListenHost {
		t.Fatalf("ListenHost should reset to default on invalid, got %q", cfg.ListenHost)
	}
	if cfg.HealthPort != defaultHealthPort {
		t.Fatalf("HealthPort should reset to default on invalid, got %d", cfg.HealthPort)
	}
	if cfg.PublicPortRangeStart != defaultPublicPortRangeStart || cfg.PublicPortRangeEnd != defaultPublicPortRangeEnd {
		t.Fatalf("public port range should reset to defaults on invalid order, got %d-%d", cfg.PublicPortRangeStart, cfg.PublicPortRangeEnd)
	}
	if cfg.RouteLookupTimeout != defaultRouteLookupTimeout {
		t.Fatalf("RouteLookupTimeout should stay default on invalid, got %v", cfg.RouteLookupTimeout)
	}
	if cfg.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout should stay default on invalid, got %v", cfg.IdleTimeout)
	}
	if cfg.StartupTimeout != defaultStartupTimeout {
		t.Fatalf("StartupTimeout should stay default on invalid, got %v", cfg.StartupTimeout)
	}
	if cfg.LocalPortRangeStart != defaultLocalPortRangeStart || cfg.LocalPortRangeEnd != defaultLocalPortRangeEnd {
		t.Fatalf("local port range should reset to defaults on invalid order, got %d-%d", cfg.LocalPortRangeStart, cfg.LocalPortRangeEnd)
	}
	if cfg.LogFormat != defaultLogFormat {
		t.Fatalf("LogFormat should remain default on invalid, got %q", cfg.LogFormat)
	}
	if cfg.RestartBackoff != defaultRestartBackoff {
		t.Fatalf("RestartBackoff should reset to default on invalid, got %v", cfg.RestartBackoff)
	}
	if cfg.MaxRestarts != defaultMaxRestarts {
		t.Fatalf("MaxRestarts should reset to default on invalid, got %d", cfg.MaxRestarts)
	}
}

func TestLoadConfigRejectsHealthPortOverlap(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envHealthPort, "30000")

	cfg, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for health port overlapping public port range")
	}
	if cfg.HealthPort != defaultHealthPort {
		t.Fatalf("health port should reset to default after overlap, got %d", cfg.HealthPort)
	}
}

func TestLoadConfigRejectsPortsAboveTCPMaximum(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envPublicPortRangeStart, "65536")
	t.Setenv(envPublicPortRangeEnd, "65537")
	t.Setenv(envLocalPortRangeStart, "65536")
	t.Setenv(envLocalPortRangeEnd, "65537")

	cfg, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid port ranges")
	}
	if cfg.PublicPortRangeStart != defaultPublicPortRangeStart || cfg.PublicPortRangeEnd != defaultPublicPortRangeEnd {
		t.Fatalf("public port range should reset to defaults, got %d-%d", cfg.PublicPortRangeStart, cfg.PublicPortRangeEnd)
	}
	if cfg.LocalPortRangeStart != defaultLocalPortRangeStart || cfg.LocalPortRangeEnd != defaultLocalPortRangeEnd {
		t.Fatalf("local port range should reset to defaults, got %d-%d", cfg.LocalPortRangeStart, cfg.LocalPortRangeEnd)
	}
}

func TestLoadConfigAllowsDisablingLocalTunnelManager(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envLocalTunnelManagerBaseURL, "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("expected no error when local tunnel manager URL is disabled, got %v", err)
	}
	if cfg.LocalTunnelManagerBaseURL != "" {
		t.Fatalf("LocalTunnelManagerBaseURL should be disabled, got %q", cfg.LocalTunnelManagerBaseURL)
	}
}

func unsetAllEnv(t *testing.T) {
	t.Helper()
	os.Unsetenv(envListenHost)
	os.Unsetenv(envHealthPort)
	os.Unsetenv(envPublicPortRangeStart)
	os.Unsetenv(envPublicPortRangeEnd)
	os.Unsetenv(envLocalTunnelManagerBaseURL)
	os.Unsetenv(envTunnelManagerBaseURL)
	os.Unsetenv(envRouteLookupTimeout)
	os.Unsetenv(envIdleTimeout)
	os.Unsetenv(envStartupTimeout)
	os.Unsetenv(envLocalPortRangeStart)
	os.Unsetenv(envLocalPortRangeEnd)
	os.Unsetenv(envLogFormat)
	os.Unsetenv(envRestartBackoff)
	os.Unsetenv(envMaxRestarts)
}
