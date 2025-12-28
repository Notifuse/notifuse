package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// LLMHandler handles LLM-related HTTP requests
type LLMHandler struct {
	service      domain.LLMService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewLLMHandler creates a new LLM handler
func NewLLMHandler(
	service domain.LLMService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *LLMHandler {
	return &LLMHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the LLM handler routes
func (h *LLMHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/llm.chat", requireAuth(http.HandlerFunc(h.handleChat)))
}

// handleChat handles the streaming chat endpoint
func (h *LLMHandler) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Parse request
	var req domain.LLMChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 2. Validate request
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 3. Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// 4. Get flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("Streaming not supported")
		WriteJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 5. Stream chat response
	err := h.service.StreamChat(r.Context(), &req, func(event domain.LLMChatEvent) error {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}

		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		if err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}

		flusher.Flush()
		return nil
	})

	// 6. Handle errors
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Stream chat failed")

		// Send error event (client may or may not receive this)
		errorEvent := domain.LLMChatEvent{
			Type:  "error",
			Error: err.Error(),
		}
		data, _ := json.Marshal(errorEvent)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}
