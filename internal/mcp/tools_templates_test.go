package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

func templatesWrite() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceTemplates: {Write: true}}
}

func templatesReadWrite() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}}
}

// stampedMJML is welcomeMJML as a save rewrites it: <mj-preview> added.
const stampedMJML = `<mjml><mj-head><mj-title>Welcome</mj-title>
    <mj-preview>Three things to try</mj-preview></mj-head><mj-body><mj-section><mj-column><mj-text>Hi {{ contact.first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>`

// compiles answers every compile check with success and records the requests.
func (f *fixture) compiles() *[]domain.CompileTemplateRequest {
	var requests []domain.CompileTemplateRequest
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			requests = append(requests, req)
			return &domain.CompileTemplateResponse{Success: true, HTML: strPtr("<html></html>")}, nil
		}).AnyTimes()
	return &requests
}

func createTemplateArgs() map[string]any {
	return map[string]any{
		"workspace_id": testWorkspaceID, "id": "welcome", "name": "Welcome", "category": "welcome",
		"subject": "Welcome {{ contact.first_name }}", "subject_preview": "Three things to try", "mjml_source": welcomeMJML,
		"reply_to": "help@acme.test", "test_data": map[string]any{"contact": map[string]any{"first_name": "John"}},
	}
}

func TestCreateTemplate_Definition(t *testing.T) {
	tool := catalogueTool(t, "create_template")
	assertWriteTool(t, tool, "create_template", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: false},
		[]Pair{{Resource: "templates", Verb: "write"}}, "templates.create", []string{"workspace_id", "id", "name", "category", "subject", "mjml_source"})
	category := tool.InputSchema["properties"].(map[string]any)["category"].(map[string]any)
	assert.Equal(t, []string{"marketing", "transactional", "welcome", "opt_in", "unsubscribe", "bounce", "blocklist", "other"}, category["enum"])
}

// A code template (D9), checked by a compile with its test data before the
// save, and returned with the MJML as the save rewrote it.
func TestCreateTemplate_SavesACodeTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesWrite())
	audit := f.captureAudit()
	compiled := f.compiles()
	var saved domain.Template
	f.templates.EXPECT().CreateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
		saved = *template
		saved.Email = new(domain.EmailTemplate)
		*saved.Email = *template.Email
		// As the service does: version 1, timestamps, mj-preview written.
		template.Version, template.CreatedAt, template.UpdatedAt = 1, templatesEpoch, templatesEpoch
		template.Email.MjmlSource = strPtr(stampedMJML)
		return nil
	})

	result := f.callTool("create_template", createTemplateArgs())
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.Equal(t, domain.Template{
		ID: "welcome", Name: "Welcome", Version: 1, Channel: "email", Category: "welcome",
		Email: &domain.EmailTemplate{
			EditorMode: "code", MjmlSource: strPtr(welcomeMJML), ReplyTo: "help@acme.test",
			Subject: "Welcome {{ contact.first_name }}", SubjectPreview: strPtr("Three things to try"), CompiledPreview: welcomeMJML,
		},
		TestData: domain.MapOfAny{"contact": map[string]any{"first_name": "John"}},
	}, saved)

	require.Len(t, *compiled, 1)
	check := (*compiled)[0]
	assert.Equal(t, welcomeMJML, *check.MjmlSource)
	assert.Equal(t, "Welcome {{ contact.first_name }}", *check.Subject)
	assert.Equal(t, "Three things to try", *check.SubjectPreview)
	assert.Equal(t, notifuse_mjml.MapOfAny{"contact": map[string]any{"first_name": "John"}}, check.TemplateData)
	assert.False(t, check.TrackingSettings.EnableTracking)

	var out struct {
		Template struct {
			Version    int64   `json:"version"`
			EditorMode string  `json:"editor_mode"`
			MJML       string  `json:"mjml"`
			MJMLOrigin string  `json:"mjml_origin"`
			Text       *string `json:"text"`
		} `json:"template"`
		TranslationsUnchanged []string `json:"translations_unchanged"`
	}
	result.decode(t, &out)
	assert.Equal(t, int64(1), out.Template.Version)
	assert.Equal(t, "code", out.Template.EditorMode)
	assert.Equal(t, stampedMJML, out.Template.MJML, "the MJML as stored")
	assert.Equal(t, "source", out.Template.MJMLOrigin)
	assert.Nil(t, out.Template.Text)
	assert.Nil(t, out.TranslationsUnchanged)
	assert.NotContains(t, result.Content[0].Text, "compiled_preview")

	row := audit.last(t)
	assert.Equal(t, "templates.create", row.Action)
	assert.Equal(t, domain.AuditCategoryTemplates, row.Category)
	assert.Equal(t, domain.AuditTargetTemplate, row.TargetType)
	assert.Equal(t, "welcome", row.TargetID)
	assert.Equal(t, "Welcome", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

// A save never compiles code-mode MJML, so the tool does, and refuses what
// would fail at every send.
func TestCreateTemplate_RefusesWhatDoesNotCompile(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesWrite())
	audit := f.captureAudit()
	f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).Return(&domain.CompileTemplateResponse{
		Success: false,
		Error:   &mjml.Error{Message: "MJML compilation failed", Details: []mjml.ErrorDetail{{Line: 3, Message: "mj-button cannot hold mj-section", TagName: "mj-button"}}},
	}, nil)

	toolErr := f.callTool("create_template", createTemplateArgs()).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeValidation,
		Message: "the template does not compile, so nothing was saved: MJML compilation failed; line 3 <mj-button>: mj-button cannot hold mj-section"}, toolErr)
	assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
}

