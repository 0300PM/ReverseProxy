package balancer

import (
	"testing"

	"reverse-proxy/internal/breaker"
)

func benchBackends(n int) []*Backend {
	backends := make([]*Backend, n)
	for i := range backends {
		b, err := New("http://backend", 1+(i%5), breaker.New(5, 1, 1))
		if err != nil {
			panic(err)
		}
		backends[i] = b
	}
	return backends
}

func BenchmarkRoundRobinNext(b *testing.B) {
	rr := NewRoundRobin(benchBackends(8))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if rr.Next() == nil {
				b.Fatal("nil backend")
			}
		}
	})
}

func BenchmarkWeightedRoundRobinNext(b *testing.B) {
	w := NewWeightedRoundRobin(benchBackends(8))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if w.Next() == nil {
				b.Fatal("nil backend")
			}
		}
	})
}

func BenchmarkLeastConnectionsNext(b *testing.B) {
	lc := NewLeastConnections(benchBackends(8))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if lc.Next() == nil {
				b.Fatal("nil backend")
			}
		}
	})
}

func BenchmarkBackendState(b *testing.B) {
	back := benchBackends(1)[0]
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			back.SetHealthy(true)
			_ = back.Available()
		}
	})
}
