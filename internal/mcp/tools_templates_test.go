package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/safe-mjml/mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

func templatesRead() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true}}
}

var templatesEpoch = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

const welcomeMJML = `<mjml><mj-head><mj-title>Welcome</mj-title></mj-head><mj-body><mj-section><mj-column><mj-text>Hi {{ contact.first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>`

// codeTemplate carries every field templates.get returns, internal ones
// included.
func codeTemplate() *domain.Template {
	return &domain.Template{
		ID: "welcome", Name: "Welcome", Version: 7, Channel: "email", Category: "welcome",
		Email: &domain.EmailTemplate{
			EditorMode: "code", MjmlSource: strPtr(welcomeMJML), SenderID: "snd_01", ReplyTo: "help@acme.test",
			Subject: "Welcome {{ contact.first_name }}", SubjectPreview: strPtr("Three things to try"),
			CompiledPreview: "<html>compiled-copy</html>", Text: strPtr("Hi {{ contact.first_name }}"),
		},
		TemplateMacroID: strPtr("macro-1"),
		IntegrationID:   strPtr("supabase-1"),
		TestData:        domain.MapOfAny{"contact": map[string]any{"first_name": "John"}},
		Settings:        domain.MapOfAny{"provider": "third-party-setting"},
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: &domain.EmailTemplate{EditorMode: "code", MjmlSource: strPtr("<mjml><mj-body>Bonjour</mj-body></mjml>"), Subject: "Bienvenue"}},
		},
		CreatedAt: templatesEpoch,
		UpdatedAt: templatesEpoch.Add(time.Hour),
	}
}

func visualTree(t *testing.T) notifuse_mjml.EmailBlock {
	t.Helper()
	block, err := notifuse_mjml.UnmarshalEmailBlock([]byte(`{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
		{"id":"section","type":"mj-section","children":[{"id":"column","type":"mj-column","children":[
			{"id":"text","type":"mj-text","content":"Hello {{ contact.first_name }}"}]}]}]}]}`))
	require.NoError(t, err)
	return block
}

func visualTemplate(t *testing.T) *domain.Template {
	return &domain.Template{
		ID: "promo", Name: "Promo", Version: 2, Channel: "email", Category: "marketing",
		Email:     &domain.EmailTemplate{Subject: "Sale", VisualEditorTree: visualTree(t), CompiledPreview: "<mjml>baked</mjml>"},
		CreatedAt: templatesEpoch, UpdatedAt: templatesEpoch,
	}
}

func TestListTemplates_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "list_templates"), "list_templates", []Pair{{Resource: "templates", Verb: "read"}}, []string{"workspace_id"})
}

