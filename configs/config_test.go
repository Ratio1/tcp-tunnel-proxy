package configs

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigENVDefaults(t *testing.T) {
	unsetAllEnv(t)
	LoadConfigENV()

	if ListenAddr != defaultListenAddr {
		t.Fatalf("ListenAddr: got %q, want %q", ListenAddr, defaultListenAddr)
	}
	if IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout: got %v, want %v", IdleTimeout, defaultIdleTimeout)
	}
	if StartupTimeout != defaultStartupTimeout {
		t.Fatalf("StartupTimeout: got %v, want %v", StartupTimeout, defaultStartupTimeout)
	}
	if ReadHelloTimeout != defaultReadHelloTimeout {
		t.Fatalf("ReadHelloTimeout: got %v, want %v", ReadHelloTimeout, defaultReadHelloTimeout)
	}
	if PortRangeStart != defaultPortRangeStart || PortRangeEnd != defaultPortRangeEnd {
		t.Fatalf("PortRange: got %d-%d, want %d-%d", PortRangeStart, PortRangeEnd, defaultPortRangeStart, defaultPortRangeEnd)
	}
}

func TestLoadConfigENVOverrides(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envListenAddr, "127.0.0.1:12345")
	t.Setenv(envIdleTimeout, "42s")
	t.Setenv(envStartupTimeout, "5s")
	t.Setenv(envReadHello, "3s")
	t.Setenv(envPortRangeStart, "25000")
	t.Setenv(envPortRangeEnd, "25010")

	LoadConfigENV()

	if ListenAddr != "127.0.0.1:12345" {
		t.Fatalf("ListenAddr override failed, got %q", ListenAddr)
	}
	if IdleTimeout != 42*time.Second {
		t.Fatalf("IdleTimeout override failed, got %v", IdleTimeout)
	}
	if StartupTimeout != 5*time.Second {
		t.Fatalf("StartupTimeout override failed, got %v", StartupTimeout)
	}
	if ReadHelloTimeout != 3*time.Second {
		t.Fatalf("ReadHelloTimeout override failed, got %v", ReadHelloTimeout)
	}
	if PortRangeStart != 25000 || PortRangeEnd != 25010 {
		t.Fatalf("PortRange override failed, got %d-%d", PortRangeStart, PortRangeEnd)
	}
}

func TestLoadConfigENVInvalidValues(t *testing.T) {
	unsetAllEnv(t)
	t.Setenv(envIdleTimeout, "bogus")
	t.Setenv(envReadHello, "-1s")
	t.Setenv(envPortRangeStart, "30000")
	t.Setenv(envPortRangeEnd, "20000") // end < start triggers reset

	LoadConfigENV()

	if IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout should stay default on invalid, got %v", IdleTimeout)
	}
	if ReadHelloTimeout != defaultReadHelloTimeout {
		t.Fatalf("ReadHelloTimeout should stay default on invalid, got %v", ReadHelloTimeout)
	}
	if PortRangeStart != defaultPortRangeStart || PortRangeEnd != defaultPortRangeEnd {
		t.Fatalf("Port range should reset to defaults on invalid order, got %d-%d", PortRangeStart, PortRangeEnd)
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
}
