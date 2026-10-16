package balancer

import (
	"sync"
	"testing"
	"time"

	"reverse-proxy/internal/breaker"
)

func newTestBackend(t *testing.T, rawURL string, weight int) *Backend {
	t.Helper()
	b, err := New(rawURL, weight, breaker.New(5, time.Second, 1))
	if err != nil {
		t.Fatalf("New(%q): %v", rawURL, err)
	}
	return b
}

func TestNewInvalidURL(t *testing.T) {
	if _, err := New("://bad", 1, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestBackendLifecycle(t *testing.T) {
	b := newTestBackend(t, "http://a:1", 3)
	if !b.Available() || !b.Healthy() || b.AdminDown() {
		t.Fatal("expected initially available")
	}
	if b.Weight() != 3 || b.String() != "http://a:1" || b.URL().Host != "a:1" {
		t.Fatal("unexpected backend metadata")
	}
	if !b.SetHealthy(false) || b.SetHealthy(false) {
		t.Fatal("SetHealthy should report change once")
	}
	if b.Available() {
		t.Fatal("expected unavailable when unhealthy")
	}
	b.SetHealthy(true)
	if !b.SetAdminDown(true) || b.SetAdminDown(true) {
		t.Fatal("SetAdminDown should report change once")
	}
	if b.Available() {
		t.Fatal("expected unavailable when admin down")
	}
	b.SetAdminDown(false)
	b.Acquire()
	b.Acquire()
	if b.Inflight() != 2 {
		t.Fatalf("inflight = %d", b.Inflight())
	}
	b.Release()
	if b.Inflight() != 1 {
		t.Fatalf("inflight = %d", b.Inflight())
	}
	if b.Breaker() == nil {
		t.Fatal("expected breaker")
	}
}

func TestRoundRobinDistributes(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 1)
	b := newTestBackend(t, "http://b:1", 1)
	rr := NewRoundRobin([]*Backend{a, b})
	counts := map[*Backend]int{}
	for i := 0; i < 10; i++ {
		counts[rr.Next()]++
	}
	if counts[a] != 5 || counts[b] != 5 {
		t.Fatalf("counts = %v", counts)
	}
	if rr.Len() != 2 {
		t.Fatalf("len = %d", rr.Len())
	}
}

func TestRoundRobinSkipsUnavailable(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 1)
	b := newTestBackend(t, "http://b:1", 1)
	rr := NewRoundRobin([]*Backend{a, b})
	a.SetHealthy(false)
	for i := 0; i < 5; i++ {
		if got := rr.Next(); got != b {
			t.Fatalf("got %v, want b", got)
		}
	}
	a.SetAdminDown(true)
	b.SetHealthy(false)
	if rr.Next() != nil {
		t.Fatal("expected nil when nothing available")
	}
}

func TestRoundRobinEmpty(t *testing.T) {
	if NewRoundRobin(nil).Next() != nil {
		t.Fatal("expected nil")
	}
}

func TestWeightedRoundRobinDistribution(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 5)
	b := newTestBackend(t, "http://b:1", 1)
	w := NewWeightedRoundRobin([]*Backend{a, b})
	counts := map[*Backend]int{}
	for i := 0; i < 600; i++ {
		counts[w.Next()]++
	}
	if counts[a] != 500 || counts[b] != 100 {
		t.Fatalf("counts = %v", counts)
	}
	if w.Len() != 6 {
		t.Fatalf("len = %d", w.Len())
	}
}

func TestWeightedRoundRobinSkipsUnavailable(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 3)
	b := newTestBackend(t, "http://b:1", 1)
	w := NewWeightedRoundRobin([]*Backend{a, b})
	b.SetHealthy(false)
	for i := 0; i < 10; i++ {
		if got := w.Next(); got != a {
			t.Fatalf("got %v, want a", got)
		}
	}
}

func TestLeastConnectionsPrefersIdle(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 1)
	b := newTestBackend(t, "http://b:1", 1)
	lc := NewLeastConnections([]*Backend{a, b})
	a.Acquire()
	a.Acquire()
	b.Acquire()
	for i := 0; i < 5; i++ {
		if got := lc.Next(); got != b {
			t.Fatalf("got %v, want b", got)
		}
	}
	b.Acquire()
	b.Acquire()
	b.Acquire()
	for i := 0; i < 5; i++ {
		if got := lc.Next(); got != a {
			t.Fatalf("got %v, want a", got)
		}
	}
}

func TestLeastConnectionsSkipsUnavailable(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 1)
	b := newTestBackend(t, "http://b:1", 1)
	lc := NewLeastConnections([]*Backend{a, b})
	a.SetAdminDown(true)
	for i := 0; i < 5; i++ {
		if got := lc.Next(); got != b {
			t.Fatalf("got %v, want b", got)
		}
	}
	b.SetHealthy(false)
	if lc.Next() != nil {
		t.Fatal("expected nil")
	}
}

func TestNewPicker(t *testing.T) {
	backends := []*Backend{newTestBackend(t, "http://a:1", 1)}
	for _, strategy := range []string{"round_robin", "weighted_round_robin", "least_connections"} {
		p, err := NewPicker(strategy, backends)
		if err != nil || p == nil {
			t.Fatalf("strategy %s: %v", strategy, err)
		}
	}
	if _, err := NewPicker("random", backends); err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestConcurrentNext(t *testing.T) {
	a := newTestBackend(t, "http://a:1", 2)
	b := newTestBackend(t, "http://b:1", 1)
	pickers := []Picker{
		NewRoundRobin([]*Backend{a, b}),
		NewWeightedRoundRobin([]*Backend{a, b}),
		NewLeastConnections([]*Backend{a, b}),
	}
	var wg sync.WaitGroup
	for _, p := range pickers {
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(p Picker) {
				defer wg.Done()
				for j := 0; j < 200; j++ {
					if p.Next() == nil {
						t.Error("unexpected nil")
						return
					}
				}
			}(p)
		}
	}
	wg.Wait()
}
