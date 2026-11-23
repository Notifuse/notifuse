package mcp

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

const listTemplatesName = "list_templates"

// maxRenderArgumentBytes bounds the arguments of render_template, as
// templates.compile bounds its request body.
const maxRenderArgumentBytes = 1 << 20

// renderMessageID is the message ID of a preview render, as the console sends
// it: it names no real message.
const renderMessageID = "preview"

var templateCategories = []string{
	string(domain.TemplateCategoryMarketing),
	string(domain.TemplateCategoryTransactional),
	string(domain.TemplateCategoryWelcome),
	string(domain.TemplateCategoryOptIn),
	string(domain.TemplateCategoryUnsubscribe),
	string(domain.TemplateCategoryBounce),
	string(domain.TemplateCategoryBlocklist),
	string(domain.TemplateCategoryBlog),
	string(domain.TemplateCategoryOther),
}

// listTemplatesTool pages through a workspace's templates. templates.list
// returns every template at once, ordered by its last update, so the tool
// pages them by offset on their creation order, which an update leaves alone.
func (s *Server) listTemplatesTool() *Tool {
	return &Tool{
		Name:  listTemplatesName,
		Title: "List templates",
		Description: "Lists the templates of a workspace, newest first: " +
			"name, latest version, channel, category, editor mode, subject, preview text and translated languages. " +
			"get_template has the content, and render_template gives the rendered email. " +
			"To get the next page, pass next_cursor back as cursor with the same filters. A template created between two pages can show up twice.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"category":     map[string]any{"type": "string", "enum": templateCategories, "description": "Only templates of this category."},
				"channel": map[string]any{
					"type":        "string",
					"enum":        []string{domain.ChannelEmail, domain.ChannelWeb},
					"description": "Only templates of this channel. Web templates are blog layouts.",
				},
				"limit":  map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor": map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same filters."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listTemplates,
	}
}

type listTemplatesArgs struct {
	Category string `json:"category"`
	Channel  string `json:"channel"`
	Limit    *int   `json:"limit"`
	Cursor   string `json:"cursor"`
}

func (a *listTemplatesArgs) filters() map[string]any {
	filters := map[string]any{}
	if a.Category != "" {
		filters["category"] = a.Category
	}
	if a.Channel != "" {
		filters["channel"] = a.Channel
	}
	return filters
}

