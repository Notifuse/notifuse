package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

// setupMockDB creates a mock database and sqlmock for testing
func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err, "Failed to create mock database")

	cleanup := func() {
		db.Close()
	}

	return db, mock, cleanup
}

func TestGetContactByEmail(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(db, nil).AnyTimes()

	repo := NewContactRepository(workspaceRepo)
	now := time.Now().UTC().Truncate(time.Microsecond)
	email := "test@example.com"

	// Test case 1: Contact found
	rows := sqlmock.NewRows([]string{
		"email", "external_id", "timezone", "language",
		"first_name", "last_name", "phone", "address_line_1", "address_line_2",
		"country", "postcode", "state", "job_title",
		"lifetime_value", "orders_count", "last_order_at",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
		"created_at", "updated_at",
	}).
		AddRow(
			email, "ext123", "Europe/Paris", "en-US",
			"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
			"USA", "12345", "CA", "Developer",
			100.50, 5, now,
			"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
			42.0, 43.0, 44.0, 45.0, 46.0,
			now, now, now, now, now,
			[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
			now, now,
		)

	mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c.email = \$1`).
		WithArgs(email).
		WillReturnRows(rows)

	// Set up expectations for contact lists query
	listRows := sqlmock.NewRows([]string{
		"list_id", "status", "created_at", "updated_at", "deleted_at", "list_name",
	}).AddRow(
		"list1", "active", now, now, nil, "Marketing List",
	)

	mock.ExpectQuery(`SELECT cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, cl\.deleted_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email = \$1 AND l\.deleted_at IS NULL`).
		WithArgs(email).
		WillReturnRows(listRows)

	contact, err := repo.GetContactByEmail(context.Background(), "workspace123", email)
	require.NoError(t, err)
	assert.Equal(t, email, contact.Email)
	assert.Len(t, contact.ContactLists, 1)
	assert.Equal(t, "list1", contact.ContactLists[0].ListID)
	assert.Equal(t, "Marketing List", contact.ContactLists[0].ListName)

	// Test case 2: Contact not found
	mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c.email = \$1`).
		WithArgs("nonexistent@example.com").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetContactByEmail(context.Background(), "workspace123", "nonexistent@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contact not found")
}

