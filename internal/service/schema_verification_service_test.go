package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockConnectionManager implements database.ConnectionManager for testing
type MockConnectionManager struct {
	systemDB       *sql.DB
	workspaceDBs   map[string]*sql.DB
	closeError     error
	connectionErr  error
}

func NewMockConnectionManager(systemDB *sql.DB) *MockConnectionManager {
	return &MockConnectionManager{
		systemDB:     systemDB,
		workspaceDBs: make(map[string]*sql.DB),
	}
}

func (m *MockConnectionManager) GetSystemConnection() *sql.DB {
	return m.systemDB
}

func (m *MockConnectionManager) GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if m.connectionErr != nil {
		return nil, m.connectionErr
	}
	db, ok := m.workspaceDBs[workspaceID]
	if !ok {
		return nil, errors.New("workspace connection not found")
	}
	return db, nil
}

func (m *MockConnectionManager) CloseWorkspaceConnection(workspaceID string) error {
	return m.closeError
}

func (m *MockConnectionManager) GetStats() interface{} {
	return nil
}

func (m *MockConnectionManager) Close() error {
	return m.closeError
}

func (m *MockConnectionManager) AddWorkspaceDB(workspaceID string, db *sql.DB) {
	m.workspaceDBs[workspaceID] = db
}

// MockWorkspaceRepository implements a subset of domain.WorkspaceRepository for testing
type MockSchemaWorkspaceRepository struct {
	workspaces []*domain.Workspace
	listError  error
}

func (m *MockSchemaWorkspaceRepository) List(ctx context.Context) ([]*domain.Workspace, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.workspaces, nil
}

func TestGetExpectedFunctions(t *testing.T) {
	t.Run("returns all 14 expected functions", func(t *testing.T) {
		functions := GetExpectedFunctions()

		// Should have 14 functions
		assert.Equal(t, 14, len(functions), "Expected 14 trigger functions")

		// Build a map for easier lookup
		functionNames := make(map[string]bool)
		for _, f := range functions {
			functionNames[f.Name] = true
			// Each function should have SQL defined
			assert.NotEmpty(t, f.SQL, "Function %s should have SQL defined", f.Name)
		}

		// Verify all expected functions are present
		expectedNames := []string{
			"track_contact_changes",
			"track_contact_list_changes",
			"track_message_history_changes",
			"track_inbound_webhook_event_changes",
			"track_contact_segment_changes",
			"queue_contact_for_segment_recomputation",
			"update_contact_lists_on_status_change",
			"track_custom_event_timeline",
			"webhook_contacts_trigger",
			"webhook_contact_lists_trigger",
			"webhook_contact_segments_trigger",
			"webhook_message_history_trigger",
			"webhook_custom_events_trigger",
			"automation_enroll_contact",
		}

		for _, name := range expectedNames {
			assert.True(t, functionNames[name], "Expected function %s to be present", name)
		}
	})

	t.Run("function SQL contains CREATE OR REPLACE FUNCTION", func(t *testing.T) {
		functions := GetExpectedFunctions()

		for _, f := range functions {
			assert.Contains(t, f.SQL, "CREATE OR REPLACE FUNCTION",
				"Function %s SQL should contain CREATE OR REPLACE FUNCTION", f.Name)
		}
	})
}

