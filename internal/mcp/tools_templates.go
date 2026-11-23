package mcp

import (
	"cmp"
	"context"
	"errors"
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

// maxTemplateArgumentBytes bounds the arguments of render_template,
// create_template and update_template, as templates.compile, templates.create
// and templates.update bound their request body.
const maxTemplateArgumentBytes = 1 << 20

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
	if len(call.RawArgs) > maxTemplateArgumentBytes {
		return nil, validationError("arguments are limited to %d bytes", maxTemplateArgumentBytes)
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

// emailTemplateCategories are the categories of an email template: all but
// blog, which belongs to web templates.
var emailTemplateCategories = slices.DeleteFunc(slices.Clone(templateCategories), func(category string) bool {
	return category == string(domain.TemplateCategoryBlog)
})

// templateWriteArgs are the arguments of create_template and update_template.
// Every field is a pointer, so update_template tells an omitted one from an
// empty one.
type templateWriteArgs struct {
	ID             string         `json:"id"`
	BaseVersion    *int64         `json:"base_version"`
	Name           *string        `json:"name"`
	Category       *string        `json:"category"`
	Subject        *string        `json:"subject"`
	SubjectPreview *string        `json:"subject_preview"`
	MJMLSource     *string        `json:"mjml_source"`
	Text           *string        `json:"text"`
	SenderID       *string        `json:"sender_id"`
	ReplyTo        *string        `json:"reply_to"`
	TestData       map[string]any `json:"test_data"`
}

// decodeTemplateWrite applies templates.create's and templates.update's body
// cap, which their handler checks and the service does not, then decodes the
// arguments.
func decodeTemplateWrite(call *Call) (templateWriteArgs, *ToolError) {
	var args templateWriteArgs
	if len(call.RawArgs) > maxTemplateArgumentBytes {
		return args, validationError("arguments are limited to %d bytes", maxTemplateArgumentBytes)
	}
	if err := call.DecodeArgs(&args); err != nil {
		return args, validationError("invalid arguments: %v", err)
	}
	return args, nil
}

// templateOutput is what create_template and update_template return: the
// template as get_template reads it, with the MJML as the save rewrote it.
type templateOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Template  templateFull `json:"template"`
	// TranslationsUnchanged lists the languages whose content update_template
	// left as it was.
	TranslationsUnchanged []string `json:"translations_unchanged,omitempty"`
}

// checkTemplateCompiles compiles a template's default email with its test
// data, as a send would, and refuses it with the error's line and tag when it
// does not compile: a save never compiles code-mode MJML, so a broken one
// would fail at every send.
func (s *Server) checkTemplateCompiles(ctx context.Context, workspaceID string, template *domain.Template) error {
	email := template.Email
	fields := map[string]any{
		"workspace_id":      workspaceID,
		"message_id":        renderMessageID,
		"channel":           domain.ChannelEmail,
		"tracking_settings": map[string]any{"enable_tracking": false},
		"subject":           email.Subject,
	}
	if email.SubjectPreview != nil {
		fields["subject_preview"] = *email.SubjectPreview
	}
	if len(template.TestData) > 0 {
		fields["test_data"] = map[string]any(template.TestData)
	}
	var req domain.CompileTemplateRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return validationError("invalid arguments: %v", err)
	}
	email.ApplyToCompileRequest(&req, nil)
	if err := req.Validate(); err != nil {
		return validationError("%v", err)
	}
	resp, err := s.deps.Templates.CompileTemplate(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("template compile returned no result")
	}
	if resp.Success {
		return nil
	}
	reason := "unknown error"
	if resp.Error != nil {
		parts := []string{resp.Error.Message}
		for _, detail := range resp.Error.Details {
			part := fmt.Sprintf("line %d", detail.Line)
			if detail.TagName != "" {
				part += " <" + detail.TagName + ">"
			}
			parts = append(parts, part+": "+detail.Message)
		}
		reason = strings.Join(parts, "; ")
	}
	message, _ := CapText("the template does not compile, so nothing was saved: " + reason)
	return &ToolError{Code: CodeValidation, Message: message}
}

