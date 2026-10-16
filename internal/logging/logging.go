package logging

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"reverse-proxy/internal/clientip"
	"reverse-proxy/internal/httpx"
)

func New(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

func Access(logger *slog.Logger, resolver *clientip.Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := httpx.NewStatusRecorder(w)
			defer func() {
				logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.Status,
					"bytes", rec.Bytes,
					"duration_ms", time.Since(start).Milliseconds(),
					"ip", resolver.FromRequest(r),
					"request_id", httpx.RequestIDFrom(r.Context()),
				)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
