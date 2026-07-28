package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateEndpointsExist(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	// Authenticate user (use root user for workspace access)
	email := "test@example.com" // Root user can access workspaces they create
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	// Create a workspace first
	workspaceID := createTestWorkspace(t, client, "Template Test Workspace")

	t.Run("Template Endpoints Exist", func(t *testing.T) {
		endpoints := map[string]string{
			"templates.list":    "/api/templates.list",
			"templates.get":     "/api/templates.get",
			"templates.create":  "/api/templates.create",
			"templates.update":  "/api/templates.update",
			"templates.delete":  "/api/templates.delete",
			"templates.compile": "/api/templates.compile",
		}

		for name, endpoint := range endpoints {
			t.Run(name, func(t *testing.T) {
				params := map[string]string{
					"workspace_id": workspaceID,
				}

				var resp *http.Response
				var err error

				if name == "templates.list" || name == "templates.get" {
					resp, err = client.Get(endpoint, params)
				} else {
					// For POST endpoints, send minimal data
					data := map[string]interface{}{
						"workspace_id": workspaceID,
					}
					resp, err = client.Post(endpoint, data)
				}

				require.NoError(t, err, "Should be able to connect to %s", endpoint)
				defer func() { _ = resp.Body.Close() }()

				// Endpoint should exist (not 404)
				assert.NotEqual(t, http.StatusNotFound, resp.StatusCode,
					"Endpoint %s should exist", endpoint)

				// Endpoint should be accessible (not 405 Method Not Allowed)
				assert.NotEqual(t, http.StatusMethodNotAllowed, resp.StatusCode,
					"Endpoint %s should accept the HTTP method", endpoint)
			})
		}
	})

	t.Run("List Templates Basic", func(t *testing.T) {
		resp, err := client.Get("/api/templates.list", map[string]string{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err, "Should be able to list templates")
		defer func() { _ = resp.Body.Close() }()

		// Should return 200 OK or some valid response
		assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
			"Should get valid response status, got %d", resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			var result map[string]interface{}
			err := client.DecodeJSON(resp, &result)
			require.NoError(t, err, "Should be able to decode JSON response")

			// Should have templates field
			_, hasTemplates := result["templates"]
			assert.True(t, hasTemplates, "Response should contain templates field")
		}
	})

	t.Run("Create Template Basic", func(t *testing.T) {
		template := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           "basic-test-template",
			"name":         "Basic Test Template",
			"channel":      "email",
			"category":     "marketing",
			"email": map[string]interface{}{
				"subject":          "Test Subject",
				"compiled_preview": "<mjml><mj-body><mj-section><mj-column><mj-text>Hello World</mj-text></mj-column></mj-section></mj-body></mjml>",
				"visual_editor_tree": map[string]interface{}{
					"type":       "mjml",
					"attributes": map[string]interface{}{},
					"children":   []interface{}{},
				},
			},
		}

		resp, err := client.CreateTemplate(template)
		require.NoError(t, err, "Should be able to create template")
		defer func() { _ = resp.Body.Close() }()

		// Should return success or meaningful error
		assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
			"Should get valid response status, got %d", resp.StatusCode)

		if resp.StatusCode == http.StatusCreated {
			var result map[string]interface{}
			err := client.DecodeJSON(resp, &result)
			require.NoError(t, err, "Should be able to decode JSON response")

			// Should have template field
			_, hasTemplate := result["template"]
			assert.True(t, hasTemplate, "Response should contain template field")
		}
	})

	t.Run("Template Validation", func(t *testing.T) {
		// Test missing required fields
		template := map[string]interface{}{
			"workspace_id": workspaceID,
			// Missing required fields
		}

		resp, err := client.CreateTemplate(template)
		require.NoError(t, err, "Should be able to make request")
		defer func() { _ = resp.Body.Close() }()

		// Should return 400 Bad Request for missing fields
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"Should return 400 for missing required fields")

		body, err := client.ReadBody(resp)
		require.NoError(t, err, "Should be able to read response body")

		// Should contain error message
		assert.Contains(t, body, "error", "Response should contain error message")
	})
}

