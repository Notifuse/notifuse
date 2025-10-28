package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
	"github.com/Notifuse/notifuse/pkg/crypto"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
)

// mockConnectionManager is a simple mock for testing
type mockConnectionManager struct {
	workspaceDBs map[string]*sql.DB
	systemDB     *sql.DB
}

func newMockConnectionManager(systemDB *sql.DB) *mockConnectionManager {
	return &mockConnectionManager{
		workspaceDBs: make(map[string]*sql.DB),
		systemDB:     systemDB,
	}
}

func (m *mockConnectionManager) GetSystemConnection() *sql.DB {
	return m.systemDB
}

func (m *mockConnectionManager) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if db, ok := m.workspaceDBs[workspaceID]; ok {
		return db, nil
	}
	// For tests, return the system DB as a fallback
	return m.systemDB, nil
}

func (m *mockConnectionManager) CloseWorkspaceConnection(workspaceID string) error {
	delete(m.workspaceDBs, workspaceID)
	return nil
}

func (m *mockConnectionManager) GetStats() pkgDatabase.ConnectionStats {
	return pkgDatabase.ConnectionStats{}
}

func (m *mockConnectionManager) Close() error {
	return nil
}

func (m *mockConnectionManager) AddWorkspaceDB(workspaceID string, db *sql.DB) {
	m.workspaceDBs[workspaceID] = db
}

func TestWorkspaceRepository_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWorkspaceRepository(ctrl)

	t.Run("successful retrieval", func(t *testing.T) {
		workspaceID := "testworkspace"
		workspaceName := "Test Workspace"
		createdAt := time.Now()
		updatedAt := time.Now()

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: workspaceName,
			Settings: domain.WorkspaceSettings{
				Timezone:  "UTC",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Integrations: []domain.Integration{
				{
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindMailgun,
					},
				},
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}

		repo.EXPECT().GetByID(context.Background(), workspaceID).Return(expectedWorkspace, nil)

		workspace, err := repo.GetByID(context.Background(), workspaceID)
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
		assert.Equal(t, workspaceName, workspace.Name)
		assert.Equal(t, "UTC", workspace.Settings.Timezone)
		assert.Equal(t, createdAt.Unix(), workspace.CreatedAt.Unix())
		assert.Equal(t, updatedAt.Unix(), workspace.UpdatedAt.Unix())
		assert.NotNil(t, workspace.Integrations)
		assert.Len(t, workspace.Integrations, 1)
	})

	t.Run("workspace not found", func(t *testing.T) {
		repo.EXPECT().GetByID(context.Background(), "nonexistent").Return(nil, fmt.Errorf("workspace not found"))

		workspace, err := repo.GetByID(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Nil(t, workspace)
	})

	t.Run("database connection error", func(t *testing.T) {
		repo.EXPECT().GetByID(context.Background(), "testworkspace").Return(nil, fmt.Errorf("connection refused"))

		workspace, err := repo.GetByID(context.Background(), "testworkspace")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Nil(t, workspace)
	})

	t.Run("empty workspace ID", func(t *testing.T) {
		repo.EXPECT().GetByID(context.Background(), "").Return(nil, fmt.Errorf("workspace not found"))

		workspace, err := repo.GetByID(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Nil(t, workspace)
	})

	t.Run("workspace with minimal settings", func(t *testing.T) {
		workspaceID := "minimal-workspace"
		workspaceName := "Minimal Workspace"
		createdAt := time.Now()
		updatedAt := time.Now()

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: workspaceName,
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
			},
			Integrations: []domain.Integration{},
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}

		repo.EXPECT().GetByID(context.Background(), workspaceID).Return(expectedWorkspace, nil)

		workspace, err := repo.GetByID(context.Background(), workspaceID)
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
		assert.Equal(t, workspaceName, workspace.Name)
		assert.Equal(t, "UTC", workspace.Settings.Timezone)
		assert.Empty(t, workspace.Integrations)
	})

	t.Run("workspace with null integrations", func(t *testing.T) {
		workspaceID := "null-integrations-workspace"
		workspaceName := "Null Integrations Workspace"
		createdAt := time.Now()
		updatedAt := time.Now()

		expectedWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: workspaceName,
			Settings: domain.WorkspaceSettings{
				Timezone:  "UTC",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Integrations: []domain.Integration{},
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}

		repo.EXPECT().GetByID(context.Background(), workspaceID).Return(expectedWorkspace, nil)

		workspace, err := repo.GetByID(context.Background(), workspaceID)
		require.NoError(t, err)
		assert.Equal(t, workspaceID, workspace.ID)
		assert.Equal(t, workspaceName, workspace.Name)
		assert.Equal(t, "UTC", workspace.Settings.Timezone)
		assert.Empty(t, workspace.Integrations)
	})
}

