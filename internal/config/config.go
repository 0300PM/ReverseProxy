package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	AdminTokenEnv  = "ADMIN_TOKEN"
	RedisSecretEnv = "REDIS_PASSWORD"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	d.Duration = v
	return nil
}

type Backend struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type LoadBalancing struct {
	Strategy string `yaml:"strategy"`
}

type HealthCheck struct {
	Interval           Duration `yaml:"interval"`
	Timeout            Duration `yaml:"timeout"`
	Path               string   `yaml:"path"`
	HealthyThreshold   int      `yaml:"healthy_threshold"`
	UnhealthyThreshold int      `yaml:"unhealthy_threshold"`
}

type RateLimit struct {
	RequestsPerMinute int      `yaml:"requests_per_minute"`
	Burst             int      `yaml:"burst"`
	TrustedProxies    []string `yaml:"trusted_proxies"`
}

type Timeouts struct {
	BackendDial           Duration `yaml:"backend_dial"`
	BackendResponseHeader Duration `yaml:"backend_response_header"`
	BackendIdleConn       Duration `yaml:"backend_idle_conn"`
	BackendTLSHandshake   Duration `yaml:"backend_tls_handshake"`
	BackendMaxIdleConns   int      `yaml:"backend_max_idle_conns"`
	BackendMaxIdlePerHost int      `yaml:"backend_max_idle_conns_per_host"`
	ServerRead            Duration `yaml:"server_read"`
	ServerReadHeader      Duration `yaml:"server_read_header"`
	ServerWrite           Duration `yaml:"server_write"`
	ServerIdle            Duration `yaml:"server_idle"`
	Shutdown              Duration `yaml:"shutdown"`
}

type Retry struct {
	MaxAttempts int `yaml:"max_attempts"`
}

type CircuitBreaker struct {
	FailureThreshold int      `yaml:"failure_threshold"`
	OpenTimeout      Duration `yaml:"open_timeout"`
	HalfOpenProbes   int      `yaml:"half_open_probes"`
}

type TLS struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

