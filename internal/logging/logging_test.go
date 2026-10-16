package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reverse-proxy/internal/clientip"
	"reverse-proxy/internal/httpx"
)

func TestNewFormats(t *testing.T) {
	for _, format := range []string{"json", "text", "default"} {
		if l := New("info", format); l == nil {
			t.Fatalf("format %q: nil logger", format)
		}
	}
	for _, level := range []string{"debug", "info", "warn", "error", "other"} {
		if l := New(level, "json"); l == nil {
			t.Fatalf("level %q: nil logger", level)
		}
	}
}

func TestAccessLogsRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	resolver, err := clientip.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := httpx.RequestID(Access(logger, resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("abc"))
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/things", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Request-ID", "req-42")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d", rec.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log line %q: %v", buf.String(), err)
	}
	if entry["msg"] != "request" {
		t.Fatalf("msg = %v", entry["msg"])
	}
	if entry["method"] != "POST" || entry["path"] != "/things" {
		t.Fatalf("entry = %v", entry)
	}
	if entry["status"].(float64) != http.StatusCreated {
		t.Fatalf("status = %v", entry["status"])
	}
	if entry["bytes"].(float64) != 3 {
		t.Fatalf("bytes = %v", entry["bytes"])
	}
	if entry["ip"] != "203.0.113.9" {
		t.Fatalf("ip = %v", entry["ip"])
	}
	if entry["request_id"] != "req-42" {
		t.Fatalf("request_id = %v", entry["request_id"])
	}
	if !strings.Contains(buf.String(), "duration_ms") {
		t.Fatalf("missing duration: %s", buf.String())
	}
}
