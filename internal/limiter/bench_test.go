package limiter

import (
	"strconv"
	"testing"
	"time"
)

func BenchmarkMemoryAllowSingleKey(b *testing.B) {
	m := NewMemory(60_000_000, 1<<30, time.Minute)
	defer m.Close()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if allowed, _ := m.Allow("client"); !allowed {
				b.Fatal("rejected")
			}
		}
	})
}

func BenchmarkMemoryAllowManyKeys(b *testing.B) {
	m := NewMemory(60_000_000, 1<<30, time.Minute)
	defer m.Close()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if allowed, _ := m.Allow(strconv.Itoa(i % 1024)); !allowed {
				b.Fatal("rejected")
			}
			i++
		}
	})
}
