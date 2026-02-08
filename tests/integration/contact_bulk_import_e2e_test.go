package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContactBulkImportE2E tests the complete end-to-end bulk contact import flow
// This test covers:
// - REAL PostgreSQL database operations (catches SQL syntax errors)
// - Bulk insert of new contacts
// - Bulk update of existing contacts
// - Mixed create/update operations
// - List subscription during bulk import
// - PostgreSQL behavior (xmax detection, ON CONFLICT)
// - Transaction integrity
func TestContactBulkImportE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Get workspace database connection for direct queries
	workspaceDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	t.Run("Bulk Insert New Contacts", func(t *testing.T) {
		testBulkInsertNewContacts(t, client, workspaceDB, workspace.ID)
	})

	t.Run("Bulk Update Existing Contacts", func(t *testing.T) {
		testBulkUpdateExistingContacts(t, client, workspaceDB, workspace.ID)
	})

	t.Run("Mixed Create and Update Operations", func(t *testing.T) {
		testMixedCreateUpdateOperations(t, client, workspaceDB, workspace.ID)
	})

	t.Run("Bulk Import with List Subscription", func(t *testing.T) {
		testBulkImportWithListSubscription(t, client, factory, workspaceDB, workspace.ID)
	})

	t.Run("Bulk Import with Validation Errors", func(t *testing.T) {
		testBulkImportWithValidationErrors(t, client, workspace.ID)
	})

	t.Run("Large Batch Performance", func(t *testing.T) {
		testLargeBatchPerformance(t, client, workspaceDB, workspace.ID)
	})

	t.Run("Transaction Integrity", func(t *testing.T) {
		testTransactionIntegrity(t, client, workspaceDB, workspace.ID)
	})

	t.Run("Custom Fields Preservation", func(t *testing.T) {
		testCustomFieldsPreservation(t, client, workspaceDB, workspace.ID)
	})

	t.Run("Duplicate Emails in Single Batch", func(t *testing.T) {
		testDuplicateEmailsInBatch(t, client, workspaceDB, workspace.ID)
	})
}