func (s *Server) createTemplateTool() *Tool {
	return &Tool{
		Name:  "create_template",
		Title: "Create template",
		Description: "Creates an email template from MJML, with the ID you choose. " +
			"It is a code template: it opens in the console's MJML editor, not the visual builder. " +
			"The MJML, subject and preview text are compiled with test_data first, and a template that does not compile is refused with a validation error that gives the line and tag. " +
			"Saving sets <mj-title> to the name and <mj-preview> to the preview text, so the result returns the MJML as stored. " +
			"Nothing is sent. The ID cannot be reused, even after the template is deleted, so a repeated call answers conflict. " +
			"Translations are not set here. All arguments together are limited to 1 MiB.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(false),
		Pairs:       []Pair{{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeWrite}},
		AuditAction: "templates.create",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":              map[string]any{"type": "string", "description": "Letters, digits, underscores and hyphens, 1 to 32 characters."},
				"name":            map[string]any{"type": "string", "description": "1 to 255 characters."},
				"category":        map[string]any{"type": "string", "enum": emailTemplateCategories, "description": "What the email is for. A marketing email should link {{ unsubscribe_url }}."},
				"subject":         map[string]any{"type": "string", "description": "Subject with Liquid, 1 to 255 bytes."},
				"subject_preview": map[string]any{"type": "string", "description": "Inbox preview text with Liquid, at most 255 bytes. Default: the name."},
				"mjml_source":     map[string]any{"type": "string", "description": "The whole email as MJML with Liquid, starting with <mjml>."},
				"text":            map[string]any{"type": "string", "description": "Custom plain-text version with Liquid, at most 64 KiB. Omit to generate it from the HTML."},
				"sender_id":       map[string]any{"type": "string", "description": "Sender ID, as get_template shows on another template. Omit for the default sender."},
				"reply_to":        map[string]any{"type": "string", "description": "Reply-to email address."},
				"test_data":       map[string]any{"type": "object", "description": "Sample Liquid data for the compile check and render_template, such as {\"contact\": {\"first_name\": \"Ada\"}}."},
			},
			"required":             []string{"workspace_id", "id", "name", "category", "subject", "mjml_source"},
			"additionalProperties": false,
		},
		Handler: s.createTemplate,
	}
}

func (s *Server) createTemplate(ctx context.Context, call *Call) (any, error) {
	args, toolErr := decodeTemplateWrite(call)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.SetTarget(domain.AuditTargetTemplate, args.ID, valueOr(args.Name, ""))

	// Code mode (D9): the MJML is the template.
	email := map[string]any{
		"editor_mode": domain.EditorModeCode,
		"mjml_source": valueOr(args.MJMLSource, ""),
		"subject":     valueOr(args.Subject, ""),
	}
	for key, value := range map[string]*string{
		"subject_preview": args.SubjectPreview,
		"text":            args.Text,
		"sender_id":       args.SenderID,
		"reply_to":        args.ReplyTo,
	} {
		if value != nil && *value != "" {
			email[key] = *value
		}
	}
	fields := map[string]any{
		"workspace_id": call.WorkspaceID,
		"id":           args.ID,
		"name":         valueOr(args.Name, ""),
		"channel":      domain.ChannelEmail,
		"category":     valueOr(args.Category, ""),
		"email":        email,
	}
	if args.TestData != nil {
		fields["test_data"] = args.TestData
	}
	var req domain.CreateTemplateRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// templates.create validates in its handler, not in the service.
	template, _, err := req.Validate()
	if err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.checkTemplateCompiles(ctx, call.WorkspaceID, template); err != nil {
		return nil, err
	}

	if err := s.deps.Templates.CreateTemplate(ctx, call.WorkspaceID, template); err != nil {
		var exists *domain.ErrTemplateExists
		if errors.As(err, &exists) {
			return nil, &ToolError{Code: CodeConflict, Message: domain.NewAlreadyExistsError("template", template.ID, exists.Deleted).Error()}
		}
		return nil, err
	}
	return templateOutput{Workspace: call.WorkspaceRef(), Template: projectTemplate(template)}, nil
}