func TestGetContactByExternalID(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(db, nil).AnyTimes()

	repo := NewContactRepository(workspaceRepo)
	now := time.Now().UTC().Truncate(time.Microsecond)
	externalID := "ext123"
	email := "test@example.com"

	// Test case 1: Contact found
	rows := sqlmock.NewRows([]string{
		"email", "external_id", "timezone", "language",
		"first_name", "last_name", "phone", "address_line_1", "address_line_2",
		"country", "postcode", "state", "job_title",
		"lifetime_value", "orders_count", "last_order_at",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
		"created_at", "updated_at",
	}).
		AddRow(
			email, externalID, "Europe/Paris", "en-US",
			"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
			"USA", "12345", "CA", "Developer",
			100.50, 5, now,
			"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
			42.0, 43.0, 44.0, 45.0, 46.0,
			now, now, now, now, now,
			[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
			now, now,
		)

	mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c.external_id = \$1`).
		WithArgs(externalID).
		WillReturnRows(rows)

	// Set up expectations for contact lists query
	listRows := sqlmock.NewRows([]string{
		"list_id", "status", "created_at", "updated_at", "deleted_at", "list_name",
	}).AddRow(
		"list1", "active", now, now, nil, "Marketing List",
	)

	mock.ExpectQuery(`SELECT cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, cl\.deleted_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email = \$1 AND l\.deleted_at IS NULL`).
		WithArgs(email).
		WillReturnRows(listRows)

	contact, err := repo.GetContactByExternalID(context.Background(), "workspace123", externalID)
	require.NoError(t, err)
	assert.Equal(t, email, contact.Email)
	assert.Equal(t, externalID, contact.ExternalID.String)
	assert.Len(t, contact.ContactLists, 1)
	assert.Equal(t, "list1", contact.ContactLists[0].ListID)
	assert.Equal(t, "Marketing List", contact.ContactLists[0].ListName)

	// Test case 2: Contact not found
	mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c.external_id = \$1`).
		WithArgs("nonexistent-ext-id").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetContactByExternalID(context.Background(), "workspace123", "nonexistent-ext-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contact not found")

	// Test: get contact by external ID successful case
	t.Run("successful_case", func(t *testing.T) {
		externalID := "e-123"
		email := "test@example.com"

		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "phone",
			"address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at",
		}).AddRow(
			email, "e-123", "Europe/Paris", "en-US", "John", "Doe", "", "", "", "", "", "", "", 0, 0, time.Time{},
			"", "", "", "", "", 0, 0, 0, 0, 0, time.Time{}, time.Time{}, time.Time{}, time.Time{}, time.Time{},
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			time.Now(), time.Now(),
		)

		mock.ExpectQuery("SELECT c\\.\\* FROM contacts c WHERE c.external_id = \\$1").
			WithArgs(externalID).
			WillReturnRows(rows)

		// Set up expectations for contact lists query (empty result)
		listRows := sqlmock.NewRows([]string{
			"list_id", "status", "created_at", "updated_at", "deleted_at", "list_name",
		})

		mock.ExpectQuery(`SELECT cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, cl\.deleted_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email = \$1`).
			WithArgs(email).
			WillReturnRows(listRows)

		// Act
		contact, err := repo.GetContactByExternalID(context.Background(), "workspace123", externalID)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, contact)
		assert.Equal(t, email, contact.Email)
		assert.Equal(t, "e-123", contact.ExternalID.String)
		assert.Empty(t, contact.ContactLists)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// Add a test for the new fetchContact method
func TestFetchContact(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(db, nil).AnyTimes()

	repo := NewContactRepository(workspaceRepo)
	now := time.Now().UTC().Truncate(time.Microsecond)
	email := "test@example.com"

	t.Run("with custom filter", func(t *testing.T) {
		// Test with a custom filter (phone number)
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at",
		}).
			AddRow(
				email, "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				100.50, 5, now,
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now,
			)

		phone := "+1234567890"
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c.phone = \$1`).
			WithArgs(phone).
			WillReturnRows(rows)

		// Set up expectations for contact lists query
		listRows := sqlmock.NewRows([]string{
			"list_id", "status", "created_at", "updated_at", "deleted_at", "list_name",
		}).AddRow(
			"list1", "active", now, now, nil, "Marketing List",
		).AddRow(
			"list2", "active", now, now, nil, "Newsletter",
		)

		mock.ExpectQuery(`SELECT cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, cl\.deleted_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email = \$1`).
			WithArgs(email).
			WillReturnRows(listRows)

		// Use the private method directly for testing
		contact, err := repo.(*contactRepository).fetchContact(context.Background(), "workspace123", sq.Eq{"c.phone": phone})
		require.NoError(t, err)
		assert.Equal(t, email, contact.Email)
		assert.Equal(t, phone, contact.Phone.String)
		assert.Len(t, contact.ContactLists, 2)
		assert.Equal(t, "list1", contact.ContactLists[0].ListID)
		assert.Equal(t, "Marketing List", contact.ContactLists[0].ListName)
		assert.Equal(t, "list2", contact.ContactLists[1].ListID)
		assert.Equal(t, "Newsletter", contact.ContactLists[1].ListName)
	})

	t.Run("with error on contact lists query", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at",
		}).
			AddRow(
				email, "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				100.50, 5, now,
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now,
			)

		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c.email = \$1`).
			WithArgs(email).
			WillReturnRows(rows)

		// Set up expectations for contact lists query with error
		mock.ExpectQuery(`SELECT cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, cl\.deleted_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email = \$1`).
			WithArgs(email).
			WillReturnError(errors.New("database error"))

		// Use GetContactByEmail which uses fetchContact internally
		_, err := repo.GetContactByEmail(context.Background(), "workspace123", email)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch contact lists")
	})
}