func TestListTemplates_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	visual := visualTemplate(t)
	visual.CreatedAt = templatesEpoch.Add(-time.Hour)
	web := &domain.Template{
		ID: "layout", Name: "Layout", Version: 1, Channel: "web", Category: "blog",
		Web: &domain.WebTemplate{PlainText: "Body", HTML: "<p>Body</p>"}, CreatedAt: templatesEpoch.Add(-2 * time.Hour), UpdatedAt: templatesEpoch,
	}
	f.templates.EXPECT().GetTemplates(gomock.Any(), testWorkspaceID, "", "").Return([]*domain.Template{web, codeTemplate(), visual}, nil)

	result := f.callTool("list_templates", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"templates":[
			{"id":"welcome","name":"Welcome","version":7,"channel":"email","category":"welcome","editor_mode":"code",
			 "subject":"Welcome {{ contact.first_name }}","subject_preview":"Three things to try","translation_languages":["fr"],
			 "managed_by_integration":true,"created_at":"2026-09-01T08:00:00Z","updated_at":"2026-09-01T09:00:00Z"},
			{"id":"promo","name":"Promo","version":2,"channel":"email","category":"marketing","editor_mode":"visual",
			 "subject":"Sale","translation_languages":[],"managed_by_integration":false,
			 "created_at":"2026-09-01T07:00:00Z","updated_at":"2026-09-01T08:00:00Z"},
			{"id":"layout","name":"Layout","version":1,"channel":"web","category":"blog","editor_mode":null,
			 "translation_languages":[],"managed_by_integration":false,
			 "created_at":"2026-09-01T06:00:00Z","updated_at":"2026-09-01T08:00:00Z"}
		]
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"mjml", "compiled", "visual_editor_tree", "test_data", "third-party-setting", "supabase-1", "macro-1", "<p>Body</p>", `"web":`} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// templates.list orders by the last update; the tool pages by creation time,
// which an update leaves alone, with the ID breaking ties.
func TestListTemplates_PagesOnAStableOrder(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().GetTemplates(gomock.Any(), testWorkspaceID, "marketing", "email").DoAndReturn(func(context.Context, string, string, string) ([]*domain.Template, error) {
		return []*domain.Template{
			{ID: "a", CreatedAt: templatesEpoch, UpdatedAt: templatesEpoch.Add(48 * time.Hour)},
			{ID: "gone", CreatedAt: templatesEpoch.Add(3 * time.Hour), DeletedAt: &templatesEpoch},
			{ID: "c", CreatedAt: templatesEpoch.Add(time.Hour), UpdatedAt: templatesEpoch.Add(time.Hour)},
			{ID: "b", CreatedAt: templatesEpoch.Add(time.Hour), UpdatedAt: templatesEpoch.Add(time.Hour)},
		}, nil
	}).Times(2)

	type page struct {
		Templates []struct {
			ID string `json:"id"`
		} `json:"templates"`
		NextCursor string `json:"next_cursor"`
	}
	args := map[string]any{"workspace_id": testWorkspaceID, "category": "marketing", "channel": "email", "limit": 2}
	var first page
	f.callTool("list_templates", args).decode(t, &first)
	require.Len(t, first.Templates, 2)
	assert.Equal(t, "b", first.Templates[0].ID)
	assert.Equal(t, "c", first.Templates[1].ID)
	require.NotEmpty(t, first.NextCursor)

	args["cursor"] = first.NextCursor
	var second page
	f.callTool("list_templates", args).decode(t, &second)
	require.Len(t, second.Templates, 1)
	assert.Equal(t, "a", second.Templates[0].ID)
	assert.Empty(t, second.NextCursor)

	toolErr := f.callTool("list_templates", map[string]any{"workspace_id": testWorkspaceID, "category": "welcome", "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to its filters")
}

func TestListTemplates_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	for name, args := range map[string]map[string]any{
		"unknown category": {"category": "newsletter"},
		"unknown channel":  {"channel": "sms"},
		"limit above 50":   {"limit": 51},
		"garbage cursor":   {"cursor": "%%%"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_templates", args).errorOf(t).Code)
		})
	}
}

func TestListTemplates_ErrorsAreMapped(t *testing.T) {
	t.Run("a key without templates:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(contactsRead())
		toolErr := f.callTool("list_templates", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the templates read permission", Resource: "templates", Permission: "read"}, toolErr)
	})

	t.Run("a permission refusal of the service", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		f.templates.EXPECT().GetTemplates(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, domain.NewPermissionError(domain.PermissionResourceTemplates, domain.PermissionTypeRead, "Insufficient permissions: read access to templates required"))
		assert.Equal(t, CodePermissionDenied, f.callTool("list_templates", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t).Code)
	})

	t.Run("a failed read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		f.templates.EXPECT().GetTemplates(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("failed to get templates: pq"))
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("list_templates", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
	})
}

func TestGetTemplate_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "get_template"), "get_template", []Pair{{Resource: "templates", Verb: "read"}}, []string{"workspace_id", "id"})
}

