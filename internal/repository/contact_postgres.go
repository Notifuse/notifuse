package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"encoding/base64"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
)

type contactRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// contactColumns defines the explicit column list for contacts table.
// IMPORTANT: This order MUST match the ScanContact function in domain/contact.go.
// Using explicit columns instead of SELECT * ensures consistent ordering
// regardless of how columns were added to the table (CREATE TABLE vs ALTER TABLE).
var contactColumns = []string{
	"email", "external_id", "timezone", "language",
	"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
	"country", "postcode", "state", "job_title",
	"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
	"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
	"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
	"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
	"created_at", "updated_at", "db_created_at", "db_updated_at",
}

// contactColumnsWithPrefix returns contact columns prefixed with a table alias
func contactColumnsWithPrefix(prefix string) []string {
	cols := make([]string, len(contactColumns))
	for i, col := range contactColumns {
		cols[i] = prefix + "." + col
	}
	return cols
}

// NewContactRepository creates a new PostgreSQL contact repository
func NewContactRepository(workspaceRepo domain.WorkspaceRepository) domain.ContactRepository {
	return &contactRepository{
		workspaceRepo: workspaceRepo,
	}
}

func (r *contactRepository) GetContactByEmail(ctx context.Context, workspaceID, email string) (*domain.Contact, error) {
	filter := sq.Eq{"c.email": email}
	return r.fetchContact(ctx, workspaceID, filter)
}

func (r *contactRepository) GetContactByExternalID(ctx context.Context, workspaceID string, externalID string) (*domain.Contact, error) {
	filter := sq.Eq{"c.external_id": externalID}
	return r.fetchContact(ctx, workspaceID, filter)
}

// fetchContact is a private helper method to fetch a single contact by a given filter
func (r *contactRepository) fetchContact(ctx context.Context, workspaceID string, filter sq.Sqlizer) (*domain.Contact, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query, args, err := psql.Select(contactColumnsWithPrefix("c")...).
		From("contacts c").
		Where(filter).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	row := db.QueryRowContext(ctx, query, args...)

	contact, err := domain.ScanContact(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrContactNotFound
		}
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}

	// Fetch contact lists for this contact
	listsQuery, listsArgs, err := psql.Select("cl.list_id", "cl.status", "cl.created_at", "cl.updated_at", "cl.deleted_at", "l.name as list_name").
		From("contact_lists cl").
		Join("lists l ON cl.list_id = l.id").
		Where(sq.Eq{"cl.email": contact.Email}).
		Where(sq.Eq{"l.deleted_at": nil}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build contact lists query: %w", err)
	}

	rows, err := db.QueryContext(ctx, listsQuery, listsArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact lists: %w", err)
	}
	defer func() { _ = rows.Close() }()

	contact.ContactLists = []*domain.ContactList{}
	for rows.Next() {
		var contactList domain.ContactList
		var deletedAt *time.Time
		var listName string
		err := rows.Scan(
			&contactList.ListID,
			&contactList.Status,
			&contactList.CreatedAt,
			&contactList.UpdatedAt,
			&deletedAt,
			&listName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact list: %w", err)
		}
		contactList.Email = contact.Email
		contactList.DeletedAt = deletedAt
		contactList.ListName = listName
		contact.ContactLists = append(contact.ContactLists, &contactList)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contact lists: %w", err)
	}

	// Fetch contact segments for this contact
	segmentsQuery, segmentsArgs, err := psql.Select("cs.segment_id", "cs.version", "cs.matched_at", "cs.computed_at", "s.name as segment_name", "s.color as segment_color").
		From("contact_segments cs").
		Join("segments s ON cs.segment_id = s.id").
		Where(sq.Eq{"cs.email": contact.Email}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build contact segments query: %w", err)
	}

	segmentRows, err := db.QueryContext(ctx, segmentsQuery, segmentsArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact segments: %w", err)
	}
	defer func() {
		_ = segmentRows.Close()
	}()

	contact.ContactSegments = []*domain.ContactSegment{}
	for segmentRows.Next() {
		var contactSegment domain.ContactSegment
		var segmentName, segmentColor string
		err := segmentRows.Scan(
			&contactSegment.SegmentID,
			&contactSegment.Version,
			&contactSegment.MatchedAt,
			&contactSegment.ComputedAt,
			&segmentName,
			&segmentColor,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact segment: %w", err)
		}
		contactSegment.Email = contact.Email
		contact.ContactSegments = append(contact.ContactSegments, &contactSegment)
	}

	if err = segmentRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contact segments: %w", err)
	}

	return contact, nil
}

