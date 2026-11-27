package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
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

	// An opted-out row keeps its status, like the bulk path below: an unsubscribe is
	// only undone by ResubscribeContactToList, never by an ordinary add.
	query := `
		INSERT INTO contact_lists (email, list_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (email, list_id) DO UPDATE
		SET status = $3, updated_at = $5, deleted_at = NULL
		WHERE contact_lists.status NOT IN ('unsubscribed', 'bounced', 'complained')
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

// ResubscribeContactToList is AddContactToList without the guard on unsubscribed: the
// write the contact's own subscription and the explicit resubscribe flag make. A bounced
// row is overwritten only with clearBounce, and a complained row never is.
func (r *contactListRepository) ResubscribeContactToList(ctx context.Context, workspaceID string, contactList *domain.ContactList, clearBounce bool) (bool, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()
	contactList.CreatedAt = now
	contactList.UpdatedAt = now

	query := `
		INSERT INTO contact_lists (email, list_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (email, list_id) DO UPDATE
		SET status = $3, updated_at = $5, deleted_at = NULL
		WHERE contact_lists.status <> 'complained'
		AND (contact_lists.status <> 'bounced' OR $6)
	`
	result, err := workspaceDB.ExecContext(ctx, query,
		contactList.Email,
		contactList.ListID,
		contactList.Status,
		contactList.CreatedAt,
		contactList.UpdatedAt,
		clearBounce,
	)
	if err != nil {
		return false, fmt.Errorf("failed to resubscribe contact to list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rows > 0, nil
}

// RestoreUnsubscribedContactList atomically moves a non-removed unsubscribed row back to
// active. Putting both status = 'unsubscribed' and deleted_at IS NULL in the UPDATE's own
// WHERE prevents a concurrent admin removal, bounce, or complaint from being overwritten.
func (r *contactListRepository) RestoreUnsubscribedContactList(ctx context.Context, workspaceID, email, listID string) (bool, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE contact_lists
		SET status = 'active', updated_at = $1
		WHERE email = $2 AND list_id = $3
		AND status = 'unsubscribed'
		AND deleted_at IS NULL
	`
	result, err := workspaceDB.ExecContext(ctx, query, time.Now().UTC(), email, listID)
	if err != nil {
		return false, fmt.Errorf("failed to restore unsubscribed contact list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rows > 0, nil
}

// UnsubscribeActiveOrPendingContactList atomically transitions a non-removed active or
// pending membership to unsubscribed and reports whether that transition happened. Using
// a single UPDATE's WHERE clause guarantees undo tokens and private list names are only
// emitted when a genuine active/pending -> unsubscribed transition occurred.
func (r *contactListRepository) UnsubscribeActiveOrPendingContactList(ctx context.Context, workspaceID, email, listID string) (bool, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE contact_lists
		SET status = 'unsubscribed', updated_at = $1
		WHERE email = $2 AND list_id = $3
		AND status IN ('active', 'pending')
		AND deleted_at IS NULL
	`
	result, err := workspaceDB.ExecContext(ctx, query, time.Now().UTC(), email, listID)
	if err != nil {
		return false, fmt.Errorf("failed to unsubscribe active or pending contact list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rows > 0, nil
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
		SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at, deleted_at = NULL
		WHERE contact_lists.status NOT IN ('unsubscribed', 'bounced', 'complained')`)

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

// GetContactListIncludingRemoved returns the contact's row on the list even when they
// were removed from it (DeletedAt set). A removal ends membership, so GetContactListByIDs
// hides the row, but it doesn't end the unsubscribe, bounce or complaint recorded on it.
func (r *contactListRepository) GetContactListIncludingRemoved(ctx context.Context, workspaceID string, email, listID string) (*domain.ContactList, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT email, list_id, status, created_at, updated_at, deleted_at
		FROM contact_lists
		WHERE email = $1 AND list_id = $2
	`

	contactList, err := domain.ScanContactList(workspaceDB.QueryRowContext(ctx, query, email, listID))
	if err == sql.ErrNoRows {
		return nil, &domain.ErrContactListNotFound{Message: "contact list not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact list: %w", err)
	}

	return contactList, nil
}

// optOutsRestoredOnInsertSQL selects what restore_contact_opt_outs would insert for the
// address, on the given lists, if it were inserted into contacts now. The hash is computed
// by contact_opt_out_hash() in SQL, as the trigger does.
const optOutsRestoredOnInsertSQL = `SELECT o.list_id, o.status, o.removed
	FROM contact_opt_out_hashes o
	JOIN lists l ON l.id = o.list_id AND l.deleted_at IS NULL
	WHERE o.email_hash = contact_opt_out_hash($1)
	AND o.list_id = ANY($2::text[])
	AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.email = $1)
	ORDER BY o.list_id`

// GetOptOutsRestoredOnInsert returns the opt-outs a deleted contact left on the given
// lists, as the rows inserting the address would put back now.
func (r *contactListRepository) GetOptOutsRestoredOnInsert(ctx context.Context, workspaceID string, email string, listIDs []string) ([]*domain.ContactList, error) {
	if len(listIDs) == 0 {
		return nil, nil
	}
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	rows, err := workspaceDB.QueryContext(ctx, optOutsRestoredOnInsertSQL, email, pq.Array(listIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to read stored opt-outs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	now := time.Now().UTC()
	var restored []*domain.ContactList
	for rows.Next() {
		var listID, status string
		var removed bool
		if err := rows.Scan(&listID, &status, &removed); err != nil {
			return nil, fmt.Errorf("failed to scan stored opt-out: %w", err)
		}
		row := &domain.ContactList{
			Email:     email,
			ListID:    listID,
			Status:    domain.ContactListStatus(status),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if removed {
			deletedAt := now
			row.DeletedAt = &deletedAt
		}
		restored = append(restored, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stored opt-outs: %w", err)
	}
	return restored, nil
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

	// The status rule of domain.ContactListStatusTransitionAllowed, in SQL, so that a
	// row opted out between the caller's read and this write still keeps its opt-out.
	query := `
		UPDATE contact_lists
		SET status = $1, updated_at = $2, deleted_at = NULL
		WHERE email = $3 AND list_id = $4
		AND (
			status = $1
			OR status NOT IN ('unsubscribed', 'bounced', 'complained')
			OR (status = 'unsubscribed' AND $1 IN ('bounced', 'complained'))
			OR (status = 'bounced' AND $1 = 'complained')
		)
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
		// No row, or one the guard kept: read it to tell the two apart.
		var stored domain.ContactListStatus
		err := workspaceDB.QueryRowContext(ctx,
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, listID,
		).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.ErrContactListNotFound{Message: "contact list not found"}
		}
		if err != nil {
			return fmt.Errorf("failed to read contact list status: %w", err)
		}
		if !domain.IsOptOutContactListStatus(stored) {
			// The row changed between the update and this read: not a refusal to report.
			return fmt.Errorf("contact list status changed concurrently (now %s), retry", stored)
		}
		return &domain.ErrContactListOptedOut{Status: stored}
	}

	return nil
}

// MarkContactListUnsubscribed records an opt-out without the read-then-write that would
// let a complaint saved in between be overwritten: the statement's own WHERE keeps a
// bounce or complaint. It leaves deleted_at alone, so an unsubscribe link in an email sent
// before a contact's removal doesn't add them back to the list. A non-zero unchangedSince
// also keeps a row changed after it.
func (r *contactListRepository) MarkContactListUnsubscribed(ctx context.Context, workspaceID string, email, listID string, unchangedSince time.Time) (bool, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		UPDATE contact_lists
		SET status = 'unsubscribed', updated_at = $1
		WHERE email = $2 AND list_id = $3
		AND status NOT IN ('bounced', 'complained')
		AND ($4::timestamptz IS NULL OR updated_at <= $4::timestamptz)
	`

	var since interface{}
	if !unchangedSince.IsZero() {
		since = unchangedSince
	}

	result, err := workspaceDB.ExecContext(ctx, query, time.Now().UTC(), email, listID, since)
	if err != nil {
		return false, fmt.Errorf("failed to unsubscribe contact from list: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rows > 0, nil
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
