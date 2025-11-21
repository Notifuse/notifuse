package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/templates"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateRoundtripSerialization tests that templates can be created and retrieved
// with complex visual_editor_tree structures without losing data
func TestTemplateRoundtripSerialization(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	// Create user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err, "Failed to create user")
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err, "Failed to create workspace")

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err, "Failed to add user to workspace")

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err, "Failed to login")
	client.SetWorkspaceID(workspace.ID)

	t.Run("Roundtrip with all component types", func(t *testing.T) {
		// Create a template with every supported MJML component type
		allComponentsTree := createAllComponentsTree(t)

		// Marshal the tree to JSON for the request
		treeJSON, err := json.Marshal(allComponentsTree)
		require.NoError(t, err, "Failed to marshal tree")

		var treeMap map[string]interface{}
		err = json.Unmarshal(treeJSON, &treeMap)
		require.NoError(t, err, "Failed to unmarshal to map")

		templateID := fmt.Sprintf("rt-all-%d", time.Now().Unix())
		template := map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           templateID,
			"name":         "All Components Test",
			"channel":      "email",
			"category":     "transactional",
			"email": map[string]interface{}{
				"subject":            "Test Subject",
				"compiled_preview":   "<mjml><mj-body></mj-body></mjml>",
				"visual_editor_tree": treeMap,
			},
		}

		// Create template via API
		createResp, err := client.CreateTemplate(template)
		require.NoError(t, err, "Failed to create template")
		require.Equal(t, http.StatusCreated, createResp.StatusCode, "Template creation should return 201")
		createResp.Body.Close()

		// Retrieve template via API
		getResp, err := client.Get("/api/templates.get", map[string]string{
			"workspace_id": workspace.ID,
			"id":           templateID,
		})
		require.NoError(t, err, "Failed to get template")
		defer getResp.Body.Close()

		// Decode response
		var result struct {
			Template domain.Template `json:"template"`
		}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err, "Failed to decode template response")

		// Verify structure
		assert.NotNil(t, result.Template.Email, "Template should have email field")
		assert.NotNil(t, result.Template.Email.VisualEditorTree, "Template should have visual_editor_tree")
		assert.Equal(t, notifuse_mjml.MJMLComponentMjml, result.Template.Email.VisualEditorTree.GetType())
	})

	t.Run("Roundtrip with nested structure", func(t *testing.T) {
		// Create a deeply nested template (similar to Supabase templates)
		nestedTree := createNestedStructure(t)

		// Marshal the tree to JSON for the request
		treeJSON, err := json.Marshal(nestedTree)
		require.NoError(t, err, "Failed to marshal tree")

		var treeMap map[string]interface{}
		err = json.Unmarshal(treeJSON, &treeMap)
		require.NoError(t, err, "Failed to unmarshal to map")

		templateID := fmt.Sprintf("rt-nest-%d", time.Now().Unix())
		template := map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           templateID,
			"name":         "Nested Structure Test",
			"channel":      "email",
			"category":     "transactional",
			"email": map[string]interface{}{
				"subject":            "Test Subject",
				"compiled_preview":   "<mjml><mj-body></mj-body></mjml>",
				"visual_editor_tree": treeMap,
			},
		}

		// Create template via API
		createResp, err := client.CreateTemplate(template)
		require.NoError(t, err, "Failed to create template")
		require.Equal(t, http.StatusCreated, createResp.StatusCode, "Template creation should return 201")
		createResp.Body.Close()

		// Retrieve template via API
		getResp, err := client.Get("/api/templates.get", map[string]string{
			"workspace_id": workspace.ID,
			"id":           templateID,
		})
		require.NoError(t, err, "Failed to get template")
		defer getResp.Body.Close()

		// Decode response
		var result struct {
			Template domain.Template `json:"template"`
		}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err, "Failed to decode template response")

		// Verify structure
		assert.NotNil(t, result.Template.Email, "Template should have email field")
		assert.NotNil(t, result.Template.Email.VisualEditorTree, "Template should have visual_editor_tree")
	})

	t.Run("List templates with complex structures", func(t *testing.T) {
		// Retrieve all templates via API
		listResp, err := client.Get("/api/templates.list", map[string]string{
			"workspace_id": workspace.ID,
		})
		require.NoError(t, err, "Failed to list templates")
		defer listResp.Body.Close()

		// Decode response
		var result struct {
			Templates []domain.Template `json:"templates"`
		}
		err = json.NewDecoder(listResp.Body).Decode(&result)
		require.NoError(t, err, "Failed to decode templates list response")

		// Should be able to unmarshal all templates without error
		assert.GreaterOrEqual(t, len(result.Templates), 2, "Should have at least 2 templates")

		for _, tmpl := range result.Templates {
			assert.NotNil(t, tmpl.Email, "Template %s should have email field", tmpl.ID)
			assert.NotNil(t, tmpl.Email.VisualEditorTree, "Template %s should have visual_editor_tree", tmpl.ID)
		}
	})

	t.Run("Production Supabase templates can be round-tripped", func(t *testing.T) {
		// Test all production Supabase templates
		supabaseTemplates := templates.AllSupabaseTemplates()

		for name, createFunc := range supabaseTemplates {
			t.Run(name, func(t *testing.T) {
				// Create the template structure
				visualTree, err := createFunc()
				require.NoError(t, err, "Failed to create %s template structure", name)

				// Marshal to JSON for API request
				treeJSON, err := json.Marshal(visualTree)
				require.NoError(t, err, "Failed to marshal tree")

				var treeMap map[string]interface{}
				err = json.Unmarshal(treeJSON, &treeMap)
				require.NoError(t, err, "Failed to unmarshal to map")

				templateID := fmt.Sprintf("sb-%s", name)
				template := map[string]interface{}{
					"workspace_id": workspace.ID,
					"id":           templateID,
					"name":         fmt.Sprintf("Supabase %s Test", name),
					"channel":      "email",
					"category":     "transactional",
					"email": map[string]interface{}{
						"subject":            "Test Subject",
						"compiled_preview":   "<mjml><mj-body></mj-body></mjml>",
						"visual_editor_tree": treeMap,
					},
				}

				// Create template via API
				createResp, err := client.CreateTemplate(template)
				require.NoError(t, err, "Failed to create %s template", name)
				require.Equal(t, http.StatusCreated, createResp.StatusCode,
					"Template creation should return 201 for %s", name)
				createResp.Body.Close()

				// Retrieve template via API
				getResp, err := client.Get("/api/templates.get", map[string]string{
					"workspace_id": workspace.ID,
					"id":           templateID,
				})
				require.NoError(t, err, "Failed to get %s template", name)
				defer getResp.Body.Close()

				// Decode response
				var result struct {
					Template domain.Template `json:"template"`
				}
				err = json.NewDecoder(getResp.Body).Decode(&result)
				require.NoError(t, err, "Failed to decode %s template response", name)

				// Verify structure
				assert.NotNil(t, result.Template.Email, "Template %s should have email field", name)
				assert.NotNil(t, result.Template.Email.VisualEditorTree,
					"Template %s should have visual_editor_tree", name)
				assert.Equal(t, notifuse_mjml.MJMLComponentMjml,
					result.Template.Email.VisualEditorTree.GetType(),
					"Template %s root should be mjml type", name)
			})
		}
	})
}