func (r *contactRepository) GetContacts(ctx context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	sb := psql.Select(contactColumnsWithPrefix("c")...).From("contacts c")

	// Add filters using squirrel
	if req.Email != "" {
		sb = sb.Where(sq.ILike{"c.email": "%" + req.Email + "%"})
	}
	if req.ExternalID != "" {
		sb = sb.Where(sq.ILike{"c.external_id": "%" + req.ExternalID + "%"})
	}
	if req.FirstName != "" {
		sb = sb.Where(sq.ILike{"c.first_name": "%" + req.FirstName + "%"})
	}
	if req.LastName != "" {
		sb = sb.Where(sq.ILike{"c.last_name": "%" + req.LastName + "%"})
	}
	if req.FullName != "" {
		sb = sb.Where(sq.ILike{"c.full_name": "%" + req.FullName + "%"})
	}
	if req.Phone != "" {
		sb = sb.Where(sq.ILike{"c.phone": "%" + req.Phone + "%"})
	}
	if req.Country != "" {
		sb = sb.Where(sq.ILike{"c.country": "%" + req.Country + "%"})
	}
	if req.Language != "" {
		sb = sb.Where(sq.ILike{"c.language": "%" + req.Language + "%"})
	}

	// Use EXISTS subquery for list_id and contact_list_status filters instead of JOIN
	if req.ListID != "" || req.ContactListStatus != "" {
		// Build the EXISTS clause manually with ? placeholders
		// Squirrel will convert these to the correct $N placeholders
		var existsClause string
		var args []interface{}

		if req.ListID != "" && req.ContactListStatus != "" {
			// Both list_id and status
			existsClause = "EXISTS (SELECT 1 FROM contact_lists cl WHERE cl.email = c.email AND cl.deleted_at IS NULL AND cl.list_id = ? AND cl.status = ?)"
			args = []interface{}{req.ListID, req.ContactListStatus}
		} else if req.ListID != "" {
			// Just list_id
			existsClause = "EXISTS (SELECT 1 FROM contact_lists cl WHERE cl.email = c.email AND cl.deleted_at IS NULL AND cl.list_id = ?)"
			args = []interface{}{req.ListID}
		} else if req.ContactListStatus != "" {
			// Just status
			existsClause = "EXISTS (SELECT 1 FROM contact_lists cl WHERE cl.email = c.email AND cl.deleted_at IS NULL AND cl.status = ?)"
			args = []interface{}{req.ContactListStatus}
		}

		sb = sb.Where(sq.Expr(existsClause, args...))
	}

	// Use EXISTS subquery for segments filter
	if len(req.Segments) > 0 {
		// Build the placeholder string for the IN clause using ? placeholders
		// Squirrel will convert these to the correct $N placeholders
		placeholders := make([]string, len(req.Segments))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		placeholdersStr := strings.Join(placeholders, ",")

		// Build the EXISTS clause with ? placeholders
		existsClause := fmt.Sprintf("EXISTS (SELECT 1 FROM contact_segments cs JOIN segments s ON cs.segment_id = s.id WHERE cs.email = c.email AND cs.segment_id IN (%s))", placeholdersStr)

		// Convert []string to []interface{} for sq.Expr
		args := make([]interface{}, len(req.Segments))
		for i, seg := range req.Segments {
			args[i] = seg
		}

		sb = sb.Where(sq.Expr(existsClause, args...))
	}

	if req.Cursor != "" {
		// Decode the base64 cursor
		decodedCursor, err := base64.StdEncoding.DecodeString(req.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor encoding: %w", err)
		}

		// Parse the compound cursor (timestamp~email)
		cursorStr := string(decodedCursor)
		cursorParts := strings.Split(cursorStr, "~")
		if len(cursorParts) != 2 {
			return nil, fmt.Errorf("invalid cursor format: expected timestamp~email")
		}

		cursorTime, err := time.Parse(time.RFC3339Nano, cursorParts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid cursor timestamp format: %w", err)
		}

		cursorEmail := cursorParts[1]

		// Use a compound condition for pagination:
		// Either created_at is less than cursor time
		// OR created_at equals cursor time AND email is greater than cursor email (for lexicographical ordering)
		sb = sb.Where(
			sq.Or{
				sq.Lt{"c.created_at": cursorTime},
				sq.And{
					sq.Eq{"c.created_at": cursorTime},
					sq.Gt{"c.email": cursorEmail},
				},
			},
		)
	}

	// Add order by with a compound sort (created_at DESC, email ASC) to ensure deterministic ordering
	sb = sb.OrderBy("c.created_at DESC", "c.email ASC").Limit(uint64(req.Limit + 1)) // Get one extra

	// Build the final query
	query, args, err := sb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Execute query
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Process results
	var contacts []*domain.Contact
	var nextCursor string

	for rows.Next() {
		contact, err := domain.ScanContact(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact: %w", err)
		}
		contacts = append(contacts, contact)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	// Handle pagination
	if len(contacts) > req.Limit {
		// Remove the extra contact we fetched
		lastContact := contacts[req.Limit-1]
		contacts = contacts[:req.Limit]

		// Create a compound cursor with timestamp and email using tilde as separator
		// Use RFC3339Nano to preserve nanosecond precision and avoid skipping contacts created within the same second
		cursorStr := fmt.Sprintf("%s~%s", lastContact.CreatedAt.Format(time.RFC3339Nano), lastContact.Email)

		// Base64 encode the cursor to make it URL-friendly
		nextCursor = base64.StdEncoding.EncodeToString([]byte(cursorStr))
	}

	// If WithContactLists is true, fetch contact lists in a separate query
	if req.WithContactLists && len(contacts) > 0 {
		// Build list of contact emails
		emails := make([]string, len(contacts))
		for i, contact := range contacts {
			emails[i] = contact.Email
		}

		// Query for ALL contact lists for these contacts, regardless of filter criteria
		listQueryBuilder := psql.Select("cl.email, cl.list_id, cl.status, cl.created_at, cl.updated_at, l.name as list_name").
			From("contact_lists cl").
			Join("lists l ON cl.list_id = l.id").
			Where(sq.Eq{"cl.email": emails}).   // squirrel handles IN clauses automatically
			Where(sq.Eq{"cl.deleted_at": nil}). // Filter out deleted contact_list entries
			Where(sq.Eq{"l.deleted_at": nil})   // Filter out deleted lists

		// We no longer apply the ListID and ContactListStatus filters here
		// This way, we show ALL lists for each contact, not just the ones that match the filter

		listQuery, listArgs, err := listQueryBuilder.ToSql()
		if err != nil {
			return nil, fmt.Errorf("failed to build contact list query: %w", err)
		}

		listRows, err := db.QueryContext(ctx, listQuery, listArgs...)
		if err != nil {
			return nil, fmt.Errorf("failed to query contact lists: %w", err)
		}
		defer func() {
			_ = listRows.Close()
		}()

		// Create a map of contacts by email for quick lookup
		contactMap := make(map[string]*domain.Contact)
		for _, contact := range contacts {
			contact.ContactLists = []*domain.ContactList{}
			contactMap[contact.Email] = contact
		}

		// Process contact list results
		for listRows.Next() {
			var email string
			var list domain.ContactList
			var listName string
			err := listRows.Scan(&email, &list.ListID, &list.Status, &list.CreatedAt, &list.UpdatedAt, &listName)
			if err != nil {
				return nil, fmt.Errorf("failed to scan contact list: %w", err)
			}

			list.ListName = listName
			if contact, ok := contactMap[email]; ok {
				contact.ContactLists = append(contact.ContactLists, &list)
			}
		}

		if err = listRows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating over contact list rows: %w", err)
		}
	}

	// Fetch contact segments for all contacts (always included)
	if len(contacts) > 0 {
		// Build list of contact emails
		emails := make([]string, len(contacts))
		for i, contact := range contacts {
			emails[i] = contact.Email
		}

		// Query for ALL contact segments for these contacts
		segmentQueryBuilder := psql.Select("cs.email", "cs.segment_id", "cs.version", "cs.matched_at", "cs.computed_at", "s.name as segment_name", "s.color as segment_color").
			From("contact_segments cs").
			Join("segments s ON cs.segment_id = s.id").
			Where(sq.Eq{"cs.email": emails}) // squirrel handles IN clauses automatically

		segmentQuery, segmentArgs, err := segmentQueryBuilder.ToSql()
		if err != nil {
			return nil, fmt.Errorf("failed to build contact segment query: %w", err)
		}

		segmentRows, err := db.QueryContext(ctx, segmentQuery, segmentArgs...)
		if err != nil {
			return nil, fmt.Errorf("failed to query contact segments: %w", err)
		}
		defer func() {
			_ = segmentRows.Close()
		}()

		// Create/use the map of contacts by email for quick lookup
		contactMap := make(map[string]*domain.Contact)
		for _, contact := range contacts {
			contact.ContactSegments = []*domain.ContactSegment{}
			contactMap[contact.Email] = contact
		}

		// Process contact segment results
		for segmentRows.Next() {
			var email string
			var segment domain.ContactSegment
			var segmentName, segmentColor string
			err := segmentRows.Scan(&email, &segment.SegmentID, &segment.Version, &segment.MatchedAt, &segment.ComputedAt, &segmentName, &segmentColor)
			if err != nil {
				return nil, fmt.Errorf("failed to scan contact segment: %w", err)
			}

			segment.Email = email
			if contact, ok := contactMap[email]; ok {
				contact.ContactSegments = append(contact.ContactSegments, &segment)
			}
		}

		if err = segmentRows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating over contact segment rows: %w", err)
		}
	}

	return &domain.GetContactsResponse{
		Contacts:   contacts,
		NextCursor: nextCursor,
	}, nil
}

func (r *contactRepository) DeleteContact(ctx context.Context, workspaceID string, email string) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query, args, err := psql.Delete("contacts").
		Where(sq.Eq{"email": email}).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	result, err := workspaceDB.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("contact not found")
	}

	return nil
}

