package middleware

import (
	"net/http"
	"strings"

	"erp-connector/internal/api/utils"
)

func Auth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] != token {
			utils.WriteError(w, http.StatusUnauthorized, "Unauthorized", "UNAUTHORIZED", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
