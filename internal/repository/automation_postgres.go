package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/lib/pq"
)

// AutomationRepository implements domain.AutomationRepository
type AutomationRepository struct {
	workspaceRepo    domain.WorkspaceRepository
	db               *sql.DB // Used for testing with sqlmock
	triggerGenerator *service.AutomationTriggerGenerator
}

// NewAutomationRepository creates a new AutomationRepository using workspace repository
func NewAutomationRepository(workspaceRepo domain.WorkspaceRepository, triggerGenerator *service.AutomationTriggerGenerator) domain.AutomationRepository {
	return &AutomationRepository{
		workspaceRepo:    workspaceRepo,
		triggerGenerator: triggerGenerator,
	}
}

// NewAutomationRepositoryWithDB creates a new AutomationRepository with a direct DB connection (for testing)
func NewAutomationRepositoryWithDB(db *sql.DB, triggerGenerator *service.AutomationTriggerGenerator) domain.AutomationRepository {
	return &AutomationRepository{
		db:               db,
		triggerGenerator: triggerGenerator,
	}
}

// getDB returns the database connection for a workspace
func (r *AutomationRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

// psql is a Squirrel StatementBuilder configured for PostgreSQL
var automationPsql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// WithTransaction executes a function within a transaction
// Note: For workspace-scoped operations, use workspaceRepo.GetConnection() first
func (r *AutomationRepository) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	// This is a placeholder - actual transactions should be managed per-workspace
	return fmt.Errorf("use workspace-specific transactions via GetConnection")
}

// Automation CRUD operations

// Create adds a new automation
func (r *AutomationRepository) Create(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	return r.CreateTx(ctx, nil, workspaceID, automation)
}

// CreateTx adds a new automation within a transaction
func (r *AutomationRepository) CreateTx(ctx context.Context, tx *sql.Tx, workspaceID string, automation *domain.Automation) error {
	triggerJSON, err := json.Marshal(automation.Trigger)
	if err != nil {
		return fmt.Errorf("failed to marshal trigger config: %w", err)
	}

	nodesJSON, err := json.Marshal(automation.Nodes)
	if err != nil {
		return fmt.Errorf("failed to marshal nodes: %w", err)
	}

	// Initialize Stats if nil to prevent JSONB null (scalar) being stored
	// JSONB null causes "cannot set path in scalar" error when automation_enroll_contact
	// tries to use jsonb_set on the stats field
	if automation.Stats == nil {
		automation.Stats = &domain.AutomationStats{}
	}

	statsJSON, err := json.Marshal(automation.Stats)
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	now := time.Now().UTC()
	automation.CreatedAt = now
	automation.UpdatedAt = now

	query, args, err := automationPsql.
		Insert("automations").
		Columns(
			"id", "workspace_id", "name", "status", "list_id", "trigger_config",
			"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "exit_on_reply",
		).
		Values(
			automation.ID, workspaceID, automation.Name, automation.Status,
			automation.ListID, triggerJSON, automation.TriggerSQL,
			automation.RootNodeID, nodesJSON, statsJSON, automation.CreatedAt, automation.UpdatedAt, automation.ExitOnReply,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get database connection: %w", err)
		}
		execer = db
	}

	_, err = execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to create automation: %w", err)
	}

	return nil
}

// GetByID retrieves an automation by ID
func (r *AutomationRepository) GetByID(ctx context.Context, workspaceID, id string) (*domain.Automation, error) {
	return r.GetByIDTx(ctx, nil, workspaceID, id)
}

// GetByIDTx retrieves an automation by ID within a transaction
// Soft-deleted automations are excluded by default
func (r *AutomationRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) (*domain.Automation, error) {
	query, args, err := automationPsql.
		Select(
			"id", "workspace_id", "name", "status", "list_id", "trigger_config",
			"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at", "exit_on_reply",
		).
		From("automations").
		Where(sq.Eq{"id": id, "workspace_id": workspaceID, "deleted_at": nil}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var queryer interface {
		QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	}
	if tx != nil {
		queryer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get database connection: %w", err)
		}
		queryer = db
	}

	var automation domain.Automation
	var triggerJSON, nodesJSON, statsJSON []byte
	var deletedAt sql.NullTime

	err = queryer.QueryRowContext(ctx, query, args...).Scan(
		&automation.ID, &automation.WorkspaceID, &automation.Name, &automation.Status,
		&automation.ListID, &triggerJSON, &automation.TriggerSQL, &automation.RootNodeID,
		&nodesJSON, &statsJSON, &automation.CreatedAt, &automation.UpdatedAt, &deletedAt, &automation.ExitOnReply,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("automation not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get automation: %w", err)
	}

	if err := json.Unmarshal(triggerJSON, &automation.Trigger); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trigger config: %w", err)
	}
	if len(nodesJSON) > 0 {
		if err := json.Unmarshal(nodesJSON, &automation.Nodes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal nodes: %w", err)
		}
	}
	if len(statsJSON) > 0 {
		if err := json.Unmarshal(statsJSON, &automation.Stats); err != nil {
			return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
		}
	}

	if deletedAt.Valid {
		automation.DeletedAt = &deletedAt.Time
	}

	return &automation, nil
}

