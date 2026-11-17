package service

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const demoGalleryTestEndpoint = "https://api.example.com/"

func demoGalleryTestService() *DemoService {
	return &DemoService{logger: logger.NewLoggerWithLevel("disabled"), config: &config.Config{APIEndpoint: demoGalleryTestEndpoint}}
}

func allDemoGalleryTemplates() []demoGalleryTemplate {
	return append(append([]demoGalleryTemplate{}, demoGallerySampleTemplates...), demoGalleryAutomationTemplates...)
}

// templateJSON is everything a template carries, as one string to search.
func templateJSON(t *testing.T, tmpl *domain.Template) string {
	t.Helper()
	raw, err := json.Marshal(tmpl)
	require.NoError(t, err)
	return string(raw)
}

// Every fixture is seeded, and every gallery template has its fixture: a fixture left behind
// by a template the demo dropped would be regenerated forever for nothing.
func TestDemoGalleryTemplates_OneFixturePerTemplate(t *testing.T) {
	files, err := fs.Glob(demoGalleryFiles, "demo_gallery/*.json")
	require.NoError(t, err)
	var fixtures, templates []string
	for _, file := range files {
		fixtures = append(fixtures, strings.TrimSuffix(strings.TrimPrefix(file, "demo_gallery/"), ".json"))
	}
	for _, spec := range allDemoGalleryTemplates() {
		templates = append(templates, spec.ID)
	}
	sort.Strings(fixtures)
	sort.Strings(templates)
	assert.Equal(t, templates, fixtures)
}

// The console builds the fixtures for the demo workspace as createDemoWorkspace creates it: its
// name in the copy, its logo in the header, its languages.
func TestDemoGalleryTemplates_AreBuiltForTheDemoWorkspace(t *testing.T) {
	for _, spec := range allDemoGalleryTemplates() {
		fixture, err := loadDemoGalleryFixture(spec.ID, "https://x")
		require.NoError(t, err)
		assert.Equal(t, demoWorkspaceName, fixture.Brand.Name, spec.ID)
		assert.Equal(t, demoWorkspaceLogoURL, fixture.Brand.LogoURL, spec.ID)
		assert.Equal(t, domain.DefaultLanguageCode, fixture.DefaultLanguage, spec.ID)
		var languages []string
		for language := range fixture.Languages {
			languages = append(languages, language)
		}
		assert.ElementsMatch(t, demoWorkspaceLanguages, languages, spec.ID)
		// The category the gallery's model gives is the one the demo creates the template with.
		assert.Equal(t, string(spec.Category), fixture.Category, spec.ID)
	}
}

// Each template, in each language, compiles through the real compiler with its test data, and
// its images are served by the instance: no placeholder survives, in the tree or the test data.
func TestDemoGalleryTemplates_CompileWithTheirImagesOnTheInstance(t *testing.T) {
	svc := demoGalleryTestService()
	for _, spec := range allDemoGalleryTemplates() {
		tmpl, err := svc.buildDemoGalleryTemplate("ws1", spec)
		require.NoError(t, err)
		require.NoError(t, tmpl.Validate(), spec.ID)

		all := templateJSON(t, tmpl)
		assert.NotContains(t, all, demoGalleryBaseURLPlaceholder, spec.ID)
		assert.NotContains(t, all, "notifuse_gallery_base_url", spec.ID)
		assert.NotRegexp(t, `\bAcme\b`, all, spec.ID)
		assert.Contains(t, all, demoWorkspaceLogoURL, spec.ID)

		emails := map[string]*domain.EmailTemplate{"en": tmpl.Email}
		for language, translation := range tmpl.Translations {
			emails[language] = translation.Email
		}
		require.Len(t, emails, len(demoWorkspaceLanguages), spec.ID)
		for language, email := range emails {
			assert.Equal(t, domain.EditorModeVisual, email.EditorMode)
			require.NotNil(t, email.SubjectPreview)
			assert.Equal(t, *email.SubjectPreview, *mjPreviewText(email.VisualEditorTree), "%s (%s)", spec.ID, language)

			resp, err := notifuse_mjml.CompileTemplate(notifuse_mjml.CompileTemplateRequest{
				WorkspaceID:      "ws1",
				MessageID:        spec.ID + "-" + language,
				VisualEditorTree: email.VisualEditorTree,
				TemplateData:     notifuse_mjml.MapOfAny(tmpl.TestData),
			})
			require.NoError(t, err, "%s (%s)", spec.ID, language)
			require.True(t, resp.Success, "%s (%s): %v", spec.ID, language, resp.Error)
			require.NotNil(t, resp.HTML)
			assert.NotEqual(t, svc.createFallbackHTML(), email.CompiledPreview, "%s (%s)", spec.ID, language)

			for _, src := range regexp.MustCompile(`<img[^>]+src="([^"]+)"`).FindAllStringSubmatch(*resp.HTML, -1) {
				if src[1] == demoWorkspaceLogoURL {
					continue
				}
				assert.True(t, strings.HasPrefix(src[1], "https://api.example.com/console/gallery/"),
					"%s (%s) shows an image from %s", spec.ID, language, src[1])
			}
		}
	}
}

