package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
	"github.com/Notifuse/notifuse/pkg/mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockWorkspaceRepository is a mock implementation of WorkspaceRepository
type MockWorkspaceRepository struct {
	mock.Mock
}

// --- Interface Methods ---

func (m *MockWorkspaceRepository) GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	args := m.Called(ctx, workspaceID)
	db, _ := args.Get(0).(*sql.DB)
	return db, args.Error(1)
}

func (m *MockWorkspaceRepository) Create(ctx context.Context, workspace *domain.Workspace) error {
	args := m.Called(ctx, workspace)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) GetByID(ctx context.Context, id string) (*domain.Workspace, error) {
	args := m.Called(ctx, id)
	ws, _ := args.Get(0).(*domain.Workspace)
	return ws, args.Error(1)
}

func (m *MockWorkspaceRepository) List(ctx context.Context) ([]*domain.Workspace, error) {
	args := m.Called(ctx)
	wss, _ := args.Get(0).([]*domain.Workspace)
	return wss, args.Error(1)
}

func (m *MockWorkspaceRepository) Update(ctx context.Context, workspace *domain.Workspace) error {
	args := m.Called(ctx, workspace)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) AddUserToWorkspace(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
	args := m.Called(ctx, userWorkspace)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) RemoveUserFromWorkspace(ctx context.Context, userID string, workspaceID string) error {
	args := m.Called(ctx, userID, workspaceID)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) GetUserWorkspaces(ctx context.Context, userID string) ([]*domain.UserWorkspace, error) {
	args := m.Called(ctx, userID)
	uws, _ := args.Get(0).([]*domain.UserWorkspace)
	return uws, args.Error(1)
}

func (m *MockWorkspaceRepository) GetWorkspaceUsersWithEmail(ctx context.Context, workspaceID string) ([]*domain.UserWorkspaceWithEmail, error) {
	args := m.Called(ctx, workspaceID)
	uwse, _ := args.Get(0).([]*domain.UserWorkspaceWithEmail)
	return uwse, args.Error(1)
}

func (m *MockWorkspaceRepository) GetUserWorkspace(ctx context.Context, userID string, workspaceID string) (*domain.UserWorkspace, error) {
	args := m.Called(ctx, userID, workspaceID)
	uw, _ := args.Get(0).(*domain.UserWorkspace)
	return uw, args.Error(1)
}

func (m *MockWorkspaceRepository) CreateInvitation(ctx context.Context, invitation *domain.WorkspaceInvitation) error {
	args := m.Called(ctx, invitation)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) GetInvitationByID(ctx context.Context, id string) (*domain.WorkspaceInvitation, error) {
	args := m.Called(ctx, id)
	inv, _ := args.Get(0).(*domain.WorkspaceInvitation)
	return inv, args.Error(1)
}

func (m *MockWorkspaceRepository) GetInvitationByEmail(ctx context.Context, workspaceID, email string) (*domain.WorkspaceInvitation, error) {
	args := m.Called(ctx, workspaceID, email)
	inv, _ := args.Get(0).(*domain.WorkspaceInvitation)
	return inv, args.Error(1)
}

func (m *MockWorkspaceRepository) IsUserWorkspaceMember(ctx context.Context, userID, workspaceID string) (bool, error) {
	args := m.Called(ctx, userID, workspaceID)
	return args.Bool(0), args.Error(1)
}

func (m *MockWorkspaceRepository) CreateDatabase(ctx context.Context, workspaceID string) error {
	args := m.Called(ctx, workspaceID)
	return args.Error(0)
}

func (m *MockWorkspaceRepository) DeleteDatabase(ctx context.Context, workspaceID string) error {
	args := m.Called(ctx, workspaceID)
	return args.Error(0)
}

// Not in interface, but used by template repository
func (m *MockWorkspaceRepository) GetAllConnections(ctx context.Context) (map[string]*sql.DB, error) {
	// Not needed for template repository tests
	return nil, nil
}