// List retrieves automations with filtering
// Soft-deleted automations are excluded unless filter.IncludeDeleted is true
func (r *AutomationRepository) List(ctx context.Context, workspaceID string, filter domain.AutomationFilter) ([]*domain.Automation, int, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Build base where clause
	whereClause := sq.Eq{"workspace_id": workspaceID}

	if len(filter.Status) > 0 {
		statuses := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			statuses[i] = string(s)
		}
		whereClause["status"] = statuses
	}
	if filter.ListID != "" {
		whereClause["list_id"] = filter.ListID
	}

	// Exclude soft-deleted automations unless IncludeDeleted is set
	if !filter.IncludeDeleted {
		whereClause["deleted_at"] = nil
	}

	// Count query
	countQuery, countArgs, err := automationPsql.
		Select("COUNT(*)").
		From("automations").
		Where(whereClause).
		ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("failed to count automations: %w", err)
	}

	// Data query
	dataQuery := automationPsql.
		Select(
			"id", "workspace_id", "name", "status", "list_id", "trigger_config",
			"trigger_sql", "root_node_id", "nodes", "stats", "created_at", "updated_at", "deleted_at", "exit_on_reply",
		).
		From("automations").
		Where(whereClause).
		OrderBy("created_at DESC")

	if filter.Limit > 0 {
		dataQuery = dataQuery.Limit(uint64(filter.Limit))
	}
	if filter.Offset > 0 {
		dataQuery = dataQuery.Offset(uint64(filter.Offset))
	}

	query, args, err := dataQuery.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list automations: %w", err)
	}
	defer rows.Close()

	var automations []*domain.Automation
	for rows.Next() {
		var automation domain.Automation
		var triggerJSON, nodesJSON, statsJSON []byte
		var deletedAt sql.NullTime

		err := rows.Scan(
			&automation.ID, &automation.WorkspaceID, &automation.Name, &automation.Status,
			&automation.ListID, &triggerJSON, &automation.TriggerSQL, &automation.RootNodeID,
			&nodesJSON, &statsJSON, &automation.CreatedAt, &automation.UpdatedAt, &deletedAt, &automation.ExitOnReply,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan automation row: %w", err)
		}

		if err := json.Unmarshal(triggerJSON, &automation.Trigger); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal trigger config: %w", err)
		}
		if len(nodesJSON) > 0 {
			if err := json.Unmarshal(nodesJSON, &automation.Nodes); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal nodes: %w", err)
			}
		}
		if len(statsJSON) > 0 {
			if err := json.Unmarshal(statsJSON, &automation.Stats); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal stats: %w", err)
			}
		}

		if deletedAt.Valid {
			automation.DeletedAt = &deletedAt.Time
		}

		automations = append(automations, &automation)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating automation rows: %w", err)
	}

	return automations, count, nil
}

// Update updates an automation
func (r *AutomationRepository) Update(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	return r.UpdateTx(ctx, nil, workspaceID, automation)
}

// automationUpdateBuilder builds the column writes every automation update performs, and
// stamps updated_at. The WHERE clause is the caller's: UpdateTx matches the row, and
// UpdateIfStatus adds the status predicate that makes the write optimistic.
//
// NOTE: Stats are NOT updated here - they should only be modified via atomic methods
// like IncrementAutomationStat or UpdateAutomationStats to prevent accidental resets
func automationUpdateBuilder(workspaceID string, automation *domain.Automation) (sq.UpdateBuilder, error) {
	triggerJSON, err := json.Marshal(automation.Trigger)
	if err != nil {
		return sq.UpdateBuilder{}, fmt.Errorf("failed to marshal trigger config: %w", err)
	}

	nodesJSON, err := json.Marshal(automation.Nodes)
	if err != nil {
		return sq.UpdateBuilder{}, fmt.Errorf("failed to marshal nodes: %w", err)
	}

	automation.UpdatedAt = time.Now().UTC()

	return automationPsql.
		Update("automations").
		Set("name", automation.Name).
		Set("status", automation.Status).
		Set("list_id", automation.ListID).
		Set("trigger_config", triggerJSON).
		Set("trigger_sql", automation.TriggerSQL).
		Set("root_node_id", automation.RootNodeID).
		Set("nodes", nodesJSON).
		Set("exit_on_reply", automation.ExitOnReply).
		Set("updated_at", automation.UpdatedAt).
		Where(sq.Eq{"id": automation.ID, "workspace_id": workspaceID}), nil
}