func TestGetContacts(t *testing.T) {
	t.Run("should get contacts with pagination", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer", 100.0, 5, time.Now(),
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT c\.\* FROM contacts c ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs().
			WillReturnRows(rows)

		// Set up expectations for the contact lists query
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		}).AddRow(
			"test@example.com", "list1", "active", time.Now(), time.Now(), "Marketing List",
		)

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)
		assert.Len(t, resp.Contacts[0].ContactLists, 1)
		assert.Equal(t, "list1", resp.Contacts[0].ContactLists[0].ListID)
		assert.Equal(t, domain.ContactListStatusActive, resp.Contacts[0].ContactLists[0].Status)
		assert.Equal(t, "Marketing List", resp.Contacts[0].ContactLists[0].ListName)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should get contacts with multiple filters", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer", 100.0, 5, time.Now(),
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c\.email ILIKE \$1 AND c\.first_name ILIKE \$2 AND c\.country ILIKE \$3 ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs("%test@example.com%", "%John%", "%US%").
			WillReturnRows(rows)

		// Set up expectations for the contact lists query
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		}).AddRow(
			"test@example.com", "list1", "active", time.Now(), time.Now(), "Marketing List",
		)

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Email:            "test@example.com",
			FirstName:        "John",
			Country:          "US",
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)
		assert.Len(t, resp.Contacts[0].ContactLists, 1)
		assert.Equal(t, "list1", resp.Contacts[0].ContactLists[0].ListID)
		assert.Equal(t, domain.ContactListStatusActive, resp.Contacts[0].ContactLists[0].Status)
		assert.Equal(t, "Marketing List", resp.Contacts[0].ContactLists[0].ListName)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should handle cursor pagination with base64 encoding", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query
		// Create multiple rows to trigger pagination
		now := time.Now()
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		})

		// Add multiple contacts to ensure pagination works
		for i := 1; i <= 11; i++ { // 11 to trigger the limit+1 logic
			rows.AddRow(
				fmt.Sprintf("test%d@example.com", i), fmt.Sprintf("ext%d", i), "UTC", "en",
				fmt.Sprintf("First%d", i), fmt.Sprintf("Last%d", i),
				fmt.Sprintf("+%d", i), "123 Main St", "Apt 4B", "US", "12345", "CA",
				"Engineer", 100.0, 5, now,
				"custom1", "custom2", "custom3", "custom4", "custom5",
				1.0, 2.0, 3.0, 4.0, 5.0,
				now, now, now, now, now,
				[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
				[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
				now.Add(time.Duration(-i)*time.Hour), now, // Use decreasing created_at times
			)
		}

		// Truncate time to seconds to match the expected format
		cursorTime := time.Now().Truncate(time.Second)
		cursorEmail := "previous@example.com"
		cursorStr := fmt.Sprintf("%s~%s", cursorTime.Format(time.RFC3339), cursorEmail)
		encodedCursor := base64.StdEncoding.EncodeToString([]byte(cursorStr))

		// Parse the time back from the string to ensure it matches exactly what the test expects
		parsedTime, _ := time.Parse(time.RFC3339, cursorTime.Format(time.RFC3339))

		// The query should have compound condition for cursor-based pagination
		// Use a simpler regex pattern that's more forgiving of whitespace variations
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE \(c\.created_at < \$1 OR \(c\.created_at = \$2 AND c\.email > \$3\)\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs(parsedTime, parsedTime, cursorEmail).
			WillReturnRows(rows)

		// Set up expectations for the contact lists query - should have multiple emails
		emails := make([]string, 10) // We only get 10 because the 11th is cut off for pagination
		for i := 1; i <= 10; i++ {
			emails[i-1] = fmt.Sprintf("test%d@example.com", i)
		}

		// Create the expected SQL pattern for the IN query with multiple params
		// Use this simpler pattern to match the actual SQL generated
		sqlPattern := `SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1,\$2,\$3,\$4,\$5,\$6,\$7,\$8,\$9,\$10\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`

		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		})

		// Add contact list records for each email
		for _, email := range emails {
			listRows.AddRow(
				email, "list1", "active", now, now, "Marketing List",
			)
		}

		// Convert emails to proper args for the mock
		emailArgs := make([]driver.Value, len(emails))
		for i, email := range emails {
			emailArgs[i] = email
		}

		mock.ExpectQuery(sqlPattern).
			WithArgs(emailArgs...).
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Cursor:           encodedCursor,
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 10) // Should get 10 contacts

		// Verify first and last contact
		assert.Equal(t, "test1@example.com", resp.Contacts[0].Email)
		assert.Equal(t, "test10@example.com", resp.Contacts[9].Email)

		// Verify contact lists
		for _, contact := range resp.Contacts {
			assert.Len(t, contact.ContactLists, 1)
			assert.Equal(t, "list1", contact.ContactLists[0].ListID)
			assert.Equal(t, domain.ContactListStatusActive, contact.ContactLists[0].Status)
		}

		assert.NoError(t, mock.ExpectationsWereMet())

		// Verify the next cursor is base64 encoded and contains expected data
		require.NotEmpty(t, resp.NextCursor, "NextCursor should not be empty")

		decodedBytes, err := base64.StdEncoding.DecodeString(resp.NextCursor)
		require.NoError(t, err)

		cursorParts := strings.Split(string(decodedBytes), "~")
		require.Len(t, cursorParts, 2)

		_, err = time.Parse(time.RFC3339, cursorParts[0])
		require.NoError(t, err)

		// The 11th contact email should be in the cursor
		assert.Equal(t, "test10@example.com", cursorParts[1])
	})

	t.Run("should handle invalid base64 encoded cursor", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Cursor:           "invalid-base64-data",
			Limit:            10,
			WithContactLists: true,
		}

		_, err := repo.GetContacts(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cursor encoding")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should handle invalid cursor format after base64 decoding", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Create a cursor with invalid format (missing tilde separator)
		invalidCursor := base64.StdEncoding.EncodeToString([]byte("invalid-cursor-format"))

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Cursor:           invalidCursor,
			Limit:            10,
			WithContactLists: true,
		}

		_, err := repo.GetContacts(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cursor format: expected timestamp~email")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should handle invalid timestamp in cursor", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Create a cursor with invalid timestamp
		invalidCursor := base64.StdEncoding.EncodeToString([]byte("invalid-time~email@example.com"))

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Cursor:           invalidCursor,
			Limit:            10,
			WithContactLists: true,
		}

		_, err := repo.GetContacts(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cursor timestamp format")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should handle workspace connection errors", func(t *testing.T) {
		// Create a new mock workspace repository without a DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(nil, errors.New("failed to get workspace connection")).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Limit:            10,
			WithContactLists: true,
		}

		_, err := repo.GetContacts(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("should handle complex filter combinations", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer", 100.0, 5, time.Now(),
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE c\.email ILIKE \$1 AND c\.external_id ILIKE \$2 AND c\.first_name ILIKE \$3 AND c\.last_name ILIKE \$4 AND c\.phone ILIKE \$5 AND c\.country ILIKE \$6 ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs("%test@example.com%", "%ext123%", "%John%", "%Doe%", "%+1234567890%", "%US%").
			WillReturnRows(rows)

		// Set up expectations for the contact lists query
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		}).AddRow(
			"test@example.com", "list1", "active", time.Now(), time.Now(), "Marketing List",
		)

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Email:            "test@example.com",
			ExternalID:       "ext123",
			FirstName:        "John",
			LastName:         "Doe",
			Phone:            "+1234567890",
			Country:          "US",
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)
		assert.Len(t, resp.Contacts[0].ContactLists, 1)
		assert.Equal(t, "list1", resp.Contacts[0].ContactLists[0].ListID)
		assert.Equal(t, domain.ContactListStatusActive, resp.Contacts[0].ContactLists[0].Status)
		assert.Equal(t, "Marketing List", resp.Contacts[0].ContactLists[0].ListName)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should handle database query errors", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the query to fail
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs().
			WillReturnError(errors.New("database query error"))

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Limit:            10,
			WithContactLists: false,
		}

		_, err := repo.GetContacts(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute query")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should filter contacts by list_id", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query with EXISTS subquery
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer", 100.0, 5, time.Now(),
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_lists cl WHERE cl\.email = c\.email AND cl\.deleted_at IS NULL AND cl\.list_id = \$1\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs("list123").
			WillReturnRows(rows)

		// Set up expectations for the contact lists query (should fetch ALL lists, not just the one used for filtering)
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		}).
			AddRow("test@example.com", "list123", "active", time.Now(), time.Now(), "Marketing List").
			AddRow("test@example.com", "list456", "active", time.Now(), time.Now(), "Sales List")

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			ListID:           "list123",
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)

		// Should return ALL lists the contact belongs to (both list123 and list456)
		require.Len(t, resp.Contacts[0].ContactLists, 2)
		assert.Contains(t, []string{resp.Contacts[0].ContactLists[0].ListID, resp.Contacts[0].ContactLists[1].ListID}, "list123")
		assert.Contains(t, []string{resp.Contacts[0].ContactLists[0].ListID, resp.Contacts[0].ContactLists[1].ListID}, "list456")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should filter contacts by contact_list_status", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query with EXISTS subquery
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer", 100.0, 5, time.Now(),
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_lists cl WHERE cl\.email = c\.email AND cl\.deleted_at IS NULL AND cl\.status = \$1\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs(string(domain.ContactListStatusActive)).
			WillReturnRows(rows)

		// Set up expectations for the contact lists query
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		}).
			AddRow("test@example.com", "list123", "active", time.Now(), time.Now(), "Marketing List").
			AddRow("test@example.com", "list456", "pending", time.Now(), time.Now(), "Sales List")

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:       "workspace123",
			ContactListStatus: string(domain.ContactListStatusActive),
			Limit:             10,
			WithContactLists:  true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)

		// Should return ALL lists the contact belongs to (both active and pending)
		require.Len(t, resp.Contacts[0].ContactLists, 2)
		assert.Contains(t, []string{string(resp.Contacts[0].ContactLists[0].Status), string(resp.Contacts[0].ContactLists[1].Status)}, "active")
		assert.Contains(t, []string{string(resp.Contacts[0].ContactLists[0].Status), string(resp.Contacts[0].ContactLists[1].Status)}, "pending")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should filter contacts by both list_id and contact_list_status", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query with EXISTS subquery
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer", 100.0, 5, time.Now(),
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery with both list_id and status filters
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_lists cl WHERE cl\.email = c\.email AND cl\.deleted_at IS NULL AND cl\.list_id = \$1 AND cl\.status = \$2\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs("list123", string(domain.ContactListStatusActive)).
			WillReturnRows(rows)

		// Set up expectations for the contact lists query
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		}).
			AddRow("test@example.com", "list123", "active", time.Now(), time.Now(), "Marketing List").
			AddRow("test@example.com", "list456", "pending", time.Now(), time.Now(), "Sales List")

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:       "workspace123",
			ListID:            "list123",
			ContactListStatus: string(domain.ContactListStatusActive),
			Limit:             10,
			WithContactLists:  true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)

		// Should return ALL lists the contact belongs to
		require.Len(t, resp.Contacts[0].ContactLists, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGetContactsForBroadcast(t *testing.T) {
	t.Run("should get contacts for broadcast with list filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings
		audience := domain.AudienceSettings{
			Lists:               []string{"list1", "list2"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Set up expectations for the database query with all 40 columns (38 contact + 2 list)
		now := time.Now().UTC().Truncate(time.Microsecond)
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
			"list_id", "list_name", // Additional columns for list filtering (makes it 40 total)
		}).
			AddRow(
				"test1@example.com", "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				100.50, 5, now,
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now,
				"list1", "Marketing List", // Additional values for list filtering
			).
			AddRow(
				"test2@example.com", "ext456", "America/New_York", "en-US",
				"Jane", "Smith", "+0987654321", "456 Oak Ave", "",
				"USA", "54321", "NY", "Designer",
				200.50, 10, now,
				"Custom 1-2", "Custom 2-2", "Custom 3-2", "Custom 4-2", "Custom 5-2",
				52.0, 53.0, 54.0, 55.0, 56.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1-2"}`), []byte(`{"key": "value2-2"}`), []byte(`{"key": "value3-2"}`), []byte(`{"key": "value4-2"}`), []byte(`{"key": "value5-2"}`),
				now, now,
				"list2", "Sales List", // Additional values for list filtering
			)

			// Expect query with JOINS for list filtering and excludeUnsubscribed
		mock.ExpectQuery(`SELECT c\.\*, cl\.list_id, l\.name as list_name FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email JOIN lists l ON cl\.list_id = l\.id WHERE cl\.list_id IN \(\$1,\$2\) AND l\.deleted_at IS NULL AND cl\.status <> \$3 AND cl\.status <> \$4 AND cl\.status <> \$5 ORDER BY c\.created_at ASC LIMIT 10 OFFSET 0`).
			WithArgs("list1", "list2",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnRows(rows)

		// Call the method being tested
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, 0)

		// Assertions
		require.NoError(t, err)
		require.Len(t, contacts, 2)

		// Check contact emails and list information
		assert.Equal(t, "test1@example.com", contacts[0].Contact.Email)
		assert.Equal(t, "list1", contacts[0].ListID)
		assert.Equal(t, "Marketing List", contacts[0].ListName)

		assert.Equal(t, "test2@example.com", contacts[1].Contact.Email)
		assert.Equal(t, "list2", contacts[1].ListID)
		assert.Equal(t, "Sales List", contacts[1].ListName)
	})

	t.Run("should handle deduplication (skip_duplicate_emails=true)", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with deduplication enabled
		audience := domain.AudienceSettings{
			Lists:               []string{"list1", "list2"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: true, // Enable deduplication
		}

		// Set up expectations for the database query with all 40 columns (38 contact + 2 list)
		now := time.Now().UTC().Truncate(time.Microsecond)
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
			"list_id", "list_name", // Additional columns for list filtering (makes it 40 total)
		}).
			AddRow(
				"test1@example.com", "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				100.50, 5, now,
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now,
				"list1", "Marketing List", // Additional values for list filtering
			)

		// Expect query with DISTINCT ON for deduplication
		mock.ExpectQuery(`SELECT DISTINCT ON \(c\.email\) .*`).
			WithArgs("list1", "list2",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnRows(rows)

		// Call the method being tested
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, 0)

		// Assertions
		require.NoError(t, err)
		require.Len(t, contacts, 1)
		assert.Equal(t, "test1@example.com", contacts[0].Contact.Email)
		assert.Equal(t, "list1", contacts[0].ListID)
		assert.Equal(t, "Marketing List", contacts[0].ListName)
	})

	t.Run("should get contacts without list filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with no lists or segments
		audience := domain.AudienceSettings{
			// Empty lists array
			Lists:               []string{},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Set up expectations for the database query with only 38 contact columns (no list columns)
		now := time.Now().UTC().Truncate(time.Microsecond)
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"lifetime_value", "orders_count", "last_order_at",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at",
		}).
			AddRow(
				"test1@example.com", "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				100.50, 5, now,
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now,
			).
			AddRow(
				"test2@example.com", "ext456", "America/New_York", "en-US",
				"Jane", "Smith", "+0987654321", "456 Oak Ave", "",
				"USA", "54321", "NY", "Designer",
				200.50, 10, now,
				"Custom 1-2", "Custom 2-2", "Custom 3-2", "Custom 4-2", "Custom 5-2",
				52.0, 53.0, 54.0, 55.0, 56.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1-2"}`), []byte(`{"key": "value2-2"}`), []byte(`{"key": "value3-2"}`), []byte(`{"key": "value4-2"}`), []byte(`{"key": "value5-2"}`),
				now, now,
			)

		// Expect query without JOINS for all contacts
		mock.ExpectQuery(`SELECT c\.\* FROM contacts c ORDER BY c\.created_at ASC LIMIT 10 OFFSET 0`).
			WillReturnRows(rows)

		// Call the method being tested
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, 0)

		// Assertions
		require.NoError(t, err)
		require.Len(t, contacts, 2)

		// Check first contact
		assert.Equal(t, "test1@example.com", contacts[0].Contact.Email)
		assert.Empty(t, contacts[0].ListID)
		assert.Empty(t, contacts[0].ListName)

		// Check second contact
		assert.Equal(t, "test2@example.com", contacts[1].Contact.Email)
		assert.Empty(t, contacts[1].ListID)
		assert.Empty(t, contacts[1].ListName)
	})

	t.Run("should handle database connection error", func(t *testing.T) {
		// Create a mock workspace database
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").
			Return(nil, fmt.Errorf("connection error"))

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings
		audience := domain.AudienceSettings{
			Lists:               []string{"list1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Call the method being tested
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, 0)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.Nil(t, contacts)
	})

	t.Run("should handle database query error", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings
		audience := domain.AudienceSettings{
			Lists:               []string{"list1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Expect query with error
		mock.ExpectQuery(`SELECT c\.\*, cl\.list_id, l\.name as list_name FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email JOIN lists l ON cl\.list_id = l\.id WHERE cl\.list_id IN \(\$1\) AND l\.deleted_at IS NULL AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4 ORDER BY c\.created_at ASC LIMIT 10 OFFSET 0`).
			WithArgs("list1",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnError(fmt.Errorf("database error"))

		// Call the method being tested
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, 0)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute query")
		assert.Nil(t, contacts)
	})

	t.Run("should return error for segments filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, _, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with segments
		audience := domain.AudienceSettings{
			Segments:            []string{"segment1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Call the method being tested
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, 0)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "segments filtering not implemented")
		assert.Nil(t, contacts)
	})
}

