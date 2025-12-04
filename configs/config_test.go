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

	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("ListenAddr: got %q, want %q", cfg.ListenAddr, defaultListenAddr)
	}
	if cfg.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout: got %v, want %v", cfg.IdleTimeout, defaultIdleTimeout)
	}
	if cfg.StartupTimeout != defaultStartupTimeout {
		t.Fatalf("StartupTimeout: got %v, want %v", cfg.StartupTimeout, defaultStartupTimeout)
	}
	if cfg.ReadHelloTimeout != defaultReadHelloTimeout {
		t.Fatalf("ReadHelloTimeout: got %v, want %v", cfg.ReadHelloTimeout, defaultReadHelloTimeout)
	}
	if cfg.PortRangeStart != defaultPortRangeStart || cfg.PortRangeEnd != defaultPortRangeEnd {
		t.Fatalf("PortRange: got %d-%d, want %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd, defaultPortRangeStart, defaultPortRangeEnd)
	}
	if cfg.LogFormat != defaultLogFormat {
		t.Fatalf("LogFormat: got %q, want %q", cfg.LogFormat, defaultLogFormat)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envListenAddr, "127.0.0.1:12345")
	t.Setenv(envIdleTimeout, "42s")
	t.Setenv(envStartupTimeout, "5s")
	t.Setenv(envReadHello, "3s")
	t.Setenv(envPortRangeStart, "25000")
	t.Setenv(envPortRangeEnd, "25010")
	t.Setenv(envLogFormat, "json")
	t.Setenv(envRestartBackoff, "1s")
	t.Setenv(envMaxRestarts, "5")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("expected no error for valid overrides, got %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:12345" {
		t.Fatalf("ListenAddr override failed, got %q", cfg.ListenAddr)
	}
	if cfg.IdleTimeout != 42*time.Second {
		t.Fatalf("IdleTimeout override failed, got %v", cfg.IdleTimeout)
	}
	if cfg.StartupTimeout != 5*time.Second {
		t.Fatalf("StartupTimeout override failed, got %v", cfg.StartupTimeout)
	}
	if cfg.ReadHelloTimeout != 3*time.Second {
		t.Fatalf("ReadHelloTimeout override failed, got %v", cfg.ReadHelloTimeout)
	}
	if cfg.PortRangeStart != 25000 || cfg.PortRangeEnd != 25010 {
		t.Fatalf("PortRange override failed, got %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd)
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
	t.Setenv(envIdleTimeout, "bogus")
	t.Setenv(envReadHello, "-1s")
	t.Setenv(envPortRangeStart, "30000")
	t.Setenv(envPortRangeEnd, "20000") // end < start triggers validation error/reset
	t.Setenv(envLogFormat, "xml")
	t.Setenv(envListenAddr, "badaddr")
	t.Setenv(envRestartBackoff, "-1s")
	t.Setenv(envMaxRestarts, "0")

	cfg, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid env values")
	}

	if cfg.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout should stay default on invalid, got %v", cfg.IdleTimeout)
	}
	if cfg.ReadHelloTimeout != defaultReadHelloTimeout {
		t.Fatalf("ReadHelloTimeout should stay default on invalid, got %v", cfg.ReadHelloTimeout)
	}
	if cfg.PortRangeStart != defaultPortRangeStart || cfg.PortRangeEnd != defaultPortRangeEnd {
		t.Fatalf("Port range should reset to defaults on invalid order, got %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd)
	}
	if cfg.LogFormat != defaultLogFormat {
		t.Fatalf("LogFormat should remain default on invalid, got %q", cfg.LogFormat)
	}
	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("ListenAddr should reset to default on invalid, got %q", cfg.ListenAddr)
	}
	if cfg.RestartBackoff != defaultRestartBackoff {
		t.Fatalf("RestartBackoff should reset to default on invalid, got %v", cfg.RestartBackoff)
	}
	if cfg.MaxRestarts != defaultMaxRestarts {
		t.Fatalf("MaxRestarts should reset to default on invalid, got %d", cfg.MaxRestarts)
	}
}

func unsetAllEnv(t *testing.T) {
	t.Helper()
	os.Unsetenv(envListenAddr)
	os.Unsetenv(envIdleTimeout)
	os.Unsetenv(envStartupTimeout)
	os.Unsetenv(envReadHello)
	os.Unsetenv(envPortRangeStart)
	os.Unsetenv(envPortRangeEnd)
	os.Unsetenv(envLogFormat)
	os.Unsetenv(envRestartBackoff)
	os.Unsetenv(envMaxRestarts)
}