// UpdateTx updates an automation within a transaction
func (r *AutomationRepository) UpdateTx(ctx context.Context, tx *sql.Tx, workspaceID string, automation *domain.Automation) error {
	builder, err := automationUpdateBuilder(workspaceID, automation)
	if err != nil {
		return err
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get database connection: %w", err)
		}
		execer = db
	}

	result, err := execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update automation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("automation not found: %s", automation.ID)
	}

	return nil
}

// UpdateIfStatus persists the automation only while its stored status is still
// expectedStatus (optimistic lock). Returns false (without error) when no row was updated —
// another transition moved the status between the caller's read and this write.
//
// The three status transitions all read the row, decide whether to install or drop the
// trigger, and only then write. Without the predicate, Update writes back the status it read
// a moment earlier, so a Pause landing in between is silently reverted to live while the
// trigger it dropped is never reinstalled — an automation showing Live that enrols nobody,
// which nothing in the product detects or repairs.
func (r *AutomationRepository) UpdateIfStatus(ctx context.Context, workspaceID string, automation *domain.Automation, expectedStatus domain.AutomationStatus) (bool, error) {
	builder, err := automationUpdateBuilder(workspaceID, automation)
	if err != nil {
		return false, err
	}

	query, args, err := builder.Where(sq.Eq{"status": expectedStatus}).ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build query: %w", err)
	}

	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get database connection: %w", err)
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to update automation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Deliberately not an error: losing a race is a normal outcome the caller turns into a
	// 409, and "automation not found" — what the unconditional Update says for the same zero
	// rows — would be a lie the caller cannot act on.
	return rowsAffected > 0, nil
}

// UpdateContactAutomationIfActive persists the contact automation only while its
// row is still 'active' (optimistic lock). Returns false (without error) when no
// row was updated — i.e. the journey was concurrently exited (e.g. by a reply
// interrupt) — so the executor can stop instead of clobbering the exit.
func (r *AutomationRepository) UpdateContactAutomationIfActive(ctx context.Context, workspaceID string, ca *domain.ContactAutomation) (bool, error) {
	contextJSON, err := json.Marshal(ca.Context)
	if err != nil {
		return false, fmt.Errorf("failed to marshal context: %w", err)
	}

	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get database connection: %w", err)
	}

	query, args, err := automationPsql.
		Update("contact_automations").
		Set("current_node_id", ca.CurrentNodeID).
		Set("status", ca.Status).
		Set("exit_reason", ca.ExitReason).
		Set("scheduled_at", ca.ScheduledAt).
		Set("context", contextJSON).
		Set("retry_count", ca.RetryCount).
		Set("last_error", ca.LastError).
		Set("last_retry_at", ca.LastRetryAt).
		Set("max_retries", ca.MaxRetries).
		Where(sq.Eq{"id": ca.ID, "status": domain.ContactAutomationStatusActive}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build query: %w", err)
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to update contact automation: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// ExitContactJourneysOnReply marks the contact's active journeys as exited when
// their automation has exit_on_reply enabled. When automationID is non-nil only
// that automation's journey is exited; otherwise all of the contact's
// exit_on_reply journeys are exited. Returns the number of journeys exited.
func (r *AutomationRepository) ExitContactJourneysOnReply(ctx context.Context, workspaceID, contactEmail string, automationID *string, reason string, before time.Time) (int, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	query := `
		UPDATE contact_automations
		SET status = 'exited', scheduled_at = NULL, exit_reason = $1
		WHERE contact_email = $2
		  AND status = 'active'
		  AND entered_at < $3
		  AND automation_id IN (
		      SELECT id FROM automations WHERE exit_on_reply = TRUE AND deleted_at IS NULL
		  )`
	args := []interface{}{reason, contactEmail, before}
	if automationID != nil && *automationID != "" {
		query += " AND automation_id = $4"
		args = append(args, *automationID)
	}

	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to exit contact journeys on reply: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Delete soft-deletes an automation by setting deleted_at timestamp
// It also drops the trigger if automation is live and exits all active contacts
func (r *AutomationRepository) Delete(ctx context.Context, workspaceID, id string) error {
	return r.DeleteTx(ctx, nil, workspaceID, id)
}

// DeleteTx soft-deletes an automation within a transaction
// It also drops the trigger if automation is live and exits all active contacts
func (r *AutomationRepository) DeleteTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	now := time.Now().UTC()

	// 1. Drop the automation trigger (ignore errors - trigger might not exist)
	_ = r.DropAutomationTrigger(ctx, workspaceID, id)

	// 2. Mark all active contact_automations as exited with reason
	exitQuery := `
		UPDATE contact_automations
		SET status = 'exited', scheduled_at = NULL, exit_reason = 'automation_deleted'
		WHERE automation_id = $1 AND status = 'active'
	`
	_, err = db.ExecContext(ctx, exitQuery, id)
	if err != nil {
		return fmt.Errorf("failed to exit active contacts: %w", err)
	}

	// 3. Soft delete: set deleted_at instead of actually deleting
	query, args, err := automationPsql.
		Update("automations").
		Set("deleted_at", now).
		Set("updated_at", now).
		Where(sq.Eq{"id": id, "workspace_id": workspaceID, "deleted_at": nil}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		execer = db
	}

	result, err := execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to soft-delete automation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("automation not found or already deleted: %s", id)
	}

	return nil
}