func TestCreateTemplate_ReplayIsAConflict(t *testing.T) {
	for name, tc := range map[string]struct {
		deleted bool
		message string
	}{
		"a live template":    {deleted: false, message: "template welcome already exists"},
		"a deleted template": {deleted: true, message: "template welcome already exists and was deleted: its identifier cannot be reused"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesWrite())
			audit := f.captureAudit()
			f.compiles()
			f.templates.EXPECT().CreateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).
				Return(fmt.Errorf("failed to create template: %w", &domain.ErrTemplateExists{Message: "template id already exists", Deleted: tc.deleted}))
			assert.Equal(t, ToolError{Code: CodeConflict, Message: tc.message}, f.callTool("create_template", createTemplateArgs()).errorOf(t))
			assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
		})
	}
}

// templates.create validates in its handler and caps the body there: the
// tool does both before any service.
func TestCreateTemplate_RefusesBadArgumentsBeforeTheServices(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesWrite())
	audit := f.captureAudit()
	for name, change := range map[string]map[string]any{
		"an ID with a space":         {"id": "wel come"},
		"a subject over 255 bytes":   {"subject": strings.Repeat("é", 128)},
		"an empty MJML":              {"mjml_source": ""},
		"a bad reply-to":             {"reply_to": "not-an-email"},
		"a blog category":            {"category": "blog"},
		"a text Liquid cannot parse": {"text": "Hi {% if contact %}"},
		"arguments over 1 MiB":       {"mjml_source": "<mjml>" + strings.Repeat("x", 1<<20) + "</mjml>"},
	} {
		t.Run(name, func(t *testing.T) {
			args := createTemplateArgs()
			for key, value := range change {
				args[key] = value
			}
			assert.Equal(t, CodeValidation, f.callTool("create_template", args).errorOf(t).Code)
			assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
		})
	}
}

func TestUpdateTemplate_Definition(t *testing.T) {
	assertWriteTool(t, catalogueTool(t, "update_template"), "update_template", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		[]Pair{{Resource: "templates", Verb: "write"}, {Resource: "templates", Verb: "read"}}, "templates.update", []string{"workspace_id", "id"})
}

// storedCodeTemplate is codeTemplate as the write path reads it: a template
// not managed by an integration.
func storedCodeTemplate() *domain.Template {
	template := codeTemplate()
	template.IntegrationID = nil
	return template
}

