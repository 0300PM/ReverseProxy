package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"reverse-proxy/internal/config"
)

func Run(ctx context.Context, cfg *config.Config, handler http.Handler, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadTimeout:       cfg.Timeouts.ServerRead.Duration,
		ReadHeaderTimeout: cfg.Timeouts.ServerReadHeader.Duration,
		WriteTimeout:      cfg.Timeouts.ServerWrite.Duration,
		IdleTimeout:       cfg.Timeouts.ServerIdle.Duration,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	errCh := make(chan error, 1)
	go func() {
		if cfg.TLS.Enabled() {
			errCh <- srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		return normalise(err)
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Shutdown.Duration)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		return normalise(<-errCh)
	}
}

func normalise(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
