package domain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen -destination mocks/mock_web_analytics_contact_visits_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain WebAnalyticsContactVisitsRepository
//go:generate mockgen -destination mocks/mock_web_analytics_contact_visits_service.go -package mocks github.com/Notifuse/notifuse/internal/domain WebAnalyticsContactVisitsService

// ErrWebAnalyticsNotEnabled is returned when a workspace has no web analytics settings, or
// has them turned off.
var ErrWebAnalyticsNotEnabled = errors.New("web analytics is not enabled")

// Filters of webAnalytics.contactVisits.
const (
	ContactVisitsFilterAll   = "all"
	ContactVisitsFilterGoal  = "goal"  // sessions with at least one goal
	ContactVisitsFilterEmail = "email" // sessions whose channel group is email
)

const (
	ContactVisitsDefaultLimit = 20
	ContactVisitsMaxLimit     = 50
	// ContactVisitsSummaryGoals and ContactVisitsSummaryTopPages bound the summary lists.
	ContactVisitsSummaryGoals    = 5
	ContactVisitsSummaryTopPages = 3
	// ContactSessionPagesMax bounds both the pages and the goals of one session.
	ContactSessionPagesMax = 200
)

// ContactVisitsRequest is the query of webAnalytics.contactVisits.
type ContactVisitsRequest struct {
	WorkspaceID string
	Email       string
	Filter      string
	Limit       int
	Cursor      string
}

// FromQuery reads the request, applies the defaults and validates it.
func (r *ContactVisitsRequest) FromQuery(query url.Values) error {
	r.WorkspaceID = query.Get("workspace_id")
	r.Email = NormalizeEmail(query.Get("email"))
	r.Filter = query.Get("filter")
	if r.Filter == "" {
		r.Filter = ContactVisitsFilterAll
	}
	r.Limit = ContactVisitsDefaultLimit
	if v := query.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid limit: must be an integer")
		}
		r.Limit = n
	}
	r.Cursor = query.Get("cursor")
	return r.Validate()
}

// Validate checks the request.
func (r *ContactVisitsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	switch r.Filter {
	case ContactVisitsFilterAll, ContactVisitsFilterGoal, ContactVisitsFilterEmail:
	default:
		return fmt.Errorf("invalid filter %q: must be all, goal or email", r.Filter)
	}
	if r.Limit < 1 || r.Limit > ContactVisitsMaxLimit {
		return fmt.Errorf("limit must be between 1 and %d", ContactVisitsMaxLimit)
	}
	if r.Cursor != "" {
		if _, _, err := DecodeContactVisitsCursor(r.Cursor); err != nil {
			return err
		}
	}
	return nil
}

// EncodeContactVisitsCursor returns the opaque cursor after the session created at
// createdAt with this ID.
func EncodeContactVisitsCursor(createdAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// DecodeContactVisitsCursor reads a cursor made by EncodeContactVisitsCursor.
func DecodeContactVisitsCursor(cursor string) (time.Time, string, error) {
	invalid := fmt.Errorf("invalid cursor")
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", invalid
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, "", invalid
	}
	createdAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, "", invalid
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return time.Time{}, "", invalid
	}
	return createdAt, parsed.String(), nil
}

// ContactSessionPagesRequest is the query of webAnalytics.contactSessionPages.
type ContactSessionPagesRequest struct {
	WorkspaceID string
	Email       string
	SessionID   string
	// SessionDate is the session's YYYY-MM-DD partition date, as contactVisits returns it.
	SessionDate string
}

// FromQuery reads and validates the request.
func (r *ContactSessionPagesRequest) FromQuery(query url.Values) error {
	r.WorkspaceID = query.Get("workspace_id")
	r.Email = NormalizeEmail(query.Get("email"))
	r.SessionID = query.Get("session_id")
	r.SessionDate = query.Get("session_date")
	return r.Validate()
}

// Validate checks the request and puts the session ID in canonical form.
func (r *ContactSessionPagesRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	parsed, err := uuid.Parse(r.SessionID)
	if err != nil {
		return fmt.Errorf("session_id must be a UUID")
	}
	r.SessionID = parsed.String()
	if r.SessionDate == "" {
		return fmt.Errorf("session_date is required")
	}
	if _, err := time.Parse(time.DateOnly, r.SessionDate); err != nil {
		return fmt.Errorf("session_date must be a YYYY-MM-DD date")
	}
	return nil
}

