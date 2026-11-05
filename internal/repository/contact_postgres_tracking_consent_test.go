package repository

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

func TestTrackingConsentInsertValues(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	provided := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	t.Run("no consent leaves date and source NULL", func(t *testing.T) {
		opens, clicks, at, source := trackingConsentInsertValues(&domain.Contact{
			TrackingConsentSource:    &domain.NullableString{String: "legacy"},
			TrackingConsentUpdatedAt: &domain.NullableTime{Time: provided},
		}, now)
		assert.False(t, opens.Valid)
		assert.False(t, clicks.Valid)
		assert.False(t, at.Valid, "a date with no consent describes nothing")
		assert.False(t, source.Valid)
	})

	t.Run("explicit null is no consent", func(t *testing.T) {
		_, _, at, _ := trackingConsentInsertValues(&domain.Contact{
			TrackingOpensConsent: &domain.NullableString{IsNull: true},
		}, now)
		assert.False(t, at.Valid, "the test is on the SQL value, not the pointer")
	})

	t.Run("a consent without date is dated now", func(t *testing.T) {
		opens, clicks, at, source := trackingConsentInsertValues(&domain.Contact{
			TrackingClicksConsent: &domain.NullableString{String: domain.TrackingConsentDenied},
			TrackingConsentSource: &domain.NullableString{String: domain.TrackingConsentSourceAPI},
		}, now)
		assert.False(t, opens.Valid)
		assert.Equal(t, sql.NullString{String: domain.TrackingConsentDenied, Valid: true}, clicks)
		assert.Equal(t, sql.NullTime{Time: now, Valid: true}, at)
		assert.Equal(t, sql.NullString{String: domain.TrackingConsentSourceAPI, Valid: true}, source)
	})

	t.Run("a provided date is kept", func(t *testing.T) {
		_, _, at, _ := trackingConsentInsertValues(&domain.Contact{
			TrackingOpensConsent:     &domain.NullableString{String: domain.TrackingConsentGranted},
			TrackingConsentUpdatedAt: &domain.NullableTime{Time: provided},
		}, now)
		assert.Equal(t, sql.NullTime{Time: provided, Valid: true}, at)
	})
}

func TestSettleTrackingConsentUpdate(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	storedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	stored := func() *domain.Contact {
		return &domain.Contact{
			TrackingOpensConsent:     &domain.NullableString{String: domain.TrackingConsentGranted},
			TrackingConsentUpdatedAt: &domain.NullableTime{Time: storedAt},
			TrackingConsentSource:    &domain.NullableString{String: domain.TrackingConsentSourceImport},
		}
	}
	apply := func(update *domain.Contact) *domain.Contact {
		merged := stored()
		before := stored()
		merged.Merge(update)
		settleTrackingConsentUpdate(merged, before, update, now)
		return merged
	}

	t.Run("resending the same consent keeps date and source", func(t *testing.T) {
		got := apply(&domain.Contact{
			TrackingOpensConsent:     &domain.NullableString{String: domain.TrackingConsentGranted},
			TrackingConsentSource:    &domain.NullableString{String: domain.TrackingConsentSourceAPI},
			TrackingConsentUpdatedAt: &domain.NullableTime{Time: now},
		})
		assert.True(t, got.TrackingConsentUpdatedAt.Time.Equal(storedAt))
		assert.Equal(t, domain.TrackingConsentSourceImport, got.TrackingConsentSource.String)
	})

	t.Run("an update that names no consent keeps date and source", func(t *testing.T) {
		got := apply(&domain.Contact{FirstName: &domain.NullableString{String: "Ann"}})
		assert.True(t, got.TrackingConsentUpdatedAt.Time.Equal(storedAt))
		assert.Equal(t, domain.TrackingConsentSourceImport, got.TrackingConsentSource.String)
	})

	t.Run("a change without date or source is dated now with no source", func(t *testing.T) {
		got := apply(&domain.Contact{TrackingOpensConsent: &domain.NullableString{String: domain.TrackingConsentDenied}})
		assert.Equal(t, domain.TrackingConsentDenied, got.TrackingOpensConsent.String)
		assert.True(t, got.TrackingConsentUpdatedAt.Time.Equal(now))
		assert.Nil(t, got.TrackingConsentSource, "the stored source described another change")
	})

	t.Run("a change keeps the date and source provided", func(t *testing.T) {
		provided := time.Date(2024, 3, 3, 0, 0, 0, 0, time.UTC)
		got := apply(&domain.Contact{
			TrackingClicksConsent:    &domain.NullableString{String: domain.TrackingConsentGranted},
			TrackingConsentUpdatedAt: &domain.NullableTime{Time: provided},
			TrackingConsentSource:    &domain.NullableString{String: domain.TrackingConsentSourceConsole},
		})
		assert.True(t, got.TrackingConsentUpdatedAt.Time.Equal(provided))
		assert.Equal(t, domain.TrackingConsentSourceConsole, got.TrackingConsentSource.String)
	})

	t.Run("back to unknown is a change", func(t *testing.T) {
		got := apply(&domain.Contact{
			TrackingOpensConsent:  &domain.NullableString{IsNull: true},
			TrackingConsentSource: &domain.NullableString{String: domain.TrackingConsentSourceAPI},
		})
		assert.True(t, got.TrackingOpensConsent.IsNull)
		assert.True(t, got.TrackingConsentUpdatedAt.Time.Equal(now))
		assert.Equal(t, domain.TrackingConsentSourceAPI, got.TrackingConsentSource.String)
	})
}

