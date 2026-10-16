package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := NewStatusRecorder(rec)
	if sr.Status != http.StatusOK {
		t.Fatalf("initial status = %d", sr.Status)
	}
	sr.WriteHeader(http.StatusTeapot)
	n, err := sr.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("write = %d, %v", n, err)
	}
	if sr.Status != http.StatusTeapot || sr.Bytes != 5 {
		t.Fatalf("status=%d bytes=%d", sr.Status, sr.Bytes)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestRequestIDGenerates(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if len(seen) != 16 {
		t.Fatalf("generated id = %q", seen)
	}
	if got := rec.Header().Get("X-Request-ID"); got != seen {
		t.Fatalf("header = %q, want %q", got, seen)
	}
}

func TestRequestIDPropagates(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "client-supplied-1")
	h.ServeHTTP(rec, req)
	if seen != "client-supplied-1" {
		t.Fatalf("seen = %q", seen)
	}
}

func TestRequestIDRejectsInvalid(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	for _, id := range []string{"bad id!", strings.Repeat("x", 65), ""} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", id)
		h.ServeHTTP(rec, req)
		if seen == id {
			t.Fatalf("accepted invalid id %q", id)
		}
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "req-1")
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusBadGateway, "bad_gateway", "upstream request failed")
	}))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", rec.Header().Get("Cache-Control"))
	}
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "bad_gateway" || body.Error.Message != "upstream request failed" || body.RequestID != "req-1" {
		t.Fatalf("body = %+v", body)
	}
}