func (m *MockWorkspaceRepository) AddConnection(ctx context.Context, workspaceID string, db *sql.DB) error {
	// Not needed for template repository tests
	return nil
}

func (m *MockWorkspaceRepository) RemoveConnection(ctx context.Context, workspaceID string) error {
	// Not needed for template repository tests
	return nil
}

func (m *MockWorkspaceRepository) CloseAllConnections() {
	// Not needed for template repository tests
}

// Helper function to create a valid template for testing
func createTestTemplate() *domain.Template {
	now := time.Now().UTC().Truncate(time.Microsecond) // Truncate for DB precision
	return &domain.Template{
		ID:      "template-id-1",
		Name:    "Test Template",
		Version: 1,
		Channel: "email", // Use string "email"
		Email: &domain.EmailTemplate{
			FromAddress:      "test@example.com",
			FromName:         "Test Sender",
			Subject:          "Test Email",
			CompiledPreview:  "<html><body>Test</body></html>",
			VisualEditorTree: mjml.EmailBlock{}, // Add required field
		},
		Category:  "Test Category",
		TestData:  domain.MapOfAny{"name": "Test User"},
		Settings:  domain.MapOfAny{"priority": "high"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Helper function to marshal data, failing the test on error
func mustMarshal(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func TestTemplateRepository_CreateTemplate(t *testing.T) {
	db, mockSQL, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	mockWorkspaceRepo := new(MockWorkspaceRepository)
	repo := NewTemplateRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-1"
	template := createTestTemplate()
	template.Version = 0 // Will be set by repo

	// Mock GetConnection
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil)

	// Expect Insert Query
	mockSQL.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO templates (
			id, name, version, channel, email, category, template_macro_id, 
			utm_source, utm_medium, utm_campaign, test_data, settings, 
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`)).WithArgs(
		template.ID, template.Name, 1, template.Channel, template.Email, template.Category,
		nil, nil, nil, nil, // macro_id and utm fields are nil
		template.TestData, template.Settings, sqlmock.AnyArg(), sqlmock.AnyArg(), // created_at, updated_at
	).WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateTemplate(ctx, workspaceID, template)
	require.NoError(t, err)
	assert.Equal(t, int64(1), template.Version) // Should be set to 1
	assert.NotZero(t, template.CreatedAt)
	assert.NotZero(t, template.UpdatedAt)
	assert.Equal(t, template.CreatedAt, template.UpdatedAt) // Should be set to the same time initially

	// Assert that expectations were met
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// Test GetConnection error
	mockWorkspaceRepo.ExpectedCalls = nil // Clear previous expectations
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error"))
	err = repo.CreateTemplate(ctx, workspaceID, template)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
	mockWorkspaceRepo.AssertExpectations(t)

	// Test DB error
	mockSQL.ExpectationsWereMet() // Clear previous expectations
	mockWorkspaceRepo.ExpectedCalls = nil
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil)
	mockSQL.ExpectExec(regexp.QuoteMeta(`INSERT INTO templates`)).
		WithArgs(
			template.ID, template.Name, 1, template.Channel, template.Email, template.Category,
			nil, nil, nil, nil, template.TestData, template.Settings, sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnError(fmt.Errorf("db insert error"))

	err = repo.CreateTemplate(ctx, workspaceID, template)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create template")
	assert.Contains(t, err.Error(), "db insert error")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

}

func TestTemplateRepository_GetTemplateByID(t *testing.T) {
	db, mockSQL, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	mockWorkspaceRepo := new(MockWorkspaceRepository)
	repo := NewTemplateRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-1"
	template := createTestTemplate()
	templateID := template.ID
	version := template.Version

	columns := []string{"id", "name", "version", "channel", "email", "category", "template_macro_id", "utm_source", "utm_medium", "utm_campaign", "test_data", "settings", "created_at", "updated_at"}

	// === Test Case 1: Get Latest Version (version = 0) ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	rowsLatest := sqlmock.NewRows(columns).
		AddRow(templateID, template.Name, version, template.Channel, template.Email, template.Category, nil, nil, nil, nil, template.TestData, template.Settings, template.CreatedAt, template.UpdatedAt)
	mockSQL.ExpectQuery(regexp.QuoteMeta(`
			SELECT 
				id, name, version, channel, email, category, template_macro_id, 
				utm_source, utm_medium, utm_campaign, test_data, settings, 
				created_at, updated_at
			FROM templates
			WHERE id = $1
			ORDER BY version DESC
			LIMIT 1
		`)).WithArgs(templateID).WillReturnRows(rowsLatest)

	result, err := repo.GetTemplateByID(ctx, workspaceID, templateID, 0) // version 0 means latest
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, template.ID, result.ID)
	assert.Equal(t, template.Name, result.Name)
	assert.Equal(t, template.Version, result.Version)
	assert.Equal(t, template.Channel, result.Channel)
	assert.Equal(t, template.Category, result.Category)
	assert.EqualValues(t, template.Email, result.Email)
	assert.EqualValues(t, template.TestData, result.TestData)
	assert.EqualValues(t, template.Settings, result.Settings)
	assert.Equal(t, template.CreatedAt.Unix(), result.CreatedAt.Unix())
	assert.Equal(t, template.UpdatedAt.Unix(), result.UpdatedAt.Unix())
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 2: Get Specific Version ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	rowsSpecific := sqlmock.NewRows(columns).
		AddRow(templateID, template.Name, version, template.Channel, template.Email, template.Category, nil, nil, nil, nil, template.TestData, template.Settings, template.CreatedAt, template.UpdatedAt)
	mockSQL.ExpectQuery(regexp.QuoteMeta(`
			SELECT 
				id, name, version, channel, email, category, template_macro_id, 
				utm_source, utm_medium, utm_campaign, test_data, settings, 
				created_at, updated_at
			FROM templates
			WHERE id = $1 AND version = $2
		`)).WithArgs(templateID, version).WillReturnRows(rowsSpecific)

	result, err = repo.GetTemplateByID(ctx, workspaceID, templateID, version)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, template.ID, result.ID)
	// ... other assertions same as above ...
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 3: Template Not Found ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, version`)). // Simplified regex
										WithArgs("not-found-id").
										WillReturnError(sql.ErrNoRows)

	result, err = repo.GetTemplateByID(ctx, workspaceID, "not-found-id", 0)
	require.Error(t, err)
	assert.Nil(t, result)
	var notFoundErr *domain.ErrTemplateNotFound
	require.ErrorAs(t, err, &notFoundErr)
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 4: DB Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, version`)). // Simplified regex
										WithArgs(templateID).
										WillReturnError(fmt.Errorf("db query error"))

	result, err = repo.GetTemplateByID(ctx, workspaceID, templateID, 0)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.NotErrorIs(t, err, sql.ErrNoRows)
	assert.Contains(t, err.Error(), "failed to get template")
	assert.Contains(t, err.Error(), "db query error")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 5: GetConnection Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error")).Once()
	result, err = repo.GetTemplateByID(ctx, workspaceID, templateID, 0)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
	mockWorkspaceRepo.AssertExpectations(t)

	// === Test Case 6: JSON Unmarshal Error (Simulated by invalid JSON) ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	rowsInvalidJSON := sqlmock.NewRows(columns).
		AddRow(templateID, template.Name, version, template.Channel, nil, template.Category, nil, nil, nil, nil, template.TestData, template.Settings, template.CreatedAt, template.UpdatedAt).
		RowError(0, fmt.Errorf("scan error"))
	mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, version, channel, email, category`)).WithArgs(templateID, version).WillReturnRows(rowsInvalidJSON)

	result, err = repo.GetTemplateByID(ctx, workspaceID, templateID, version)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get template")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())
}