func TestGetContacts_TrackingConsentFilters(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil).AnyTimes()
	repo := NewContactRepository(workspaceRepo)

	mock.ExpectQuery(`FROM contacts c WHERE c\.tracking_opens_consent IS NULL AND c\.tracking_clicks_consent = \$1 ORDER BY`).
		WithArgs(domain.TrackingConsentDenied).
		WillReturnRows(sqlmock.NewRows(contactColumns))

	_, err := repo.GetContacts(context.Background(), &domain.GetContactsRequest{
		WorkspaceID:           "ws",
		Limit:                 10,
		TrackingOpensConsent:  domain.TrackingConsentFilterUnknown,
		TrackingClicksConsent: domain.TrackingConsentDenied,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkUpsertContacts_TrackingConsentMerge(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil).AnyTimes()
	repo := NewContactRepository(workspaceRepo)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`tracking_opens_consent = CASE WHEN EXCLUDED.tracking_opens_consent IS NOT NULL THEN EXCLUDED.tracking_opens_consent ELSE contacts.tracking_opens_consent END`) +
		`.*` + regexp.QuoteMeta(`tracking_consent_updated_at = CASE WHEN `+bulkTrackingConsentChanged+` THEN COALESCE(EXCLUDED.tracking_consent_updated_at, NOW()) ELSE contacts.tracking_consent_updated_at END`) +
		`.*` + regexp.QuoteMeta(`tracking_consent_source = CASE WHEN `+bulkTrackingConsentChanged+` THEN EXCLUDED.tracking_consent_source ELSE contacts.tracking_consent_source END`)).
		WillReturnRows(sqlmock.NewRows([]string{"email", "is_new"}).AddRow("a@example.com", false))
	mock.ExpectCommit()

	_, err := repo.BulkUpsertContacts(context.Background(), "ws", []*domain.Contact{{
		Email:                 "a@example.com",
		TrackingOpensConsent:  &domain.NullableString{String: domain.TrackingConsentDenied},
		TrackingConsentSource: &domain.NullableString{String: domain.TrackingConsentSourceImport},
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContactInsertArgs_CarryTheConsentColumns(t *testing.T) {
	now := time.Now().UTC()
	args := contactInsertArgs(&domain.Contact{
		Email:                 "a@example.com",
		TrackingOpensConsent:  &domain.NullableString{String: domain.TrackingConsentGranted},
		TrackingConsentSource: &domain.NullableString{String: domain.TrackingConsentSourceAPI},
	}, now)
	require.Len(t, args, len(strings.Split(contactInsertColumns, ",")), "one value per insert column")
	require.Equal(t, contactInsertFieldCount, len(args), "one placeholder per value")
	tail := args[len(args)-4:]
	assert.Equal(t, sql.NullString{String: domain.TrackingConsentGranted, Valid: true}, tail[0])
	assert.Equal(t, sql.NullString{}, tail[1])
	assert.Equal(t, sql.NullTime{Time: now, Valid: true}, tail[2])
	assert.Equal(t, sql.NullString{String: domain.TrackingConsentSourceAPI, Valid: true}, tail[3])
}
