package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ConnectionManagerInterface is the interface for managing database connections
type ConnectionManagerInterface interface {
	GetSystemConnection() *sql.DB
	GetWorkspaceConnection(ctx context.Context, workspaceID string) (*sql.DB, error)
}

// WorkspaceListRepository is a subset of WorkspaceRepository for listing workspaces
type WorkspaceListRepository interface {
	List(ctx context.Context) ([]*domain.Workspace, error)
}

// SchemaVerificationService handles schema verification and repair
type SchemaVerificationService struct {
	connMgr       ConnectionManagerInterface
	workspaceRepo WorkspaceListRepository
	logger        logger.Logger
}

// NewSchemaVerificationService creates a new schema verification service
func NewSchemaVerificationService(
	connMgr ConnectionManagerInterface,
	workspaceRepo WorkspaceListRepository,
	log logger.Logger,
) *SchemaVerificationService {
	return &SchemaVerificationService{
		connMgr:       connMgr,
		workspaceRepo: workspaceRepo,
		logger:        log,
	}
}

// VerifyAllSchemas verifies schema for system DB and all workspace DBs
func (s *SchemaVerificationService) VerifyAllSchemas(ctx context.Context) (*domain.SchemaVerificationResult, error) {
	result := &domain.SchemaVerificationResult{
		VerifiedAt:   time.Now().UTC(),
		WorkspaceDBs: []domain.WorkspaceVerification{},
		Summary: domain.VerificationSummary{
			TotalDatabases:  0,
			PassedDatabases: 0,
			FailedDatabases: 0,
			TotalIssues:     0,
		},
	}

	// Get all workspaces
	workspaces, err := s.workspaceRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Verify each workspace
	for _, ws := range workspaces {
		result.Summary.TotalDatabases++

		wsVerification := domain.WorkspaceVerification{
			WorkspaceID:   ws.ID,
			WorkspaceName: ws.Name,
		}

		// Get workspace connection
		db, err := s.connMgr.GetWorkspaceConnection(ctx, ws.ID)
		if err != nil {
			wsVerification.Status = "error"
			wsVerification.Error = fmt.Sprintf("cannot connect to workspace database: %v", err)
			result.Summary.FailedDatabases++
			result.WorkspaceDBs = append(result.WorkspaceDBs, wsVerification)
			continue
		}

		// Verify functions
		functions, err := s.verifyFunctions(ctx, db)
		if err != nil {
			wsVerification.Status = "error"
			wsVerification.Error = fmt.Sprintf("failed to verify functions: %v", err)
			result.Summary.FailedDatabases++
			result.WorkspaceDBs = append(result.WorkspaceDBs, wsVerification)
			continue
		}
		wsVerification.TriggerFunctions = functions

		// Verify triggers
		triggers, err := s.verifyTriggers(ctx, db)
		if err != nil {
			wsVerification.Status = "error"
			wsVerification.Error = fmt.Sprintf("failed to verify triggers: %v", err)
			result.Summary.FailedDatabases++
			result.WorkspaceDBs = append(result.WorkspaceDBs, wsVerification)
			continue
		}
		wsVerification.Triggers = triggers

		// Calculate issues
		issues := 0
		for _, f := range functions {
			if !f.Exists {
				issues++
			}
		}
		for _, tr := range triggers {
			if !tr.Exists {
				issues++
			}
		}

		result.Summary.TotalIssues += issues

		if issues == 0 {
			wsVerification.Status = "passed"
			result.Summary.PassedDatabases++
		} else {
			wsVerification.Status = "failed"
			result.Summary.FailedDatabases++
		}

		result.WorkspaceDBs = append(result.WorkspaceDBs, wsVerification)
	}

	return result, nil
}