// Trigger management

// CreateAutomationTrigger creates a database trigger for an automation
func (r *AutomationRepository) CreateAutomationTrigger(ctx context.Context, workspaceID string, automation *domain.Automation) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Use generator to create trigger SQL. Everything it rejects came from the caller's
	// trigger configuration, so this is a 400 and not an opaque 500.
	triggerSQL, err := r.triggerGenerator.Generate(automation)
	if err != nil {
		return domain.NewTriggerConditionError(fmt.Sprintf("failed to generate trigger SQL: %v", err))
	}

	// One transaction for all of it. Run as separate statements they autocommit, so a
	// failure on CREATE TRIGGER leaves the two DROPs applied — destroying the trigger
	// the automation was running on before the re-activation was attempted, and leaving
	// an orphan function behind.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin trigger transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// DROP/CREATE TRIGGER take ACCESS EXCLUSIVE on contact_timeline, and one
	// transaction now holds it across the whole sequence instead of four short locks.
	// Bound the wait rather than blocking event ingestion behind it.
	if _, err := tx.ExecContext(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return fmt.Errorf("failed to set lock_timeout: %w", err)
	}

	// Execute in order: drop existing trigger, drop existing function, create function, create trigger
	statements := []struct {
		sql    string
		errMsg string
	}{
		{triggerSQL.DropTrigger, "failed to drop existing trigger"},
		{triggerSQL.DropFunction, "failed to drop existing function"},
		{triggerSQL.FunctionBody, "failed to create trigger function"},
		{triggerSQL.TriggerDDL, "failed to create trigger"},
	}

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt.sql); err != nil {
			return fmt.Errorf("%s: %w", stmt.errMsg, err)
		}
	}

	// Resolve the condition's column references. CREATE FUNCTION only syntax-checks a
	// plpgsql body — it does not resolve names — so an unresolvable column would install
	// cleanly and then abort every write to contact_timeline, which is fed by triggers on
	// contacts, lists, message_history, custom_events, segments and inbound webhooks.
	//
	// This runs after the DDL rather than before it, even though the point is to install
	// nothing invalid: the transaction rolls back either way, and probing first would take
	// ACCESS SHARE on contact_timeline and then upgrade to the ACCESS EXCLUSIVE the DROPs
	// need — two concurrent activations in the same workspace would deadlock on that.
	if triggerSQL.ValidationQuery != "" {
		if err := probeTriggerConditions(ctx, tx, triggerSQL.ValidationQuery); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit trigger installation: %w", err)
	}

	return nil
}

