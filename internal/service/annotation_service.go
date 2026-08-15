package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// AnnotationService handles annotation business logic: the operator-facing CRUD
// and the automatic annotation written when a broadcast starts sending.
type AnnotationService struct {
	repo          domain.AnnotationRepository
	workspaceRepo domain.WorkspaceRepository
	authService   domain.AuthService
	logger        logger.Logger
}

// NewAnnotationService creates a new AnnotationService.
//
// It takes no broadcast repository on purpose: the sending-started event already
// carries the broadcast name, and the annotation is deliberately kept orphanable
// — deleting a broadcast leaves its annotation, because the send did happen.
func NewAnnotationService(
	repo domain.AnnotationRepository,
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *AnnotationService {
	return &AnnotationService{
		repo:          repo,
		workspaceRepo: workspaceRepo,
		authService:   authService,
		logger:        logger,
	}
}

// authorize confirms the caller is a member of the workspace they named and holds
// the web analytics permission at the requested level.
//
// INVARIANT: every authenticated method below takes workspace_id from the request
// and must call this before touching a repository — workspace_id selects a
// database and asserts nothing more.
//
// Annotations reuse PermissionResourceWebAnalytics rather than introducing a
// resource of their own: a new PermissionResource is invisible to existing
// members until a system migration backfills it, which would lock every current
// member out of the page on upgrade.
//
// The returned context carries the authenticated user and must be used downstream.
func (s *AnnotationService) authorize(ctx context.Context, workspaceID string, write bool) (context.Context, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}

	permission := domain.PermissionTypeRead
	access := "read"
	if write {
		permission = domain.PermissionTypeWrite
		access = "write"
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceWebAnalytics, permission) {
		return ctx, domain.NewPermissionError(
			domain.PermissionResourceWebAnalytics,
			permission,
			fmt.Sprintf("Insufficient permissions: %s access to web analytics required", access),
		)
	}

	return ctx, nil
}

// ListAnnotations returns the annotations matching the request's range and sources.
func (s *AnnotationService) ListAnnotations(ctx context.Context, req *domain.ListAnnotationsRequest) ([]*domain.Annotation, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID, false)
	if err != nil {
		return nil, err
	}

	// Validate normalises as well as checks: it is what clamps Limit into range.
	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	annotations, err := s.repo.List(ctx, req.WorkspaceID, domain.AnnotationFilter{
		Start:   req.Start,
		End:     req.End,
		Sources: req.Sources,
		Limit:   req.Limit,
	})
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": req.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to list annotations")
		return nil, fmt.Errorf("failed to list annotations: %w", err)
	}

	return annotations, nil
}

// GetAnnotation returns a single annotation.
func (s *AnnotationService) GetAnnotation(ctx context.Context, req *domain.GetAnnotationRequest) (*domain.Annotation, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID, false)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	annotation, err := s.repo.Get(ctx, req.WorkspaceID, req.ID)
	if err != nil {
		// Wrapped, not logged: a missing id is a 404 the handler unwraps with
		// errors.As, not an incident.
		return nil, fmt.Errorf("failed to get annotation: %w", err)
	}

	return annotation, nil
}

// CreateAnnotation writes a manual annotation.
func (s *AnnotationService) CreateAnnotation(ctx context.Context, req *domain.CreateAnnotationRequest) (*domain.Annotation, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID, true)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	color := req.Color
	if color == "" {
		color = domain.AnnotationDefaultColor
	}

	now := time.Now().UTC()
	annotation := &domain.Annotation{
		ID:          strings.ReplaceAll(uuid.New().String(), "-", ""),
		AnnotatedAt: req.AnnotatedAt,
		Timezone:    s.resolveTimezone(ctx, req.WorkspaceID, req.Timezone),
		Title:       req.Title,
		Description: req.Description,
		Color:       color,
		// Source and SourceID are forced rather than read from the request: only the
		// platform writes system rows, and a client-chosen source_id would claim the
		// idempotency slot of an automatic annotation.
		Source:    domain.AnnotationSourceManual,
		SourceID:  nil,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := annotation.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid annotation: %s", err.Error()))
	}

	if err := s.repo.Create(ctx, req.WorkspaceID, annotation); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": req.WorkspaceID,
			"error":        err.Error(),
		}).Error("Failed to create annotation")
		return nil, fmt.Errorf("failed to create annotation: %w", err)
	}

	return annotation, nil
}

// UpdateAnnotation edits an annotation's presentation and moment. System rows are
// editable — an operator may want to reword a broadcast's title — but their origin
// is not: Source and SourceID are reloaded from storage and carried forward, so an
// edit can neither promote a manual row to a system one nor steal another
// broadcast's idempotency slot.
func (s *AnnotationService) UpdateAnnotation(ctx context.Context, req *domain.UpdateAnnotationRequest) (*domain.Annotation, error) {
	ctx, err := s.authorize(ctx, req.WorkspaceID, true)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	existing, err := s.repo.Get(ctx, req.WorkspaceID, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation: %w", err)
	}

	color := req.Color
	if color == "" {
		color = existing.Color
	}
	if color == "" {
		color = domain.AnnotationDefaultColor
	}

	timezone := req.Timezone
	if timezone == "" {
		timezone = existing.Timezone
	}
	if timezone == "" {
		timezone = s.resolveTimezone(ctx, req.WorkspaceID, "")
	}

	annotation := &domain.Annotation{
		ID:          existing.ID,
		AnnotatedAt: req.AnnotatedAt,
		Timezone:    timezone,
		Title:       req.Title,
		Description: req.Description,
		Color:       color,
		Source:      existing.Source,
		SourceID:    existing.SourceID,
		CreatedAt:   existing.CreatedAt,
		UpdatedAt:   time.Now().UTC(),
	}

	if err := annotation.Validate(); err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("invalid annotation: %s", err.Error()))
	}

	if err := s.repo.Update(ctx, req.WorkspaceID, annotation); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id":  req.WorkspaceID,
			"annotation_id": req.ID,
			"error":         err.Error(),
		}).Error("Failed to update annotation")
		return nil, fmt.Errorf("failed to update annotation: %w", err)
	}

	return annotation, nil
}

