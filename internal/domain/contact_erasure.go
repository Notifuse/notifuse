package domain

//go:generate mockgen -destination mocks/mock_contact_erasure_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain ContactErasureRepository

import (
	"context"
	"time"
)

// OptOutHashSource says which deletion recorded an opt-out hash.
type OptOutHashSource string

const (
	// OptOutHashSourceDelete is contacts.delete.
	OptOutHashSourceDelete OptOutHashSource = "delete"
	// OptOutHashSourceRetention is the data retention purge of inactive contacts.
	OptOutHashSourceRetention OptOutHashSource = "retention"
)

// OptOutHashRetentionYears is how long a deleted contact's opt-outs are remembered, in
// calendar years. The CNIL asks for an opposition list kept at least three years with only
// the data it needs.
const OptOutHashRetentionYears = 3

// OptOutHashExpiry returns the recording date before which an opt-out hash is deleted.
// Calendar years, not a count of days: three years span a leap day or not.
func OptOutHashExpiry(now time.Time) time.Time {
	return now.AddDate(-OptOutHashRetentionYears, 0, 0)
}

// ContactErasureRepository holds the erasure steps of a contact that span tables no other
// repository owns: their automation journeys, the webhook deliveries that carry their
// address, and the hashes that keep their opt-outs once they are deleted.
type ContactErasureRepository interface {
	// DeleteListRowsKeepingOptOuts deletes the contact's contact_lists rows and, in the same
	// statement, copies the unsubscribed, bounced and complained ones, removed ones included,
	// into contact_opt_out_hashes under the keyed hash of the address. One statement, so an
	// opt-out written while the contact is being deleted is either deleted with its hash or
	// not deleted at all. A hash already there for a list takes the newer status. It reports
	// how many hashes it wrote.
	DeleteListRowsKeepingOptOuts(ctx context.Context, workspaceID, email string, source OptOutHashSource) (int64, error)

	// ForgetOptOutHashes deletes every hash kept for the address, such as one an earlier
	// attempt to delete the contact recorded: forget_opt_outs keeps nothing.
	ForgetOptOutHashes(ctx context.Context, workspaceID, email string) error

	// DeleteJourneysForEmail deletes the contact's automation journeys (their step
	// executions cascade) and their automation_trigger_log rows.
	DeleteJourneysForEmail(ctx context.Context, workspaceID, email string) error

	// DeleteEngagementForEmail deletes the contact's engagement row (the data retention
	// clock), so a contact created again later does not inherit it.
	DeleteEngagementForEmail(ctx context.Context, workspaceID, email string) error

	// DeleteWebhookDeliveriesForEmail deletes the webhook deliveries whose payload carries
	// the address, whatever their status, except the two kinds the deletion itself sends,
	// contact.deleted and segment.left: they are how subscribers learn of it.
	DeleteWebhookDeliveriesForEmail(ctx context.Context, workspaceID, email string) (int64, error)

	// PurgeOptOutHashes deletes the hashes recorded before recordedBefore, and those of
	// lists that were deleted since. It reports how many it deleted.
	PurgeOptOutHashes(ctx context.Context, workspaceID string, recordedBefore time.Time) (int64, error)
}
