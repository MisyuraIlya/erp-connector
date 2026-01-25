package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"erp-connector/internal/api"
	"erp-connector/internal/config"
	"erp-connector/internal/logger"
)

func main() {
	bootstrapLog := logger.NewStderr()

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			bootstrapLog.Error("config not found; run erp-connector UI to create it", nil)
			os.Exit(1)
		}
		bootstrapLog.Error("failed to load config", err)
		os.Exit(1)
	}

	logSvc, err := logger.New(cfg)
	if err != nil {
		bootstrapLog.Error("logger init failed; using stderr", err)
		logSvc = bootstrapLog
	}
	defer logSvc.Close()

	srv, err := api.NewServer(cfg)
	if err != nil {
		logSvc.Error("config validation error", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	logSvc.Info(fmt.Sprintf("erp-connectord listening on %s", srv.Addr))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logSvc.Error("server stopped", err)
			os.Exit(1)
		}
	case sig := <-sigCh:
		logSvc.Info(fmt.Sprintf("shutdown signal: %s", sig))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logSvc.Error("shutdown error", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logSvc.Error("server stopped", err)
			os.Exit(1)
		}
	}
}