func testBulkInsertNewContacts(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// Create batch of new contacts - TESTS REAL SQL EXECUTION
	contacts := []map[string]interface{}{
		{
			"email":      fmt.Sprintf("bulk_new1_%d@example.com", time.Now().UnixNano()),
			"first_name": "John",
			"last_name":  "Doe",
		},
		{
			"email":      fmt.Sprintf("bulk_new2_%d@example.com", time.Now().UnixNano()),
			"first_name": "Jane",
			"last_name":  "Smith",
		},
		{
			"email":      fmt.Sprintf("bulk_new3_%d@example.com", time.Now().UnixNano()),
			"first_name": "Bob",
			"last_name":  "Johnson",
		},
	}

	// Import contacts - executes REAL multi-row INSERT against PostgreSQL
	resp, err := client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
			Error  string `json:"error,omitempty"`
		} `json:"operations"`
		Error string `json:"error,omitempty"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	// Verify all operations were successful creates
	assert.Len(t, importResp.Operations, 3)
	for i, op := range importResp.Operations {
		assert.Equal(t, contacts[i]["email"], op.Email)
		assert.Equal(t, "create", op.Action, "Expected create action for new contact")
		assert.Empty(t, op.Error)
	}

	// Verify contacts exist in database with correct data
	for _, contact := range contacts {
		email := contact["email"].(string)
		var firstName, lastName string
		err := workspaceDB.QueryRow(
			"SELECT first_name, last_name FROM contacts WHERE email = $1",
			email,
		).Scan(&firstName, &lastName)
		require.NoError(t, err, "Contact should exist in database")
		assert.Equal(t, contact["first_name"], firstName)
		assert.Equal(t, contact["last_name"], lastName)
	}
}

func testBulkUpdateExistingContacts(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// First, create some contacts
	originalContacts := []map[string]interface{}{
		{
			"email":      fmt.Sprintf("bulk_update1_%d@example.com", time.Now().UnixNano()),
			"first_name": "Original",
			"last_name":  "Name1",
		},
		{
			"email":      fmt.Sprintf("bulk_update2_%d@example.com", time.Now().UnixNano()),
			"first_name": "Original",
			"last_name":  "Name2",
		},
	}

	// Import original contacts
	resp, err := client.BatchImportContacts(originalContacts, nil)
	require.NoError(t, err)
	_ = resp.Body.Close()

	// Wait a moment to ensure different updated_at timestamps
	time.Sleep(100 * time.Millisecond)

	// Update the same contacts with new data - TESTS ON CONFLICT DO UPDATE
	updatedContacts := []map[string]interface{}{
		{
			"email":      originalContacts[0]["email"],
			"first_name": "Updated",
			"last_name":  "Name1",
			"phone":      "+1234567890",
		},
		{
			"email":      originalContacts[1]["email"],
			"first_name": "Updated",
			"last_name":  "Name2",
			"phone":      "+0987654321",
		},
	}

	// Import updated contacts
	resp, err = client.BatchImportContacts(updatedContacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
			Error  string `json:"error,omitempty"`
		} `json:"operations"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	// Verify all operations were successful updates (xmax detection)
	assert.Len(t, importResp.Operations, 2)
	for i, op := range importResp.Operations {
		assert.Equal(t, updatedContacts[i]["email"], op.Email)
		assert.Equal(t, "update", op.Action, "Expected update action for existing contact")
		assert.Empty(t, op.Error)
	}

	// Verify contacts were updated in database
	for _, contact := range updatedContacts {
		email := contact["email"].(string)
		var firstName, lastName, phone string
		err := workspaceDB.QueryRow(
			"SELECT first_name, last_name, COALESCE(phone, '') FROM contacts WHERE email = $1",
			email,
		).Scan(&firstName, &lastName, &phone)
		require.NoError(t, err)
		assert.Equal(t, contact["first_name"], firstName)
		assert.Equal(t, contact["last_name"], lastName)
		assert.Equal(t, contact["phone"], phone)
	}

	// Verify there's still only one row per email (no duplicates)
	for _, contact := range updatedContacts {
		email := contact["email"].(string)
		var count int
		err := workspaceDB.QueryRow(
			"SELECT COUNT(*) FROM contacts WHERE email = $1",
			email,
		).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Should have exactly one contact per email")
	}
}

func testMixedCreateUpdateOperations(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// Create one existing contact
	existingEmail := fmt.Sprintf("mixed_existing_%d@example.com", time.Now().UnixNano())
	resp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      existingEmail,
			"first_name": "Existing",
		},
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Verify the contact was actually created
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		t.Logf("CreateContact failed with status %d: %+v", resp.StatusCode, errResp)
	}
	require.Equal(t, http.StatusOK, resp.StatusCode, "CreateContact should succeed")

	// Verify contact exists in database before the bulk operation
	var existsCount int
	err = workspaceDB.QueryRow("SELECT COUNT(*) FROM contacts WHERE email = $1", existingEmail).Scan(&existsCount)
	require.NoError(t, err)
	require.Equal(t, 1, existsCount, "Contact should exist in database before bulk import")

	// Create batch with mix of new and existing contacts
	newEmail1 := fmt.Sprintf("mixed_new1_%d@example.com", time.Now().UnixNano())
	newEmail2 := fmt.Sprintf("mixed_new2_%d@example.com", time.Now().UnixNano())

	contacts := []map[string]interface{}{
		{
			"email":      newEmail1,
			"first_name": "New",
			"last_name":  "Contact1",
		},
		{
			"email":      existingEmail,
			"first_name": "Updated",
			"last_name":  "Existing",
		},
		{
			"email":      newEmail2,
			"first_name": "New",
			"last_name":  "Contact2",
		},
	}

	// Import mixed batch - TESTS xmax DETECTION ACCURACY
	resp, err = client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
		} `json:"operations"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	// Verify operations
	assert.Len(t, importResp.Operations, 3)

	// Find operations by email
	opsByEmail := make(map[string]string)
	for _, op := range importResp.Operations {
		opsByEmail[op.Email] = op.Action
	}

	assert.Equal(t, "create", opsByEmail[newEmail1], "New contact should be created")
	assert.Equal(t, "update", opsByEmail[existingEmail], "Existing contact should be updated")
	assert.Equal(t, "create", opsByEmail[newEmail2], "New contact should be created")

	// Verify all contacts exist with correct data
	var firstName string
	err = workspaceDB.QueryRow(
		"SELECT first_name FROM contacts WHERE email = $1",
		existingEmail,
	).Scan(&firstName)
	require.NoError(t, err)
	assert.Equal(t, "Updated", firstName, "Existing contact should be updated")
}

func testBulkImportWithListSubscription(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceDB *sql.DB, workspaceID string) {
	// Create a test list
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create batch of contacts to import
	contacts := []map[string]interface{}{
		{
			"email":      fmt.Sprintf("list_sub1_%d@example.com", time.Now().UnixNano()),
			"first_name": "List",
			"last_name":  "Subscriber1",
		},
		{
			"email":      fmt.Sprintf("list_sub2_%d@example.com", time.Now().UnixNano()),
			"first_name": "List",
			"last_name":  "Subscriber2",
		},
	}

	// Import contacts with list subscription - TESTS BULK LIST SUBSCRIPTION
	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify contacts were created
	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
		} `json:"operations"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)
	assert.Len(t, importResp.Operations, 2)

	// Verify contacts are subscribed to the list
	for _, contact := range contacts {
		email := contact["email"].(string)
		var status string
		err := workspaceDB.QueryRow(`
			SELECT status FROM contact_lists 
			WHERE email = $1 AND list_id = $2
		`, email, list.ID).Scan(&status)
		require.NoError(t, err, "Contact should be subscribed to list")
		assert.Equal(t, "active", status)
	}
}