// templateSummary is the projection list_templates returns: no body, no test
// data, no third-party settings.
type templateSummary struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Version              int64     `json:"version"`
	Channel              string    `json:"channel"`
	Category             string    `json:"category"`
	EditorMode           *string   `json:"editor_mode"`
	Subject              *string   `json:"subject,omitempty"`
	SubjectPreview       *string   `json:"subject_preview,omitempty"`
	TranslationLanguages []string  `json:"translation_languages"`
	ManagedByIntegration bool      `json:"managed_by_integration"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	Truncation
}

type listTemplatesOutput struct {
	Workspace  WorkspaceRef      `json:"workspace"`
	Templates  []templateSummary `json:"templates"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (s *Server) listTemplates(ctx context.Context, call *Call) (any, error) {
	var args listTemplatesArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	filters := args.filters()
	offset := 0
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listTemplatesName, call.WorkspaceID, filters, &offset); toolErr != nil {
			return nil, toolErr
		}
		if offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	templates, err := s.deps.Templates.GetTemplates(ctx, call.WorkspaceID, args.Category, args.Channel)
	if err != nil {
		return nil, err
	}
	templates = slices.DeleteFunc(templates, func(template *domain.Template) bool {
		return template == nil || template.DeletedAt != nil
	})
	slices.SortFunc(templates, func(a, b *domain.Template) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	out := listTemplatesOutput{Workspace: call.WorkspaceRef(), Templates: []templateSummary{}}
	if offset >= len(templates) {
		return out, nil
	}
	end := min(offset+limit, len(templates))
	for _, template := range templates[offset:end] {
		out.Templates = append(out.Templates, summarizeTemplate(template))
	}
	if end < len(templates) {
		next, err := EncodeCursor(listTemplatesName, call.WorkspaceID, filters, end)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

func summarizeTemplate(template *domain.Template) templateSummary {
	out := templateSummary{
		ID:                   template.ID,
		Version:              template.Version,
		Channel:              template.Channel,
		Category:             template.Category,
		EditorMode:           editorMode(template),
		TranslationLanguages: slices.Sorted(maps.Keys(template.Translations)),
		ManagedByIntegration: template.IntegrationID != nil,
		CreatedAt:            template.CreatedAt,
		UpdatedAt:            template.UpdatedAt,
	}
	if out.TranslationLanguages == nil {
		out.TranslationLanguages = []string{}
	}
	out.Name = out.Truncation.Text("name", template.Name)
	if template.Email != nil {
		subject := template.Email.Subject
		out.Subject = &subject
		out.SubjectPreview = template.Email.SubjectPreview
	}
	return out
}

// editorMode is the editor of a template's default email content: visual when
// it was saved before the field existed, nil for a web template.
func editorMode(template *domain.Template) *string {
	if template.Email == nil {
		return nil
	}
	mode := template.Email.EditorMode
	if mode == "" {
		mode = domain.EditorModeVisual
	}
	return &mode
}

// getTemplateTool reads one template whole.
func (s *Server) getTemplateTool() *Tool {
	return &Tool{
		Name:  "get_template",
		Title: "Get template",
		Description: "Reads one template: " +
			"name, version, channel, category, editor mode, subject, preview text, sender, reply-to, the MJML, the custom plain-text version, the Liquid test data and each translation. " +
			"The plain-text version is absent when the template uses the automatic one. " +
			"For a code template, mjml is the stored source (mjml_origin source). " +
			"For a visual template, mjml is an export of the visual design (mjml_origin export). " +
			"A web template (blog layout) has its text in web_text, cut at 2000 characters. Returns a not_found error for a deleted template.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":           map[string]any{"type": "string", "description": "Template ID: letters, digits, underscores and hyphens, 1 to 32 characters."},
				"version":      map[string]any{"type": "integer", "description": "A past version to read, 1 or more. Omit for the latest."},
			},
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.getTemplate,
	}
}

type getTemplateArgs struct {
	ID      string `json:"id"`
	Version *int64 `json:"version"`
}

// The origins of a template's mjml in get_template.
const (
	mjmlOriginSource = "source"
	mjmlOriginExport = "export"
)

type templateTranslationContent struct {
	Subject        *string `json:"subject,omitempty"`
	SubjectPreview *string `json:"subject_preview,omitempty"`
	MJML           *string `json:"mjml,omitempty"`
	MJMLOrigin     string  `json:"mjml_origin,omitempty"`
	Text           *string `json:"text,omitempty"`
	WebText        string  `json:"web_text,omitempty"`
}

// templateFull is the projection get_template returns. Left out: the visual
// editor's block tree, the compiled preview, the web body's Tiptap JSON and
// HTML, third-party settings, the macro and integration IDs.
type templateFull struct {
	ID                   string                                `json:"id"`
	Name                 string                                `json:"name"`
	Version              int64                                 `json:"version"`
	Channel              string                                `json:"channel"`
	Category             string                                `json:"category"`
	EditorMode           *string                               `json:"editor_mode"`
	Subject              *string                               `json:"subject,omitempty"`
	SubjectPreview       *string                               `json:"subject_preview,omitempty"`
	SenderID             string                                `json:"sender_id,omitempty"`
	ReplyTo              string                                `json:"reply_to,omitempty"`
	MJML                 *string                               `json:"mjml,omitempty"`
	MJMLOrigin           string                                `json:"mjml_origin,omitempty"`
	Text                 *string                               `json:"text,omitempty"`
	WebText              string                                `json:"web_text,omitempty"`
	TestData             map[string]any                        `json:"test_data,omitempty"`
	Translations         map[string]templateTranslationContent `json:"translations,omitempty"`
	ManagedByIntegration bool                                  `json:"managed_by_integration"`
	CreatedAt            time.Time                             `json:"created_at"`
	UpdatedAt            time.Time                             `json:"updated_at"`
	Truncation
}

type getTemplateOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Template  templateFull `json:"template"`
}

