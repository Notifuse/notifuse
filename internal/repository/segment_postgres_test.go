package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper: creates a simple valid tree for testing
func validTestTree() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Table: "contacts",
			Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{
						FieldName:    "email",
						FieldType:    "string",
						Operator:     "equals",
						StringValues: []string{"test@example.com"},
					},
				},
			},
		},
	}
}

func setupSegmentRepositoryTest(t *testing.T) (domain.SegmentRepository, sqlmock.Sqlmock, *mocks.MockWorkspaceRepository) {
	ctrl := gomock.NewController(t)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	repo := NewSegmentRepository(mockWorkspaceRepo)

	return repo, nil, mockWorkspaceRepo
}

func createTestSegment() *domain.Segment {
	now := time.Now().UTC()
	sql := "SELECT email FROM contacts WHERE orders_count >= $1"
	return &domain.Segment{
		ID:            "seg123",
		Name:          "VIP Customers",
		Color:         "#FF5733",
		Tree:          validTestTree(),
		Timezone:      "America/New_York",
		Version:       1,
		Status:        string(domain.SegmentStatusBuilding),
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{5}, // Array of query arguments
		DBCreatedAt:   now,
		DBUpdatedAt:   now,
		UsersCount:    0,
	}
}

func TestSegmentRepository_CreateSegment(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)
		testSegment := createTestSegment()

		db, sqlMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), "workspace123").
			Return(db, nil)

		sqlMock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO segments (
			id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`)).WithArgs(
			testSegment.ID,
			testSegment.Name,
			testSegment.Color,
			sqlmock.AnyArg(), // tree JSONB
			testSegment.Timezone,
			testSegment.Version,
			testSegment.Status,
			testSegment.GeneratedSQL,
			sqlmock.AnyArg(), // generated_args JSONB
			sqlmock.AnyArg(), // recompute_after
			sqlmock.AnyArg(), // db_created_at
			sqlmock.AnyArg(), // db_updated_at
		).WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.CreateSegment(context.Background(), "workspace123", testSegment)
		require.NoError(t, err)
		assert.NotZero(t, testSegment.DBCreatedAt)
		assert.NotZero(t, testSegment.DBUpdatedAt)
	})

	t.Run("database error", func(t *testing.T) {
		repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)
		testSegment := createTestSegment()

		db, sqlMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), "workspace123").
			Return(db, nil)

		sqlMock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO segments (
			id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`)).WillReturnError(errors.New("database error"))

		err = repo.CreateSegment(context.Background(), "workspace123", testSegment)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create segment")
	})

	t.Run("workspace connection error", func(t *testing.T) {
		repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)
		testSegment := createTestSegment()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), "workspace123").
			Return(nil, errors.New("connection error"))

		err := repo.CreateSegment(context.Background(), "workspace123", testSegment)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})
}

