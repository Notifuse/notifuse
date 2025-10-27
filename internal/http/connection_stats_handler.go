package http

import (
	"encoding/json"
	"net/http"

	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ConnectionStatsHandler struct {
	logger logger.Logger
}

func NewConnectionStatsHandler(logger logger.Logger) *ConnectionStatsHandler {
	return &ConnectionStatsHandler{
		logger: logger,
	}
}

// GetConnectionStats returns current connection statistics (admin only)
func (h *ConnectionStatsHandler) GetConnectionStats(w http.ResponseWriter, r *http.Request) {
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
