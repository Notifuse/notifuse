package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// capturedArg matches anything and keeps a copy of what it saw, so a test can
// assert on one argument buried in a 25-parameter statement without spelling
// out the other 24.
type capturedArg struct{ value []byte }

func (c *capturedArg) Match(v driver.Value) bool {
	if b, ok := v.([]byte); ok {
		c.value = append([]byte(nil), b...)
	}
	return true
}

// insertArgsCapturing returns the argument list for Create/Upsert with the
// message_data parameter replaced by the given capture.
func insertArgsCapturing(blob *capturedArg) []driver.Value {
	args := make([]driver.Value, 25)
	for i := range args {
		args[i] = sqlmock.AnyArg()
	}
	args[11] = blob // message_data is the 12th parameter of both INSERT statements
	return args
}

// TestMessageHistoryMessageDataEncryptionAtRest pins the contract 16.0 advertised
// — template data encrypted at rest with the workspace secret — on every path
// that writes the column.
//
// It had no test at all: both writers' existing cases pass sqlmock.AnyArg() for
// message_data, so a writer that skipped encryption asserted just as green as
// one that applied it. A failed send did exactly that for 22 releases, storing
// the blob in clear, and nothing failed — because a plaintext blob also reads
// back perfectly through the pre-16.0 compatibility branch covered below.
func TestMessageHistoryMessageDataEncryptionAtRest(t *testing.T) {
	const canary = "sk_live_the-token-inside-the-template-data"
	const workspaceID = "workspace-123"

	newMessage := func() *domain.MessageHistory {
		m := createSampleMessageHistory()
		m.MessageData = domain.MessageData{
			Data:     map[string]interface{}{"api_key": canary},
			Metadata: map[string]interface{}{"campaign": "onboarding"},
		}
		return m
	}

	assertEncrypts := func(t *testing.T, write func(repo domain.MessageHistoryRepository, ctx context.Context, m *domain.MessageHistory) error) {
		t.Helper()
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		blob := &capturedArg{}
		mock.ExpectExec(`INSERT INTO message_history`).
			WithArgs(insertArgsCapturing(blob)...).
			WillReturnResult(sqlmock.NewResult(1, 1))

		require.NoError(t, write(repo, context.Background(), newMessage()))
		require.NoError(t, mock.ExpectationsWereMet())
		require.NotEmpty(t, blob.value, "the message_data parameter was never captured")

		assert.NotContains(t, string(blob.value), canary, "the template data reached the column in clear")

		var stored domain.MessageData
		require.NoError(t, json.Unmarshal(blob.value, &stored))
		require.Contains(t, stored.Data, "_encrypted", "the blob went in unencrypted")

		// An envelope holding the wrong thing would pass the assertions above,
		// so decrypt it and look for the data itself.
		hex, ok := stored.Data["_encrypted"].(string)
		require.True(t, ok)
		plain, err := crypto.DecryptFromHexString(hex, testSecretKey)
		require.NoError(t, err)
		assert.Contains(t, plain, canary)

		// Metadata is deliberately left readable — 16.0 exempted it for query
		// performance — so an implementation that encrypted the whole column
		// would be a different contract, not a stricter one.
		assert.Equal(t, "onboarding", stored.Metadata["campaign"])
	}

	t.Run("Create encrypts the template data", func(t *testing.T) {
		assertEncrypts(t, func(repo domain.MessageHistoryRepository, ctx context.Context, m *domain.MessageHistory) error {
			return repo.Create(ctx, workspaceID, testSecretKey, m)
		})
	})

	t.Run("Upsert encrypts the template data", func(t *testing.T) {
		assertEncrypts(t, func(repo domain.MessageHistoryRepository, ctx context.Context, m *domain.MessageHistory) error {
			return repo.Upsert(ctx, workspaceID, testSecretKey, m)
		})
	})

	// Reads: the column can legitimately hold either shape, and the reader tells
	// them apart by the presence of the "_encrypted" key and nothing else.
	//
	// Pinned through ListMessages because messages.list is the only read of this
	// column that production reaches — the repository's other getters were unused
	// and are gone.
	listWithBlob := func(t *testing.T, storedBlob []byte) ([]*domain.MessageHistory, error) {
		t.Helper()
		mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
		defer cleanup()

		message := createSampleMessageHistory()
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "transactional_notification_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).AddRow(
			message.ID, message.ExternalID, message.ContactEmail, message.BroadcastID, message.AutomationID,
			nil, nil, message.TemplateID, message.TemplateVersion, message.Channel, message.StatusInfo,
			storedBlob, nil, []byte("[]"), message.SentAt, message.DeliveredAt, message.FailedAt,
			message.OpenedAt, message.ClickedAt, message.BouncedAt, message.ComplainedAt,
			message.UnsubscribedAt, message.CreatedAt, message.UpdatedAt,
		)
		mock.ExpectQuery(`SELECT .+ FROM message_history ORDER BY created_at DESC, id DESC LIMIT 2`).
			WillReturnRows(rows)

		messages, _, err := repo.ListMessages(context.Background(), workspaceID, testSecretKey, domain.MessageListParams{Limit: 1})
		return messages, err
	}

	readBack := func(t *testing.T, storedBlob []byte) *domain.MessageHistory {
		t.Helper()
		messages, err := listWithBlob(t, storedBlob)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		return messages[0]
	}

	t.Run("a read decrypts the blob back to the template data", func(t *testing.T) {
		ciphertext, err := crypto.EncryptString(`{"api_key":"`+canary+`"}`, testSecretKey)
		require.NoError(t, err)
		blob, err := json.Marshal(domain.MessageData{
			Data:     map[string]interface{}{"_encrypted": ciphertext},
			Metadata: map[string]interface{}{"campaign": "onboarding"},
		})
		require.NoError(t, err)

		result := readBack(t, blob)
		assert.Equal(t, canary, result.MessageData.Data["api_key"])
		assert.Equal(t, "onboarding", result.MessageData.Metadata["campaign"])
	})

	t.Run("a pre-16.0 plaintext blob still reads", func(t *testing.T) {
		// Encryption shipped without a backfill, so rows written before it stay
		// readable forever. This branch is also what makes an accidental
		// plaintext write invisible: it round-trips exactly like an encrypted one.
		blob, err := json.Marshal(domain.MessageData{
			Data: map[string]interface{}{"api_key": canary},
		})
		require.NoError(t, err)

		result := readBack(t, blob)
		assert.Equal(t, canary, result.MessageData.Data["api_key"])
	})

	t.Run("a blob that cannot be decrypted is an error, not a passthrough", func(t *testing.T) {
		// The alternative is serving the ciphertext to the caller as if it were
		// the template data, which is how a wrong secret key would read.
		blob, err := json.Marshal(domain.MessageData{
			Data: map[string]interface{}{"_encrypted": "6e6f742d612d76616c69642d626c6f62"},
		})
		require.NoError(t, err)

		messages, err := listWithBlob(t, blob)
		require.Error(t, err)
		assert.Empty(t, messages)
	})

	t.Run("an erased blob reads as empty instead of failing", func(t *testing.T) {
		// Contact erasure blanks the column to '{}'. Returning an error here
		// would take down the whole messages.list page, not just the one row.
		result := readBack(t, []byte(`{}`))
		assert.Empty(t, result.MessageData.Data)
	})
}