// liquidRoots lists the template data a Liquid text reads: {{ x… }}, {% if x… %}, {% for i in x %}
// and an unquoted default: x.
var liquidRootPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\{\{-?\s*([A-Za-z_]\w*)`),
	regexp.MustCompile(`\{%-?\s*(?:if|elsif|unless)\s+([A-Za-z_]\w*)`),
	regexp.MustCompile(`\{%-?\s*for\s+\w+\s+in\s+([A-Za-z_]\w*)`),
	regexp.MustCompile(`default:\s*([A-Za-z_]\w*)`),
}

func liquidRoots(text string) map[string]bool {
	roots := map[string]bool{}
	for _, pattern := range liquidRootPatterns {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			roots[match[1]] = true
		}
	}
	return roots
}

// Every variable a template reads is one the demo's messages give it. The seeded messages carry
// the contact (and the reset link for the password reset); a preview adds the workspace, a real
// send to a list adds the list, its links and the notification center; an automation adds the
// event that started it. What only a real send has is read with a fallback or behind a guard.
func TestDemoGalleryTemplates_ReadOnlyWhatTheDemoProvides(t *testing.T) {
	svc := demoGalleryTestService()
	everywhere := []string{"contact", "workspace", "notification_center_url", "item"}
	provided := map[string][]string{
		"newsletter-weekly":       {"list", "unsubscribe_url"},
		"newsletter-weekly-v2":    {"list", "unsubscribe_url"},
		"welcome-email":           {"list", "unsubscribe_url"},
		"password-reset":          {"reset_url"},
		demoTemplateCartRecoveryA: {"list", "unsubscribe_url", "trigger_event"},
		demoTemplateCartRecoveryB: {"list", "unsubscribe_url", "trigger_event"},
		demoTemplateWinbackOffer:  {"list", "unsubscribe_url"},
	}
	listNameWithoutFallback := regexp.MustCompile(`\{\{-?\s*list\.name\s*-?\}\}`)

	for _, spec := range allDemoGalleryTemplates() {
		allowed, ok := provided[spec.ID]
		require.True(t, ok, "say what the sends of %s carry", spec.ID)
		allowed = append(allowed, everywhere...)

		tmpl, err := svc.buildDemoGalleryTemplate("ws1", spec)
		require.NoError(t, err)
		emails := []*domain.EmailTemplate{tmpl.Email}
		for _, translation := range tmpl.Translations {
			emails = append(emails, translation.Email)
		}
		for _, email := range emails {
			text := email.Subject + *email.SubjectPreview + notifuse_mjml.ConvertJSONToMJML(email.VisualEditorTree)
			for root := range liquidRoots(text) {
				assert.Contains(t, allowed, root, "%s reads %s, which its sends do not carry", spec.ID, root)
			}
			assert.NotRegexp(t, listNameWithoutFallback, text, "%s names the list without a fallback: the seeded messages carry none", spec.ID)
		}
	}
}

// A seeded message holds the contact only. Rendered with that, a list email names the demo's
// list rather than "subscribed to .".
func TestDemoGalleryTemplates_RenderFromASeededMessage(t *testing.T) {
	svc := demoGalleryTestService()
	for _, spec := range allDemoGalleryTemplates() {
		if spec.Category == domain.TemplateCategoryTransactional {
			continue
		}
		tmpl, err := svc.buildDemoGalleryTemplate("ws1", spec)
		require.NoError(t, err)
		resp, err := notifuse_mjml.CompileTemplate(notifuse_mjml.CompileTemplateRequest{
			WorkspaceID:      "ws1",
			MessageID:        "m1",
			VisualEditorTree: tmpl.Email.VisualEditorTree,
			TemplateData: notifuse_mjml.MapOfAny{
				"contact": map[string]interface{}{"email": "ana@example.com", "first_name": "Ana", "last_name": "Diaz"},
			},
		})
		require.NoError(t, err)
		require.True(t, resp.Success, "%s: %v", spec.ID, resp.Error)
		assert.Contains(t, *resp.HTML, "subscribed to Newsletter", spec.ID)
		assert.Contains(t, *resp.HTML, "Ana", spec.ID)
	}
}

// The broadcasts, the transactional notifications and their seeded messages send templates the
// seed creates.
func TestDemoSends_FindTheirTemplates(t *testing.T) {
	seeded := demoSeededTemplateIDs()
	for _, campaign := range demoBroadcastCampaigns {
		for _, id := range campaign.Templates {
			assert.True(t, seeded[id], "broadcast %s sends %s, which the demo does not create", campaign.Name, id)
		}
	}
	for _, campaign := range demoTransactionalCampaigns {
		assert.True(t, seeded[campaign.templateID], "the seeded %s messages send %s, which the demo does not create", campaign.messageType, campaign.templateID)
	}
	for _, id := range []string{"password-reset", "welcome-email"} {
		assert.True(t, seeded[id], "a transactional notification sends %s", id)
	}
}

// The gallery's images are served by the instance's console, at its API endpoint.
func TestDemoGalleryBaseURL(t *testing.T) {
	assert.Equal(t, "https://api.example.com/console/gallery", demoGalleryBaseURL("https://api.example.com/"))
	assert.Equal(t, "https://api.example.com/console/gallery", demoGalleryBaseURL("https://api.example.com"))
}

func TestLoadDemoGalleryFixture_RejectsAnUnknownTemplate(t *testing.T) {
	_, err := loadDemoGalleryFixture("no-such-template", "https://x")
	assert.Error(t, err)
}
