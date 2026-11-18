package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"unicode/utf8"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateIDCharset mirrors the charset domain.validateTemplateID enforces. It is spelled out here
// rather than imported so a loosening of the domain pattern does not silently loosen this guard.
var templateIDCharset = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func newAutomationTemplatesTestService() *DemoService {
	return &DemoService{logger: logger.NewLoggerWithLevel("disabled"), config: &config.Config{APIEndpoint: "https://api.example.com"}}
}

// demoAutomationTemplatesOf builds the automation templates, failing the test on an error.
func demoAutomationTemplatesOf(t *testing.T, svc *DemoService) []*domain.Template {
	t.Helper()
	templates, err := svc.demoAutomationTemplates("ws1")
	require.NoError(t, err)
	return templates
}

// TestDemoAutomationTemplates_AreCreatable guards the templates the demo's automations send.
//
// createAutomationTemplates only warns when CreateTemplate rejects a template, so anything the
// validator refuses — an over-long name above all — produces a demo whose automations point at a
// template that does not exist, with nothing on screen to say so.
func TestDemoAutomationTemplates_AreCreatable(t *testing.T) {
	svc := newAutomationTemplatesTestService()
	templates := demoAutomationTemplatesOf(t, svc)
	require.NotEmpty(t, templates)

	fallback := svc.createFallbackHTML()
	byID := map[string]*domain.Template{}

	for _, tmpl := range templates {
		byID[tmpl.ID] = tmpl

		// The whole point: every template must pass the same validation CreateTemplate runs.
		require.NoError(t, tmpl.Validate(), "template %s does not validate", tmpl.ID)

		// Asserted directly rather than through the domain's own limit, so a loosening of the
		// domain rule does not silently loosen this guard.
		assert.LessOrEqual(t, utf8.RuneCountInString(tmpl.Name), 255, "template %s has a name longer than 255 characters", tmpl.ID)
		assert.NotEmpty(t, tmpl.Name, "template %s has no name", tmpl.ID)

		assert.LessOrEqual(t, len(tmpl.ID), 32, "template id %s is longer than 32 characters", tmpl.ID)
		assert.Regexp(t, templateIDCharset, tmpl.ID, "template id %s is outside the accepted charset", tmpl.ID)

		require.NotNil(t, tmpl.Email, "template %s has no email content", tmpl.ID)
		assert.NotEmpty(t, tmpl.Email.Subject, "template %s has no subject", tmpl.ID)
		require.NotNil(t, tmpl.Email.VisualEditorTree, "template %s has no visual editor tree", tmpl.ID)

		// A compile failure is swallowed by compileTemplateToHTML, which returns the placeholder
		// instead — so a broken MJML tree ships as a template that renders "This is a demo email
		// template." to every recipient.
		assert.NotEmpty(t, tmpl.Email.CompiledPreview, "template %s compiled to nothing", tmpl.ID)
		assert.NotEqual(t, fallback, tmpl.Email.CompiledPreview,
			"template %s fell back to the placeholder HTML", tmpl.ID)
		assert.NotContains(t, tmpl.Email.CompiledPreview, "This is a demo email template.",
			"template %s fell back to the placeholder HTML", tmpl.ID)
	}

	for _, want := range []string{
		demoTemplateCartRecoveryA,
		demoTemplateCartRecoveryB,
		demoTemplateOrderThankYou,
		demoTemplateWinbackOffer,
	} {
		assert.Contains(t, byID, want, "the automations reference %s, so the demo has to create it", want)
	}
	assert.Len(t, byID, len(templates), "two automation templates share an id")
}

