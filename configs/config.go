package configs

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr       string
	IdleTimeout      time.Duration
	StartupTimeout   time.Duration
	ReadHelloTimeout time.Duration
	PortRangeStart   int
	PortRangeEnd     int
	LogFormat        string // plain | json
}

const (
	defaultListenAddr       = ":19000"
	defaultIdleTimeout      = 300 * time.Second
	defaultStartupTimeout   = 15 * time.Second
	defaultReadHelloTimeout = 10 * time.Second
	defaultPortRangeStart   = 20000
	defaultPortRangeEnd     = 20100
	defaultLogFormat        = "plain"
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

// LoadConfigFromEnv returns configuration populated from environment variables, falling back to defaults.
// It returns validation/parse errors so callers can decide how to handle them.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:       defaultListenAddr,
		IdleTimeout:      defaultIdleTimeout,
		StartupTimeout:   defaultStartupTimeout,
		ReadHelloTimeout: defaultReadHelloTimeout,
		PortRangeStart:   defaultPortRangeStart,
		PortRangeEnd:     defaultPortRangeEnd,
		LogFormat:        defaultLogFormat,
	}

	var errs []error

	if v := strings.TrimSpace(os.Getenv(envListenAddr)); v != "" {
		cfg.ListenAddr = v
	}

	if v := strings.TrimSpace(os.Getenv(envIdleTimeout)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envIdleTimeout, v, err))
		} else {
			cfg.IdleTimeout = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envStartupTimeout)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envStartupTimeout, v, err))
		} else {
			cfg.StartupTimeout = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envReadHello)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envReadHello, v, err))
		} else {
			cfg.ReadHelloTimeout = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envPortRangeStart)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envPortRangeStart, v, err))
		} else {
			cfg.PortRangeStart = n
		}
	}

	if v := strings.TrimSpace(os.Getenv(envPortRangeEnd)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envPortRangeEnd, v, err))
		} else {
			cfg.PortRangeEnd = n
		}
	}

	if v := strings.TrimSpace(os.Getenv(envLogFormat)); v != "" {
		switch strings.ToLower(v) {
		case "plain", "json":
			cfg.LogFormat = v
		default:
			errs = append(errs, fmt.Errorf("invalid %s: %q (must be plain|json)", envLogFormat, v))
		}
	}

	if err := validateConfig(&cfg); err != nil {
		errs = append(errs, err)
	}

	return cfg, errors.Join(errs...)
}

func validateConfig(cfg *Config) error {
	var errs []error

	if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddr); err != nil {
		errs = append(errs, fmt.Errorf("invalid listen address %q: %w", cfg.ListenAddr, err))
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.IdleTimeout <= 0 {
		errs = append(errs, fmt.Errorf("idle timeout must be positive, got %s", cfg.IdleTimeout))
		cfg.IdleTimeout = defaultIdleTimeout
	}
	if cfg.StartupTimeout <= 0 {
		errs = append(errs, fmt.Errorf("startup timeout must be positive, got %s", cfg.StartupTimeout))
		cfg.StartupTimeout = defaultStartupTimeout
	}
	if cfg.ReadHelloTimeout <= 0 {
		errs = append(errs, fmt.Errorf("read hello timeout must be positive, got %s", cfg.ReadHelloTimeout))
		cfg.ReadHelloTimeout = defaultReadHelloTimeout
	}
	if cfg.PortRangeStart <= 0 {
		errs = append(errs, fmt.Errorf("port range start must be positive, got %d", cfg.PortRangeStart))
		cfg.PortRangeStart = defaultPortRangeStart
	}
	if cfg.PortRangeEnd <= 0 || cfg.PortRangeEnd < cfg.PortRangeStart {
		errs = append(errs, fmt.Errorf("port range end must be >= start, got %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd))
		cfg.PortRangeStart = defaultPortRangeStart
		cfg.PortRangeEnd = defaultPortRangeEnd
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = defaultLogFormat
	}

	return errors.Join(errs...)
}