// RepairSchemas repairs schema issues for specified workspaces
func (s *SchemaVerificationService) RepairSchemas(ctx context.Context, req *domain.SchemaRepairRequest) (*domain.SchemaRepairResult, error) {
	result := &domain.SchemaRepairResult{
		RepairedAt:   time.Now().UTC(),
		WorkspaceDBs: []domain.WorkspaceRepairResult{},
		Summary: domain.RepairSummary{
			TotalWorkspaces:    0,
			SuccessfulRepairs:  0,
			FailedRepairs:      0,
			FunctionsRecreated: 0,
			TriggersRecreated:  0,
		},
	}

	// Get workspaces to repair
	workspaces, err := s.workspaceRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Filter workspaces if specific IDs provided
	var targetWorkspaces []*domain.Workspace
	if len(req.WorkspaceIDs) > 0 {
		wsMap := make(map[string]*domain.Workspace)
		for _, ws := range workspaces {
			wsMap[ws.ID] = ws
		}
		for _, id := range req.WorkspaceIDs {
			if ws, ok := wsMap[id]; ok {
				targetWorkspaces = append(targetWorkspaces, ws)
			}
		}
	} else {
		targetWorkspaces = workspaces
	}

	// Repair each workspace
	for _, ws := range targetWorkspaces {
		result.Summary.TotalWorkspaces++

		wsResult := domain.WorkspaceRepairResult{
			WorkspaceID:        ws.ID,
			WorkspaceName:      ws.Name,
			FunctionsRecreated: []string{},
			TriggersRecreated:  []string{},
			FunctionsFailed:    []string{},
			TriggersFailed:     []string{},
		}

		// Get workspace connection
		db, err := s.connMgr.GetWorkspaceConnection(ctx, ws.ID)
		if err != nil {
			wsResult.Status = "failed"
			wsResult.Error = fmt.Sprintf("cannot connect to workspace database: %v", err)
			result.Summary.FailedRepairs++
			result.WorkspaceDBs = append(result.WorkspaceDBs, wsResult)
			continue
		}

		// Repair functions if requested
		if req.RepairFunctions {
			allFunctionNames := make([]string, len(GetExpectedFunctions()))
			for i, f := range GetExpectedFunctions() {
				allFunctionNames[i] = f.Name
			}

			recreated, failed, err := s.repairFunctions(ctx, db, allFunctionNames)
			if err != nil {
				s.logger.WithField("workspace_id", ws.ID).WithField("error", err.Error()).Error("Failed to repair functions")
			}
			wsResult.FunctionsRecreated = recreated
			wsResult.FunctionsFailed = failed
			result.Summary.FunctionsRecreated += len(recreated)
		}

		// Repair triggers if requested
		if req.RepairTriggers {
			allTriggerNames := make([]string, len(GetExpectedTriggers()))
			for i, tr := range GetExpectedTriggers() {
				allTriggerNames[i] = tr.Name
			}

			recreated, failed, err := s.repairTriggers(ctx, db, allTriggerNames)
			if err != nil {
				s.logger.WithField("workspace_id", ws.ID).WithField("error", err.Error()).Error("Failed to repair triggers")
			}
			wsResult.TriggersRecreated = recreated
			wsResult.TriggersFailed = failed
			result.Summary.TriggersRecreated += len(recreated)
		}

		// Determine status
		if len(wsResult.FunctionsFailed) == 0 && len(wsResult.TriggersFailed) == 0 {
			wsResult.Status = "success"
			result.Summary.SuccessfulRepairs++
		} else if len(wsResult.FunctionsRecreated) > 0 || len(wsResult.TriggersRecreated) > 0 {
			wsResult.Status = "partial"
			result.Summary.FailedRepairs++
		} else {
			wsResult.Status = "failed"
			result.Summary.FailedRepairs++
		}

		result.WorkspaceDBs = append(result.WorkspaceDBs, wsResult)
	}

	return result, nil
}

// verifyFunctions verifies that all expected functions exist in the database
func (s *SchemaVerificationService) verifyFunctions(ctx context.Context, db *sql.DB) ([]domain.FunctionVerification, error) {
	// Query for existing functions
	query := `
		SELECT p.proname
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		WHERE n.nspname = 'public' AND p.prokind = 'f'
		ORDER BY p.proname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query functions: %w", err)
	}
	defer rows.Close()

	// Build set of existing functions
	existingFunctions := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan function name: %w", err)
		}
		existingFunctions[name] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating functions: %w", err)
	}

	// Check each expected function
	expectedFunctions := GetExpectedFunctions()
	result := make([]domain.FunctionVerification, len(expectedFunctions))

	for i, f := range expectedFunctions {
		result[i] = domain.FunctionVerification{
			Name:   f.Name,
			Exists: existingFunctions[f.Name],
		}
	}

	return result, nil
}

// verifyTriggers verifies that all expected triggers exist in the database
func (s *SchemaVerificationService) verifyTriggers(ctx context.Context, db *sql.DB) ([]domain.TriggerVerification, error) {
	// Query for existing triggers
	query := `
		SELECT t.tgname, c.relname, p.proname
		FROM pg_trigger t
		JOIN pg_class c ON t.tgrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		LEFT JOIN pg_proc p ON t.tgfoid = p.oid
		WHERE n.nspname = 'public' AND NOT t.tgisinternal
		ORDER BY c.relname, t.tgname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query triggers: %w", err)
	}
	defer rows.Close()

	// Build map of existing triggers (key: name_table)
	existingTriggers := make(map[string]string) // name -> function
	for rows.Next() {
		var name, table, function string
		if err := rows.Scan(&name, &table, &function); err != nil {
			return nil, fmt.Errorf("failed to scan trigger: %w", err)
		}
		existingTriggers[name] = function
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating triggers: %w", err)
	}

	// Check each expected trigger
	expectedTriggers := GetExpectedTriggers()
	result := make([]domain.TriggerVerification, len(expectedTriggers))

	for i, tr := range expectedTriggers {
		function, exists := existingTriggers[tr.Name]
		result[i] = domain.TriggerVerification{
			Name:      tr.Name,
			TableName: tr.TableName,
			Exists:    exists,
			Function:  function,
		}
	}

	return result, nil
}

