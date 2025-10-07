package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
)

// writeJSONError writes a JSON error response with the given message and status code
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// AuthConfig holds the configuration for the auth middleware
type AuthConfig struct {
	GetPublicKey func() (paseto.V4AsymmetricPublicKey, error)
}

// NewAuthMiddleware creates a new auth middleware with the given public key provider
func NewAuthMiddleware(getPublicKey func() (paseto.V4AsymmetricPublicKey, error)) *AuthConfig {
	return &AuthConfig{
		GetPublicKey: getPublicKey,
	}
}

// RequireAuth creates a middleware that verifies the PASETO token and user session
func (ac *AuthConfig) RequireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, "Authorization header is required", http.StatusUnauthorized)
				return
			}

			// Check if it's a Bearer token
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeJSONError(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			token := parts[1]

			// Get public key on-demand
			publicKey, err := ac.GetPublicKey()
			if err != nil {
				writeJSONError(w, fmt.Sprintf("Authentication unavailable: %v", err), http.StatusServiceUnavailable)
				return
			}

			// Parse and verify the token
			parser := paseto.NewParser()
			parser.AddRule(paseto.NotExpired())

			// Verify token and get claims
			verified, err := parser.ParseV4Public(publicKey, token, nil)
			if err != nil {
				writeJSONError(w, fmt.Sprintf("Invalid token: %v", err), http.StatusUnauthorized)
				return
			}

			// Get user ID from claims
			userID, err := verified.GetString(string(domain.UserIDKey))
			if err != nil {
				writeJSONError(w, "User ID not found in token", http.StatusUnauthorized)
				return
			}

			// Get user type from claims
			userType, err := verified.GetString(string(domain.UserTypeKey))
			if err != nil {
				writeJSONError(w, "User type not found in token", http.StatusUnauthorized)
				return
			}

			// only users have session IDs
			var sessionID string
			if userType == string(domain.UserTypeUser) {
				sessionID, err = verified.GetString(string(domain.SessionIDKey))
				if err != nil {
					writeJSONError(w, "Session ID not found in token", http.StatusUnauthorized)
					return
				}
			}

			// put userId and sessionId in the context
			ctx := context.WithValue(r.Context(), domain.UserIDKey, userID)
			ctx = context.WithValue(ctx, domain.UserTypeKey, userType)
			if userType == string(domain.UserTypeUser) {
				ctx = context.WithValue(ctx, domain.SessionIDKey, sessionID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RestrictedInDemo creates a middleware that returns a 400 error if the server is in demo mode
//
// Usage example:
//
//	restrictedMiddleware := middleware.RestrictedInDemo(true)
//	mux.Handle("/api/sensitive.operation", restrictedMiddleware(http.HandlerFunc(handler)))
//
// This middleware should be applied to endpoints that should be disabled in demo environments,
// such as operations that modify critical data or perform destructive actions.
func RestrictedInDemo(isDemo bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isDemo {
				writeJSONError(w, "This operation is not allowed in demo mode", http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