func testBulkImportWithValidationErrors(t *testing.T, client *testutil.APIClient, workspaceID string) {
	// Create batch with some invalid contacts
	contacts := []map[string]interface{}{
		{
			"email":      fmt.Sprintf("valid1_%d@example.com", time.Now().UnixNano()),
			"first_name": "Valid",
		},
		{
			// Missing email - invalid
			"first_name": "Invalid",
		},
		{
			"email":      fmt.Sprintf("valid2_%d@example.com", time.Now().UnixNano()),
			"first_name": "Valid",
		},
	}

	// Import contacts - TESTS PARTIAL SUCCESS BEHAVIOR
	resp, err := client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
			Error  string `json:"error,omitempty"`
		} `json:"operations"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	// Verify response includes all operations
	assert.Len(t, importResp.Operations, 3)

	// Count successful and failed operations
	var successCount, errorCount int
	for _, op := range importResp.Operations {
		if op.Action == "error" {
			errorCount++
			assert.NotEmpty(t, op.Error)
		} else {
			successCount++
		}
	}

	assert.Equal(t, 2, successCount, "Two contacts should succeed")
	assert.Equal(t, 1, errorCount, "One contact should fail validation")
}

func testLargeBatchPerformance(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// Create a large batch of contacts (1000 contacts)
	batchSize := 1000
	contacts := make([]map[string]interface{}, batchSize)
	timestamp := time.Now().UnixNano()

	for i := 0; i < batchSize; i++ {
		contacts[i] = map[string]interface{}{
			"email":      fmt.Sprintf("perf_test_%d_%d@example.com", timestamp, i),
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "Performance",
		}
	}

	// Measure import time - TESTS PERFORMANCE & SCALABILITY
	startTime := time.Now()
	resp, err := client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	duration := time.Since(startTime)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var importResp struct {
		Operations []struct {
			Action string `json:"action"`
		} `json:"operations"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	// Verify all contacts were imported
	assert.Len(t, importResp.Operations, batchSize)
	for _, op := range importResp.Operations {
		assert.Equal(t, "create", op.Action)
	}

	// Performance assertion
	t.Logf("Imported %d contacts in %v", batchSize, duration)
	assert.Less(t, duration, 30*time.Second, "Bulk import should complete within 30 seconds")

	// Verify count in database
	var count int
	err = workspaceDB.QueryRow(
		"SELECT COUNT(*) FROM contacts WHERE email LIKE $1",
		fmt.Sprintf("perf_test_%d_%%@example.com", timestamp),
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, batchSize, count)
}

