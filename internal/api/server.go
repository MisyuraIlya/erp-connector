package api

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-connector/internal/api/handlers"
	"erp-connector/internal/api/middleware"
	"erp-connector/internal/api/utils"
	"erp-connector/internal/config"
	"erp-connector/internal/logger"
)

type ServerDeps struct {
	DBPassword string
	DB         *sql.DB
	Logger     logger.LoggerService
}

func NewServer(cfg config.Config, deps ServerDeps) (*http.Server, error) {
	addr := strings.TrimSpace(cfg.APIListen)
	if err := validateListenAddr(addr); err != nil {
		return nil, err
	}

	token := strings.TrimSpace(cfg.BearerToken)
	if token == "" {
		return nil, errors.New("bearerToken is required")
	}

	mux := http.NewServeMux()
	withAuth := func(h http.Handler) http.Handler {
		return middleware.Auth(token, h)
	}
	withLog := func(h http.Handler) http.Handler {
		return middleware.Logging(deps.Logger, cfg.Debug, h)
	}
	wrap := func(h http.Handler) http.Handler {
		return withLog(withAuth(h))
	}

	healthHandler := handlers.NewHealthHandler(cfg, deps.DBPassword)
	sqlHandler := handlers.NewSQLHandler(deps.DB)
	priceStockHandler := handlers.NewPriceAndStockHandler(cfg, deps.DB)
	folderFilesHandler := handlers.NewListFolderFilesHandler(cfg.ImageFolders)
	fileHandler := handlers.NewFileHandler(cfg.ImageFolders)

	mux.Handle("GET /api/health", wrap(healthHandler))
	mux.Handle("POST /api/sql", wrap(sqlHandler))
	mux.Handle("GET /api/folders/list", wrap(folderFilesHandler))
	mux.Handle("POST /api/file", wrap(fileHandler))
	mux.Handle("POST /api/sendOrder", wrap(http.HandlerFunc(handlers.SendOrder)))
	mux.Handle("POST /api/priceAndStockHandler", wrap(priceStockHandler))
	mux.Handle("/api/", wrap(http.HandlerFunc(NotFound)))

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}, nil
}

func validateListenAddr(addr string) error {
	if addr == "" {
		return errors.New("apiListen is required")
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("apiListen must be in host:port format")
	}
	if host == "" {
		return errors.New("apiListen host is required")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("apiListen port is invalid")
	}

	return nil
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	// TODO implement
	utils.WriteError(w, http.StatusNotFound, "Not found", "NOT_FOUND", nil)
}
