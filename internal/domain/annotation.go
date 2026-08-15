package domain

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

//go:generate mockgen -destination mocks/mock_annotation_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AnnotationService
//go:generate mockgen -destination mocks/mock_annotation_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AnnotationRepository

// Annotation sources. Only two exist: a row is either typed by an operator or
// written by the platform on their behalf. There is deliberately no "api"
// source — the public endpoints force manual, so nothing outside can claim a
// source_id and the idempotency slot that comes with it.
const (
	AnnotationSourceManual    = "manual"
	AnnotationSourceBroadcast = "broadcast"
)

// ValidAnnotationSources is the list of all accepted annotation sources.
var ValidAnnotationSources = []string{
	AnnotationSourceManual,
	AnnotationSourceBroadcast,
}

const (
	// AnnotationMaxTitleLength mirrors annotations.title VARCHAR(100). Counted in
	// runes, not bytes, because Postgres counts characters — a byte-based bound
	// would reject a title the column accepts.
	AnnotationMaxTitleLength       = 100
	AnnotationMaxDescriptionLength = 500
	AnnotationDefaultColor         = "#3b82f6"
	AnnotationBroadcastColor       = "#7763f1"
	AnnotationDefaultListLimit     = 100
	AnnotationMaxListLimit         = 1000
)

var annotationColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Annotation marks a moment on the web analytics charts — a launch, a campaign,
// an outage — so a spike has an explanation attached to it.
type Annotation struct {
	ID          string    `json:"id"`
	AnnotatedAt time.Time `json:"annotated_at"`
	// Timezone is display intent, never used for filtering: annotated_at already
	// fixes the instant. It is what lets "9am in Tokyo" render back as 9am rather
	// than as the reader's local equivalent.
	Timezone    string `json:"timezone"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color"`
	Source      string `json:"source"`
	// SourceID ties an automatic annotation to the entity that caused it (a
	// broadcast id today). It is the idempotency key of the partial unique index
	// on (source, source_id), and is always nil for manual rows.
	SourceID  *string   `json:"source_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks an annotation before it is written. It returns plain errors;
// the service is what converts them into a ValidationError, so the domain stays
// free of transport concerns.
func (a *Annotation) Validate() error {
	if a.Title == "" {
		return fmt.Errorf("title is required")
	}
	if utf8.RuneCountInString(a.Title) > AnnotationMaxTitleLength {
		return fmt.Errorf("title must be %d characters or less", AnnotationMaxTitleLength)
	}
	if utf8.RuneCountInString(a.Description) > AnnotationMaxDescriptionLength {
		return fmt.Errorf("description must be %d characters or less", AnnotationMaxDescriptionLength)
	}
	if !annotationColorRegex.MatchString(a.Color) {
		return fmt.Errorf("color must be a hex color like #3b82f6")
	}
	// IsValidTimezone, not time.LoadLocation: LoadLocation accepts "" and "Local",
	// neither of which means anything to a reader looking at a stored instant.
	// This is the same check WorkspaceSettings.Validate applies.
	if a.Timezone == "" {
		return fmt.Errorf("timezone is required")
	}
	if !IsValidTimezone(a.Timezone) {
		return fmt.Errorf("invalid timezone: %s", a.Timezone)
	}
	if a.AnnotatedAt.IsZero() {
		return fmt.Errorf("annotated_at is required")
	}
	if !isValidAnnotationSource(a.Source) {
		return fmt.Errorf("source must be one of: %v", ValidAnnotationSources)
	}
	return nil
}

// IsSystem reports whether the annotation was written by the platform rather
// than typed by an operator. System rows are editable but keep their source: the
// console tells the reader where they came from and warns differently on delete.
func (a *Annotation) IsSystem() bool {
	return a.Source != AnnotationSourceManual
}

func isValidAnnotationSource(source string) bool {
	for _, s := range ValidAnnotationSources {
		if s == source {
			return true
		}
	}
	return false
}

// AnnotationFilter is the List query. Every field is optional; the repository
// applies AnnotationDefaultListLimit when Limit is unset.
type AnnotationFilter struct {
	Start   *time.Time
	End     *time.Time
	Sources []string
	Limit   int
}

// ListAnnotationsRequest is the query behind GET /api/annotations.list.
type ListAnnotationsRequest struct {
	WorkspaceID string     `json:"workspace_id"`
	Start       *time.Time `json:"start,omitempty"`
	End         *time.Time `json:"end,omitempty"`
	Sources     []string   `json:"sources,omitempty"`
	Limit       int        `json:"limit,omitempty"`
}

// FromURLParams parses the list query string. A malformed timestamp is an error
// rather than a silently dropped filter: returning "no filter" for ?start=junk
// would answer a different question than the one asked.
func (r *ListAnnotationsRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if raw := values.Get("start"); raw != "" {
		start, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("start must be an RFC3339 timestamp")
		}
		r.Start = &start
	}

	if raw := values.Get("end"); raw != "" {
		end, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("end must be an RFC3339 timestamp")
		}
		r.End = &end
	}

	// Comma-separated, so the console can send ?sources=manual,broadcast.
	if raw := values.Get("sources"); raw != "" {
		for _, source := range strings.Split(raw, ",") {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			r.Sources = append(r.Sources, source)
		}
	}

	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("limit must be an integer")
		}
		r.Limit = limit
	}

	return nil
}

// Validate normalises the request, clamping Limit into range rather than
// rejecting it — a caller asking for too much gets the maximum, not a 400.
func (r *ListAnnotationsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Start != nil && r.End != nil && r.End.Before(*r.Start) {
		return fmt.Errorf("end must not be before start")
	}
	for _, source := range r.Sources {
		if !isValidAnnotationSource(source) {
			return fmt.Errorf("source must be one of: %v", ValidAnnotationSources)
		}
	}
	if r.Limit <= 0 {
		r.Limit = AnnotationDefaultListLimit
	}
	if r.Limit > AnnotationMaxListLimit {
		r.Limit = AnnotationMaxListLimit
	}
	return nil
}

// GetAnnotationRequest is the query behind GET /api/annotations.get.
type GetAnnotationRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *GetAnnotationRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	r.ID = values.Get("id")
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

func (r *GetAnnotationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

// CreateAnnotationRequest is the body of POST /api/annotations.create.
//
// Color and Timezone may be omitted: the service fills them from the workspace
// defaults. Source may only ever be empty or "manual" — see Validate.
type CreateAnnotationRequest struct {
	WorkspaceID string    `json:"workspace_id"`
	AnnotatedAt time.Time `json:"annotated_at"`
	Timezone    string    `json:"timezone,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Color       string    `json:"color,omitempty"`
	Source      string    `json:"source,omitempty"`
}