func TestGetExpectedTriggers(t *testing.T) {
	t.Run("returns all 13 expected triggers", func(t *testing.T) {
		triggers := GetExpectedTriggers()

		// Should have 13 triggers
		assert.Equal(t, 13, len(triggers), "Expected 13 triggers")

		// Build a map for easier lookup
		triggerMap := make(map[string]domain.ExpectedTrigger)
		for _, tr := range triggers {
			triggerMap[tr.Name] = tr
			// Each trigger should have table name and SQL
			assert.NotEmpty(t, tr.TableName, "Trigger %s should have TableName", tr.Name)
			assert.NotEmpty(t, tr.DropSQL, "Trigger %s should have DropSQL", tr.Name)
			assert.NotEmpty(t, tr.CreateSQL, "Trigger %s should have CreateSQL", tr.Name)
		}

		// Verify all expected triggers with their tables
		expectedTriggers := map[string]string{
			"contact_changes_trigger":              "contacts",
			"contact_list_changes_trigger":         "contact_lists",
			"message_history_changes_trigger":      "message_history",
			"inbound_webhook_event_changes_trigger": "inbound_webhook_events",
			"contact_segment_changes_trigger":      "contact_segments",
			"contact_timeline_queue_trigger":       "contact_timeline",
			"message_history_status_trigger":       "message_history",
			"custom_event_timeline_trigger":        "custom_events",
			"webhook_contacts":                     "contacts",
			"webhook_contact_lists":                "contact_lists",
			"webhook_contact_segments":             "contact_segments",
			"webhook_message_history":              "message_history",
			"webhook_custom_events":                "custom_events",
		}

		for triggerName, tableName := range expectedTriggers {
			tr, exists := triggerMap[triggerName]
			assert.True(t, exists, "Expected trigger %s to be present", triggerName)
			if exists {
				assert.Equal(t, tableName, tr.TableName,
					"Trigger %s should be on table %s", triggerName, tableName)
			}
		}
	})

	t.Run("trigger SQL patterns are correct", func(t *testing.T) {
		triggers := GetExpectedTriggers()

		for _, tr := range triggers {
			assert.Contains(t, tr.DropSQL, "DROP TRIGGER IF EXISTS",
				"Trigger %s DropSQL should contain DROP TRIGGER IF EXISTS", tr.Name)
			assert.Contains(t, tr.CreateSQL, "CREATE TRIGGER",
				"Trigger %s CreateSQL should contain CREATE TRIGGER", tr.Name)
		}
	})
}

