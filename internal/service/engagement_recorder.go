package service

import (
	"context"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// EngagementRecorder records when a contact did something themselves, for the engagement
// clock of the data retention plan. It is best effort: a failure is logged and never fails the
// click, the subscription, the reply or the visit that triggered it. A nil recorder records
// nothing, so a service built without one (tests, tools) works as before.
//
// Only what the contact does counts (data retention plan, D5): a verified click on a real
// destination, a reply that is not an auto-reply, their own subscription that leaves them
// active (a double opt-in confirmation included), an identified visit. An open, a send, an
// import, an API call or an admin's edit never does.
type EngagementRecorder struct {
	repo   domain.ContactEngagementRepository
	logger logger.Logger
}

// NewEngagementRecorder creates the recorder.
func NewEngagementRecorder(repo domain.ContactEngagementRepository, log logger.Logger) *EngagementRecorder {
	return &EngagementRecorder{repo: repo, logger: log}
}

// Touch records engagement for an address.
func (r *EngagementRecorder) Touch(ctx context.Context, workspaceID, email string, at time.Time) {
	if r == nil || email == "" {
		return
	}
	if err := r.repo.Touch(ctx, workspaceID, domain.NormalizeEmail(email), at); err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Warn("Failed to record contact engagement")
	}
}

// TouchForMessage records engagement for the recipient of a message.
func (r *EngagementRecorder) TouchForMessage(ctx context.Context, workspaceID, messageID string, at time.Time) {
	if r == nil || messageID == "" {
		return
	}
	if err := r.repo.TouchForMessage(ctx, workspaceID, messageID, at); err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Warn("Failed to record contact engagement for a click")
	}
}

// TouchForWebSessions records engagement for the contacts identified on web sessions.
func (r *EngagementRecorder) TouchForWebSessions(ctx context.Context, workspaceID string, sessions []*domain.WebSession) {
	if r == nil || len(sessions) == 0 {
		return
	}
	if err := r.repo.TouchForWebSessions(ctx, workspaceID, sessions); err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Warn("Failed to record contact engagement for visits")
	}
}