func TestTemplateIntegrationBasic(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	// Authenticate user (use root user for workspace access)
	email := "test@example.com" // Root user can access workspaces they create
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	// Create a workspace first
	workspaceID := createTestWorkspace(t, client, "Integration Test Workspace")

	t.Run("Template CRUD Operations", func(t *testing.T) {
		// Test Create
		templateID := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
		template := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           templateID,
			"name":         "Integration Test Template",
			"channel":      "email",
			"category":     "marketing",
			"email": map[string]interface{}{
				"subject":          "Integration Test Subject",
				"compiled_preview": "<mjml><mj-body><mj-section><mj-column><mj-text>Integration Test</mj-text></mj-column></mj-section></mj-body></mjml>",
				"visual_editor_tree": map[string]interface{}{
					"type":       "mjml",
					"attributes": map[string]interface{}{},
					"children":   []interface{}{},
				},
			},
		}

		createResp, err := client.CreateTemplate(template)
		require.NoError(t, err, "Should be able to create template")
		_ = createResp.Body.Close()

		// Test List (should include our template)
		listResp, err := client.Get("/api/templates.list", map[string]string{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err, "Should be able to list templates")
		_ = listResp.Body.Close()

		// Test Get (should retrieve our template)
		getResp, err := client.Get("/api/templates.get", map[string]string{
			"workspace_id": workspaceID,
			"id":           templateID,
		})
		require.NoError(t, err, "Should be able to get template")
		_ = getResp.Body.Close()

		// Test Update (should update our template)
		updateData := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           templateID,
			"name":         "Updated Integration Test Template",
			"channel":      "email",
			"category":     "transactional",
			"email": map[string]interface{}{
				"subject":          "Updated Subject",
				"compiled_preview": "<mjml><mj-body><mj-section><mj-column><mj-text>Updated</mj-text></mj-column></mj-section></mj-body></mjml>",
				"visual_editor_tree": map[string]interface{}{
					"type":       "mjml",
					"attributes": map[string]interface{}{},
					"children":   []interface{}{},
				},
			},
		}

		updateResp, err := client.Post("/api/templates.update", updateData)
		require.NoError(t, err, "Should be able to update template")
		_ = updateResp.Body.Close()

		// Test Delete (should remove our template)
		deleteData := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           templateID,
		}

		deleteResp, err := client.Post("/api/templates.delete", deleteData)
		require.NoError(t, err, "Should be able to delete template")
		_ = deleteResp.Body.Close()

		// All operations should succeed or give meaningful errors
		t.Logf("Template CRUD operations completed - Create: %d, List: %d, Get: %d, Update: %d, Delete: %d",
			createResp.StatusCode, listResp.StatusCode, getResp.StatusCode, updateResp.StatusCode, deleteResp.StatusCode)
	})
}