func (r *CreateAnnotationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	// A client asking for a system source is refused rather than quietly
	// downgraded to manual: it is trying to claim a source_id slot that belongs to
	// an automatic row, and a silent coercion would hide that from it.
	if r.Source != "" && r.Source != AnnotationSourceManual {
		return fmt.Errorf("source must be %q", AnnotationSourceManual)
	}
	return validateAnnotationFields(r.Title, r.Description, r.Color, r.Timezone, r.AnnotatedAt)
}

// UpdateAnnotationRequest is the body of POST /api/annotations.update.
//
// It carries no Source or SourceID: an edit must not be able to turn a manual
// row into a broadcast one, and the service reloads both from storage.
type UpdateAnnotationRequest struct {
	WorkspaceID string    `json:"workspace_id"`
	ID          string    `json:"id"`
	AnnotatedAt time.Time `json:"annotated_at"`
	Timezone    string    `json:"timezone,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Color       string    `json:"color,omitempty"`
}

func (r *UpdateAnnotationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	return validateAnnotationFields(r.Title, r.Description, r.Color, r.Timezone, r.AnnotatedAt)
}

// DeleteAnnotationRequest is the body of POST /api/annotations.delete.
type DeleteAnnotationRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

func (r *DeleteAnnotationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

// validateAnnotationFields holds the rules shared by create and update. Color and
// timezone are optional here — unlike on Annotation itself, where they are
// already resolved — because the service supplies the workspace defaults after
// validation.
func validateAnnotationFields(title, description, color, timezone string, annotatedAt time.Time) error {
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if utf8.RuneCountInString(title) > AnnotationMaxTitleLength {
		return fmt.Errorf("title must be %d characters or less", AnnotationMaxTitleLength)
	}
	if utf8.RuneCountInString(description) > AnnotationMaxDescriptionLength {
		return fmt.Errorf("description must be %d characters or less", AnnotationMaxDescriptionLength)
	}
	if color != "" && !annotationColorRegex.MatchString(color) {
		return fmt.Errorf("color must be a hex color like #3b82f6")
	}
	if timezone != "" && !IsValidTimezone(timezone) {
		return fmt.Errorf("invalid timezone: %s", timezone)
	}
	if annotatedAt.IsZero() {
		return fmt.Errorf("annotated_at is required")
	}
	return nil
}

// AnnotationRepository is the workspace-database persistence for annotations.
type AnnotationRepository interface {
	List(ctx context.Context, workspaceID string, filter AnnotationFilter) ([]*Annotation, error)
	Get(ctx context.Context, workspaceID, id string) (*Annotation, error)
	Create(ctx context.Context, workspaceID string, annotation *Annotation) error
	// CreateFromSource inserts an automatic annotation, returning created=false
	// when one already exists for the same (source, source_id). It is what makes a
	// retried or duplicated platform event collapse to a single row.
	CreateFromSource(ctx context.Context, workspaceID string, annotation *Annotation) (created bool, err error)
	Update(ctx context.Context, workspaceID string, annotation *Annotation) error
	Delete(ctx context.Context, workspaceID, id string) error
}

// AnnotationService is the authenticated business logic for annotations.
type AnnotationService interface {
	ListAnnotations(ctx context.Context, req *ListAnnotationsRequest) ([]*Annotation, error)
	GetAnnotation(ctx context.Context, req *GetAnnotationRequest) (*Annotation, error)
	CreateAnnotation(ctx context.Context, req *CreateAnnotationRequest) (*Annotation, error)
	UpdateAnnotation(ctx context.Context, req *UpdateAnnotationRequest) (*Annotation, error)
	DeleteAnnotation(ctx context.Context, req *DeleteAnnotationRequest) error
}