// probeTriggerConditions plans the compiled conditions and reports whether the failure is
// the caller's fault. Only a genuine syntax or semantic complaint from PostgreSQL means the
// configuration is wrong; a lock timeout or a dropped connection is transient, and calling
// it a bad condition would tell the caller not to retry something that would have worked.
func probeTriggerConditions(ctx context.Context, tx *sql.Tx, query string) error {
	rows, err := tx.QueryContext(ctx, query)
	if err == nil {
		closeErr := rows.Close()
		err = rows.Err()
		if err == nil {
			err = closeErr
		}
	}
	if err == nil {
		return nil
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		// Named codes, not the whole 42 class: class 42 also holds undefined_table and
		// insufficient_privilege, which describe the workspace database rather than the
		// condition. Blaming those on the caller answers 400 and tells them not to retry
		// a condition that is perfectly well formed.
		switch pqErr.Code {
		case "42601", // syntax_error
			"42703", // undefined_column
			"42883", // undefined_function — includes "operator does not exist"
			"42804", // datatype_mismatch
			"42846", // cannot_coerce
			// A filter whose field_type contradicts the column it names compiles fine and
			// is caught only when PostgreSQL tries to read the value — still the caller's
			// mistake, and one they cannot diagnose from a generic failure.
			"22007", // invalid_datetime_format
			"22P02", // invalid_text_representation
			"0A000": // feature_not_supported
			return domain.NewTriggerConditionError(fmt.Sprintf("invalid trigger conditions: %v", err))
		}
	}

	return fmt.Errorf("failed to validate trigger conditions: %w", err)
}

// DropAutomationTrigger removes the database trigger for an automation
func (r *AutomationRepository) DropAutomationTrigger(ctx context.Context, workspaceID, automationID string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// automations.delete validates only that the id is non-empty and never loads the row,
	// so unlike the install path this id has passed through nothing at all — no length cap,
	// no format check. It is about to be concatenated into DDL as a bare identifier.
	triggerName, err := service.AutomationTriggerName(automationID)
	if err != nil {
		return domain.NewTriggerConditionError(err.Error())
	}
	functionName := triggerName

	// Drop the trigger
	dropTriggerSQL := fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON contact_timeline", triggerName)
	_, err = db.ExecContext(ctx, dropTriggerSQL)
	if err != nil {
		return fmt.Errorf("failed to drop trigger: %w", err)
	}

	// Drop the function
	dropFunctionSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName)
	_, err = db.ExecContext(ctx, dropFunctionSQL)
	if err != nil {
		return fmt.Errorf("failed to drop trigger function: %w", err)
	}

	return nil
}

// Contact automation operations

// GetContactAutomation retrieves a contact automation by ID
func (r *AutomationRepository) GetContactAutomation(ctx context.Context, workspaceID, id string) (*domain.ContactAutomation, error) {
	return r.GetContactAutomationTx(ctx, nil, workspaceID, id)
}

// GetContactAutomationTx retrieves a contact automation by ID within a transaction
func (r *AutomationRepository) GetContactAutomationTx(ctx context.Context, tx *sql.Tx, workspaceID, id string) (*domain.ContactAutomation, error) {
	query, args, err := automationPsql.
		Select(
			"id", "automation_id", "contact_email", "current_node_id", "status",
			"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
			"last_retry_at", "max_retries",
		).
		From("contact_automations").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var queryer interface {
		QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	}
	if tx != nil {
		queryer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get database connection: %w", err)
		}
		queryer = db
	}

	var ca domain.ContactAutomation
	var contextJSON []byte

	err = queryer.QueryRowContext(ctx, query, args...).Scan(
		&ca.ID, &ca.AutomationID, &ca.ContactEmail, &ca.CurrentNodeID, &ca.Status,
		&ca.ExitReason, &ca.EnteredAt, &ca.ScheduledAt, &contextJSON, &ca.RetryCount, &ca.LastError,
		&ca.LastRetryAt, &ca.MaxRetries,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("contact automation not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact automation: %w", err)
	}

	if len(contextJSON) > 0 {
		if err := json.Unmarshal(contextJSON, &ca.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context: %w", err)
		}
	}

	return &ca, nil
}

// GetContactAutomationByEmail retrieves a contact automation by automation ID and email
func (r *AutomationRepository) GetContactAutomationByEmail(ctx context.Context, workspaceID, automationID, email string) (*domain.ContactAutomation, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query, args, err := automationPsql.
		Select(
			"id", "automation_id", "contact_email", "current_node_id", "status",
			"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
			"last_retry_at", "max_retries",
		).
		From("contact_automations").
		Where(sq.Eq{"automation_id": automationID, "contact_email": email}).
		OrderBy("entered_at DESC").
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var ca domain.ContactAutomation
	var contextJSON []byte

	err = db.QueryRowContext(ctx, query, args...).Scan(
		&ca.ID, &ca.AutomationID, &ca.ContactEmail, &ca.CurrentNodeID, &ca.Status,
		&ca.ExitReason, &ca.EnteredAt, &ca.ScheduledAt, &contextJSON, &ca.RetryCount, &ca.LastError,
		&ca.LastRetryAt, &ca.MaxRetries,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("contact automation not found for email: %s", email)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact automation by email: %w", err)
	}

	if len(contextJSON) > 0 {
		if err := json.Unmarshal(contextJSON, &ca.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context: %w", err)
		}
	}

	return &ca, nil
}