func TestTemplateRepository_GetTemplateLatestVersion(t *testing.T) {
	db, mockSQL, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	mockWorkspaceRepo := new(MockWorkspaceRepository)
	repo := NewTemplateRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-1"
	templateID := "template-id-1"
	latestVersion := int64(5)

	// === Test Case 1: Success ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	rows := sqlmock.NewRows([]string{"max_version"}).AddRow(latestVersion)
	mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version) FROM templates WHERE id = $1`)).
		WithArgs(templateID).
		WillReturnRows(rows)

	version, err := repo.GetTemplateLatestVersion(ctx, workspaceID, templateID)
	require.NoError(t, err)
	assert.Equal(t, latestVersion, version)
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 2: Template Not Found (ErrNoRows) ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version)`)).
		WithArgs("not-found-id").
		WillReturnError(sql.ErrNoRows) // Simulate not found by returning ErrNoRows

	version, err = repo.GetTemplateLatestVersion(ctx, workspaceID, "not-found-id")
	require.Error(t, err)
	assert.Zero(t, version)
	var notFoundErr *domain.ErrTemplateNotFound
	require.ErrorAs(t, err, &notFoundErr)
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 3: DB Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version)`)).
		WithArgs(templateID).
		WillReturnError(fmt.Errorf("db query error"))

	version, err = repo.GetTemplateLatestVersion(ctx, workspaceID, templateID)
	require.Error(t, err)
	assert.Zero(t, version)
	assert.NotErrorIs(t, err, sql.ErrNoRows)
	assert.Contains(t, err.Error(), "failed to get template latest version")
	assert.Contains(t, err.Error(), "db query error")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 4: GetConnection Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error")).Once()
	version, err = repo.GetTemplateLatestVersion(ctx, workspaceID, templateID)
	require.Error(t, err)
	assert.Zero(t, version)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
	mockWorkspaceRepo.AssertExpectations(t)
}

func TestTemplateRepository_GetTemplates(t *testing.T) {
	db, mockSQL, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	mockWorkspaceRepo := new(MockWorkspaceRepository)
	repo := NewTemplateRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-1"

	tmpl1 := createTestTemplate()
	tmpl1.ID = "tmpl-1"
	tmpl1.Version = 2 // Latest version for tmpl-1
	tmpl1.UpdatedAt = time.Now().UTC().Add(-1 * time.Hour)

	tmpl2 := createTestTemplate()
	tmpl2.ID = "tmpl-2"
	tmpl2.Version = 1 // Latest version for tmpl-2
	tmpl2.UpdatedAt = time.Now().UTC()

	columns := []string{"id", "name", "version", "channel", "email", "category", "template_macro_id", "utm_source", "utm_medium", "utm_campaign", "test_data", "settings", "created_at", "updated_at"}

	// === Test Case 1: Success ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	rows := sqlmock.NewRows(columns).
		AddRow(tmpl2.ID, tmpl2.Name, tmpl2.Version, tmpl2.Channel, tmpl2.Email, tmpl2.Category, nil, nil, nil, nil, tmpl2.TestData, tmpl2.Settings, tmpl2.CreatedAt, tmpl2.UpdatedAt). // tmpl2 is newer
		AddRow(tmpl1.ID, tmpl1.Name, tmpl1.Version, tmpl1.Channel, tmpl1.Email, tmpl1.Category, nil, nil, nil, nil, tmpl1.TestData, tmpl1.Settings, tmpl1.CreatedAt, tmpl1.UpdatedAt)

	mockSQL.ExpectQuery(regexp.QuoteMeta(`
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM templates
			GROUP BY id
		)
		SELECT 
			t.id, t.name, t.version, t.channel, t.email, t.category, t.template_macro_id, 
			t.utm_source, t.utm_medium, t.utm_campaign, t.test_data, t.settings, 
			t.created_at, t.updated_at
		FROM templates t
		JOIN latest_versions lv ON t.id = lv.id AND t.version = lv.max_version
		WHERE t.deleted_at IS NULL
		ORDER BY t.updated_at DESC
	`)).WillReturnRows(rows)

	templates, err := repo.GetTemplates(ctx, workspaceID)
	require.NoError(t, err)
	require.Len(t, templates, 2)

	// Check order (tmpl2 first due to updated_at DESC)
	assert.Equal(t, tmpl2.ID, templates[0].ID)
	assert.Equal(t, tmpl2.Version, templates[0].Version)
	assert.EqualValues(t, tmpl2.Email, templates[0].Email)

	assert.Equal(t, tmpl1.ID, templates[1].ID)
	assert.Equal(t, tmpl1.Version, templates[1].Version)
	assert.EqualValues(t, tmpl1.Email, templates[1].Email)

	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 2: No Templates Found ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	emptyRows := sqlmock.NewRows(columns) // No rows added
	mockSQL.ExpectQuery(regexp.QuoteMeta(`WITH latest_versions AS`)).WillReturnRows(emptyRows)

	templates, err = repo.GetTemplates(ctx, workspaceID)
	require.NoError(t, err)
	require.Empty(t, templates)
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 3: DB Query Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectQuery(regexp.QuoteMeta(`WITH latest_versions AS`)).WillReturnError(fmt.Errorf("db query error"))

	templates, err = repo.GetTemplates(ctx, workspaceID)
	require.Error(t, err)
	assert.Nil(t, templates)
	assert.Contains(t, err.Error(), "failed to get templates")
	assert.Contains(t, err.Error(), "db query error")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 4: Row Scan Error (Simulated by invalid JSON) ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	invalidJSONRows := sqlmock.NewRows(columns).
		AddRow(tmpl1.ID, tmpl1.Name, tmpl1.Version, tmpl1.Channel, nil, tmpl1.Category, nil, nil, nil, nil, tmpl1.TestData, tmpl1.Settings, tmpl1.CreatedAt, tmpl1.UpdatedAt).
		RowError(0, fmt.Errorf("scan error"))
	mockSQL.ExpectQuery(regexp.QuoteMeta(`WITH latest_versions AS`)).WillReturnRows(invalidJSONRows)

	templates, err = repo.GetTemplates(ctx, workspaceID)
	require.Error(t, err)
	assert.Nil(t, templates)
	assert.Contains(t, err.Error(), "error iterating template rows")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 5: GetConnection Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error")).Once()
	templates, err = repo.GetTemplates(ctx, workspaceID)
	require.Error(t, err)
	assert.Nil(t, templates)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
	mockWorkspaceRepo.AssertExpectations(t)
}

func TestTemplateRepository_UpdateTemplate(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-1"
	existingTemplate := createTestTemplate()
	existingTemplate.Version = 2 // Current latest version

	updatedTemplateBase := createTestTemplate() // Create a base for updates
	updatedTemplateBase.ID = existingTemplate.ID
	updatedTemplateBase.CreatedAt = existingTemplate.CreatedAt // Keep original creation time

	// === Test Case 1: Success ===
	t.Run("Success", func(t *testing.T) {
		db, mockSQL, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		mockWorkspaceRepo := new(MockWorkspaceRepository)
		repo := NewTemplateRepository(mockWorkspaceRepo)
		updatedTemplate := *updatedTemplateBase
		updatedTemplate.Name = "Updated Success"
		emailJSON := mustMarshal(t, updatedTemplate.Email)
		settingsJSON := mustMarshal(t, updatedTemplate.Settings)
		testDataJSON := mustMarshal(t, updatedTemplate.TestData)
		expectedNewVersion := existingTemplate.Version + 1

		// Expect TWO GetConnection calls: 1 in UpdateTemplate, 1 in GetTemplateLatestVersion
		mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Twice()
		latestVersionRows := sqlmock.NewRows([]string{"max_version"}).AddRow(existingTemplate.Version)
		mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version) FROM templates WHERE id = $1`)).
			WithArgs(updatedTemplate.ID).
			WillReturnRows(latestVersionRows)
		mockSQL.ExpectExec(regexp.QuoteMeta(`INSERT INTO templates`)).WithArgs(
			updatedTemplate.ID, updatedTemplate.Name, expectedNewVersion, updatedTemplate.Channel, emailJSON,
			updatedTemplate.Category, nil, nil, nil, nil, testDataJSON, settingsJSON,
			updatedTemplate.CreatedAt, sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.UpdateTemplate(ctx, workspaceID, &updatedTemplate)
		require.NoError(t, err)
		assert.Equal(t, expectedNewVersion, updatedTemplate.Version)
		assert.True(t, updatedTemplate.UpdatedAt.After(existingTemplate.UpdatedAt))
		mockWorkspaceRepo.AssertExpectations(t)
		require.NoError(t, mockSQL.ExpectationsWereMet())
	})

	// === Test Case 2: GetConnection Error (First call) ===
	t.Run("GetConnection Error (First call)", func(t *testing.T) {
		mockWorkspaceRepo := new(MockWorkspaceRepository)
		repo := NewTemplateRepository(mockWorkspaceRepo)
		updatedTemplate := *updatedTemplateBase

		// Expect ONE GetConnection call (the first one in UpdateTemplate which fails)
		mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error 1")).Once()
		err := repo.UpdateTemplate(ctx, workspaceID, &updatedTemplate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		mockWorkspaceRepo.AssertExpectations(t)
	})

	// === Test Case 3: GetLatestVersion Fails ===
	t.Run("GetLatestVersion Fails", func(t *testing.T) {
		db, mockSQL, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		updatedTemplate := *updatedTemplateBase
		mockWorkspaceRepo := new(MockWorkspaceRepository)
		repo := NewTemplateRepository(mockWorkspaceRepo)

		// Expect TWO GetConnection calls: 1 in UpdateTemplate, 1 in GetTemplateLatestVersion (both succeed)
		mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Twice()
		mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version) FROM templates WHERE id = $1`)).
			WithArgs(updatedTemplate.ID).
			WillReturnError(fmt.Errorf("get latest version error")) // The SQL query fails

		err := repo.UpdateTemplate(ctx, workspaceID, &updatedTemplate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get template latest version")
		assert.Contains(t, err.Error(), "get latest version error")
		mockWorkspaceRepo.AssertExpectations(t)
		require.NoError(t, mockSQL.ExpectationsWereMet())
	})

	// === Test Case 4: GetConnection Error (Second call) ===
	t.Run("GetConnection Error (Second call)", func(t *testing.T) {
		db, mockSQL, cleanup := testutil.SetupMockDB(t) // Need DB for the first successful call
		defer cleanup()
		updatedTemplate := *updatedTemplateBase
		mockWorkspaceRepo := new(MockWorkspaceRepository)
		repo := NewTemplateRepository(mockWorkspaceRepo)

		// Expect TWO GetConnection calls: 1 in UpdateTemplate (succeeds), 1 in GetTemplateLatestVersion (fails)
		mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
		mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error 2")).Once()

		err := repo.UpdateTemplate(ctx, workspaceID, &updatedTemplate)
		require.Error(t, err)
		// The error comes from GetTemplateLatestVersion's GetConnection call
		assert.Contains(t, err.Error(), "failed to get template latest version")
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.Contains(t, err.Error(), "connection error 2")
		mockWorkspaceRepo.AssertExpectations(t) // Checks both GetConnection calls
		// No SQL expectations should have been met
		require.NoError(t, mockSQL.ExpectationsWereMet())
	})

	// === Test Case 5: Insert Fails ===
	t.Run("Insert Fails", func(t *testing.T) {
		db, mockSQL, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		updatedTemplate := *updatedTemplateBase
		updatedTemplate.Name = "Updated Fail Insert"
		emailJSON := mustMarshal(t, updatedTemplate.Email)
		settingsJSON := mustMarshal(t, updatedTemplate.Settings)
		testDataJSON := mustMarshal(t, updatedTemplate.TestData)
		expectedNewVersion := existingTemplate.Version + 1
		mockWorkspaceRepo := new(MockWorkspaceRepository)
		repo := NewTemplateRepository(mockWorkspaceRepo)

		// Expect TWO GetConnection calls: 1 in UpdateTemplate, 1 in GetTemplateLatestVersion (both succeed)
		mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Twice()
		latestVersionRows := sqlmock.NewRows([]string{"max_version"}).AddRow(existingTemplate.Version)
		mockSQL.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version) FROM templates WHERE id = $1`)).
			WithArgs(updatedTemplate.ID).
			WillReturnRows(latestVersionRows)
		// Expect the INSERT to fail
		mockSQL.ExpectExec(regexp.QuoteMeta(`INSERT INTO templates`)).
			WithArgs(
				updatedTemplate.ID, updatedTemplate.Name, expectedNewVersion, updatedTemplate.Channel, emailJSON,
				updatedTemplate.Category, nil, nil, nil, nil, testDataJSON, settingsJSON,
				updatedTemplate.CreatedAt, sqlmock.AnyArg(),
			).WillReturnError(fmt.Errorf("db insert error"))

		err := repo.UpdateTemplate(ctx, workspaceID, &updatedTemplate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update template")
		assert.Contains(t, err.Error(), "db insert error")
		mockWorkspaceRepo.AssertExpectations(t)           // Checks both GetConnection calls
		require.NoError(t, mockSQL.ExpectationsWereMet()) // Checks both SQL expectations
	})
}