// repairFunctions recreates specified functions
func (s *SchemaVerificationService) repairFunctions(ctx context.Context, db *sql.DB, functionNames []string) ([]string, []string, error) {
	var recreated, failed []string

	// Build lookup map
	functionMap := make(map[string]domain.ExpectedFunction)
	for _, f := range GetExpectedFunctions() {
		functionMap[f.Name] = f
	}

	for _, name := range functionNames {
		f, ok := functionMap[name]
		if !ok {
			failed = append(failed, name)
			continue
		}

		_, err := db.ExecContext(ctx, f.SQL)
		if err != nil {
			s.logger.WithField("function", name).WithField("error", err.Error()).Warn("Failed to recreate function")
			failed = append(failed, name)
			continue
		}

		recreated = append(recreated, name)
	}

	return recreated, failed, nil
}

// repairTriggers recreates specified triggers
func (s *SchemaVerificationService) repairTriggers(ctx context.Context, db *sql.DB, triggerNames []string) ([]string, []string, error) {
	var recreated, failed []string

	// Build lookup map
	triggerMap := make(map[string]domain.ExpectedTrigger)
	for _, tr := range GetExpectedTriggers() {
		triggerMap[tr.Name] = tr
	}

	for _, name := range triggerNames {
		tr, ok := triggerMap[name]
		if !ok {
			failed = append(failed, name)
			continue
		}

		// Drop existing trigger
		_, err := db.ExecContext(ctx, tr.DropSQL)
		if err != nil {
			s.logger.WithField("trigger", name).WithField("error", err.Error()).Warn("Failed to drop trigger")
			failed = append(failed, name)
			continue
		}

		// Create trigger
		_, err = db.ExecContext(ctx, tr.CreateSQL)
		if err != nil {
			s.logger.WithField("trigger", name).WithField("error", err.Error()).Warn("Failed to create trigger")
			failed = append(failed, name)
			continue
		}

		recreated = append(recreated, name)
	}

	return recreated, failed, nil
}

// GetExpectedFunctions returns the list of expected trigger functions
func GetExpectedFunctions() []domain.ExpectedFunction {
	return []domain.ExpectedFunction{
		{
			Name: "track_contact_changes",
			SQL:  trackContactChangesSQL,
		},
		{
			Name: "track_contact_list_changes",
			SQL:  trackContactListChangesSQL,
		},
		{
			Name: "track_message_history_changes",
			SQL:  trackMessageHistoryChangesSQL,
		},
		{
			Name: "track_inbound_webhook_event_changes",
			SQL:  trackInboundWebhookEventChangesSQL,
		},
		{
			Name: "track_contact_segment_changes",
			SQL:  trackContactSegmentChangesSQL,
		},
		{
			Name: "queue_contact_for_segment_recomputation",
			SQL:  queueContactForSegmentRecomputationSQL,
		},
		{
			Name: "update_contact_lists_on_status_change",
			SQL:  updateContactListsOnStatusChangeSQL,
		},
		{
			Name: "track_custom_event_timeline",
			SQL:  trackCustomEventTimelineSQL,
		},
		{
			Name: "webhook_contacts_trigger",
			SQL:  webhookContactsTriggerSQL,
		},
		{
			Name: "webhook_contact_lists_trigger",
			SQL:  webhookContactListsTriggerSQL,
		},
		{
			Name: "webhook_contact_segments_trigger",
			SQL:  webhookContactSegmentsTriggerSQL,
		},
		{
			Name: "webhook_message_history_trigger",
			SQL:  webhookMessageHistoryTriggerSQL,
		},
		{
			Name: "webhook_custom_events_trigger",
			SQL:  webhookCustomEventsTriggerSQL,
		},
		{
			Name: "automation_enroll_contact",
			SQL:  automationEnrollContactSQL,
		},
	}
}

