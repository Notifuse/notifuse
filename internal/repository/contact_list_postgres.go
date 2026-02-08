package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

type contactListRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewContactListRepository creates a new PostgreSQL contact list repository
func NewContactListRepository(workspaceRepo domain.WorkspaceRepository) domain.ContactListRepository {
	return &contactListRepository{
		workspaceRepo: workspaceRepo,
	}
}

func (r *contactListRepository) AddContactToList(ctx context.Context, workspaceID string, contactList *domain.ContactList) error {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()
	contactList.CreatedAt = now
	contactList.UpdatedAt = now

	query := `
		INSERT INTO contact_lists (email, list_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (email, list_id) DO UPDATE
		SET status = $3, updated_at = $5, deleted_at = NULL
	`
	_, err = workspaceDB.ExecContext(ctx, query,
		contactList.Email,
		contactList.ListID,
		contactList.Status,
		contactList.CreatedAt,
		contactList.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add contact to list: %w", err)
	}

	return nil
}

// BulkAddContactsToLists adds multiple contacts to multiple lists in a single database operation
// It creates a cross-product of emails x listIDs and inserts them all at once
func (r *contactListRepository) BulkAddContactsToLists(ctx context.Context, workspaceID string, emails []string, listIDs []string, status domain.ContactListStatus) error {
	if len(emails) == 0 || len(listIDs) == 0 {
		return nil // Nothing to do
	}

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()

	// Calculate emails per batch based on cross-product size
	// Each row uses 5 params; PostgreSQL limit is 65,535 params
	emailsPerBatch := domain.BulkListAssignMaxRows / len(listIDs)
	if emailsPerBatch < 1 {
		emailsPerBatch = 1
	}

	for i := 0; i < len(emails); i += emailsPerBatch {
		end := i + emailsPerBatch
		if end > len(emails) {
			end = len(emails)
		}
		batchEmails := emails[i:end]

		var qb strings.Builder
		args := make([]interface{}, 0, len(batchEmails)*len(listIDs)*5)
		argIndex := 1

		qb.WriteString(`INSERT INTO contact_lists (email, list_id, status, created_at, updated_at, deleted_at) VALUES `)

		first := true
		for _, email := range batchEmails {
			for _, listID := range listIDs {
				if !first {
					qb.WriteString(", ")
				}
				first = false

				qb.WriteString("($")
			qb.WriteString(strconv.Itoa(argIndex))
			qb.WriteString(", $")
			qb.WriteString(strconv.Itoa(argIndex + 1))
			qb.WriteString(", $")
			qb.WriteString(strconv.Itoa(argIndex + 2))
			qb.WriteString(", $")
			qb.WriteString(strconv.Itoa(argIndex + 3))
			qb.WriteString(", $")
			qb.WriteString(strconv.Itoa(argIndex + 4))
			qb.WriteString(", NULL)")
				argIndex += 5

				args = append(args, email, listID, status, now, now)
			}
		}

		qb.WriteString(`
		ON CONFLICT (email, list_id) DO UPDATE
		SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at, deleted_at = NULL`)

		_, err = workspaceDB.ExecContext(ctx, qb.String(), args...)
		if err != nil {
			return fmt.Errorf("failed to bulk add contacts to lists: %w", err)
		}
	}

	return nil
}

func (r *contactListRepository) GetContactListByIDs(ctx context.Context, workspaceID string, email, listID string) (*domain.ContactList, error) {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT email, list_id, status, created_at, updated_at, deleted_at
		FROM contact_lists
		WHERE email = $1 AND list_id = $2 AND deleted_at IS NULL
	`

	row := workspaceDB.QueryRowContext(ctx, query, email, listID)
	contactList, err := domain.ScanContactList(row)

	if err == sql.ErrNoRows {
		return nil, &domain.ErrContactListNotFound{Message: "contact list not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact list: %w", err)
	}

	return contactList, nil
}

func (r *contactListRepository) GetContactsByListID(ctx context.Context, workspaceID string, listID string) ([]*domain.ContactList, error) {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT email, list_id, status, created_at, updated_at, deleted_at
		FROM contact_lists
		WHERE list_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := workspaceDB.QueryContext(ctx, query, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to get contacts for list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	contactLists := make([]*domain.ContactList, 0)
	for rows.Next() {
		contactList, err := domain.ScanContactList(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact list: %w", err)
		}
		contactLists = append(contactLists, contactList)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contact list rows: %w", err)
	}

	return contactLists, nil
}

func (r *contactListRepository) GetListsByEmail(ctx context.Context, workspaceID string, email string) ([]*domain.ContactList, error) {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT email, list_id, status, created_at, updated_at, deleted_at
		FROM contact_lists
		WHERE email = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := workspaceDB.QueryContext(ctx, query, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get lists for contact: %w", err)
	}
	defer func() { _ = rows.Close() }()

	contactLists := make([]*domain.ContactList, 0)
	for rows.Next() {
		contactList, err := domain.ScanContactList(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact list: %w", err)
		}
		contactLists = append(contactLists, contactList)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contact list rows: %w", err)
	}

	return contactLists, nil
}

func (r *contactListRepository) UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status domain.ContactListStatus) error {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()

	query := `
		UPDATE contact_lists
		SET status = $1, updated_at = $2, deleted_at = NULL	
		WHERE email = $3 AND list_id = $4
	`

	result, err := workspaceDB.ExecContext(ctx, query,
		status,
		now,
		email,
		listID,
	)

	if err != nil {
		return fmt.Errorf("failed to update contact list status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return &domain.ErrContactListNotFound{Message: "contact list not found"}
	}

	return nil
}

func (r *contactListRepository) RemoveContactFromList(ctx context.Context, workspaceID string, email, listID string) error {

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `UPDATE contact_lists SET deleted_at = $1 WHERE email = $2 AND list_id = $3`

	result, err := workspaceDB.ExecContext(ctx, query, time.Now().UTC(), email, listID)
	if err != nil {
		return fmt.Errorf("failed to remove contact from list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return &domain.ErrContactListNotFound{Message: "contact list not found"}
	}

	return nil
}

// DeleteForEmail deletes all contact list relationships for a specific email
func (r *contactListRepository) DeleteForEmail(ctx context.Context, workspaceID, email string) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `DELETE FROM contact_lists WHERE email = $1`

	result, err := workspaceDB.ExecContext(ctx, query, email)
	if err != nil {
		return fmt.Errorf("failed to delete contact list relationships: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	// Note: We don't return an error if no rows were affected since the contact might not have been in any lists
	_ = rows

	return nil
}
