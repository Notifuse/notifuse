package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactTimelineRepository_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactTimelineRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"
	email := "user@example.com"
	limit := 50

	t.Run("Success - List timeline entries", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		now := time.Now()
		dbNow := time.Now()
		entityData1 := []byte(`{"email":"user@example.com","first_name":"John","last_name":"Doe"}`)
		entityData2 := []byte(`{"email":"user@example.com","first_name":"Jane","last_name":"Doe"}`)

		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"}).
			AddRow("entry1", email, "insert", "contact", "insert_contact", nil, nil, now, dbNow, entityData1).
			AddRow("entry2", email, "update", "contact", "update_contact", []byte(`{"first_name":{"old":"John","new":"Jane"}}`), nil, now.Add(-1*time.Hour), dbNow.Add(-1*time.Hour), entityData2)

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.Nil(t, nextCursor)
		assert.Equal(t, "entry1", entries[0].ID)
		assert.Equal(t, "insert", entries[0].Operation)
		assert.Nil(t, entries[0].Changes)
		assert.NotNil(t, entries[0].EntityData)
		assert.Equal(t, "John", entries[0].EntityData["first_name"])
		assert.Equal(t, "entry2", entries[1].ID)
		assert.Equal(t, "update", entries[1].Operation)
		assert.NotNil(t, entries[1].Changes)
		assert.NotNil(t, entries[1].EntityData)
		assert.Equal(t, "Jane", entries[1].EntityData["first_name"])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - List with cursor pagination", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		cursorTime := time.Now().Add(-1 * time.Hour)
		cursorID := "entry1"
		cursorStr := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%s", cursorTime.Format(time.RFC3339Nano), cursorID)))

		entityData := []byte(`{"email":"user@example.com","first_name":"Jane","last_name":"Doe"}`)
		dbNow := time.Now()
		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"}).
			AddRow("entry2", email, "update", "contact", "update_contact", []byte(`{"first_name":{"old":"John","new":"Jane"}}`), nil, cursorTime.Add(-1*time.Minute), dbNow, entityData)

		// The query uses email, cursorTime (twice for the OR condition), cursorID, and limit
		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg(), sqlmock.AnyArg(), cursorID, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, &cursorStr)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Nil(t, nextCursor)
		assert.Equal(t, "entry2", entries[0].ID)
		assert.NotNil(t, entries[0].EntityData)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - List with next cursor", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		now := time.Now()
		dbNow := time.Now()
		entityData := []byte(`{"email":"user@example.com","first_name":"Test","last_name":"User"}`)
		// Return limit + 1 entries to trigger next cursor
		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"})
		for i := 0; i <= 10; i++ {
			rows.AddRow(fmt.Sprintf("entry%d", i), email, "insert", "contact", "insert_contact", nil, nil, now.Add(-time.Duration(i)*time.Hour), dbNow.Add(-time.Duration(i)*time.Hour), entityData)
		}

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, 10, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 10) // Should return only 10, not 11
		assert.NotNil(t, nextCursor)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - List with entity_id", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		listID := "list123"
		now := time.Now()
		dbNow := time.Now()
		entityData := []byte(`{"id":"list123","name":"Test List","status":"active"}`)
		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"}).
			AddRow("entry1", email, "insert", "contact_list", "insert_contact_list", []byte(`{"list_id":{"new":"list123"},"status":{"new":"active"}}`), listID, now, dbNow, entityData)

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Nil(t, nextCursor)
		assert.Equal(t, "contact_list", entries[0].EntityType)
		assert.NotNil(t, entries[0].EntityID)
		assert.Equal(t, "list123", *entries[0].EntityID)
		assert.NotNil(t, entries[0].EntityData)
		assert.Equal(t, "Test List", entries[0].EntityData["name"])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Empty result", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"})

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		require.NoError(t, err)
		assert.Empty(t, entries)
		assert.Nil(t, nextCursor)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Applies default limit", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"})

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		// Pass 0 limit, should default to 50
		entries, nextCursor, err := repo.List(ctx, workspaceID, email, 0, nil)

		require.NoError(t, err)
		assert.Empty(t, entries)
		assert.Nil(t, nextCursor)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Caps limit at 100", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"})

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		// Pass 150 limit, should be capped at 100
		entries, nextCursor, err := repo.List(ctx, workspaceID, email, 150, nil)

		require.NoError(t, err)
		assert.Empty(t, entries)
		assert.Nil(t, nextCursor)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Failed to get workspace connection", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, assert.AnError)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Error - Invalid cursor", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		invalidCursor := "invalid-cursor!!!"

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, &invalidCursor)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "invalid cursor")
	})

	t.Run("Error - Invalid cursor format", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Valid base64 but wrong format (missing pipe separator)
		invalidCursor := base64.StdEncoding.EncodeToString([]byte("no-pipe-separator"))

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, &invalidCursor)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "invalid cursor format")
	})

	t.Run("Error - Query execution failed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnError(sql.ErrConnDone)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "failed to query timeline")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Row scan failed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Return wrong number of columns
		rows := sqlmock.NewRows([]string{"id", "email"}).
			AddRow("entry1", email)

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "failed to scan timeline entry")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Invalid JSON in changes field", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		now := time.Now()
		dbNow := time.Now()
		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"}).
			AddRow("entry1", email, "update", "contact", "update_contact", []byte(`{invalid json}`), nil, now, dbNow, nil)

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "failed to parse changes JSON")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Invalid JSON in entity_data field", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		now := time.Now()
		dbNow := time.Now()
		rows := sqlmock.NewRows([]string{"id", "email", "operation", "entity_type", "kind", "changes", "entity_id", "created_at", "db_created_at", "entity_data"}).
			AddRow("entry1", email, "insert", "contact", "insert_contact", nil, nil, now, dbNow, []byte(`{invalid json}`))

		mock.ExpectQuery("SELECT(.+)FROM contact_timeline ct(.+)").
			WithArgs(email, sqlmock.AnyArg()).
			WillReturnRows(rows)

		entries, nextCursor, err := repo.List(ctx, workspaceID, email, limit, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
		assert.Contains(t, err.Error(), "failed to parse entity data JSON")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestNewContactTimelineRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	repo := NewContactTimelineRepository(mockWorkspaceRepo)

	assert.NotNil(t, repo)
	assert.IsType(t, &ContactTimelineRepository{}, repo)
}

