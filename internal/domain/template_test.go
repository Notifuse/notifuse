package domain

import (
	"encoding/json"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
)

// createValidMJMLBlock creates a valid MJML EmailBlock for testing
func createValidMJMLBlock() notifuse_mjml.EmailBlock {
	return &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "mjml-root",
			Type: notifuse_mjml.MJMLComponentMjml,
			Attributes: map[string]interface{}{
				"lang": "en",
			},
			Children: []interface{}{
				&notifuse_mjml.MJBodyBlock{
					BaseBlock: notifuse_mjml.BaseBlock{
						ID:   "body-1",
						Type: notifuse_mjml.MJMLComponentMjBody,
					},
				},
			},
		},
	}
}

// createInvalidMJMLBlock creates an invalid MJML EmailBlock for testing
func createInvalidMJMLBlock(blockType notifuse_mjml.MJMLComponentType) notifuse_mjml.EmailBlock {
	return &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "text-1",
			Type: blockType,
		},
	}
}

func TestTemplateCategory_Validate(t *testing.T) {
	tests := []struct {
		name     string
		category TemplateCategory
		wantErr  bool
	}{
		{
			name:     "valid marketing category",
			category: TemplateCategoryMarketing,
			wantErr:  false,
		},
		{
			name:     "valid transactional category",
			category: TemplateCategoryTransactional,
			wantErr:  false,
		},
		{
			name:     "valid welcome category",
			category: TemplateCategoryWelcome,
			wantErr:  false,
		},
		{
			name:     "valid opt_in category",
			category: TemplateCategoryOptIn,
			wantErr:  false,
		},
		{
			name:     "valid unsubscribe category",
			category: TemplateCategoryUnsubscribe,
			wantErr:  false,
		},
		{
			name:     "valid bounce category",
			category: TemplateCategoryBounce,
			wantErr:  false,
		},
		{
			name:     "valid blocklist category",
			category: TemplateCategoryBlocklist,
			wantErr:  false,
		},
		{
			name:     "valid other category",
			category: TemplateCategoryOther,
			wantErr:  false,
		},
		{
			name:     "invalid category",
			category: "invalid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.category.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTemplate_Validate(t *testing.T) {
	now := time.Now()

	createValidTemplate := func() *Template {
		return &Template{
			ID:      "test123",
			Name:    "Test Template",
			Version: 1,
			Channel: "email",
			Email: &EmailTemplate{
				SenderID:         "test123",
				Subject:          "Test Subject",
				CompiledPreview:  "<html>Test content</html>",
				VisualEditorTree: createValidMJMLBlock(),
			},
			Category:  string(TemplateCategoryMarketing),
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	tests := []struct {
		name     string
		template *Template
		wantErr  bool
	}{
		{
			name:     "valid template",
			template: createValidTemplate(),
			wantErr:  false,
		},
		{
			name: "invalid template with version 0",
			template: func() *Template {
				t := createValidTemplate()
				t.Version = 0
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - missing ID",
			template: func() *Template {
				t := createValidTemplate()
				t.ID = ""
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - missing name",
			template: func() *Template {
				t := createValidTemplate()
				t.Name = ""
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - missing channel",
			template: func() *Template {
				t := createValidTemplate()
				t.Channel = ""
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - channel too long",
			template: func() *Template {
				t := createValidTemplate()
				t.Channel = "this_channel_name_is_too_long_for_validation"
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - missing category",
			template: func() *Template {
				t := createValidTemplate()
				t.Category = ""
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - category too long",
			template: func() *Template {
				t := createValidTemplate()
				t.Category = "this_category_name_is_too_long_for_validation"
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - zero version",
			template: func() *Template {
				t := createValidTemplate()
				t.Version = 0
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - negative version",
			template: func() *Template {
				t := createValidTemplate()
				t.Version = -1
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - missing email",
			template: func() *Template {
				t := createValidTemplate()
				t.Email = nil
				return t
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.template.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTemplateReference_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ref     *TemplateReference
		wantErr bool
	}{
		{
			name: "valid reference",
			ref: &TemplateReference{
				ID:      "test123",
				Version: 1,
			},
			wantErr: false,
		},
		{
			name: "valid reference with version 0",
			ref: &TemplateReference{
				ID:      "test123",
				Version: 0,
			},
			wantErr: false,
		},
		{
			name: "invalid reference - missing ID",
			ref: &TemplateReference{
				ID:      "",
				Version: 1,
			},
			wantErr: true,
		},
		{
			name: "invalid reference - negative version",
			ref: &TemplateReference{
				ID:      "test123",
				Version: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTemplateReference_Scan_Value(t *testing.T) {
	ref := &TemplateReference{
		ID:      "test123",
		Version: 1,
	}

	// Test Value() method
	value, err := ref.Value()
	assert.NoError(t, err)
	assert.NotNil(t, value)

	// Test Scan() method with []byte
	bytes, err := json.Marshal(ref)
	assert.NoError(t, err)

	newRef := &TemplateReference{}
	err = newRef.Scan(bytes)
	assert.NoError(t, err)
	assert.Equal(t, ref.ID, newRef.ID)
	assert.Equal(t, ref.Version, newRef.Version)

	// Test Scan() method with string
	err = newRef.Scan(string(bytes))
	assert.NoError(t, err)
	assert.Equal(t, ref.ID, newRef.ID)
	assert.Equal(t, ref.Version, newRef.Version)

	// Test Scan() method with nil
	err = newRef.Scan(nil)
	assert.NoError(t, err)
}

func TestEmailTemplate_Validate(t *testing.T) {
	tests := []struct {
		name     string
		template *EmailTemplate
		testData MapOfAny
		wantErr  bool
	}{
		{
			name: "valid template",
			template: &EmailTemplate{
				SenderID:         "test123",
				Subject:          "Test Subject",
				CompiledPreview:  "<html>Test content</html>",
				VisualEditorTree: createValidMJMLBlock(),
			},
			testData: nil,
			wantErr:  false,
		},
		{
			name: "invalid email template - missing subject",
			template: func() *EmailTemplate {
				e := &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				}
				e.Subject = ""
				return e
			}(),
			testData: nil,
			wantErr:  true,
		},
		{
			name: "invalid email template - missing compiled_preview but valid tree",
			template: func() *EmailTemplate {
				e := &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "",
					VisualEditorTree: createValidMJMLBlock(),
				}
				return e
			}(),
			testData: nil,
			wantErr:  false,
		},
		{
			name: "valid email template - missing compiled_preview and missing root data",
			template: func() *EmailTemplate {
				e := &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "",
					VisualEditorTree: createInvalidMJMLBlock(notifuse_mjml.MJMLComponentMjml),
				}
				return e
			}(),
			testData: nil,
			wantErr:  false,
		},
		{
			name: "valid email template - missing compiled_preview and invalid root data type",
			template: func() *EmailTemplate {
				e := &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "",
					VisualEditorTree: createInvalidMJMLBlock(notifuse_mjml.MJMLComponentMjml),
				}
				return e
			}(),
			testData: nil,
			wantErr:  false,
		},
		{
			name: "valid email template - missing compiled_preview and missing styles in root data",
			template: func() *EmailTemplate {
				e := &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "",
					VisualEditorTree: createInvalidMJMLBlock(notifuse_mjml.MJMLComponentMjml),
				}
				return e
			}(),
			testData: nil,
			wantErr:  false,
		},
		{
			name: "invalid email template - invalid visual_editor_tree kind",
			template: func() *EmailTemplate {
				e := &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createInvalidMJMLBlock(notifuse_mjml.MJMLComponentMjText),
				}
				return e
			}(),
			testData: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.template.Validate(tt.testData)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.name == "invalid email template - missing compiled_preview but valid tree" {
					assert.NotEmpty(t, tt.template.CompiledPreview, "CompiledPreview should be populated after validation")
				}
			}
		})
	}
}

func TestEmailTemplate_Scan_Value(t *testing.T) {
	email := &EmailTemplate{
		SenderID:         "test123",
		Subject:          "Test Subject",
		CompiledPreview:  "<html>Test content</html>",
		VisualEditorTree: createValidMJMLBlock(),
	}

	// Test Value() method
	value, err := email.Value()
	assert.NoError(t, err)
	assert.NotNil(t, value)

	// For JSON serialization of interfaces, we need custom marshalling
	// For now, test the basic structure without full JSON roundtrip
	// since the interface can't be unmarshalled directly

	// Test basic validation instead
	err = email.Validate(nil)
	assert.NoError(t, err)

	// Test Scan() method with nil
	newEmail := &EmailTemplate{}
	err = newEmail.Scan(nil)
	assert.NoError(t, err)
}

func TestCreateTemplateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *CreateTemplateRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: &CreateTemplateRequest{
				WorkspaceID: "",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "ID too long",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "this_id_is_way_too_long_for_the_validation_to_pass_properly",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing name",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing channel",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing category",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: "",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email:       nil,
				Category:    string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "invalid email template",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					// no subject
					Subject:          "",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, template)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, template.ID)
				assert.Equal(t, tt.request.Name, template.Name)
				assert.Equal(t, int64(1), template.Version)
				assert.Equal(t, tt.request.Channel, template.Channel)
				assert.Equal(t, tt.request.Email, template.Email)
				assert.Equal(t, tt.request.Category, template.Category)
			}
		})
	}
}

func TestGetTemplatesRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name        string
		queryParams url.Values
		wantErr     bool
	}{
		{
			name: "valid request",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			wantErr: false,
		},
		{
			name:        "missing workspace_id",
			queryParams: url.Values{},
			wantErr:     true,
		},
		{
			name: "workspace_id too long",
			queryParams: url.Values{
				"workspace_id": []string{"workspace_id_that_is_way_too_long_for_validation"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetTemplatesRequest{}
			err := req.FromURLParams(tt.queryParams)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.queryParams.Get("workspace_id"), req.WorkspaceID)
			}
		})
	}
}

func TestGetTemplateRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name        string
		queryParams url.Values
		wantErr     bool
	}{
		{
			name: "valid request with ID only",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"template123"},
			},
			wantErr: false,
		},
		{
			name: "valid request with ID and version",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"template123"},
				"version":      []string{"2"},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			queryParams: url.Values{
				"id": []string{"template123"},
			},
			wantErr: true,
		},
		{
			name: "missing id",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			wantErr: true,
		},
		{
			name: "id too long",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"template_id_that_is_way_too_long_for_validation_to_pass_properly"},
			},
			wantErr: true,
		},
		{
			name: "invalid version format",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"template123"},
				"version":      []string{"not-a-number"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetTemplateRequest{}
			err := req.FromURLParams(tt.queryParams)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.queryParams.Get("workspace_id"), req.WorkspaceID)
				assert.Equal(t, tt.queryParams.Get("id"), req.ID)
				if versionStr := tt.queryParams.Get("version"); versionStr != "" {
					version, _ := strconv.ParseInt(versionStr, 10, 64)
					assert.Equal(t, version, req.Version)
				}
			}
		})
	}
}

func TestUpdateTemplateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *UpdateTemplateRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: &UpdateTemplateRequest{
				WorkspaceID: "",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "ID too long",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "this_id_is_way_too_long_for_the_validation_to_pass_properly",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing name",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing channel",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "missing category",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Category: "",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email:       nil,
				Category:    string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "invalid email template",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
				Name:        "Test Template",
				Channel:     "email",
				Email:       &EmailTemplate{},
				Category:    string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, template)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, template.ID)
				assert.Equal(t, tt.request.Name, template.Name)
				assert.Equal(t, tt.request.Channel, template.Channel)
				assert.Equal(t, tt.request.Email, template.Email)
				assert.Equal(t, tt.request.Category, template.Category)
			}
		})
	}
}

func TestDeleteTemplateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *DeleteTemplateRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &DeleteTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "template123",
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: &DeleteTemplateRequest{
				WorkspaceID: "",
				ID:          "template123",
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: &DeleteTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "",
			},
			wantErr: true,
		},
		{
			name: "ID too long",
			request: &DeleteTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "this_id_is_way_too_long_for_the_validation_to_pass_properly",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID, id, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, id)
			}
		})
	}
}

func TestErrTemplateNotFound_Error(t *testing.T) {
	err := &ErrTemplateNotFound{Message: "template not found"}
	assert.Equal(t, "template not found", err.Error())
}

