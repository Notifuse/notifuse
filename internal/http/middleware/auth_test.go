package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestNewAuthMiddleware(t *testing.T) {
	// Generate a key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	// Create the middleware
	middleware := NewAuthMiddleware(publicKey)

	// Assert the middleware is created with the given key
	assert.Equal(t, publicKey, middleware.PublicKey)
}

func TestRequireAuth(t *testing.T) {
	// Generate a key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	// Create the middleware
	authConfig := NewAuthMiddleware(publicKey)

	t.Run("missing authorization header", func(t *testing.T) {
		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Authorization header is required")
	})

	t.Run("invalid authorization header format", func(t *testing.T) {
		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with invalid header
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "InvalidFormat")
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid authorization header format")
	})

	t.Run("invalid token", func(t *testing.T) {
		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with invalid token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer invalidtoken")
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid token")
	})

	t.Run("missing user_id in token", func(t *testing.T) {
		// Create a token with missing user_id
		token := paseto.NewToken()
		token.SetExpiration(time.Now().Add(time.Hour))
		// Intentionally omit setting user_id
		token.SetString(string(domain.UserTypeKey), string(domain.UserTypeUser))
		token.SetString(string(domain.SessionIDKey), "test-session")

		signedToken := token.V4Sign(secretKey, nil)

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with the token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+signedToken)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "User ID not found in token")
	})

	t.Run("missing user_type in token", func(t *testing.T) {
		// Create a token with missing user_type
		token := paseto.NewToken()
		token.SetExpiration(time.Now().Add(time.Hour))
		token.SetString(string(domain.UserIDKey), "test-user")
		// Intentionally omit setting user_type
		token.SetString(string(domain.SessionIDKey), "test-session")

		signedToken := token.V4Sign(secretKey, nil)

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with the token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+signedToken)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "User type not found in token")
	})

	t.Run("missing session_id for user type", func(t *testing.T) {
		// Create a token with missing session_id for user type
		token := paseto.NewToken()
		token.SetExpiration(time.Now().Add(time.Hour))
		token.SetString(string(domain.UserIDKey), "test-user")
		token.SetString(string(domain.UserTypeKey), string(domain.UserTypeUser))
		// Intentionally omit setting session_id

		signedToken := token.V4Sign(secretKey, nil)

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with the token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+signedToken)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Session ID not found in token")
	})

	t.Run("successful auth for user type", func(t *testing.T) {
		// Create a valid token for user type
		token := paseto.NewToken()
		token.SetExpiration(time.Now().Add(time.Hour))
		token.SetString(string(domain.UserIDKey), "test-user")
		token.SetString(string(domain.UserTypeKey), string(domain.UserTypeUser))
		token.SetString(string(domain.SessionIDKey), "test-session")

		signedToken := token.V4Sign(secretKey, nil)

		// Create a test handler that checks for context values
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check all values are correctly set in context
			userID := r.Context().Value(domain.UserIDKey)
			userType := r.Context().Value(domain.UserTypeKey)
			sessionID := r.Context().Value(domain.SessionIDKey)

			assert.Equal(t, "test-user", userID)
			assert.Equal(t, string(domain.UserTypeUser), userType)
			assert.Equal(t, "test-session", sessionID)

			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with the token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+signedToken)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("successful auth for api_key type", func(t *testing.T) {
		// Create a valid token for api_key type
		token := paseto.NewToken()
		token.SetExpiration(time.Now().Add(time.Hour))
		token.SetString(string(domain.UserIDKey), "test-api-key")
		token.SetString(string(domain.UserTypeKey), string(domain.UserTypeAPIKey))
		// No session ID needed for API keys

		signedToken := token.V4Sign(secretKey, nil)

		// Create a test handler that checks for context values
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check values are correctly set in context
			userID := r.Context().Value(domain.UserIDKey)
			userType := r.Context().Value(domain.UserTypeKey)
			sessionID := r.Context().Value(domain.SessionIDKey)

			assert.Equal(t, "test-api-key", userID)
			assert.Equal(t, string(domain.UserTypeAPIKey), userType)
			assert.Nil(t, sessionID) // Session ID should not be set for API keys

			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with the token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+signedToken)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
