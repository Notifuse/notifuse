package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContactListStatusTransitionAllowed(t *testing.T) {
	all := []ContactListStatus{
		ContactListStatusActive, ContactListStatusPending, ContactListStatusUnsubscribed,
		ContactListStatusBounced, ContactListStatusComplained,
	}
	allowed := map[ContactListStatus][]ContactListStatus{
		// Not opted out: anywhere.
		ContactListStatusActive:  all,
		ContactListStatusPending: all,
		// Opted out: itself or a stronger opt-out, never back to a subscription.
		ContactListStatusUnsubscribed: {ContactListStatusUnsubscribed, ContactListStatusBounced, ContactListStatusComplained},
		ContactListStatusBounced:      {ContactListStatusBounced, ContactListStatusComplained},
		ContactListStatusComplained:   {ContactListStatusComplained},
	}
	for _, from := range all {
		for _, to := range all {
			want := false
			for _, ok := range allowed[from] {
				if ok == to {
					want = true
				}
			}
			assert.Equal(t, want, ContactListStatusTransitionAllowed(from, to), "%s -> %s", from, to)
		}
	}
}

func TestIsOptOutContactListStatus(t *testing.T) {
	assert.True(t, IsOptOutContactListStatus(ContactListStatusUnsubscribed))
	assert.True(t, IsOptOutContactListStatus(ContactListStatusBounced))
	assert.True(t, IsOptOutContactListStatus(ContactListStatusComplained))
	assert.False(t, IsOptOutContactListStatus(ContactListStatusActive))
	assert.False(t, IsOptOutContactListStatus(ContactListStatusPending))
}

func TestUpdateContactListStatusRequest_Resubscribe(t *testing.T) {
	base := UpdateContactListStatusRequest{WorkspaceID: "ws", Email: "a@example.com", ListID: "l"}

	withActive := base
	withActive.Status = "active"
	withActive.Resubscribe = true
	_, _, err := withActive.Validate()
	assert.NoError(t, err)

	for _, status := range []string{"pending", "unsubscribed", "bounced", "complained"} {
		req := base
		req.Status = status
		req.Resubscribe = true
		_, _, err := req.Validate()
		assert.Error(t, err, status)
	}
}

func TestErrContactListOptedOut_Message(t *testing.T) {
	assert.Contains(t, (&ErrContactListOptedOut{Status: ContactListStatusUnsubscribed}).Error(), "resubscribe")
	assert.Contains(t, (&ErrContactListOptedOut{Status: ContactListStatusBounced}).Error(), "console")
	assert.Contains(t, (&ErrContactListOptedOut{Status: ContactListStatusComplained}).Error(), "complained")
}

func TestAuditRecord_HasMetadata(t *testing.T) {
	var nilRecord *AuditRecord
	assert.False(t, nilRecord.HasMetadata(AuditResubscribedListIDs))

	record := NewAuditRecord("", "", "")
	assert.False(t, record.HasMetadata(AuditResubscribedListIDs))
	record.AddMetadata("other", 1)
	assert.False(t, record.HasMetadata(AuditResubscribedListIDs))
	record.AddMetadata(AuditResubscribedListIDs, []string{"l"})
	assert.True(t, record.HasMetadata(AuditResubscribedListIDs))

	record.Finalize(200)
	assert.True(t, record.HasMetadata(AuditResubscribedListIDs), "still readable once finalized")
}
