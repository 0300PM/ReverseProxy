package balancer

import (
	"fmt"
	"net/url"
	"sync/atomic"

	"reverse-proxy/internal/breaker"
)

type Backend struct {
	url       *url.URL
	weight    int
	healthy   atomic.Bool
	adminDown atomic.Bool
	inflight  atomic.Int64
	brk       *breaker.Breaker
}

func New(rawURL string, weight int, brk *breaker.Breaker) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse backend URL %q: %w", rawURL, err)
	}
	b := &Backend{url: u, weight: weight, brk: brk}
	b.healthy.Store(true)
	return b, nil
}

func (b *Backend) URL() *url.URL {
	return b.url
}

func (b *Backend) String() string {
	return b.url.String()
}

func (b *Backend) Weight() int {
	return b.weight
}

func (b *Backend) Healthy() bool {
	return b.healthy.Load()
}

func (b *Backend) SetHealthy(healthy bool) bool {
	return b.healthy.Swap(healthy) != healthy
}

func (b *Backend) AdminDown() bool {
	return b.adminDown.Load()
}

func (b *Backend) SetAdminDown(down bool) bool {
	return b.adminDown.Swap(down) != down
}

func (b *Backend) Available() bool {
	return b.healthy.Load() && !b.adminDown.Load()
}

func (b *Backend) Inflight() int64 {
	return b.inflight.Load()
}

func (b *Backend) Acquire() {
	b.inflight.Add(1)
}

func (b *Backend) Release() {
	b.inflight.Add(-1)
}

func (b *Backend) Breaker() *breaker.Breaker {
	return b.brk
}
