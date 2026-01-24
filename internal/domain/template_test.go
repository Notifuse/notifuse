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

// createValidMJMLBlock creates a valid MJML EmailBlock for testing EmailTemplate
func createValidMJMLBlock() notifuse_mjml.EmailBlock {
	bodyBlock := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("body-1", notifuse_mjml.MJMLComponentMjBody),
	}
	bodyBlock.Children = []notifuse_mjml.EmailBlock{}

	mjmlBlock := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml),
	}
	mjmlBlock.Attributes["lang"] = "en"
	mjmlBlock.Children = []notifuse_mjml.EmailBlock{bodyBlock}

	return mjmlBlock
}

// createInvalidMJMLBlock creates an invalid MJML EmailBlock for testing EmailTemplate
func createInvalidMJMLBlock(blockType notifuse_mjml.MJMLComponentType) notifuse_mjml.EmailBlock {
	return &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("text-1", blockType),
	}
}

func TestValidateTemplateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid alphanumeric",
			id:      "template123",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			id:      "my_template_123",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			id:      "my-template-123",
			wantErr: false,
		},
		{
			name:    "valid mixed underscores and hyphens",
			id:      "my_template-123",
			wantErr: false,
		},
		{
			name:    "empty id",
			id:      "",
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name:    "too long",
			id:      "this_is_a_very_long_template_id_that_exceeds_32_characters",
			wantErr: true,
			errMsg:  "id length must be between 1 and 32",
		},
		{
			name:    "invalid characters - space",
			id:      "my template",
			wantErr: true,
			errMsg:  "id must contain only letters, numbers, underscores, and hyphens",
		},
		{
			name:    "invalid characters - special chars",
			id:      "my@template",
			wantErr: true,
			errMsg:  "id must contain only letters, numbers, underscores, and hyphens",
		},
		{
			name:    "invalid characters - dots",
			id:      "my.template",
			wantErr: true,
			errMsg:  "id must contain only letters, numbers, underscores, and hyphens",
		},
		{
			name:    "uppercase letters allowed",
			id:      "MyTemplate_123",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Equal(t, tt.errMsg, err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
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
			name:     "valid blog category",
			category: TemplateCategoryBlog,
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
		{
			name: "valid web template",
			template: &Template{
				ID:      "test-web",
				Name:    "Test Web Template",
				Version: 1,
				Channel: "web",
				Web: &WebTemplate{
					Content:   MapOfAny{"type": "doc", "content": []interface{}{}},
					HTML:      "<div>Test content</div>",
					PlainText: "Test content",
				},
				Category:  string(TemplateCategoryBlog),
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "invalid web template - missing web field",
			template: &Template{
				ID:        "test-web",
				Name:      "Test Web Template",
				Version:   1,
				Channel:   "web",
				Web:       nil,
				Category:  string(TemplateCategoryBlog),
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
		},
		{
			name: "invalid template - email channel with web field",
			template: func() *Template {
				t := createValidTemplate()
				t.Web = &WebTemplate{
					Content: MapOfAny{"type": "doc"},
				}
				return t
			}(),
			wantErr: true,
		},
		{
			name: "invalid template - web channel with email field",
			template: &Template{
				ID:      "test-web",
				Name:    "Test Web Template",
				Version: 1,
				Channel: "web",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Web:       nil,
				Category:  string(TemplateCategoryBlog),
				CreatedAt: now,
				UpdatedAt: now,
			},
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

func TestWebTemplate_Validate(t *testing.T) {
	tests := []struct {
		name     string
		template *WebTemplate
		wantErr  bool
	}{
		{
			name: "valid web template with Tiptap content",
			template: &WebTemplate{
				Content:   MapOfAny{"type": "doc", "content": []interface{}{}},
				HTML:      "<div>Test content</div>",
				PlainText: "Test content",
			},
			wantErr: false,
		},
		{
			name: "valid web template - minimal content",
			template: &WebTemplate{
				Content: MapOfAny{"type": "doc"},
			},
			wantErr: false,
		},
		{
			name: "invalid web template - missing content",
			template: &WebTemplate{
				HTML:      "<div>Test</div>",
				PlainText: "Test",
			},
			wantErr: true,
		},
		{
			name: "invalid web template - empty content",
			template: &WebTemplate{
				Content: MapOfAny{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.template.Validate(nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWebTemplate_Scan_Value(t *testing.T) {
	web := &WebTemplate{
		Content:   MapOfAny{"type": "doc", "content": []interface{}{}},
		HTML:      "<div>Test content</div>",
		PlainText: "Test content",
	}

	// Test Value() method
	value, err := web.Value()
	assert.NoError(t, err)
	assert.NotNil(t, value)

	// Test basic validation
	err = web.Validate(nil)
	assert.NoError(t, err)

	// Test Scan() method with nil
	newWeb := &WebTemplate{}
	err = newWeb.Scan(nil)
	assert.NoError(t, err)
}

func TestWebTemplate_UnmarshalJSON(t *testing.T) {
	// Test WebTemplate.UnmarshalJSON - this was at 0% coverage
	t.Run("valid JSON", func(t *testing.T) {
		jsonData := []byte(`{
			"content": {"type": "doc", "content": []},
			"html": "<div>Test HTML</div>",
			"plain_text": "Test plain text"
		}`)

		web := &WebTemplate{}
		err := web.UnmarshalJSON(jsonData)
		assert.NoError(t, err)
		assert.NotNil(t, web.Content)
		assert.Equal(t, "<div>Test HTML</div>", web.HTML)
		assert.Equal(t, "Test plain text", web.PlainText)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		jsonData := []byte(`{invalid json}`)

		web := &WebTemplate{}
		err := web.UnmarshalJSON(jsonData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal WebTemplate")
	})

	t.Run("empty JSON", func(t *testing.T) {
		jsonData := []byte(`{}`)

		web := &WebTemplate{}
		err := web.UnmarshalJSON(jsonData)
		assert.NoError(t, err)
	})

	t.Run("partial fields", func(t *testing.T) {
		jsonData := []byte(`{
			"content": {"type": "doc"}
		}`)

		web := &WebTemplate{}
		err := web.UnmarshalJSON(jsonData)
		assert.NoError(t, err)
		assert.NotNil(t, web.Content)
	})
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
		{
			name: "valid web template request",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "web-template",
				Name:        "Test Web Template",
				Channel:     "web",
				Web: &WebTemplate{
					Content:   MapOfAny{"type": "doc", "content": []interface{}{}},
					HTML:      "<div>Test content</div>",
					PlainText: "Test content",
				},
				Category: string(TemplateCategoryBlog),
			},
			wantErr: false,
		},
		{
			name: "invalid web template - missing web field",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "web-template",
				Name:        "Test Web Template",
				Channel:     "web",
				Web:         nil,
				Category:    string(TemplateCategoryBlog),
			},
			wantErr: true,
		},
		{
			name: "invalid request - email channel with web field",
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
				Web: &WebTemplate{
					Content: MapOfAny{"type": "doc"},
				},
				Category: string(TemplateCategoryMarketing),
			},
			wantErr: true,
		},
		{
			name: "invalid request - web channel with email field",
			request: &CreateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "web-template",
				Name:        "Test Web Template",
				Channel:     "web",
				Email: &EmailTemplate{
					SenderID:         "test123",
					Subject:          "Test Subject",
					CompiledPreview:  "<html>Test content</html>",
					VisualEditorTree: createValidMJMLBlock(),
				},
				Web:      nil,
				Category: string(TemplateCategoryBlog),
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
		{
			name: "valid request with channel filter",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"channel":      []string{"email"},
			},
			wantErr: false,
		},
		{
			name: "valid request with category and channel filters",
			queryParams: url.Values{
				"workspace_id": []string{"workspace123"},
				"category":     []string{"marketing"},
				"channel":      []string{"web"},
			},
			wantErr: false,
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
				if channel := tt.queryParams.Get("channel"); channel != "" {
					assert.Equal(t, channel, req.Channel)
				}
				if category := tt.queryParams.Get("category"); category != "" {
					assert.Equal(t, category, req.Category)
				}
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
		{
			name: "valid web template update request",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "web-template",
				Name:        "Updated Web Template",
				Channel:     "web",
				Web: &WebTemplate{
					Content:   MapOfAny{"type": "doc", "content": []interface{}{}},
					HTML:      "<div>Updated content</div>",
					PlainText: "Updated content",
				},
				Category: string(TemplateCategoryBlog),
			},
			wantErr: false,
		},
		{
			name: "invalid web update - missing web field",
			request: &UpdateTemplateRequest{
				WorkspaceID: "workspace123",
				ID:          "web-template",
				Name:        "Updated Web Template",
				Channel:     "web",
				Web:         nil,
				Category:    string(TemplateCategoryBlog),
			},
			wantErr: true,
		},
		{
			name: "invalid update - email channel with web field",
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
				Web: &WebTemplate{
					Content: MapOfAny{"type": "doc"},
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
				assert.Equal(t, tt.request.Channel, template.Channel)
				switch tt.request.Channel {
				case "email":
					assert.Equal(t, tt.request.Email, template.Email)
				case "web":
					assert.Equal(t, tt.request.Web, template.Web)
				}
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
	// Use a fixed timestamp for consistent testing
	testTimestamp := int64(1699564800)

	tests := []struct {
		name           string
		workspaceID    string
		messageID      string
		apiEndpoint    string
		destinationURL string
		expectedBase   string // The base URL without timestamp
	}{
		{
			name:           "with all parameters",
			workspaceID:    "ws-123",
			messageID:      "msg-456",
			apiEndpoint:    "https://api.example.com",
			destinationURL: "https://example.com",
			expectedBase:   "https://api.example.com/visit?mid=msg-456&wid=ws-123&ts=1699564800&url=https%3A%2F%2Fexample.com",
		},
		{
			name:           "with empty api endpoint",
			workspaceID:    "ws-123",
			messageID:      "msg-456",
			apiEndpoint:    "",
			destinationURL: "https://example.com",
			expectedBase:   "/visit?mid=msg-456&wid=ws-123&ts=1699564800&url=https%3A%2F%2Fexample.com",
		},
		{
			name:           "with special characters that need encoding",
			workspaceID:    "ws/123&test=1",
			messageID:      "msg=456?test=1",
			apiEndpoint:    "https://api.example.com",
			destinationURL: "https://example.com/page?param=value&other=test",
			expectedBase:   "https://api.example.com/visit?mid=msg%3D456%3Ftest%3D1&wid=ws%2F123%26test%3D1&ts=1699564800&url=https%3A%2F%2Fexample.com%2Fpage%3Fparam%3Dvalue%26other%3Dtest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := notifuse_mjml.GenerateEmailRedirectionEndpoint(tt.workspaceID, tt.messageID, tt.apiEndpoint, tt.destinationURL, testTimestamp)
			assert.Equal(t, tt.expectedBase, url)
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

// TestEmailTemplate_MjRawContent_JSONRoundTrip tests that mj-raw block content is preserved
// through JSON serialization/deserialization (GitHub issue #229)
func TestEmailTemplate_MjRawContent_JSONRoundTrip(t *testing.T) {
	// JSON that represents a template with mj-raw content
	jsonData := []byte(`{
		"sender_id": "test-sender",
		"subject": "Test Subject",
		"compiled_preview": "<html>test</html>",
		"visual_editor_tree": {
			"id": "mjml-1",
			"type": "mjml",
			"children": [
				{
					"id": "body-1",
					"type": "mj-body",
					"children": [
						{
							"id": "section-1",
							"type": "mj-section",
							"children": [
								{
									"id": "column-1",
									"type": "mj-column",
									"children": [
										{
											"id": "raw-1",
											"type": "mj-raw",
											"content": "<table><tr><td>Cell 1</td><td>Cell 2</td></tr></table>",
											"attributes": {}
										}
									]
								}
							]
						}
					]
				}
			]
		}
	}`)

	// Unmarshal the JSON
	var emailTemplate EmailTemplate
	err := emailTemplate.UnmarshalJSON(jsonData)
	assert.NoError(t, err, "Failed to unmarshal EmailTemplate")

	// Verify the visual_editor_tree was unmarshaled correctly
	assert.NotNil(t, emailTemplate.VisualEditorTree, "VisualEditorTree should not be nil")
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, emailTemplate.VisualEditorTree.GetType())

	// Find the mj-raw block and verify content
	var rawBlock notifuse_mjml.EmailBlock
	bodyBlock := emailTemplate.VisualEditorTree.GetChildren()[0]
	sectionBlock := bodyBlock.GetChildren()[0]
	columnBlock := sectionBlock.GetChildren()[0]
	rawBlock = columnBlock.GetChildren()[0]

	assert.Equal(t, notifuse_mjml.MJMLComponentMjRaw, rawBlock.GetType(), "Expected mj-raw block")
	content := rawBlock.GetContent()
	assert.NotNil(t, content, "mj-raw content should not be nil")
	assert.Equal(t, "<table><tr><td>Cell 1</td><td>Cell 2</td></tr></table>", *content)

	// Marshal back to JSON
	marshaledJSON, err := emailTemplate.MarshalJSON()
	assert.NoError(t, err, "Failed to marshal EmailTemplate")

	// Verify the content is preserved in the marshaled JSON
	assert.Contains(t, string(marshaledJSON), "Cell 1", "Marshaled JSON should contain mj-raw content")
	assert.Contains(t, string(marshaledJSON), "Cell 2", "Marshaled JSON should contain mj-raw content")

	// Unmarshal again to verify round-trip
	var emailTemplate2 EmailTemplate
	err = emailTemplate2.UnmarshalJSON(marshaledJSON)
	assert.NoError(t, err, "Failed to unmarshal EmailTemplate after round-trip")

	// Find the mj-raw block again and verify content
	var rawBlock2 notifuse_mjml.EmailBlock
	bodyBlock2 := emailTemplate2.VisualEditorTree.GetChildren()[0]
	sectionBlock2 := bodyBlock2.GetChildren()[0]
	columnBlock2 := sectionBlock2.GetChildren()[0]
	rawBlock2 = columnBlock2.GetChildren()[0]

	content2 := rawBlock2.GetContent()
	assert.NotNil(t, content2, "mj-raw content should not be nil after round-trip")
	assert.Equal(t, "<table><tr><td>Cell 1</td><td>Cell 2</td></tr></table>", *content2)
}

// TestEmailTemplate_MjRawContent_Value_Scan tests that mj-raw content is preserved
// when using database Value() and Scan() methods (simulating database save/load)
func TestEmailTemplate_MjRawContent_Value_Scan(t *testing.T) {
	// Create an EmailTemplate with mj-raw content
	rawContent := "<table><tr><td>Cell 1</td><td>Cell 2</td></tr></table>"

	rawBase := notifuse_mjml.NewBaseBlock("raw-1", notifuse_mjml.MJMLComponentMjRaw)
	rawBase.Content = &rawContent
	rawBlock := &notifuse_mjml.MJRawBlock{BaseBlock: rawBase}

	columnBlock := &notifuse_mjml.MJColumnBlock{BaseBlock: notifuse_mjml.NewBaseBlock("column-1", notifuse_mjml.MJMLComponentMjColumn)}
	columnBlock.Children = []notifuse_mjml.EmailBlock{rawBlock}

	sectionBlock := &notifuse_mjml.MJSectionBlock{BaseBlock: notifuse_mjml.NewBaseBlock("section-1", notifuse_mjml.MJMLComponentMjSection)}
	sectionBlock.Children = []notifuse_mjml.EmailBlock{columnBlock}

	bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: notifuse_mjml.NewBaseBlock("body-1", notifuse_mjml.MJMLComponentMjBody)}
	bodyBlock.Children = []notifuse_mjml.EmailBlock{sectionBlock}

	mjmlBlock := &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("mjml-1", notifuse_mjml.MJMLComponentMjml)}
	mjmlBlock.Children = []notifuse_mjml.EmailBlock{bodyBlock}

	emailTemplate := &EmailTemplate{
		SenderID:         "test-sender",
		Subject:          "Test Subject",
		CompiledPreview:  "<html>test</html>",
		VisualEditorTree: mjmlBlock,
	}

	// Test Value() - simulates database save
	value, err := emailTemplate.Value()
	assert.NoError(t, err, "Value() should not return error")
	assert.NotNil(t, value, "Value() should return data")

	// Verify the value contains the content
	valueBytes, ok := value.([]byte)
	assert.True(t, ok, "Value() should return []byte")
	assert.Contains(t, string(valueBytes), "Cell 1", "Value() should contain mj-raw content")

	// Test Scan() - simulates database load
	var emailTemplate2 EmailTemplate
	err = emailTemplate2.Scan(valueBytes)
	assert.NoError(t, err, "Scan() should not return error")

	// Verify the visual_editor_tree was scanned correctly
	assert.NotNil(t, emailTemplate2.VisualEditorTree, "VisualEditorTree should not be nil after Scan")

	// Find the mj-raw block and verify content
	var rawBlock2 notifuse_mjml.EmailBlock
	bodyBlock2 := emailTemplate2.VisualEditorTree.GetChildren()[0]
	sectionBlock2 := bodyBlock2.GetChildren()[0]
	columnBlock2 := sectionBlock2.GetChildren()[0]
	rawBlock2 = columnBlock2.GetChildren()[0]

	assert.Equal(t, notifuse_mjml.MJMLComponentMjRaw, rawBlock2.GetType(), "Expected mj-raw block after Scan")
	content2 := rawBlock2.GetContent()
	assert.NotNil(t, content2, "mj-raw content should not be nil after Scan")
	assert.Equal(t, rawContent, *content2, "mj-raw content should be preserved after Value/Scan round-trip")
}