func TestSegmentRepository_GetSegmentByID(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	testSegment := createTestSegment()

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("segment found", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "color", "tree", "timezone", "version", "status",
			"generated_sql", "generated_args", "recompute_after", "db_created_at", "db_updated_at", "users_count",
		}).AddRow(
			testSegment.ID,
			testSegment.Name,
			testSegment.Color,
			`{"operator":"and"}`,
			testSegment.Timezone,
			testSegment.Version,
			testSegment.Status,
			testSegment.GeneratedSQL,
			`[5]`,
			nil, // recompute_after
			testSegment.DBCreatedAt,
			testSegment.DBUpdatedAt,
			42, // users_count
		)

		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			COALESCE(COUNT(cs.email), 0) as users_count
		FROM segments s
		LEFT JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE s.id = $1
		GROUP BY s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at
	`)).WithArgs(testSegment.ID).WillReturnRows(rows)

		segment, err := repo.GetSegmentByID(context.Background(), "workspace123", testSegment.ID)
		require.NoError(t, err)
		assert.Equal(t, testSegment.ID, segment.ID)
		assert.Equal(t, testSegment.Name, segment.Name)
		assert.Equal(t, testSegment.Color, segment.Color)
		assert.Equal(t, testSegment.Timezone, segment.Timezone)
		assert.Equal(t, testSegment.Version, segment.Version)
		assert.Equal(t, 42, segment.UsersCount)
	})

	t.Run("segment not found", func(t *testing.T) {
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			COALESCE(COUNT(cs.email), 0) as users_count
		FROM segments s
		LEFT JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE s.id = $1
	`)).WithArgs("nonexistent").WillReturnError(sql.ErrNoRows)

		segment, err := repo.GetSegmentByID(context.Background(), "workspace123", "nonexistent")
		require.Error(t, err)
		assert.Nil(t, segment)
		assert.IsType(t, &domain.ErrSegmentNotFound{}, err)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			COALESCE(COUNT(cs.email), 0) as users_count
		FROM segments s
	`)).WillReturnError(errors.New("database error"))

		segment, err := repo.GetSegmentByID(context.Background(), "workspace123", testSegment.ID)
		require.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "failed to get segment")
	})
}

func TestSegmentRepository_GetSegments(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	testSegment := createTestSegment()

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("segments found with counts", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "color", "tree", "timezone", "version", "status",
			"generated_sql", "generated_args", "recompute_after", "db_created_at", "db_updated_at", "users_count",
		}).AddRow(
			testSegment.ID,
			testSegment.Name,
			testSegment.Color,
			`{"operator":"and"}`,
			testSegment.Timezone,
			testSegment.Version,
			testSegment.Status,
			testSegment.GeneratedSQL,
			`[5]`,
			nil, // recompute_after
			testSegment.DBCreatedAt,
			testSegment.DBUpdatedAt,
			42,
		).AddRow(
			"seg456",
			"Premium Users",
			"#00FF00",
			`{"operator":"or"}`,
			"Europe/Paris",
			1,
			"active",
			testSegment.GeneratedSQL,
			`[10]`,
			nil, // recompute_after
			testSegment.DBCreatedAt,
			testSegment.DBUpdatedAt,
			25,
		)

		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			COALESCE(COUNT(cs.email), 0) as users_count
		FROM segments s
		LEFT JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE s.status != 'deleted'
	`)).WillReturnRows(rows)

		segments, err := repo.GetSegments(context.Background(), "workspace123", true)
		require.NoError(t, err)
		assert.Len(t, segments, 2)
		assert.Equal(t, testSegment.ID, segments[0].ID)
		assert.Equal(t, 42, segments[0].UsersCount)
		assert.Equal(t, "seg456", segments[1].ID)
		assert.Equal(t, 25, segments[1].UsersCount)
	})

	t.Run("segments found without counts", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "color", "tree", "timezone", "version", "status",
			"generated_sql", "generated_args", "recompute_after", "db_created_at", "db_updated_at", "users_count",
		}).AddRow(
			testSegment.ID,
			testSegment.Name,
			testSegment.Color,
			`{"operator":"and"}`,
			testSegment.Timezone,
			testSegment.Version,
			testSegment.Status,
			testSegment.GeneratedSQL,
			`[5]`,
			nil, // recompute_after
			testSegment.DBCreatedAt,
			testSegment.DBUpdatedAt,
			0, // No count when withCount=false
		)

		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			0 as users_count
		FROM segments s
		WHERE s.status != 'deleted'
	`)).WillReturnRows(rows)

		segments, err := repo.GetSegments(context.Background(), "workspace123", false)
		require.NoError(t, err)
		assert.Len(t, segments, 1)
		assert.Equal(t, testSegment.ID, segments[0].ID)
		assert.Equal(t, 0, segments[0].UsersCount)
	})

	t.Run("no segments found", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "color", "tree", "timezone", "version", "status",
			"generated_sql", "generated_args", "db_created_at", "db_updated_at", "users_count",
		})

		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
	`)).WillReturnRows(rows)

		segments, err := repo.GetSegments(context.Background(), "workspace123", true)
		require.NoError(t, err)
		assert.Len(t, segments, 0)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
	`)).WillReturnError(errors.New("database error"))

		segments, err := repo.GetSegments(context.Background(), "workspace123", false)
		require.Error(t, err)
		assert.Nil(t, segments)
		assert.Contains(t, err.Error(), "failed to query segments")
	})
}

func TestSegmentRepository_UpdateSegment(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	testSegment := createTestSegment()
	testSegment.Status = string(domain.SegmentStatusActive)
	testSegment.Version = 2

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("successful update", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
		SET 
			name = $2,
			color = $3,
			tree = $4,
			timezone = $5,
			version = $6,
			status = $7,
			generated_sql = $8,
			generated_args = $9,
			recompute_after = $10,
			db_updated_at = $11
		WHERE id = $1
	`)).WithArgs(
			testSegment.ID,
			testSegment.Name,
			testSegment.Color,
			sqlmock.AnyArg(), // tree JSONB
			testSegment.Timezone,
			testSegment.Version,
			testSegment.Status,
			testSegment.GeneratedSQL,
			sqlmock.AnyArg(), // generated_args JSONB
			sqlmock.AnyArg(), // recompute_after
			sqlmock.AnyArg(), // db_updated_at
		).WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateSegment(context.Background(), "workspace123", testSegment)
		require.NoError(t, err)
	})

	t.Run("segment not found", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
		SET 
			name = $2,
	`)).WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateSegment(context.Background(), "workspace123", testSegment)
		require.Error(t, err)
		assert.IsType(t, &domain.ErrSegmentNotFound{}, err)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
	`)).WillReturnError(errors.New("database error"))

		err := repo.UpdateSegment(context.Background(), "workspace123", testSegment)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update segment")
	})
}

