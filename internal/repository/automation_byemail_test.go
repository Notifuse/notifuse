package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// GetContactAutomationByEmail backs the "is this contact on that journey"
// lookups: not-found is a named, plain error (callers branch on it), the
// latest entry wins (ORDER BY entered_at DESC LIMIT 1 is in the SQL), and
// a corrupt context blob is an error, never a half-empty journey.

var contactAutomationCols = []string{
	"id", "automation_id", "contact_email", "current_node_id", "status",
	"exit_reason", "entered_at", "scheduled_at", "context", "retry_count",
	"last_error", "last_retry_at", "max_retries",
}

func TestAutomationRepository_GetContactAutomationByEmail_Contract(t *testing.T) {
	ctx := context.Background()
	entered := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("not found names the email and does not wrap ErrNoRows", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery(regexp.QuoteMeta("ORDER BY entered_at DESC")).
			WithArgs("auto-1", "ghost@example.com").
			WillReturnRows(sqlmock.NewRows(contactAutomationCols))

		ca, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "ghost@example.com")
		assert.Nil(t, ca)
		require.EqualError(t, err, "contact automation not found for email: ghost@example.com")
		assert.NotErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("the row maps verbatim including decoded context", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		nodeID := "node-9"
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WillReturnRows(sqlmock.NewRows(contactAutomationCols).
				AddRow("ca-1", "auto-1", "a@example.com", nodeID, "active",
					nil, entered, nil, []byte(`{"step":3,"vip":true}`), 2, nil, nil, 5))

		ca, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "a@example.com")
		require.NoError(t, err)
		assert.Equal(t, "ca-1", ca.ID)
		assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
		require.NotNil(t, ca.CurrentNodeID)
		assert.Equal(t, "node-9", *ca.CurrentNodeID)
		assert.Equal(t, map[string]interface{}{"step": float64(3), "vip": true}, ca.Context)
		assert.Equal(t, 2, ca.RetryCount)
		assert.Equal(t, 5, ca.MaxRetries)
	})

	t.Run("a corrupt context blob is an unmarshal error, not an empty context", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WillReturnRows(sqlmock.NewRows(contactAutomationCols).
				AddRow("ca-1", "auto-1", "a@example.com", nil, "active",
					nil, entered, nil, []byte(`{corrupt`), 0, nil, nil, 3))

		_, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "a@example.com")
		require.ErrorContains(t, err, "failed to unmarshal context:")
	})

	t.Run("driver errors are wrapped with the operation", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		sentinel := errors.New("connection reset")
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).WillReturnError(sentinel)

		_, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "a@example.com")
		require.ErrorContains(t, err, "failed to get contact automation by email:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("an empty stored context stays nil rather than an empty map", func(t *testing.T) {
		_, mock, repo := setupAutomationMock(t)
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_automations")).
			WillReturnRows(sqlmock.NewRows(contactAutomationCols).
				AddRow("ca-1", "auto-1", "a@example.com", nil, "completed",
					nil, entered, nil, nil, 0, nil, nil, 3))

		ca, err := repo.GetContactAutomationByEmail(ctx, "ws-1", "auto-1", "a@example.com")
		require.NoError(t, err)
		assert.Nil(t, ca.Context)
	})
}