// DeleteAnnotation removes an annotation, system rows included: the console asks a
// different question before deleting one, and a broadcast annotation cannot come
// back by accident — its broadcast has already started.
func (s *AnnotationService) DeleteAnnotation(ctx context.Context, req *domain.DeleteAnnotationRequest) error {
	ctx, err := s.authorize(ctx, req.WorkspaceID, true)
	if err != nil {
		return err
	}

	if err := req.Validate(); err != nil {
		return domain.NewValidationError(fmt.Sprintf("invalid request: %s", err.Error()))
	}

	if err := s.repo.Delete(ctx, req.WorkspaceID, req.ID); err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}

	return nil
}

// RegisterWithEventBus subscribes the automatic broadcast annotation.
func (s *AnnotationService) RegisterWithEventBus(eventBus domain.EventBus) {
	// A nil receiver or bus means the wiring in app.go was reordered and the service
	// is not built yet. Refusing rather than panicking keeps startup alive, but it
	// must not be silent: without this subscription no broadcast is ever annotated
	// and nothing else in the system notices.
	if s == nil {
		return
	}
	if eventBus == nil {
		s.logger.Error("Annotation service not registered with event bus: nil bus, broadcast annotations are disabled")
		return
	}

	eventBus.Subscribe(domain.EventBroadcastSendingStarted, s.HandleBroadcastSendingStarted)
	s.logger.Info("Annotation service registered with event bus")
}

// HandleBroadcastSendingStarted writes the annotation for a broadcast that has
// begun sending.
//
// This is NOT an authenticated path: it runs from the event bus on behalf of the
// platform, so it goes straight to the repositories and must never call authorize
// — there is no user in the context and authorize has no system-call bypass.
func (s *AnnotationService) HandleBroadcastSendingStarted(ctx context.Context, payload domain.EventPayload) {
	// The bus gives a plain Publish no deadline whatsoever: it fires handlers into
	// detached goroutines and the 5s timeout lives on the PublishWithAck path only.
	// So the handler bounds itself, and detaches from the publisher's context — the
	// request or task run that started the send may be gone before this runs.
	// The accepted cost: an annotation is lost if the process exits right after the
	// publish. Acceptable for an annotation, never for the send itself.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if payload.WorkspaceID == "" || payload.EntityID == "" {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": payload.WorkspaceID,
			"entity_id":    payload.EntityID,
		}).Error("Cannot annotate broadcast send: incomplete event payload")
		return
	}

	title, _ := payload.Data["broadcast_name"].(string)
	if title == "" {
		// An unnamed broadcast still deserves its vertical on the chart; a blank title
		// would fail validation and lose the annotation entirely.
		title = "Broadcast"
	}
	// Truncated by runes, not bytes: title is VARCHAR(100) characters, and slicing
	// bytes would both over-truncate and be able to cut a multi-byte rune in half.
	if runes := []rune(title); len(runes) > domain.AnnotationMaxTitleLength {
		title = string(runes[:domain.AnnotationMaxTitleLength])
	}

	annotatedAt := time.Now().UTC()
	if raw, ok := payload.Data["started_at"].(string); ok && raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			annotatedAt = parsed
		}
	}

	sourceID := payload.EntityID
	annotation := &domain.Annotation{
		ID:          strings.ReplaceAll(uuid.New().String(), "-", ""),
		AnnotatedAt: annotatedAt,
		Timezone:    s.resolveTimezone(ctx, payload.WorkspaceID, ""),
		Title:       title,
		Color:       domain.AnnotationBroadcastColor,
		Source:      domain.AnnotationSourceBroadcast,
		SourceID:    &sourceID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := annotation.Validate(); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": payload.EntityID,
			"error":        err.Error(),
		}).Error("Failed to annotate broadcast send: invalid annotation")
		return
	}

	// CreateFromSource collapses a duplicate publish — a run that annotated and then
	// failed before persisting its offset republishes on the next attempt — onto the
	// existing row instead of raising.
	created, err := s.repo.CreateFromSource(ctx, payload.WorkspaceID, annotation)
	if err != nil {
		// Logged and swallowed: an annotation must never be able to fail a send.
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": payload.EntityID,
			"error":        err.Error(),
		}).Error("Failed to annotate broadcast send")
		return
	}

	if !created {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": payload.EntityID,
		}).Debug("Broadcast already annotated, skipping")
		return
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": payload.WorkspaceID,
		"broadcast_id": payload.EntityID,
	}).Info("Broadcast send annotated")
}

// resolveTimezone picks the annotation's display timezone: what the caller asked
// for, else the workspace default, else UTC. It never returns an empty string —
// the column is NOT NULL and Validate rejects a blank.
//
// A workspace lookup failure is deliberately not fatal: timezone is display intent
// only, and an annotation stored as UTC is worth far more than no annotation.
func (s *AnnotationService) resolveTimezone(ctx context.Context, workspaceID, requested string) string {
	if requested != "" {
		return requested
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil || workspace == nil {
		return "UTC"
	}

	// Guard the stored value too: a workspace row predating timezone validation
	// would otherwise make every annotation fail to validate.
	if !domain.IsValidTimezone(workspace.Settings.Timezone) {
		return "UTC"
	}

	return workspace.Settings.Timezone
}