func TestTemplateRepository_DeleteTemplate(t *testing.T) {
	db, mockSQL, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	mockWorkspaceRepo := new(MockWorkspaceRepository)
	repo := NewTemplateRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-1"
	templateID := "template-to-delete"

	// === Test Case 1: Success ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectExec(regexp.QuoteMeta(`UPDATE templates SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(templateID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	err := repo.DeleteTemplate(ctx, workspaceID, templateID)
	require.NoError(t, err)
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 2: Template Not Found (0 rows affected) ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectExec(regexp.QuoteMeta(`UPDATE templates SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs("not-found-id").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	err = repo.DeleteTemplate(ctx, workspaceID, "not-found-id")
	require.Error(t, err)
	var notFoundErr *domain.ErrTemplateNotFound
	require.ErrorAs(t, err, &notFoundErr)
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 3: DB Error on Exec ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectExec(regexp.QuoteMeta(`UPDATE templates SET deleted_at = NOW()`)). // Simplified regex for update
												WithArgs(templateID).
												WillReturnError(fmt.Errorf("db update error"))

	err = repo.DeleteTemplate(ctx, workspaceID, templateID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, sql.ErrNoRows)
	assert.Contains(t, err.Error(), "failed to delete template")
	assert.Contains(t, err.Error(), "db update error") // Check for update error message
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 4: Error getting RowsAffected (less common) ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(db, nil).Once()
	mockSQL.ExpectExec(regexp.QuoteMeta(`UPDATE templates SET deleted_at = NOW()`)). // Simplified regex for update
												WithArgs(templateID).
												WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error"))) // Simulate error getting rows affected

	err = repo.DeleteTemplate(ctx, workspaceID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get affected rows")
	assert.Contains(t, err.Error(), "rows affected error")
	mockWorkspaceRepo.AssertExpectations(t)
	require.NoError(t, mockSQL.ExpectationsWereMet())

	// === Test Case 5: GetConnection Error ===
	mockWorkspaceRepo.On("GetConnection", ctx, workspaceID).Return(nil, fmt.Errorf("connection error")).Once()
	err = repo.DeleteTemplate(ctx, workspaceID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
	mockWorkspaceRepo.AssertExpectations(t)
}
