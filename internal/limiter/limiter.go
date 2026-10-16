package limiter

import (
	"net/http"

	"reverse-proxy/internal/clientip"
	"reverse-proxy/internal/httpx"
	"reverse-proxy/internal/metrics"
)

type Limiter interface {
	Allow(key string) (bool, error)
	Close() error
}

type Middleware struct {
	limiter  Limiter
	resolver *clientip.Resolver
}

func NewMiddleware(l Limiter, r *clientip.Resolver) *Middleware {
	return &Middleware{limiter: l, resolver: r}
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, err := m.limiter.Allow(m.resolver.FromRequest(r))
		if err != nil {
			metrics.RateLimitErrors.Inc()
			allowed = true
		}
		if !allowed {
			metrics.RateLimitRejected.Inc()
			httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}