func (t TLS) Enabled() bool {
	return t.CertFile != "" && t.KeyFile != ""
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Redis struct {
	Addr string `yaml:"addr"`
}

type Config struct {
	Listen         string         `yaml:"listen"`
	Backends       []Backend      `yaml:"backends"`
	LoadBalancing  LoadBalancing  `yaml:"load_balancing"`
	HealthCheck    HealthCheck    `yaml:"health_check"`
	RateLimit      RateLimit      `yaml:"rate_limit"`
	Timeouts       Timeouts       `yaml:"timeouts"`
	Retry          Retry          `yaml:"retry"`
	CircuitBreaker CircuitBreaker `yaml:"circuit_breaker"`
	TLS            TLS            `yaml:"tls"`
	Log            Log            `yaml:"log"`
	Redis          Redis          `yaml:"redis"`
}

func Default() *Config {
	return &Config{
		Listen: ":8080",
		LoadBalancing: LoadBalancing{
			Strategy: "round_robin",
		},
		HealthCheck: HealthCheck{
			Interval:           Duration{5 * time.Second},
			Timeout:            Duration{2 * time.Second},
			Path:               "/health",
			HealthyThreshold:   2,
			UnhealthyThreshold: 3,
		},
		RateLimit: RateLimit{
			RequestsPerMinute: 100,
			Burst:             50,
		},
		Timeouts: Timeouts{
			BackendDial:           Duration{3 * time.Second},
			BackendResponseHeader: Duration{10 * time.Second},
			BackendIdleConn:       Duration{90 * time.Second},
			BackendTLSHandshake:   Duration{5 * time.Second},
			BackendMaxIdleConns:   256,
			BackendMaxIdlePerHost: 64,
			ServerRead:            Duration{15 * time.Second},
			ServerReadHeader:      Duration{5 * time.Second},
			ServerWrite:           Duration{30 * time.Second},
			ServerIdle:            Duration{60 * time.Second},
			Shutdown:              Duration{10 * time.Second},
		},
		Retry: Retry{
			MaxAttempts: 2,
		},
		CircuitBreaker: CircuitBreaker{
			FailureThreshold: 5,
			OpenTimeout:      Duration{30 * time.Second},
			HalfOpenProbes:   1,
		},
		Log: Log{
			Level:  "info",
			Format: "json",
		},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for i := range cfg.Backends {
		if cfg.Backends[i].Weight == 0 {
			cfg.Backends[i].Weight = 1
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	var errs []error
	if strings.TrimSpace(c.Listen) == "" {
		errs = append(errs, errors.New("listen must not be empty"))
	}
	if len(c.Backends) == 0 {
		errs = append(errs, errors.New("at least one backend is required"))
	}
	seen := make(map[string]struct{}, len(c.Backends))
	for i, b := range c.Backends {
		u, err := url.Parse(b.URL)
		if err != nil {
			errs = append(errs, fmt.Errorf("backend %d: %w", i, err))
			continue
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			errs = append(errs, fmt.Errorf("backend %d: scheme must be http or https", i))
		}
		if u.Host == "" {
			errs = append(errs, fmt.Errorf("backend %d: missing host", i))
		}
		if u.User != nil {
			errs = append(errs, fmt.Errorf("backend %d: credentials in URL are not allowed", i))
		}
		if u.RawQuery != "" || u.Fragment != "" {
			errs = append(errs, fmt.Errorf("backend %d: query and fragment are not allowed", i))
		}
		if b.Weight < 1 || b.Weight > 100 {
			errs = append(errs, fmt.Errorf("backend %d: weight must be between 1 and 100", i))
		}
		key := strings.TrimSuffix(u.String(), "/")
		if _, dup := seen[key]; dup {
			errs = append(errs, fmt.Errorf("backend %d: duplicate URL %s", i, b.URL))
		}
		seen[key] = struct{}{}
	}
	switch c.LoadBalancing.Strategy {
	case "round_robin", "weighted_round_robin", "least_connections":
	default:
		errs = append(errs, fmt.Errorf("unknown load balancing strategy %q", c.LoadBalancing.Strategy))
	}
	if c.HealthCheck.Interval.Duration <= 0 {
		errs = append(errs, errors.New("health_check.interval must be positive"))
	}
	if c.HealthCheck.Timeout.Duration <= 0 {
		errs = append(errs, errors.New("health_check.timeout must be positive"))
	}
	if !strings.HasPrefix(c.HealthCheck.Path, "/") {
		errs = append(errs, errors.New("health_check.path must start with /"))
	}
	if c.HealthCheck.HealthyThreshold < 1 {
		errs = append(errs, errors.New("health_check.healthy_threshold must be at least 1"))
	}
	if c.HealthCheck.UnhealthyThreshold < 1 {
		errs = append(errs, errors.New("health_check.unhealthy_threshold must be at least 1"))
	}
	if c.RateLimit.RequestsPerMinute < 1 {
		errs = append(errs, errors.New("rate_limit.requests_per_minute must be at least 1"))
	}
	if c.RateLimit.Burst < 1 {
		errs = append(errs, errors.New("rate_limit.burst must be at least 1"))
	}
	for i, p := range c.RateLimit.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil {
			if net.ParseIP(p) == nil {
				errs = append(errs, fmt.Errorf("rate_limit.trusted_proxies[%d]: %w", i, err))
			}
		}
	}
	for _, d := range []struct {
		name string
		val  Duration
	}{
		{"timeouts.backend_dial", c.Timeouts.BackendDial},
		{"timeouts.backend_response_header", c.Timeouts.BackendResponseHeader},
		{"timeouts.backend_idle_conn", c.Timeouts.BackendIdleConn},
		{"timeouts.backend_tls_handshake", c.Timeouts.BackendTLSHandshake},
		{"timeouts.server_read", c.Timeouts.ServerRead},
		{"timeouts.server_read_header", c.Timeouts.ServerReadHeader},
		{"timeouts.server_write", c.Timeouts.ServerWrite},
		{"timeouts.server_idle", c.Timeouts.ServerIdle},
		{"timeouts.shutdown", c.Timeouts.Shutdown},
		{"circuit_breaker.open_timeout", c.CircuitBreaker.OpenTimeout},
	} {
		if d.val.Duration <= 0 {
			errs = append(errs, fmt.Errorf("%s must be positive", d.name))
		}
	}
	if c.Timeouts.BackendMaxIdleConns < 1 {
		errs = append(errs, errors.New("timeouts.backend_max_idle_conns must be at least 1"))
	}
	if c.Timeouts.BackendMaxIdlePerHost < 1 {
		errs = append(errs, errors.New("timeouts.backend_max_idle_conns_per_host must be at least 1"))
	}
	if c.Retry.MaxAttempts < 1 || c.Retry.MaxAttempts > 10 {
		errs = append(errs, errors.New("retry.max_attempts must be between 1 and 10"))
	}
	if c.CircuitBreaker.FailureThreshold < 1 {
		errs = append(errs, errors.New("circuit_breaker.failure_threshold must be at least 1"))
	}
	if c.CircuitBreaker.HalfOpenProbes < 1 {
		errs = append(errs, errors.New("circuit_breaker.half_open_probes must be at least 1"))
	}
	if c.TLS.Enabled() {
		for _, f := range []string{c.TLS.CertFile, c.TLS.KeyFile} {
			if _, err := os.Stat(f); err != nil {
				errs = append(errs, fmt.Errorf("tls file: %w", err))
			}
		}
	} else if c.TLS.CertFile != "" || c.TLS.KeyFile != "" {
		errs = append(errs, errors.New("tls.cert_file and tls.key_file must both be set"))
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("unknown log.level %q", c.Log.Level))
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("unknown log.format %q", c.Log.Format))
	}
	if c.Redis.Addr != "" && strings.Contains(c.Redis.Addr, " ") {
		errs = append(errs, errors.New("redis.addr must not contain spaces"))
	}
	return errors.Join(errs...)
}