func TestGetTemplate_CodeTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(codeTemplate(), nil)

	result := f.callTool("get_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome"})
	require.False(t, result.IsError, string(result.StructuredContent))
	expected, err := json.Marshal(map[string]any{
		"workspace": map[string]any{"id": "ws1", "name": "Acme"},
		"template": map[string]any{
			"id": "welcome", "name": "Welcome", "version": 7, "channel": "email", "category": "welcome",
			"editor_mode": "code",
			"subject":     "Welcome {{ contact.first_name }}", "subject_preview": "Three things to try",
			"sender_id": "snd_01", "reply_to": "help@acme.test",
			"mjml": welcomeMJML, "mjml_origin": "source", "text": "Hi {{ contact.first_name }}",
			"test_data": map[string]any{"contact": map[string]any{"first_name": "John"}},
			"translations": map[string]any{
				"fr": map[string]any{"subject": "Bienvenue", "mjml": "<mjml><mj-body>Bonjour</mj-body></mjml>", "mjml_origin": "source"},
			},
			"managed_by_integration": true,
			"created_at":             "2026-09-01T08:00:00Z", "updated_at": "2026-09-01T09:00:00Z",
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(result.StructuredContent))
	for _, dropped := range []string{"compiled", "visual_editor_tree", "third-party-setting", "supabase-1", "macro-1", "settings", "deleted_at"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// The MJML is editable content: returned whole, however long.
func TestGetTemplate_MJMLIsWhole(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	template := codeTemplate()
	long := "<mjml><mj-body>" + strings.Repeat("x", 5000) + "</mj-body></mjml>"
	template.Email.MjmlSource = &long
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(template, nil)

	var out struct {
		Template map[string]json.RawMessage `json:"template"`
	}
	f.callTool("get_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome"}).decode(t, &out)
	var mjmlOut string
	require.NoError(t, json.Unmarshal(out.Template["mjml"], &mjmlOut))
	assert.Equal(t, long, mjmlOut)
	assert.NotContains(t, out.Template, "truncated")
}

// A visual template's MJML is an export of its design, to read only.
func TestGetTemplate_VisualTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "promo", int64(2)).Return(visualTemplate(t), nil)

	var out struct {
		Template struct {
			EditorMode string `json:"editor_mode"`
			MJML       string `json:"mjml"`
			MJMLOrigin string `json:"mjml_origin"`
		} `json:"template"`
	}
	result := f.callTool("get_template", map[string]any{"workspace_id": testWorkspaceID, "id": "promo", "version": 2})
	result.decode(t, &out)
	assert.Equal(t, "visual", out.Template.EditorMode)
	assert.NotContains(t, result.Content[0].Text, "editable_by_mcp")
	assert.Equal(t, "export", out.Template.MJMLOrigin)
	assert.True(t, strings.HasPrefix(out.Template.MJML, "<mjml"), out.Template.MJML)
	assert.Contains(t, out.Template.MJML, "Hello {{ contact.first_name }}", "Liquid is kept as written")
	assert.NotContains(t, result.Content[0].Text, "baked")
}