// TestDemoAutomationTemplates_Translations pins the language set. "en" lives on the top-level Email
// field, so listing it among the translations too would create a duplicate the console then shows as
// a second English tab.
func TestDemoAutomationTemplates_Translations(t *testing.T) {
	svc := newAutomationTemplatesTestService()
	fallback := svc.createFallbackHTML()

	for _, tmpl := range demoAutomationTemplatesOf(t, svc) {
		require.Contains(t, tmpl.Translations, "fr", "template %s has no French translation", tmpl.ID)
		require.Contains(t, tmpl.Translations, "es", "template %s has no Spanish translation", tmpl.ID)
		assert.NotContains(t, tmpl.Translations, "en",
			"template %s duplicates English as a translation of itself", tmpl.ID)
		assert.Len(t, tmpl.Translations, 2, "template %s carries an unexpected language", tmpl.ID)

		for lang, translation := range tmpl.Translations {
			require.NotNil(t, translation.Email, "template %s has empty %s content", tmpl.ID, lang)
			assert.NotEmpty(t, translation.Email.Subject,
				"template %s has no %s subject", tmpl.ID, lang)
			assert.NotEqual(t, tmpl.Email.Subject, translation.Email.Subject,
				"template %s reuses the English subject for %s", tmpl.ID, lang)
			assert.NotEqual(t, fallback, translation.Email.CompiledPreview,
				"template %s fell back to the placeholder HTML in %s", tmpl.ID, lang)
			require.NotNil(t, translation.Email.VisualEditorTree,
				"template %s has no %s tree", tmpl.ID, lang)
			// The tree is the translated one: its preview text is the language's, not English.
			assert.NotEqual(t, mjPreviewText(tmpl.Email.VisualEditorTree), mjPreviewText(translation.Email.VisualEditorTree),
				"template %s carries the English body in %s", tmpl.ID, lang)
		}
	}
}

// TestDemoAutomationTemplates_Categories pins the category split. Marketing and blog templates are
// the only ones the automation executor's subscription guard covers, so the three promotional
// templates have to be marketing — and the order confirmation has to not be, or an unsubscribed
// customer would never learn their order shipped.
func TestDemoAutomationTemplates_Categories(t *testing.T) {
	svc := newAutomationTemplatesTestService()

	wantCategory := map[string]domain.TemplateCategory{
		demoTemplateCartRecoveryA: domain.TemplateCategoryMarketing,
		demoTemplateCartRecoveryB: domain.TemplateCategoryMarketing,
		demoTemplateWinbackOffer:  domain.TemplateCategoryMarketing,
		demoTemplateOrderThankYou: domain.TemplateCategoryTransactional,
	}

	seen := 0
	for _, tmpl := range demoAutomationTemplatesOf(t, svc) {
		want, ok := wantCategory[tmpl.ID]
		require.True(t, ok, "unexpected automation template %s", tmpl.ID)
		assert.Equal(t, string(want), tmpl.Category, "template %s has the wrong category", tmpl.ID)
		seen++
	}
	assert.Equal(t, len(wantCategory), seen, "an automation template is missing")
}

// TestDemoAutomationTemplates_EveryLanguageKeepsItsDestination: each template has a call to
// action, and its translations send the reader to the same place.
func TestDemoAutomationTemplates_EveryLanguageKeepsItsDestination(t *testing.T) {
	svc := newAutomationTemplatesTestService()

	for _, tmpl := range demoAutomationTemplatesOf(t, svc) {
		href := findAutomationButtonHref(t, tmpl.Email.VisualEditorTree)
		require.NotEmpty(t, href, "template %s has no CTA destination", tmpl.ID)
		for lang, translation := range tmpl.Translations {
			assert.Equal(t, href, findAutomationButtonHref(t, translation.Email.VisualEditorTree),
				"template %s links somewhere else in %s", tmpl.ID, lang)
		}
	}
}

// TestDemoAutomationTemplates_CreateContinuesAfterFailure pins the log-and-continue convention: one
// rejected template must not cost the demo the other three.
func TestDemoAutomationTemplates_CreateContinuesAfterFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)

	mockAuth.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws1").
		DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
				UserID:      "user1",
				WorkspaceID: workspaceID,
				Role:        "owner",
				Permissions: domain.FullPermissions,
			}, nil
		}).
		AnyTimes()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(&domain.Workspace{
			ID:       "ws1",
			Settings: domain.WorkspaceSettings{DefaultLanguage: "en", Languages: []string{"en", "fr", "es"}},
		}, nil).
		AnyTimes()

	var created []string
	mockRepo.EXPECT().
		CreateTemplate(gomock.Any(), "ws1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, tmpl *domain.Template) error {
			if tmpl.ID == demoTemplateCartRecoveryA {
				return errors.New("boom")
			}
			created = append(created, tmpl.ID)
			return nil
		}).
		AnyTimes()

	svc := &DemoService{
		logger: logger.NewLoggerWithLevel("disabled"),
		templateService: NewTemplateService(
			mockRepo,
			mockWorkspaceRepo,
			mockAuth,
			logger.NewLoggerWithLevel("disabled"),
			"https://api.example.com",
			fullyLicensedProvider(ctrl),
		),
	}

	// A rejected template is the seed's failure, not a footnote: the demo is seeded on top
	// of these IDs, and a reset that reports success with one missing hands the operator a
	// workspace whose automations point at nothing. The error names the template.
	err := svc.createAutomationTemplates(context.Background(), "ws1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), demoTemplateCartRecoveryA)
	assert.ErrorContains(t, err, "boom")
	assert.NotContains(t, created, demoTemplateCartRecoveryA)
}

