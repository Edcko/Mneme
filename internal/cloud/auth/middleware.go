package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Middleware returns an HTTP middleware that authenticates requests.
//
// It accepts two auth methods:
//   - Bearer token: Authorization: Bearer <jwt>
//   - API key:      X-API-Key: <raw-api-key>
//
// If neither is present or both are invalid, it responds with 401.
// On success, CustomClaims are injected into the request context.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try Bearer token first
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := m.ValidateToken(tokenStr)
			if err == nil {
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Try API key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" {
			if m.validateAnyKey(apiKey) {
				keyID := m.keyIDByRawKey(apiKey)
				now := time.Now()
				claims := &CustomClaims{
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   "api-key:" + keyID,
						Issuer:    "mneme-cloud",
						IssuedAt:  jwt.NewNumericDate(now),
						ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
					},
					Project: "",
				}
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})
}