// GetExpectedTriggers returns the list of expected triggers
func GetExpectedTriggers() []domain.ExpectedTrigger {
	return []domain.ExpectedTrigger{
		{
			Name:      "contact_changes_trigger",
			TableName: "contacts",
			DropSQL:   "DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts",
			CreateSQL: "CREATE TRIGGER contact_changes_trigger AFTER INSERT OR UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION track_contact_changes()",
		},
		{
			Name:      "contact_list_changes_trigger",
			TableName: "contact_lists",
			DropSQL:   "DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists",
			CreateSQL: "CREATE TRIGGER contact_list_changes_trigger AFTER INSERT OR UPDATE ON contact_lists FOR EACH ROW EXECUTE FUNCTION track_contact_list_changes()",
		},
		{
			Name:      "message_history_changes_trigger",
			TableName: "message_history",
			DropSQL:   "DROP TRIGGER IF EXISTS message_history_changes_trigger ON message_history",
			CreateSQL: "CREATE TRIGGER message_history_changes_trigger AFTER INSERT OR UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION track_message_history_changes()",
		},
		{
			Name:      "inbound_webhook_event_changes_trigger",
			TableName: "inbound_webhook_events",
			DropSQL:   "DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger ON inbound_webhook_events",
			CreateSQL: "CREATE TRIGGER inbound_webhook_event_changes_trigger AFTER INSERT ON inbound_webhook_events FOR EACH ROW EXECUTE FUNCTION track_inbound_webhook_event_changes()",
		},
		{
			Name:      "contact_segment_changes_trigger",
			TableName: "contact_segments",
			DropSQL:   "DROP TRIGGER IF EXISTS contact_segment_changes_trigger ON contact_segments",
			CreateSQL: "CREATE TRIGGER contact_segment_changes_trigger AFTER INSERT OR DELETE ON contact_segments FOR EACH ROW EXECUTE FUNCTION track_contact_segment_changes()",
		},
		{
			Name:      "contact_timeline_queue_trigger",
			TableName: "contact_timeline",
			DropSQL:   "DROP TRIGGER IF EXISTS contact_timeline_queue_trigger ON contact_timeline",
			CreateSQL: "CREATE TRIGGER contact_timeline_queue_trigger AFTER INSERT ON contact_timeline FOR EACH ROW EXECUTE FUNCTION queue_contact_for_segment_recomputation()",
		},
		{
			Name:      "message_history_status_trigger",
			TableName: "message_history",
			DropSQL:   "DROP TRIGGER IF EXISTS message_history_status_trigger ON message_history",
			CreateSQL: "CREATE TRIGGER message_history_status_trigger AFTER UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION update_contact_lists_on_status_change()",
		},
		{
			Name:      "custom_event_timeline_trigger",
			TableName: "custom_events",
			DropSQL:   "DROP TRIGGER IF EXISTS custom_event_timeline_trigger ON custom_events",
			CreateSQL: "CREATE TRIGGER custom_event_timeline_trigger AFTER INSERT OR UPDATE ON custom_events FOR EACH ROW EXECUTE FUNCTION track_custom_event_timeline()",
		},
		{
			Name:      "webhook_contacts",
			TableName: "contacts",
			DropSQL:   "DROP TRIGGER IF EXISTS webhook_contacts ON contacts",
			CreateSQL: "CREATE TRIGGER webhook_contacts AFTER INSERT OR UPDATE OR DELETE ON contacts FOR EACH ROW EXECUTE FUNCTION webhook_contacts_trigger()",
		},
		{
			Name:      "webhook_contact_lists",
			TableName: "contact_lists",
			DropSQL:   "DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists",
			CreateSQL: "CREATE TRIGGER webhook_contact_lists AFTER INSERT OR UPDATE ON contact_lists FOR EACH ROW EXECUTE FUNCTION webhook_contact_lists_trigger()",
		},
		{
			Name:      "webhook_contact_segments",
			TableName: "contact_segments",
			DropSQL:   "DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments",
			CreateSQL: "CREATE TRIGGER webhook_contact_segments AFTER INSERT OR DELETE ON contact_segments FOR EACH ROW EXECUTE FUNCTION webhook_contact_segments_trigger()",
		},
		{
			Name:      "webhook_message_history",
			TableName: "message_history",
			DropSQL:   "DROP TRIGGER IF EXISTS webhook_message_history ON message_history",
			CreateSQL: "CREATE TRIGGER webhook_message_history AFTER INSERT OR UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION webhook_message_history_trigger()",
		},
		{
			Name:      "webhook_custom_events",
			TableName: "custom_events",
			DropSQL:   "DROP TRIGGER IF EXISTS webhook_custom_events ON custom_events",
			CreateSQL: "CREATE TRIGGER webhook_custom_events AFTER INSERT OR UPDATE ON custom_events FOR EACH ROW EXECUTE FUNCTION webhook_custom_events_trigger()",
		},
	}
}