func TestBuildTemplateData(t *testing.T) {
	t.Run("with complete data", func(t *testing.T) {
		// Setup test data
		workspaceID := "ws-123"
		apiEndpoint := "https://api.example.com"
		messageID := "msg-456"
		workspaceSecretKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

		firstName := &NullableString{String: "John", IsNull: false}
		lastName := &NullableString{String: "Doe", IsNull: false}

		contact := &Contact{
			Email:     "test@example.com",
			FirstName: firstName,
			LastName:  lastName,
			// Don't use Properties field as it doesn't exist in Contact struct
		}

		contactWithList := ContactWithList{
			Contact:  contact,
			ListID:   "list-789",
			ListName: "Newsletter",
		}

		broadcast := &Broadcast{
			ID:   "broadcast-001",
			Name: "Test Broadcast",
		}

		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:    apiEndpoint,
			UTMSource:   "newsletter",
			UTMMedium:   "email",
			UTMCampaign: "welcome",
			UTMTerm:     "new-users",
			UTMContent:  "button-1",
		}

		// Call the function with the workspace secret key using the new struct
		req := TemplateDataRequest{
			WorkspaceID:        workspaceID,
			WorkspaceSecretKey: workspaceSecretKey,
			ContactWithList:    contactWithList,
			MessageID:          messageID,
			TrackingSettings:   trackingSettings,
			Broadcast:          broadcast,
		}
		data, err := BuildTemplateData(req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, data)

		// Check contact data
		contactData, ok := data["contact"].(MapOfAny)
		assert.True(t, ok)
		assert.Equal(t, "test@example.com", contactData["email"])
		assert.Equal(t, "John", contactData["first_name"])
		assert.Equal(t, "Doe", contactData["last_name"])

		// Check broadcast data
		broadcastData, ok := data["broadcast"].(MapOfAny)
		assert.True(t, ok)
		assert.Equal(t, "broadcast-001", broadcastData["id"])
		assert.Equal(t, "Test Broadcast", broadcastData["name"])

		// Check UTM parameters
		assert.Equal(t, "newsletter", data["utm_source"])
		assert.Equal(t, "email", data["utm_medium"])
		assert.Equal(t, "welcome", data["utm_campaign"])
		assert.Equal(t, "new-users", data["utm_term"])
		assert.Equal(t, "button-1", data["utm_content"])

		// Check list data
		listData, ok := data["list"].(MapOfAny)
		assert.True(t, ok)
		assert.Equal(t, "list-789", listData["id"])
		assert.Equal(t, "Newsletter", listData["name"])

		// Check unsubscribe URL
		unsubscribeURL, ok := data["unsubscribe_url"].(string)
		assert.True(t, ok)
		assert.Contains(t, unsubscribeURL, "https://api.example.com/notification-center?action=unsubscribe")
		assert.Contains(t, unsubscribeURL, "email=test%40example.com")
		assert.Contains(t, unsubscribeURL, "lid=list-789")
		assert.Contains(t, unsubscribeURL, "lname=Newsletter")
		assert.Contains(t, unsubscribeURL, "wid=ws-123")
		assert.Contains(t, unsubscribeURL, "mid=msg-456")

		// Check tracking data
		assert.Equal(t, messageID, data["message_id"])

		// Check tracking pixel URL
		trackingPixelURL, ok := data["tracking_opens_url"].(string)
		assert.True(t, ok)
		assert.Contains(t, trackingPixelURL, "https://api.example.com/opens")
		assert.Contains(t, trackingPixelURL, "mid=msg-456")
		assert.Contains(t, trackingPixelURL, "wid=ws-123")

		// Check confirm subscription URL
		confirmURL, ok := data["confirm_subscription_url"].(string)
		assert.True(t, ok)
		assert.Contains(t, confirmURL, "https://api.example.com/notification-center?action=confirm")
		assert.Contains(t, confirmURL, "email=test%40example.com")
		assert.Contains(t, confirmURL, "lid=list-789")
		assert.Contains(t, confirmURL, "lname=Newsletter")
		assert.Contains(t, confirmURL, "wid=ws-123")
		assert.Contains(t, confirmURL, "mid=msg-456")

		// Check notification center URL
		notificationCenterURL, ok := data["notification_center_url"].(string)
		assert.True(t, ok)
		assert.Contains(t, notificationCenterURL, "https://api.example.com/notification-center")
		assert.Contains(t, notificationCenterURL, "email=test%40example.com")
		assert.Contains(t, notificationCenterURL, "wid=ws-123")
		assert.NotContains(t, notificationCenterURL, "action=") // Should not contain action parameter
		assert.NotContains(t, notificationCenterURL, "lid=")    // Should not contain list ID
	})

	t.Run("with minimal data", func(t *testing.T) {
		// Setup minimal test data
		workspaceID := "ws-123"
		messageID := "msg-456"
		workspaceSecretKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

		contactWithList := ContactWithList{
			Contact: nil,
		}
		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:    "https://api.example.com",
			UTMSource:   "newsletter",
			UTMMedium:   "email",
			UTMCampaign: "welcome",
			UTMTerm:     "new-users",
			UTMContent:  "button-1",
		}
		// Call the function with the workspace secret key using the new struct
		req := TemplateDataRequest{
			WorkspaceID:        workspaceID,
			WorkspaceSecretKey: workspaceSecretKey,
			ContactWithList:    contactWithList,
			MessageID:          messageID,
			TrackingSettings:   trackingSettings,
			Broadcast:          nil,
		}
		data, err := BuildTemplateData(req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, data)

		// Check contact data should be empty
		contactData, ok := data["contact"].(MapOfAny)
		assert.True(t, ok)
		assert.Empty(t, contactData)

		// Check message ID still exists
		assert.Equal(t, messageID, data["message_id"])

		// Check tracking opens URL still exists even without API endpoint
		trackingPixelURL, ok := data["tracking_opens_url"].(string)
		assert.True(t, ok)
		assert.Contains(t, trackingPixelURL, "/opens")
		assert.Contains(t, trackingPixelURL, "mid=msg-456")
		assert.Contains(t, trackingPixelURL, "wid=ws-123")

		// No unsubscribe URL should be present
		_, exists := data["unsubscribe_url"]
		assert.False(t, exists)

		// No notification center URL should be present (no contact)
		_, exists = data["notification_center_url"]
		assert.False(t, exists)
	})

	t.Run("with contact but no list (transactional email)", func(t *testing.T) {
		// Setup test data with contact but no list
		workspaceID := "ws-123"
		messageID := "msg-456"
		workspaceSecretKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

		contactWithList := ContactWithList{
			Contact: &Contact{
				Email:     "test@example.com",
				FirstName: &NullableString{String: "John", IsNull: false},
				LastName:  &NullableString{String: "Doe", IsNull: false},
			},
			ListID:   "", // No list
			ListName: "",
		}
		trackingSettings := notifuse_mjml.TrackingSettings{
			Endpoint:    "https://api.example.com",
			UTMSource:   "app",
			UTMMedium:   "email",
			UTMCampaign: "transactional",
		}

		req := TemplateDataRequest{
			WorkspaceID:        workspaceID,
			WorkspaceSecretKey: workspaceSecretKey,
			ContactWithList:    contactWithList,
			MessageID:          messageID,
			TrackingSettings:   trackingSettings,
			Broadcast:          nil,
		}
		data, err := BuildTemplateData(req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, data)

		// Check contact data exists
		contactData, ok := data["contact"].(MapOfAny)
		assert.True(t, ok)
		assert.Equal(t, "test@example.com", contactData["email"])

		// Check notification center URL is present even without list
		notificationCenterURL, ok := data["notification_center_url"].(string)
		assert.True(t, ok)
		assert.Contains(t, notificationCenterURL, "https://api.example.com/notification-center")
		assert.Contains(t, notificationCenterURL, "email=test%40example.com")
		assert.Contains(t, notificationCenterURL, "wid=ws-123")
		assert.NotContains(t, notificationCenterURL, "lid=") // Should not contain list ID

		// No list-specific URLs should be present
		_, exists := data["unsubscribe_url"]
		assert.False(t, exists)
		_, exists = data["confirm_subscription_url"]
		assert.False(t, exists)
	})

	// We'll skip other test cases since they would require mocking
}