func (r *contactRepository) UpsertContact(ctx context.Context, workspaceID string, contact *domain.Contact) (isNew bool, err error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Use squirrel placeholder format
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Start a transaction
	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Rollback if there's a panic or error

	// Check if contact exists with FOR UPDATE lock using squirrel
	selectQuery, selectArgs, err := psql.Select(contactColumnsWithPrefix("c")...).
		From("contacts c").
		Where(sq.Eq{"c.email": contact.Email}).
		Suffix("FOR UPDATE").
		ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build select for update query: %w", err)
	}

	var existingContact *domain.Contact
	row := tx.QueryRowContext(ctx, selectQuery, selectArgs...)
	existingContact, err = domain.ScanContact(row)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("failed to check existing contact: %w", err)
		}

		// --- INSERT path ---
		isNew = true

		// Set DB timestamps
		now := time.Now().UTC()
		contact.DBCreatedAt = now
		contact.DBUpdatedAt = now

		// Convert domain nullable types to SQL nullable types
		var externalIDSQL, timezoneSQL, languageSQL sql.NullString
		var firstNameSQL, lastNameSQL, fullNameSQL, phoneSQL, addressLine1SQL, addressLine2SQL sql.NullString
		var countrySQL, postcodeSQL, stateSQL, jobTitleSQL sql.NullString
		var customString1SQL, customString2SQL, customString3SQL, customString4SQL, customString5SQL sql.NullString
		var customNumber1SQL, customNumber2SQL, customNumber3SQL, customNumber4SQL, customNumber5SQL sql.NullFloat64
		var customDatetime1SQL, customDatetime2SQL, customDatetime3SQL, customDatetime4SQL, customDatetime5SQL sql.NullTime
		var customJSON1SQL, customJSON2SQL, customJSON3SQL, customJSON4SQL, customJSON5SQL sql.NullString

		// String fields
		if contact.ExternalID != nil {
			if !contact.ExternalID.IsNull {
				externalIDSQL = sql.NullString{String: contact.ExternalID.String, Valid: true}
			} else {
				externalIDSQL = sql.NullString{Valid: false}
			}
		}
		if contact.Timezone != nil {
			if !contact.Timezone.IsNull {
				timezoneSQL = sql.NullString{String: contact.Timezone.String, Valid: true}
			} else {
				timezoneSQL = sql.NullString{Valid: false}
			}
		}
		if contact.Language != nil {
			if !contact.Language.IsNull {
				languageSQL = sql.NullString{String: contact.Language.String, Valid: true}
			} else {
				languageSQL = sql.NullString{Valid: false}
			}
		}
		if contact.FirstName != nil {
			if !contact.FirstName.IsNull {
				firstNameSQL = sql.NullString{String: contact.FirstName.String, Valid: true}
			} else {
				firstNameSQL = sql.NullString{Valid: false}
			}
		}
		if contact.LastName != nil {
			if !contact.LastName.IsNull {
				lastNameSQL = sql.NullString{String: contact.LastName.String, Valid: true}
			} else {
				lastNameSQL = sql.NullString{Valid: false}
			}
		}
		if contact.FullName != nil {
			if !contact.FullName.IsNull {
				fullNameSQL = sql.NullString{String: contact.FullName.String, Valid: true}
			} else {
				fullNameSQL = sql.NullString{Valid: false}
			}
		}
		if contact.Phone != nil {
			if !contact.Phone.IsNull {
				phoneSQL = sql.NullString{String: contact.Phone.String, Valid: true}
			} else {
				phoneSQL = sql.NullString{Valid: false}
			}
		}
		if contact.AddressLine1 != nil {
			if !contact.AddressLine1.IsNull {
				addressLine1SQL = sql.NullString{String: contact.AddressLine1.String, Valid: true}
			} else {
				addressLine1SQL = sql.NullString{Valid: false}
			}
		}
		if contact.AddressLine2 != nil {
			if !contact.AddressLine2.IsNull {
				addressLine2SQL = sql.NullString{String: contact.AddressLine2.String, Valid: true}
			} else {
				addressLine2SQL = sql.NullString{Valid: false}
			}
		}
		if contact.Country != nil {
			if !contact.Country.IsNull {
				countrySQL = sql.NullString{String: contact.Country.String, Valid: true}
			} else {
				countrySQL = sql.NullString{Valid: false}
			}
		}
		if contact.Postcode != nil {
			if !contact.Postcode.IsNull {
				postcodeSQL = sql.NullString{String: contact.Postcode.String, Valid: true}
			} else {
				postcodeSQL = sql.NullString{Valid: false}
			}
		}
		if contact.State != nil {
			if !contact.State.IsNull {
				stateSQL = sql.NullString{String: contact.State.String, Valid: true}
			} else {
				stateSQL = sql.NullString{Valid: false}
			}
		}
		if contact.JobTitle != nil {
			if !contact.JobTitle.IsNull {
				jobTitleSQL = sql.NullString{String: contact.JobTitle.String, Valid: true}
			} else {
				jobTitleSQL = sql.NullString{Valid: false}
			}
		}

		// Custom string fields
		if contact.CustomString1 != nil {
			if !contact.CustomString1.IsNull {
				customString1SQL = sql.NullString{String: contact.CustomString1.String, Valid: true}
			} else {
				customString1SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomString2 != nil {
			if !contact.CustomString2.IsNull {
				customString2SQL = sql.NullString{String: contact.CustomString2.String, Valid: true}
			} else {
				customString2SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomString3 != nil {
			if !contact.CustomString3.IsNull {
				customString3SQL = sql.NullString{String: contact.CustomString3.String, Valid: true}
			} else {
				customString3SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomString4 != nil {
			if !contact.CustomString4.IsNull {
				customString4SQL = sql.NullString{String: contact.CustomString4.String, Valid: true}
			} else {
				customString4SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomString5 != nil {
			if !contact.CustomString5.IsNull {
				customString5SQL = sql.NullString{String: contact.CustomString5.String, Valid: true}
			} else {
				customString5SQL = sql.NullString{Valid: false}
			}
		}

		// Custom number fields
		if contact.CustomNumber1 != nil {
			if !contact.CustomNumber1.IsNull {
				customNumber1SQL = sql.NullFloat64{Float64: contact.CustomNumber1.Float64, Valid: true}
			} else {
				customNumber1SQL = sql.NullFloat64{Valid: false}
			}
		}
		if contact.CustomNumber2 != nil {
			if !contact.CustomNumber2.IsNull {
				customNumber2SQL = sql.NullFloat64{Float64: contact.CustomNumber2.Float64, Valid: true}
			} else {
				customNumber2SQL = sql.NullFloat64{Valid: false}
			}
		}
		if contact.CustomNumber3 != nil {
			if !contact.CustomNumber3.IsNull {
				customNumber3SQL = sql.NullFloat64{Float64: contact.CustomNumber3.Float64, Valid: true}
			} else {
				customNumber3SQL = sql.NullFloat64{Valid: false}
			}
		}
		if contact.CustomNumber4 != nil {
			if !contact.CustomNumber4.IsNull {
				customNumber4SQL = sql.NullFloat64{Float64: contact.CustomNumber4.Float64, Valid: true}
			} else {
				customNumber4SQL = sql.NullFloat64{Valid: false}
			}
		}
		if contact.CustomNumber5 != nil {
			if !contact.CustomNumber5.IsNull {
				customNumber5SQL = sql.NullFloat64{Float64: contact.CustomNumber5.Float64, Valid: true}
			} else {
				customNumber5SQL = sql.NullFloat64{Valid: false}
			}
		}

		// Custom datetime fields
		if contact.CustomDatetime1 != nil {
			if !contact.CustomDatetime1.IsNull {
				customDatetime1SQL = sql.NullTime{Time: contact.CustomDatetime1.Time, Valid: true}
			} else {
				customDatetime1SQL = sql.NullTime{Valid: false}
			}
		}
		if contact.CustomDatetime2 != nil {
			if !contact.CustomDatetime2.IsNull {
				customDatetime2SQL = sql.NullTime{Time: contact.CustomDatetime2.Time, Valid: true}
			} else {
				customDatetime2SQL = sql.NullTime{Valid: false}
			}
		}
		if contact.CustomDatetime3 != nil {
			if !contact.CustomDatetime3.IsNull {
				customDatetime3SQL = sql.NullTime{Time: contact.CustomDatetime3.Time, Valid: true}
			} else {
				customDatetime3SQL = sql.NullTime{Valid: false}
			}
		}
		if contact.CustomDatetime4 != nil {
			if !contact.CustomDatetime4.IsNull {
				customDatetime4SQL = sql.NullTime{Time: contact.CustomDatetime4.Time, Valid: true}
			} else {
				customDatetime4SQL = sql.NullTime{Valid: false}
			}
		}
		if contact.CustomDatetime5 != nil {
			if !contact.CustomDatetime5.IsNull {
				customDatetime5SQL = sql.NullTime{Time: contact.CustomDatetime5.Time, Valid: true}
			} else {
				customDatetime5SQL = sql.NullTime{Valid: false}
			}
		}

		// Custom JSON fields
		if contact.CustomJSON1 != nil {
			if !contact.CustomJSON1.IsNull {
				jsonBytes, err := json.Marshal(contact.CustomJSON1.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_1: %w", err)
				}
				customJSON1SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			} else {
				customJSON1SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomJSON2 != nil {
			if !contact.CustomJSON2.IsNull {
				jsonBytes, err := json.Marshal(contact.CustomJSON2.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_2: %w", err)
				}
				customJSON2SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			} else {
				customJSON2SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomJSON3 != nil {
			if !contact.CustomJSON3.IsNull {
				jsonBytes, err := json.Marshal(contact.CustomJSON3.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_3: %w", err)
				}
				customJSON3SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			} else {
				customJSON3SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomJSON4 != nil {
			if !contact.CustomJSON4.IsNull {
				jsonBytes, err := json.Marshal(contact.CustomJSON4.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_4: %w", err)
				}
				customJSON4SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			} else {
				customJSON4SQL = sql.NullString{Valid: false}
			}
		}
		if contact.CustomJSON5 != nil {
			if !contact.CustomJSON5.IsNull {
				jsonBytes, err := json.Marshal(contact.CustomJSON5.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_5: %w", err)
				}
				customJSON5SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			} else {
				customJSON5SQL = sql.NullString{Valid: false}
			}
		}

		// Build insert query using squirrel
		// Note: We include all columns in one call to avoid misalignment issues
		createdAtValue := contact.CreatedAt
		if createdAtValue.IsZero() {
			createdAtValue = contact.DBCreatedAt // Use db timestamp if not provided
		}
		updatedAtValue := contact.UpdatedAt
		if updatedAtValue.IsZero() {
			updatedAtValue = contact.DBUpdatedAt // Use db timestamp if not provided
		}

		insertBuilder := psql.Insert("contacts").
			Columns(
				"email", "external_id", "timezone", "language",
				"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
				"country", "postcode", "state", "job_title",
				"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
				"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
				"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
				"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
				"created_at", "updated_at", "db_created_at", "db_updated_at",
			).
			Values(
				contact.Email, externalIDSQL, timezoneSQL, languageSQL,
				firstNameSQL, lastNameSQL, fullNameSQL, phoneSQL, addressLine1SQL, addressLine2SQL,
				countrySQL, postcodeSQL, stateSQL, jobTitleSQL,
				customString1SQL, customString2SQL, customString3SQL, customString4SQL, customString5SQL,
				customNumber1SQL, customNumber2SQL, customNumber3SQL, customNumber4SQL, customNumber5SQL,
				customDatetime1SQL, customDatetime2SQL, customDatetime3SQL, customDatetime4SQL, customDatetime5SQL,
				customJSON1SQL, customJSON2SQL, customJSON3SQL, customJSON4SQL, customJSON5SQL,
				createdAtValue.UTC(), updatedAtValue.UTC(), contact.DBCreatedAt, contact.DBUpdatedAt,
			)

		insertQuery, insertArgs, err := insertBuilder.ToSql()
		if err != nil {
			return false, fmt.Errorf("failed to build insert query: %w", err)
		}

		// Execute the insert query within the transaction
		_, err = tx.ExecContext(ctx, insertQuery, insertArgs...)
		if err != nil {
			// Check if the error is a constraint violation or similar if needed
			return false, fmt.Errorf("failed to insert contact: %w", err)
		}

	} else {
		// --- UPDATE path ---
		isNew = false

		// Update DB timestamps
		existingContact.DBUpdatedAt = time.Now().UTC()

		// Merge changes from the input 'contact' into the 'existingContact'
		existingContact.Merge(contact)

		// Convert domain nullable types to SQL nullable types for the update
		var externalIDSQL, timezoneSQL, languageSQL sql.NullString
		var firstNameSQL, lastNameSQL, fullNameSQL, phoneSQL, addressLine1SQL, addressLine2SQL sql.NullString
		var countrySQL, postcodeSQL, stateSQL, jobTitleSQL sql.NullString
		var customString1SQL, customString2SQL, customString3SQL, customString4SQL, customString5SQL sql.NullString
		var customNumber1SQL, customNumber2SQL, customNumber3SQL, customNumber4SQL, customNumber5SQL sql.NullFloat64
		var customDatetime1SQL, customDatetime2SQL, customDatetime3SQL, customDatetime4SQL, customDatetime5SQL sql.NullTime
		var customJSON1SQL, customJSON2SQL, customJSON3SQL, customJSON4SQL, customJSON5SQL sql.NullString

		// Convert external ID, timezone, language
		if existingContact.ExternalID != nil {
			if !existingContact.ExternalID.IsNull {
				externalIDSQL = sql.NullString{String: existingContact.ExternalID.String, Valid: true}
			}
		}
		if existingContact.Timezone != nil {
			if !existingContact.Timezone.IsNull {
				timezoneSQL = sql.NullString{String: existingContact.Timezone.String, Valid: true}
			}
		}
		if existingContact.Language != nil {
			if !existingContact.Language.IsNull {
				languageSQL = sql.NullString{String: existingContact.Language.String, Valid: true}
			}
		}

		// Convert string fields
		if existingContact.FirstName != nil {
			if !existingContact.FirstName.IsNull {
				firstNameSQL = sql.NullString{String: existingContact.FirstName.String, Valid: true}
			}
		}
		if existingContact.LastName != nil {
			if !existingContact.LastName.IsNull {
				lastNameSQL = sql.NullString{String: existingContact.LastName.String, Valid: true}
			}
		}
		if existingContact.FullName != nil {
			if !existingContact.FullName.IsNull {
				fullNameSQL = sql.NullString{String: existingContact.FullName.String, Valid: true}
			}
		}
		if existingContact.Phone != nil {
			if !existingContact.Phone.IsNull {
				phoneSQL = sql.NullString{String: existingContact.Phone.String, Valid: true}
			}
		}
		if existingContact.AddressLine1 != nil {
			if !existingContact.AddressLine1.IsNull {
				addressLine1SQL = sql.NullString{String: existingContact.AddressLine1.String, Valid: true}
			}
		}
		if existingContact.AddressLine2 != nil {
			if !existingContact.AddressLine2.IsNull {
				addressLine2SQL = sql.NullString{String: existingContact.AddressLine2.String, Valid: true}
			}
		}
		if existingContact.Country != nil {
			if !existingContact.Country.IsNull {
				countrySQL = sql.NullString{String: existingContact.Country.String, Valid: true}
			}
		}
		if existingContact.Postcode != nil {
			if !existingContact.Postcode.IsNull {
				postcodeSQL = sql.NullString{String: existingContact.Postcode.String, Valid: true}
			}
		}
		if existingContact.State != nil {
			if !existingContact.State.IsNull {
				stateSQL = sql.NullString{String: existingContact.State.String, Valid: true}
			}
		}
		if existingContact.JobTitle != nil {
			if !existingContact.JobTitle.IsNull {
				jobTitleSQL = sql.NullString{String: existingContact.JobTitle.String, Valid: true}
			}
		}

		// Convert custom string fields
		if existingContact.CustomString1 != nil {
			if !existingContact.CustomString1.IsNull {
				customString1SQL = sql.NullString{String: existingContact.CustomString1.String, Valid: true}
			}
		}
		if existingContact.CustomString2 != nil {
			if !existingContact.CustomString2.IsNull {
				customString2SQL = sql.NullString{String: existingContact.CustomString2.String, Valid: true}
			}
		}
		if existingContact.CustomString3 != nil {
			if !existingContact.CustomString3.IsNull {
				customString3SQL = sql.NullString{String: existingContact.CustomString3.String, Valid: true}
			}
		}
		if existingContact.CustomString4 != nil {
			if !existingContact.CustomString4.IsNull {
				customString4SQL = sql.NullString{String: existingContact.CustomString4.String, Valid: true}
			}
		}
		if existingContact.CustomString5 != nil {
			if !existingContact.CustomString5.IsNull {
				customString5SQL = sql.NullString{String: existingContact.CustomString5.String, Valid: true}
			}
		}

		// Convert custom number fields
		if existingContact.CustomNumber1 != nil {
			if !existingContact.CustomNumber1.IsNull {
				customNumber1SQL = sql.NullFloat64{Float64: existingContact.CustomNumber1.Float64, Valid: true}
			}
		}
		if existingContact.CustomNumber2 != nil {
			if !existingContact.CustomNumber2.IsNull {
				customNumber2SQL = sql.NullFloat64{Float64: existingContact.CustomNumber2.Float64, Valid: true}
			}
		}
		if existingContact.CustomNumber3 != nil {
			if !existingContact.CustomNumber3.IsNull {
				customNumber3SQL = sql.NullFloat64{Float64: existingContact.CustomNumber3.Float64, Valid: true}
			}
		}
		if existingContact.CustomNumber4 != nil {
			if !existingContact.CustomNumber4.IsNull {
				customNumber4SQL = sql.NullFloat64{Float64: existingContact.CustomNumber4.Float64, Valid: true}
			}
		}
		if existingContact.CustomNumber5 != nil {
			if !existingContact.CustomNumber5.IsNull {
				customNumber5SQL = sql.NullFloat64{Float64: existingContact.CustomNumber5.Float64, Valid: true}
			}
		}

		// Convert custom datetime fields
		if existingContact.CustomDatetime1 != nil {
			if !existingContact.CustomDatetime1.IsNull {
				customDatetime1SQL = sql.NullTime{Time: existingContact.CustomDatetime1.Time, Valid: true}
			}
		}
		if existingContact.CustomDatetime2 != nil {
			if !existingContact.CustomDatetime2.IsNull {
				customDatetime2SQL = sql.NullTime{Time: existingContact.CustomDatetime2.Time, Valid: true}
			}
		}
		if existingContact.CustomDatetime3 != nil {
			if !existingContact.CustomDatetime3.IsNull {
				customDatetime3SQL = sql.NullTime{Time: existingContact.CustomDatetime3.Time, Valid: true}
			}
		}
		if existingContact.CustomDatetime4 != nil {
			if !existingContact.CustomDatetime4.IsNull {
				customDatetime4SQL = sql.NullTime{Time: existingContact.CustomDatetime4.Time, Valid: true}
			}
		}
		if existingContact.CustomDatetime5 != nil {
			if !existingContact.CustomDatetime5.IsNull {
				customDatetime5SQL = sql.NullTime{Time: existingContact.CustomDatetime5.Time, Valid: true}
			}
		}

		// Convert JSON fields
		if existingContact.CustomJSON1 != nil {
			if !existingContact.CustomJSON1.IsNull {
				jsonBytes, err := json.Marshal(existingContact.CustomJSON1.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_1: %w", err)
				}
				customJSON1SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			}
		}
		if existingContact.CustomJSON2 != nil {
			if !existingContact.CustomJSON2.IsNull {
				jsonBytes, err := json.Marshal(existingContact.CustomJSON2.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_2: %w", err)
				}
				customJSON2SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			}
		}
		if existingContact.CustomJSON3 != nil {
			if !existingContact.CustomJSON3.IsNull {
				jsonBytes, err := json.Marshal(existingContact.CustomJSON3.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_3: %w", err)
				}
				customJSON3SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			}
		}
		if existingContact.CustomJSON4 != nil {
			if !existingContact.CustomJSON4.IsNull {
				jsonBytes, err := json.Marshal(existingContact.CustomJSON4.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_4: %w", err)
				}
				customJSON4SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			}
		}
		if existingContact.CustomJSON5 != nil {
			if !existingContact.CustomJSON5.IsNull {
				jsonBytes, err := json.Marshal(existingContact.CustomJSON5.Data)
				if err != nil {
					return false, fmt.Errorf("failed to marshal custom_json_5: %w", err)
				}
				customJSON5SQL = sql.NullString{String: string(jsonBytes), Valid: true}
			}
		}

		// Build update query using squirrel
		updateMap := sq.Eq{
			"external_id":       externalIDSQL,
			"timezone":          timezoneSQL,
			"language":          languageSQL,
			"first_name":        firstNameSQL,
			"last_name":         lastNameSQL,
			"full_name":         fullNameSQL,
			"phone":             phoneSQL,
			"address_line_1":    addressLine1SQL,
			"address_line_2":    addressLine2SQL,
			"country":           countrySQL,
			"postcode":          postcodeSQL,
			"state":             stateSQL,
			"job_title":         jobTitleSQL,
			"custom_string_1":   customString1SQL,
			"custom_string_2":   customString2SQL,
			"custom_string_3":   customString3SQL,
			"custom_string_4":   customString4SQL,
			"custom_string_5":   customString5SQL,
			"custom_number_1":   customNumber1SQL,
			"custom_number_2":   customNumber2SQL,
			"custom_number_3":   customNumber3SQL,
			"custom_number_4":   customNumber4SQL,
			"custom_number_5":   customNumber5SQL,
			"custom_datetime_1": customDatetime1SQL,
			"custom_datetime_2": customDatetime2SQL,
			"custom_datetime_3": customDatetime3SQL,
			"custom_datetime_4": customDatetime4SQL,
			"custom_datetime_5": customDatetime5SQL,
			"custom_json_1":     customJSON1SQL,
			"custom_json_2":     customJSON2SQL,
			"custom_json_3":     customJSON3SQL,
			"custom_json_4":     customJSON4SQL,
			"custom_json_5":     customJSON5SQL,
			"db_updated_at":     existingContact.DBUpdatedAt,
		}

		// Always update updated_at to current time for updates
		// If the incoming contact provided an updated_at, use it; otherwise use NOW()
		if !contact.UpdatedAt.IsZero() {
			updateMap["updated_at"] = contact.UpdatedAt.UTC()
		} else {
			updateMap["updated_at"] = time.Now().UTC()
		}

		updateBuilder := psql.Update("contacts").
			SetMap(updateMap).
			Where(sq.Eq{"email": existingContact.Email})

		updateQuery, updateArgs, err := updateBuilder.ToSql()
		if err != nil {
			return false, fmt.Errorf("failed to build update query: %w", err)
		}

		// Execute the update query
		_, err = tx.ExecContext(ctx, updateQuery, updateArgs...)
		if err != nil {
			return false, fmt.Errorf("failed to update contact: %w", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return isNew, nil
}

func contactToNullString(n *domain.NullableString) sql.NullString {
	if n != nil && !n.IsNull {
		return sql.NullString{String: n.String, Valid: true}
	}
	return sql.NullString{Valid: false}
}

func contactToNullFloat64(n *domain.NullableFloat64) sql.NullFloat64 {
	if n != nil && !n.IsNull {
		return sql.NullFloat64{Float64: n.Float64, Valid: true}
	}
	return sql.NullFloat64{Valid: false}
}

func contactToNullTime(n *domain.NullableTime) sql.NullTime {
	if n != nil && !n.IsNull {
		return sql.NullTime{Time: n.Time, Valid: true}
	}
	return sql.NullTime{Valid: false}
}

func contactToNullJSON(n *domain.NullableJSON) sql.NullString {
	if n != nil && !n.IsNull {
		jsonBytes, err := json.Marshal(n.Data)
		if err != nil {
			return sql.NullString{Valid: false}
		}
		return sql.NullString{String: string(jsonBytes), Valid: true}
	}
	return sql.NullString{Valid: false}
}

// BulkUpsertContacts creates or updates multiple contacts in a single database operation
// It uses PostgreSQL's INSERT ... ON CONFLICT to efficiently handle both inserts and updates
// Returns per-contact results indicating whether each was inserted (IsNew=true) or updated (IsNew=false)
func (r *contactRepository) BulkUpsertContacts(ctx context.Context, workspaceID string, contacts []*domain.Contact) ([]domain.BulkUpsertResult, error) {
	if len(contacts) == 0 {
		return []domain.BulkUpsertResult{}, nil
	}

	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Start a transaction
	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Rollback if there's a panic or error

	now := time.Now().UTC()

	// Build the multi-row INSERT statement
	// We'll use a raw SQL query because squirrel doesn't handle complex ON CONFLICT well
	var queryBuilder strings.Builder
	args := make([]interface{}, 0, len(contacts)*36) // 36 fields per contact (db_created_at and db_updated_at are managed by DB)
	argIndex := 1

	queryBuilder.WriteString(`INSERT INTO contacts (
		email, external_id, timezone, language,
		first_name, last_name, full_name, phone, address_line_1, address_line_2,
		country, postcode, state, job_title,
		custom_string_1, custom_string_2, custom_string_3, custom_string_4, custom_string_5,
		custom_number_1, custom_number_2, custom_number_3, custom_number_4, custom_number_5,
		custom_datetime_1, custom_datetime_2, custom_datetime_3, custom_datetime_4, custom_datetime_5,
		custom_json_1, custom_json_2, custom_json_3, custom_json_4, custom_json_5,
		created_at, updated_at
	) VALUES `)

	// Add value placeholders for each contact
	for i, contact := range contacts {
		if i > 0 {
			queryBuilder.WriteString(", ")
		}
		queryBuilder.WriteString("(")

		// Add 36 placeholders for contact fields (excluding db_created_at and db_updated_at)
		for j := 0; j < 36; j++ {
			if j > 0 {
				queryBuilder.WriteString(", ")
			}
			queryBuilder.WriteByte('$')
			queryBuilder.WriteString(strconv.Itoa(argIndex))
			argIndex++
		}
		queryBuilder.WriteString(")")

		// Determine timestamps - use provided or default to now
		createdAt := now
		if !contact.CreatedAt.IsZero() {
			createdAt = contact.CreatedAt.UTC()
		}
		updatedAt := now
		if !contact.UpdatedAt.IsZero() {
			updatedAt = contact.UpdatedAt.UTC()
		}

		// Add all field values in the correct order
		// Note: db_created_at and db_updated_at are NOT included - they have DEFAULT CURRENT_TIMESTAMP in the schema
		args = append(args,
			contact.Email,                        // 1
			contactToNullString(contact.ExternalID),     // 2
			contactToNullString(contact.Timezone),       // 3
			contactToNullString(contact.Language),       // 4
			contactToNullString(contact.FirstName),      // 5
			contactToNullString(contact.LastName),       // 6
			contactToNullString(contact.FullName),       // 7
			contactToNullString(contact.Phone),          // 8
			contactToNullString(contact.AddressLine1),   // 9
			contactToNullString(contact.AddressLine2),   // 10
			contactToNullString(contact.Country),        // 11
			contactToNullString(contact.Postcode),       // 12
			contactToNullString(contact.State),          // 13
			contactToNullString(contact.JobTitle),       // 14
			contactToNullString(contact.CustomString1),  // 15
			contactToNullString(contact.CustomString2),  // 16
			contactToNullString(contact.CustomString3),  // 17
			contactToNullString(contact.CustomString4),  // 18
			contactToNullString(contact.CustomString5),  // 19
			contactToNullFloat64(contact.CustomNumber1), // 20
			contactToNullFloat64(contact.CustomNumber2), // 21
			contactToNullFloat64(contact.CustomNumber3), // 22
			contactToNullFloat64(contact.CustomNumber4), // 23
			contactToNullFloat64(contact.CustomNumber5), // 24
			contactToNullTime(contact.CustomDatetime1),  // 25
			contactToNullTime(contact.CustomDatetime2),  // 26
			contactToNullTime(contact.CustomDatetime3),  // 27
			contactToNullTime(contact.CustomDatetime4),  // 28
			contactToNullTime(contact.CustomDatetime5),  // 29
			contactToNullJSON(contact.CustomJSON1),      // 30
			contactToNullJSON(contact.CustomJSON2),      // 31
			contactToNullJSON(contact.CustomJSON3),      // 32
			contactToNullJSON(contact.CustomJSON4),      // 33
			contactToNullJSON(contact.CustomJSON5),      // 34
			createdAt,                            // 35 - application-level timestamp
			updatedAt,                            // 36 - application-level timestamp
		)
	}

	// Add ON CONFLICT clause with merge semantics
	// For updates, we only update fields that were provided (non-null in the import)
	// This preserves the merge behavior from the single upsert
	queryBuilder.WriteString(`
	ON CONFLICT (email) DO UPDATE SET
		external_id = CASE WHEN EXCLUDED.external_id IS NOT NULL THEN EXCLUDED.external_id ELSE contacts.external_id END,
		timezone = CASE WHEN EXCLUDED.timezone IS NOT NULL THEN EXCLUDED.timezone ELSE contacts.timezone END,
		language = CASE WHEN EXCLUDED.language IS NOT NULL THEN EXCLUDED.language ELSE contacts.language END,
		first_name = CASE WHEN EXCLUDED.first_name IS NOT NULL THEN EXCLUDED.first_name ELSE contacts.first_name END,
		last_name = CASE WHEN EXCLUDED.last_name IS NOT NULL THEN EXCLUDED.last_name ELSE contacts.last_name END,
		full_name = CASE WHEN EXCLUDED.full_name IS NOT NULL THEN EXCLUDED.full_name ELSE contacts.full_name END,
		phone = CASE WHEN EXCLUDED.phone IS NOT NULL THEN EXCLUDED.phone ELSE contacts.phone END,
		address_line_1 = CASE WHEN EXCLUDED.address_line_1 IS NOT NULL THEN EXCLUDED.address_line_1 ELSE contacts.address_line_1 END,
		address_line_2 = CASE WHEN EXCLUDED.address_line_2 IS NOT NULL THEN EXCLUDED.address_line_2 ELSE contacts.address_line_2 END,
		country = CASE WHEN EXCLUDED.country IS NOT NULL THEN EXCLUDED.country ELSE contacts.country END,
		postcode = CASE WHEN EXCLUDED.postcode IS NOT NULL THEN EXCLUDED.postcode ELSE contacts.postcode END,
		state = CASE WHEN EXCLUDED.state IS NOT NULL THEN EXCLUDED.state ELSE contacts.state END,
		job_title = CASE WHEN EXCLUDED.job_title IS NOT NULL THEN EXCLUDED.job_title ELSE contacts.job_title END,
		custom_string_1 = CASE WHEN EXCLUDED.custom_string_1 IS NOT NULL THEN EXCLUDED.custom_string_1 ELSE contacts.custom_string_1 END,
		custom_string_2 = CASE WHEN EXCLUDED.custom_string_2 IS NOT NULL THEN EXCLUDED.custom_string_2 ELSE contacts.custom_string_2 END,
		custom_string_3 = CASE WHEN EXCLUDED.custom_string_3 IS NOT NULL THEN EXCLUDED.custom_string_3 ELSE contacts.custom_string_3 END,
		custom_string_4 = CASE WHEN EXCLUDED.custom_string_4 IS NOT NULL THEN EXCLUDED.custom_string_4 ELSE contacts.custom_string_4 END,
		custom_string_5 = CASE WHEN EXCLUDED.custom_string_5 IS NOT NULL THEN EXCLUDED.custom_string_5 ELSE contacts.custom_string_5 END,
		custom_number_1 = CASE WHEN EXCLUDED.custom_number_1 IS NOT NULL THEN EXCLUDED.custom_number_1 ELSE contacts.custom_number_1 END,
		custom_number_2 = CASE WHEN EXCLUDED.custom_number_2 IS NOT NULL THEN EXCLUDED.custom_number_2 ELSE contacts.custom_number_2 END,
		custom_number_3 = CASE WHEN EXCLUDED.custom_number_3 IS NOT NULL THEN EXCLUDED.custom_number_3 ELSE contacts.custom_number_3 END,
		custom_number_4 = CASE WHEN EXCLUDED.custom_number_4 IS NOT NULL THEN EXCLUDED.custom_number_4 ELSE contacts.custom_number_4 END,
		custom_number_5 = CASE WHEN EXCLUDED.custom_number_5 IS NOT NULL THEN EXCLUDED.custom_number_5 ELSE contacts.custom_number_5 END,
		custom_datetime_1 = CASE WHEN EXCLUDED.custom_datetime_1 IS NOT NULL THEN EXCLUDED.custom_datetime_1 ELSE contacts.custom_datetime_1 END,
		custom_datetime_2 = CASE WHEN EXCLUDED.custom_datetime_2 IS NOT NULL THEN EXCLUDED.custom_datetime_2 ELSE contacts.custom_datetime_2 END,
		custom_datetime_3 = CASE WHEN EXCLUDED.custom_datetime_3 IS NOT NULL THEN EXCLUDED.custom_datetime_3 ELSE contacts.custom_datetime_3 END,
		custom_datetime_4 = CASE WHEN EXCLUDED.custom_datetime_4 IS NOT NULL THEN EXCLUDED.custom_datetime_4 ELSE contacts.custom_datetime_4 END,
		custom_datetime_5 = CASE WHEN EXCLUDED.custom_datetime_5 IS NOT NULL THEN EXCLUDED.custom_datetime_5 ELSE contacts.custom_datetime_5 END,
		custom_json_1 = CASE WHEN EXCLUDED.custom_json_1 IS NOT NULL THEN EXCLUDED.custom_json_1 ELSE contacts.custom_json_1 END,
		custom_json_2 = CASE WHEN EXCLUDED.custom_json_2 IS NOT NULL THEN EXCLUDED.custom_json_2 ELSE contacts.custom_json_2 END,
		custom_json_3 = CASE WHEN EXCLUDED.custom_json_3 IS NOT NULL THEN EXCLUDED.custom_json_3 ELSE contacts.custom_json_3 END,
		custom_json_4 = CASE WHEN EXCLUDED.custom_json_4 IS NOT NULL THEN EXCLUDED.custom_json_4 ELSE contacts.custom_json_4 END,
		custom_json_5 = CASE WHEN EXCLUDED.custom_json_5 IS NOT NULL THEN EXCLUDED.custom_json_5 ELSE contacts.custom_json_5 END,
		created_at = EXCLUDED.created_at,
		updated_at = EXCLUDED.updated_at,
		db_updated_at = NOW()
	RETURNING email, (xmax = 0) AS is_new`)

	query := queryBuilder.String()

	// Execute the bulk upsert
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute bulk upsert: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Collect results
	results := make([]domain.BulkUpsertResult, 0, len(contacts))
	for rows.Next() {
		var result domain.BulkUpsertResult
		if err := rows.Scan(&result.Email, &result.IsNew); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return results, nil
}

// GetContactsForBroadcast retrieves contacts based on broadcast audience settings
// It supports filtering by lists, handling unsubscribed contacts, and deduplication
// Uses cursor-based pagination with afterEmail for deterministic ordering (fixes Issue #157)
func (r *contactRepository) GetContactsForBroadcast(
	ctx context.Context,
	workspaceID string,
	audience domain.AudienceSettings,
	limit int,
	afterEmail string,
) ([]*domain.ContactWithList, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Start building the main query
	var query sq.SelectBuilder
	var includeListID bool

	// If we're filtering by list, include list_id in the result
	if audience.List != "" {
		includeListID = true
		// Build column list: all contact columns plus list_id and list_name
		selectCols := append(contactColumnsWithPrefix("c"), "cl.list_id", "l.name as list_name")
		query = psql.Select(selectCols...).
			From("contacts c").
			Join("contact_lists cl ON c.email = cl.email").
			Join("lists l ON cl.list_id = l.id"). // Join with lists table to get the name
			Where(sq.Eq{"cl.list_id": audience.List}).
			Where(sq.Eq{"l.deleted_at": nil}). // Filter out deleted lists
			Limit(uint64(limit)).
			OrderBy("c.email ASC") // Sort by email only (unique, deterministic)

		// Cursor-based pagination: fetch contacts with email > afterEmail
		if afterEmail != "" {
			query = query.Where(sq.Gt{"c.email": afterEmail})
		}

		// Exclude unsubscribed contacts if required
		if audience.ExcludeUnsubscribed {
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusUnsubscribed})
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusBounced})
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusComplained})
		}
	} else {
		// For non-list based audiences (e.g., segments in the future)
		includeListID = false
		query = psql.Select(contactColumnsWithPrefix("c")...).
			From("contacts c").
			Limit(uint64(limit)).
			OrderBy("c.email ASC") // Sort by email only (unique, deterministic)

		// Cursor-based pagination: fetch contacts with email > afterEmail
		if afterEmail != "" {
			query = query.Where(sq.Gt{"c.email": afterEmail})
		}
	}

	// Handle segments filtering
	if len(audience.Segments) > 0 {
		// If we already have list filtering, we need to add segments as an additional filter
		// This means contacts must be in BOTH the specified list AND segments
		if audience.List != "" {
			// Join with contact_segments table in addition to the existing list joins
			query = query.Join("contact_segments cs ON c.email = cs.email")
			query = query.Where(sq.Eq{"cs.segment_id": audience.Segments})
		} else {
			// No list filtering, so we're filtering by segments only
			// We need to select from contacts and join with contact_segments
			includeListID = false
			query = psql.Select(contactColumnsWithPrefix("c")...).
				From("contacts c").
				Join("contact_segments cs ON c.email = cs.email").
				Where(sq.Eq{"cs.segment_id": audience.Segments}).
				Limit(uint64(limit)).
				OrderBy("c.email ASC") // Sort by email only (unique, deterministic)

			// Cursor-based pagination: fetch contacts with email > afterEmail
			if afterEmail != "" {
				query = query.Where(sq.Gt{"c.email": afterEmail})
			}
		}
	}

	// Build the final query
	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Execute the query
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Process the results
	var contactsWithList []*domain.ContactWithList

	for rows.Next() {
		var listID sql.NullString
		var listName sql.NullString
		var contact *domain.Contact
		var scanErr error

		if includeListID {
			// We need to scan all columns at once since we selected c.*, cl.list_id, l.name
			// Create all the scan destinations for contact fields plus list_id and list_name
			var email, externalID, timezone, language sql.NullString
			var firstName, lastName, fullName, phone, addressLine1, addressLine2 sql.NullString
			var country, postcode, state, jobTitle sql.NullString
			var customString1, customString2, customString3, customString4, customString5 sql.NullString
			var customNumber1, customNumber2, customNumber3, customNumber4, customNumber5 sql.NullFloat64
			var customDatetime1, customDatetime2, customDatetime3, customDatetime4, customDatetime5 sql.NullTime
			var customJSON1, customJSON2, customJSON3, customJSON4, customJSON5 sql.NullString
			var createdAt, updatedAt, dbCreatedAt, dbUpdatedAt time.Time

			// Scan all columns including contact fields + list_id + list_name
			scanErr = rows.Scan(
				&email, &externalID, &timezone, &language,
				&firstName, &lastName, &fullName, &phone, &addressLine1, &addressLine2,
				&country, &postcode, &state, &jobTitle,
				&customString1, &customString2, &customString3, &customString4, &customString5,
				&customNumber1, &customNumber2, &customNumber3, &customNumber4, &customNumber5,
				&customDatetime1, &customDatetime2, &customDatetime3, &customDatetime4, &customDatetime5,
				&customJSON1, &customJSON2, &customJSON3, &customJSON4, &customJSON5,
				&createdAt, &updatedAt, &dbCreatedAt, &dbUpdatedAt,
				&listID, &listName, // Additional columns
			)
			if scanErr != nil {
				return nil, fmt.Errorf("failed to scan contact with list: %w", scanErr)
			}

			// Convert scanned values to domain.Contact
			contact = &domain.Contact{
				Email:       email.String,
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
				DBCreatedAt: dbCreatedAt,
				DBUpdatedAt: dbUpdatedAt,
			}

			// Set nullable fields
			if externalID.Valid {
				contact.ExternalID = &domain.NullableString{String: externalID.String, IsNull: false}
			}
			if timezone.Valid {
				contact.Timezone = &domain.NullableString{String: timezone.String, IsNull: false}
			}
			if language.Valid {
				contact.Language = &domain.NullableString{String: language.String, IsNull: false}
			}
			if firstName.Valid {
				contact.FirstName = &domain.NullableString{String: firstName.String, IsNull: false}
			}
			if lastName.Valid {
				contact.LastName = &domain.NullableString{String: lastName.String, IsNull: false}
			}
			if fullName.Valid {
				contact.FullName = &domain.NullableString{String: fullName.String, IsNull: false}
			}
			if phone.Valid {
				contact.Phone = &domain.NullableString{String: phone.String, IsNull: false}
			}
			if addressLine1.Valid {
				contact.AddressLine1 = &domain.NullableString{String: addressLine1.String, IsNull: false}
			}
			if addressLine2.Valid {
				contact.AddressLine2 = &domain.NullableString{String: addressLine2.String, IsNull: false}
			}
			if country.Valid {
				contact.Country = &domain.NullableString{String: country.String, IsNull: false}
			}
			if postcode.Valid {
				contact.Postcode = &domain.NullableString{String: postcode.String, IsNull: false}
			}
			if state.Valid {
				contact.State = &domain.NullableString{String: state.String, IsNull: false}
			}
			if jobTitle.Valid {
				contact.JobTitle = &domain.NullableString{String: jobTitle.String, IsNull: false}
			}
			// Handle custom fields similarly...
			if customString1.Valid {
				contact.CustomString1 = &domain.NullableString{String: customString1.String, IsNull: false}
			}
			if customString2.Valid {
				contact.CustomString2 = &domain.NullableString{String: customString2.String, IsNull: false}
			}
			if customString3.Valid {
				contact.CustomString3 = &domain.NullableString{String: customString3.String, IsNull: false}
			}
			if customString4.Valid {
				contact.CustomString4 = &domain.NullableString{String: customString4.String, IsNull: false}
			}
			if customString5.Valid {
				contact.CustomString5 = &domain.NullableString{String: customString5.String, IsNull: false}
			}
			if customNumber1.Valid {
				contact.CustomNumber1 = &domain.NullableFloat64{Float64: customNumber1.Float64, IsNull: false}
			}
			if customNumber2.Valid {
				contact.CustomNumber2 = &domain.NullableFloat64{Float64: customNumber2.Float64, IsNull: false}
			}
			if customNumber3.Valid {
				contact.CustomNumber3 = &domain.NullableFloat64{Float64: customNumber3.Float64, IsNull: false}
			}
			if customNumber4.Valid {
				contact.CustomNumber4 = &domain.NullableFloat64{Float64: customNumber4.Float64, IsNull: false}
			}
			if customNumber5.Valid {
				contact.CustomNumber5 = &domain.NullableFloat64{Float64: customNumber5.Float64, IsNull: false}
			}
			if customDatetime1.Valid {
				contact.CustomDatetime1 = &domain.NullableTime{Time: customDatetime1.Time, IsNull: false}
			}
			if customDatetime2.Valid {
				contact.CustomDatetime2 = &domain.NullableTime{Time: customDatetime2.Time, IsNull: false}
			}
			if customDatetime3.Valid {
				contact.CustomDatetime3 = &domain.NullableTime{Time: customDatetime3.Time, IsNull: false}
			}
			if customDatetime4.Valid {
				contact.CustomDatetime4 = &domain.NullableTime{Time: customDatetime4.Time, IsNull: false}
			}
			if customDatetime5.Valid {
				contact.CustomDatetime5 = &domain.NullableTime{Time: customDatetime5.Time, IsNull: false}
			}
			if customJSON1.Valid {
				var jsonData interface{}
				if err := json.Unmarshal([]byte(customJSON1.String), &jsonData); err == nil {
					contact.CustomJSON1 = &domain.NullableJSON{Data: jsonData, IsNull: false}
				}
			}
			if customJSON2.Valid {
				var jsonData interface{}
				if err := json.Unmarshal([]byte(customJSON2.String), &jsonData); err == nil {
					contact.CustomJSON2 = &domain.NullableJSON{Data: jsonData, IsNull: false}
				}
			}
			if customJSON3.Valid {
				var jsonData interface{}
				if err := json.Unmarshal([]byte(customJSON3.String), &jsonData); err == nil {
					contact.CustomJSON3 = &domain.NullableJSON{Data: jsonData, IsNull: false}
				}
			}
			if customJSON4.Valid {
				var jsonData interface{}
				if err := json.Unmarshal([]byte(customJSON4.String), &jsonData); err == nil {
					contact.CustomJSON4 = &domain.NullableJSON{Data: jsonData, IsNull: false}
				}
			}
			if customJSON5.Valid {
				var jsonData interface{}
				if err := json.Unmarshal([]byte(customJSON5.String), &jsonData); err == nil {
					contact.CustomJSON5 = &domain.NullableJSON{Data: jsonData, IsNull: false}
				}
			}
		} else {
			// No list ID to scan, just get the contact using the existing ScanContact function
			contact, scanErr = domain.ScanContact(rows)
			if scanErr != nil {
				return nil, fmt.Errorf("failed to scan contact: %w", scanErr)
			}
		}

		// Create ContactWithList object
		contactWithList := &domain.ContactWithList{
			Contact:  contact,
			ListID:   listID.String,   // Will be empty string if NULL or if not in a list-filtered query
			ListName: listName.String, // Will be empty string if NULL or if not in a list-filtered query
		}
		contactsWithList = append(contactsWithList, contactWithList)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over contact rows: %w", err)
	}

	return contactsWithList, nil
}

// CountContactsForBroadcast counts how many contacts match broadcast audience settings
// without retrieving all contact records
func (r *contactRepository) CountContactsForBroadcast(
	ctx context.Context,
	workspaceID string,
	audience domain.AudienceSettings,
) (int, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Start building the count query
	query := psql.Select("COUNT(*)").
		From("contacts c")

	// Handle list filtering
	if audience.List != "" {
		// Join with contact_lists table to filter by list membership and status
		query = query.Join("contact_lists cl ON c.email = cl.email")
		// Join with lists table to filter by list deletion status (matches GetContactsForBroadcast)
		query = query.Join("lists l ON cl.list_id = l.id")

		// Filter by the specified list
		query = query.Where(sq.Eq{"cl.list_id": audience.List})
		// Filter out soft-deleted lists (matches GetContactsForBroadcast)
		query = query.Where(sq.Eq{"l.deleted_at": nil})

		// Exclude unsubscribed contacts if required
		if audience.ExcludeUnsubscribed {
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusUnsubscribed})
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusBounced})
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusComplained})
		}
	}

	// Handle segments filtering
	if len(audience.Segments) > 0 {
		// If we already have list filtering, we need to add segments as an additional filter
		// This means contacts must be in BOTH the specified list AND segments
		if audience.List != "" {
			// Join with contact_segments table in addition to the existing list joins
			query = query.Join("contact_segments cs ON c.email = cs.email")
			query = query.Where(sq.Eq{"cs.segment_id": audience.Segments})
		} else {
			// No list filtering, so we're filtering by segments only
			query = psql.Select("COUNT(*)").
				From("contacts c").
				Join("contact_segments cs ON c.email = cs.email").
				Where(sq.Eq{"cs.segment_id": audience.Segments})
		}
	}

	// Build and execute the query
	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int
	err = db.QueryRowContext(ctx, sqlQuery, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	return count, nil
}

// Count returns the total number of contacts in a workspace
func (r *contactRepository) Count(ctx context.Context, workspaceID string) (int, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query, args, err := psql.Select("COUNT(*)").
		From("contacts").
		ToSql()

	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int
	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	return count, nil
}

// GetBatchForSegment retrieves a batch of email addresses for segment processing
// Optimized to only fetch emails instead of full contact objects
func (r *contactRepository) GetBatchForSegment(ctx context.Context, workspaceID string, offset int64, limit int) ([]string, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT email 
		FROM contacts 
		ORDER BY email ASC 
		LIMIT $1 OFFSET $2
	`

	rows, err := db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query emails: %w", err)
	}
	defer func() { _ = rows.Close() }()

	emails := make([]string, 0, limit)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return emails, nil
}