// ListContactAutomations retrieves contact automations with filtering
func (r *AutomationRepository) ListContactAutomations(ctx context.Context, workspaceID string, filter domain.ContactAutomationFilter) ([]*domain.ContactAutomation, int, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Build where clause
	whereClause := sq.And{}

	if filter.AutomationID != "" {
		whereClause = append(whereClause, sq.Eq{"automation_id": filter.AutomationID})
	}
	if filter.ContactEmail != "" {
		whereClause = append(whereClause, sq.Eq{"contact_email": filter.ContactEmail})
	}
	if len(filter.Status) > 0 {
		statuses := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			statuses[i] = string(s)
		}
		whereClause = append(whereClause, sq.Eq{"status": statuses})
	}
	if filter.ScheduledBy != nil {
		whereClause = append(whereClause, sq.LtOrEq{"scheduled_at": *filter.ScheduledBy})
	}

	// Count query
	countQuery, countArgs, err := automationPsql.
		Select("COUNT(*)").
		From("contact_automations").
		Where(whereClause).
		ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("failed to count contact automations: %w", err)
	}

	// Data query
	dataQuery := automationPsql.
		Select(
			"id", "automation_id", "contact_email", "current_node_id", "status",
			"exit_reason", "entered_at", "scheduled_at", "context", "retry_count", "last_error",
			"last_retry_at", "max_retries",
		).
		From("contact_automations").
		Where(whereClause).
		OrderBy("entered_at DESC")

	if filter.Limit > 0 {
		dataQuery = dataQuery.Limit(uint64(filter.Limit))
	}
	if filter.Offset > 0 {
		dataQuery = dataQuery.Offset(uint64(filter.Offset))
	}

	query, args, err := dataQuery.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list contact automations: %w", err)
	}
	defer rows.Close()

	var cas []*domain.ContactAutomation
	for rows.Next() {
		var ca domain.ContactAutomation
		var contextJSON []byte

		err := rows.Scan(
			&ca.ID, &ca.AutomationID, &ca.ContactEmail, &ca.CurrentNodeID, &ca.Status,
			&ca.ExitReason, &ca.EnteredAt, &ca.ScheduledAt, &contextJSON, &ca.RetryCount, &ca.LastError,
			&ca.LastRetryAt, &ca.MaxRetries,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan contact automation row: %w", err)
		}

		if len(contextJSON) > 0 {
			if err := json.Unmarshal(contextJSON, &ca.Context); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal context: %w", err)
			}
		}

		cas = append(cas, &ca)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating contact automation rows: %w", err)
	}

	return cas, count, nil
}

// UpdateContactAutomation updates a contact automation
func (r *AutomationRepository) UpdateContactAutomation(ctx context.Context, workspaceID string, ca *domain.ContactAutomation) error {
	return r.UpdateContactAutomationTx(ctx, nil, workspaceID, ca)
}

// UpdateContactAutomationTx updates a contact automation within a transaction
func (r *AutomationRepository) UpdateContactAutomationTx(ctx context.Context, tx *sql.Tx, workspaceID string, ca *domain.ContactAutomation) error {
	contextJSON, err := json.Marshal(ca.Context)
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	query, args, err := automationPsql.
		Update("contact_automations").
		Set("current_node_id", ca.CurrentNodeID).
		Set("status", ca.Status).
		Set("exit_reason", ca.ExitReason).
		Set("scheduled_at", ca.ScheduledAt).
		Set("context", contextJSON).
		Set("retry_count", ca.RetryCount).
		Set("last_error", ca.LastError).
		Set("last_retry_at", ca.LastRetryAt).
		Set("max_retries", ca.MaxRetries).
		Where(sq.Eq{"id": ca.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get database connection: %w", err)
		}
		execer = db
	}

	result, err := execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update contact automation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("contact automation not found: %s", ca.ID)
	}

	return nil
}

