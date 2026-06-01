package integration

import (
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
		"workspace_id": workspaceID,
		"message_id":   "preview",
		"subject":      "Hi {{ contact.first_name }}",
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
