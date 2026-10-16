package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"reverse-proxy/internal/config"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func testConfig(addr string) *config.Config {
	cfg := config.Default()
	cfg.Listen = addr
	cfg.Timeouts.Shutdown = config.Duration{Duration: 2 * time.Second}
	return cfg
}

func TestRunGracefulShutdown(t *testing.T) {
	addr := freeAddr(t)
	cfg := testConfig(addr)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, handler, logger) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestRunSurfacesListenError(t *testing.T) {
	addr := freeAddr(t)
	blocker, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()

	cfg := testConfig(addr)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Run(ctx, cfg, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), logger); err == nil {
		t.Fatal("expected listen error")
	}
}
