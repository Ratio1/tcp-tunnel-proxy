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
	ListenHost           string
	PublicPortRangeStart int
	PublicPortRangeEnd   int
	TunnelManagerBaseURL string
	RouteLookupTimeout   time.Duration
	IdleTimeout          time.Duration
	StartupTimeout       time.Duration
	LocalPortRangeStart  int
	LocalPortRangeEnd    int
	LogFormat            string // plain | json
	RestartBackoff       time.Duration
	MaxRestarts          int
}

const (
	defaultListenHost           = ""
	defaultPublicPortRangeStart = 30000
	defaultPublicPortRangeEnd   = 30499
	defaultTunnelManagerBaseURL = "https://1f8b266e9dbf.ratio1.link"
	defaultRouteLookupTimeout   = 5 * time.Second
	defaultIdleTimeout          = 300 * time.Second
	defaultStartupTimeout       = 15 * time.Second
	defaultLocalPortRangeStart  = 20000
	defaultLocalPortRangeEnd    = 20100
	defaultLogFormat            = "plain"
	defaultRestartBackoff       = 2 * time.Second
	defaultMaxRestarts          = 3
	maxTCPPort                  = 65535
)

const (
	envListenHost           = "LISTEN_HOST"
	envPublicPortRangeStart = "PUBLIC_PORT_RANGE_START"
	envPublicPortRangeEnd   = "PUBLIC_PORT_RANGE_END"
	envTunnelManagerBaseURL = "TUNNEL_MANAGER_BASE_URL"
	envRouteLookupTimeout   = "ROUTE_LOOKUP_TIMEOUT"
	envIdleTimeout          = "IDLE_TIMEOUT"
	envStartupTimeout       = "STARTUP_TIMEOUT"
	envLocalPortRangeStart  = "LOCAL_PORT_RANGE_START"
	envLocalPortRangeEnd    = "LOCAL_PORT_RANGE_END"
	envLogFormat            = "LOG_FORMAT"
	envRestartBackoff       = "RESTART_BACKOFF"
	envMaxRestarts          = "MAX_RESTARTS"
)

// LoadConfigFromEnv returns configuration populated from environment variables, falling back to defaults.
// It returns validation/parse errors so callers can decide how to handle them.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		ListenHost:           defaultListenHost,
		PublicPortRangeStart: defaultPublicPortRangeStart,
		PublicPortRangeEnd:   defaultPublicPortRangeEnd,
		TunnelManagerBaseURL: defaultTunnelManagerBaseURL,
		RouteLookupTimeout:   defaultRouteLookupTimeout,
		IdleTimeout:          defaultIdleTimeout,
		StartupTimeout:       defaultStartupTimeout,
		LocalPortRangeStart:  defaultLocalPortRangeStart,
		LocalPortRangeEnd:    defaultLocalPortRangeEnd,
		LogFormat:            defaultLogFormat,
		RestartBackoff:       defaultRestartBackoff,
		MaxRestarts:          defaultMaxRestarts,
	}

	var errs []error

	if v := strings.TrimSpace(os.Getenv(envListenHost)); v != "" {
		cfg.ListenHost = v
	}

	if v := strings.TrimSpace(os.Getenv(envPublicPortRangeStart)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envPublicPortRangeStart, v, err))
		} else {
			cfg.PublicPortRangeStart = n
		}
	}

	if v := strings.TrimSpace(os.Getenv(envPublicPortRangeEnd)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envPublicPortRangeEnd, v, err))
		} else {
			cfg.PublicPortRangeEnd = n
		}
	}

	if v := strings.TrimSpace(os.Getenv(envTunnelManagerBaseURL)); v != "" {
		cfg.TunnelManagerBaseURL = v
	}

	if v := strings.TrimSpace(os.Getenv(envRouteLookupTimeout)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envRouteLookupTimeout, v, err))
		} else {
			cfg.RouteLookupTimeout = d
		}
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

	if v := strings.TrimSpace(os.Getenv(envLocalPortRangeStart)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envLocalPortRangeStart, v, err))
		} else {
			cfg.LocalPortRangeStart = n
		}
	}

	if v := strings.TrimSpace(os.Getenv(envLocalPortRangeEnd)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envLocalPortRangeEnd, v, err))
		} else {
			cfg.LocalPortRangeEnd = n
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

	if v := strings.TrimSpace(os.Getenv(envRestartBackoff)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envRestartBackoff, v, err))
		} else {
			cfg.RestartBackoff = d
		}
	}

	if v := strings.TrimSpace(os.Getenv(envMaxRestarts)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("invalid %s: %q (%v)", envMaxRestarts, v, err))
		} else {
			cfg.MaxRestarts = n
		}
	}

	if err := validateConfig(&cfg); err != nil {
		errs = append(errs, err)
	}

	return cfg, errors.Join(errs...)
}

