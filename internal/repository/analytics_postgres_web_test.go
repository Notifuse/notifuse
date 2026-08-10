package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func webAnalyticsWorkspace(bounceSeconds int) *domain.Workspace {
	return &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			WebAnalytics: &domain.WebAnalyticsSettings{
				Enabled:                true,
				BounceThresholdSeconds: bounceSeconds,
			},
		},
	}
}

func TestAnalyticsRepository_WebSchemas(t *testing.T) {
	t.Run("GetSchemas includes the per-workspace web schemas", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
			Return(webAnalyticsWorkspace(25), nil)

		repo := NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "")
		schemas, err := repo.GetSchemas(context.Background(), "workspace-123")
		require.NoError(t, err)
		assert.Contains(t, schemas, "web_sessions")
		assert.Contains(t, schemas, "web_pages")
		assert.Contains(t, schemas, "web_goals")
		assert.Contains(t, schemas, "message_history")
		assert.Contains(t, schemas["web_sessions"].Measures["bounce_rate"].SQL, "duration_ms < 25000")
	})

	t.Run("Query against a web schema reflects the workspace bounce threshold", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
			Return(webAnalyticsWorkspace(25), nil)
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)

		sqlMock.ExpectQuery(`SELECT .+duration_ms < 25000.+ FROM web_sessions`).
			WillReturnRows(sqlmock.NewRows([]string{"bounce_rate"}).AddRow(42.5))

		repo := NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "")
		response, err := repo.Query(context.Background(), "workspace-123", analytics.Query{
			Schema:   "web_sessions",
			Measures: []string{"bounce_rate"},
		})
		require.NoError(t, err)
		assert.Len(t, response.Data, 1)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("web schemas absent when the workspace has no settings", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
			Return(&domain.Workspace{ID: "workspace-123"}, nil)

		repo := NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "")
		schemas, err := repo.GetSchemas(context.Background(), "workspace-123")
		require.NoError(t, err)
		assert.NotContains(t, schemas, "web_sessions")
	})
}

func TestAnalyticsRepository_WorkMem(t *testing.T) {
	t.Run("configured work_mem wraps the query in a SET LOCAL transaction", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
			Return(webAnalyticsWorkspace(10), nil)
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)

		sqlMock.ExpectBegin()
		sqlMock.ExpectExec(`SET LOCAL work_mem = '64MB'`).WillReturnResult(sqlmock.NewResult(0, 0))
		sqlMock.ExpectQuery(`SELECT .+ FROM web_sessions`).
			WillReturnRows(sqlmock.NewRows([]string{"sessions"}).AddRow(7))
		sqlMock.ExpectCommit()

		repo := NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "64MB")
		_, err = repo.Query(context.Background(), "workspace-123", analytics.Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
		})
		require.NoError(t, err)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("invalid work_mem values are ignored", func(t *testing.T) {
		db, sqlMock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").
			Return(webAnalyticsWorkspace(10), nil)
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace-123").Return(db, nil)

		// No ExpectBegin: the query must run without a transaction.
		sqlMock.ExpectQuery(`SELECT .+ FROM web_sessions`).
			WillReturnRows(sqlmock.NewRows([]string{"sessions"}).AddRow(7))

		repo := NewAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger(), "64MB; DROP TABLE x")
		_, err = repo.Query(context.Background(), "workspace-123", analytics.Query{
			Schema:   "web_sessions",
			Measures: []string{"sessions"},
		})
		require.NoError(t, err)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})
}
