package http

import (
	"encoding/json"
	"net/http"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ConnectionStatsHandler struct {
	logger       logger.Logger
	getPublicKey func() (paseto.V4AsymmetricPublicKey, error)
}

func NewConnectionStatsHandler(
	logger logger.Logger,
	getPublicKey func() (paseto.V4AsymmetricPublicKey, error),
) *ConnectionStatsHandler {
	return &ConnectionStatsHandler{
		logger:       logger,
		getPublicKey: getPublicKey,
	}
}

// RegisterRoutes registers all connection stats routes
func (h *ConnectionStatsHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getPublicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register routes with authentication
	mux.Handle("/api/admin.connectionStats", requireAuth(http.HandlerFunc(h.getConnectionStats)))
}

// getConnectionStats returns current connection statistics (authenticated users only)
func (h *ConnectionStatsHandler) getConnectionStats(w http.ResponseWriter, r *http.Request) {
	// Get connection manager
	connManager, err := pkgDatabase.GetConnectionManager()
	if err != nil {
		h.logger.Error("Failed to get connection manager")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get stats
	stats := connManager.GetStats()

	// Return as JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to encode connection stats")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