func validateConfig(cfg *Config) error {
	var errs []error

	if cfg.ListenHost != "" {
		if _, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(cfg.ListenHost, "0")); err != nil {
			errs = append(errs, fmt.Errorf("invalid listen host %q: %w", cfg.ListenHost, err))
			cfg.ListenHost = defaultListenHost
		}
	}
	if cfg.PublicPortRangeStart <= 0 || cfg.PublicPortRangeStart > maxTCPPort {
		errs = append(errs, fmt.Errorf("public port range start must be between 1 and %d, got %d", maxTCPPort, cfg.PublicPortRangeStart))
		cfg.PublicPortRangeStart = defaultPublicPortRangeStart
	}
	if cfg.PublicPortRangeEnd <= 0 || cfg.PublicPortRangeEnd > maxTCPPort || cfg.PublicPortRangeEnd < cfg.PublicPortRangeStart {
		errs = append(errs, fmt.Errorf("public port range end must be between start and %d, got %d-%d", maxTCPPort, cfg.PublicPortRangeStart, cfg.PublicPortRangeEnd))
		cfg.PublicPortRangeStart = defaultPublicPortRangeStart
		cfg.PublicPortRangeEnd = defaultPublicPortRangeEnd
	}
	if strings.TrimSpace(cfg.TunnelManagerBaseURL) == "" {
		errs = append(errs, fmt.Errorf("tunnel manager base URL must not be empty"))
		cfg.TunnelManagerBaseURL = defaultTunnelManagerBaseURL
	}
	if cfg.RouteLookupTimeout <= 0 {
		errs = append(errs, fmt.Errorf("route lookup timeout must be positive, got %s", cfg.RouteLookupTimeout))
		cfg.RouteLookupTimeout = defaultRouteLookupTimeout
	}
	if cfg.IdleTimeout <= 0 {
		errs = append(errs, fmt.Errorf("idle timeout must be positive, got %s", cfg.IdleTimeout))
		cfg.IdleTimeout = defaultIdleTimeout
	}
	if cfg.StartupTimeout <= 0 {
		errs = append(errs, fmt.Errorf("startup timeout must be positive, got %s", cfg.StartupTimeout))
		cfg.StartupTimeout = defaultStartupTimeout
	}
	if cfg.LocalPortRangeStart <= 0 || cfg.LocalPortRangeStart > maxTCPPort {
		errs = append(errs, fmt.Errorf("local port range start must be between 1 and %d, got %d", maxTCPPort, cfg.LocalPortRangeStart))
		cfg.LocalPortRangeStart = defaultLocalPortRangeStart
	}
	if cfg.LocalPortRangeEnd <= 0 || cfg.LocalPortRangeEnd > maxTCPPort || cfg.LocalPortRangeEnd < cfg.LocalPortRangeStart {
		errs = append(errs, fmt.Errorf("local port range end must be between start and %d, got %d-%d", maxTCPPort, cfg.LocalPortRangeStart, cfg.LocalPortRangeEnd))
		cfg.LocalPortRangeStart = defaultLocalPortRangeStart
		cfg.LocalPortRangeEnd = defaultLocalPortRangeEnd
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = defaultLogFormat
	}
	if cfg.RestartBackoff <= 0 {
		errs = append(errs, fmt.Errorf("restart backoff must be positive, got %s", cfg.RestartBackoff))
		cfg.RestartBackoff = defaultRestartBackoff
	}
	if cfg.MaxRestarts <= 0 {
		errs = append(errs, fmt.Errorf("max restarts must be positive, got %d", cfg.MaxRestarts))
		cfg.MaxRestarts = defaultMaxRestarts
	}

	return errors.Join(errs...)
}