func TestEncodeCursor(t *testing.T) {
	timestamp := time.Date(2025, 10, 2, 12, 0, 0, 0, time.UTC)
	id := "entry123"

	cursor := encodeCursor(timestamp, id)

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	require.NoError(t, err)

	expected := fmt.Sprintf("%s|%s", timestamp.Format(time.RFC3339Nano), id)
	assert.Equal(t, expected, string(decoded))
}

func TestDecodeCursor(t *testing.T) {
	t.Run("Success - Valid cursor", func(t *testing.T) {
		timestamp := time.Date(2025, 10, 2, 12, 0, 0, 0, time.UTC)
		id := "entry123"
		cursorData := fmt.Sprintf("%s|%s", timestamp.Format(time.RFC3339Nano), id)
		cursor := base64.StdEncoding.EncodeToString([]byte(cursorData))

		decoded, err := decodeCursor(cursor)

		require.NoError(t, err)
		assert.Equal(t, cursorData, decoded)
	})

	t.Run("Error - Invalid base64", func(t *testing.T) {
		invalidCursor := "not-valid-base64!!!"

		decoded, err := decodeCursor(invalidCursor)

		assert.Error(t, err)
		assert.Empty(t, decoded)
	})
}

func TestParseJSON(t *testing.T) {
	t.Run("Success - Valid JSON", func(t *testing.T) {
		jsonData := []byte(`{"field":"value"}`)
		var result map[string]interface{}

		err := parseJSON(jsonData, &result)

		require.NoError(t, err)
		assert.Equal(t, "value", result["field"])
	})

	t.Run("Success - Empty data", func(t *testing.T) {
		var result map[string]interface{}

		err := parseJSON([]byte{}, &result)

		require.NoError(t, err)
	})

	t.Run("Error - Invalid JSON", func(t *testing.T) {
		jsonData := []byte(`{invalid}`)
		var result map[string]interface{}

		err := parseJSON(jsonData, &result)

		assert.Error(t, err)
	})
}

func TestContactTimelineRepository_DeleteForEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactTimelineRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws123"
	email := "user@example.com"

	t.Run("Success - Delete timeline entries", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec("DELETE FROM contact_timeline").
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 5)) // 5 rows deleted

		err = repo.DeleteForEmail(ctx, workspaceID, email)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - No entries to delete", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec("DELETE FROM contact_timeline").
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows deleted

		err = repo.DeleteForEmail(ctx, workspaceID, email)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Failed to get workspace connection", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, assert.AnError)

		err := repo.DeleteForEmail(ctx, workspaceID, email)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Error - Delete query failed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec("DELETE FROM contact_timeline").
			WithArgs(email).
			WillReturnError(sql.ErrConnDone)

		err = repo.DeleteForEmail(ctx, workspaceID, email)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete timeline entries")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
