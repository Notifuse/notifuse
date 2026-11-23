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

	// 3. The SSE headers go out with the first event, not before: StreamChat
	// authenticates first, and a refusal (a dead session, a missing second factor, a
	// missing grant) must still answer with its status and a JSON body, as the audit
	// export does (exportWriter).
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("Streaming not supported")
		WriteJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	streamStarted := false
	startStream := func() {
		if streamStarted {
			return
		}
		streamStarted = true
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
		w.WriteHeader(http.StatusOK)
	}

	// 4. Stream chat response.
	// Track whether a terminal event (done/error) reached the client so we can
	// guarantee one is always sent — otherwise the frontend spinner hangs forever
	// on any unexpected stream end (e.g. a provider panic or early return).
	terminalSent := false
	writeEvent := func(event domain.LLMChatEvent) {
		data, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return
		}
		startStream()
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		if event.Type == "done" || event.Type == "error" {
			terminalSent = true
		}
		flusher.Flush()
	}

	// Recover from a provider panic so net/http doesn't abort the TCP connection
	// (which surfaces in the browser as an opaque "NetworkError"). Emit a clean
	// error event instead.
	defer func() {
		if rec := recover(); rec != nil {
			h.logger.WithField("panic", fmt.Sprintf("%v", rec)).Error("Panic during LLM stream chat")
			if !terminalSent {
				writeEvent(domain.LLMChatEvent{Type: "error", Error: "internal server error"})
			}
		}
	}()

	err := h.service.StreamChat(r.Context(), &req, func(event domain.LLMChatEvent) error {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}

		startStream()
		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		if err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}

		if event.Type == "done" || event.Type == "error" {
			terminalSent = true
		}

		flusher.Flush()
		return nil
	})

	// 5. Refused before anything was streamed: a status and a JSON body.
	if err != nil && !streamStarted && writeServiceError(w, err, "Access denied") {
		return
	}

	// 6. Always deliver a terminal event so the client stops loading.
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Stream chat failed")
	}
	if !terminalSent {
		ev := domain.LLMChatEvent{Type: "error", Error: "stream ended unexpectedly"}
		if err != nil {
			ev.Error = err.Error()
		}
		writeEvent(ev)
	}
}
