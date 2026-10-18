package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

type listRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewListRepository creates a new PostgreSQL list repository
func NewListRepository(workspaceRepo domain.WorkspaceRepository) domain.ListRepository {
	return &listRepository{
		workspaceRepo: workspaceRepo,
	}
}

func (r *listRepository) CreateList(ctx context.Context, workspaceID string, list *domain.List) error {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()
	list.CreatedAt = now
	list.UpdatedAt = now

	query := `
		INSERT INTO lists (id, name, is_double_optin, is_public, description,
		                   double_optin_template, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err = workspaceDB.ExecContext(ctx, query,
		list.ID,
		list.Name,
		list.IsDoubleOptin,
		list.IsPublic,
		list.Description,
		list.DoubleOptInTemplate,
		list.CreatedAt,
		list.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create list: %w", err)
	}
	return nil
}

func (r *listRepository) GetListByID(ctx context.Context, workspaceID string, id string) (*domain.List, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT id, name, is_double_optin, is_public, description, double_optin_template,
		created_at, updated_at, deleted_at
		FROM lists
		WHERE id = $1 AND deleted_at IS NULL
	`

	row := workspaceDB.QueryRowContext(ctx, query, id)
	list, err := domain.ScanList(row)

	if err == sql.ErrNoRows {
		return nil, &domain.ErrListNotFound{Message: "list not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	return list, nil
}

func (r *listRepository) GetLists(ctx context.Context, workspaceID string) ([]*domain.List, error) {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT id, name, is_double_optin, is_public, description, double_optin_template,
		created_at, updated_at, deleted_at
		FROM lists
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := workspaceDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get lists: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var lists []*domain.List
	for rows.Next() {
		list, err := domain.ScanList(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan list: %w", err)
		}
		lists = append(lists, list)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating list rows: %w", err)
	}

	return lists, nil
}

func (r *listRepository) UpdateList(ctx context.Context, workspaceID string, list *domain.List) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	list.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE lists
		SET name = $1, is_double_optin = $2, is_public = $3, description = $4, updated_at = $5,
		    double_optin_template = $6
		WHERE id = $7 AND deleted_at IS NULL
	`

	result, err := workspaceDB.ExecContext(ctx, query,
		list.Name,
		list.IsDoubleOptin,
		list.IsPublic,
		list.Description,
		list.UpdatedAt,
		list.DoubleOptInTemplate,
		list.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return &domain.ErrListNotFound{Message: "list not found or already deleted"}
	}

	return nil
}

func (r *listRepository) DeleteList(ctx context.Context, workspaceID string, id string) error {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Start a transaction to ensure both list and contact_list entries are deleted together.
	// READ COMMITTED whatever the server's default, which the in-use check below relies on.
	tx, err := workspaceDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Defer rollback - it will be a no-op if Commit() is called
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()

	// First soft delete the list. The row lock this takes, held until commit, is what makes
	// the in-use check below sound: an activation locks the lists it requires FOR SHARE in the
	// transaction that writes it live (AutomationRepository.UpdateIfStatus), so it either
	// committed before this update got the row, or waits for this transaction to end and then
	// reads the list as it left it.
	var name string
	listQuery := `UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL RETURNING name`
	err = tx.QueryRowContext(ctx, listQuery, now, id).Scan(&name)
	if err == sql.ErrNoRows {
		return &domain.ErrListNotFound{Message: "list not found or already deleted"}
	}
	if err != nil {
		return fmt.Errorf("failed to soft delete list: %w", err)
	}

	// Its own statement, after the lock, not part of the update above: under READ COMMITTED
	// each statement reads from a fresh snapshot, and only one taken once the lock is held
	// sees an activation that committed while this transaction waited for the row.
	users, err := liveAutomationsRequiringList(ctx, tx, workspaceID, id)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return &domain.ErrListInUse{ListID: id, ListName: name, Automations: users}
	}

	// Then soft delete all related contact_list entries
	contactListQuery := `UPDATE contact_lists SET deleted_at = $1 WHERE list_id = $2 AND deleted_at IS NULL`
	_, err = tx.ExecContext(ctx, contactListQuery, now, id)
	if err != nil {
		return fmt.Errorf("failed to soft delete contact list entries: %w", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// liveAutomationsRequiringList returns the live automations that require the list, by name.
//
// The automations are read here, in the deletion's own transaction, rather than by the
// service through the automation repository beforehand: a check made outside the transaction
// that holds the list could pass just before an activation commits. Which references count is
// Automation.RequiredListIDs, decoded from the stored row, so this and the activation check
// cannot disagree.
func liveAutomationsRequiringList(ctx context.Context, tx *sql.Tx, workspaceID, listID string) ([]domain.AutomationRef, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, COALESCE(list_id, ''), trigger_config, nodes
		FROM automations
		WHERE workspace_id = $1 AND status = 'live' AND deleted_at IS NULL
		ORDER BY name, id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to read live automations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []domain.AutomationRef
	for rows.Next() {
		var automation domain.Automation
		var triggerJSON, nodesJSON []byte
		if err := rows.Scan(&automation.ID, &automation.Name, &automation.ListID, &triggerJSON, &nodesJSON); err != nil {
			return nil, fmt.Errorf("failed to scan live automation: %w", err)
		}
		// A row that cannot be decoded cannot be cleared either: refusing is the only answer
		// that cannot break an automation.
		if len(triggerJSON) > 0 {
			if err := json.Unmarshal(triggerJSON, &automation.Trigger); err != nil {
				return nil, fmt.Errorf("failed to read the trigger of automation %s: %w", automation.ID, err)
			}
		}
		if len(nodesJSON) > 0 {
			if err := json.Unmarshal(nodesJSON, &automation.Nodes); err != nil {
				return nil, fmt.Errorf("failed to read the nodes of automation %s: %w", automation.ID, err)
			}
		}
		if slices.Contains(automation.RequiredListIDs(), listID) {
			users = append(users, domain.AutomationRef{ID: automation.ID, Name: automation.Name})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating live automations: %w", err)
	}

	return users, nil
}

func (r *listRepository) GetListStats(ctx context.Context, workspaceID string, id string) (*domain.ListStats, error) {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0) as total_active,
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as total_pending,
			COALESCE(SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END), 0) as total_unsubscribed,
			COALESCE(SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END), 0) as total_bounced,
			COALESCE(SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END), 0) as total_complained
		FROM contact_lists
		WHERE list_id = $1 AND deleted_at IS NULL
	`

	row := workspaceDB.QueryRowContext(ctx, query, id)
	stats := &domain.ListStats{}

	err = row.Scan(
		&stats.TotalActive,
		&stats.TotalPending,
		&stats.TotalUnsubscribed,
		&stats.TotalBounced,
		&stats.TotalComplained,
	)

	if err == sql.ErrNoRows {
		return nil, &domain.ErrListNotFound{Message: "list not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get list stats: %w", err)
	}

	return stats, nil
}
