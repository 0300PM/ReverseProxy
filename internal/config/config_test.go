package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validYAML = `
listen: ":9090"
backends:
  - url: http://backend-a:8080
    weight: 2
  - url: https://backend-b:8443
load_balancing:
  strategy: least_connections
health_check:
  interval: 3s
  timeout: 1s
  path: /healthz
  healthy_threshold: 1
  unhealthy_threshold: 2
rate_limit:
  requests_per_minute: 60
  burst: 10
  trusted_proxies:
    - 10.0.0.0/8
    - 192.168.1.1
timeouts:
  backend_dial: 2s
  backend_response_header: 5s
  backend_idle_conn: 30s
  backend_tls_handshake: 4s
  backend_max_idle_conns: 100
  backend_max_idle_conns_per_host: 20
  server_read: 10s
  server_read_header: 4s
  server_write: 20s
  server_idle: 40s
  shutdown: 5s
retry:
  max_attempts: 3
circuit_breaker:
  failure_threshold: 4
  open_timeout: 15s
  half_open_probes: 2
log:
  level: debug
  format: text
redis:
  addr: 127.0.0.1:6379
`

func TestLoadValid(t *testing.T) {
	cfg, err := Load(writeConfig(t, validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen != ":9090" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if len(cfg.Backends) != 2 || cfg.Backends[0].Weight != 2 || cfg.Backends[1].Weight != 1 {
		t.Errorf("backends = %+v", cfg.Backends)
	}
	if cfg.LoadBalancing.Strategy != "least_connections" {
		t.Errorf("strategy = %q", cfg.LoadBalancing.Strategy)
	}
	if cfg.HealthCheck.Interval.Duration != 3*time.Second {
		t.Errorf("interval = %v", cfg.HealthCheck.Interval.Duration)
	}
	if cfg.RateLimit.RequestsPerMinute != 60 || cfg.RateLimit.Burst != 10 {
		t.Errorf("rate limit = %+v", cfg.RateLimit)
	}
	if len(cfg.RateLimit.TrustedProxies) != 2 {
		t.Errorf("trusted proxies = %v", cfg.RateLimit.TrustedProxies)
	}
	if cfg.Timeouts.BackendDial.Duration != 2*time.Second {
		t.Errorf("dial = %v", cfg.Timeouts.BackendDial.Duration)
	}
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("max attempts = %d", cfg.Retry.MaxAttempts)
	}
	if cfg.CircuitBreaker.HalfOpenProbes != 2 {
		t.Errorf("half open probes = %d", cfg.CircuitBreaker.HalfOpenProbes)
	}
	if cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Errorf("redis addr = %q", cfg.Redis.Addr)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, "backends:\n  - url: http://localhost:1\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if cfg.LoadBalancing.Strategy != "round_robin" {
		t.Errorf("strategy = %q", cfg.LoadBalancing.Strategy)
	}
	if cfg.Retry.MaxAttempts != 2 {
		t.Errorf("max attempts = %d", cfg.Retry.MaxAttempts)
	}
	if cfg.CircuitBreaker.FailureThreshold != 5 {
		t.Errorf("failure threshold = %d", cfg.CircuitBreaker.FailureThreshold)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateRejectsInvalid(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"empty listen", func(c *Config) { c.Listen = "" }, "listen"},
		{"no backends", func(c *Config) { c.Backends = nil }, "at least one backend"},
		{"bad scheme", func(c *Config) { c.Backends[0].URL = "ftp://x" }, "scheme"},
		{"missing host", func(c *Config) { c.Backends[0].URL = "http://" }, "host"},
		{"userinfo", func(c *Config) { c.Backends[0].URL = "http://user:pass@host" }, "credentials"},
		{"query", func(c *Config) { c.Backends[0].URL = "http://host?a=1" }, "query"},
		{"fragment", func(c *Config) { c.Backends[0].URL = "http://host#f" }, "fragment"},
		{"weight zero", func(c *Config) { c.Backends[0].Weight = 0 }, "weight"},
		{"weight high", func(c *Config) { c.Backends[0].Weight = 101 }, "weight"},
		{"duplicate", func(c *Config) { c.Backends = append(c.Backends, Backend{URL: c.Backends[0].URL, Weight: 1}) }, "duplicate"},
		{"strategy", func(c *Config) { c.LoadBalancing.Strategy = "random" }, "strategy"},
		{"interval", func(c *Config) { c.HealthCheck.Interval.Duration = 0 }, "interval"},
		{"timeout", func(c *Config) { c.HealthCheck.Timeout.Duration = -1 }, "timeout"},
		{"health path", func(c *Config) { c.HealthCheck.Path = "health" }, "path"},
		{"healthy threshold", func(c *Config) { c.HealthCheck.HealthyThreshold = 0 }, "healthy_threshold"},
		{"unhealthy threshold", func(c *Config) { c.HealthCheck.UnhealthyThreshold = 0 }, "unhealthy_threshold"},
		{"rpm", func(c *Config) { c.RateLimit.RequestsPerMinute = 0 }, "requests_per_minute"},
		{"burst", func(c *Config) { c.RateLimit.Burst = 0 }, "burst"},
		{"cidr", func(c *Config) { c.RateLimit.TrustedProxies = []string{"not-a-cidr"} }, "trusted_proxies"},
		{"dial", func(c *Config) { c.Timeouts.BackendDial.Duration = 0 }, "backend_dial"},
		{"shutdown", func(c *Config) { c.Timeouts.Shutdown.Duration = 0 }, "shutdown"},
		{"idle conns", func(c *Config) { c.Timeouts.BackendMaxIdleConns = 0 }, "backend_max_idle_conns"},
		{"idle per host", func(c *Config) { c.Timeouts.BackendMaxIdlePerHost = 0 }, "backend_max_idle_conns_per_host"},
		{"attempts low", func(c *Config) { c.Retry.MaxAttempts = 0 }, "max_attempts"},
		{"attempts high", func(c *Config) { c.Retry.MaxAttempts = 11 }, "max_attempts"},
		{"failure threshold", func(c *Config) { c.CircuitBreaker.FailureThreshold = 0 }, "failure_threshold"},
		{"half open", func(c *Config) { c.CircuitBreaker.HalfOpenProbes = 0 }, "half_open_probes"},
		{"open timeout", func(c *Config) { c.CircuitBreaker.OpenTimeout.Duration = 0 }, "open_timeout"},
		{"tls partial", func(c *Config) { c.TLS.CertFile = "cert.pem" }, "both be set"},
		{"tls missing file", func(c *Config) { c.TLS.CertFile = "cert.pem"; c.TLS.KeyFile = "key.pem" }, "tls file"},
		{"log level", func(c *Config) { c.Log.Level = "verbose" }, "log.level"},
		{"log format", func(c *Config) { c.Log.Format = "xml" }, "log.format"},
		{"redis addr", func(c *Config) { c.Redis.Addr = "host:6379 extra" }, "redis.addr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Backends = []Backend{{URL: "http://localhost:8081", Weight: 1}}
			tc.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestValidateAcceptsTLSCertKey(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	key := filepath.Join(dir, "key.pem")
	for _, f := range []string{cert, key} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Default()
	cfg.Backends = []Backend{{URL: "http://localhost:8081", Weight: 1}}
	cfg.TLS.CertFile = cert
	cfg.TLS.KeyFile = key
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.TLS.Enabled() {
		t.Error("expected TLS enabled")
	}
}

func TestLoadBadDuration(t *testing.T) {
	path := writeConfig(t, "backends:\n  - url: http://localhost:1\nhealth_check:\n  interval: soon\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected duration parse error")
	}
}