// GetScheduledContactAutomations retrieves contact automations scheduled for processing
// Uses FOR UPDATE SKIP LOCKED to prevent concurrent processing of the same records
// Only returns contacts from LIVE automations (paused automations' contacts stay frozen)
func (r *AutomationRepository) GetScheduledContactAutomations(ctx context.Context, workspaceID string, beforeTime time.Time, limit int) ([]*domain.ContactAutomation, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Build query with FOR UPDATE SKIP LOCKED to prevent concurrent processing
	// Join with automations to filter by automation status (only process contacts from live automations)
	// This implements the "pause" behavior: paused automations' contacts stay frozen at their current node
	query := `
		SELECT ca.id, ca.automation_id, ca.contact_email, ca.current_node_id, ca.status,
		       ca.exit_reason, ca.entered_at, ca.scheduled_at, ca.context, ca.retry_count, ca.last_error,
		       ca.last_retry_at, ca.max_retries
		FROM contact_automations ca
		JOIN automations a ON ca.automation_id = a.id
		WHERE ca.status = 'active'
		  AND ca.scheduled_at <= $1
		  AND a.status = 'live'
		  AND a.deleted_at IS NULL
		ORDER BY ca.scheduled_at ASC
		LIMIT $2
		FOR UPDATE OF ca SKIP LOCKED
	`
	args := []interface{}{beforeTime, limit}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled contact automations: %w", err)
	}
	defer rows.Close()

	var cas []*domain.ContactAutomation
	for rows.Next() {
		var ca domain.ContactAutomation
		var contextJSON []byte

		err := rows.Scan(
			&ca.ID, &ca.AutomationID, &ca.ContactEmail, &ca.CurrentNodeID, &ca.Status,
			&ca.ExitReason, &ca.EnteredAt, &ca.ScheduledAt, &contextJSON, &ca.RetryCount, &ca.LastError,
			&ca.LastRetryAt, &ca.MaxRetries,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact automation row: %w", err)
		}

		if len(contextJSON) > 0 {
			if err := json.Unmarshal(contextJSON, &ca.Context); err != nil {
				return nil, fmt.Errorf("failed to unmarshal context: %w", err)
			}
		}

		cas = append(cas, &ca)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contact automation rows: %w", err)
	}

	return cas, nil
}

// GetScheduledContactAutomationsGlobal retrieves contacts from all workspaces using round-robin
// to prevent starvation of any single workspace
func (r *AutomationRepository) GetScheduledContactAutomationsGlobal(ctx context.Context, beforeTime time.Time, limit int) ([]*domain.ContactAutomationWithWorkspace, error) {
	if r.workspaceRepo == nil {
		return nil, fmt.Errorf("workspace repository is required for global scheduling")
	}

	// Get all workspaces
	workspaces, err := r.workspaceRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces: %w", err)
	}

	if len(workspaces) == 0 {
		return nil, nil
	}

	// Round-robin: fetch equal amounts from each workspace
	perWorkspace := (limit / len(workspaces)) + 1
	if perWorkspace < 1 {
		perWorkspace = 1
	}

	var allContacts []*domain.ContactAutomationWithWorkspace

	// First pass: get perWorkspace from each
	for _, ws := range workspaces {
		contacts, err := r.GetScheduledContactAutomations(ctx, ws.ID, beforeTime, perWorkspace)
		if err != nil {
			// Log error but continue with other workspaces
			// In production, we'd want proper logging here
			continue
		}

		for _, ca := range contacts {
			allContacts = append(allContacts, &domain.ContactAutomationWithWorkspace{
				WorkspaceID:       ws.ID,
				ContactAutomation: *ca,
			})
		}

		// Stop if we have enough
		if len(allContacts) >= limit {
			break
		}
	}

	// Trim to limit
	if len(allContacts) > limit {
		allContacts = allContacts[:limit]
	}

	return allContacts, nil
}

// Node execution logging

// CreateNodeExecution creates a new node execution entry
func (r *AutomationRepository) CreateNodeExecution(ctx context.Context, workspaceID string, entry *domain.NodeExecution) error {
	return r.CreateNodeExecutionTx(ctx, nil, workspaceID, entry)
}

// CreateNodeExecutionTx creates a new node execution entry within a transaction
func (r *AutomationRepository) CreateNodeExecutionTx(ctx context.Context, tx *sql.Tx, workspaceID string, entry *domain.NodeExecution) error {
	outputJSON, err := json.Marshal(entry.Output)
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}

	query, args, err := automationPsql.
		Insert("automation_node_executions").
		Columns(
			"id", "contact_automation_id", "automation_id", "node_id", "node_type", "action",
			"entered_at", "completed_at", "duration_ms", "output", "error",
		).
		Values(
			entry.ID, entry.ContactAutomationID, entry.AutomationID, entry.NodeID, entry.NodeType, entry.Action,
			entry.EnteredAt, entry.CompletedAt, entry.DurationMs, outputJSON, entry.Error,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get database connection: %w", err)
		}
		execer = db
	}

	_, err = execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to create node execution: %w", err)
	}

	return nil
}