func (s *Server) getTemplate(ctx context.Context, call *Call) (any, error) {
	var args getTemplateArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	template, toolErr, err := s.readTemplate(ctx, call.WorkspaceID, args.ID, args.Version)
	if toolErr != nil {
		return nil, toolErr
	}
	if err != nil {
		return nil, err
	}
	return getTemplateOutput{Workspace: call.WorkspaceRef(), Template: projectTemplate(template)}, nil
}

// readTemplate reads a template as templates.get does, with the request's own
// checks first, and answers not_found for a deleted one: templates.get
// returns it, since sent messages still render from it.
func (s *Server) readTemplate(ctx context.Context, workspaceID, id string, version *int64) (*domain.Template, *ToolError, error) {
	params := url.Values{"workspace_id": {workspaceID}, "id": {id}}
	if version != nil {
		if *version < 1 {
			return nil, validationError("version must be 1 or more"), nil
		}
		params.Set("version", strconv.FormatInt(*version, 10))
	}
	var req domain.GetTemplateRequest
	if err := req.FromURLParams(params); err != nil {
		return nil, validationError("%v", err), nil
	}
	template, err := s.deps.Templates.GetTemplateByID(ctx, req.WorkspaceID, req.ID, req.Version)
	if err != nil {
		return nil, nil, err
	}
	if template == nil || template.DeletedAt != nil {
		return nil, nil, &domain.ErrTemplateNotFound{Message: "template not found"}
	}
	return template, nil, nil
}

func projectTemplate(template *domain.Template) templateFull {
	out := templateFull{
		ID:                   template.ID,
		Version:              template.Version,
		Channel:              template.Channel,
		Category:             template.Category,
		EditorMode:           editorMode(template),
		ManagedByIntegration: template.IntegrationID != nil,
		CreatedAt:            template.CreatedAt,
		UpdatedAt:            template.UpdatedAt,
	}
	out.Name = out.Truncation.Text("name", template.Name)
	if len(template.TestData) > 0 {
		out.TestData = map[string]any(template.TestData)
	}
	if email := template.Email; email != nil {
		subject := email.Subject
		out.Subject = &subject
		out.SubjectPreview = email.SubjectPreview
		out.SenderID = out.Truncation.Text("sender_id", email.SenderID)
		out.ReplyTo = out.Truncation.Text("reply_to", email.ReplyTo)
		out.MJML, out.MJMLOrigin = emailMJML(email)
		out.Text = email.Text
	}
	if template.Web != nil {
		out.WebText = out.Truncation.Text("web_text", template.Web.PlainText)
	}
	if len(template.Translations) > 0 {
		out.Translations = make(map[string]templateTranslationContent, len(template.Translations))
		for _, language := range slices.Sorted(maps.Keys(template.Translations)) {
			translation := template.Translations[language]
			var content templateTranslationContent
			if email := translation.Email; email != nil {
				subject := email.Subject
				content.Subject = &subject
				content.SubjectPreview = email.SubjectPreview
				content.MJML, content.MJMLOrigin = emailMJML(email)
				content.Text = email.Text
			}
			if translation.Web != nil {
				content.WebText = out.Truncation.Text("translations."+language+".web_text", translation.Web.PlainText)
			}
			out.Translations[language] = content
		}
	}
	return out
}

// emailMJML is the MJML of an email content: the stored source of a code
// template, else an export of the visual design that keeps Liquid as written.
func emailMJML(email *domain.EmailTemplate) (*string, string) {
	if source := email.GetCodeModeMjmlSource(); source != nil {
		mjml := *source
		return &mjml, mjmlOriginSource
	}
	if email.VisualEditorTree == nil {
		return nil, ""
	}
	mjml := notifuse_mjml.ConvertJSONToMJMLRaw(email.VisualEditorTree)
	return &mjml, mjmlOriginExport
}