func TestWorkspaceRepository_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWorkspaceRepository(ctrl)

	t.Run("successful retrieval with multiple workspaces", func(t *testing.T) {
		workspace1CreatedAt := time.Now().Add(-2 * time.Hour)
		workspace1UpdatedAt := time.Now().Add(-2 * time.Hour)
		workspace2CreatedAt := time.Now().Add(-1 * time.Hour)
		workspace2UpdatedAt := time.Now().Add(-1 * time.Hour)

		expectedWorkspaces := []*domain.Workspace{
			{
				ID:   "workspace2",
				Name: "Workspace 2",
				Settings: domain.WorkspaceSettings{
					Timezone:  "Europe/London",
					SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
				Integrations: []domain.Integration{},
				CreatedAt:    workspace2CreatedAt,
				UpdatedAt:    workspace2UpdatedAt,
			},
			{
				ID:   "workspace1",
				Name: "Workspace 1",
				Settings: domain.WorkspaceSettings{
					Timezone:  "UTC",
					SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
				Integrations: []domain.Integration{
					{
						Type: domain.IntegrationTypeEmail,
						EmailProvider: domain.EmailProvider{
							Kind: domain.EmailProviderKindMailgun,
						},
					},
				},
				CreatedAt: workspace1CreatedAt,
				UpdatedAt: workspace1UpdatedAt,
			},
		}

		repo.EXPECT().List(context.Background()).Return(expectedWorkspaces, nil)

		workspaces, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, len(workspaces))

		// Verify order (newest first)
		assert.Equal(t, "workspace2", workspaces[0].ID)
		assert.Equal(t, "Workspace 2", workspaces[0].Name)
		assert.Equal(t, "Europe/London", workspaces[0].Settings.Timezone)

		assert.Equal(t, "workspace1", workspaces[1].ID)
		assert.Equal(t, "Workspace 1", workspaces[1].Name)
		assert.Equal(t, "UTC", workspaces[1].Settings.Timezone)
		assert.Len(t, workspaces[1].Integrations, 1)
	})

	t.Run("empty result set", func(t *testing.T) {
		repo.EXPECT().List(context.Background()).Return([]*domain.Workspace{}, nil)

		workspaces, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, workspaces)
	})

	t.Run("single workspace", func(t *testing.T) {
		createdAt := time.Now()
		updatedAt := time.Now()

		expectedWorkspaces := []*domain.Workspace{
			{
				ID:   "single-workspace",
				Name: "Single Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:  "America/New_York",
					SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
				Integrations: []domain.Integration{
					{
						Type: domain.IntegrationTypeEmail,
						EmailProvider: domain.EmailProvider{
							Kind: domain.EmailProviderKindSMTP,
						},
					},
				},
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
		}

		repo.EXPECT().List(context.Background()).Return(expectedWorkspaces, nil)

		workspaces, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, len(workspaces))
		assert.Equal(t, "single-workspace", workspaces[0].ID)
		assert.Equal(t, "Single Workspace", workspaces[0].Name)
		assert.Equal(t, "America/New_York", workspaces[0].Settings.Timezone)
		assert.Len(t, workspaces[0].Integrations, 1)
	})

	t.Run("database connection error", func(t *testing.T) {
		repo.EXPECT().List(context.Background()).Return(nil, fmt.Errorf("connection timeout"))

		workspaces, err := repo.List(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection timeout")
		assert.Nil(t, workspaces)
	})

	t.Run("workspaces with various configurations", func(t *testing.T) {
		workspace1CreatedAt := time.Now().Add(-2 * time.Hour)
		workspace1UpdatedAt := time.Now().Add(-2 * time.Hour)
		workspace2CreatedAt := time.Now().Add(-1 * time.Hour)
		workspace2UpdatedAt := time.Now().Add(-1 * time.Hour)

		expectedWorkspaces := []*domain.Workspace{
			{
				ID:   "full-workspace",
				Name: "Full Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone:  "Asia/Tokyo",
					SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
				Integrations: []domain.Integration{
					{
						Type: domain.IntegrationTypeEmail,
						EmailProvider: domain.EmailProvider{
							Kind: domain.EmailProviderKindMailgun,
						},
					},
					{
						Type: domain.IntegrationTypeEmail,
						EmailProvider: domain.EmailProvider{
							Kind: domain.EmailProviderKindSMTP,
						},
					},
				},
				CreatedAt: workspace2CreatedAt,
				UpdatedAt: workspace2UpdatedAt,
			},
			{
				ID:   "minimal-workspace",
				Name: "Minimal Workspace",
				Settings: domain.WorkspaceSettings{
					Timezone: "UTC",
				},
				Integrations: []domain.Integration{},
				CreatedAt:    workspace1CreatedAt,
				UpdatedAt:    workspace1UpdatedAt,
			},
		}

		repo.EXPECT().List(context.Background()).Return(expectedWorkspaces, nil)

		workspaces, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, len(workspaces))

		// Verify full workspace
		assert.Equal(t, "full-workspace", workspaces[0].ID)
		assert.Equal(t, "Asia/Tokyo", workspaces[0].Settings.Timezone)
		assert.Len(t, workspaces[0].Integrations, 2)

		// Verify minimal workspace
		assert.Equal(t, "minimal-workspace", workspaces[1].ID)
		assert.Equal(t, "UTC", workspaces[1].Settings.Timezone)
		assert.Empty(t, workspaces[1].Integrations)
	})
}