func TestSchemaVerificationService_VerifyWorkspaceFunctions(t *testing.T) {
	t.Run("all functions exist", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query to return all expected functions
		expectedFunctions := GetExpectedFunctions()
		rows := sqlmock.NewRows([]string{"proname"})
		for _, f := range expectedFunctions {
			rows.AddRow(f.Name)
		}
		mock.ExpectQuery("SELECT p.proname").WillReturnRows(rows)

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		result, err := service.verifyFunctions(ctx, db)

		require.NoError(t, err)
		assert.Equal(t, 14, len(result))

		// All should exist
		for _, f := range result {
			assert.True(t, f.Exists, "Function %s should exist", f.Name)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("some functions missing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Return only 10 functions (missing 4)
		rows := sqlmock.NewRows([]string{"proname"}).
			AddRow("track_contact_changes").
			AddRow("track_contact_list_changes").
			AddRow("track_message_history_changes").
			AddRow("track_inbound_webhook_event_changes").
			AddRow("track_contact_segment_changes").
			AddRow("queue_contact_for_segment_recomputation").
			AddRow("update_contact_lists_on_status_change").
			AddRow("track_custom_event_timeline").
			AddRow("webhook_contacts_trigger").
			AddRow("webhook_contact_lists_trigger")

		mock.ExpectQuery("SELECT p.proname").WillReturnRows(rows)

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		result, err := service.verifyFunctions(ctx, db)

		require.NoError(t, err)
		assert.Equal(t, 14, len(result))

		// Count missing functions
		missingCount := 0
		for _, f := range result {
			if !f.Exists {
				missingCount++
			}
		}
		assert.Equal(t, 4, missingCount, "Should have 4 missing functions")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery("SELECT p.proname").WillReturnError(errors.New("database connection failed"))

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		result, err := service.verifyFunctions(ctx, db)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "database connection failed")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaVerificationService_VerifyWorkspaceTriggers(t *testing.T) {
	t.Run("all triggers exist", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock query to return all expected triggers
		rows := sqlmock.NewRows([]string{"tgname", "relname", "proname"}).
			AddRow("contact_changes_trigger", "contacts", "track_contact_changes").
			AddRow("contact_list_changes_trigger", "contact_lists", "track_contact_list_changes").
			AddRow("message_history_changes_trigger", "message_history", "track_message_history_changes").
			AddRow("inbound_webhook_event_changes_trigger", "inbound_webhook_events", "track_inbound_webhook_event_changes").
			AddRow("contact_segment_changes_trigger", "contact_segments", "track_contact_segment_changes").
			AddRow("contact_timeline_queue_trigger", "contact_timeline", "queue_contact_for_segment_recomputation").
			AddRow("message_history_status_trigger", "message_history", "update_contact_lists_on_status_change").
			AddRow("custom_event_timeline_trigger", "custom_events", "track_custom_event_timeline").
			AddRow("webhook_contacts", "contacts", "webhook_contacts_trigger").
			AddRow("webhook_contact_lists", "contact_lists", "webhook_contact_lists_trigger").
			AddRow("webhook_contact_segments", "contact_segments", "webhook_contact_segments_trigger").
			AddRow("webhook_message_history", "message_history", "webhook_message_history_trigger").
			AddRow("webhook_custom_events", "custom_events", "webhook_custom_events_trigger")

		mock.ExpectQuery("SELECT t.tgname").WillReturnRows(rows)

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		result, err := service.verifyTriggers(ctx, db)

		require.NoError(t, err)
		assert.Equal(t, 13, len(result))

		// All should exist
		for _, tr := range result {
			assert.True(t, tr.Exists, "Trigger %s should exist", tr.Name)
		}

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("some triggers missing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Return only 8 triggers (missing 5)
		rows := sqlmock.NewRows([]string{"tgname", "relname", "proname"}).
			AddRow("contact_changes_trigger", "contacts", "track_contact_changes").
			AddRow("contact_list_changes_trigger", "contact_lists", "track_contact_list_changes").
			AddRow("message_history_changes_trigger", "message_history", "track_message_history_changes").
			AddRow("inbound_webhook_event_changes_trigger", "inbound_webhook_events", "track_inbound_webhook_event_changes").
			AddRow("contact_segment_changes_trigger", "contact_segments", "track_contact_segment_changes").
			AddRow("contact_timeline_queue_trigger", "contact_timeline", "queue_contact_for_segment_recomputation").
			AddRow("message_history_status_trigger", "message_history", "update_contact_lists_on_status_change").
			AddRow("custom_event_timeline_trigger", "custom_events", "track_custom_event_timeline")

		mock.ExpectQuery("SELECT t.tgname").WillReturnRows(rows)

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		result, err := service.verifyTriggers(ctx, db)

		require.NoError(t, err)
		assert.Equal(t, 13, len(result))

		// Count missing triggers
		missingCount := 0
		for _, tr := range result {
			if !tr.Exists {
				missingCount++
			}
		}
		assert.Equal(t, 5, missingCount, "Should have 5 missing triggers")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery("SELECT t.tgname").WillReturnError(errors.New("database timeout"))

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		result, err := service.verifyTriggers(ctx, db)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "database timeout")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaVerificationService_RepairFunctions(t *testing.T) {
	t.Run("repairs missing functions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Expect CREATE OR REPLACE FUNCTION calls for specified functions
		functionsToRepair := []string{"track_contact_changes", "webhook_contacts_trigger"}

		for range functionsToRepair {
			mock.ExpectExec("CREATE OR REPLACE FUNCTION").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		recreated, failed, err := service.repairFunctions(ctx, db, functionsToRepair)

		require.NoError(t, err)
		assert.Equal(t, 2, len(recreated))
		assert.Equal(t, 0, len(failed))

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles function creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		functionsToRepair := []string{"track_contact_changes", "webhook_contacts_trigger"}

		// First succeeds, second fails
		mock.ExpectExec("CREATE OR REPLACE FUNCTION").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE OR REPLACE FUNCTION").WillReturnError(errors.New("syntax error"))

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		recreated, failed, err := service.repairFunctions(ctx, db, functionsToRepair)

		// Should not return error, just track failed
		require.NoError(t, err)
		assert.Equal(t, 1, len(recreated))
		assert.Equal(t, 1, len(failed))

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaVerificationService_RepairTriggers(t *testing.T) {
	t.Run("repairs missing triggers", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		triggersToRepair := []string{"contact_changes_trigger", "webhook_contacts"}

		// Each trigger needs DROP + CREATE
		for range triggersToRepair {
			mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("CREATE TRIGGER").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		recreated, failed, err := service.repairTriggers(ctx, db, triggersToRepair)

		require.NoError(t, err)
		assert.Equal(t, 2, len(recreated))
		assert.Equal(t, 0, len(failed))

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles trigger creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		triggersToRepair := []string{"contact_changes_trigger", "webhook_contacts"}

		// First trigger succeeds
		mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER").WillReturnResult(sqlmock.NewResult(0, 0))

		// Second trigger fails on create
		mock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TRIGGER").WillReturnError(errors.New("function does not exist"))

		log := logger.NewLogger()
		service := &SchemaVerificationService{logger: log}

		ctx := context.Background()
		recreated, failed, err := service.repairTriggers(ctx, db, triggersToRepair)

		require.NoError(t, err)
		assert.Equal(t, 1, len(recreated))
		assert.Equal(t, 1, len(failed))

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaVerificationService_VerifyAllSchemas(t *testing.T) {
	t.Run("verifies system and workspace databases", func(t *testing.T) {
		// Create mock system DB
		systemDB, _, err := sqlmock.New()
		require.NoError(t, err)
		defer systemDB.Close()

		// Create mock workspace DB
		workspaceDB, workspaceMock, err := sqlmock.New()
		require.NoError(t, err)
		defer workspaceDB.Close()

		// System DB expectations (no functions/triggers in system DB for now)
		// We're not checking system DB functions in this implementation

		// Workspace DB expectations - functions query
		funcRows := sqlmock.NewRows([]string{"proname"})
		for _, f := range GetExpectedFunctions() {
			funcRows.AddRow(f.Name)
		}
		workspaceMock.ExpectQuery("SELECT p.proname").WillReturnRows(funcRows)

		// Workspace DB expectations - triggers query
		triggerRows := sqlmock.NewRows([]string{"tgname", "relname", "proname"}).
			AddRow("contact_changes_trigger", "contacts", "track_contact_changes").
			AddRow("contact_list_changes_trigger", "contact_lists", "track_contact_list_changes").
			AddRow("message_history_changes_trigger", "message_history", "track_message_history_changes").
			AddRow("inbound_webhook_event_changes_trigger", "inbound_webhook_events", "track_inbound_webhook_event_changes").
			AddRow("contact_segment_changes_trigger", "contact_segments", "track_contact_segment_changes").
			AddRow("contact_timeline_queue_trigger", "contact_timeline", "queue_contact_for_segment_recomputation").
			AddRow("message_history_status_trigger", "message_history", "update_contact_lists_on_status_change").
			AddRow("custom_event_timeline_trigger", "custom_events", "track_custom_event_timeline").
			AddRow("webhook_contacts", "contacts", "webhook_contacts_trigger").
			AddRow("webhook_contact_lists", "contact_lists", "webhook_contact_lists_trigger").
			AddRow("webhook_contact_segments", "contact_segments", "webhook_contact_segments_trigger").
			AddRow("webhook_message_history", "message_history", "webhook_message_history_trigger").
			AddRow("webhook_custom_events", "custom_events", "webhook_custom_events_trigger")
		workspaceMock.ExpectQuery("SELECT t.tgname").WillReturnRows(triggerRows)

		// Setup mock connection manager
		connMgr := NewMockConnectionManager(systemDB)
		connMgr.AddWorkspaceDB("ws-123", workspaceDB)

		// Setup mock workspace repository
		workspaceRepo := &MockSchemaWorkspaceRepository{
			workspaces: []*domain.Workspace{
				{ID: "ws-123", Name: "Test Workspace"},
			},
		}

		log := logger.NewLogger()
		service := NewSchemaVerificationService(connMgr, workspaceRepo, log)

		ctx := context.Background()
		result, err := service.VerifyAllSchemas(ctx)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Check summary
		assert.Equal(t, 1, result.Summary.TotalDatabases)
		assert.Equal(t, 1, result.Summary.PassedDatabases)
		assert.Equal(t, 0, result.Summary.FailedDatabases)
		assert.Equal(t, 0, result.Summary.TotalIssues)

		// Check workspace verification
		assert.Equal(t, 1, len(result.WorkspaceDBs))
		assert.Equal(t, "ws-123", result.WorkspaceDBs[0].WorkspaceID)
		assert.Equal(t, "passed", result.WorkspaceDBs[0].Status)

		assert.NoError(t, workspaceMock.ExpectationsWereMet())
	})

	t.Run("handles workspace connection error", func(t *testing.T) {
		systemDB, _, err := sqlmock.New()
		require.NoError(t, err)
		defer systemDB.Close()

		connMgr := NewMockConnectionManager(systemDB)
		connMgr.connectionErr = errors.New("cannot connect to workspace database")

		workspaceRepo := &MockSchemaWorkspaceRepository{
			workspaces: []*domain.Workspace{
				{ID: "ws-123", Name: "Test Workspace"},
			},
		}

		log := logger.NewLogger()
		service := NewSchemaVerificationService(connMgr, workspaceRepo, log)

		ctx := context.Background()
		result, err := service.VerifyAllSchemas(ctx)

		require.NoError(t, err) // Should not fail, just report error in result
		require.NotNil(t, result)

		assert.Equal(t, 1, result.Summary.TotalDatabases)
		assert.Equal(t, 0, result.Summary.PassedDatabases)
		assert.Equal(t, 1, result.Summary.FailedDatabases)
		assert.Equal(t, "error", result.WorkspaceDBs[0].Status)
		assert.Contains(t, result.WorkspaceDBs[0].Error, "cannot connect")
	})
}

func TestSchemaVerificationService_RepairSchemas(t *testing.T) {
	t.Run("repairs specified workspace", func(t *testing.T) {
		systemDB, _, err := sqlmock.New()
		require.NoError(t, err)
		defer systemDB.Close()

		workspaceDB, workspaceMock, err := sqlmock.New()
		require.NoError(t, err)
		defer workspaceDB.Close()

		// Expect function repairs (14 functions)
		for i := 0; i < 14; i++ {
			workspaceMock.ExpectExec("CREATE OR REPLACE FUNCTION").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Expect trigger repairs (13 triggers, each with DROP + CREATE)
		for i := 0; i < 13; i++ {
			workspaceMock.ExpectExec("DROP TRIGGER IF EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
			workspaceMock.ExpectExec("CREATE TRIGGER").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		connMgr := NewMockConnectionManager(systemDB)
		connMgr.AddWorkspaceDB("ws-123", workspaceDB)

		workspaceRepo := &MockSchemaWorkspaceRepository{
			workspaces: []*domain.Workspace{
				{ID: "ws-123", Name: "Test Workspace"},
			},
		}

		log := logger.NewLogger()
		service := NewSchemaVerificationService(connMgr, workspaceRepo, log)

		ctx := context.Background()
		req := &domain.SchemaRepairRequest{
			WorkspaceIDs:    []string{"ws-123"},
			RepairTriggers:  true,
			RepairFunctions: true,
		}

		result, err := service.RepairSchemas(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 1, result.Summary.TotalWorkspaces)
		assert.Equal(t, 1, result.Summary.SuccessfulRepairs)
		assert.Equal(t, 0, result.Summary.FailedRepairs)
		assert.Equal(t, 14, result.Summary.FunctionsRecreated)
		assert.Equal(t, 13, result.Summary.TriggersRecreated)

		assert.NoError(t, workspaceMock.ExpectationsWereMet())
	})

	t.Run("repairs only functions when specified", func(t *testing.T) {
		systemDB, _, err := sqlmock.New()
		require.NoError(t, err)
		defer systemDB.Close()

		workspaceDB, workspaceMock, err := sqlmock.New()
		require.NoError(t, err)
		defer workspaceDB.Close()

		// Expect only function repairs (14 functions)
		for i := 0; i < 14; i++ {
			workspaceMock.ExpectExec("CREATE OR REPLACE FUNCTION").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// No trigger repairs expected

		connMgr := NewMockConnectionManager(systemDB)
		connMgr.AddWorkspaceDB("ws-123", workspaceDB)

		workspaceRepo := &MockSchemaWorkspaceRepository{
			workspaces: []*domain.Workspace{
				{ID: "ws-123", Name: "Test Workspace"},
			},
		}

		log := logger.NewLogger()
		service := NewSchemaVerificationService(connMgr, workspaceRepo, log)

		ctx := context.Background()
		req := &domain.SchemaRepairRequest{
			WorkspaceIDs:    []string{"ws-123"},
			RepairTriggers:  false,
			RepairFunctions: true,
		}

		result, err := service.RepairSchemas(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 14, result.Summary.FunctionsRecreated)
		assert.Equal(t, 0, result.Summary.TriggersRecreated)

		assert.NoError(t, workspaceMock.ExpectationsWereMet())
	})

	t.Run("handles workspace not found", func(t *testing.T) {
		systemDB, _, err := sqlmock.New()
		require.NoError(t, err)
		defer systemDB.Close()

		connMgr := NewMockConnectionManager(systemDB)
		// No workspace DB added

		workspaceRepo := &MockSchemaWorkspaceRepository{
			workspaces: []*domain.Workspace{}, // Empty - workspace not found
		}

		log := logger.NewLogger()
		service := NewSchemaVerificationService(connMgr, workspaceRepo, log)

		ctx := context.Background()
		req := &domain.SchemaRepairRequest{
			WorkspaceIDs:    []string{"ws-nonexistent"},
			RepairTriggers:  true,
			RepairFunctions: true,
		}

		result, err := service.RepairSchemas(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should report the workspace as not found
		assert.Equal(t, 0, result.Summary.TotalWorkspaces)
	})
}
