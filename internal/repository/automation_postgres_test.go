package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAutomationMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *AutomationRepository) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	// Create the trigger generator for the repository
	qb := service.NewQueryBuilder()
	triggerGen := service.NewAutomationTriggerGenerator(qb)

	repo := NewAutomationRepositoryWithDB(db, triggerGen).(*AutomationRepository)
	return db, mock, repo
}

// Helper to create a test automation with default values
func createTestAutomation(id, workspaceID string) *domain.Automation {
	now := time.Now().UTC()
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Test Automation",
		Status:      domain.AutomationStatusDraft,
		ListID:      "list-123",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "email.opened",
			Frequency: domain.TriggerFrequencyOnce,
		},
		RootNodeID: "node-root",
		Stats: &domain.AutomationStats{
			Enrolled:  0,
			Completed: 0,
			Exited:    0,
			Failed:    0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Helper to create a test automation node
func createTestAutomationNode(id, automationID string, nodeType domain.NodeType) *domain.AutomationNode {
	now := time.Now().UTC()
	return &domain.AutomationNode{
		ID:           id,
		AutomationID: automationID,
		Type:         nodeType,
		Config: map[string]interface{}{
			"key": "value",
		},
		Position: domain.NodePosition{
			X: 100,
			Y: 200,
		},
		CreatedAt: now,
	}
}

// Helper to create a test contact automation
func createTestContactAutomation(id, automationID, email string) *domain.ContactAutomation {
	now := time.Now().UTC()
	return &domain.ContactAutomation{
		ID:            id,
		AutomationID:  automationID,
		ContactEmail:  email,
		CurrentNodeID: nil,
		Status:        domain.ContactAutomationStatusActive,
		EnteredAt:     now,
		ScheduledAt:   &now,
		Context:       map[string]interface{}{"source": "test"},
		RetryCount:    0,
		MaxRetries:    3,
	}
}

// Helper to create a test node execution entry
func createTestNodeExecution(id, contactAutomationID, nodeID string) *domain.NodeExecution {
	now := time.Now().UTC()
	return &domain.NodeExecution{
		ID:                  id,
		ContactAutomationID: contactAutomationID,
		AutomationID:        "automation-123",
		NodeID:              nodeID,
		NodeType:            domain.NodeTypeEmail,
		Action:              domain.NodeActionEntered,
		EnteredAt:           now,
		Output:              map[string]interface{}{},
	}
}

func TestAutomationRepository_WithTransaction(t *testing.T) {
	// WithTransaction is not supported in workspace-scoped repository pattern
	// Transactions should be managed via workspace-specific database connections
	t.Skip("WithTransaction is deprecated for workspace-scoped repositories")
}

func TestAutomationRepository_Create(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automation := createTestAutomation("auto-123", workspaceID)

	// Test successful create
	mock.ExpectExec("INSERT INTO automations").
		WithArgs(
			automation.ID,
			workspaceID,
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(), // trigger_config JSON
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes JSON
			sqlmock.AnyArg(), // stats JSON
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, workspaceID, automation)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectExec("INSERT INTO automations").
		WithArgs(
			automation.ID,
			workspaceID,
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(),
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes JSON
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.Create(ctx, workspaceID, automation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create automation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetByID(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"
	now := time.Now().UTC()

	triggerJSON, _ := json.Marshal(&domain.TimelineTriggerConfig{
		EventKind: "email.opened",
		Frequency: domain.TriggerFrequencyOnce,
	})
	nodesJSON, _ := json.Marshal([]*domain.AutomationNode{})
	statsJSON, _ := json.Marshal(&domain.AutomationStats{})

	// Test successful retrieval (includes deleted_at IS NULL filter)
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		automationID, workspaceID, "Test Automation", "draft", "list-123",
		triggerJSON, nil, "node-root", nodesJSON, statsJSON, now, now, nil,
	)

	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnRows(rows)

	automation, err := repo.GetByID(ctx, workspaceID, automationID)
	assert.NoError(t, err)
	assert.NotNil(t, automation)
	assert.Equal(t, automationID, automation.ID)
	assert.Equal(t, workspaceID, automation.WorkspaceID)
	assert.Nil(t, automation.DeletedAt)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found
	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnError(sql.ErrNoRows)

	automation, err = repo.GetByID(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Nil(t, automation)
	assert.Contains(t, err.Error(), "automation not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnError(fmt.Errorf("database error"))

	automation, err = repo.GetByID(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Nil(t, automation)
	assert.Contains(t, err.Error(), "failed to get automation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_List(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()

	triggerJSON, _ := json.Marshal(&domain.TimelineTriggerConfig{
		EventKind: "email.opened",
		Frequency: domain.TriggerFrequencyOnce,
	})
	nodesJSON, _ := json.Marshal([]*domain.AutomationNode{})
	statsJSON, _ := json.Marshal(&domain.AutomationStats{})

	// Simple filter without status and list_id (excludes deleted by default)
	filter := domain.AutomationFilter{
		Limit:  10,
		Offset: 0,
	}

	// Test count query (includes deleted_at IS NULL)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnRows(countRows)

	// Test data query (includes deleted_at IS NULL)
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		"auto-1", workspaceID, "Auto 1", "draft", "list-123",
		triggerJSON, nil, "node-1", nodesJSON, statsJSON, now, now, nil,
	).AddRow(
		"auto-2", workspaceID, "Auto 2", "live", "list-123",
		triggerJSON, nil, "node-2", nodesJSON, statsJSON, now, now, nil,
	)

	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnRows(rows)

	automations, count, err := repo.List(ctx, workspaceID, filter)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, automations, 2)
	assert.Equal(t, "auto-1", automations[0].ID)
	assert.Equal(t, "auto-2", automations[1].ID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test empty result
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "name", "status", "list_id", "trigger_config",
			"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
		}))

	automations, count, err = repo.List(ctx, workspaceID, filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, automations, 0)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error on count
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnError(fmt.Errorf("database error"))

	automations, count, err = repo.List(ctx, workspaceID, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count automations")
	assert.Equal(t, 0, count)
	assert.Nil(t, automations)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Update(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automation := createTestAutomation("auto-123", workspaceID)
	automation.Name = "Updated Automation"
	automation.Status = domain.AutomationStatusLive

	// Test successful update
	// NOTE: stats is NOT included in the update query - it's only modified via atomic methods
	mock.ExpectExec("UPDATE automations SET").
		WithArgs(
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(), // trigger_config JSON
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes JSON
			sqlmock.AnyArg(), // updated_at
			automation.ID,
			workspaceID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Update(ctx, workspaceID, automation)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found
	mock.ExpectExec("UPDATE automations SET").
		WithArgs(
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(), // trigger_config JSON
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes JSON
			sqlmock.AnyArg(), // updated_at
			automation.ID,
			workspaceID,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Update(ctx, workspaceID, automation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "automation not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectExec("UPDATE automations SET").
		WithArgs(
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(), // trigger_config JSON
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes JSON
			sqlmock.AnyArg(), // updated_at
			automation.ID,
			workspaceID,
		).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.Update(ctx, workspaceID, automation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update automation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Delete(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Test successful soft-delete
	// 1. Drop trigger (2 statements)
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	// 2. Exit active contacts
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 5))
	// 3. Soft delete
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found (soft-delete)
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.Delete(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "automation not found or already deleted")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error (soft-delete)
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnError(fmt.Errorf("database error"))

	err = repo.Delete(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to soft-delete automation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_CreateAutomationTrigger(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automation := createTestAutomation("auto-123", workspaceID)
	automation.Status = domain.AutomationStatusLive

	// Test successful trigger creation (4 statements: drop trigger, drop function, create function, create trigger)
	mock.ExpectExec("DROP TRIGGER IF EXISTS").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE OR REPLACE FUNCTION").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TRIGGER").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.CreateAutomationTrigger(ctx, workspaceID, automation)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error on drop trigger
	mock.ExpectExec("DROP TRIGGER IF EXISTS").
		WillReturnError(fmt.Errorf("database error"))

	err = repo.CreateAutomationTrigger(ctx, workspaceID, automation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to drop existing trigger")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error on drop function
	mock.ExpectExec("DROP TRIGGER IF EXISTS").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").
		WillReturnError(fmt.Errorf("database error"))

	err = repo.CreateAutomationTrigger(ctx, workspaceID, automation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to drop existing function")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_DropAutomationTrigger(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Test successful drop (2 statements: drop trigger, drop function)
	mock.ExpectExec("DROP TRIGGER IF EXISTS").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.DropAutomationTrigger(ctx, workspaceID, automationID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error on drop trigger
	mock.ExpectExec("DROP TRIGGER IF EXISTS").
		WillReturnError(fmt.Errorf("database error"))

	err = repo.DropAutomationTrigger(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to drop trigger")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetContactAutomation(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	id := "ca-123"
	now := time.Now().UTC()

	contextJSON, _ := json.Marshal(map[string]interface{}{"source": "test"})

	// Test successful retrieval
	rows := sqlmock.NewRows([]string{
		"id", "automation_id", "contact_email", "current_node_id", "status",
		"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
		"last_retry_at", "max_retries",
	}).AddRow(
		id, "auto-123", "test@example.com", "node-1", "active",
		nil, now, now, contextJSON, 0, nil, nil, 3,
	)

	mock.ExpectQuery("SELECT .* FROM contact_automations WHERE id = .*").
		WithArgs(id).
		WillReturnRows(rows)

	ca, err := repo.GetContactAutomation(ctx, workspaceID, id)
	assert.NoError(t, err)
	assert.NotNil(t, ca)
	assert.Equal(t, id, ca.ID)
	assert.Equal(t, "test@example.com", ca.ContactEmail)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found
	mock.ExpectQuery("SELECT .* FROM contact_automations WHERE id = .*").
		WithArgs(id).
		WillReturnError(sql.ErrNoRows)

	ca, err = repo.GetContactAutomation(ctx, workspaceID, id)
	assert.Error(t, err)
	assert.Nil(t, ca)
	assert.Contains(t, err.Error(), "contact automation not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetContactAutomationByEmail(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"
	email := "test@example.com"
	now := time.Now().UTC()

	contextJSON, _ := json.Marshal(map[string]interface{}{})

	// Test successful retrieval
	rows := sqlmock.NewRows([]string{
		"id", "automation_id", "contact_email", "current_node_id", "status",
		"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
		"last_retry_at", "max_retries",
	}).AddRow(
		"ca-123", automationID, email, nil, "active",
		nil, now, nil, contextJSON, 0, nil, nil, 3,
	)

	mock.ExpectQuery("SELECT .* FROM contact_automations WHERE automation_id = .* AND contact_email = .*").
		WithArgs(automationID, email).
		WillReturnRows(rows)

	ca, err := repo.GetContactAutomationByEmail(ctx, workspaceID, automationID, email)
	assert.NoError(t, err)
	assert.NotNil(t, ca)
	assert.Equal(t, email, ca.ContactEmail)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_ListContactAutomations(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()

	contextJSON, _ := json.Marshal(map[string]interface{}{})

	// Simple filter with just automation ID
	filter := domain.ContactAutomationFilter{
		AutomationID: "auto-123",
		Limit:        10,
		Offset:       0,
	}

	// Test count query
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery("SELECT COUNT.*FROM contact_automations.*").
		WithArgs("auto-123").
		WillReturnRows(countRows)

	// Test data query
	rows := sqlmock.NewRows([]string{
		"id", "automation_id", "contact_email", "current_node_id", "status",
		"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
		"last_retry_at", "max_retries",
	}).AddRow(
		"ca-1", "auto-123", "user1@example.com", nil, "active",
		nil, now, nil, contextJSON, 0, nil, nil, 3,
	).AddRow(
		"ca-2", "auto-123", "user2@example.com", nil, "active",
		nil, now, nil, contextJSON, 0, nil, nil, 3,
	)

	mock.ExpectQuery("SELECT .* FROM contact_automations WHERE").
		WithArgs("auto-123").
		WillReturnRows(rows)

	cas, count, err := repo.ListContactAutomations(ctx, workspaceID, filter)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, cas, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_UpdateContactAutomation(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	ca := createTestContactAutomation("ca-123", "auto-123", "test@example.com")
	ca.Status = domain.ContactAutomationStatusCompleted

	// Test successful update
	mock.ExpectExec("UPDATE contact_automations SET").
		WithArgs(
			ca.CurrentNodeID,
			ca.Status,
			ca.ExitReason,
			ca.ScheduledAt,
			sqlmock.AnyArg(), // context JSON
			ca.RetryCount,
			ca.LastError,
			ca.LastRetryAt,
			ca.MaxRetries,
			ca.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateContactAutomation(ctx, workspaceID, ca)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found
	mock.ExpectExec("UPDATE contact_automations SET").
		WithArgs(
			ca.CurrentNodeID,
			ca.Status,
			ca.ExitReason,
			ca.ScheduledAt,
			sqlmock.AnyArg(),
			ca.RetryCount,
			ca.LastError,
			ca.LastRetryAt,
			ca.MaxRetries,
			ca.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.UpdateContactAutomation(ctx, workspaceID, ca)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contact automation not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetScheduledContactAutomations(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()
	limit := 100

	contextJSON, _ := json.Marshal(map[string]interface{}{})

	// Test successful retrieval - query now joins with automations to filter by live status
	rows := sqlmock.NewRows([]string{
		"id", "automation_id", "contact_email", "current_node_id", "status",
		"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
		"last_retry_at", "max_retries",
	}).AddRow(
		"ca-1", "auto-123", "user1@example.com", "node-1", "active",
		nil, now, now, contextJSON, 0, nil, nil, 3,
	)

	mock.ExpectQuery("SELECT ca.* FROM contact_automations ca JOIN automations a").
		WillReturnRows(rows)

	cas, err := repo.GetScheduledContactAutomations(ctx, workspaceID, now, limit)
	assert.NoError(t, err)
	assert.Len(t, cas, 1)
	assert.Equal(t, "ca-1", cas[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test empty result
	mock.ExpectQuery("SELECT ca.* FROM contact_automations ca JOIN automations a").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "automation_id", "contact_email", "current_node_id", "status",
			"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
			"last_retry_at", "max_retries",
		}))

	cas, err = repo.GetScheduledContactAutomations(ctx, workspaceID, now, limit)
	assert.NoError(t, err)
	assert.Len(t, cas, 0)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectQuery("SELECT ca.* FROM contact_automations ca JOIN automations a").
		WillReturnError(fmt.Errorf("database error"))

	cas, err = repo.GetScheduledContactAutomations(ctx, workspaceID, now, limit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get scheduled contact automations")
	assert.Nil(t, cas)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_CreateNodeExecution(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	entry := createTestNodeExecution("entry-123", "ca-123", "node-123")

	// Test successful create
	mock.ExpectExec("INSERT INTO automation_node_executions").
		WithArgs(
			entry.ID,
			entry.ContactAutomationID,
			entry.AutomationID,
			entry.NodeID,
			entry.NodeType,
			entry.Action,
			sqlmock.AnyArg(), // entered_at
			entry.CompletedAt,
			entry.DurationMs,
			sqlmock.AnyArg(), // output JSON
			entry.Error,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateNodeExecution(ctx, workspaceID, entry)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectExec("INSERT INTO automation_node_executions").
		WithArgs(
			entry.ID,
			entry.ContactAutomationID,
			entry.AutomationID,
			entry.NodeID,
			entry.NodeType,
			entry.Action,
			sqlmock.AnyArg(),
			entry.CompletedAt,
			entry.DurationMs,
			sqlmock.AnyArg(),
			entry.Error,
		).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.CreateNodeExecution(ctx, workspaceID, entry)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create node execution")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetNodeExecutions(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	contactAutomationID := "ca-123"
	now := time.Now().UTC()

	outputJSON, _ := json.Marshal(map[string]interface{}{})

	// Test successful retrieval
	rows := sqlmock.NewRows([]string{
		"id", "contact_automation_id", "node_id", "node_type", "action",
		"entered_at", "completed_at", "duration_ms", "output", "error",
	}).AddRow(
		"entry-1", contactAutomationID, "node-1", "trigger", "entered",
		now, nil, nil, outputJSON, nil,
	).AddRow(
		"entry-2", contactAutomationID, "node-2", "email", "completed",
		now, &now, int64Ptr(100), outputJSON, nil,
	)

	mock.ExpectQuery("SELECT .* FROM automation_node_executions WHERE contact_automation_id = .*").
		WithArgs(contactAutomationID).
		WillReturnRows(rows)

	entries, err := repo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
	assert.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "entry-1", entries[0].ID)
	assert.Equal(t, "entry-2", entries[1].ID)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test empty result
	mock.ExpectQuery("SELECT .* FROM automation_node_executions WHERE contact_automation_id = .*").
		WithArgs(contactAutomationID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "contact_automation_id", "node_id", "node_type", "action",
			"entered_at", "completed_at", "duration_ms", "output", "error",
		}))

	entries, err = repo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
	assert.NoError(t, err)
	assert.Len(t, entries, 0)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectQuery("SELECT .* FROM automation_node_executions WHERE contact_automation_id = .*").
		WithArgs(contactAutomationID).
		WillReturnError(fmt.Errorf("database error"))

	entries, err = repo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get node executions")
	assert.Nil(t, entries)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_UpdateNodeExecution(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	entry := createTestNodeExecution("entry-123", "ca-123", "node-123")
	now := time.Now().UTC()
	duration := int64(150)
	entry.CompletedAt = &now
	entry.DurationMs = &duration
	entry.Action = domain.NodeActionCompleted

	// Test successful update
	mock.ExpectExec("UPDATE automation_node_executions SET").
		WithArgs(
			entry.Action,
			entry.CompletedAt,
			entry.DurationMs,
			sqlmock.AnyArg(), // output JSON
			entry.Error,
			entry.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateNodeExecution(ctx, workspaceID, entry)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found
	mock.ExpectExec("UPDATE automation_node_executions SET").
		WithArgs(
			entry.Action,
			entry.CompletedAt,
			entry.DurationMs,
			sqlmock.AnyArg(),
			entry.Error,
			entry.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.UpdateNodeExecution(ctx, workspaceID, entry)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node execution not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_UpdateAutomationStats(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"
	stats := &domain.AutomationStats{
		Enrolled:  100,
		Completed: 80,
		Exited:    10,
		Failed:    5,
	}

	// Test successful update
	mock.ExpectExec("UPDATE automations SET stats = .*, updated_at = .*").
		WithArgs(
			sqlmock.AnyArg(), // stats JSON
			sqlmock.AnyArg(), // updated_at
			automationID,
			workspaceID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateAutomationStats(ctx, workspaceID, automationID, stats)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test not found
	mock.ExpectExec("UPDATE automations SET stats = .*, updated_at = .*").
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			automationID,
			workspaceID,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.UpdateAutomationStats(ctx, workspaceID, automationID, stats)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "automation not found")
	assert.NoError(t, mock.ExpectationsWereMet())

	// Test database error
	mock.ExpectExec("UPDATE automations SET stats = .*, updated_at = .*").
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			automationID,
			workspaceID,
		).
		WillReturnError(fmt.Errorf("database error"))

	err = repo.UpdateAutomationStats(ctx, workspaceID, automationID, stats)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update automation stats")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Helper function for int64 pointer
func int64Ptr(i int64) *int64 {
	return &i
}

// Test transaction variants
func TestAutomationRepository_CreateTx(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automation := createTestAutomation("auto-123", workspaceID)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO automations").
		WithArgs(
			automation.ID,
			workspaceID,
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(), // trigger_config
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes
			sqlmock.AnyArg(), // stats
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	err = repo.CreateTx(ctx, tx, workspaceID, automation)
	assert.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetByIDTx(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"
	now := time.Now().UTC()

	triggerJSON, _ := json.Marshal(&domain.TimelineTriggerConfig{
		EventKind: "email.opened",
		Frequency: domain.TriggerFrequencyOnce,
	})
	nodesJSON, _ := json.Marshal([]*domain.AutomationNode{})
	statsJSON, _ := json.Marshal(&domain.AutomationStats{})

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		automationID, workspaceID, "Test", "draft", "list-123",
		triggerJSON, nil, "node-root", nodesJSON, statsJSON, now, now, nil,
	)
	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnRows(rows)
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	automation, err := repo.GetByIDTx(ctx, tx, workspaceID, automationID)
	assert.NoError(t, err)
	assert.NotNil(t, automation)
	assert.Equal(t, automationID, automation.ID)

	err = tx.Commit()
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_WithTransaction_BeginError(t *testing.T) {
	// WithTransaction is not supported in workspace-scoped repository pattern
	t.Skip("WithTransaction is deprecated for workspace-scoped repositories")
}

func TestAutomationRepository_WithTransaction_CommitError(t *testing.T) {
	// WithTransaction is not supported in workspace-scoped repository pattern
	t.Skip("WithTransaction is deprecated for workspace-scoped repositories")
}

// Test JSON unmarshal errors
func TestAutomationRepository_GetByID_JSONUnmarshalError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"
	now := time.Now().UTC()

	// Invalid JSON for trigger_config
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		automationID, workspaceID, "Test", "draft", "list-123",
		"invalid json", nil, "node-root", "[]", "{}", now, now, nil,
	)

	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnRows(rows)

	automation, err := repo.GetByID(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Nil(t, automation)
	assert.Contains(t, err.Error(), "failed to unmarshal trigger config")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_List_JSONUnmarshalError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()

	filter := domain.AutomationFilter{
		Limit:  10,
		Offset: 0,
	}

	// Test count query (includes deleted_at IS NULL)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnRows(countRows)

	// Invalid JSON for trigger_config
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		"auto-1", workspaceID, "Auto 1", "draft", "list-123",
		"invalid json", nil, "node-1", "[]", "{}", now, now, nil,
	)

	mock.ExpectQuery("SELECT .* FROM automations.*deleted_at IS NULL").
		WillReturnRows(rows)

	automations, count, err := repo.List(ctx, workspaceID, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal trigger config")
	assert.Equal(t, 0, count)
	assert.Nil(t, automations)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_List_DataQueryError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"

	filter := domain.AutomationFilter{
		Limit:  10,
		Offset: 0,
	}

	// Test count query succeeds (includes deleted_at IS NULL)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnRows(countRows)

	// Data query fails
	mock.ExpectQuery("SELECT .* FROM automations.*deleted_at IS NULL").
		WillReturnError(fmt.Errorf("database error"))

	automations, count, err := repo.List(ctx, workspaceID, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list automations")
	assert.Equal(t, 0, count)
	assert.Nil(t, automations)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_List_ScanError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"

	filter := domain.AutomationFilter{
		Limit:  10,
		Offset: 0,
	}

	// Test count query (includes deleted_at IS NULL)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnRows(countRows)

	// Return rows with wrong number of columns
	rows := sqlmock.NewRows([]string{"id", "workspace_id"}).
		AddRow("auto-1", workspaceID)

	mock.ExpectQuery("SELECT .* FROM automations.*deleted_at IS NULL").
		WillReturnRows(rows)

	automations, count, err := repo.List(ctx, workspaceID, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan automation row")
	assert.Equal(t, 0, count)
	assert.Nil(t, automations)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetScheduledContactAutomations_ScanError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()
	limit := 100

	// Return rows with wrong number of columns
	rows := sqlmock.NewRows([]string{"id", "automation_id"}).
		AddRow("ca-1", "auto-123")

	mock.ExpectQuery("SELECT ca.* FROM contact_automations ca JOIN automations a").
		WillReturnRows(rows)

	cas, err := repo.GetScheduledContactAutomations(ctx, workspaceID, now, limit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan contact automation row")
	assert.Nil(t, cas)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetNodeExecutions_ScanError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	contactAutomationID := "ca-123"

	// Return rows with wrong number of columns
	rows := sqlmock.NewRows([]string{"id", "contact_automation_id"}).
		AddRow("entry-1", contactAutomationID)

	mock.ExpectQuery("SELECT .* FROM automation_node_executions WHERE contact_automation_id = .*").
		WithArgs(contactAutomationID).
		WillReturnRows(rows)

	entries, err := repo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan node execution row")
	assert.Nil(t, entries)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Update_RowsAffectedError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automation := createTestAutomation("auto-123", workspaceID)

	mock.ExpectExec("UPDATE automations SET").
		WithArgs(
			automation.Name,
			automation.Status,
			automation.ListID,
			sqlmock.AnyArg(), // trigger_config
			automation.TriggerSQL,
			automation.RootNodeID,
			sqlmock.AnyArg(), // nodes
			sqlmock.AnyArg(), // updated_at
			automation.ID,
			workspaceID,
		).
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))

	err := repo.Update(ctx, workspaceID, automation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get rows affected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Delete_RowsAffectedError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Drop trigger, exit contacts, then soft-delete returns rows affected error
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE automations SET deleted_at").
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected error")))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get rows affected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ========================================
// Soft-Delete Tests (TDD)
// ========================================

func TestAutomationRepository_Delete_SoftDelete(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Test successful soft-delete with full flow:
	// 1. Drop trigger (2 statements)
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	// 2. Exit active contacts
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 3))
	// 3. Soft delete
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Delete_NotFound(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// No rows affected - automation not found or already deleted
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "automation not found or already deleted")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Delete_DatabaseError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_automations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnError(fmt.Errorf("database error"))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to soft-delete automation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Delete_ExitsActiveContacts(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Verify that active contacts are exited
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	// Exit contacts returns 10 updated rows
	mock.ExpectExec("UPDATE contact_automations.*status = 'exited'").WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectExec("UPDATE automations SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_Delete_ExitContactsError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Exit contacts fails
	mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP FUNCTION IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE contact_automations").WillReturnError(fmt.Errorf("exit contacts error"))

	err := repo.Delete(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to exit active contacts")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_GetByID_ExcludesDeleted(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	// Query should include deleted_at IS NULL condition
	// When automation is soft-deleted, it should not be returned
	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WithArgs(automationID, workspaceID).
		WillReturnError(sql.ErrNoRows)

	automation, err := repo.GetByID(ctx, workspaceID, automationID)
	assert.Error(t, err)
	assert.Nil(t, automation)
	assert.Contains(t, err.Error(), "automation not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_List_ExcludesDeleted(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()

	triggerJSON, _ := json.Marshal(&domain.TimelineTriggerConfig{
		EventKind: "email.opened",
		Frequency: domain.TriggerFrequencyOnce,
	})
	nodesJSON, _ := json.Marshal([]*domain.AutomationNode{})
	statsJSON, _ := json.Marshal(&domain.AutomationStats{})

	// Default filter should exclude deleted automations
	filter := domain.AutomationFilter{
		Limit:  10,
		Offset: 0,
	}

	// Count query should include deleted_at IS NULL
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*deleted_at IS NULL").
		WillReturnRows(countRows)

	// Data query should include deleted_at IS NULL
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		"auto-1", workspaceID, "Auto 1", "draft", "list-123",
		triggerJSON, nil, "node-1", nodesJSON, statsJSON, now, now, nil,
	)

	mock.ExpectQuery("SELECT .* FROM automations WHERE.*deleted_at IS NULL").
		WillReturnRows(rows)

	automations, count, err := repo.List(ctx, workspaceID, filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, automations, 1)
	assert.Nil(t, automations[0].DeletedAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_List_IncludeDeleted(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"
	now := time.Now().UTC()
	deletedAt := now.Add(-time.Hour)

	triggerJSON, _ := json.Marshal(&domain.TimelineTriggerConfig{
		EventKind: "email.opened",
		Frequency: domain.TriggerFrequencyOnce,
	})
	nodesJSON, _ := json.Marshal([]*domain.AutomationNode{})
	statsJSON, _ := json.Marshal(&domain.AutomationStats{})

	// Filter with IncludeDeleted = true
	filter := domain.AutomationFilter{
		IncludeDeleted: true,
		Limit:          10,
		Offset:         0,
	}

	// Count query should NOT filter by deleted_at
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery("SELECT COUNT.*FROM automations.*workspace_id").
		WillReturnRows(countRows)

	// Data query should NOT filter by deleted_at
	rows := sqlmock.NewRows([]string{
		"id", "workspace_id", "name", "status", "list_id", "trigger_config",
		"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at",
	}).AddRow(
		"auto-1", workspaceID, "Auto 1", "draft", "list-123",
		triggerJSON, nil, "node-1", nodesJSON, statsJSON, now, now, nil,
	).AddRow(
		"auto-2", workspaceID, "Auto 2 (Deleted)", "draft", "list-123",
		triggerJSON, nil, "node-2", nodesJSON, statsJSON, now, now, deletedAt,
	)

	mock.ExpectQuery("SELECT .* FROM automations WHERE").
		WillReturnRows(rows)

	automations, count, err := repo.List(ctx, workspaceID, filter)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, automations, 2)
	// First automation is not deleted
	assert.Nil(t, automations[0].DeletedAt)
	// Second automation is deleted
	assert.NotNil(t, automations[1].DeletedAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_ListContactAutomations_CountError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"

	// Empty filter so we can test without complex argument matching
	filter := domain.ContactAutomationFilter{
		Limit: 10,
	}

	mock.ExpectQuery("SELECT COUNT.*FROM contact_automations.*").
		WillReturnError(fmt.Errorf("database error"))

	cas, count, err := repo.ListContactAutomations(ctx, workspaceID, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count contact automations")
	assert.Equal(t, 0, count)
	assert.Nil(t, cas)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomationRepository_ListContactAutomations_DataQueryError(t *testing.T) {
	db, mock, repo := setupAutomationMock(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "workspace-123"

	// Empty filter so we can test without complex argument matching
	filter := domain.ContactAutomationFilter{
		Limit: 10,
	}

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT.*FROM contact_automations.*").
		WillReturnRows(countRows)

	mock.ExpectQuery("SELECT .* FROM contact_automations.*").
		WillReturnError(fmt.Errorf("database error"))

	cas, count, err := repo.ListContactAutomations(ctx, workspaceID, filter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list contact automations")
	assert.Equal(t, 0, count)
	assert.Nil(t, cas)
	assert.NoError(t, mock.ExpectationsWereMet())
}
