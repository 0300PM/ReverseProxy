package ratelimit

import (
	"net/http"
	"sync"

	"reverse-proxy/internal/metrics"

	"golang.org/x/time/rate"
)

type Client struct {
	Limiter *rate.Limiter
}
type Limiter struct {
	clients map[string]*Client
	mu      sync.Mutex
	rps     rate.Limit
	burst   int
}

func NewLimiter(requestsPerMinute int) *Limiter {
	rps := rate.Limit(float64(requestsPerMinute) / 60.0)
	return &Limiter{clients: make(map[string]*Client), rps: rps, burst: requestsPerMinute}
}

func (l *Limiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock(); defer l.mu.Unlock()
	if c, ok := l.clients[ip]; ok { return c.Limiter }
	c := &Client{Limiter: rate.NewLimiter(l.rps, l.burst)}
	l.clients[ip] = c
	return c.Limiter
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.getLimiter(r.RemoteAddr).Allow() {
			metrics.RecordRateLimitReject()
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