func TestSegmentRepository_DeleteSegment(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("successful deletion", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
		SET status = 'deleted', db_updated_at = $2
		WHERE id = $1
	`)).WithArgs("seg123", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))

		sqlMock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments WHERE segment_id = $1`)).
			WithArgs("seg123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.DeleteSegment(context.Background(), "workspace123", "seg123")
		require.NoError(t, err)
	})

	t.Run("segment not found", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
		SET status = 'deleted', db_updated_at = $2
		WHERE id = $1
	`)).WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.DeleteSegment(context.Background(), "workspace123", "nonexistent")
		require.Error(t, err)
		assert.IsType(t, &domain.ErrSegmentNotFound{}, err)
	})

	t.Run("database error on segment update", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
	`)).WillReturnError(errors.New("database error"))

		err := repo.DeleteSegment(context.Background(), "workspace123", "seg123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete segment")
	})

	t.Run("database error on contact_segments deletion", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE segments
		SET status = 'deleted', db_updated_at = $2
		WHERE id = $1
	`)).WithArgs("seg123", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))

		sqlMock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments WHERE segment_id = $1`)).
			WithArgs("seg123").
			WillReturnError(errors.New("database error"))

		err := repo.DeleteSegment(context.Background(), "workspace123", "seg123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete contact_segments for segment")
	})
}

func TestSegmentRepository_AddContactToSegment(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("successful addition", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email, segment_id)
		DO UPDATE SET version = $3, computed_at = $5
	`)).WithArgs(
			"test@example.com",
			"seg123",
			int64(1),
			sqlmock.AnyArg(), // matched_at
			sqlmock.AnyArg(), // computed_at
		).WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.AddContactToSegment(context.Background(), "workspace123", "test@example.com", "seg123", 1)
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO contact_segments
	`)).WillReturnError(errors.New("database error"))

		err := repo.AddContactToSegment(context.Background(), "workspace123", "test@example.com", "seg123", 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add contact to segment")
	})
}

func TestSegmentRepository_RemoveContactFromSegment(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("successful removal", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments WHERE email = $1 AND segment_id = $2`)).
			WithArgs("test@example.com", "seg123").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.RemoveContactFromSegment(context.Background(), "workspace123", "test@example.com", "seg123")
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments`)).
			WillReturnError(errors.New("database error"))

		err := repo.RemoveContactFromSegment(context.Background(), "workspace123", "test@example.com", "seg123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove contact from segment")
	})
}

