package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reverse-proxy/internal/app"
	"reverse-proxy/internal/config"
)

type backendServer struct {
	*httptest.Server
	name    string
	healthy atomic.Bool
}

func newBackend(t *testing.T, name string) *backendServer {
	t.Helper()
	b := &backendServer{name: name}
	b.healthy.Store(true)
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if !b.healthy.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("ok"))
		default:
			w.Header().Set("X-Backend", name)
			_, _ = w.Write([]byte(name))
		}
	}))
	return b
}

func newTestConfig(t *testing.T, urls ...string) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.HealthCheck.Interval = config.Duration{Duration: 10 * time.Millisecond}
	cfg.HealthCheck.Timeout = config.Duration{Duration: 100 * time.Millisecond}
	cfg.HealthCheck.HealthyThreshold = 1
	cfg.HealthCheck.UnhealthyThreshold = 1
	cfg.Retry.MaxAttempts = 2
	cfg.Backends = make([]config.Backend, len(urls))
	for i, u := range urls {
		cfg.Backends[i] = config.Backend{URL: u, Weight: 1}
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newApp(t *testing.T, cfg *config.Config) (*app.App, *httptest.Server) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(func() {
		srv.Close()
		_ = a.Close()
	})
	return a, srv
}

func get(t *testing.T, url string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestProxiedRequest(t *testing.T) {
	backend := newBackend(t, "A")
	defer backend.Close()
	_, srv := newApp(t, newTestConfig(t, backend.URL+"/upstream"))

	resp := get(t, srv.URL+"/api/items", map[string]string{"X-Request-ID": "integration-1"})
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "A" {
		t.Fatalf("status = %d body = %q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Request-ID") != "integration-1" {
		t.Fatalf("request id = %q", resp.Header.Get("X-Request-ID"))
	}
	if resp.Header.Get("X-Backend") != "A" {
		t.Fatalf("backend header = %q", resp.Header.Get("X-Backend"))
	}
}

func TestFailoverRetry(t *testing.T) {
	good := newBackend(t, "A")
	defer good.Close()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	cfg := newTestConfig(t, good.URL, deadURL)
	_, srv := newApp(t, cfg)

	for i := 0; i < 4; i++ {
		resp := get(t, srv.URL, nil)
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "A" {
			t.Fatalf("request %d: status = %d body = %q", i, resp.StatusCode, body)
		}
	}
}

func TestBadGatewayErrorEnvelope(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	cfg := newTestConfig(t, deadURL)
	cfg.Retry.MaxAttempts = 1
	_, srv := newApp(t, cfg)

	resp := get(t, srv.URL, map[string]string{"X-Request-ID": "err-1"})
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "bad_gateway" || body.RequestID != "err-1" {
		t.Fatalf("body = %+v", body)
	}
}

func TestRateLimit(t *testing.T) {
	backend := newBackend(t, "A")
	defer backend.Close()
	cfg := newTestConfig(t, backend.URL)
	cfg.RateLimit.RequestsPerMinute = 60
	cfg.RateLimit.Burst = 2
	_, srv := newApp(t, cfg)

	codes := make([]int, 0, 4)
	for i := 0; i < 4; i++ {
		resp := get(t, srv.URL, nil)
		codes = append(codes, resp.StatusCode)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Fatalf("codes = %v", codes)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v", codes)
	}
}

func TestAdminAuthAndTrafficControl(t *testing.T) {
	aBackend := newBackend(t, "A")
	defer aBackend.Close()
	bBackend := newBackend(t, "B")
	defer bBackend.Close()

	cfg := newTestConfig(t, aBackend.URL, bBackend.URL)
	t.Setenv("ADMIN_TOKEN", "integration-token")
	_, srv := newApp(t, cfg)

	unauth := get(t, srv.URL+"/admin/backends", nil)
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", unauth.StatusCode)
	}

	auth := map[string]string{"Authorization": "Bearer integration-token"}
	resp := get(t, srv.URL+"/admin/backends", auth)
	var infos []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("infos = %v", infos)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/backends/0/down", nil)
	for k, v := range auth {
		req.Header.Set(k, v)
	}
	downResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer downResp.Body.Close()
	if downResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", downResp.StatusCode)
	}

	for i := 0; i < 4; i++ {
		r := get(t, srv.URL, nil)
		body, _ := io.ReadAll(r.Body)
		if string(body) != "B" {
			t.Fatalf("request %d: body = %q", i, body)
		}
	}

	reqUp, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/backends/0/up", nil)
	for k, v := range auth {
		reqUp.Header.Set(k, v)
	}
	upResp, err := http.DefaultClient.Do(reqUp)
	if err != nil {
		t.Fatal(err)
	}
	defer upResp.Body.Close()
}

func TestAdminDisabledWithoutToken(t *testing.T) {
	backend := newBackend(t, "A")
	defer backend.Close()
	t.Setenv("ADMIN_TOKEN", "")
	_, srv := newApp(t, newTestConfig(t, backend.URL))

	resp := get(t, srv.URL+"/admin/backends", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHealthCheckRecovery(t *testing.T) {
	backend := newBackend(t, "A")
	defer backend.Close()
	cfg := newTestConfig(t, backend.URL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Checker.Start(ctx)

	waitFor(t, func() bool {
		resp := get(t, srv.URL, nil)
		return resp.StatusCode == http.StatusOK
	}, "initial healthy traffic")

	backend.healthy.Store(false)
	waitFor(t, func() bool {
		resp := get(t, srv.URL, nil)
		return resp.StatusCode == http.StatusServiceUnavailable
	}, "traffic stops after health check marks down")

	backend.healthy.Store(true)
	waitFor(t, func() bool {
		resp := get(t, srv.URL, nil)
		return resp.StatusCode == http.StatusOK
	}, "traffic resumes after recovery")
}

func TestHealthzAndMetrics(t *testing.T) {
	backend := newBackend(t, "A")
	defer backend.Close()
	_, srv := newApp(t, newTestConfig(t, backend.URL))

	resp := get(t, srv.URL+"/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}

	resp = get(t, srv.URL+"/metrics", nil)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "proxy_requests_total") {
		t.Fatalf("metrics body missing proxy_requests_total")
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("timed out waiting for %s", what))
}