// templates.update erases every field it is not sent: the tool reads the
// latest version and sends each stored field it was not given.
func TestUpdateTemplate_KeepsTheFieldsNotGiven(t *testing.T) {
	stored := func(change func(email *domain.EmailTemplate, template *domain.Template)) domain.Template {
		template := domain.Template{
			ID: "welcome", Name: "Welcome", Channel: "email", Category: "welcome",
			Email: &domain.EmailTemplate{
				EditorMode: "code", MjmlSource: strPtr(welcomeMJML), SenderID: "snd_01", ReplyTo: "help@acme.test",
				Subject: "Welcome {{ contact.first_name }}", SubjectPreview: strPtr("Three things to try"),
				CompiledPreview: welcomeMJML, Text: strPtr("Hi {{ contact.first_name }}"),
			},
			TemplateMacroID: strPtr("macro-1"),
			TestData:        domain.MapOfAny{"contact": map[string]any{"first_name": "John"}},
			Settings:        domain.MapOfAny{"provider": "third-party-setting"},
		}
		change(template.Email, &template)
		return template
	}
	newMJML := `<mjml><mj-body><mj-section><mj-column><mj-text>Hello again</mj-text></mj-column></mj-section></mj-body></mjml>`
	for name, tc := range map[string]struct {
		args map[string]any
		want domain.Template
	}{
		"a new subject": {
			args: map[string]any{"subject": "Hello"},
			want: stored(func(email *domain.EmailTemplate, _ *domain.Template) { email.Subject = "Hello" }),
		},
		"new MJML": {
			args: map[string]any{"mjml_source": newMJML},
			want: stored(func(email *domain.EmailTemplate, _ *domain.Template) {
				email.MjmlSource, email.CompiledPreview = strPtr(newMJML), newMJML
			}),
		},
		"a rename and a category": {
			args: map[string]any{"name": "Hello", "category": "transactional"},
			want: stored(func(_ *domain.EmailTemplate, template *domain.Template) {
				template.Name, template.Category = "Hello", "transactional"
			}),
		},
		"empty texts clear the optional fields": {
			args: map[string]any{"subject_preview": "", "text": "", "sender_id": "", "reply_to": ""},
			want: stored(func(email *domain.EmailTemplate, _ *domain.Template) {
				email.SubjectPreview, email.Text, email.SenderID, email.ReplyTo = nil, nil, "", ""
			}),
		},
		"new test data": {
			args: map[string]any{"test_data": map[string]any{"order": map[string]any{"id": "42"}}},
			want: stored(func(_ *domain.EmailTemplate, template *domain.Template) {
				template.TestData = domain.MapOfAny{"order": map[string]any{"id": "42"}}
			}),
		},
		"a base version": {
			args: map[string]any{"subject": "Hello", "base_version": 7},
			want: stored(func(email *domain.EmailTemplate, template *domain.Template) {
				email.Subject, template.Version = "Hello", 7
			}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesReadWrite())
			f.captureAudit()
			f.compiles()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(storedCodeTemplate(), nil)
			var sent domain.Template
			f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
				sent = *template
				sent.Email = new(domain.EmailTemplate)
				*sent.Email = *template.Email
				return nil
			})

			tc.args["workspace_id"], tc.args["id"] = testWorkspaceID, "welcome"
			result := f.callTool("update_template", tc.args)
			require.False(t, result.IsError, string(result.StructuredContent))
			assert.Equal(t, tc.want, sent)
			assert.Nil(t, sent.Translations, "translations stay out of the request, so the service keeps the stored ones")
		})
	}
}

// The compile check runs when anything it compiles changes, not for a rename.
func TestUpdateTemplate_CompilesOnlyANewContent(t *testing.T) {
	for name, tc := range map[string]struct {
		args     map[string]any
		compiles int
	}{
		"a rename":           {args: map[string]any{"name": "Hello"}, compiles: 0},
		"a new reply-to":     {args: map[string]any{"reply_to": "team@acme.test"}, compiles: 0},
		"a new subject":      {args: map[string]any{"subject": "Hello"}, compiles: 1},
		"a new preview text": {args: map[string]any{"subject_preview": "Hi"}, compiles: 1},
		"new MJML":           {args: map[string]any{"mjml_source": welcomeMJML}, compiles: 1},
		"a new text":         {args: map[string]any{"text": "Hi {{ contact.first_name }}"}, compiles: 1},
		"new test data":      {args: map[string]any{"test_data": map[string]any{"order": map[string]any{"id": "42"}}}, compiles: 1},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesReadWrite())
			f.captureAudit()
			compiled := f.compiles()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(storedCodeTemplate(), nil)
			f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).Return(nil)
			tc.args["workspace_id"], tc.args["id"] = testWorkspaceID, "welcome"
			require.False(t, f.callTool("update_template", tc.args).IsError)
			assert.Len(t, *compiled, tc.compiles)
		})
	}
}