func TestWorkspaceRepository_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWorkspaceRepository(ctrl)

	t.Run("successful creation", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:   "test-workspace",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:  "UTC",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}

		repo.EXPECT().Create(context.Background(), workspace).Return(nil)

		err := repo.Create(context.Background(), workspace)
		require.NoError(t, err)
	})

	t.Run("workspace ID already exists", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:   "existing-workspace",
			Name: "Existing Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:  "UTC",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}

		repo.EXPECT().Create(context.Background(), workspace).Return(fmt.Errorf("workspace with ID existing-workspace already exists"))

		err := repo.Create(context.Background(), workspace)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("database error during creation", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:   "test-workspace",
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:  "UTC",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}

		repo.EXPECT().Create(context.Background(), workspace).Return(fmt.Errorf("failed to create workspace: database error"))

		err := repo.Create(context.Background(), workspace)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace")
	})
}

func TestWorkspaceRepository_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWorkspaceRepository(ctrl)

	t.Run("successful update", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:   "workspace1",
			Name: "Updated Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:  "America/New_York",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Integrations: []domain.Integration{
				{
					ID:   "integration-1",
					Name: "SMTP Integration",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind: domain.EmailProviderKindSMTP,
					},
				},
			},
		}

		repo.EXPECT().Update(context.Background(), workspace).Return(nil)

		err := repo.Update(context.Background(), workspace)
		require.NoError(t, err)
	})

	t.Run("workspace not found", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:   "nonexistent-workspace",
			Name: "Nonexistent Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:  "UTC",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}

		repo.EXPECT().Update(context.Background(), workspace).Return(fmt.Errorf("workspace with ID nonexistent-workspace not found"))

		err := repo.Update(context.Background(), workspace)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("database connection error", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:   "workspace1",
			Name: "Updated Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:  "Europe/Paris",
				SecretKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}

		repo.EXPECT().Update(context.Background(), workspace).Return(fmt.Errorf("failed to update workspace: connection lost"))

		err := repo.Update(context.Background(), workspace)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection lost")
	})
}