func (s *Server) updateTemplateTool() *Tool {
	return &Tool{
		Name:  "update_template",
		Title: "Update template",
		Description: "Changes the fields you give on a code email template and keeps the others. Each update saves a new version. " +
			"Every later send that uses it, from broadcasts, transactional notifications and automations, gets the new content, so confirm the change with the user first. " +
			"A template made in the visual editor, or a web template, is refused with a validation error: edit it in the console, or make a new one with create_template. " +
			"A new mjml_source, subject or subject_preview is compiled with the test data first, and a template that does not compile is refused. " +
			"Saving sets <mj-title> to the name and <mj-preview> to the preview text, so the result returns the MJML as stored. " +
			"Translations are not changed: translations_unchanged lists their languages. " +
			"The template is read then written. Give base_version to refuse the update when someone saved a newer version since: it then answers conflict with latest_version. " +
			"Without base_version, calling it again with the same arguments saves the same content as one more version; with it, a repeated call answers conflict. " +
			"All arguments together are limited to 1 MiB.",
		Class:      ClassWriteOpenWorld,
		Idempotent: boolPtr(true),
		Pairs: []Pair{
			{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeWrite},
			{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeRead},
		},
		AuditAction: "templates.update",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":              map[string]any{"type": "string", "description": "The template's ID."},
				"base_version":    map[string]any{"type": "integer", "description": "The version your change starts from, as get_template gives it. Omit to save over whatever is latest."},
				"name":            map[string]any{"type": "string", "description": "1 to 255 characters."},
				"category":        map[string]any{"type": "string", "enum": emailTemplateCategories},
				"subject":         map[string]any{"type": "string", "description": "Subject with Liquid, 1 to 255 bytes."},
				"subject_preview": map[string]any{"type": "string", "description": "Inbox preview text with Liquid, at most 255 bytes. An empty text clears it, and the name is used."},
				"mjml_source":     map[string]any{"type": "string", "description": "The whole email as MJML with Liquid, replacing the stored one."},
				"text":            map[string]any{"type": "string", "description": "Custom plain-text version with Liquid, at most 64 KiB. An empty text goes back to the one generated from the HTML."},
				"sender_id":       map[string]any{"type": "string", "description": "Sender ID. An empty text goes back to the default sender."},
				"reply_to":        map[string]any{"type": "string", "description": "Reply-to email address. An empty text clears it."},
				"test_data":       map[string]any{"type": "object", "description": "Sample Liquid data, replacing the stored data."},
			},
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.updateTemplate,
	}
}

// codeEmailFields are a stored code-mode email as templates.update takes it
// back: every field but the compiled preview, which the save makes again.
func codeEmailFields(email *domain.EmailTemplate) map[string]any {
	fields := map[string]any{"editor_mode": domain.EditorModeCode, "subject": email.Subject}
	if email.MjmlSource != nil {
		fields["mjml_source"] = *email.MjmlSource
	}
	if email.SubjectPreview != nil {
		fields["subject_preview"] = *email.SubjectPreview
	}
	if email.Text != nil {
		fields["text"] = *email.Text
	}
	if email.SenderID != "" {
		fields["sender_id"] = email.SenderID
	}
	if email.ReplyTo != "" {
		fields["reply_to"] = email.ReplyTo
	}
	if email.VisualEditorTree != nil {
		fields["visual_editor_tree"] = email.VisualEditorTree
	}
	return fields
}