func TestGetTemplate_WebTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&domain.Template{
		ID: "layout", Name: "Layout", Version: 1, Channel: "web", Category: "blog",
		Web:       &domain.WebTemplate{Content: domain.MapOfAny{"type": "doc"}, HTML: "<p>secret-html</p>", PlainText: strings.Repeat("w", 2001)},
		CreatedAt: templatesEpoch, UpdatedAt: templatesEpoch,
	}, nil)

	result := f.callTool("get_template", map[string]any{"workspace_id": testWorkspaceID, "id": "layout"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"template":{"id":"layout","name":"Layout","version":1,"channel":"web","category":"blog","editor_mode":null,
			"web_text":"`+strings.Repeat("w", 2000)+`","managed_by_integration":false,
			"created_at":"2026-09-01T08:00:00Z","updated_at":"2026-09-01T08:00:00Z",
			"truncated":true,"truncated_fields":["web_text"]}
	}`, string(result.StructuredContent))
}

func TestGetTemplate_NotFound(t *testing.T) {
	t.Run("a deleted template, which templates.get returns", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		deleted := codeTemplate()
		deleted.DeletedAt = &templatesEpoch
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(deleted, nil)
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "template not found"}, f.callTool("get_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome"}).errorOf(t))
	})

	t.Run("an unknown template", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &domain.ErrTemplateNotFound{Message: "template not found"})
		assert.Equal(t, CodeNotFound, f.callTool("get_template", map[string]any{"workspace_id": testWorkspaceID, "id": "nope"}).errorOf(t).Code)
	})
}

func TestGetTemplate_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	for name, args := range map[string]map[string]any{
		"no id":          {},
		"id with spaces": {"id": "my template"},
		"long id":        {"id": strings.Repeat("a", 33)},
		"version 0":      {"id": "welcome", "version": 0},
		"version text":   {"id": "welcome", "version": "7"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("get_template", args).errorOf(t).Code)
		})
	}
}

func TestRenderTemplate_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "render_template"), "render_template", []Pair{{Resource: "templates", Verb: "read"}}, []string{"workspace_id"})
}

// A draft of MJML is compiled as the console's preview compiles it, with
// tracking off; the HTML comes only when asked for.
func TestRenderTemplate_Draft(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	var got domain.CompileTemplateRequest
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			got = req
			return &domain.CompileTemplateResponse{
				Success: true, MJML: strPtr("<mjml>compiled-mjml</mjml>"), HTML: strPtr("<html>Hi Ada</html>"),
				Subject: strPtr("Hello Ada"), SubjectPreview: strPtr("Preview"), Text: strPtr("Hi Ada"), TextSource: "auto",
				TemplateData: notifuse_mjml.MapOfAny{"workspace": map[string]any{"base_url": "https://api.acme.test"}},
			}, nil
		}).Times(2)

	args := map[string]any{
		"workspace_id": testWorkspaceID, "mjml_source": welcomeMJML, "subject": "Hello {{ contact.first_name }}",
		"subject_preview": "Preview", "test_data": map[string]any{"contact": map[string]any{"first_name": "Ada"}},
	}
	result := f.callTool("render_template", args)
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"success":true,"subject":"Hello Ada","subject_preview":"Preview","text":"Hi Ada","text_source":"auto","error":null
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"compiled-mjml", "api.acme.test", "<html>"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
	assert.Equal(t, domain.CompileTemplateRequest{
		WorkspaceID: testWorkspaceID, MessageID: "preview", Channel: "email",
		MjmlSource: strPtr(welcomeMJML), Subject: strPtr("Hello {{ contact.first_name }}"), SubjectPreview: strPtr("Preview"),
		TemplateData:     notifuse_mjml.MapOfAny{"contact": map[string]any{"first_name": "Ada"}},
		TrackingSettings: notifuse_mjml.TrackingSettings{EnableTracking: false},
		IncludeText:      true,
	}, got)

	args["include_html"] = true
	var withHTML struct {
		HTML string `json:"html"`
	}
	f.callTool("render_template", args).decode(t, &withHTML)
	assert.Equal(t, "<html>Hi Ada</html>", withHTML.HTML)
}

// A saved template renders as a send would: its source, subject, preview
// text injected in mj-preview, custom text and saved test data.
func TestRenderTemplate_SavedTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(7)).Return(codeTemplate(), nil)
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			assert.Equal(t, welcomeMJML, *req.MjmlSource)
			assert.Equal(t, "Welcome {{ contact.first_name }}", *req.Subject)
			assert.Equal(t, "Three things to try", *req.SubjectPreview)
			assert.Equal(t, "Three things to try", *req.SubjectPreviewOverride)
			assert.Equal(t, "Hi {{ contact.first_name }}", *req.Text)
			assert.Equal(t, notifuse_mjml.MapOfAny{"contact": map[string]any{"first_name": "John"}}, req.TemplateData)
			assert.Equal(t, "preview", req.MessageID)
			assert.False(t, req.TrackingSettings.EnableTracking)
			assert.True(t, req.IncludeText)
			return &domain.CompileTemplateResponse{Success: true, Subject: strPtr("Welcome John")}, nil
		})

	var out struct {
		Subject string `json:"subject"`
	}
	f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "template_id": "welcome", "version": 7}).decode(t, &out)
	assert.Equal(t, "Welcome John", out.Subject)
}

func TestRenderTemplate_Translation(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(codeTemplate(), nil).Times(2)
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			assert.Equal(t, "<mjml><mj-body>Bonjour</mj-body></mjml>", *req.MjmlSource)
			assert.Equal(t, "Bienvenue", *req.Subject)
			assert.Equal(t, notifuse_mjml.MapOfAny{"first_name": "Zoé"}, req.TemplateData, "the argument replaces the saved test data")
			return &domain.CompileTemplateResponse{Success: true}, nil
		})

	result := f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "template_id": "welcome", "language": "fr", "test_data": map[string]any{"first_name": "Zoé"}})
	require.False(t, result.IsError, string(result.StructuredContent))

	toolErr := f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "template_id": "welcome", "language": "de"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeValidation, Message: "template welcome has no de translation; its translations: fr"}, toolErr)
}

// An MJML or Liquid error is the answer to the question asked, not a failed
// call.
func TestRenderTemplate_CompileFailure(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).Return(&domain.CompileTemplateResponse{
		Success: false, Subject: strPtr("Hi"),
		Error: &mjml.Error{Message: "MJML compilation failed", Details: []mjml.ErrorDetail{{Line: 12, Message: strings.Repeat("e", 2001), TagName: "mj-button"}},
			Warnings: []mjml.Warning{{Message: "lint"}}},
	}, nil)

	result := f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "mjml_source": "<mjml><mj-body><mj-button></mjml>"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},"success":false,"subject":"Hi",
		"error":{"message":"MJML compilation failed","details":[{"line":12,"message":"`+strings.Repeat("e", 2000)+`","tag":"mj-button"}]},
		"truncated":true,"truncated_fields":["error.details[0].message"]
	}`, string(result.StructuredContent))
}

