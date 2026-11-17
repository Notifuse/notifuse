package service

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// The demo's templates that the console's template gallery has a model for.
//
// The gallery's models are TypeScript builders (console/src/components/templates/gallery), so
// the console builds them, with the demo's brand, languages, subjects and copy, into
// demo_gallery/*.json (console/src/components/templates/gallery/demo/demoFixtures.ts, run with
// `npm run demo-fixtures`). Its test fails when a committed file differs from what the builders
// build, so the demo keeps the gallery's look without a hand translation to drift.
//
// Order Thank You has no fixture: the gallery's order model reads the order's items, totals and
// address from {{ order }}, which the post-purchase automation's sends do not carry (they hold
// the contact and the purchase event), so it would render an empty order. It keeps the shared
// automation builder (demo_automation_templates.go).

//go:embed demo_gallery/*.json
var demoGalleryFiles embed.FS

// demoGalleryBaseURLPlaceholder stands, in the fixtures, for where the instance serves the
// gallery's images. Not Liquid on purpose: left unreplaced it must show, not render empty.
const demoGalleryBaseURLPlaceholder = "[[notifuse_gallery_base_url]]"

// demoGalleryTemplate is one of the demo's gallery-built templates: what the rest of the demo
// knows it by. The fixture holds its content.
type demoGalleryTemplate struct {
	ID       string
	Name     string
	Category domain.TemplateCategory
}

// The templates createSampleTemplates seeds from the gallery, and the automation templates
// demoAutomationTemplates does. Each ID is the name of its fixture.
var (
	demoGallerySampleTemplates = []demoGalleryTemplate{
		{ID: "newsletter-weekly", Name: "Weekly Newsletter", Category: domain.TemplateCategoryMarketing},
		{ID: "newsletter-weekly-v2", Name: "Weekly Newsletter v2", Category: domain.TemplateCategoryMarketing},
		{ID: "welcome-email", Name: "Welcome Email", Category: domain.TemplateCategoryWelcome},
		{ID: "password-reset", Name: "Password Reset", Category: domain.TemplateCategoryTransactional},
	}
	demoGalleryAutomationTemplates = []demoGalleryTemplate{
		{ID: demoTemplateCartRecoveryA, Name: "Cart Recovery A", Category: domain.TemplateCategoryMarketing},
		{ID: demoTemplateCartRecoveryB, Name: "Cart Recovery B", Category: domain.TemplateCategoryMarketing},
		{ID: demoTemplateWinbackOffer, Name: "Win-back Offer", Category: domain.TemplateCategoryMarketing},
	}
)

// demoGalleryFixture is a fixture file as the console writes it.
type demoGalleryFixture struct {
	TemplateID      string `json:"template_id"`
	Model           string `json:"model"`
	Category        string `json:"category"`
	DefaultLanguage string `json:"default_language"`
	Brand           struct {
		Name      string `json:"name"`
		LogoURL   string `json:"logo_url"`
		LogoWidth int    `json:"logo_width"`
	} `json:"brand"`
	TestData  domain.MapOfAny                    `json:"test_data"`
	Languages map[string]demoGalleryFixtureEmail `json:"languages"`
}

type demoGalleryFixtureEmail struct {
	Subject          string          `json:"subject"`
	SubjectPreview   string          `json:"subject_preview"`
	VisualEditorTree json.RawMessage `json:"visual_editor_tree"`
}

// demoGalleryBaseURL is where an instance serves the gallery's images: its console, at the API
// endpoint recipients reach, as the console's own gallery links them.
func demoGalleryBaseURL(apiEndpoint string) string {
	return strings.TrimRight(apiEndpoint, "/") + "/console/gallery"
}

// loadDemoGalleryFixture reads a template's fixture, its images pointed at baseURL.
func loadDemoGalleryFixture(templateID, baseURL string) (*demoGalleryFixture, error) {
	raw, err := demoGalleryFiles.ReadFile("demo_gallery/" + templateID + ".json")
	if err != nil {
		return nil, fmt.Errorf("demo gallery fixture %s: %w", templateID, err)
	}
	// The URL goes into JSON strings: encoded as one, quotes stripped.
	encoded, err := json.Marshal(baseURL)
	if err != nil {
		return nil, err
	}
	raw = []byte(strings.ReplaceAll(string(raw), demoGalleryBaseURLPlaceholder, string(encoded[1:len(encoded)-1])))

	var fixture demoGalleryFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return nil, fmt.Errorf("demo gallery fixture %s: %w", templateID, err)
	}
	if fixture.TemplateID != templateID {
		return nil, fmt.Errorf("demo gallery fixture %s holds template %q", templateID, fixture.TemplateID)
	}
	if _, ok := fixture.Languages[fixture.DefaultLanguage]; !ok {
		return nil, fmt.Errorf("demo gallery fixture %s has no %q content", templateID, fixture.DefaultLanguage)
	}
	return &fixture, nil
}

// demoGalleryEmail turns one language of a fixture into the template's email, its preview
// compiled from the test data.
func (s *DemoService) demoGalleryEmail(workspaceID, messageID string, email demoGalleryFixtureEmail, testData domain.MapOfAny) (*domain.EmailTemplate, error) {
	tree, err := notifuse_mjml.UnmarshalEmailBlock(email.VisualEditorTree)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", messageID, err)
	}
	preview := email.SubjectPreview
	return &domain.EmailTemplate{
		EditorMode:       domain.EditorModeVisual,
		Subject:          email.Subject,
		SubjectPreview:   &preview,
		CompiledPreview:  s.compileTemplateToHTML(workspaceID, messageID, tree, testData),
		VisualEditorTree: tree,
	}, nil
}

// buildDemoGalleryTemplate builds a demo template from its gallery fixture: the default
// language as the email, the others as its translations.
func (s *DemoService) buildDemoGalleryTemplate(workspaceID string, spec demoGalleryTemplate) (*domain.Template, error) {
	apiEndpoint := ""
	if s.config != nil {
		apiEndpoint = s.config.APIEndpoint
	}
	fixture, err := loadDemoGalleryFixture(spec.ID, demoGalleryBaseURL(apiEndpoint))
	if err != nil {
		return nil, err
	}

	email, err := s.demoGalleryEmail(workspaceID, spec.ID+"-preview", fixture.Languages[fixture.DefaultLanguage], fixture.TestData)
	if err != nil {
		return nil, err
	}
	translations := make(map[string]domain.TemplateTranslation, len(fixture.Languages)-1)
	for language, content := range fixture.Languages {
		if language == fixture.DefaultLanguage {
			continue
		}
		translated, err := s.demoGalleryEmail(workspaceID, spec.ID+"-"+language, content, fixture.TestData)
		if err != nil {
			return nil, err
		}
		translations[language] = domain.TemplateTranslation{Email: translated}
	}

	now := time.Now()
	return &domain.Template{
		ID:           spec.ID,
		Name:         spec.Name,
		Version:      1,
		Channel:      "email",
		Category:     string(spec.Category),
		Email:        email,
		TestData:     fixture.TestData,
		Translations: translations,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}
