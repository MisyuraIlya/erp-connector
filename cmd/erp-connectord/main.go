package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"erp-connector/internal/api"
	"erp-connector/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			log.Print("config not found; run erp-connector UI to create it")
			os.Exit(1)
		}
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	srv, err := api.NewServer(cfg)
	if err != nil {
		log.Printf("config validation error: %v", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	log.Printf("erp-connectord listening on %s", srv.Addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
			os.Exit(1)
		}
	case sig := <-sigCh:
		log.Printf("shutdown signal: %s", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
			os.Exit(1)
		}
	}
}