func TestRenderTemplate_CapsTheText(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		Return(&domain.CompileTemplateResponse{Success: true, Text: strPtr(strings.Repeat("t", 2500)), HTML: strPtr(strings.Repeat("h", 5000))}, nil)

	var out struct {
		Text            string   `json:"text"`
		HTML            string   `json:"html"`
		TruncatedFields []string `json:"truncated_fields"`
	}
	f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "mjml_source": welcomeMJML, "include_html": true}).decode(t, &out)
	assert.Equal(t, strings.Repeat("t", 2000), out.Text)
	assert.Equal(t, strings.Repeat("h", 5000), out.HTML, "the HTML is returned whole")
	assert.Equal(t, []string{"text"}, out.TruncatedFields)
}

func TestRenderTemplate_RefusesBadArgumentsBeforeTheServices(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	for name, args := range map[string]map[string]any{
		"neither template_id nor mjml_source": {},
		"both":                                {"template_id": "welcome", "mjml_source": welcomeMJML},
		"empty mjml_source":                   {"mjml_source": "  "},
		"version with a draft":                {"mjml_source": welcomeMJML, "version": 2},
		"language with a draft":               {"mjml_source": welcomeMJML, "language": "fr"},
		"subject with a saved template":       {"template_id": "welcome", "subject": "Hi"},
		"test_data that is not an object":     {"mjml_source": welcomeMJML, "test_data": "x"},
		"bad template id":                     {"template_id": "a b"},
		"arguments above 1 MiB":               {"mjml_source": "<mjml>" + strings.Repeat("x", 1<<20) + "</mjml>"},
		"version 0":                           {"template_id": "welcome", "version": 0},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("render_template", args).errorOf(t).Code)
		})
	}
}

func TestRenderTemplate_RefusesWhatCannotRender(t *testing.T) {
	t.Run("a web template", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&domain.Template{ID: "layout", Channel: "web", Web: &domain.WebTemplate{}}, nil)
		toolErr := f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "template_id": "layout"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeValidation, Message: "template layout is a web template: only email templates render"}, toolErr)
	})

	t.Run("a deleted template", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		deleted := codeTemplate()
		deleted.DeletedAt = &templatesEpoch
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(deleted, nil)
		assert.Equal(t, CodeNotFound, f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "template_id": "welcome"}).errorOf(t).Code)
	})

	t.Run("a visual template without a design", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(templatesRead())
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&domain.Template{ID: "empty", Channel: "email", Email: &domain.EmailTemplate{Subject: "x"}}, nil)
		toolErr := f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "template_id": "empty"}).errorOf(t)
		assert.Equal(t, CodeValidation, toolErr.Code)
		assert.Contains(t, toolErr.Message, "visual_editor_tree")
	})
}

// CompileTemplate skips its own templates:read check under the context the
// wrapper hands it, so the pre-check is what refuses a key without it.
func TestRenderTemplate_AKeyWithoutTemplatesReadNeverReachesTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceTemplates: {Write: true}})
	toolErr := f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "mjml_source": welcomeMJML}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the templates read permission", Resource: "templates", Permission: "read"}, toolErr)
}

func TestRenderTemplate_ServiceErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesRead())
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("failed to authenticate user: %w", domain.ErrUserNotInWorkspace))
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "workspace not found"},
		f.callTool("render_template", map[string]any{"workspace_id": testWorkspaceID, "mjml_source": welcomeMJML}).errorOf(t))
}
