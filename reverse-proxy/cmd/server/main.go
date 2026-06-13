package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"reverse-proxy/internal/admin"
	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/health"
	"reverse-proxy/internal/logger"
	"reverse-proxy/internal/proxy"
	"reverse-proxy/internal/ratelimit"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port                int      `yaml:"port"`
	Backends            []string `yaml:"backends"`
	HealthCheckInterval string   `yaml:"health_check_interval"`
	HealthCheckPath     string   `yaml:"health_check_path"`
	RateLimit           struct {
		RequestsPerMinute int `yaml:"requests_per_minute"`
	} `yaml:"rate_limit"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil { return nil, err }
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil { return nil, err }
	return &cfg, nil
}

func main() {
	cfg, err := loadConfig("config/config.yaml")
	if err != nil { log.Fatalf("failed to load config: %v", err) }

	var backends []*balancer.Backend
	for _, u := range cfg.Backends {
		b, err := balancer.NewBackend(u)
		if err != nil { log.Fatalf("backend %s: %v", u, err) }
		backends = append(backends, b)
	}
	if len(backends) == 0 { log.Fatal("no backends") }

	rr := balancer.NewRoundRobin(backends)

	interval := 5 * time.Second
	if d, err := time.ParseDuration(cfg.HealthCheckInterval); err == nil { interval = d }
	checker := health.NewChecker(backends, interval, cfg.HealthCheckPath)
	checker.Start()

	rateLimiter := ratelimit.NewLimiter(cfg.RateLimit.RequestsPerMinute)
	reverseProxy := proxy.NewReverseProxy(rr)

	handler := logger.Logger(rateLimiter.Middleware(reverseProxy))
	adminAPI := admin.NewAdmin(backends)

	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/admin/", http.StripPrefix("/admin", adminAPI))

	port := cfg.Port
	if port == 0 { port = 8080 }
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting on %s | Admin: %s/admin/backends | Metrics: %s/metrics", addr, addr, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