// createAllComponentsTree creates a tree with all supported MJML component types
func createAllComponentsTree(t *testing.T) notifuse_mjml.EmailBlock {
	jsonTemplate := `{
		"id": "mjml-root",
		"type": "mjml",
		"attributes": {},
		"children": [
			{
				"id": "head-1",
				"type": "mj-head",
				"attributes": {},
				"children": [
					{
						"id": "title-1",
						"type": "mj-title",
						"content": "Test Title",
						"attributes": {}
					},
					{
						"id": "preview-1",
						"type": "mj-preview",
						"content": "Test Preview",
						"attributes": {}
					},
					{
						"id": "style-1",
						"type": "mj-style",
						"content": ".custom { color: red; }",
						"attributes": {"inline": "inline"}
					},
					{
						"id": "font-1",
						"type": "mj-font",
						"attributes": {
							"name": "Roboto",
							"href": "https://fonts.googleapis.com/css?family=Roboto"
						}
					},
					{
						"id": "breakpoint-1",
						"type": "mj-breakpoint",
						"attributes": {"width": "480px"}
					},
					{
						"id": "attributes-1",
						"type": "mj-attributes",
						"attributes": {}
					},
					{
						"id": "html-attributes-1",
						"type": "mj-html-attributes",
						"attributes": {}
					},
					{
						"id": "raw-head-1",
						"type": "mj-raw",
						"content": "<meta name=\"test\" content=\"value\">",
						"attributes": {}
					}
				]
			},
			{
				"id": "body-1",
				"type": "mj-body",
				"attributes": {"backgroundColor": "#f0f0f0"},
				"children": [
					{
						"id": "wrapper-1",
						"type": "mj-wrapper",
						"attributes": {"padding": "20px"},
						"children": [
							{
								"id": "section-1",
								"type": "mj-section",
								"attributes": {"backgroundColor": "#ffffff"},
								"children": [
									{
										"id": "group-1",
										"type": "mj-group",
										"attributes": {},
										"children": [
											{
												"id": "column-1",
												"type": "mj-column",
												"attributes": {"width": "50%"},
												"children": [
													{
														"id": "image-1",
														"type": "mj-image",
														"attributes": {
															"src": "https://example.com/logo.png",
															"width": "100px"
														}
													},
													{
														"id": "text-1",
														"type": "mj-text",
														"content": "<p>Hello World</p>",
														"attributes": {"fontSize": "16px"}
													},
													{
														"id": "button-1",
														"type": "mj-button",
														"content": "Click Me",
														"attributes": {
															"backgroundColor": "#007bff",
															"href": "https://example.com"
														}
													},
													{
														"id": "divider-1",
														"type": "mj-divider",
														"attributes": {"borderColor": "#cccccc"}
													},
													{
														"id": "spacer-1",
														"type": "mj-spacer",
														"attributes": {"height": "20px"}
													}
												]
											},
											{
												"id": "column-2",
												"type": "mj-column",
												"attributes": {"width": "50%"},
												"children": [
													{
														"id": "social-1",
														"type": "mj-social",
														"attributes": {"mode": "horizontal"},
														"children": [
															{
																"id": "social-element-1",
																"type": "mj-social-element",
																"content": "Facebook",
																"attributes": {
																	"name": "facebook",
																	"href": "https://facebook.com"
																}
															},
															{
																"id": "social-element-2",
																"type": "mj-social-element",
																"content": "Twitter",
																"attributes": {
																	"name": "twitter",
																	"href": "https://twitter.com"
																}
															}
														]
													}
												]
											}
										]
									}
								]
							}
						]
					}
				]
			}
		]
	}`

	block, err := notifuse_mjml.UnmarshalEmailBlock([]byte(jsonTemplate))
	require.NoError(t, err, "Failed to unmarshal all components tree")
	return block
}