// TestDemoAutomationTemplates_CreatesEveryTemplate asserts the happy path reaches the repository for
// each definition.
func TestDemoAutomationTemplates_CreatesEveryTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)

	mockAuth.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws1").
		DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
				UserID:      "user1",
				WorkspaceID: workspaceID,
				Role:        "owner",
				Permissions: domain.FullPermissions,
			}, nil
		}).
		AnyTimes()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "ws1").
		Return(&domain.Workspace{
			ID:       "ws1",
			Settings: domain.WorkspaceSettings{DefaultLanguage: "en", Languages: []string{"en", "fr", "es"}},
		}, nil).
		AnyTimes()

	var created []string
	mockRepo.EXPECT().
		CreateTemplate(gomock.Any(), "ws1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, tmpl *domain.Template) error {
			created = append(created, tmpl.ID)
			return nil
		}).
		AnyTimes()

	svc := &DemoService{
		logger: logger.NewLoggerWithLevel("disabled"),
		templateService: NewTemplateService(
			mockRepo,
			mockWorkspaceRepo,
			mockAuth,
			logger.NewLoggerWithLevel("disabled"),
			"https://api.example.com",
			fullyLicensedProvider(ctrl),
		),
	}

	require.NoError(t, svc.createAutomationTemplates(context.Background(), "ws1"))

	assert.ElementsMatch(t, []string{
		demoTemplateCartRecoveryA,
		demoTemplateCartRecoveryB,
		demoTemplateOrderThankYou,
		demoTemplateWinbackOffer,
	}, created)
}

// findAutomationButtonHref walks an MJML tree and returns the first button's href.
func findAutomationButtonHref(t *testing.T, block notifuse_mjml.EmailBlock) string {
	t.Helper()
	if block == nil {
		return ""
	}
	if block.GetType() == notifuse_mjml.MJMLComponentMjButton {
		if href, ok := block.GetAttributes()["href"].(string); ok {
			return href
		}
		return ""
	}
	for _, child := range block.GetChildren() {
		if href := findAutomationButtonHref(t, child); href != "" {
			return href
		}
	}
	return ""
}

// TestDemoTemplates_WriteEachAttributeOnce guards the editor's attribute keys. A block made by
// NewBaseBlock starts with camelCase defaults (fontSize); a kebab-case key set next to them
// (font-size) is a second attribute, and the MJML then carries both — font-size="24px" …
// font-size="14px" — which each renderer and each preview resolves its own way.
func TestDemoTemplates_WriteEachAttributeOnce(t *testing.T) {
	svc := newAutomationTemplatesTestService()
	trees := map[string]notifuse_mjml.EmailBlock{}
	for _, spec := range append(append([]demoGalleryTemplate{}, demoGallerySampleTemplates...), demoGalleryAutomationTemplates...) {
		tmpl, err := svc.buildDemoGalleryTemplate("ws1", spec)
		require.NoError(t, err)
		trees[spec.ID] = tmpl.Email.VisualEditorTree
	}
	tagPattern := regexp.MustCompile(`<mj-[a-z-]+((?:\s+[a-z-]+="[^"]*")*)\s*/?>`)
	attrPattern := regexp.MustCompile(`\s([a-z-]+)="`)

	for name, tree := range trees {
		mjml := notifuse_mjml.ConvertJSONToMJML(tree)
		for _, tag := range tagPattern.FindAllStringSubmatch(mjml, -1) {
			seen := map[string]bool{}
			for _, attr := range attrPattern.FindAllStringSubmatch(tag[1], -1) {
				assert.False(t, seen[attr[1]], "%s: %q is written twice in %s", name, attr[1], tag[0])
				seen[attr[1]] = true
			}
		}
	}
}
