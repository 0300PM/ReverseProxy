package health

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/breaker"
)

func newChecker(t *testing.T, backends []*balancer.Backend) *Checker {
	t.Helper()
	return New(backends, time.Minute, time.Second, "/health", 2, 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newBackend(t *testing.T, rawURL string) *balancer.Backend {
	t.Helper()
	b, err := balancer.New(rawURL, 1, breaker.New(5, time.Second, 1))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProbeDetectsDown(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	b := newBackend(t, srv.URL)
	c := newChecker(t, []*balancer.Backend{b})

	c.checkOne(context.Background(), 0, b)
	if !b.Healthy() {
		t.Fatal("expected healthy after first success")
	}

	healthy.Store(false)
	c.checkOne(context.Background(), 0, b)
	if !b.Healthy() {
		t.Fatal("expected still healthy below threshold")
	}
	c.checkOne(context.Background(), 0, b)
	if b.Healthy() {
		t.Fatal("expected unhealthy after threshold")
	}

	healthy.Store(true)
	c.checkOne(context.Background(), 0, b)
	if b.Healthy() {
		t.Fatal("expected still unhealthy below threshold")
	}
	c.checkOne(context.Background(), 0, b)
	if !b.Healthy() {
		t.Fatal("expected healthy after recovery threshold")
	}
	c.Close()
}

func TestProbeUnreachable(t *testing.T) {
	b := newBackend(t, "http://127.0.0.1:1")
	c := newChecker(t, []*balancer.Backend{b})
	c.checkOne(context.Background(), 0, b)
	c.checkOne(context.Background(), 0, b)
	if b.Healthy() {
		t.Fatal("expected unhealthy")
	}
	c.Close()
}

func TestProbeRejectsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusMovedPermanently)
	}))
	defer srv.Close()
	b := newBackend(t, srv.URL)
	c := newChecker(t, []*balancer.Backend{b})
	if c.probe(context.Background(), b) {
		t.Fatal("expected redirect to count as failure")
	}
	c.Close()
}

func TestRunStopsOnContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	b := newBackend(t, srv.URL)
	c := New([]*balancer.Backend{b}, time.Millisecond, time.Second, "/health", 1, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	deadline := time.After(time.Second)
	for {
		if b.Healthy() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("checker did not run")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	c.Close()
}

func TestConcurrentChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	backends := []*balancer.Backend{
		newBackend(t, srv.URL),
		newBackend(t, "http://127.0.0.1:1"),
		newBackend(t, srv.URL),
	}
	c := newChecker(t, backends)
	for i := 0; i < 5; i++ {
		c.checkAll(context.Background())
	}
	if backends[0].Healthy() != true || backends[1].Healthy() != false || backends[2].Healthy() != true {
		t.Fatal("unexpected health states after concurrent checks")
	}
	c.Close()
}