// TestGenerateEmailRedirectionEndpoint tests the generation of the URL for tracking email redirections
func TestGenerateEmailRedirectionEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		workspaceID    string
		messageID      string
		apiEndpoint    string
		destinationURL string
		expected       string
	}{
		{
			name:           "with all parameters",
			workspaceID:    "ws-123",
			messageID:      "msg-456",
			apiEndpoint:    "https://api.example.com",
			destinationURL: "https://example.com",
			expected:       "https://api.example.com/visit?mid=msg-456&wid=ws-123&url=https%3A%2F%2Fexample.com",
		},
		{
			name:           "with empty api endpoint",
			workspaceID:    "ws-123",
			messageID:      "msg-456",
			apiEndpoint:    "",
			destinationURL: "https://example.com",
			expected:       "/visit?mid=msg-456&wid=ws-123&url=https%3A%2F%2Fexample.com",
		},
		{
			name:           "with special characters that need encoding",
			workspaceID:    "ws/123&test=1",
			messageID:      "msg=456?test=1",
			apiEndpoint:    "https://api.example.com",
			destinationURL: "https://example.com/page?param=value&other=test",
			expected:       "https://api.example.com/visit?mid=msg%3D456%3Ftest%3D1&wid=ws%2F123%26test%3D1&url=https%3A%2F%2Fexample.com%2Fpage%3Fparam%3Dvalue%26other%3Dtest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := notifuse_mjml.GenerateEmailRedirectionEndpoint(tt.workspaceID, tt.messageID, tt.apiEndpoint, tt.destinationURL)
			assert.Equal(t, tt.expected, url)
		})
	}
}

func TestTemplateDataRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request TemplateDataRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: TemplateDataRequest{
				WorkspaceID:        "ws-123",
				WorkspaceSecretKey: "secret-key",
				MessageID:          "msg-456",
				ContactWithList:    ContactWithList{},
				TrackingSettings:   notifuse_mjml.TrackingSettings{},
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: TemplateDataRequest{
				WorkspaceSecretKey: "secret-key",
				MessageID:          "msg-456",
				ContactWithList:    ContactWithList{},
				TrackingSettings:   notifuse_mjml.TrackingSettings{},
			},
			wantErr: true,
		},
		{
			name: "missing workspace secret key",
			request: TemplateDataRequest{
				WorkspaceID:      "ws-123",
				MessageID:        "msg-456",
				ContactWithList:  ContactWithList{},
				TrackingSettings: notifuse_mjml.TrackingSettings{},
			},
			wantErr: true,
		},
		{
			name: "missing message ID",
			request: TemplateDataRequest{
				WorkspaceID:        "ws-123",
				WorkspaceSecretKey: "secret-key",
				ContactWithList:    ContactWithList{},
				TrackingSettings:   notifuse_mjml.TrackingSettings{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEmailTemplate_UnmarshalJSON_Minimal_ExistingFile(t *testing.T) {
	// Minimal JSON with a valid empty mjml root
	data := []byte(`{"subject":"Hello","compiled_preview":"<mjml></mjml>","visual_editor_tree":{"id":"root","type":"mjml","children":[]}}`)
	var et EmailTemplate
	if err := et.UnmarshalJSON(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