// A text or test data that fails to render would fail at every send, so the
// tool compiles them with the stored content and saves nothing when that fails.
func TestUpdateTemplate_RefusesATextOrTestDataThatDoesNotCompile(t *testing.T) {
	for name, tc := range map[string]struct {
		args  map[string]any
		check func(t *testing.T, req domain.CompileTemplateRequest)
	}{
		"a new text": {
			args: map[string]any{"text": "Hi {{ contact.first_name | no_such_filter }}"},
			check: func(t *testing.T, req domain.CompileTemplateRequest) {
				require.NotNil(t, req.Text)
				assert.Equal(t, "Hi {{ contact.first_name | no_such_filter }}", *req.Text)
			},
		},
		"new test data": {
			args: map[string]any{"test_data": map[string]any{"order": map[string]any{"id": "42"}}},
			check: func(t *testing.T, req domain.CompileTemplateRequest) {
				assert.Equal(t, map[string]any{"order": map[string]any{"id": "42"}}, map[string]any(req.TemplateData))
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesReadWrite())
			audit := f.captureAudit()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(storedCodeTemplate(), nil)
			f.templates.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
					tc.check(t, req)
					return &domain.CompileTemplateResponse{Success: false, Error: &mjml.Error{Message: "Liquid error in the text"}}, nil
				})
			// No UpdateTemplate expectation: a save would fail the mock.
			tc.args["workspace_id"], tc.args["id"] = testWorkspaceID, "welcome"
			toolErr := f.callTool("update_template", tc.args).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeValidation, Message: "the template does not compile, so nothing was saved: Liquid error in the text"}, toolErr)
			assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
		})
	}
}

func TestUpdateTemplate_ReturnsTheStoredTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesReadWrite())
	audit := f.captureAudit()
	f.compiles()
	stored := storedCodeTemplate()
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(stored, nil)
	f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
		// As the service does: the stored translations kept, mj-title and
		// mj-preview written, a new version.
		template.Translations = codeTemplate().Translations
		template.Email.MjmlSource = strPtr(stampedMJML)
		template.Version, template.CreatedAt, template.UpdatedAt = 8, templatesEpoch, templatesEpoch.Add(2*time.Hour)
		return nil
	})

	result := f.callTool("update_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome", "subject": "Hello"})
	require.False(t, result.IsError, string(result.StructuredContent))
	var out struct {
		Template struct {
			Version      int64                     `json:"version"`
			Subject      string                    `json:"subject"`
			MJML         string                    `json:"mjml"`
			Translations map[string]map[string]any `json:"translations"`
		} `json:"template"`
		TranslationsUnchanged []string `json:"translations_unchanged"`
	}
	result.decode(t, &out)
	assert.Equal(t, int64(8), out.Template.Version)
	assert.Equal(t, "Hello", out.Template.Subject)
	assert.Equal(t, stampedMJML, out.Template.MJML)
	assert.Equal(t, "Bienvenue", out.Template.Translations["fr"]["subject"])
	assert.Equal(t, []string{"fr"}, out.TranslationsUnchanged)

	row := audit.last(t)
	assert.Equal(t, "templates.update", row.Action)
	assert.Equal(t, domain.AuditTargetTemplate, row.TargetType)
	assert.Equal(t, "welcome", row.TargetID)
	assert.Equal(t, "Welcome", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

// D9: MCP writes code templates only. A visual template, a legacy one saved
// before the mode existed, and a web template are refused before any write.
func TestUpdateTemplate_RefusesVisualAndWebTemplates(t *testing.T) {
	legacy := visualTemplate(t)
	legacy.ID = "legacy"
	for name, tc := range map[string]struct {
		stored  *domain.Template
		message string
	}{
		"a visual template": {
			stored: func() *domain.Template {
				template := visualTemplate(t)
				template.Email.EditorMode = "visual"
				return template
			}(),
			message: "template promo is edited in visual mode: update_template changes code templates only. Edit it in the console, or make a new template with create_template",
		},
		"a template saved before the editor mode existed": {
			stored:  legacy,
			message: "template legacy is edited in visual mode: update_template changes code templates only. Edit it in the console, or make a new template with create_template",
		},
		"a web template": {
			stored: &domain.Template{ID: "layout", Name: "Layout", Version: 1, Channel: "web", Category: "blog",
				Web: &domain.WebTemplate{Content: domain.MapOfAny{"type": "doc"}}},
			message: "template layout is a web template (a blog layout): update_template changes code email templates only",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesReadWrite())
			audit := f.captureAudit()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, tc.stored.ID, int64(0)).Return(tc.stored, nil)
			toolErr := f.callTool("update_template", map[string]any{"workspace_id": testWorkspaceID, "id": tc.stored.ID, "subject": "Hello"}).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeValidation, Message: tc.message}, toolErr)
			assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
		})
	}
}

