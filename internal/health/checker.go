package health

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/metrics"
)

type Checker struct {
	backends           []*balancer.Backend
	client             *http.Client
	path               string
	interval           time.Duration
	healthyThreshold   int
	unhealthyThreshold int
	streaks            []int
	logger             *slog.Logger
}

func New(backends []*balancer.Backend, interval, timeout time.Duration, path string, healthyThreshold, unhealthyThreshold int, logger *slog.Logger) *Checker {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
		MaxIdleConns:        len(backends),
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: timeout,
	}
	return &Checker{
		backends: backends,
		client: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		path:               path,
		interval:           interval,
		healthyThreshold:   healthyThreshold,
		unhealthyThreshold: unhealthyThreshold,
		streaks:            make([]int, len(backends)),
		logger:             logger,
	}
}

func (c *Checker) Start(ctx context.Context) {
	go c.run(ctx)
}

func (c *Checker) Close() {
	c.client.CloseIdleConnections()
}

func (c *Checker) run(ctx context.Context) {
	c.checkAll(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAll(ctx)
		}
	}
}

func (c *Checker) checkAll(ctx context.Context) {
	var wg sync.WaitGroup
	for i, b := range c.backends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.checkOne(ctx, i, b)
		}()
	}
	wg.Wait()
}

func (c *Checker) checkOne(ctx context.Context, i int, b *balancer.Backend) {
	alive := c.probe(ctx, b)
	result := "failure"
	if alive {
		result = "success"
	}
	metrics.HealthChecksTotal.WithLabelValues(b.String(), result).Inc()
	if alive {
		if c.streaks[i] < 0 {
			c.streaks[i] = 0
		}
		c.streaks[i]++
		if c.streaks[i] >= c.healthyThreshold && b.SetHealthy(true) {
			metrics.SetBackendHealth(b.String(), true)
			c.logger.Info("backend healthy", "backend", b.String())
		}
		return
	}
	if c.streaks[i] > 0 {
		c.streaks[i] = 0
	}
	c.streaks[i]--
	if -c.streaks[i] >= c.unhealthyThreshold && b.SetHealthy(false) {
		metrics.SetBackendHealth(b.String(), false)
		c.logger.Warn("backend unhealthy", "backend", b.String())
	}
}

func (c *Checker) probe(ctx context.Context, b *balancer.Backend) bool {
	reqCtx, cancel := context.WithTimeout(ctx, c.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, b.URL().String()+c.path, nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}
