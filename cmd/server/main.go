package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"reverse-proxy/internal/app"
	"reverse-proxy/internal/config"
	"reverse-proxy/internal/logging"
	"reverse-proxy/internal/server"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger := logging.New(cfg.Log.Level, cfg.Log.Format)

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("initialisation failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application.Checker.Start(ctx)
	logger.Info("listening",
		"addr", cfg.Listen,
		"strategy", cfg.LoadBalancing.Strategy,
		"backends", len(cfg.Backends),
		"tls", cfg.TLS.Enabled(),
	)
	if err := server.Run(ctx, cfg, application.Handler, logger); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