func TestCountContactsForBroadcast(t *testing.T) {
	t.Run("should count contacts for broadcast with list filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings
		audience := domain.AudienceSettings{
			Lists:               []string{"list1", "list2"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(25)

		// Expect query with JOINS for list filtering and excludeUnsubscribed
		// Note: SkipDuplicateEmails is false, so we expect COUNT(*) not COUNT(DISTINCT)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email WHERE cl\.list_id IN \(\$1,\$2\) AND cl\.status <> \$3 AND cl\.status <> \$4 AND cl\.status <> \$5`).
			WithArgs("list1", "list2",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnRows(rows)

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.NoError(t, err)
		assert.Equal(t, 25, count)
	})

	t.Run("should count all contacts without filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with no lists
		audience := domain.AudienceSettings{
			Lists:               []string{},
			ExcludeUnsubscribed: false,
			SkipDuplicateEmails: false,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(100)

		// Expect simple count query without filtering
		// Note: SkipDuplicateEmails is false, so we expect COUNT(*) not COUNT(DISTINCT)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts c`).
			WillReturnRows(rows)

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.NoError(t, err)
		assert.Equal(t, 100, count)
	})

	t.Run("should count distinct emails when SkipDuplicateEmails is true", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with SkipDuplicateEmails enabled
		audience := domain.AudienceSettings{
			Lists:               []string{"list1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: true,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(90)

		// Expect query with DISTINCT when SkipDuplicateEmails is true
		mock.ExpectQuery(`SELECT COUNT\(DISTINCT c\.email\) FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email WHERE cl\.list_id IN \(\$1\) AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4`).
			WithArgs("list1",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnRows(rows)

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.NoError(t, err)
		assert.Equal(t, 90, count)
	})

	t.Run("should handle database connection error", func(t *testing.T) {
		// Create a mock workspace database
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").
			Return(nil, fmt.Errorf("connection error"))

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings
		audience := domain.AudienceSettings{
			Lists:               []string{"list1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.Equal(t, 0, count)
	})

	t.Run("should handle database query error", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings
		audience := domain.AudienceSettings{
			Lists:               []string{"list1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Expect query with error
		mock.ExpectQuery(`SELECT COUNT\(DISTINCT c\.email\) FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email WHERE cl\.list_id IN \(\$1\) AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4`).
			WithArgs("list1",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnError(fmt.Errorf("database error"))

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute count query")
		assert.Equal(t, 0, count)
	})

	t.Run("should return error for segments filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, _, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with segments
		audience := domain.AudienceSettings{
			Segments:            []string{"segment1"},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: false,
		}

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "segments filtering not implemented")
		assert.Equal(t, 0, count)
	})
}

func TestDeleteContact(t *testing.T) {
	t.Run("should successfully delete existing contact", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		email := "test@example.com"

		// Set up expectations for the delete query
		mock.ExpectExec(`DELETE FROM contacts WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

		// Call the method being tested
		err := repo.DeleteContact(context.Background(), "workspace123", email)

		// Assertions
		require.NoError(t, err)
	})

	t.Run("should return error when contact not found", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		email := "nonexistent@example.com"

		// Set up expectations for the delete query with 0 rows affected
		mock.ExpectExec(`DELETE FROM contacts WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

		// Call the method being tested
		err := repo.DeleteContact(context.Background(), "workspace123", email)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contact not found")
	})

	t.Run("should handle database connection error", func(t *testing.T) {
		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").
			Return(nil, fmt.Errorf("connection error"))

		repo := NewContactRepository(workspaceRepo)

		email := "test@example.com"

		// Call the method being tested
		err := repo.DeleteContact(context.Background(), "workspace123", email)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("should handle database execution error", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		email := "test@example.com"

		// Set up expectations for the delete query with database error
		mock.ExpectExec(`DELETE FROM contacts WHERE email = \$1`).
			WithArgs(email).
			WillReturnError(fmt.Errorf("database execution error"))

		// Call the method being tested
		err := repo.DeleteContact(context.Background(), "workspace123", email)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete contact")
	})

	t.Run("should handle rows affected error", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		email := "test@example.com"

		// Create a mock result that returns an error when RowsAffected is called
		mockResult := sqlmock.NewErrorResult(fmt.Errorf("rows affected error"))

		// Set up expectations for the delete query
		mock.ExpectExec(`DELETE FROM contacts WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(mockResult)

		// Call the method being tested
		err := repo.DeleteContact(context.Background(), "workspace123", email)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get affected rows")
	})

	t.Run("should handle empty email", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		email := ""

		// Set up expectations for the delete query with empty email
		mock.ExpectExec(`DELETE FROM contacts WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

		// Call the method being tested
		err := repo.DeleteContact(context.Background(), "workspace123", email)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contact not found")
	})

	t.Run("should handle context cancellation", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		email := "test@example.com"

		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Set up expectations for the delete query with context cancellation
		mock.ExpectExec(`DELETE FROM contacts WHERE email = \$1`).
			WithArgs(email).
			WillReturnError(context.Canceled)

		// Call the method being tested
		err := repo.DeleteContact(ctx, "workspace123", email)

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete contact")
	})
}