func (s *Server) updateTemplate(ctx context.Context, call *Call) (any, error) {
	args, toolErr := decodeTemplateWrite(call)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.SetTarget(domain.AuditTargetTemplate, args.ID, "")
	if args.Name == nil && args.Category == nil && args.Subject == nil && args.SubjectPreview == nil && args.MJMLSource == nil &&
		args.Text == nil && args.SenderID == nil && args.ReplyTo == nil && args.TestData == nil {
		return nil, validationError("give at least one field to change besides id and base_version")
	}
	if args.BaseVersion != nil && *args.BaseVersion < 1 {
		return nil, validationError("base_version must be 1 or more")
	}

	// templates.update replaces the whole template, so the tool reads the
	// latest version and writes it back with the given fields changed.
	stored, toolErr, err := s.readTemplate(ctx, call.WorkspaceID, args.ID, nil)
	if toolErr != nil {
		return nil, toolErr
	}
	if err != nil {
		return nil, err
	}
	call.Audit.SetTarget("", "", stored.Name)
	if stored.Channel != domain.ChannelEmail || stored.Email == nil {
		return nil, validationError("template %s is a %s template (a blog layout): update_template changes code email templates only", stored.ID, stored.Channel)
	}
	if mode := *editorMode(stored); mode != domain.EditorModeCode {
		return nil, validationError("template %s is edited in %s mode: update_template changes code templates only. Edit it in the console, or make a new template with create_template", stored.ID, mode)
	}

	email := codeEmailFields(stored.Email)
	for key, value := range map[string]*string{
		"subject":         args.Subject,
		"mjml_source":     args.MJMLSource,
		"subject_preview": args.SubjectPreview,
		"text":            args.Text,
		"sender_id":       args.SenderID,
		"reply_to":        args.ReplyTo,
	} {
		switch {
		case value == nil:
		case *value == "" && key != "subject" && key != "mjml_source":
			delete(email, key)
		default:
			email[key] = *value
		}
	}
	fields := map[string]any{
		"workspace_id": call.WorkspaceID,
		"id":           stored.ID,
		"name":         valueOr(args.Name, stored.Name),
		"channel":      domain.ChannelEmail,
		"category":     valueOr(args.Category, stored.Category),
		"email":        email,
	}
	if stored.TemplateMacroID != nil {
		fields["template_macro_id"] = *stored.TemplateMacroID
	}
	if len(stored.Settings) > 0 {
		fields["settings"] = map[string]any(stored.Settings)
	}
	switch {
	case args.TestData != nil:
		fields["test_data"] = args.TestData
	case len(stored.TestData) > 0:
		fields["test_data"] = map[string]any(stored.TestData)
	}
	if args.BaseVersion != nil {
		fields["base_version"] = *args.BaseVersion
	}
	// Translations stay out of the request: the service keeps the stored ones.
	var req domain.UpdateTemplateRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// templates.update validates in its handler, not in the service.
	template, _, err := req.Validate()
	if err != nil {
		return nil, validationError("%v", err)
	}
	// Every field the compile reads: the MJML, subject and preview, the plain
	// text, and the test data the Liquid renders with.
	if args.MJMLSource != nil || args.Subject != nil || args.SubjectPreview != nil || args.Text != nil || args.TestData != nil {
		if err := s.checkTemplateCompiles(ctx, call.WorkspaceID, template); err != nil {
			return nil, err
		}
	}

	if err := s.deps.Templates.UpdateTemplate(ctx, call.WorkspaceID, template); err != nil {
		return nil, err
	}
	call.Audit.SetTarget("", "", template.Name)
	out := templateOutput{Workspace: call.WorkspaceRef(), Template: projectTemplate(template)}
	if len(stored.Translations) > 0 {
		out.TranslationsUnchanged = slices.Sorted(maps.Keys(stored.Translations))
	}
	return out, nil
}

func (s *Server) deleteTemplateTool() *Tool {
	return &Tool{
		Name:  "delete_template",
		Title: "Delete template",
		Description: "Deletes a template. Messages already sent with it still display, and its ID cannot be used again. " +
			"Nothing checks whether broadcasts, transactional notifications or automations still use it, and this cannot be undone: confirm with the user first. " +
			"A template managed by an integration is refused with a validation error. " +
			"Returns not_found for a template that does not exist or is already deleted, which is also what a repeated call answers.",
		Class:       ClassDestructive,
		Pairs:       []Pair{{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeWrite}},
		AuditAction: "templates.delete",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":           map[string]any{"type": "string", "description": "The template's ID."},
			},
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.deleteTemplate,
	}
}

type deleteTemplateOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Deleted   bool         `json:"deleted"`
	ID        string       `json:"id"`
}

func (s *Server) deleteTemplate(ctx context.Context, call *Call) (any, error) {
	id, _ := call.Args["id"].(string)
	call.Audit.SetTarget(domain.AuditTargetTemplate, id, "")
	var req domain.DeleteTemplateRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// templates.delete validates in its handler, not in the service.
	if _, _, err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	// The repository deletes only a template not yet deleted, and answers not
	// found otherwise, so a repeated call answers not_found without a read.
	if err := s.deps.Templates.DeleteTemplate(ctx, call.WorkspaceID, id); err != nil {
		return nil, err
	}
	return deleteTemplateOutput{Workspace: call.WorkspaceRef(), Deleted: true, ID: id}, nil
}