// TestTemplateCompileWithSubject verifies that /api/templates.compile renders
// subject and subject_preview through the Liquid engine using test_data.
// Regression for https://github.com/Notifuse/notifuse/issues/329.
func TestTemplateCompileWithSubject(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	email := "test@example.com"
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	workspaceID := createTestWorkspace(t, client, "Compile Subject Test Workspace")

	compileReq := map[string]interface{}{
		"workspace_id":    workspaceID,
		"message_id":      "preview",
		"subject":         "Hi {{ contact.first_name }}",
		"subject_preview": "Welcome {{ contact.first_name }}",
		"test_data": map[string]interface{}{
			"contact": map[string]interface{}{
				"first_name": "Pierre",
			},
		},
		"visual_editor_tree": map[string]interface{}{
			"id":         "mjml-1",
			"type":       "mjml",
			"attributes": map[string]interface{}{},
			"children": []interface{}{
				map[string]interface{}{
					"id":         "body-1",
					"type":       "mj-body",
					"attributes": map[string]interface{}{},
					"children": []interface{}{
						map[string]interface{}{
							"id":         "section-1",
							"type":       "mj-section",
							"attributes": map[string]interface{}{},
							"children": []interface{}{
								map[string]interface{}{
									"id":         "column-1",
									"type":       "mj-column",
									"attributes": map[string]interface{}{},
									"children": []interface{}{
										map[string]interface{}{
											"id":         "text-1",
											"type":       "mj-text",
											"attributes": map[string]interface{}{},
											"content":    "hello",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.CompileTemplate(compileReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "compile should return 200")

	var body struct {
		Success        bool    `json:"success"`
		Subject        *string `json:"subject"`
		SubjectPreview *string `json:"subject_preview"`
		HTML           *string `json:"html"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.True(t, body.Success, "compile should succeed")
	if assert.NotNil(t, body.Subject, "response should include rendered subject") {
		assert.Equal(t, "Hi Pierre", *body.Subject)
	}
	if assert.NotNil(t, body.SubjectPreview, "response should include rendered subject_preview") {
		assert.Equal(t, "Welcome Pierre", *body.SubjectPreview)
	}
	assert.NotNil(t, body.HTML, "response should still include compiled HTML")
}

// TestTemplateCompileInjectsWorkspaceURLs verifies the compile endpoint exposes
// workspace.website_url / workspace.base_url to Liquid (injected server-side, so
// any API consumer gets them — not just the console) and echoes the effective
// template data back so the console can display it.
func TestTemplateCompileInjectsWorkspaceURLs(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	email := "test@example.com"
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	websiteURL := "https://app.example.com"
	// base_url resolution (custom endpoint / API-endpoint fallback / trimming) is covered
	// by the unit tests. This test focuses on the website_url end-to-end path.
	workspaceID := createTestWorkspaceWithWebsite(t, client, "Workspace URL Test", websiteURL)

	// test_data intentionally omits a "workspace" key so the server injects it.
	compileReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"message_id":   "preview",
		"test_data": map[string]interface{}{
			"verify_path": "/users/verify/abc123",
		},
		"visual_editor_tree": map[string]interface{}{
			"id":         "mjml-1",
			"type":       "mjml",
			"attributes": map[string]interface{}{},
			"children": []interface{}{
				map[string]interface{}{
					"id":         "body-1",
					"type":       "mj-body",
					"attributes": map[string]interface{}{},
					"children": []interface{}{
						map[string]interface{}{
							"id":         "section-1",
							"type":       "mj-section",
							"attributes": map[string]interface{}{},
							"children": []interface{}{
								map[string]interface{}{
									"id":         "column-1",
									"type":       "mj-column",
									"attributes": map[string]interface{}{},
									"children": []interface{}{
										map[string]interface{}{
											"id":         "text-1",
											"type":       "mj-text",
											"attributes": map[string]interface{}{},
											"content":    "Verify: {{ workspace.website_url }}{{ verify_path }}",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.CompileTemplate(compileReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "compile should return 200")

	var body struct {
		Success  bool                   `json:"success"`
		HTML     *string                `json:"html"`
		TestData map[string]interface{} `json:"test_data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.True(t, body.Success, "compile should succeed")

	// The composed application link renders from the workspace website URL.
	require.NotNil(t, body.HTML, "response should include compiled HTML")
	assert.Contains(t, *body.HTML, "https://app.example.com/users/verify/abc123",
		"workspace.website_url should be composed with the relative path")

	// The effective data is echoed back with the injected workspace object.
	require.NotNil(t, body.TestData, "response should echo the effective test_data")
	ws, ok := body.TestData["workspace"].(map[string]interface{})
	require.True(t, ok, "effective test_data should contain the injected workspace object")
	assert.Equal(t, websiteURL, ws["website_url"], "website_url should match the workspace setting")
	_, hasBaseURL := ws["base_url"]
	assert.True(t, hasBaseURL, "workspace object should include base_url")
}

// TestTemplateCompileFillsMissingWorkspaceURL covers the older-template shape end-to-end:
// test_data already carries a partial workspace object (just base_url, as templates saved
// before this change did). The server must preserve the provided base_url and fill in the
// missing website_url — exercising the real JSON-decode (map[string]any) path.
func TestTemplateCompileFillsMissingWorkspaceURL(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	email := "test@example.com"
	token := performCompleteSignInFlow(t, client, email)
	client.SetToken(token)

	websiteURL := "https://app.example.com"
	workspaceID := createTestWorkspaceWithWebsite(t, client, "Partial Workspace Test", websiteURL)

	compileReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"message_id":   "preview",
		"test_data": map[string]interface{}{
			// Partial workspace object: base_url present, website_url missing.
			"workspace": map[string]interface{}{
				"base_url": "https://snapshot.example.com",
			},
		},
		"visual_editor_tree": map[string]interface{}{
			"id":         "mjml-1",
			"type":       "mjml",
			"attributes": map[string]interface{}{},
			"children": []interface{}{
				map[string]interface{}{
					"id":         "body-1",
					"type":       "mj-body",
					"attributes": map[string]interface{}{},
					"children": []interface{}{
						map[string]interface{}{
							"id":         "section-1",
							"type":       "mj-section",
							"attributes": map[string]interface{}{},
							"children": []interface{}{
								map[string]interface{}{
									"id":         "column-1",
									"type":       "mj-column",
									"attributes": map[string]interface{}{},
									"children": []interface{}{
										map[string]interface{}{
											"id":         "text-1",
											"type":       "mj-text",
											"attributes": map[string]interface{}{},
											"content":    "Visit {{ workspace.website_url }}/verify",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.CompileTemplate(compileReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "compile should return 200")

	var body struct {
		Success  bool                   `json:"success"`
		HTML     *string                `json:"html"`
		TestData map[string]interface{} `json:"test_data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.True(t, body.Success, "compile should succeed")
	require.NotNil(t, body.HTML)
	assert.Contains(t, *body.HTML, "https://app.example.com/verify",
		"missing website_url should be filled from the workspace")

	require.NotNil(t, body.TestData)
	ws, ok := body.TestData["workspace"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://snapshot.example.com", ws["base_url"], "provided base_url must be preserved")
	assert.Equal(t, websiteURL, ws["website_url"], "missing website_url must be filled")
}

// Helper functions for creating test data
// These are currently unused but kept for potential future use

// createTestTemplatePayload, createSimpleMJMLBlock, and createSimpleMJMLString are unused test helpers
// They are kept for potential future use but currently not called by any tests
// Uncomment and use them when needed:
/*
func createTestTemplatePayload() map[string]interface{} {
	return map[string]interface{}{
		"id":       fmt.Sprintf("test-template-%d", time.Now().UnixNano()),
		"name":     "Test Template",
		"channel":  "email",
		"category": "marketing",
		"email": map[string]interface{}{
			"subject":            "Test Email Subject",
			"compiled_preview":   createSimpleMJMLString(),
			"visual_editor_tree": createSimpleMJMLBlock(),
		},
		"test_data": map[string]interface{}{
			"name":    "Test User",
			"product": "Test Product",
		},
	}
}

func createSimpleMJMLBlock() map[string]interface{} {
	return map[string]interface{}{
		"type": "mjml",
		"attributes": map[string]interface{}{
			"version": "4.0.0",
		},
		"children": []interface{}{
			map[string]interface{}{
				"type":       "mj-body",
				"attributes": map[string]interface{}{},
				"children": []interface{}{
					map[string]interface{}{
						"type":       "mj-section",
						"attributes": map[string]interface{}{},
						"children": []interface{}{
							map[string]interface{}{
								"type":       "mj-column",
								"attributes": map[string]interface{}{},
								"children": []interface{}{
									map[string]interface{}{
										"type":       "mj-text",
										"attributes": map[string]interface{}{},
										"children": []interface{}{
											"Hello {{name}}! Welcome to {{product}}!",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func createSimpleMJMLString() string {
	return `<mjml version="4.0.0">
		<mj-body>
			<mj-section>
				<mj-column>
					<mj-text>
						Hello {{name}}! Welcome to {{product}}!
					</mj-text>
				</mj-column>
			</mj-section>
		</mj-body>
	</mjml>`
}
*/

// TestTemplateVersionConflict exercises the optimistic-concurrency guard end-to-end
// against real Postgres: a save based on a stale revision is rejected with 409, while
// a save based on the current revision (or with no base_version) succeeds. This is the
// one path sqlmock cannot validate — the atomic `WHERE base_version = MAX(version)`
// guard inside the CTE INSERT.
func TestTemplateVersionConflict(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	token := performCompleteSignInFlow(t, client, "test@example.com")
	client.SetToken(token)
	workspaceID := createTestWorkspace(t, client, "Version Conflict Workspace")

	templateID := fmt.Sprintf("conflict-%d", time.Now().UnixNano()) // keep <= 32 chars
	mjmlSrc := "<mjml><mj-body><mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>"
	payload := func(name string, baseVersion *int) map[string]interface{} {
		m := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           templateID,
			"name":         name,
			"channel":      "email",
			"category":     "marketing",
			"email": map[string]interface{}{
				"editor_mode":      "code",
				"mjml_source":      mjmlSrc,
				"subject":          "Conflict Subject",
				"compiled_preview": mjmlSrc,
			},
		}
		if baseVersion != nil {
			m["base_version"] = *baseVersion
		}
		return m
	}

	type updateResult struct {
		Template struct {
			Version int `json:"version"`
		} `json:"template"`
		Error         string `json:"error"`
		LatestVersion int    `json:"latest_version"`
		BaseVersion   int    `json:"base_version"`
	}
	doUpdate := func(name string, base *int) (int, updateResult) {
		resp, err := client.Post("/api/templates.update", payload(name, base))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		var r updateResult
		_ = json.NewDecoder(resp.Body).Decode(&r)
		return resp.StatusCode, r
	}

	// Create the template (version 1).
	createResp, err := client.CreateTemplate(payload("Conflict Base", nil))
	require.NoError(t, err, "Should be able to create template")
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Should be able to create template")
	_ = createResp.Body.Close()

	one, two := 1, 2

	// Save based on the current version 1 → creates version 2.
	status, r := doUpdate("Update from v1", &one)
	require.Equal(t, http.StatusOK, status, "save based on the latest revision should succeed")
	assert.Equal(t, 2, r.Template.Version)

	// Save AGAIN based on the now-stale version 1 → rejected with 409.
	status, r = doUpdate("Stale update from v1", &one)
	require.Equal(t, http.StatusConflict, status, "save based on a stale revision must be rejected")
	assert.Equal(t, 2, r.LatestVersion, "409 body should report the current latest version")
	assert.Equal(t, 1, r.BaseVersion)

	// Save based on the current version 2 → creates version 3.
	status, r = doUpdate("Update from v2", &two)
	require.Equal(t, http.StatusOK, status, "save based on the refreshed revision should succeed")
	assert.Equal(t, 3, r.Template.Version)

	// Legacy save with no base_version → last-writer-wins, creates version 4.
	status, r = doUpdate("Legacy no base_version", nil)
	require.Equal(t, http.StatusOK, status, "omitting base_version preserves last-writer-wins")
	assert.Equal(t, 4, r.Template.Version)
}

// TestTemplateConcurrentSaveRace is the core proof that the optimistic-concurrency guard
// is atomic under real concurrency (not just sequentially). It fires N simultaneous saves
// all based on the same revision against real Postgres and asserts that EXACTLY ONE wins,
// every other gets a clean 409 (never a 500 or a silent success), and the template
// advances by exactly one version — i.e. no lost update and no double-write.
func TestTemplateConcurrentSaveRace(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	token := performCompleteSignInFlow(t, client, "test@example.com")
	client.SetToken(token)
	workspaceID := createTestWorkspace(t, client, "Concurrent Save Workspace")

	templateID := fmt.Sprintf("race-%d", time.Now().UnixNano()) // keep <= 32 chars
	mjmlSrc := "<mjml><mj-body><mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>"
	payload := func(name string, baseVersion int) map[string]interface{} {
		return map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           templateID,
			"name":         name,
			"channel":      "email",
			"category":     "marketing",
			"base_version": baseVersion,
			"email": map[string]interface{}{
				"editor_mode":      "code",
				"mjml_source":      mjmlSrc,
				"subject":          "Race",
				"compiled_preview": mjmlSrc,
			},
		}
	}

	// Create version 1.
	createResp, err := client.CreateTemplate(payload("Race Base", 0))
	require.NoError(t, err, "Should be able to create template")
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	_ = createResp.Body.Close()

	// Fire N concurrent saves all based on version 1, released simultaneously to maximize
	// contention. The PRIMARY KEY (id, version) and the atomic WHERE guard must let only one win.
	const n = 8
	type result struct {
		status        int
		latestVersion int
	}
	results := make(chan result, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			resp, err := client.Post("/api/templates.update", payload(fmt.Sprintf("Racer %d", i), 1))
			if err != nil {
				results <- result{status: -1}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			var body struct {
				LatestVersion int `json:"latest_version"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			results <- result{status: resp.StatusCode, latestVersion: body.LatestVersion}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	okCount, conflictCount, otherCount := 0, 0, 0
	for r := range results {
		switch r.status {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
			assert.Equal(t, 2, r.latestVersion, "409 body should report the winning version (2)")
		default:
			otherCount++
			t.Logf("unexpected status from a concurrent save: %d", r.status)
		}
	}

	assert.Equal(t, 1, okCount, "exactly one concurrent save must win")
	assert.Equal(t, n-1, conflictCount, "every other concurrent save must get a clean 409 (not a 500 or a silent success)")
	assert.Equal(t, 0, otherCount, "no concurrent save should error or return an unexpected status")

	// The template must have advanced by exactly one version — proving no lost update and no double-write.
	getResp, err := client.Get("/api/templates.get", map[string]string{
		"workspace_id": workspaceID,
		"id":           templateID,
	})
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()
	var getBody struct {
		Template struct {
			Version int `json:"version"`
		} `json:"template"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&getBody))
	assert.Equal(t, 2, getBody.Template.Version, "exactly one new version should exist after the race")
}