// createNestedStructure creates a deeply nested structure similar to Supabase templates
func createNestedStructure(t *testing.T) notifuse_mjml.EmailBlock {
	jsonTemplate := `{
		"id": "mjml-1",
		"type": "mjml",
		"attributes": {},
		"children": [
			{
				"id": "head-1",
				"type": "mj-head",
				"attributes": {},
				"children": []
			},
			{
				"id": "body-1",
				"type": "mj-body",
				"attributes": {
					"width": "600px",
					"backgroundColor": "#ffffff"
				},
				"children": [
					{
						"id": "wrapper-1",
						"type": "mj-wrapper",
						"attributes": {
							"paddingTop": "20px",
							"paddingRight": "20px",
							"paddingBottom": "20px",
							"paddingLeft": "20px"
						},
						"children": [
							{
								"id": "section-1",
								"type": "mj-section",
								"attributes": {
									"backgroundColor": "transparent",
									"paddingTop": "20px",
									"paddingRight": "0px",
									"paddingBottom": "20px",
									"paddingLeft": "0px",
									"textAlign": "center"
								},
								"children": [
									{
										"id": "column-1",
										"type": "mj-column",
										"attributes": {
											"width": "100%"
										},
										"children": [
											{
												"id": "image-1",
												"type": "mj-image",
												"attributes": {
													"align": "center",
													"src": "https://storage.googleapis.com/readonlydemo/logo-large.png",
													"width": "120px",
													"paddingTop": "10px",
													"paddingRight": "25px",
													"paddingBottom": "10px",
													"paddingLeft": "25px"
												}
											},
											{
												"id": "text-1",
												"type": "mj-text",
												"attributes": {
													"align": "center",
													"color": "#333333",
													"fontFamily": "Arial, sans-serif",
													"fontSize": "24px",
													"fontWeight": "bold",
													"lineHeight": "1.6",
													"paddingTop": "30px",
													"paddingRight": "25px",
													"paddingBottom": "30px",
													"paddingLeft": "25px"
												},
												"content": "<p>Test Heading</p>"
											},
											{
												"id": "button-1",
												"type": "mj-button",
												"attributes": {
													"align": "center",
													"backgroundColor": "#5850ec",
													"borderRadius": "4px",
													"color": "#ffffff",
													"fontFamily": "Arial, sans-serif",
													"fontSize": "16px",
													"fontWeight": "bold",
													"href": "https://example.com",
													"innerPadding": "12px 24px",
													"paddingTop": "15px",
													"paddingRight": "25px",
													"paddingBottom": "15px",
													"paddingLeft": "25px"
												},
												"content": "Click Here"
											}
										]
									}
								]
							}
						]
					}
				]
			}
		]
	}`

	block, err := notifuse_mjml.UnmarshalEmailBlock([]byte(jsonTemplate))
	require.NoError(t, err, "Failed to unmarshal nested structure")
	return block
}