func testTransactionIntegrity(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// TESTS ACID TRANSACTION PROPERTIES
	timestamp := time.Now().UnixNano()
	email1 := fmt.Sprintf("txn_test1_%d@example.com", timestamp)
	email2 := fmt.Sprintf("txn_test2_%d@example.com", timestamp)

	contacts := []map[string]interface{}{
		{
			"email":      email1,
			"first_name": "Transaction",
			"last_name":  "Test1",
		},
		{
			"email":      email2,
			"first_name": "Transaction",
			"last_name":  "Test2",
		},
	}

	// Import contacts
	resp, err := client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify both contacts exist (transaction was committed)
	for _, contact := range contacts {
		email := contact["email"].(string)
		var exists bool
		err := workspaceDB.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM contacts WHERE email = $1)",
			email,
		).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "Contact should exist after successful batch import")
	}

	// Now update one contact to verify update transaction
	contacts[0]["first_name"] = "Updated"
	resp, err = client.BatchImportContacts(contacts[:1], nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Verify update was applied
	var firstName string
	err = workspaceDB.QueryRow(
		"SELECT first_name FROM contacts WHERE email = $1",
		email1,
	).Scan(&firstName)
	require.NoError(t, err)
	assert.Equal(t, "Updated", firstName)
}

func testCustomFieldsPreservation(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// Test importing a contact with ALL fields populated
	// This ensures the column count matches the placeholder count

	email := fmt.Sprintf("all_fields_%d@example.com", time.Now().UnixNano())
	contacts := []map[string]interface{}{
		{
			"email":             email,
			"external_id":       "ext_123",
			"timezone":          "America/New_York",
			"language":          "en-US",
			"first_name":        "Full",
			"last_name":         "Fields",
			"phone":             "+1234567890",
			"address_line_1":    "123 Main St",
			"address_line_2":    "Apt 4",
			"country":           "US",
			"postcode":          "12345",
			"state":             "NY",
			"job_title":         "Engineer",
			"custom_string_1":   "value1",
			"custom_string_2":   "value2",
			"custom_string_3":   "value3",
			"custom_string_4":   "value4",
			"custom_string_5":   "value5",
			"custom_number_1":   10.5,
			"custom_number_2":   20.5,
			"custom_number_3":   30.5,
			"custom_number_4":   40.5,
			"custom_number_5":   50.5,
			"custom_datetime_1": "2023-01-01T00:00:00Z",
			"custom_datetime_2": "2023-02-01T00:00:00Z",
			"custom_datetime_3": "2023-03-01T00:00:00Z",
			"custom_datetime_4": "2023-04-01T00:00:00Z",
			"custom_datetime_5": "2023-05-01T00:00:00Z",
			"custom_json_1":     map[string]interface{}{"key": "value1"},
			"custom_json_2":     map[string]interface{}{"key": "value2"},
			"custom_json_3":     map[string]interface{}{"key": "value3"},
			"custom_json_4":     map[string]interface{}{"key": "value4"},
			"custom_json_5":     map[string]interface{}{"key": "value5"},
		},
	}

	resp, err := client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
			Error  string `json:"error,omitempty"`
		} `json:"operations"`
		Error string `json:"error,omitempty"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	assert.Empty(t, importResp.Error)
	assert.Len(t, importResp.Operations, 1)
	assert.Equal(t, "create", importResp.Operations[0].Action)
	assert.Empty(t, importResp.Operations[0].Error)

	// Verify ALL fields in database
	var (
		dbExternalID, dbTimezone, dbLanguage, dbFirstName, dbLastName  string
		dbPhone, dbAddressLine1, dbAddressLine2, dbCountry, dbPostcode string
		dbState, dbJobTitle                                            string
		dbCustomString1, dbCustomString2                               string
	)
	err = workspaceDB.QueryRow(`
		SELECT 
			COALESCE(external_id, ''),
			COALESCE(timezone, ''),
			COALESCE(language, ''),
			COALESCE(first_name, ''),
			COALESCE(last_name, ''),
			COALESCE(phone, ''),
			COALESCE(address_line_1, ''),
			COALESCE(address_line_2, ''),
			COALESCE(country, ''),
			COALESCE(postcode, ''),
			COALESCE(state, ''),
			COALESCE(job_title, ''),
			COALESCE(custom_string_1, ''),
			COALESCE(custom_string_2, '')
		FROM contacts WHERE email = $1
	`, email).Scan(
		&dbExternalID, &dbTimezone, &dbLanguage, &dbFirstName, &dbLastName,
		&dbPhone, &dbAddressLine1, &dbAddressLine2, &dbCountry, &dbPostcode,
		&dbState, &dbJobTitle, &dbCustomString1, &dbCustomString2,
	)
	require.NoError(t, err)

	assert.Equal(t, "ext_123", dbExternalID)
	assert.Equal(t, "America/New_York", dbTimezone)
	assert.Equal(t, "en-US", dbLanguage)
	assert.Equal(t, "Full", dbFirstName)
	assert.Equal(t, "Fields", dbLastName)
	assert.Equal(t, "value1", dbCustomString1)
	assert.Equal(t, "value2", dbCustomString2)
}

// testDuplicateEmailsInBatch tests that duplicate emails in a single batch are handled correctly
// This specifically tests the fix for the PostgreSQL error:
// "ON CONFLICT DO UPDATE command cannot affect row a second time"
// The last occurrence of a duplicate email should be kept
func testDuplicateEmailsInBatch(t *testing.T, client *testutil.APIClient, workspaceDB *sql.DB, workspaceID string) {
	// Create a unique email that will appear multiple times
	duplicateEmail := fmt.Sprintf("duplicate_%d@example.com", time.Now().UnixNano())
	uniqueEmail := fmt.Sprintf("unique_%d@example.com", time.Now().UnixNano())

	// Batch with the same email appearing 3 times with different data
	// The last occurrence (with "Third") should be kept
	contacts := []map[string]interface{}{
		{
			"email":      duplicateEmail,
			"first_name": "First",
			"last_name":  "Occurrence",
		},
		{
			"email":      uniqueEmail,
			"first_name": "Unique",
			"last_name":  "Contact",
		},
		{
			"email":      duplicateEmail,
			"first_name": "Second",
			"last_name":  "Occurrence",
		},
		{
			"email":      duplicateEmail,
			"first_name": "Third",  // This should be kept
			"last_name":  "Winner", // This should be kept
		},
	}

	// Import contacts - this would fail before the fix with:
	// "pq: ON CONFLICT DO UPDATE command cannot affect row a second time"
	resp, err := client.BatchImportContacts(contacts, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var importResp struct {
		Operations []struct {
			Email  string `json:"email"`
			Action string `json:"action"`
			Error  string `json:"error,omitempty"`
		} `json:"operations"`
		Error string `json:"error,omitempty"`
	}
	err = json.NewDecoder(resp.Body).Decode(&importResp)
	require.NoError(t, err)

	// Should have no error
	assert.Empty(t, importResp.Error)

	// Should have 2 operations (deduplicated from 4 contacts)
	assert.Len(t, importResp.Operations, 2, "Should have 2 operations after deduplication")

	// All operations should be successful
	for _, op := range importResp.Operations {
		assert.Empty(t, op.Error, "Operation should not have error")
		assert.Equal(t, "create", op.Action, "Should be create for new contacts")
	}

	// Verify only one contact exists in DB for the duplicate email
	var count int
	err = workspaceDB.QueryRow(
		"SELECT COUNT(*) FROM contacts WHERE email = $1",
		duplicateEmail,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly one contact for duplicate email")

	// Verify the data matches the LAST occurrence (first_name="Third", last_name="Winner")
	var firstName, lastName string
	err = workspaceDB.QueryRow(
		"SELECT first_name, last_name FROM contacts WHERE email = $1",
		duplicateEmail,
	).Scan(&firstName, &lastName)
	require.NoError(t, err)
	assert.Equal(t, "Third", firstName, "Should have last occurrence's first_name")
	assert.Equal(t, "Winner", lastName, "Should have last occurrence's last_name")

	// Verify the unique contact also exists
	err = workspaceDB.QueryRow(
		"SELECT first_name, last_name FROM contacts WHERE email = $1",
		uniqueEmail,
	).Scan(&firstName, &lastName)
	require.NoError(t, err)
	assert.Equal(t, "Unique", firstName)
	assert.Equal(t, "Contact", lastName)
}
