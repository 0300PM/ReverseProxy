package health

import (
	"log"
	"net/http"
	"time"

	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/metrics"
)

type Checker struct {
	backends []*balancer.Backend
	interval time.Duration
	path     string
}

func NewChecker(backends []*balancer.Backend, interval time.Duration, healthPath string) *Checker {
	if healthPath == "" {
		healthPath = "/health"
	}
	return &Checker{
		backends: backends,
		interval: interval,
		path:     healthPath,
	}
}

func (c *Checker) Start() {
	ticker := time.NewTicker(c.interval)
	go func() {
		for range ticker.C {
			for _, backend := range c.backends {
				wasAlive := backend.IsAlive()
				alive := c.check(backend)
				backend.SetAlive(alive)

				// Update Prometheus metric
				metrics.SetBackendHealth(backend.URL.String(), alive)

				// Log only on state change (no spam every 5s)
				if alive != wasAlive {
					status := "UP"
					if !alive {
						status = "DOWN"
					}
					log.Printf("Health check state change: %s -> %s", backend.URL.String(), status)
				}
			}
		}
	}()
}

func (c *Checker) check(backend *balancer.Backend) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Use HEAD instead of GET (lighter)
	req, err := http.NewRequest(http.MethodHead, backend.URL.String()+c.path, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
