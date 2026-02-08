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

// contactColumnsPattern is the regex pattern for matching explicit contact columns in queries.
// This matches the contactColumnsWithPrefix("c") output in contact_postgres.go.
const contactColumnsPattern = `c\.email, c\.external_id, c\.timezone, c\.language, c\.first_name, c\.last_name, c\.full_name, c\.phone, c\.address_line_1, c\.address_line_2, c\.country, c\.postcode, c\.state, c\.job_title, c\.custom_string_1, c\.custom_string_2, c\.custom_string_3, c\.custom_string_4, c\.custom_string_5, c\.custom_number_1, c\.custom_number_2, c\.custom_number_3, c\.custom_number_4, c\.custom_number_5, c\.custom_datetime_1, c\.custom_datetime_2, c\.custom_datetime_3, c\.custom_datetime_4, c\.custom_datetime_5, c\.custom_json_1, c\.custom_json_2, c\.custom_json_3, c\.custom_json_4, c\.custom_json_5, c\.created_at, c\.updated_at, c\.db_created_at, c\.db_updated_at`

// setupMockDB creates a mock database and sqlmock for testing
func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err, "Failed to create mock database")

	cleanup := func() {
		_ = db.Close()
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
		"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
		"country", "postcode", "state", "job_title",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
		"created_at", "updated_at", "db_created_at", "db_updated_at",
	}).
		AddRow(
			email, "ext123", "Europe/Paris", "en-US",
			"John", "Doe", "John Doe", "+1234567890", "123 Main St", "Apt 4B",
			"USA", "12345", "CA", "Developer",
			"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
			42.0, 43.0, 44.0, 45.0, 46.0,
			now, now, now, now, now,
			[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
			now, now, now, now,
		)

	mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.email = \$1`).
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

	// Set up expectations for contact segments query
	segmentRows := sqlmock.NewRows([]string{
		"segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
	}).AddRow(
		"segment1", int64(1), now, now, "Active Users", "#FF5733",
	)

	mock.ExpectQuery(`SELECT cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email = \$1`).
		WithArgs(email).
		WillReturnRows(segmentRows)

	contact, err := repo.GetContactByEmail(context.Background(), "workspace123", email)
	require.NoError(t, err)
	assert.Equal(t, email, contact.Email)
	assert.Len(t, contact.ContactLists, 1)
	assert.Equal(t, "list1", contact.ContactLists[0].ListID)
	assert.Equal(t, "Marketing List", contact.ContactLists[0].ListName)
	assert.Len(t, contact.ContactSegments, 1)
	assert.Equal(t, "segment1", contact.ContactSegments[0].SegmentID)
	assert.Equal(t, int64(1), contact.ContactSegments[0].Version)

	// Test case 2: Contact not found
	mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.email = \$1`).
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
		"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
		"country", "postcode", "state", "job_title",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
		"created_at", "updated_at", "db_created_at", "db_updated_at",
	}).
		AddRow(
			email, externalID, "Europe/Paris", "en-US",
			"John", "Doe", "John Doe", "+1234567890", "123 Main St", "Apt 4B",
			"USA", "12345", "CA", "Developer",
			"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
			42.0, 43.0, 44.0, 45.0, 46.0,
			now, now, now, now, now,
			[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
			now, now, now, now,
		)

	mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.external_id = \$1`).
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

	// Set up expectations for contact segments query
	segmentRows := sqlmock.NewRows([]string{
		"segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
	}).AddRow(
		"segment1", int64(1), now, now, "Active Users", "#FF5733",
	)

	mock.ExpectQuery(`SELECT cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email = \$1`).
		WithArgs(email).
		WillReturnRows(segmentRows)

	contact, err := repo.GetContactByExternalID(context.Background(), "workspace123", externalID)
	require.NoError(t, err)
	assert.Equal(t, email, contact.Email)
	assert.Equal(t, externalID, contact.ExternalID.String)
	assert.Len(t, contact.ContactLists, 1)
	assert.Equal(t, "list1", contact.ContactLists[0].ListID)
	assert.Equal(t, "Marketing List", contact.ContactLists[0].ListName)
	assert.Len(t, contact.ContactSegments, 1)
	assert.Equal(t, "segment1", contact.ContactSegments[0].SegmentID)

	// Test case 2: Contact not found
	mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.external_id = \$1`).
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
			"first_name", "last_name", "full_name", "phone",
			"address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			email, "e-123", "Europe/Paris", "en-US", "John", "Doe", "John Doe", "", "", "", "", "", "", "",
			"", "", "", "", "", 0, 0, 0, 0, 0, time.Time{}, time.Time{}, time.Time{}, time.Time{}, time.Time{},
			[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.external_id = \$1`).
			WithArgs(externalID).
			WillReturnRows(rows)

		// Set up expectations for contact lists query (empty result)
		listRows := sqlmock.NewRows([]string{
			"list_id", "status", "created_at", "updated_at", "deleted_at", "list_name",
		})

		mock.ExpectQuery(`SELECT cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, cl\.deleted_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email = \$1`).
			WithArgs(email).
			WillReturnRows(listRows)

		// Set up expectations for contact segments query (empty result)
		segmentRows := sqlmock.NewRows([]string{
			"segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})

		mock.ExpectQuery(`SELECT cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email = \$1`).
			WithArgs(email).
			WillReturnRows(segmentRows)

		// Act
		contact, err := repo.GetContactByExternalID(context.Background(), "workspace123", externalID)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, contact)
		assert.Equal(t, email, contact.Email)
		assert.Equal(t, "e-123", contact.ExternalID.String)
		assert.Empty(t, contact.ContactLists)
		assert.Empty(t, contact.ContactSegments)
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
			"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at", "db_created_at", "db_updated_at",
		}).
			AddRow(
				email, "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "John Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now, now, now,
			)

		phone := "+1234567890"
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.phone = \$1`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		}).AddRow(
			"segment1", int64(1), now, now, "Active Users", "#FF5733",
		).AddRow(
			"segment2", int64(2), now, now, "Premium Users", "#00FF00",
		)

		mock.ExpectQuery(`SELECT cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email = \$1`).
			WithArgs(email).
			WillReturnRows(segmentRows)

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
		assert.Len(t, contact.ContactSegments, 2)
		assert.Equal(t, "segment1", contact.ContactSegments[0].SegmentID)
		assert.Equal(t, "segment2", contact.ContactSegments[1].SegmentID)
	})

	t.Run("with error on contact lists query", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at", "db_created_at", "db_updated_at",
		}).
			AddRow(
				email, "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "John Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now, now, now,
			)

		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c.email = \$1`).
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		}).AddRow(
			"test@example.com", "segment1", int64(1), time.Now(), time.Now(), "Active Users", "#FF5733",
		)

		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

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
		assert.Len(t, resp.Contacts[0].ContactSegments, 1)
		assert.Equal(t, "segment1", resp.Contacts[0].ContactSegments[0].SegmentID)
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT `+contactColumnsPattern+` FROM contacts c WHERE c\.email ILIKE \$1 AND c\.first_name ILIKE \$2 AND c\.country ILIKE \$3 ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})
		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

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
		assert.Empty(t, resp.Contacts[0].ContactSegments)
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		})

		// Add multiple contacts to ensure pagination works
		for i := 1; i <= 11; i++ { // 11 to trigger the limit+1 logic
			rows.AddRow(
				fmt.Sprintf("test%d@example.com", i), fmt.Sprintf("ext%d", i), "UTC", "en",
				fmt.Sprintf("First%d", i), fmt.Sprintf("Last%d", i), fmt.Sprintf("First%d Last%d", i, i),
				fmt.Sprintf("+%d", i), "123 Main St", "Apt 4B", "US", "12345", "CA",
				"Engineer",
				"custom1", "custom2", "custom3", "custom4", "custom5",
				1.0, 2.0, 3.0, 4.0, 5.0,
				now, now, now, now, now,
				[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
				[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
				now.Add(time.Duration(-i)*time.Hour), now, now.Add(time.Duration(-i)*time.Hour), now, // Use decreasing created_at times
			)
		}

		// Use nanosecond precision to match the cursor format
		cursorTime := time.Now()
		cursorEmail := "previous@example.com"
		cursorStr := fmt.Sprintf("%s~%s", cursorTime.Format(time.RFC3339Nano), cursorEmail)
		encodedCursor := base64.StdEncoding.EncodeToString([]byte(cursorStr))

		// Parse the time back from the string to ensure it matches exactly what the test expects
		parsedTime, _ := time.Parse(time.RFC3339Nano, cursorTime.Format(time.RFC3339Nano))

		// The query should have compound condition for cursor-based pagination
		// Use a simpler regex pattern that's more forgiving of whitespace variations
		mock.ExpectQuery(`SELECT `+contactColumnsPattern+` FROM contacts c WHERE \(c\.created_at < \$1 OR \(c\.created_at = \$2 AND c\.email > \$3\)\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentSqlPattern := `SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1,\$2,\$3,\$4,\$5,\$6,\$7,\$8,\$9,\$10\)`
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})
		mock.ExpectQuery(segmentSqlPattern).
			WithArgs(emailArgs...).
			WillReturnRows(segmentRows)

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
			assert.Empty(t, contact.ContactSegments)
		}

		assert.NoError(t, mock.ExpectationsWereMet())

		// Verify the next cursor is base64 encoded and contains expected data
		require.NotEmpty(t, resp.NextCursor, "NextCursor should not be empty")

		decodedBytes, err := base64.StdEncoding.DecodeString(resp.NextCursor)
		require.NoError(t, err)

		cursorParts := strings.Split(string(decodedBytes), "~")
		require.Len(t, cursorParts, 2)

		_, err = time.Parse(time.RFC3339Nano, cursorParts[0])
		require.NoError(t, err)

		// The 10th contact email should be in the cursor (last item of the returned page)
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		mock.ExpectQuery(`SELECT `+contactColumnsPattern+` FROM contacts c WHERE c\.email ILIKE \$1 AND c\.external_id ILIKE \$2 AND c\.first_name ILIKE \$3 AND c\.last_name ILIKE \$4 AND c\.phone ILIKE \$5 AND c\.country ILIKE \$6 ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})
		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

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
		assert.Empty(t, resp.Contacts[0].ContactSegments)
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
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_lists cl WHERE cl\.email = c\.email AND cl\.deleted_at IS NULL AND cl\.list_id = \$1\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})
		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

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
		assert.Empty(t, resp.Contacts[0].ContactSegments)
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_lists cl WHERE cl\.email = c\.email AND cl\.deleted_at IS NULL AND cl\.status = \$1\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})
		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

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
		assert.Empty(t, resp.Contacts[0].ContactSegments)
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
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery with both list_id and status filters
		mock.ExpectQuery(`SELECT `+contactColumnsPattern+` FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_lists cl WHERE cl\.email = c\.email AND cl\.deleted_at IS NULL AND cl\.list_id = \$1 AND cl\.status = \$2\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
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

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		})
		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

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
		assert.Empty(t, resp.Contacts[0].ContactSegments)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should filter contacts by segments", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query with EXISTS subquery for segments
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery for segments
		mock.ExpectQuery(`SELECT `+contactColumnsPattern+` FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email = c\.email AND cs\.segment_id IN \(\$1,\$2\)\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs("segment123", "segment456").
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

		// Set up expectations for contact segments query - should return ALL segments the contact belongs to
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		}).
			AddRow("test@example.com", "segment123", int64(1), time.Now(), time.Now(), "Active Users", "#FF5733").
			AddRow("test@example.com", "segment456", int64(1), time.Now(), time.Now(), "Premium Users", "#00FF00").
			AddRow("test@example.com", "segment789", int64(1), time.Now(), time.Now(), "New Users", "#0000FF")

		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Segments:         []string{"segment123", "segment456"},
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)

		// Should return ALL segments the contact belongs to (including segment789 which wasn't in the filter)
		require.Len(t, resp.Contacts[0].ContactSegments, 3)
		segmentIDs := []string{
			resp.Contacts[0].ContactSegments[0].SegmentID,
			resp.Contacts[0].ContactSegments[1].SegmentID,
			resp.Contacts[0].ContactSegments[2].SegmentID,
		}
		assert.Contains(t, segmentIDs, "segment123")
		assert.Contains(t, segmentIDs, "segment456")
		assert.Contains(t, segmentIDs, "segment789")
		assert.Len(t, resp.Contacts[0].ContactLists, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should filter contacts by single segment", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil).AnyTimes()

		repo := NewContactRepository(workspaceRepo)

		// Set up expectations for the workspace database query with EXISTS subquery for single segment
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name",
			"phone", "address_line_1", "address_line_2", "country", "postcode", "state",
			"job_title", "custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4",
			"custom_string_5", "custom_number_1", "custom_number_2", "custom_number_3",
			"custom_number_4", "custom_number_5", "custom_datetime_1", "custom_datetime_2",
			"custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).AddRow(
			"test@example.com", "ext123", "UTC", "en", "John", "Doe", "John Doe",
			"+1234567890", "123 Main St", "Apt 4B", "US", "12345", "CA",
			"Engineer",
			"custom1", "custom2", "custom3", "custom4", "custom5",
			1.0, 2.0, 3.0, 4.0, 5.0,
			time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			[]byte(`{"key": "value"}`), []byte(`{"key": "value"}`),
			time.Now(), time.Now(), time.Now(), time.Now(),
		)

		// Match the query using a regex pattern that includes the EXISTS subquery for a single segment
		// Note: Squirrel generates IN ($1) even for single values
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE EXISTS \(SELECT 1 FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email = c\.email AND cs\.segment_id IN \(\$1\)\) ORDER BY c\.created_at DESC, c\.email ASC LIMIT 11`).
			WithArgs("segment123").
			WillReturnRows(rows)

		// Set up expectations for the contact lists query
		listRows := sqlmock.NewRows([]string{
			"email", "list_id", "status", "created_at", "updated_at", "list_name",
		})

		mock.ExpectQuery(`SELECT cl\.email, cl\.list_id, cl\.status, cl\.created_at, cl\.updated_at, l\.name as list_name FROM contact_lists cl JOIN lists l ON cl\.list_id = l\.id WHERE cl\.email IN \(\$1\) AND cl\.deleted_at IS NULL AND l\.deleted_at IS NULL`).
			WithArgs("test@example.com").
			WillReturnRows(listRows)

		// Set up expectations for contact segments query
		segmentRows := sqlmock.NewRows([]string{
			"email", "segment_id", "version", "matched_at", "computed_at", "segment_name", "segment_color",
		}).AddRow("test@example.com", "segment123", int64(1), time.Now(), time.Now(), "Active Users", "#FF5733")

		mock.ExpectQuery(`SELECT cs\.email, cs\.segment_id, cs\.version, cs\.matched_at, cs\.computed_at, s\.name as segment_name, s\.color as segment_color FROM contact_segments cs JOIN segments s ON cs\.segment_id = s\.id WHERE cs\.email IN \(\$1\)`).
			WithArgs("test@example.com").
			WillReturnRows(segmentRows)

		req := &domain.GetContactsRequest{
			WorkspaceID:      "workspace123",
			Segments:         []string{"segment123"},
			Limit:            10,
			WithContactLists: true,
		}

		resp, err := repo.GetContacts(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, resp.Contacts, 1)
		assert.Equal(t, "test@example.com", resp.Contacts[0].Email)
		require.Len(t, resp.Contacts[0].ContactSegments, 1)
		assert.Equal(t, "segment123", resp.Contacts[0].ContactSegments[0].SegmentID)
		assert.Empty(t, resp.Contacts[0].ContactLists)
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
			List:                "list1",
			ExcludeUnsubscribed: true,
		}

		// Set up expectations for the database query with all 42 columns (40 contact + 2 list)
		now := time.Now().UTC().Truncate(time.Microsecond)
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
			"list_id", "list_name", // Additional columns for list filtering (makes it 42 total)
		}).
			AddRow(
				"test1@example.com", "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "John Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now, now, now,
				"list1", "Marketing List", // Additional values for list filtering
			).
			AddRow(
				"test2@example.com", "ext456", "America/New_York", "en-US",
				"Jane", "Smith", "Jane Smith", "+0987654321", "456 Oak Ave", "",
				"USA", "54321", "NY", "Designer",
				"Custom 1-2", "Custom 2-2", "Custom 3-2", "Custom 4-2", "Custom 5-2",
				52.0, 53.0, 54.0, 55.0, 56.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1-2"}`), []byte(`{"key": "value2-2"}`), []byte(`{"key": "value3-2"}`), []byte(`{"key": "value4-2"}`), []byte(`{"key": "value5-2"}`),
				now, now, now, now,
				"list1", "Marketing List", // Additional values for list filtering - same list
			)

			// Expect query with JOINS for list filtering and excludeUnsubscribed (cursor-based pagination)
		mock.ExpectQuery(`SELECT `+contactColumnsPattern+`, cl\.list_id, l\.name as list_name FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email JOIN lists l ON cl\.list_id = l\.id WHERE cl\.list_id = \$1 AND l\.deleted_at IS NULL AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4 ORDER BY c\.email ASC LIMIT 10`).
			WithArgs("list1",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnRows(rows)

		// Call the method being tested (empty string for first batch cursor)
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, "")

		// Assertions
		require.NoError(t, err)
		require.Len(t, contacts, 2)

		// Check contact emails and list information
		assert.Equal(t, "test1@example.com", contacts[0].Contact.Email)
		assert.Equal(t, "list1", contacts[0].ListID)
		assert.Equal(t, "Marketing List", contacts[0].ListName)

		assert.Equal(t, "test2@example.com", contacts[1].Contact.Email)
		assert.Equal(t, "list1", contacts[1].ListID)
		assert.Equal(t, "Marketing List", contacts[1].ListName)
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
			List:                "",
			ExcludeUnsubscribed: true,
		}

		// Set up expectations for the database query with only 38 contact columns (no list columns)
		now := time.Now().UTC().Truncate(time.Microsecond)
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "full_name", "phone", "address_line_1", "address_line_2",
			"country", "postcode", "state", "job_title",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4",
			"custom_json_5", "created_at", "updated_at", "db_created_at", "db_updated_at",
		}).
			AddRow(
				"test1@example.com", "ext123", "Europe/Paris", "en-US",
				"John", "Doe", "John Doe", "+1234567890", "123 Main St", "Apt 4B",
				"USA", "12345", "CA", "Developer",
				"Custom 1", "Custom 2", "Custom 3", "Custom 4", "Custom 5",
				42.0, 43.0, 44.0, 45.0, 46.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1"}`), []byte(`{"key": "value2"}`), []byte(`{"key": "value3"}`), []byte(`{"key": "value4"}`), []byte(`{"key": "value5"}`),
				now, now, now, now,
			).
			AddRow(
				"test2@example.com", "ext456", "America/New_York", "en-US",
				"Jane", "Smith", "Jane Smith", "+0987654321", "456 Oak Ave", "",
				"USA", "54321", "NY", "Designer",
				"Custom 1-2", "Custom 2-2", "Custom 3-2", "Custom 4-2", "Custom 5-2",
				52.0, 53.0, 54.0, 55.0, 56.0,
				now, now, now, now, now,
				[]byte(`{"key": "value1-2"}`), []byte(`{"key": "value2-2"}`), []byte(`{"key": "value3-2"}`), []byte(`{"key": "value4-2"}`), []byte(`{"key": "value5-2"}`),
				now, now, now, now,
			)

		// Expect query without JOINS for all contacts (cursor-based pagination)
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c ORDER BY c\.email ASC LIMIT 10`).
			WillReturnRows(rows)

		// Call the method being tested (empty string for first batch cursor)
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, "")

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
			List:                "list1",
			ExcludeUnsubscribed: true,
		}

		// Call the method being tested (empty string for first batch cursor)
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, "")

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
			List:                "list1",
			ExcludeUnsubscribed: true,
		}

		// Expect query with error (cursor-based pagination)
		mock.ExpectQuery(`SELECT `+contactColumnsPattern+`, cl\.list_id, l\.name as list_name FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email JOIN lists l ON cl\.list_id = l\.id WHERE cl\.list_id = \$1 AND l\.deleted_at IS NULL AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4 ORDER BY c\.email ASC LIMIT 10`).
			WithArgs("list1",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained).
			WillReturnError(fmt.Errorf("database error"))

		// Call the method being tested (empty string for first batch cursor)
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, "")

		// Assertions
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute query")
		assert.Nil(t, contacts)
	})

	t.Run("should get contacts for broadcast with segments filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
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
			ExcludeUnsubscribed: false,
		}

		// Set up expectations for the query
		// When selecting from contacts with segment filtering, we should see a JOIN with contact_segments
		createdAt1 := time.Now().UTC().Add(-24 * time.Hour)
		createdAt2 := time.Now().UTC()
		rows := sqlmock.NewRows([]string{
			"email", "external_id", "timezone", "language", "first_name", "last_name", "full_name", "phone",
			"address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at", "db_created_at", "db_updated_at",
		}).
			AddRow("test1@example.com", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				nil, nil, nil, nil, nil, createdAt1, createdAt1, createdAt1, createdAt1).
			AddRow("test2@example.com", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				nil, nil, nil, nil, nil, createdAt2, createdAt2, createdAt2, createdAt2)

		// Expect the query to join contacts with contact_segments (cursor-based pagination)
		mock.ExpectQuery(`SELECT ` + contactColumnsPattern + ` FROM contacts c JOIN contact_segments cs ON c\.email = cs\.email WHERE cs\.segment_id IN \(\$1\) ORDER BY c\.email ASC LIMIT 10`).
			WithArgs("segment1").
			WillReturnRows(rows)

		// Call the method being tested (empty string for first batch cursor)
		contacts, err := repo.GetContactsForBroadcast(context.Background(), "workspace123", audience, 10, "")

		// Assertions
		require.NoError(t, err)
		require.Len(t, contacts, 2)
		assert.Equal(t, "test1@example.com", contacts[0].Contact.Email)
		assert.Equal(t, "test2@example.com", contacts[1].Contact.Email)
		// When filtering by segments only, ListID and ListName should be empty
		assert.Equal(t, "", contacts[0].ListID)
		assert.Equal(t, "", contacts[0].ListName)
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
			List:                "list1",
			ExcludeUnsubscribed: true,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(25)

		// Expect query with JOINS for list filtering, soft-deleted lists filtering, and excludeUnsubscribed
		// Note: SkipDuplicateEmails is false, so we expect COUNT(*) not COUNT(DISTINCT)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email JOIN lists l ON cl\.list_id = l\.id WHERE cl\.list_id = \$1 AND l\.deleted_at IS NULL AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4`).
			WithArgs("list1",
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
			List:                "",
			ExcludeUnsubscribed: false,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(100)

		// Expect simple count query without filtering
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts c`).
			WillReturnRows(rows)

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.NoError(t, err)
		assert.Equal(t, 100, count)
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
			List:                "list1",
			ExcludeUnsubscribed: true,
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
			List:                "list1",
			ExcludeUnsubscribed: true,
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

	t.Run("should count contacts for broadcast with segments filtering", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with segments
		audience := domain.AudienceSettings{
			Segments:            []string{"segment1", "segment2"},
			ExcludeUnsubscribed: false,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(42)

		// Expect query with JOIN for segment filtering
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts c JOIN contact_segments cs ON c\.email = cs\.email WHERE cs\.segment_id IN \(\$1,\$2\)`).
			WithArgs("segment1", "segment2").
			WillReturnRows(rows)

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.NoError(t, err)
		assert.Equal(t, 42, count)
	})

	t.Run("should count contacts for broadcast with both lists and segments", func(t *testing.T) {
		// Create a mock workspace database
		mockDB, mock, cleanup := setupMockDB(t)
		defer cleanup()

		// Create a new repository with the mock DB
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(mockDB, nil)

		repo := NewContactRepository(workspaceRepo)

		// Create test audience settings with both lists and segments
		audience := domain.AudienceSettings{
			List:                "list1",
			Segments:            []string{"segment1"},
			ExcludeUnsubscribed: true,
		}

		// Set up expectations for the count query
		rows := sqlmock.NewRows([]string{"count"}).AddRow(15)

		// Expect query with JOINs for both list, lists table (for soft-delete filter), and segment filtering
		// The query should join contact_lists, lists, then also join contact_segments
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts c JOIN contact_lists cl ON c\.email = cl\.email JOIN lists l ON cl\.list_id = l\.id JOIN contact_segments cs ON c\.email = cs\.email WHERE cl\.list_id = \$1 AND l\.deleted_at IS NULL AND cl\.status <> \$2 AND cl\.status <> \$3 AND cl\.status <> \$4 AND cs\.segment_id IN \(\$5\)`).
			WithArgs("list1",
				domain.ContactListStatusUnsubscribed,
				domain.ContactListStatusBounced,
				domain.ContactListStatusComplained,
				"segment1").
			WillReturnRows(rows)

		// Call the method being tested
		count, err := repo.CountContactsForBroadcast(context.Background(), "workspace123", audience)

		// Assertions
		require.NoError(t, err)
		assert.Equal(t, 15, count)
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

func TestContactRepository_BulkUpsertContacts(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceID := "workspace123"
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil).AnyTimes()

	repo := NewContactRepository(workspaceRepo)
	ctx := context.Background()
	now := time.Now()

	t.Run("successful bulk insert of new contacts", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test1@example.com", CreatedAt: now, UpdatedAt: now},
			{Email: "test2@example.com", CreatedAt: now, UpdatedAt: now},
			{Email: "test3@example.com", CreatedAt: now, UpdatedAt: now},
		}

		// Expect transaction begin
		mock.ExpectBegin()

		// Expect the multi-row INSERT with ON CONFLICT
		// Don't match args precisely - too brittle for this complex query
		mock.ExpectQuery(`INSERT INTO contacts`).
			WillReturnRows(
				sqlmock.NewRows([]string{"email", "is_new"}).
					AddRow("test1@example.com", true).
					AddRow("test2@example.com", true).
					AddRow("test3@example.com", true),
			)

		// Expect transaction commit
		mock.ExpectCommit()

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.NoError(t, err)
		assert.Len(t, results, 3)
		assert.Equal(t, "test1@example.com", results[0].Email)
		assert.True(t, results[0].IsNew)
		assert.Equal(t, "test2@example.com", results[1].Email)
		assert.True(t, results[1].IsNew)
		assert.Equal(t, "test3@example.com", results[2].Email)
		assert.True(t, results[2].IsNew)

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("successful bulk upsert with mixed creates and updates", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "new@example.com", CreatedAt: now, UpdatedAt: now},
			{Email: "existing@example.com", CreatedAt: now, UpdatedAt: now},
		}

		mock.ExpectBegin()

		mock.ExpectQuery(`INSERT INTO contacts`).
			WillReturnRows(
				sqlmock.NewRows([]string{"email", "is_new"}).
					AddRow("new@example.com", true).
					AddRow("existing@example.com", false),
			)

		mock.ExpectCommit()

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "new@example.com", results[0].Email)
		assert.True(t, results[0].IsNew)
		assert.Equal(t, "existing@example.com", results[1].Email)
		assert.False(t, results[1].IsNew)

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("empty contacts slice", func(t *testing.T) {
		contacts := []*domain.Contact{}

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("transaction begin fails", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test@example.com", CreatedAt: now, UpdatedAt: now},
		}

		mock.ExpectBegin().WillReturnError(errors.New("begin error"))

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed to begin transaction")

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("query execution fails", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test@example.com", CreatedAt: now, UpdatedAt: now},
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO contacts`).
			WillReturnError(errors.New("query error"))
		mock.ExpectRollback()

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed to execute bulk upsert")

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("row scan fails", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test@example.com", CreatedAt: now, UpdatedAt: now},
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO contacts`).
			WillReturnRows(
				sqlmock.NewRows([]string{"email", "is_new"}).
					AddRow("test@example.com", "invalid_boolean"), // Invalid bool value
			)
		mock.ExpectRollback()

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.Error(t, err)
		assert.Nil(t, results)

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("transaction commit fails", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test@example.com", CreatedAt: now, UpdatedAt: now},
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO contacts`).
			WillReturnRows(
				sqlmock.NewRows([]string{"email", "is_new"}).
					AddRow("test@example.com", true),
			)
		mock.ExpectCommit().WillReturnError(errors.New("commit error"))

		results, err := repo.BulkUpsertContacts(ctx, workspaceID, contacts)

		require.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "failed to commit transaction")

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})
}