func TestUpdateTemplate_VersionConflict(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesReadWrite())
	audit := f.captureAudit()
	f.compiles()
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(storedCodeTemplate(), nil)
	f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(&domain.ErrTemplateVersionConflict{BaseVersion: 6, LatestVersion: 7})

	result := f.callTool("update_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome", "subject": "Hello", "base_version": 6})
	latest := int64(7)
	assert.Equal(t, ToolError{Code: CodeConflict, Message: "template was modified: edit is based on version 6 but latest is 7", LatestVersion: &latest}, result.errorOf(t))
	assert.Contains(t, string(result.StructuredContent), `"latest_version":7`)
	assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
}

func TestUpdateTemplate_DeletedOrUnknownIsNotFound(t *testing.T) {
	deleted := storedCodeTemplate()
	deleted.DeletedAt = &templatesEpoch
	for name, tc := range map[string]struct {
		template *domain.Template
		err      error
	}{
		"a deleted template":  {template: deleted},
		"an unknown template": {err: &domain.ErrTemplateNotFound{Message: "template not found"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesReadWrite())
			audit := f.captureAudit()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(tc.template, tc.err)
			toolErr := f.callTool("update_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome", "subject": "Hello"}).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeNotFound, Message: "template not found"}, toolErr)
			assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
		})
	}
}

func TestUpdateTemplate_RefusesBadArgumentsBeforeTheWrite(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"nothing to change":    {"base_version": 3},
		"base version zero":    {"subject": "Hello", "base_version": 0},
		"an empty subject":     {"subject": ""},
		"an empty MJML":        {"mjml_source": ""},
		"a bad reply-to":       {"reply_to": "nope"},
		"arguments over 1 MiB": {"mjml_source": "<mjml>" + strings.Repeat("x", 1<<20) + "</mjml>"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesReadWrite())
			f.captureAudit()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "welcome", int64(0)).Return(storedCodeTemplate(), nil).MaxTimes(1)
			args["workspace_id"], args["id"] = testWorkspaceID, "welcome"
			assert.Equal(t, CodeValidation, f.callTool("update_template", args).errorOf(t).Code)
		})
	}
}

// The read goes through templates.get, which needs templates:read.
func TestUpdateTemplate_NeedsTheReadPair(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesWrite())
	audit := f.captureAudit()
	toolErr := f.callTool("update_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome", "subject": "Hello"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the templates read permission", Resource: "templates", Permission: "read"}, toolErr)
	assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
}

func TestDeleteTemplate_Definition(t *testing.T) {
	assertWriteTool(t, catalogueTool(t, "delete_template"), "delete_template", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		[]Pair{{Resource: "templates", Verb: "write"}}, "templates.delete", []string{"workspace_id", "id"})
}

func TestDeleteTemplate(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesWrite())
	audit := f.captureAudit()
	f.templates.EXPECT().DeleteTemplate(gomock.Any(), testWorkspaceID, "welcome").Return(nil)

	result := f.callTool("delete_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"deleted":true,"id":"welcome"}`, string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "templates.delete", row.Action)
	assert.Equal(t, domain.AuditCategoryTemplates, row.Category)
	assert.Equal(t, domain.AuditTargetTemplate, row.TargetType)
	assert.Equal(t, "welcome", row.TargetID)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

func TestDeleteTemplate_RefusalsAreMapped(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		want   ToolError
		status int
	}{
		// The repository deletes only a template not yet deleted.
		"a replay": {
			err:    &domain.ErrTemplateNotFound{Message: "template not found"},
			want:   ToolError{Code: CodeNotFound, Message: "template not found"},
			status: http.StatusNotFound,
		},
		"a template an integration manages": {
			err:    domain.NewValidationError("cannot delete integration-managed template: template is managed by integration supabase-1"),
			want:   ToolError{Code: CodeValidation, Message: "cannot delete integration-managed template: template is managed by integration supabase-1"},
			status: http.StatusBadRequest,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(templatesWrite())
			audit := f.captureAudit()
			f.templates.EXPECT().DeleteTemplate(gomock.Any(), testWorkspaceID, "welcome").Return(tc.err)
			assert.Equal(t, tc.want, f.callTool("delete_template", map[string]any{"workspace_id": testWorkspaceID, "id": "welcome"}).errorOf(t))
			assert.Equal(t, tc.status, audit.last(t).StatusCode)
		})
	}
}

func TestDeleteTemplate_RefusesABadIDBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(templatesWrite())
	f.captureAudit()
	assert.Equal(t, CodeValidation, f.callTool("delete_template", map[string]any{"workspace_id": testWorkspaceID, "id": "wel come"}).errorOf(t).Code)
}
