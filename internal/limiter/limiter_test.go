package limiter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"reverse-proxy/internal/clientip"
)

func TestMemoryAllowBurstThenReject(t *testing.T) {
	m := NewMemory(60, 3, time.Minute)
	defer m.Close()
	for i := 0; i < 3; i++ {
		allowed, err := m.Allow("1.2.3.4")
		if err != nil || !allowed {
			t.Fatalf("request %d: allowed=%v err=%v", i, allowed, err)
		}
	}
	allowed, err := m.Allow("1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("expected rejection after burst")
	}
}

func TestMemoryKeysAreIndependent(t *testing.T) {
	m := NewMemory(60, 1, time.Minute)
	defer m.Close()
	if allowed, _ := m.Allow("a"); !allowed {
		t.Fatal("expected allow")
	}
	if allowed, _ := m.Allow("a"); allowed {
		t.Fatal("expected reject")
	}
	if allowed, _ := m.Allow("b"); !allowed {
		t.Fatal("expected allow for other key")
	}
}

func TestMemoryEvictsIdleClients(t *testing.T) {
	m := NewMemory(60, 1, 20*time.Millisecond)
	defer m.Close()
	for _, k := range []string{"a", "b", "c"} {
		if allowed, _ := m.Allow(k); !allowed {
			t.Fatal("expected allow")
		}
	}
	m.mu.Lock()
	size := len(m.clients)
	m.mu.Unlock()
	if size != 3 {
		t.Fatalf("size = %d", size)
	}
	time.Sleep(80 * time.Millisecond)
	m.mu.Lock()
	size = len(m.clients)
	m.mu.Unlock()
	if size != 0 {
		t.Fatalf("size after eviction = %d", size)
	}
}

func TestMemoryRefills(t *testing.T) {
	m := NewMemory(1200, 1, time.Minute)
	defer m.Close()
	if allowed, _ := m.Allow("a"); !allowed {
		t.Fatal("expected allow")
	}
	if allowed, _ := m.Allow("a"); allowed {
		t.Fatal("expected reject")
	}
	time.Sleep(60 * time.Millisecond)
	if allowed, _ := m.Allow("a"); !allowed {
		t.Fatal("expected allow after refill")
	}
}

type stubEvaler struct {
	result  any
	err     error
	calls   atomic.Int64
	lastKey string
}

func (s *stubEvaler) Eval(_ context.Context, _ string, keys []string, _ ...any) (any, error) {
	s.calls.Add(1)
	s.lastKey = keys[0]
	return s.result, s.err
}

func (s *stubEvaler) Close() error { return nil }

func TestRedisAllow(t *testing.T) {
	stub := &stubEvaler{result: int64(1)}
	r := newRedis(stub, 60, 5)
	allowed, err := r.Allow("ip")
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if stub.lastKey != "ratelimit:ip" {
		t.Fatalf("key = %q", stub.lastKey)
	}
	stub.result = int64(0)
	allowed, err = r.Allow("ip")
	if err != nil || allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRedisAllowError(t *testing.T) {
	stub := &stubEvaler{err: errors.New("connection refused")}
	r := newRedis(stub, 60, 5)
	allowed, err := r.Allow("ip")
	if allowed || err == nil {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
}

type stubLimiter struct {
	allowed bool
	err     error
}

func (s *stubLimiter) Allow(string) (bool, error) { return s.allowed, s.err }
func (s *stubLimiter) Close() error               { return nil }

func middlewareWith(l Limiter) http.Handler {
	resolver, err := clientip.New(nil)
	if err != nil {
		panic(err)
	}
	return NewMiddleware(l, resolver).Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestMiddlewareRejects(t *testing.T) {
	h := middlewareWith(&stubLimiter{allowed: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestMiddlewareFailsOpenOnError(t *testing.T) {
	h := middlewareWith(&stubLimiter{err: errors.New("redis down")})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestMiddlewareAllows(t *testing.T) {
	h := middlewareWith(&stubLimiter{allowed: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}