func TestWorkspaceRepository_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockWorkspaceRepository(ctrl)

	t.Run("successful deletion", func(t *testing.T) {
		workspaceID := "test-workspace"

		repo.EXPECT().Delete(context.Background(), workspaceID).Return(nil)

		err := repo.Delete(context.Background(), workspaceID)
		require.NoError(t, err)
	})

	t.Run("workspace not found", func(t *testing.T) {
		workspaceID := "nonexistent-workspace"

		repo.EXPECT().Delete(context.Background(), workspaceID).Return(fmt.Errorf("workspace not found"))

		err := repo.Delete(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace not found")
	})

	t.Run("database error during deletion", func(t *testing.T) {
		workspaceID := "error-workspace"

		repo.EXPECT().Delete(context.Background(), workspaceID).Return(fmt.Errorf("database error"))

		err := repo.Delete(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("permission denied during database deletion", func(t *testing.T) {
		workspaceID := "permission-denied-workspace"

		repo.EXPECT().Delete(context.Background(), workspaceID).Return(fmt.Errorf("permission denied"))

		err := repo.Delete(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}

// Tests below exercise the real repository implementation against a mocked SQL database

func TestWorkspaceRepository_GetByID_Postgres(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{Prefix: "notifuse"}
	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)

	workspaceID := "ws_123"
	createdAt := time.Now().Truncate(time.Second)
	updatedAt := createdAt

	enc, err := crypto.EncryptString("mysecret", "secret-key")
	require.NoError(t, err)

	settings := domain.WorkspaceSettings{Timezone: "UTC", EncryptedSecretKey: enc}
	settingsJSON, err := json.Marshal(settings)
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
		AddRow(workspaceID, "Test WS", settingsJSON, nil, createdAt, updatedAt)

	mock.ExpectQuery(`SELECT id, name, settings, integrations, created_at, updated_at\s+FROM workspaces\s+WHERE id = \$1`).
		WithArgs(workspaceID).
		WillReturnRows(rows)

	w, err := repo.GetByID(context.Background(), workspaceID)
	require.NoError(t, err)
	assert.Equal(t, workspaceID, w.ID)
	assert.Equal(t, "Test WS", w.Name)
	assert.Equal(t, "UTC", w.Settings.Timezone)
	assert.Equal(t, "mysecret", w.Settings.SecretKey)

	// not found
	mock.ExpectQuery(`SELECT id, name, settings, integrations, created_at, updated_at\s+FROM workspaces\s+WHERE id = \$1`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByID(context.Background(), "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// db error
	mock.ExpectQuery(`SELECT id, name, settings, integrations, created_at, updated_at\s+FROM workspaces\s+WHERE id = \$1`).
		WithArgs("boom").
		WillReturnError(fmt.Errorf("db error"))

	_, err = repo.GetByID(context.Background(), "boom")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestWorkspaceRepository_List_Postgres(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{Prefix: "notifuse"}
	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)

	enc1, err := crypto.EncryptString("s1", "secret-key")
	require.NoError(t, err)
	enc2, err := crypto.EncryptString("s2", "secret-key")
	require.NoError(t, err)

	s1, _ := json.Marshal(domain.WorkspaceSettings{Timezone: "Europe/London", EncryptedSecretKey: enc1})
	s2, _ := json.Marshal(domain.WorkspaceSettings{Timezone: "UTC", EncryptedSecretKey: enc2})

	newer := time.Now().Truncate(time.Second)
	older := newer.Add(-time.Hour)

	rows := sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"}).
		AddRow("w2", "Workspace 2", s1, nil, newer, newer).
		AddRow("w1", "Workspace 1", s2, nil, older, older)

	mock.ExpectQuery(`SELECT id, name, settings, integrations, created_at, updated_at\s+FROM workspaces\s+ORDER BY created_at DESC`).
		WillReturnRows(rows)

	list, err := repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "w2", list[0].ID)
	assert.Equal(t, "s1", list[0].Settings.SecretKey)
	assert.Equal(t, "w1", list[1].ID)
	assert.Equal(t, "s2", list[1].Settings.SecretKey)

	// empty result
	emptyRows := sqlmock.NewRows([]string{"id", "name", "settings", "integrations", "created_at", "updated_at"})
	mock.ExpectQuery(`SELECT id, name, settings, integrations, created_at, updated_at\s+FROM workspaces\s+ORDER BY created_at DESC`).
		WillReturnRows(emptyRows)

	list, err = repo.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)

	// db error
	mock.ExpectQuery(`SELECT id, name, settings, integrations, created_at, updated_at\s+FROM workspaces\s+ORDER BY created_at DESC`).
		WillReturnError(fmt.Errorf("db err"))

	_, err = repo.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db err")
}

func TestWorkspaceRepository_Update_Postgres(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{Prefix: "notifuse"}
	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)

	w := &domain.Workspace{
		ID:   "ws1",
		Name: "Updated Name",
		Settings: domain.WorkspaceSettings{
			Timezone:  "UTC",
			SecretKey: "supersecret",
		},
		Integrations: []domain.Integration{},
	}

	mock.ExpectExec(`UPDATE workspaces\s+SET name = \$1, settings = \$2, integrations = \$3, updated_at = \$4\s+WHERE id = \$5`).
		WithArgs(w.Name, sqlmock.AnyArg(), nil, sqlmock.AnyArg(), w.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Update(context.Background(), w)
	require.NoError(t, err)
	assert.Equal(t, "supersecret", w.Settings.SecretKey)

	// not found (0 rows affected)
	mock.ExpectExec(`UPDATE workspaces\s+SET name = \$1, settings = \$2, integrations = \$3, updated_at = \$4\s+WHERE id = \$5`).
		WithArgs(w.Name, sqlmock.AnyArg(), nil, sqlmock.AnyArg(), "missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	wMissing := *w
	wMissing.ID = "missing"
	err = repo.Update(context.Background(), &wMissing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// db error during update
	mock.ExpectExec(`UPDATE workspaces\s+SET name = \$1, settings = \$2, integrations = \$3, updated_at = \$4\s+WHERE id = \$5`).
		WithArgs(w.Name, sqlmock.AnyArg(), nil, sqlmock.AnyArg(), w.ID).
		WillReturnError(fmt.Errorf("update failed"))

	err = repo.Update(context.Background(), w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")

	// error getting affected rows
	mock.ExpectExec(`UPDATE workspaces\s+SET name = \$1, settings = \$2, integrations = \$3, updated_at = \$4\s+WHERE id = \$5`).
		WithArgs(w.Name, sqlmock.AnyArg(), nil, sqlmock.AnyArg(), w.ID).
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))

	err = repo.Update(context.Background(), w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rows affected error")

	// before save validation error (missing secret key)
	wNoSecret := &domain.Workspace{ID: "ws1", Name: "Updated Name", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
	err = repo.Update(context.Background(), wNoSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret key")
}

func TestWorkspaceRepository_checkWorkspaceIDExists_Postgres(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, &config.DatabaseConfig{Prefix: "notifuse"}, "secret-key", connMgr).(*workspaceRepository)

	// exists = true
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).
		WithArgs("ws_exists").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	exists, err := repo.checkWorkspaceIDExists(context.Background(), "ws_exists")
	require.NoError(t, err)
	assert.True(t, exists)

	// exists = false
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).
		WithArgs("ws_missing").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	exists, err = repo.checkWorkspaceIDExists(context.Background(), "ws_missing")
	require.NoError(t, err)
	assert.False(t, exists)

	// db error
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).
		WithArgs("boom").
		WillReturnError(fmt.Errorf("db error"))
	_, err = repo.checkWorkspaceIDExists(context.Background(), "boom")
	require.Error(t, err)
}

func TestWorkspaceRepository_Create_Postgres_ErrorsBeforeDBCreation(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, &config.DatabaseConfig{Prefix: "notifuse"}, "secret-key", connMgr).(*workspaceRepository)

	// 1) ID already exists -> early error
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).
		WithArgs("dup").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	dup := &domain.Workspace{ID: "dup", Name: "Dup", Settings: domain.WorkspaceSettings{Timezone: "UTC", SecretKey: "s"}}
	err := repo.Create(context.Background(), dup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// 2) BeforeSave error (missing secret key)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).
		WithArgs("no-secret").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	noSecret := &domain.Workspace{ID: "no-secret", Name: "NS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
	err = repo.Create(context.Background(), noSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret key")

	// 3) Insert error
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM workspaces WHERE id = \$1\)`).
		WithArgs("insert-fail").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	good := &domain.Workspace{ID: "insert-fail", Name: "Good", Settings: domain.WorkspaceSettings{Timezone: "UTC", SecretKey: "supersecret"}}
	mock.ExpectExec(`\s*INSERT INTO workspaces \(id, name, settings, integrations, created_at, updated_at\)\s+VALUES \(\$1, \$2, \$3, \$4, \$5, \$6\)`).
		WithArgs(good.ID, good.Name, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(fmt.Errorf("insert failed"))
	err = repo.Create(context.Background(), good)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert failed")
}

func TestWorkspaceRepository_Delete_Postgres(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{User: "postgres", Prefix: "notifuse"}
	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr).(*workspaceRepository)

	workspaceID := "ws1"
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	dbName := fmt.Sprintf("%s_ws_%s", dbConfig.Prefix, safeID)

	// Error from DeleteDatabase (revoke fails)
	revokeQuery := fmt.Sprintf(`
		REVOKE ALL PRIVILEGES ON DATABASE %s FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s;
		REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s;
		REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %s;`,
		dbName, dbName, dbConfig.User, dbConfig.User, dbConfig.User)
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnError(fmt.Errorf("perm denied"))
	err := repo.Delete(context.Background(), workspaceID)
	require.Error(t, err)

	// Successful delete path
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	terminateQuery := fmt.Sprintf(`
		SELECT pg_terminate_backend(pid) 
		FROM pg_stat_activity 
		WHERE datname = '%s' 
		AND pid <> pg_backend_pid()`, dbName)
	mock.ExpectExec(regexp.QuoteMeta(terminateQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// user_workspaces
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 3))
	// invitations
	mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	// workspace row
	mock.ExpectExec(`DELETE FROM workspaces WHERE id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Delete(context.Background(), workspaceID)
	require.NoError(t, err)

	// Not found on final delete
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(terminateQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM workspaces WHERE id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Delete(context.Background(), workspaceID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Error on intermediate deletes
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(terminateQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnError(fmt.Errorf("uw error"))
	err = repo.Delete(context.Background(), workspaceID)
	require.Error(t, err)

	// Next: fail invitations
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(terminateQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnError(fmt.Errorf("inv error"))
	err = repo.Delete(context.Background(), workspaceID)
	require.Error(t, err)

	// Next: fail workspace delete exec
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(terminateQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM user_workspaces WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM workspace_invitations WHERE workspace_id = \$1`).
		WithArgs(workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM workspaces WHERE id = \$1`).
		WithArgs(workspaceID).
		WillReturnError(fmt.Errorf("ws del error"))
	err = repo.Delete(context.Background(), workspaceID)
	require.Error(t, err)
}

func TestWorkspaceRepository_WithWorkspaceTransaction(t *testing.T) {
	// system DB is not used in this test
	systemDB, _, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	connMgr := newMockConnectionManager(systemDB)
	repo := NewWorkspaceRepository(systemDB, &config.DatabaseConfig{Prefix: "notifuse"}, "secret-key", connMgr).(*workspaceRepository)
	workspaceID := "ws_tx"

	// Create a mock workspace DB and put it in the connection manager
	wsDB, wsMock, err := sqlmock.New()
	require.NoError(t, err)
	defer wsDB.Close()
	connMgr.AddWorkspaceDB(workspaceID, wsDB)

	ctx := context.Background()

	// success: begin -> exec -> commit
	wsMock.ExpectBegin()
	wsMock.ExpectExec(`SELECT 1`).WillReturnResult(sqlmock.NewResult(0, 0))
	wsMock.ExpectCommit()

	err = repo.WithWorkspaceTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "SELECT 1")
		return e
	})
	require.NoError(t, err)

	// fn returns error -> rollback
	wsMock.ExpectBegin()
	wsMock.ExpectRollback()

	err = repo.WithWorkspaceTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return fmt.Errorf("fn error")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fn error")

	// begin error
	wsMock.ExpectBegin().WillReturnError(fmt.Errorf("begin fail"))
	err = repo.WithWorkspaceTransaction(ctx, workspaceID, func(tx *sql.Tx) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin")

	// commit error
	wsMock.ExpectBegin()
	wsMock.ExpectExec(`SELECT 1`).WillReturnResult(sqlmock.NewResult(0, 0))
	wsMock.ExpectCommit().WillReturnError(fmt.Errorf("commit fail"))
	err = repo.WithWorkspaceTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "SELECT 1")
		return e
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit")
}