func TestContactRepository_Count(t *testing.T) {
	// Test contactRepository.Count - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactRepository(mockWorkspaceRepo)

	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"

	t.Run("Success - Returns count", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

		count, err := repo.Count(ctx, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, 42, count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		count, err := repo.Count(ctx, workspaceID)
		assert.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Error - Query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM contacts`).
			WillReturnError(errors.New("query error"))

		count, err := repo.Count(ctx, workspaceID)
		assert.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to execute count query")
	})
}

func TestContactRepository_GetBatchForSegment(t *testing.T) {
	// Test contactRepository.GetBatchForSegment - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewContactRepository(mockWorkspaceRepo)

	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"

	t.Run("Success - Returns emails", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT email FROM contacts ORDER BY email ASC LIMIT \$1 OFFSET \$2`).
			WithArgs(10, int64(0)).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).
				AddRow("test1@example.com").
				AddRow("test2@example.com"))

		emails, err := repo.GetBatchForSegment(ctx, workspaceID, 0, 10)
		assert.NoError(t, err)
		assert.Len(t, emails, 2)
		assert.Equal(t, "test1@example.com", emails[0])
		assert.Equal(t, "test2@example.com", emails[1])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Empty result", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT email FROM contacts ORDER BY email ASC LIMIT \$1 OFFSET \$2`).
			WithArgs(10, int64(100)).
			WillReturnRows(sqlmock.NewRows([]string{"email"}))

		emails, err := repo.GetBatchForSegment(ctx, workspaceID, 100, 10)
		assert.NoError(t, err)
		assert.Empty(t, emails)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - Connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		emails, err := repo.GetBatchForSegment(ctx, workspaceID, 0, 10)
		assert.Error(t, err)
		assert.Nil(t, emails)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Error - Query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT email FROM contacts ORDER BY email ASC LIMIT \$1 OFFSET \$2`).
			WithArgs(10, int64(0)).
			WillReturnError(errors.New("query error"))

		emails, err := repo.GetBatchForSegment(ctx, workspaceID, 0, 10)
		assert.Error(t, err)
		assert.Nil(t, emails)
		assert.Contains(t, err.Error(), "failed to query emails")
	})
}
