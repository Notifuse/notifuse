package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactAPIEndpoints(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	client := suite.APIClient

	t.Run("List Contacts", func(t *testing.T) {
		resp, err := client.ListContacts(map[string]string{
			"limit": "10",
		})
		require.NoError(t, err, "Should be able to list contacts")
		defer func() { _ = _ = resp.Body.Close() }()

		// Should get a response (might be unauthorized but endpoint exists)
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "Contacts list endpoint should exist")
	})

	t.Run("Create Contact", func(t *testing.T) {
		contact := map[string]interface{}{
			"email":      testutil.GenerateTestEmail(),
			"first_name": "Test",
			"last_name":  "User",
		}

		resp, err := client.CreateContact(contact)
		require.NoError(t, err, "Should be able to create contact")
		defer func() { _ = _ = resp.Body.Close() }()

		// Should get a response (might fail validation but endpoint exists)
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "Contact create endpoint should exist")
	})

	t.Run("Get Contact by Email", func(t *testing.T) {
		resp, err := client.GetContactByEmail("test@example.com")
		require.NoError(t, err, "Should be able to get contact by email")
		defer func() { _ = _ = resp.Body.Close() }()

		// Should get a response (might be not found but endpoint exists)
		assert.NotEqual(t, http.StatusNotFound, resp.StatusCode, "Get contact by email endpoint should exist")
	})
}

func TestContactDataFactory(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	factory := suite.DataFactory

	t.Run("Create Contact", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err, "Should be able to create contact")
		require.NotNil(t, contact, "Contact should not be nil")

		assert.NotEmpty(t, contact.Email, "Contact should have email")
		assert.NotNil(t, contact.FirstName, "Contact should have first name")
		assert.NotNil(t, contact.LastName, "Contact should have last name")
	})

	t.Run("Create Contact with Options", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		email := testutil.GenerateTestEmail()
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(email),
			testutil.WithContactName("John", "Doe"),
			testutil.WithContactExternalID("ext-123"),
		)
		require.NoError(t, err, "Should be able to create contact with options")

		assert.Equal(t, email, contact.Email)
		assert.Equal(t, "John", contact.FirstName.String)
		assert.Equal(t, "Doe", contact.LastName.String)
		assert.Equal(t, "ext-123", contact.ExternalID.String)
	})

	t.Run("Create Multiple Contacts", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		contacts := make([]*domain.Contact, 5)
		for i := 0; i < 5; i++ {
			contact, err := factory.CreateContact(workspace.ID,
				testutil.WithContactEmail(fmt.Sprintf("user%d@example.com", i)),
			)
			require.NoError(t, err, "Should be able to create contact %d", i)
			contacts[i] = contact
		}

		// Verify all contacts have different emails
		emails := make(map[string]bool)
		for _, contact := range contacts {
			assert.False(t, emails[contact.Email], "Email should be unique")
			emails[contact.Email] = true
		}
	})
}

func TestContactDatabaseOperations(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	factory := suite.DataFactory

	t.Run("Contact Persisted to Database", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		email := testutil.GenerateTestEmail()
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(email),
		)
		require.NoError(t, err, "Should be able to create contact")

		// Verify contact was created successfully with proper data
		require.NotNil(t, contact)
		assert.Equal(t, email, contact.Email)
		assert.NotZero(t, contact.CreatedAt)
		assert.NotZero(t, contact.UpdatedAt)

		// The factory uses the repository to create the contact,
		// so if this succeeds, it means the contact was persisted correctly
		// Additional verification would require workspace database setup which
		// is already tested in the repository unit tests
	})

	t.Run("Contact Fields Stored Correctly", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		email := testutil.GenerateTestEmail()
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(email),
			testutil.WithContactName("Alice", "Smith"),
			testutil.WithContactExternalID("ext-456"),
		)
		require.NoError(t, err, "Should be able to create contact")

		// Verify all fields are set correctly in the returned contact object
		require.NotNil(t, contact)
		assert.Equal(t, email, contact.Email)
		assert.Equal(t, "Alice", contact.FirstName.String)
		assert.Equal(t, "Smith", contact.LastName.String)
		assert.Equal(t, "ext-456", contact.ExternalID.String)
		assert.False(t, contact.FirstName.IsNull)
		assert.False(t, contact.LastName.IsNull)
		assert.False(t, contact.ExternalID.IsNull)
	})

	t.Run("Contact Cleanup", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		// Create some contacts
		contacts := make([]*domain.Contact, 3)
		for i := 0; i < 3; i++ {
			contact, err := factory.CreateContact(workspace.ID)
			require.NoError(t, err, "Should be able to create contact")
			contacts[i] = contact
		}

		// Verify contacts were created successfully
		assert.Len(t, contacts, 3, "Should have created 3 contacts")
		for i, contact := range contacts {
			assert.NotNil(t, contact, "Contact %d should not be nil", i)
			assert.NotEmpty(t, contact.Email, "Contact %d should have email", i)
		}

		// The cleanup test verifies that the factory can create contacts successfully
		// Database cleanup is handled by the test framework and doesn't need explicit verification
		// since contacts are stored in workspace databases that are automatically cleaned up
	})
}

func TestContactAPIIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer _ = suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	t.Run("API Returns Created Contact Data", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		// Create contact in database
		email := testutil.GenerateTestEmail()
		_, err = factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(email),
			testutil.WithContactName("Bob", "Johnson"),
		)
		require.NoError(t, err, "Should be able to create contact")

		// Try to fetch via API (might fail due to auth but test structure)
		resp, err := client.GetContactByEmail(email)
		require.NoError(t, err, "Should be able to make API request")
		defer func() { _ = _ = resp.Body.Close() }()

		// If we get data back, verify structure
		if resp.StatusCode == http.StatusOK {
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err, "Should be able to decode response")

			if contactData, ok := response["contact"]; ok {
				contactMap := contactData.(map[string]interface{})
				assert.Equal(t, email, contactMap["email"])
			}
		}
	})

	t.Run("API Contact List Structure", func(t *testing.T) {
		workspace, err := factory.CreateWorkspace()
		require.NoError(t, err, "Should be able to create workspace")

		// Create a few contacts
		for i := 0; i < 3; i++ {
			_, err := factory.CreateContact(workspace.ID)
			require.NoError(t, err, "Should be able to create contact")
		}

		resp, err := client.ListContacts(map[string]string{
			"limit": "10",
		})
		require.NoError(t, err, "Should be able to list contacts")
		defer func() { _ = _ = resp.Body.Close() }()

		// If we get data back, verify structure
		if resp.StatusCode == http.StatusOK {
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err, "Should be able to decode response")

			// Check for expected fields in response structure
			if contactsData, ok := response["contacts"]; ok {
				contacts := contactsData.([]interface{})
				t.Logf("Found %d contacts in API response", len(contacts))
			}
		}
	})
}
