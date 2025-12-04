package configs

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr       = ":19000"
	defaultIdleTimeout      = 300 * time.Second
	defaultStartupTimeout   = 15 * time.Second
	defaultReadHelloTimeout = 10 * time.Second
	defaultPortRangeStart   = 20000
	defaultPortRangeEnd     = 20100
)

const (
	envListenAddr     = "LISTEN_ADDR"
	envIdleTimeout    = "IDLE_TIMEOUT"
	envStartupTimeout = "STARTUP_TIMEOUT"
	envReadHello      = "READ_HELLO_TIMEOUT"
	envPortRangeStart = "PORT_RANGE_START"
	envPortRangeEnd   = "PORT_RANGE_END"
	envLogFormat      = "LOG_FORMAT"
)

var (
	ListenAddr       = defaultListenAddr
	IdleTimeout      = defaultIdleTimeout
	StartupTimeout   = defaultStartupTimeout
	ReadHelloTimeout = defaultReadHelloTimeout
	PortRangeStart   = defaultPortRangeStart
	PortRangeEnd     = defaultPortRangeEnd
	LogFormat        = "plain" // plain | json
)

// LoadConfigENV overrides defaults from environment variables when provided.
func LoadConfigENV() {
	if v := strings.TrimSpace(os.Getenv(envListenAddr)); v != "" {
		ListenAddr = v
	}

	if v := strings.TrimSpace(os.Getenv(envIdleTimeout)); v != "" {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			log.Printf("Invalid IDLE_TIMEOUT %q, keeping default %s: %v", v, IdleTimeout, err)
		} else {
			IdleTimeout = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envStartupTimeout)); v != "" {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			log.Printf("Invalid STARTUP_TIMEOUT %q, keeping default %s: %v", v, StartupTimeout, err)
		} else {
			StartupTimeout = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envReadHello)); v != "" {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			log.Printf("Invalid READ_HELLO_TIMEOUT %q, keeping default %s: %v", v, ReadHelloTimeout, err)
		} else {
			ReadHelloTimeout = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envPortRangeStart)); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n <= 0 {
			log.Printf("Invalid PORT_RANGE_START %q, keeping default %d: %v", v, PortRangeStart, err)
		} else {
			PortRangeStart = n
		}
	}

	if v := strings.TrimSpace(os.Getenv(envPortRangeEnd)); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n <= 0 {
			log.Printf("Invalid PORT_RANGE_END %q, keeping default %d: %v", v, PortRangeEnd, err)
		} else {
			PortRangeEnd = n
		}
	}

	if PortRangeEnd < PortRangeStart {
		log.Printf("Invalid port range order %d-%d; resetting to defaults %d-%d", PortRangeStart, PortRangeEnd, defaultPortRangeStart, defaultPortRangeEnd)
		PortRangeStart = defaultPortRangeStart
		PortRangeEnd = defaultPortRangeEnd
	}

	if v := strings.TrimSpace(os.Getenv(envLogFormat)); v != "" {
		switch strings.ToLower(v) {
		case "json":
			LogFormat = "json"
		case "plain":
			LogFormat = "plain"
		default:
			log.Printf("Invalid LOG_FORMAT %q, keeping default %s", v, LogFormat)
		}
	}
}
