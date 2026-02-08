package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// MissingParameterError is an error type for missing URL parameters
type MissingParameterError struct {
	Param string
}

// Error returns the error message
func (e *MissingParameterError) Error() string {
	return fmt.Sprintf("Missing parameter: %s", e.Param)
}

type BroadcastHandler struct {
	service      domain.BroadcastService
	templateSvc  domain.TemplateService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	isDemo       bool
}

func NewBroadcastHandler(service domain.BroadcastService, templateSvc domain.TemplateService, getJWTSecret func() ([]byte, error), logger logger.Logger, isDemo bool) *BroadcastHandler {
	return &BroadcastHandler{
		service:      service,
		templateSvc:  templateSvc,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		isDemo:       isDemo,
	}
}

func (h *BroadcastHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/broadcasts.list", requireAuth(http.HandlerFunc(h.HandleList)))
	mux.Handle("/api/broadcasts.get", requireAuth(http.HandlerFunc(h.HandleGet)))
	mux.Handle("/api/broadcasts.create", requireAuth(http.HandlerFunc(h.HandleCreate)))
	mux.Handle("/api/broadcasts.update", requireAuth(http.HandlerFunc(h.HandleUpdate)))
	mux.Handle("/api/broadcasts.schedule", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleSchedule))))
	mux.Handle("/api/broadcasts.pause", requireAuth(http.HandlerFunc(h.HandlePause)))
	mux.Handle("/api/broadcasts.resume", requireAuth(http.HandlerFunc(h.HandleResume)))
	mux.Handle("/api/broadcasts.cancel", requireAuth(http.HandlerFunc(h.HandleCancel)))
	mux.Handle("/api/broadcasts.sendToIndividual", requireAuth(http.HandlerFunc(h.HandleSendToIndividual)))
	mux.Handle("/api/broadcasts.delete", requireAuth(http.HandlerFunc(h.HandleDelete)))
	// A/B Testing endpoints
	mux.Handle("/api/broadcasts.getTestResults", requireAuth(http.HandlerFunc(h.HandleGetTestResults)))
	mux.Handle("/api/broadcasts.selectWinner", restrictedInDemo(requireAuth(http.HandlerFunc(h.HandleSelectWinner))))
	// Global feed endpoints
	mux.Handle("/api/broadcasts.refreshGlobalFeed", requireAuth(http.HandlerFunc(h.HandleRefreshGlobalFeed)))
	// Recipient feed endpoints
	mux.Handle("/api/broadcasts.testRecipientFeed", requireAuth(http.HandlerFunc(h.HandleTestRecipientFeed)))
}

// HandleList handles the broadcast list request
func (h *BroadcastHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetBroadcastsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	params := domain.ListBroadcastsParams{
		WorkspaceID:   req.WorkspaceID,
		Status:        domain.BroadcastStatus(req.Status),
		Limit:         req.Limit,
		Offset:        req.Offset,
		WithTemplates: req.WithTemplates,
	}

	response, err := h.service.ListBroadcasts(r.Context(), params)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list broadcasts")
		WriteJSONError(w, "Failed to list broadcasts", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"broadcasts":  response.Broadcasts,
		"total_count": response.TotalCount,
	})
}

