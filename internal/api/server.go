package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-connector/internal/config"
)

type ErrorResponse struct {
	Error   string         `json:"error"`
	Code    string         `json:"code"`
	Details map[string]any `json:"details,omitempty"`
}

func NewServer(cfg config.Config) (*http.Server, error) {
	addr := strings.TrimSpace(cfg.APIListen)
	if err := validateListenAddr(addr); err != nil {
		return nil, err
	}

	token := strings.TrimSpace(cfg.BearerToken)
	if token == "" {
		return nil, errors.New("bearerToken is required")
	}

	mux := http.NewServeMux()
	mux.Handle("/api/health", authMiddleware(token, http.HandlerFunc(healthHandler)))
	mux.Handle("/api/", authMiddleware(token, http.HandlerFunc(notFoundHandler)))

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

func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] != token {
			writeError(w, http.StatusUnauthorized, "Unauthorized", "UNAUTHORIZED")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "Not found", "NOT_FOUND")
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, ErrorResponse{
		Error:   message,
		Code:    code,
		Details: map[string]any{},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
