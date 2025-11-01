package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// Test JWT secret (32 bytes minimum for HS256)
var testJWTSecret = []byte("test-jwt-secret-key-1234567890123456")

func TestNewAuthMiddleware(t *testing.T) {
	// Create a callback that returns the JWT secret
	getJWTSecret := func() ([]byte, error) {
		return testJWTSecret, nil
	}

	// Create the middleware
	middleware := NewAuthMiddleware(getJWTSecret)

	// Assert the middleware is created
	assert.NotNil(t, middleware)
	assert.NotNil(t, middleware.GetJWTSecret)
}

func TestRequireAuth(t *testing.T) {
	// Create a callback that returns the JWT secret
	getJWTSecret := func() ([]byte, error) {
		return testJWTSecret, nil
	}

	// Create the middleware
	authConfig := NewAuthMiddleware(getJWTSecret)

	// Helper function to create a JWT token
	createToken := func(userID, userType, sessionID string, expiration time.Time) string {
		claims := jwt.MapClaims{
			"user_id": userID,
			"type":    userType,
			"exp":     expiration.Unix(),
			"iat":     time.Now().Unix(),
			"nbf":     time.Now().Unix(),
		}

		if sessionID != "" {
			claims["session_id"] = sessionID
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedToken, _ := token.SignedString(testJWTSecret)
		return signedToken
	}

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
		claims := jwt.MapClaims{
			"type":       string(domain.UserTypeUser),
			"session_id": "test-session",
			"exp":        time.Now().Add(time.Hour).Unix(),
			"iat":        time.Now().Unix(),
			"nbf":        time.Now().Unix(),
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedToken, _ := token.SignedString(testJWTSecret)

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

	t.Run("missing type in token", func(t *testing.T) {
		// Create a token with missing type
		claims := jwt.MapClaims{
			"user_id":    "test-user",
			"session_id": "test-session",
			"exp":        time.Now().Add(time.Hour).Unix(),
			"iat":        time.Now().Unix(),
			"nbf":        time.Now().Unix(),
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedToken, _ := token.SignedString(testJWTSecret)

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
		signedToken := createToken("test-user", string(domain.UserTypeUser), "", time.Now().Add(time.Hour))

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
		signedToken := createToken("test-user", string(domain.UserTypeUser), "test-session", time.Now().Add(time.Hour))

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
		signedToken := createToken("test-api-key", string(domain.UserTypeAPIKey), "", time.Now().Add(time.Hour))

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

	t.Run("expired token", func(t *testing.T) {
		// Create an expired token
		signedToken := createToken("test-user", string(domain.UserTypeUser), "test-session", time.Now().Add(-time.Hour))

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := authConfig.RequireAuth()(next)

		// Create a test request with the expired token
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+signedToken)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid token")
	})

	t.Run("token with wrong signing method", func(t *testing.T) {
		// Create a token with "none" algorithm to test algorithm confusion prevention
		claims := jwt.MapClaims{
			"user_id":    "test-user",
			"type":       string(domain.UserTypeUser),
			"session_id": "test-session",
			"exp":        time.Now().Add(time.Hour).Unix(),
			"iat":        time.Now().Unix(),
			"nbf":        time.Now().Unix(),
		}

		// Create a token with "none" algorithm (algorithm confusion attack)
		token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
		signedToken, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

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

		// Assert the response - should fail due to algorithm mismatch
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid token")
	})
}

func TestRestrictedInDemo(t *testing.T) {
	t.Run("allows request when not in demo mode", func(t *testing.T) {
		// Create config with demo mode disabled
		isDemo := false

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		})

		// Apply the middleware
		handler := RestrictedInDemo(isDemo)(next)

		// Create a test request
		req := httptest.NewRequest("POST", "/", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "success", w.Body.String())
	})

	t.Run("blocks request when in demo mode", func(t *testing.T) {
		// Create config with demo mode enabled
		isDemo := true

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		})

		// Apply the middleware
		handler := RestrictedInDemo(isDemo)(next)

		// Create a test request
		req := httptest.NewRequest("POST", "/", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "This operation is not allowed in demo mode")
	})

	t.Run("allows request when environment is development", func(t *testing.T) {
		// Create config with development environment
		isDemo := false

		// Create a test handler
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		})

		// Apply the middleware
		handler := RestrictedInDemo(isDemo)(next)

		// Create a test request
		req := httptest.NewRequest("POST", "/", nil)
		w := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(w, req)

		// Assert the response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "success", w.Body.String())
	})
}
