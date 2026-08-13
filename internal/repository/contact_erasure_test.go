package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Erasure of a deleted contact, at the SQL level.
//
// Two of these purges redact an identifying column and leave the address sitting
// in a payload blob on the SAME surviving row, which is worse than not redacting
// at all: the row reads as anonymised. Both blobs are returned by their list
// endpoints, so the address goes out beside a "DELETED_EMAIL" label.
//
// These tests assert on the SET list itself, because that is where the omission
// lives — a test that only checks the row count passes against either version.

func erasureMocks(t *testing.T) (*mocks.MockWorkspaceRepository, sqlmock.Sqlmock, *gomock.Controller, func()) {
	t.Helper()
	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).AnyTimes()

	return workspaceRepo, mock, ctrl, func() {
		_ = db.Close()
		ctrl.Finish()
	}
}

func TestMessageHistoryDeleteForEmailClearsTheMessageDataBlob(t *testing.T) {
	workspaceRepo, mock, _, cleanup := erasureMocks(t)
	defer cleanup()

	repo := NewMessageHistoryRepository(workspaceRepo)

	// message_data holds data.contact.email, plus the address embedded in the
	// notification-center and unsubscribe URLs, and the list endpoint DECRYPTS it
	// before returning. Redacting contact_email alone leaves the real address
	// beside the "DELETED_EMAIL" label.
	mock.ExpectExec(`UPDATE message_history SET .*message_data`).
		WithArgs("DELETED_EMAIL", "gone@example.com").
		WillReturnResult(sqlmock.NewResult(0, 2))

	require.NoError(t, repo.DeleteForEmail(context.Background(), "ws-1", "gone@example.com"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The column is NOT NULL, so the blob has to become an empty object rather than
// NULL — a NULL would abort the statement and leave the contact half-erased.
func TestMessageHistoryDeleteForEmailWritesAnEmptyJSONObject(t *testing.T) {
	workspaceRepo, mock, _, cleanup := erasureMocks(t)
	defer cleanup()

	repo := NewMessageHistoryRepository(workspaceRepo)

	mock.ExpectExec(`message_data = '\{\}'::jsonb`).
		WithArgs("DELETED_EMAIL", "gone@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.DeleteForEmail(context.Background(), "ws-1", "gone@example.com"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInboundWebhookEventDeleteForEmailClearsTheRawPayload(t *testing.T) {
	workspaceRepo, mock, _, cleanup := erasureMocks(t)
	defer cleanup()

	repo := NewInboundWebhookEventRepository(workspaceRepo)

	// raw_payload holds the verbatim provider body the address was parsed out of,
	// and for replies the from/to pair as top-level JSON keys.
	mock.ExpectExec(`UPDATE inbound_webhook_events SET .*raw_payload`).
		WithArgs("DELETED_EMAIL", "gone@example.com").
		WillReturnResult(sqlmock.NewResult(0, 3))

	require.NoError(t, repo.DeleteForEmail(context.Background(), "ws-1", "gone@example.com"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Both statements must still redact the column they always did. Adding the blob
// to the SET list and dropping the original would trade one leak for another.
func TestErasureStatementsStillRedactTheirIdentifyingColumn(t *testing.T) {
	t.Run("message_history", func(t *testing.T) {
		workspaceRepo, mock, _, cleanup := erasureMocks(t)
		defer cleanup()

		mock.ExpectExec(`contact_email = \$1`).
			WithArgs("DELETED_EMAIL", "gone@example.com").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, NewMessageHistoryRepository(workspaceRepo).
			DeleteForEmail(context.Background(), "ws-1", "gone@example.com"))
	})

	t.Run("inbound_webhook_events", func(t *testing.T) {
		workspaceRepo, mock, _, cleanup := erasureMocks(t)
		defer cleanup()

		mock.ExpectExec(`recipient_email = \$1`).
			WithArgs("DELETED_EMAIL", "gone@example.com").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, NewInboundWebhookEventRepository(workspaceRepo).
			DeleteForEmail(context.Background(), "ws-1", "gone@example.com"))
	})
}

// contact_segments has no email-only delete: RemoveContactFromSegment needs a
// segment id, and erasure does not know which segments a contact was in.
func TestSegmentRepositoryDeleteForEmailRemovesEverySegmentMembership(t *testing.T) {
	workspaceRepo, mock, _, cleanup := erasureMocks(t)
	defer cleanup()

	repo := NewSegmentRepository(workspaceRepo)

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments WHERE email = $1`)).
		WithArgs("gone@example.com").
		WillReturnResult(sqlmock.NewResult(0, 4))

	require.NoError(t, repo.DeleteForEmail(context.Background(), "ws-1", "gone@example.com"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSegmentRepositoryDeleteForEmailWrapsDatabaseErrors(t *testing.T) {
	workspaceRepo, mock, _, cleanup := erasureMocks(t)
	defer cleanup()

	repo := NewSegmentRepository(workspaceRepo)

	mock.ExpectExec(`DELETE FROM contact_segments`).
		WillReturnError(assertAnError)

	err := repo.DeleteForEmail(context.Background(), "ws-1", "gone@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete contact segments")
}

var assertAnError = &erasureTestError{}

type erasureTestError struct{}

func (e *erasureTestError) Error() string { return "db down" }

var _ domain.SegmentRepository = (domain.SegmentRepository)(nil)
