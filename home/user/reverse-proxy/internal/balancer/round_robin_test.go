package balancer

import (
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestRoundRobin_Next(t *testing.T) {
	u1, _ := url.Parse("http://backend1")
	u2, _ := url.Parse("http://backend2")

	b1 := &Backend{URL: u1, Alive: true, Proxy: httputil.NewSingleHostReverseProxy(u1)}
	b2 := &Backend{URL: u2, Alive: true, Proxy: httputil.NewSingleHostReverseProxy(u2)}

	rr := NewRoundRobin([]*Backend{b1, b2})

	seen := make(map[*Backend]int)
	for i := 0; i < 10; i++ {
		b := rr.Next()
		if b != nil {
			seen[b]++
		}
	}

	if len(seen) != 2 {
		t.Errorf("expected 2 different backends, got %d", len(seen))
	}

	// Skip dead backend
	b1.SetAlive(false)

	for i := 0; i < 5; i++ {
		b := rr.Next()
		if b == nil || b == b1 {
			t.Error("should skip dead backend")
		}
	}
}

func TestRoundRobin_Empty(t *testing.T) {
	rr := NewRoundRobin([]*Backend{})
	if rr.Next() != nil {
		t.Error("expected nil for empty backends")
	}
}