// ContactVisitSession is one session of a contact.
type ContactVisitSession struct {
	ID             string    `json:"id"`
	SessionDate    string    `json:"session_date"` // YYYY-MM-DD, what contactSessionPages takes
	CreatedAt      time.Time `json:"created_at"`
	DurationMs     int64     `json:"duration_ms"`
	PageviewCount  int       `json:"pageview_count"`
	LandingPath    string    `json:"landing_path"`
	ExitPath       string    `json:"exit_path"`
	Channel        string    `json:"channel"`
	ChannelGroup   string    `json:"channel_group"`
	ReferrerDomain string    `json:"referrer_domain"`
	UTMSource      string    `json:"utm_source"`
	UTMCampaign    string    `json:"utm_campaign"`
	Device         string    `json:"device"`
	OS             string    `json:"os"`
	Browser        string    `json:"browser"`
	Country        string    `json:"country"`
	GoalCount      int       `json:"goal_count"`
	GoalValue      float64   `json:"goal_value"`
	// FirstGoalName is the session's earliest goal, null without one.
	FirstGoalName *string `json:"first_goal_name"`
}

// ContactVisitGoal is one goal of a contact's summary.
type ContactVisitGoal struct {
	GoalName string  `json:"goal_name"`
	Count    int     `json:"count"`
	Value    float64 `json:"value"`
}

// ContactVisitPage is one of a contact's most viewed pages.
type ContactVisitPage struct {
	Path  string `json:"path"`
	Views int    `json:"views"`
}

// ContactVisitsSummary covers all of a contact's sessions, whatever the filter.
type ContactVisitsSummary struct {
	Sessions         int                  `json:"sessions"`
	GoalSessions     int                  `json:"goal_sessions"`
	EmailSessions    int                  `json:"email_sessions"`
	MedianDurationMs int64                `json:"median_duration_ms"`
	FirstSession     *ContactVisitSession `json:"first_session"`
	Goals            []ContactVisitGoal   `json:"goals"`
	TopPages         []ContactVisitPage   `json:"top_pages"`
}

// ContactVisitsResponse is the answer of webAnalytics.contactVisits. Summary is only on
// the first page.
type ContactVisitsResponse struct {
	Summary    *ContactVisitsSummary `json:"summary,omitempty"`
	Sessions   []ContactVisitSession `json:"sessions"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

// ContactSessionPage is one pageview of a session.
type ContactSessionPage struct {
	EnteredAt  time.Time `json:"entered_at"`
	Path       string    `json:"path"`
	DurationMs int64     `json:"duration_ms"`
	PageNumber int       `json:"page_number"`
	TabID      int64     `json:"tab_id"`
}

// ContactSessionGoal is one goal reached in a session.
type ContactSessionGoal struct {
	GoalAt    time.Time `json:"goal_at"`
	GoalName  string    `json:"goal_name"`
	GoalValue float64   `json:"goal_value"`
	Path      string    `json:"path"`
	TabID     int64     `json:"tab_id"`
}

// ContactSessionPagesResponse is the answer of webAnalytics.contactSessionPages.
type ContactSessionPagesResponse struct {
	Pages []ContactSessionPage `json:"pages"`
	Goals []ContactSessionGoal `json:"goals"`
}

// WebAnalyticsContactVisitsRepository reads one contact's web sessions. Every email is
// normalized by the caller.
type WebAnalyticsContactVisitsRepository interface {
	// ContactVisitsSummary summarizes all of the contact's sessions.
	ContactVisitsSummary(ctx context.Context, workspaceID string, email string) (*ContactVisitsSummary, error)
	// ListContactSessions returns a page of the contact's sessions, newest first, and the
	// cursor of the next page ("" on the last one).
	ListContactSessions(ctx context.Context, workspaceID string, email string, filter string, limit int, cursor string) ([]ContactVisitSession, string, error)
	// ContactSessionPages returns the pages and goals of one session, empty unless the
	// session belongs to the contact.
	ContactSessionPages(ctx context.Context, workspaceID string, email string, sessionDate string, sessionID string) (*ContactSessionPagesResponse, error)
}

// WebAnalyticsContactVisitsService serves a contact's web visits to the console.
type WebAnalyticsContactVisitsService interface {
	ContactVisits(ctx context.Context, req ContactVisitsRequest) (*ContactVisitsResponse, error)
	ContactSessionPages(ctx context.Context, req ContactSessionPagesRequest) (*ContactSessionPagesResponse, error)
}