// renderTemplateTool renders a saved template, or a draft of MJML, with Liquid
// test data.
func (s *Server) renderTemplateTool() *Tool {
	return &Tool{
		Name:  "render_template",
		Title: "Render template",
		Description: "Renders an email with Liquid test data, as it would be sent. " +
			"It returns the subject, the preview text, the plain-text version and, with include_html, the HTML. " +
			"Give template_id to render a saved template, with version and language to pick a past version or a translation. " +
			"Or give mjml_source to check a draft before saving it. Tracking is off, so links stay as written. " +
			"An MJML or Liquid error still makes a successful call: success is false, with the error's message, line and tag. " +
			"The plain text and the error messages are cut at 2000 characters. The HTML is returned whole. All arguments together are limited to 1 MiB.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"template_id":     map[string]any{"type": "string", "description": "Render this saved email template. Give template_id or mjml_source, not both."},
				"version":         map[string]any{"type": "integer", "description": "With template_id: a past version, 1 or more. Omit for the latest."},
				"language":        map[string]any{"type": "string", "description": "With template_id: render this translation, such as fr. Omit for the default content."},
				"mjml_source":     map[string]any{"type": "string", "description": "Render this MJML instead of a saved template, to check a draft."},
				"subject":         map[string]any{"type": "string", "description": "With mjml_source: a subject to render with Liquid."},
				"subject_preview": map[string]any{"type": "string", "description": "With mjml_source: a preview text to render with Liquid."},
				"test_data":       map[string]any{"type": "object", "description": "Liquid data, such as {\"contact\": {\"first_name\": \"Ada\"}}. Default: the saved template's test data, or none for mjml_source."},
				"include_html":    map[string]any{"type": "boolean", "description": "Also return the rendered HTML. Default false."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.renderTemplate,
	}
}

type renderTemplateArgs struct {
	TemplateID     string         `json:"template_id"`
	Version        *int64         `json:"version"`
	Language       string         `json:"language"`
	MJMLSource     *string        `json:"mjml_source"`
	Subject        *string        `json:"subject"`
	SubjectPreview *string        `json:"subject_preview"`
	TestData       map[string]any `json:"test_data"`
	IncludeHTML    bool           `json:"include_html"`
}

type renderErrorDetail struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
	Tag     string `json:"tag,omitempty"`
}

type renderError struct {
	Message string              `json:"message"`
	Details []renderErrorDetail `json:"details"`
}

// renderTemplateOutput is what render_template returns. Left out: the
// compiled MJML, and the test data echoed with the workspace URLs added.
type renderTemplateOutput struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Success        bool         `json:"success"`
	Subject        *string      `json:"subject,omitempty"`
	SubjectPreview *string      `json:"subject_preview,omitempty"`
	Text           *string      `json:"text,omitempty"`
	TextSource     string       `json:"text_source,omitempty"`
	TextError      string       `json:"text_error,omitempty"`
	HTML           *string      `json:"html,omitempty"`
	Error          *renderError `json:"error"`
	Truncation
}

