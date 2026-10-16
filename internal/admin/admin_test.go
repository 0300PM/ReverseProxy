package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/breaker"
)

const testToken = "secret-token"

func newTestAdmin(t *testing.T) (*Admin, []*balancer.Backend) {
	t.Helper()
	a, err := balancer.New("http://backend-a:8080", 1, breaker.New(5, time.Second, 1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := balancer.New("http://backend-b:8080", 1, breaker.New(5, time.Second, 1))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New([]*balancer.Backend{a, b}, testToken, logger), []*balancer.Backend{a, b}
}

func do(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthRequired(t *testing.T) {
	admin, _ := newTestAdmin(t)
	cases := []struct {
		name  string
		token string
		want  int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic " + testToken, http.StatusUnauthorized},
		{"wrong token", "wrong", http.StatusUnauthorized},
		{"good", testToken, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, admin, http.MethodGet, "/admin/backends", tc.token)
			if rec.Code != tc.want {
				t.Fatalf("code = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestListBackends(t *testing.T) {
	admin, backends := newTestAdmin(t)
	backends[1].SetAdminDown(true)
	rec := do(t, admin, http.MethodGet, "/admin/backends", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache control = %q", cc)
	}
	var infos []BackendInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &infos); err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("infos = %+v", infos)
	}
	if infos[0].URL != "http://backend-a:8080" || infos[0].AdminDown {
		t.Fatalf("info[0] = %+v", infos[0])
	}
	if !infos[1].AdminDown {
		t.Fatalf("info[1] = %+v", infos[1])
	}
}

func TestBackendUpAndDown(t *testing.T) {
	admin, backends := newTestAdmin(t)
	rec := do(t, admin, http.MethodPost, "/admin/backends/0/down", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !backends[0].AdminDown() {
		t.Fatal("expected admin down")
	}
	var status statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Index != 0 || status.Status != "down" {
		t.Fatalf("status = %+v", status)
	}
	rec = do(t, admin, http.MethodPost, "/admin/backends/0/up", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if backends[0].AdminDown() {
		t.Fatal("expected admin up")
	}
}

func TestInvalidIndex(t *testing.T) {
	admin, _ := newTestAdmin(t)
	for _, path := range []string{"/admin/backends/9/down", "/admin/backends/x/down", "/admin/backends/-1/down"} {
		rec := do(t, admin, http.MethodPost, path, testToken)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path %s: code = %d", path, rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("path %s: body %q: %v", path, rec.Body.String(), err)
		}
		if body["error"] == nil {
			t.Fatalf("path %s: body = %v", path, body)
		}
	}
}

func TestUnknownRoute(t *testing.T) {
	admin, _ := newTestAdmin(t)
	rec := do(t, admin, http.MethodGet, "/admin/unknown", testToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	admin, _ := newTestAdmin(t)
	rec := do(t, admin, http.MethodDelete, "/admin/backends", testToken)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code = %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Fatalf("allow = %q", allow)
	}
}