// HandleGet handles the broadcast get request
func (h *BroadcastHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetBroadcastRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	broadcast, err := h.service.GetBroadcast(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get broadcast")
		WriteJSONError(w, "Failed to get broadcast", http.StatusInternalServerError)
		return
	}

	// If WithTemplates is true, fetch template details for each variation
	if req.WithTemplates {
		// Use the same logic as in ListBroadcasts to fetch templates
		for i, variation := range broadcast.TestSettings.Variations {
			if variation.TemplateID != "" {
				// Fetch the template for this variation
				template, err := h.templateSvc.GetTemplateByID(r.Context(), req.WorkspaceID, variation.TemplateID, 1)
				if err != nil {
					h.logger.WithFields(map[string]interface{}{
						"error":        err,
						"workspace_id": req.WorkspaceID,
						"broadcast_id": broadcast.ID,
						"template_id":  variation.TemplateID,
					}).Warn("Failed to fetch template for broadcast variation")
					// Continue with the next variation rather than failing the whole request
					continue
				}

				// Assign the template to the variation
				broadcast.SetTemplateForVariation(i, template)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"broadcast": broadcast,
	})
}

// HandleCreate handles the broadcast create request
func (h *BroadcastHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CreateBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err := req.Validate()
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	broadcast, err := h.service.CreateBroadcast(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to create broadcast")
		WriteJSONError(w, "Failed to create broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"broadcast": broadcast,
	})
}

// HandleUpdate handles the broadcast update request
func (h *BroadcastHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UpdateBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the existing broadcast to pass to Validate
	existingBroadcast, err := h.service.GetBroadcast(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get existing broadcast")
		WriteJSONError(w, "Failed to get existing broadcast", http.StatusInternalServerError)
		return
	}

	_, err = req.Validate(existingBroadcast)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	updatedBroadcast, err := h.service.UpdateBroadcast(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to update broadcast")
		WriteJSONError(w, "Failed to update broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"broadcast": updatedBroadcast,
	})
}

// HandleSchedule handles the broadcast schedule request
func (h *BroadcastHandler) HandleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ScheduleBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.ScheduleBroadcast(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to schedule broadcast")
		WriteJSONError(w, "Failed to schedule broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandlePause handles the broadcast pause request
func (h *BroadcastHandler) HandlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.PauseBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.PauseBroadcast(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to pause broadcast")
		WriteJSONError(w, "Failed to pause broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleResume handles the broadcast resume request
func (h *BroadcastHandler) HandleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ResumeBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.ResumeBroadcast(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to resume broadcast")
		WriteJSONError(w, "Failed to resume broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleCancel handles the broadcast cancel request
func (h *BroadcastHandler) HandleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CancelBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.CancelBroadcast(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to cancel broadcast")
		WriteJSONError(w, "Failed to cancel broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleSendToIndividual handles the broadcast send to individual request
func (h *BroadcastHandler) HandleSendToIndividual(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SendToIndividualRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.SendToIndividual(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to send broadcast to individual")
		WriteJSONError(w, "Failed to send broadcast to individual", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleDelete handles the broadcast delete request
func (h *BroadcastHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.DeleteBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.DeleteBroadcast(r.Context(), &req)
	if err != nil {
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to delete broadcast")
		WriteJSONError(w, "Failed to delete broadcast", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleGetTestResults handles the A/B test results request
func (h *BroadcastHandler) HandleGetTestResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetTestResultsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := h.service.GetTestResults(r.Context(), req.WorkspaceID, req.ID)
	if err != nil {
		h.logger.WithFields(map[string]interface{}{
			"workspace_id": req.WorkspaceID,
			"broadcast_id": req.ID,
			"error":        err.Error(),
		}).Error("Failed to get test results")
		WriteJSONError(w, "Failed to get test results", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// HandleSelectWinner handles the winner selection request
func (h *BroadcastHandler) HandleSelectWinner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SelectWinnerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.service.SelectWinner(r.Context(), req.WorkspaceID, req.ID, req.TemplateID)
	if err != nil {
		h.logger.WithFields(map[string]interface{}{
			"workspace_id": req.WorkspaceID,
			"broadcast_id": req.ID,
			"template_id":  req.TemplateID,
			"error":        err.Error(),
		}).Error("Failed to select winner")
		WriteJSONError(w, "Failed to select winner", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// HandleRefreshGlobalFeed handles the POST /api/broadcasts.refreshGlobalFeed request
func (h *BroadcastHandler) HandleRefreshGlobalFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.RefreshGlobalFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	response, err := h.service.RefreshGlobalFeed(r.Context(), &req)
	if err != nil {
		// Check for specific error types
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to refresh global feed")
		WriteJSONError(w, "Failed to refresh global feed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

// HandleTestRecipientFeed handles the POST /api/broadcasts.testRecipientFeed request
func (h *BroadcastHandler) HandleTestRecipientFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.TestRecipientFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	response, err := h.service.TestRecipientFeed(r.Context(), &req)
	if err != nil {
		// Check for specific error types
		if _, ok := err.(*domain.ErrBroadcastNotFound); ok {
			WriteJSONError(w, "Broadcast not found", http.StatusNotFound)
			return
		}
		if _, ok := err.(*domain.ErrContactNotFoundForFeed); ok {
			WriteJSONError(w, "Contact not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to test recipient feed")
		WriteJSONError(w, "Failed to test recipient feed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
}