func (s *Server) renderTemplate(ctx context.Context, call *Call) (any, error) {
	// templates.compile refuses a body above 1 MiB before it reads it.
	if len(call.RawArgs) > maxRenderArgumentBytes {
		return nil, validationError("arguments are limited to %d bytes", maxRenderArgumentBytes)
	}
	var args renderTemplateArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	hasSource := args.MJMLSource != nil
	if (args.TemplateID == "") == !hasSource {
		return nil, validationError("give either template_id or mjml_source, not both")
	}
	if hasSource && strings.TrimSpace(*args.MJMLSource) == "" {
		return nil, validationError("mjml_source is empty")
	}
	if hasSource && (args.Version != nil || args.Language != "") {
		return nil, validationError("version and language go with template_id")
	}
	if !hasSource && (args.Subject != nil || args.SubjectPreview != nil) {
		return nil, validationError("subject and subject_preview go with mjml_source: a saved template renders its own")
	}

	fields := map[string]any{
		"workspace_id":      call.WorkspaceID,
		"message_id":        renderMessageID,
		"channel":           domain.ChannelEmail,
		"tracking_settings": map[string]any{"enable_tracking": false},
	}
	var content *domain.EmailTemplate
	testData := args.TestData
	if hasSource {
		fields["mjml_source"] = *args.MJMLSource
		if args.Subject != nil {
			fields["subject"] = *args.Subject
		}
		if args.SubjectPreview != nil {
			fields["subject_preview"] = *args.SubjectPreview
		}
	} else {
		template, toolErr, err := s.readTemplate(ctx, call.WorkspaceID, args.TemplateID, args.Version)
		if toolErr != nil {
			return nil, toolErr
		}
		if err != nil {
			return nil, err
		}
		if template.Email == nil {
			return nil, validationError("template %s is a web template: only email templates render", template.ID)
		}
		if args.Language != "" {
			if translation, ok := template.Translations[args.Language]; !ok || translation.Email == nil {
				return nil, validationError("template %s has no %s translation; its translations: %s", template.ID, args.Language, emailLanguages(template))
			}
		}
		content = template.EmailContentForLanguage(args.Language)
		fields["subject"] = content.Subject
		if content.SubjectPreview != nil {
			fields["subject_preview"] = *content.SubjectPreview
		}
		if testData == nil && len(template.TestData) > 0 {
			testData = map[string]any(template.TestData)
		}
	}
	if testData != nil {
		fields["test_data"] = testData
	}

	var req domain.CompileTemplateRequest
	if err := decodeRequest(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if content != nil {
		// As every send does: the design or source, the preview text injected in
		// mj-preview, and the custom plain text.
		content.ApplyToCompileRequest(&req, nil)
	}
	// templates.compile validates in its handler, not in the service.
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	req.IncludeText = true

	// CompileTemplate skips its own templates:read check when the context holds
	// the workspace user, as this one does: the wrapper's pre-check guards it.
	resp, err := s.deps.Templates.CompileTemplate(ctx, req)
	if err != nil {
		return nil, err
	}
	return projectRender(call.WorkspaceRef(), resp, args.IncludeHTML), nil
}

// emailLanguages lists the languages of a template's email translations, for
// an error message.
func emailLanguages(template *domain.Template) string {
	var languages []string
	for _, language := range slices.Sorted(maps.Keys(template.Translations)) {
		if template.Translations[language].Email != nil {
			languages = append(languages, language)
		}
	}
	if len(languages) == 0 {
		return "none"
	}
	return strings.Join(languages, ", ")
}

func projectRender(workspace WorkspaceRef, resp *domain.CompileTemplateResponse, includeHTML bool) renderTemplateOutput {
	out := renderTemplateOutput{Workspace: workspace}
	if resp == nil {
		return out
	}
	out.Success = resp.Success
	text := func(path string, value *string) *string {
		if value == nil {
			return nil
		}
		capped := out.Truncation.Text(path, *value)
		return &capped
	}
	out.Subject = text("subject", resp.Subject)
	out.SubjectPreview = text("subject_preview", resp.SubjectPreview)
	out.Text = text("text", resp.Text)
	out.TextSource = resp.TextSource
	out.TextError = out.Truncation.Text("text_error", resp.TextError)
	if includeHTML && resp.HTML != nil {
		html := *resp.HTML
		out.HTML = &html
	}
	if resp.Error != nil {
		out.Error = &renderError{
			Message: out.Truncation.Text("error.message", resp.Error.Message),
			Details: make([]renderErrorDetail, 0, len(resp.Error.Details)),
		}
		for i, detail := range resp.Error.Details {
			out.Error.Details = append(out.Error.Details, renderErrorDetail{
				Line:    detail.Line,
				Message: out.Truncation.Text(fmt.Sprintf("error.details[%d].message", i), detail.Message),
				Tag:     out.Truncation.Text(fmt.Sprintf("error.details[%d].tag", i), detail.TagName),
			})
		}
	}
	return out
}
