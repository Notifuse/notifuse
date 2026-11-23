package domain

//go:generate mockgen -destination mocks/mock_contact_engagement_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactEngagementRepository

import (
	"context"
	"time"
)

// DataRetentionBackfillTaskType is the task that reads the engagement of a workspace's
// existing contacts back from their history, once, when the workspace is upgraded to 45.
const DataRetentionBackfillTaskType = "data_retention_backfill"

// DataRetentionBackfillState is the resumable state of that task: the contacts are walked
// in email order, a batch per step.
type DataRetentionBackfillState struct {
	AfterEmail string `json:"after_email,omitempty"` // the last email of the last batch done
	Contacts   int64  `json:"contacts"`              // contacts walked so far
	Engaged    int64  `json:"engaged"`               // engagement rows written so far
}

// ContactEngagementRepository records when a contact last did something themselves (the
// engagement clock of the data retention plan). Every write keeps the later date, never moves
// it back, and only touches contacts that exist: an engagement arriving after a deletion
// recreates nothing.
type ContactEngagementRepository interface {
	// Touch records engagement for an address at a time (capped at now).
	Touch(ctx context.Context, workspaceID, email string, at time.Time) error

	// TouchForMessage records engagement for the recipient of a message.
	TouchForMessage(ctx context.Context, workspaceID, messageID string, at time.Time) error

	// TouchForWebSessions records engagement for the contacts identified on web sessions,
	// at each session's last activity.
	TouchForWebSessions(ctx context.Context, workspaceID string, sessions []*WebSession) error

	// BackfillBatch reads the engagement of up to limit contacts after afterEmail (in email
	// order) back from history and records it. It returns the last email of the batch, the
	// number of contacts walked and of rows written. Zero contacts walked means done.
	BackfillBatch(ctx context.Context, workspaceID, afterEmail string, limit int) (lastEmail string, contacts int64, engaged int64, err error)

	// CompleteBackfill marks the workspace ready (data_retention_state).
	CompleteBackfill(ctx context.Context, workspaceID string) error

	// BackfillCompleted reports whether the workspace is ready.
	BackfillCompleted(ctx context.Context, workspaceID string) (bool, error)
}