// GetNodeExecutions retrieves the node executions for a contact automation
func (r *AutomationRepository) GetNodeExecutions(ctx context.Context, workspaceID, contactAutomationID string) ([]*domain.NodeExecution, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	query, args, err := automationPsql.
		Select(
			"id", "contact_automation_id", "node_id", "node_type", "action",
			"entered_at", "completed_at", "duration_ms", "output", "error",
		).
		From("automation_node_executions").
		Where(sq.Eq{"contact_automation_id": contactAutomationID}).
		OrderBy("entered_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get node executions: %w", err)
	}
	defer rows.Close()

	var entries []*domain.NodeExecution
	for rows.Next() {
		var entry domain.NodeExecution
		var outputJSON []byte

		err := rows.Scan(
			&entry.ID, &entry.ContactAutomationID, &entry.NodeID, &entry.NodeType, &entry.Action,
			&entry.EnteredAt, &entry.CompletedAt, &entry.DurationMs, &outputJSON, &entry.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node execution row: %w", err)
		}

		if len(outputJSON) > 0 {
			if err := json.Unmarshal(outputJSON, &entry.Output); err != nil {
				return nil, fmt.Errorf("failed to unmarshal output: %w", err)
			}
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node execution rows: %w", err)
	}

	return entries, nil
}

// UpdateNodeExecution updates a node execution entry
func (r *AutomationRepository) UpdateNodeExecution(ctx context.Context, workspaceID string, entry *domain.NodeExecution) error {
	return r.UpdateNodeExecutionTx(ctx, nil, workspaceID, entry)
}

// UpdateNodeExecutionTx updates a node execution entry within a transaction
func (r *AutomationRepository) UpdateNodeExecutionTx(ctx context.Context, tx *sql.Tx, workspaceID string, entry *domain.NodeExecution) error {
	outputJSON, err := json.Marshal(entry.Output)
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}

	query, args, err := automationPsql.
		Update("automation_node_executions").
		Set("action", entry.Action).
		Set("completed_at", entry.CompletedAt).
		Set("duration_ms", entry.DurationMs).
		Set("output", outputJSON).
		Set("error", entry.Error).
		Where(sq.Eq{"id": entry.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get database connection: %w", err)
		}
		execer = db
	}

	result, err := execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update node execution: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("node execution not found: %s", entry.ID)
	}

	return nil
}

// Stats

// UpdateAutomationStats updates the stats for an automation
func (r *AutomationRepository) UpdateAutomationStats(ctx context.Context, workspaceID, automationID string, stats *domain.AutomationStats) error {
	return r.UpdateAutomationStatsTx(ctx, nil, workspaceID, automationID, stats)
}

// UpdateAutomationStatsTx updates the stats for an automation within a transaction
func (r *AutomationRepository) UpdateAutomationStatsTx(ctx context.Context, tx *sql.Tx, workspaceID, automationID string, stats *domain.AutomationStats) error {
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	query, args, err := automationPsql.
		Update("automations").
		Set("stats", statsJSON).
		Set("updated_at", time.Now().UTC()).
		Where(sq.Eq{"id": automationID, "workspace_id": workspaceID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		db, err := r.getDB(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to get database connection: %w", err)
		}
		execer = db
	}

	result, err := execer.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update automation stats: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("automation not found: %s", automationID)
	}

	return nil
}

// IncrementAutomationStat increments a single stat counter for an automation
// Valid stat names: enrolled, completed, exited, failed
func (r *AutomationRepository) IncrementAutomationStat(ctx context.Context, workspaceID, automationID, statName string) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Validate stat name
	validStats := map[string]bool{
		"enrolled":  true,
		"completed": true,
		"exited":    true,
		"failed":    true,
	}
	if !validStats[statName] {
		return fmt.Errorf("invalid stat name: %s", statName)
	}

	// Use JSONB update to increment the specific stat
	query := fmt.Sprintf(`
		UPDATE automations
		SET stats = COALESCE(stats, '{}'::jsonb) ||
			jsonb_build_object('%s', COALESCE((stats->>'%s')::int, 0) + 1),
			updated_at = $1
		WHERE id = $2 AND workspace_id = $3
	`, statName, statName)

	result, err := db.ExecContext(ctx, query, time.Now().UTC(), automationID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to increment automation stat: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("automation not found: %s", automationID)
	}

	return nil
}
