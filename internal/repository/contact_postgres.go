package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"encoding/base64"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
)

type contactRepository struct {
	workspaceRepo domain.WorkspaceRepository
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

func (r *contactRepository) GetContactByExternalID(ctx context.Context, externalID, workspaceID string) (*domain.Contact, error) {
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
	query, args, err := psql.Select("c.*").
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
	defer rows.Close()

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

	return contact, nil
}

func (r *contactRepository) GetContacts(ctx context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	sb := psql.Select("c.*").From("contacts c")

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
		// Start building the subquery
		subquery := psql.Select("1").
			From("contact_lists cl").
			Where("cl.email = c.email").
			Where(sq.Eq{"cl.deleted_at": nil})

		// Add specific conditions to the subquery
		if req.ListID != "" && req.ContactListStatus != "" {
			// Both list_id and status
			subquery = subquery.Where(sq.Eq{"cl.list_id": req.ListID, "cl.status": req.ContactListStatus})
		} else if req.ListID != "" {
			// Just list_id
			subquery = subquery.Where(sq.Eq{"cl.list_id": req.ListID})
		} else if req.ContactListStatus != "" {
			// Just status
			subquery = subquery.Where(sq.Eq{"cl.status": req.ContactListStatus})
		}

		// Convert subquery to SQL
		subquerySql, subqueryArgs, err := subquery.ToSql()
		if err != nil {
			return nil, fmt.Errorf("failed to build subquery: %w", err)
		}

		// Add the EXISTS condition to the main query
		sb = sb.Where(fmt.Sprintf("EXISTS (%s)", subquerySql), subqueryArgs...)
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

		cursorTime, err := time.Parse(time.RFC3339, cursorParts[0])
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
	defer rows.Close()

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
		cursorStr := fmt.Sprintf("%s~%s", lastContact.CreatedAt.Format(time.RFC3339), lastContact.Email)

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
		defer listRows.Close()

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

	return &domain.GetContactsResponse{
		Contacts:   contacts,
		NextCursor: nextCursor,
	}, nil
}

func (r *contactRepository) DeleteContact(ctx context.Context, email string, workspaceID string) error {
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
	defer tx.Rollback() // Rollback if there's a panic or error

	// Check if contact exists with FOR UPDATE lock using squirrel
	selectQuery, selectArgs, err := psql.Select("c.*").
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
		now := time.Now()
		contact.DBCreatedAt = now
		contact.DBUpdatedAt = now

		// Convert domain nullable types to SQL nullable types
		var externalIDSQL, timezoneSQL, languageSQL sql.NullString
		var firstNameSQL, lastNameSQL, phoneSQL, addressLine1SQL, addressLine2SQL sql.NullString
		var countrySQL, postcodeSQL, stateSQL, jobTitleSQL sql.NullString
		var lifetimeValueSQL, ordersCountSQL sql.NullFloat64
		var lastOrderAtSQL sql.NullTime
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

		// Number fields
		if contact.LifetimeValue != nil {
			if !contact.LifetimeValue.IsNull {
				lifetimeValueSQL = sql.NullFloat64{Float64: contact.LifetimeValue.Float64, Valid: true}
			} else {
				lifetimeValueSQL = sql.NullFloat64{Valid: false}
			}
		}
		if contact.OrdersCount != nil {
			if !contact.OrdersCount.IsNull {
				ordersCountSQL = sql.NullFloat64{Float64: contact.OrdersCount.Float64, Valid: true}
			} else {
				ordersCountSQL = sql.NullFloat64{Valid: false}
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

		// Datetime fields
		if contact.LastOrderAt != nil {
			if !contact.LastOrderAt.IsNull {
				lastOrderAtSQL = sql.NullTime{Time: contact.LastOrderAt.Time, Valid: true}
			} else {
				lastOrderAtSQL = sql.NullTime{Valid: false}
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
		insertBuilder := psql.Insert("contacts").
			Columns(
				"email", "external_id", "timezone", "language",
				"first_name", "last_name", "phone", "address_line_1", "address_line_2",
				"country", "postcode", "state", "job_title",
				"lifetime_value", "orders_count", "last_order_at",
				"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
				"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
				"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
				"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
				"created_at", "updated_at",
			).
			Values(
				contact.Email, externalIDSQL, timezoneSQL, languageSQL,
				firstNameSQL, lastNameSQL, phoneSQL, addressLine1SQL, addressLine2SQL,
				countrySQL, postcodeSQL, stateSQL, jobTitleSQL,
				lifetimeValueSQL, ordersCountSQL, lastOrderAtSQL,
				customString1SQL, customString2SQL, customString3SQL, customString4SQL, customString5SQL,
				customNumber1SQL, customNumber2SQL, customNumber3SQL, customNumber4SQL, customNumber5SQL,
				customDatetime1SQL, customDatetime2SQL, customDatetime3SQL, customDatetime4SQL, customDatetime5SQL,
				customJSON1SQL, customJSON2SQL, customJSON3SQL, customJSON4SQL, customJSON5SQL,
				contact.DBCreatedAt, contact.DBUpdatedAt, // Use DB timestamps
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
		existingContact.DBUpdatedAt = time.Now()

		// Merge changes from the input 'contact' into the 'existingContact'
		existingContact.Merge(contact)

		// Convert domain nullable types to SQL nullable types for the update
		var externalIDSQL, timezoneSQL, languageSQL sql.NullString
		var firstNameSQL, lastNameSQL, phoneSQL, addressLine1SQL, addressLine2SQL sql.NullString
		var countrySQL, postcodeSQL, stateSQL, jobTitleSQL sql.NullString
		var lifetimeValueSQL, ordersCountSQL sql.NullFloat64
		var lastOrderAtSQL sql.NullTime
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

		// Convert number fields
		if existingContact.LifetimeValue != nil {
			if !existingContact.LifetimeValue.IsNull {
				lifetimeValueSQL = sql.NullFloat64{Float64: existingContact.LifetimeValue.Float64, Valid: true}
			}
		}
		if existingContact.OrdersCount != nil {
			if !existingContact.OrdersCount.IsNull {
				ordersCountSQL = sql.NullFloat64{Float64: existingContact.OrdersCount.Float64, Valid: true}
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

		// Convert datetime fields
		if existingContact.LastOrderAt != nil {
			if !existingContact.LastOrderAt.IsNull {
				lastOrderAtSQL = sql.NullTime{Time: existingContact.LastOrderAt.Time, Valid: true}
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
		updateBuilder := psql.Update("contacts").
			SetMap(sq.Eq{
				"external_id":       externalIDSQL,
				"timezone":          timezoneSQL,
				"language":          languageSQL,
				"first_name":        firstNameSQL,
				"last_name":         lastNameSQL,
				"phone":             phoneSQL,
				"address_line_1":    addressLine1SQL,
				"address_line_2":    addressLine2SQL,
				"country":           countrySQL,
				"postcode":          postcodeSQL,
				"state":             stateSQL,
				"job_title":         jobTitleSQL,
				"lifetime_value":    lifetimeValueSQL,
				"orders_count":      ordersCountSQL,
				"last_order_at":     lastOrderAtSQL,
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
				"updated_at":        existingContact.DBUpdatedAt, // Use DB timestamps
			}).
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

// GetContactsForBroadcast retrieves contacts based on broadcast audience settings
// It supports filtering by lists, handling unsubscribed contacts, and deduplication
func (r *contactRepository) GetContactsForBroadcast(
	ctx context.Context,
	workspaceID string,
	audience domain.AudienceSettings,
	limit int,
	offset int,
) ([]*domain.ContactWithList, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// Start building the main query
	var query sq.SelectBuilder
	var includeListID bool

	// If we're filtering by lists, include list_id in the result
	if len(audience.Lists) > 0 {
		includeListID = true
		query = psql.Select("c.*", "cl.list_id", "l.name as list_name").
			From("contacts c").
			Join("contact_lists cl ON c.email = cl.email").
			Join("lists l ON cl.list_id = l.id"). // Join with lists table to get the name
			Where(sq.Eq{"cl.list_id": audience.Lists}).
			Where(sq.Eq{"l.deleted_at": nil}). // Filter out deleted lists
			Limit(uint64(limit)).
			Offset(uint64(offset))

		// Set order by clause based on whether we need deduplication
		if audience.SkipDuplicateEmails {
			// For DISTINCT ON (c.email), we must order by c.email first
			query = query.OrderBy("c.email ASC", "c.created_at ASC")
		} else {
			query = query.OrderBy("c.created_at ASC")
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
		query = psql.Select("c.*").
			From("contacts c").
			Limit(uint64(limit)).
			Offset(uint64(offset))

		// Set order by clause based on whether we need deduplication
		if audience.SkipDuplicateEmails {
			// For DISTINCT ON (c.email), we must order by c.email first
			query = query.OrderBy("c.email ASC", "c.created_at ASC")
		} else {
			query = query.OrderBy("c.created_at ASC")
		}
	}

	// Handle segments filtering (if implemented)
	if len(audience.Segments) > 0 {
		// This would involve joining with segments tables or applying segment conditions
		// Implementation depends on how segments are structured in the database
		// For now, we'll just return an error
		return nil, fmt.Errorf("segments filtering not implemented")
	}

	// Build the final query
	sqlQuery, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Handle deduplication if required by modifying the SQL string
	if audience.SkipDuplicateEmails {
		// Replace "SELECT" with "SELECT DISTINCT ON (c.email)" at the beginning
		sqlQuery = strings.Replace(sqlQuery, "SELECT", "SELECT DISTINCT ON (c.email)", 1)
	}

	// Execute the query
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

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
			var firstName, lastName, phone, addressLine1, addressLine2 sql.NullString
			var country, postcode, state, jobTitle sql.NullString
			var lifetimeValue, ordersCount sql.NullFloat64
			var lastOrderAt sql.NullTime
			var customString1, customString2, customString3, customString4, customString5 sql.NullString
			var customNumber1, customNumber2, customNumber3, customNumber4, customNumber5 sql.NullFloat64
			var customDatetime1, customDatetime2, customDatetime3, customDatetime4, customDatetime5 sql.NullTime
			var customJSON1, customJSON2, customJSON3, customJSON4, customJSON5 sql.NullString
			var createdAt, updatedAt time.Time

			// Scan all columns including contact fields + list_id + list_name
			scanErr = rows.Scan(
				&email, &externalID, &timezone, &language,
				&firstName, &lastName, &phone, &addressLine1, &addressLine2,
				&country, &postcode, &state, &jobTitle,
				&lifetimeValue, &ordersCount, &lastOrderAt,
				&customString1, &customString2, &customString3, &customString4, &customString5,
				&customNumber1, &customNumber2, &customNumber3, &customNumber4, &customNumber5,
				&customDatetime1, &customDatetime2, &customDatetime3, &customDatetime4, &customDatetime5,
				&customJSON1, &customJSON2, &customJSON3, &customJSON4, &customJSON5,
				&createdAt, &updatedAt,
				&listID, &listName, // Additional columns
			)
			if scanErr != nil {
				return nil, fmt.Errorf("failed to scan contact with list: %w", scanErr)
			}

			// Convert scanned values to domain.Contact
			contact = &domain.Contact{
				Email:       email.String,
				DBCreatedAt: createdAt,
				DBUpdatedAt: updatedAt,
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
			if lifetimeValue.Valid {
				contact.LifetimeValue = &domain.NullableFloat64{Float64: lifetimeValue.Float64, IsNull: false}
			}
			if ordersCount.Valid {
				contact.OrdersCount = &domain.NullableFloat64{Float64: ordersCount.Float64, IsNull: false}
			}
			if lastOrderAt.Valid {
				contact.LastOrderAt = &domain.NullableTime{Time: lastOrderAt.Time, IsNull: false}
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
	// Use DISTINCT only if audience settings require deduplication, otherwise count all rows
	var countExpression string
	if audience.SkipDuplicateEmails {
		countExpression = "COUNT(DISTINCT c.email)"
	} else {
		countExpression = "COUNT(*)"
	}
	query := psql.Select(countExpression).
		From("contacts c")

	// Handle lists filtering
	if len(audience.Lists) > 0 {
		// Join with contact_lists table to filter by list membership and status
		query = query.Join("contact_lists cl ON c.email = cl.email")

		// Filter by the specified lists
		query = query.Where(sq.Eq{"cl.list_id": audience.Lists})

		// Exclude unsubscribed contacts if required
		if audience.ExcludeUnsubscribed {
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusUnsubscribed})
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusBounced})
			query = query.Where(sq.NotEq{"cl.status": domain.ContactListStatusComplained})
		}
	}

	// Handle segments filtering (if implemented)
	if len(audience.Segments) > 0 {
		// This would involve joining with segments tables or applying segment conditions
		// Implementation depends on how segments are structured in the database
		return 0, fmt.Errorf("segments filtering not implemented")
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
