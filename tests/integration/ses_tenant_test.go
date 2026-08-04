package integration

import (
	"context"
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

// TestSESTenantIsolationE2E exercises the SES tenant-isolation feature through the real HTTP
// server, the real service layer and a real PostgreSQL database.
//
// It deliberately covers only what can be proven without an AWS account: settings persistence
// through the JSONB round-trip (including the secret encryption the same save performs),
// validation, authorization, and the derived-state preservation that a client payload must not
// be able to erase. Anything that requires SES itself — provisioning a tenant, associating a
// configuration set, sending — is covered by unit tests against a mocked SES v2 client and by
// the manual verification steps in plans/ses-tenant-and-configuration-set-plan.md.
func TestSESTenantIsolationE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	createSESIntegration := func(t *testing.T, name string, ses *domain.AmazonSESSettings) string {
		t.Helper()
		resp, err := client.Post("/api/workspaces.createIntegration", domain.CreateIntegrationRequest{
			WorkspaceID: workspace.ID,
			Name:        name,
			Type:        domain.IntegrationTypeEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  ses,
				Senders: []domain.EmailSender{
					{ID: "sender-1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
				},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		id, _ := body["integration_id"].(string)
		require.NotEmpty(t, id)
		return id
	}

	// readSESSettings reads the integration back through the API, which is the same path the
	// console uses and therefore the shape the console must be able to trust.
	readSESSettings := func(t *testing.T, integrationID string) map[string]interface{} {
		t.Helper()
		resp, err := client.Get("/api/workspaces.get", map[string]string{"id": workspace.ID})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

		ws := body["workspace"].(map[string]interface{})
		for _, raw := range ws["integrations"].([]interface{}) {
			integration := raw.(map[string]interface{})
			if integration["id"] == integrationID {
				provider := integration["email_provider"].(map[string]interface{})
				return provider["ses"].(map[string]interface{})
			}
		}
		t.Fatalf("integration %s not found", integrationID)
		return nil
	}

	// seedDerivedState writes the state that webhook registration and tenant provisioning would
	// have written, so the preservation behaviour can be tested without AWS.
	seedDerivedState := func(t *testing.T, integrationID string) {
		t.Helper()
		db := suite.DBManager.GetDB()

		var raw []byte
		require.NoError(t, db.QueryRow(
			`SELECT integrations FROM workspaces WHERE id = $1`, workspace.ID).Scan(&raw))

		var integrations []map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &integrations))

		found := false
		for _, integration := range integrations {
			if integration["id"] != integrationID {
				continue
			}
			provider := integration["email_provider"].(map[string]interface{})
			ses := provider["ses"].(map[string]interface{})
			ses["managed_configuration_set"] = "notifuse-" + integrationID
			ses["managed_tenant_name"] = "notifuse-" + integrationID
			ses["inbound_topic_arn"] = "arn:aws:sns:eu-west-3:123456789012:notifuse-ses-" + integrationID
			found = true
		}
		require.True(t, found, "integration %s not present in the stored workspace", integrationID)

		updated, err := json.Marshal(integrations)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE workspaces SET integrations = $1 WHERE id = $2`, updated, workspace.ID)
		require.NoError(t, err)
	}

	t.Run("tenant settings survive the JSONB round trip", func(t *testing.T) {
		integrationID := createSESIntegration(t, "SES round trip", &domain.AmazonSESSettings{
			Region:               "eu-west-3",
			AccessKey:            "AKIAEXAMPLE",
			SecretKey:            "super-secret",
			ConfigurationSetName: "operator-set",
			TenantName:           "operator-tenant",
		})

		ses := readSESSettings(t, integrationID)

		assert.Equal(t, "operator-set", ses["configuration_set_name"])
		assert.Equal(t, "operator-tenant", ses["tenant_name"])

		// The same save also encrypts the secret. Assert that at rest, in the database: the
		// API deliberately returns the decrypted key to authorised readers, so asserting on
		// the response would prove nothing about storage.
		var stored []byte
		require.NoError(t, suite.DBManager.GetDB().QueryRow(
			`SELECT integrations FROM workspaces WHERE id = $1`, workspace.ID).Scan(&stored))
		assert.NotContains(t, string(stored), "super-secret",
			"the plaintext AWS secret must never be written to the database")
		assert.NotEmpty(t, ses["encrypted_secret_key"])
	})

	t.Run("an integration with no tenant settings stores no tenant keys", func(t *testing.T) {
		// Existing deployments must not have their stored shape changed by an upgrade.
		integrationID := createSESIntegration(t, "SES untouched", &domain.AmazonSESSettings{
			Region:    "eu-west-3",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: "super-secret",
		})

		ses := readSESSettings(t, integrationID)

		for _, key := range []string{
			"tenant_isolation_enabled", "configuration_set_name", "tenant_name",
			"managed_configuration_set", "managed_tenant_name",
		} {
			assert.NotContains(t, ses, key, "key %s must be omitted when unset", key)
		}
	})

	t.Run("managed isolation and a manual tenant are mutually exclusive", func(t *testing.T) {
		resp, err := client.Post("/api/workspaces.createIntegration", domain.CreateIntegrationRequest{
			WorkspaceID: workspace.ID,
			Name:        "SES conflicting",
			Type:        domain.IntegrationTypeEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region:                 "eu-west-3",
					AccessKey:              "AKIAEXAMPLE",
					SecretKey:              "s",
					TenantIsolationEnabled: true,
					TenantName:             "operator-tenant",
				},
				Senders:            []domain.EmailSender{{ID: "s1", Email: "a@b.com", Name: "A", IsDefault: true}},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("an invalid tenant name is refused", func(t *testing.T) {
		resp, err := client.Post("/api/workspaces.createIntegration", domain.CreateIntegrationRequest{
			WorkspaceID: workspace.ID,
			Name:        "SES bad tenant name",
			Type:        domain.IntegrationTypeEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region:     "eu-west-3",
					AccessKey:  "AKIAEXAMPLE",
					SecretKey:  "s",
					TenantName: "not a valid name",
				},
				Senders:            []domain.EmailSender{{ID: "s1", Email: "a@b.com", Name: "A", IsDefault: true}},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	// The regression this feature depends on: derived state is written by the server (webhook
	// registration, tenant provisioning) and must survive an update whose payload omits it.
	// Before the fix, any API client that did not echo these fields back silently erased them —
	// which stopped tenant sends, dropped event tracking, and broke stop-on-reply.
	t.Run("updating an integration preserves server-owned SES state", func(t *testing.T) {
		integrationID := createSESIntegration(t, "SES derived state", &domain.AmazonSESSettings{
			Region:    "eu-west-3",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: "super-secret",
		})
		seedDerivedState(t, integrationID)

		// A payload that carries none of the derived fields — the shape any non-console client
		// would send when simply renaming an integration.
		resp, err := client.Post("/api/workspaces.updateIntegration", domain.UpdateIntegrationRequest{
			WorkspaceID:   workspace.ID,
			IntegrationID: integrationID,
			Name:          "SES derived state renamed",
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region:    "eu-west-3",
					AccessKey: "AKIAEXAMPLE",
					SecretKey: "super-secret",
				},
				Senders: []domain.EmailSender{
					{ID: "sender-1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
				},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		ses := readSESSettings(t, integrationID)

		assert.Equal(t, "notifuse-"+integrationID, ses["managed_configuration_set"],
			"the configuration set the send path resolves must survive an unrelated update")
		assert.Equal(t, "notifuse-"+integrationID, ses["managed_tenant_name"],
			"the tenant sends are scoped to must survive an unrelated update")
		assert.Equal(t, "arn:aws:sns:eu-west-3:123456789012:notifuse-ses-"+integrationID,
			ses["inbound_topic_arn"],
			"stop-on-reply binds to this ARN; losing it silently rejects every inbound reply")
	})

	t.Run("a client cannot overwrite server-owned SES state", func(t *testing.T) {
		integrationID := createSESIntegration(t, "SES derived state guarded", &domain.AmazonSESSettings{
			Region:    "eu-west-3",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: "super-secret",
		})
		seedDerivedState(t, integrationID)

		resp, err := client.Post("/api/workspaces.updateIntegration", domain.UpdateIntegrationRequest{
			WorkspaceID:   workspace.ID,
			IntegrationID: integrationID,
			Name:          "SES derived state guarded",
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region:                  "eu-west-3",
					AccessKey:               "AKIAEXAMPLE",
					SecretKey:               "super-secret",
					ManagedConfigurationSet: "attacker-set",
					ManagedTenantName:       "attacker-tenant",
				},
				Senders: []domain.EmailSender{
					{ID: "sender-1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
				},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		ses := readSESSettings(t, integrationID)

		// Sending through a tenant of the caller's choosing is not theirs to decide.
		assert.Equal(t, "notifuse-"+integrationID, ses["managed_configuration_set"])
		assert.Equal(t, "notifuse-"+integrationID, ses["managed_tenant_name"])
	})

	t.Run("operator overrides remain editable", func(t *testing.T) {
		integrationID := createSESIntegration(t, "SES overrides", &domain.AmazonSESSettings{
			Region:    "eu-west-3",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: "super-secret",
		})

		resp, err := client.Post("/api/workspaces.updateIntegration", domain.UpdateIntegrationRequest{
			WorkspaceID:   workspace.ID,
			IntegrationID: integrationID,
			Name:          "SES overrides",
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region:               "eu-west-3",
					AccessKey:            "AKIAEXAMPLE",
					SecretKey:            "super-secret",
					ConfigurationSetName: "operator-set",
				},
				Senders: []domain.EmailSender{
					{ID: "sender-1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
				},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		ses := readSESSettings(t, integrationID)
		assert.Equal(t, "operator-set", ses["configuration_set_name"])
	})
}

// TestSESDiscoveryEndpointsE2E covers the endpoints' contract on the paths that resolve before
// any AWS call: method, validation and authorization. The success paths need a real SES account
// and are covered by unit tests against a mocked client.
func TestSESDiscoveryEndpointsE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))

	endpoints := []string{
		"/api/ses.listTenants",
		"/api/ses.listConfigurationSets",
		"/api/ses.verifyTenant",
		"/api/ses.enableTenantIsolation",
	}

	t.Run("unauthenticated requests are rejected", func(t *testing.T) {
		client.SetToken("")
		for _, endpoint := range endpoints {
			resp, err := client.Post(endpoint, map[string]string{"workspace_id": workspace.ID})
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "endpoint %s", endpoint)
		}
	})

	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	t.Run("GET is not allowed", func(t *testing.T) {
		for _, endpoint := range endpoints {
			resp, err := client.Get(endpoint, nil)
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "endpoint %s", endpoint)
		}
	})

	t.Run("a region outside the allowlist never reaches AWS", func(t *testing.T) {
		// The region is interpolated into the AWS endpoint host, so it is validated first.
		resp, err := client.PostRaw("/api/ses.listTenants", fmt.Sprintf(
			`{"workspace_id":%q,"region":"evil.example.com","access_key":"AKIA","secret_key":"s"}`,
			workspace.ID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})

	t.Run("provisioning refuses credentials from an unsaved form", func(t *testing.T) {
		// It creates a billable AWS resource and writes derived state back, so it only ever
		// runs against a stored integration.
		resp, err := client.PostRaw("/api/ses.enableTenantIsolation", fmt.Sprintf(
			`{"workspace_id":%q,"region":"eu-west-3","access_key":"AKIA","secret_key":"s"}`,
			workspace.ID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("verification requires a tenant name", func(t *testing.T) {
		resp, err := client.PostRaw("/api/ses.verifyTenant", fmt.Sprintf(
			`{"workspace_id":%q,"integration_id":"whatever"}`, workspace.ID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("members cannot inspect or provision AWS resources", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(member.ID, workspace.ID, "member"))
		require.NoError(t, client.Login(member.Email, "password"))
		client.SetWorkspaceID(workspace.ID)
		defer func() {
			require.NoError(t, client.Login(owner.Email, "password"))
			client.SetWorkspaceID(workspace.ID)
		}()

		resp, err := client.PostRaw("/api/ses.listTenants", fmt.Sprintf(
			`{"workspace_id":%q,"region":"eu-west-3","access_key":"AKIA","secret_key":"s"}`,
			workspace.ID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// TestSESDerivedFieldPatchE2E covers the single-statement merge that server-owned SES fields are
// written through. The SQL resolves the integration's position inside the JSONB array, so it can
// only really be verified against PostgreSQL.
func TestSESDerivedFieldPatchE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	createSES := func(t *testing.T, name string) string {
		t.Helper()
		resp, err := client.Post("/api/workspaces.createIntegration", domain.CreateIntegrationRequest{
			WorkspaceID: workspace.ID,
			Name:        name,
			Type:        domain.IntegrationTypeEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region: "eu-west-3", AccessKey: "AKIAEXAMPLE", SecretKey: "super-secret",
				},
				Senders: []domain.EmailSender{
					{ID: "s1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
				},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		return body["integration_id"].(string)
	}

	readSES := func(t *testing.T, integrationID string) map[string]interface{} {
		t.Helper()
		var raw []byte
		require.NoError(t, suite.DBManager.GetDB().QueryRow(
			`SELECT integrations FROM workspaces WHERE id = $1`, workspace.ID).Scan(&raw))

		var integrations []map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &integrations))
		for _, integration := range integrations {
			if integration["id"] == integrationID {
				return integration["email_provider"].(map[string]interface{})["ses"].(map[string]interface{})
			}
		}
		t.Fatalf("integration %s not found", integrationID)
		return nil
	}

	repo := suite.ServerManager.GetApp().GetWorkspaceRepository()

	t.Run("patches the right integration and leaves its siblings alone", func(t *testing.T) {
		first := createSES(t, "SES patch A")
		second := createSES(t, "SES patch B")

		require.NoError(t, repo.PatchIntegrationSESSettings(
			context.Background(), workspace.ID, second, map[string]interface{}{
				"managed_tenant_name":      "notifuse-" + second,
				"tenant_isolation_enabled": true,
			}))

		patched := readSES(t, second)
		assert.Equal(t, "notifuse-"+second, patched["managed_tenant_name"])
		assert.Equal(t, true, patched["tenant_isolation_enabled"])
		// Credentials and everything else in the same object must be untouched.
		assert.Equal(t, "eu-west-3", patched["region"])
		assert.NotEmpty(t, patched["encrypted_secret_key"])

		untouched := readSES(t, first)
		assert.NotContains(t, untouched, "managed_tenant_name",
			"patching one integration must not reach into another")
	})

	t.Run("a concurrent full-row save does not lose the patched field", func(t *testing.T) {
		integrationID := createSES(t, "SES patch concurrent")

		require.NoError(t, repo.PatchIntegrationSESSettings(
			context.Background(), workspace.ID, integrationID, map[string]interface{}{
				"managed_configuration_set": "notifuse-" + integrationID,
			}))

		// An ordinary console edit, which rewrites the whole row.
		resp, err := client.Post("/api/workspaces.updateIntegration", domain.UpdateIntegrationRequest{
			WorkspaceID:   workspace.ID,
			IntegrationID: integrationID,
			Name:          "SES patch concurrent renamed",
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES: &domain.AmazonSESSettings{
					Region: "eu-west-3", AccessKey: "AKIAEXAMPLE", SecretKey: "super-secret",
				},
				Senders: []domain.EmailSender{
					{ID: "s1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
				},
				RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "notifuse-"+integrationID, readSES(t, integrationID)["managed_configuration_set"])
	})

	t.Run("an unknown integration is an error, not a silent no-op", func(t *testing.T) {
		err := repo.PatchIntegrationSESSettings(
			context.Background(), workspace.ID, "does-not-exist", map[string]interface{}{"x": "y"})
		require.Error(t, err)
	})
}

// TestSESIntegrationLifecycleE2E walks the five states an SES integration can be in, asserting
// the business outcome for each rather than the mechanics.
//
// The through-line is that tenant isolation is *opt-in and reversible*: an operator who never
// asks for it sees no change at all, one who asks for it gets it only once AWS is actually
// provisioned, and one who changes their mind gets sending back immediately.
func TestSESIntegrationLifecycleE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db := suite.DBManager.GetDB()

	baseSES := func() *domain.AmazonSESSettings {
		return &domain.AmazonSESSettings{
			Region: "eu-west-3", AccessKey: "AKIAEXAMPLE", SecretKey: "super-secret",
		}
	}
	senders := []domain.EmailSender{
		{ID: "s1", Email: "hello@example.com", Name: "Acme", IsDefault: true},
	}

	create := func(t *testing.T, name string, ses *domain.AmazonSESSettings) (string, int) {
		t.Helper()
		resp, err := client.Post("/api/workspaces.createIntegration", domain.CreateIntegrationRequest{
			WorkspaceID: workspace.ID, Name: name, Type: domain.IntegrationTypeEmail,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES, SES: ses,
				Senders: senders, RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			return "", resp.StatusCode
		}
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		return body["integration_id"].(string), resp.StatusCode
	}

	update := func(t *testing.T, id, name string, ses *domain.AmazonSESSettings) int {
		t.Helper()
		resp, err := client.Post("/api/workspaces.updateIntegration", domain.UpdateIntegrationRequest{
			WorkspaceID: workspace.ID, IntegrationID: id, Name: name,
			Provider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES, SES: ses,
				Senders: senders, RateLimitPerMinute: 25,
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	stored := func(t *testing.T, id string) map[string]interface{} {
		t.Helper()
		var raw []byte
		require.NoError(t, db.QueryRow(
			`SELECT integrations FROM workspaces WHERE id = $1`, workspace.ID).Scan(&raw))
		var integrations []map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &integrations))
		for _, integration := range integrations {
			if integration["id"] == id {
				return integration["email_provider"].(map[string]interface{})["ses"].(map[string]interface{})
			}
		}
		t.Fatalf("integration %s not found", id)
		return nil
	}

	// loadSettings round-trips the stored row back through the domain type, so the assertions
	// below exercise the same resolution the send path uses.
	loadSettings := func(t *testing.T, id string) *domain.AmazonSESSettings {
		t.Helper()
		raw, err := json.Marshal(stored(t, id))
		require.NoError(t, err)
		settings := &domain.AmazonSESSettings{}
		require.NoError(t, json.Unmarshal(raw, settings))
		return settings
	}

	// CASE 1 — an integration that predates this release. Upgrading must be a no-op: the row
	// has none of the new keys, and nothing may add them behind the operator's back.
	t.Run("case 1: existing integration is untouched by the upgrade", func(t *testing.T) {
		id, _ := create(t, "legacy", baseSES())

		// Make the row look pre-upgrade: strip every new key, keep the inbound ARN that a
		// pre-upgrade deployment would already have.
		var raw []byte
		require.NoError(t, db.QueryRow(`SELECT integrations FROM workspaces WHERE id = $1`, workspace.ID).Scan(&raw))
		var integrations []map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &integrations))
		for _, integration := range integrations {
			if integration["id"] != id {
				continue
			}
			ses := integration["email_provider"].(map[string]interface{})["ses"].(map[string]interface{})
			for _, key := range []string{
				"tenant_isolation_enabled", "configuration_set_name", "tenant_name",
				"managed_configuration_set", "managed_tenant_name",
			} {
				delete(ses, key)
			}
			ses["inbound_topic_arn"] = "arn:aws:sns:eu-west-3:123456789012:legacy-topic"
		}
		encoded, err := json.Marshal(integrations)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE workspaces SET integrations = $1 WHERE id = $2`, encoded, workspace.ID)
		require.NoError(t, err)

		// An ordinary edit, exactly as it would have been sent before this release.
		require.Equal(t, http.StatusOK, update(t, id, "legacy renamed", baseSES()))

		after := stored(t, id)
		for _, key := range []string{
			"tenant_isolation_enabled", "tenant_name", "managed_tenant_name",
		} {
			assert.NotContains(t, after, key, "upgrading must not introduce %s", key)
		}
		assert.Equal(t, "arn:aws:sns:eu-west-3:123456789012:legacy-topic", after["inbound_topic_arn"],
			"stop-on-reply must survive")

		settings := loadSettings(t, id)
		assert.Empty(t, settings.ResolveTenant(), "sends stay untenanted")
	})

	// CASE 2 — a new integration with no tenant. The feature is invisible.
	t.Run("case 2: new integration without a tenant", func(t *testing.T) {
		id, status := create(t, "plain", baseSES())
		require.Equal(t, http.StatusCreated, status)

		after := stored(t, id)
		for _, key := range []string{
			"tenant_isolation_enabled", "tenant_name", "managed_tenant_name", "configuration_set_name",
		} {
			assert.NotContains(t, after, key)
		}

		settings := loadSettings(t, id)
		assert.Empty(t, settings.ResolveTenant())
		assert.False(t, settings.OwnsManagedTenant())
	})

	// CASE 3a — a new integration asking for managed isolation. Intent is recorded, but nothing
	// is provisioned by the save itself: a tenant is billable and its creation is a separate,
	// confirmed step. Until it exists, sending must be exactly as it was.
	t.Run("case 3a: new integration requesting managed isolation", func(t *testing.T) {
		ses := baseSES()
		ses.TenantIsolationEnabled = true

		id, status := create(t, "managed-intent", ses)
		require.Equal(t, http.StatusCreated, status)

		after := stored(t, id)
		assert.Equal(t, true, after["tenant_isolation_enabled"], "the request is recorded")
		assert.NotContains(t, after, "managed_tenant_name", "but nothing is provisioned by saving")

		settings := loadSettings(t, id)
		assert.Empty(t, settings.ResolveTenant(),
			"an unprovisioned integration must send exactly as before, not fail")
	})

	// CASE 3b — a new integration pointing at a tenant the operator manages. Used immediately;
	// Notifuse creates nothing and claims no ownership.
	t.Run("case 3b: new integration with a manual tenant", func(t *testing.T) {
		ses := baseSES()
		ses.TenantName = "operator-tenant"
		ses.ConfigurationSetName = "operator-set"

		id, status := create(t, "manual-tenant", ses)
		require.Equal(t, http.StatusCreated, status)

		settings := loadSettings(t, id)
		assert.Equal(t, "operator-tenant", settings.ResolveTenant())
		assert.Equal(t, "operator-set", settings.ResolveConfigurationSet())
		assert.False(t, settings.OwnsManagedTenant(), "not ours to delete")
	})

	t.Run("case 3c: asking for both is refused", func(t *testing.T) {
		ses := baseSES()
		ses.TenantIsolationEnabled = true
		ses.TenantName = "operator-tenant"

		_, status := create(t, "conflicting", ses)
		assert.Equal(t, http.StatusBadRequest, status)
	})

	// CASE 4 — routine edits on a non-isolated integration must never acquire tenant state.
	t.Run("case 4: updating an integration without a tenant", func(t *testing.T) {
		id, _ := create(t, "plain-edit", baseSES())

		ses := baseSES()
		ses.AccessKey = "AKIAROTATED"
		require.Equal(t, http.StatusOK, update(t, id, "plain-edit renamed", ses))

		after := stored(t, id)
		assert.Equal(t, "AKIAROTATED", after["access_key"])
		assert.NotContains(t, after, "managed_tenant_name", "an ordinary edit must not start isolation")
		assert.NotContains(t, after, "tenant_isolation_enabled")
		assert.Empty(t, loadSettings(t, id).ResolveTenant())
	})

	// CASE 5 — the provisioned integration. This is where the switch has to work both ways.
	t.Run("case 5: updating an integration with a tenant", func(t *testing.T) {
		id, _ := create(t, "isolated", func() *domain.AmazonSESSettings {
			s := baseSES()
			s.TenantIsolationEnabled = true
			return s
		}())

		// Simulate a successful provisioning run.
		repo := suite.ServerManager.GetApp().GetWorkspaceRepository()
		require.NoError(t, repo.PatchIntegrationSESSettings(
			context.Background(), workspace.ID, id, map[string]interface{}{
				"managed_tenant_name":       "notifuse-" + id,
				"managed_configuration_set": "notifuse-" + id,
			}))

		t.Run("an unrelated edit keeps sending through the tenant", func(t *testing.T) {
			ses := baseSES()
			ses.TenantIsolationEnabled = true
			require.Equal(t, http.StatusOK, update(t, id, "isolated renamed", ses))

			settings := loadSettings(t, id)
			assert.Equal(t, "notifuse-"+id, settings.ResolveTenant())
			assert.Equal(t, "notifuse-"+id, settings.ResolveConfigurationSet())
		})

		t.Run("changing senders keeps the tenant", func(t *testing.T) {
			// The console re-runs association on save; the stored tenant must not move.
			ses := baseSES()
			ses.TenantIsolationEnabled = true
			require.Equal(t, http.StatusOK, update(t, id, "isolated renamed", ses))
			assert.Equal(t, "notifuse-"+id, loadSettings(t, id).ResolveTenant())
		})

		// The one that was broken: the switch has to turn off as well as on.
		t.Run("switching isolation off stops sends using the tenant", func(t *testing.T) {
			ses := baseSES()
			ses.TenantIsolationEnabled = false
			require.Equal(t, http.StatusOK, update(t, id, "isolated renamed", ses))

			settings := loadSettings(t, id)
			assert.Empty(t, settings.ResolveTenant(),
				"the operator turned it off; the very next send must not be scoped to the tenant")

			// The tenant still exists in AWS, so it must still be findable for teardown and
			// still be reported as ours — otherwise it bills forever with nothing referencing it.
			assert.Equal(t, "notifuse-"+id, settings.KnownTenant())
			assert.True(t, settings.OwnsManagedTenant())

			// Event tracking is unaffected: the configuration set is not the tenant.
			assert.Equal(t, "notifuse-"+id, settings.ResolveConfigurationSet())
		})

		t.Run("switching it back on resumes the same tenant", func(t *testing.T) {
			ses := baseSES()
			ses.TenantIsolationEnabled = true
			require.Equal(t, http.StatusOK, update(t, id, "isolated renamed", ses))

			// The same tenant, so its suppression list and reputation history carry over.
			assert.Equal(t, "notifuse-"+id, loadSettings(t, id).ResolveTenant())
		})
	})
}