func TestSegmentRepository_RemoveOldMemberships(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("successful cleanup", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments WHERE segment_id = $1 AND version < $2`)).
			WithArgs("seg123", int64(2)).
			WillReturnResult(sqlmock.NewResult(0, 5))

		err := repo.RemoveOldMemberships(context.Background(), "workspace123", "seg123", 2)
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectExec(regexp.QuoteMeta(`DELETE FROM contact_segments WHERE segment_id = $1 AND version < $2`)).
			WillReturnError(errors.New("database error"))

		err := repo.RemoveOldMemberships(context.Background(), "workspace123", "seg123", 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove old memberships")
	})
}

func TestSegmentRepository_GetContactSegments(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	testSegment := createTestSegment()

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("segments found for contact", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "color", "tree", "timezone", "version", "status",
			"generated_sql", "generated_args", "recompute_after", "db_created_at", "db_updated_at", "users_count",
		}).AddRow(
			testSegment.ID,
			testSegment.Name,
			testSegment.Color,
			`{"operator":"and"}`,
			testSegment.Timezone,
			testSegment.Version,
			"active",
			testSegment.GeneratedSQL,
			`[5]`,
			nil, // recompute_after
			testSegment.DBCreatedAt,
			testSegment.DBUpdatedAt,
			0,
		)

		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			0 as users_count
		FROM segments s
		INNER JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE cs.email = $1 AND s.status = 'active'
		ORDER BY s.name ASC
	`)).WithArgs("test@example.com").WillReturnRows(rows)

		segments, err := repo.GetContactSegments(context.Background(), "workspace123", "test@example.com")
		require.NoError(t, err)
		assert.Len(t, segments, 1)
		assert.Equal(t, testSegment.ID, segments[0].ID)
		assert.Equal(t, testSegment.Name, segments[0].Name)
	})

	t.Run("no segments found", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "color", "tree", "timezone", "version", "status",
			"generated_sql", "generated_args", "recompute_after", "db_created_at", "db_updated_at", "users_count",
		})

		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			0 as users_count
		FROM segments s
		INNER JOIN contact_segments cs ON s.id = cs.segment_id AND s.version = cs.version
		WHERE cs.email = $1 AND s.status = 'active'
		ORDER BY s.name ASC
	`)).WillReturnRows(rows)

		segments, err := repo.GetContactSegments(context.Background(), "workspace123", "test@example.com")
		require.NoError(t, err)
		assert.Len(t, segments, 0)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT 
			s.id, s.name, s.color, s.tree, s.timezone, s.version, s.status,
			s.generated_sql, s.generated_args, s.recompute_after, s.db_created_at, s.db_updated_at,
			0 as users_count
		FROM segments s
	`)).WillReturnError(errors.New("database error"))

		segments, err := repo.GetContactSegments(context.Background(), "workspace123", "test@example.com")
		require.Error(t, err)
		assert.Nil(t, segments)
		assert.Contains(t, err.Error(), "failed to query contact segments")
	})
}

func TestSegmentRepository_GetSegmentContactCount(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("count returned", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(42)

		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM contact_segments WHERE segment_id = $1`)).
			WithArgs("seg123").
			WillReturnRows(rows)

		count, err := repo.GetSegmentContactCount(context.Background(), "workspace123", "seg123")
		require.NoError(t, err)
		assert.Equal(t, 42, count)
	})

	t.Run("zero count", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(0)

		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM contact_segments WHERE segment_id = $1`)).
			WithArgs("seg123").
			WillReturnRows(rows)

		count, err := repo.GetSegmentContactCount(context.Background(), "workspace123", "seg123")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("database error", func(t *testing.T) {
		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM contact_segments`)).
			WillReturnError(errors.New("database error"))

		count, err := repo.GetSegmentContactCount(context.Background(), "workspace123", "seg123")
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to get segment contact count")
	})
}

func TestSegmentRepository_PreviewSegment(t *testing.T) {
	repo, _, mockWorkspaceRepo := setupSegmentRepositoryTest(t)

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("successful preview with count", func(t *testing.T) {
		testQuery := "SELECT email FROM contacts WHERE status = $1"
		testArgs := []interface{}{"active"}
		limit := 10

		countRows := sqlmock.NewRows([]string{"count"}).AddRow(150)
		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM (SELECT email FROM contacts WHERE status = $1) AS segment_results`)).
			WithArgs("active").
			WillReturnRows(countRows)

		count, err := repo.PreviewSegment(context.Background(), "workspace123", testQuery, testArgs, limit)
		require.NoError(t, err)
		assert.Equal(t, 150, count)
	})

	t.Run("successful preview with zero count", func(t *testing.T) {
		testQuery := "SELECT email FROM contacts WHERE status = $1"
		testArgs := []interface{}{"inactive"}
		limit := 10

		countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM (SELECT email FROM contacts WHERE status = $1) AS segment_results`)).
			WithArgs("inactive").
			WillReturnRows(countRows)

		count, err := repo.PreviewSegment(context.Background(), "workspace123", testQuery, testArgs, limit)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		repo2, _, mockWorkspaceRepo2 := setupSegmentRepositoryTest(t)

		mockWorkspaceRepo2.EXPECT().
			GetConnection(gomock.Any(), "workspace123").
			Return(nil, errors.New("connection error"))

		testQuery := "SELECT email FROM contacts"
		testArgs := []interface{}{}
		limit := 10

		count, err := repo2.PreviewSegment(context.Background(), "workspace123", testQuery, testArgs, limit)
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("database query error", func(t *testing.T) {
		testQuery := "SELECT email FROM contacts WHERE status = $1"
		testArgs := []interface{}{"active"}
		limit := 10

		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM (SELECT email FROM contacts WHERE status = $1) AS segment_results`)).
			WithArgs("active").
			WillReturnError(errors.New("database error"))

		count, err := repo.PreviewSegment(context.Background(), "workspace123", testQuery, testArgs, limit)
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to execute preview count query")
	})
}
