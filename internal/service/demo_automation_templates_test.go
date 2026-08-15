package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

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
	return &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
}

// TestDemoAutomationTemplates_AreCreatable guards the templates the demo's automations send.
//
// createAutomationTemplates only warns when CreateTemplate rejects a template, so anything the
// validator refuses — an over-long name above all — produces a demo whose automations point at a
// template that does not exist, with nothing on screen to say so.
func TestDemoAutomationTemplates_AreCreatable(t *testing.T) {
	svc := newAutomationTemplatesTestService()
	templates := svc.demoAutomationTemplates("ws1")
	require.NotEmpty(t, templates)

	fallback := svc.createFallbackHTML()
	byID := map[string]*domain.Template{}

	for _, tmpl := range templates {
		byID[tmpl.ID] = tmpl

		// The whole point: every template must pass the same validation CreateTemplate runs.
		require.NoError(t, tmpl.Validate(), "template %s does not validate", tmpl.ID)

		// Asserted directly rather than through the domain's own limit, because it is the limit
		// itself that is the trap: templates are capped far tighter than lists or automations.
		assert.LessOrEqual(t, len(tmpl.Name), 32, "template %s has a name longer than 32 characters", tmpl.ID)
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

	for _, tmpl := range svc.demoAutomationTemplates("ws1") {
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
			assert.Equal(t, lang, translation.Email.VisualEditorTree.GetAttributes()["lang"],
				"template %s tags its %s tree with the wrong language", tmpl.ID, lang)
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
	for _, tmpl := range svc.demoAutomationTemplates("ws1") {
		want, ok := wantCategory[tmpl.ID]
		require.True(t, ok, "unexpected automation template %s", tmpl.ID)
		assert.Equal(t, string(want), tmpl.Category, "template %s has the wrong category", tmpl.ID)
		seen++
	}
	assert.Equal(t, len(wantCategory), seen, "an automation template is missing")
}

// TestAutomationEmailMJMLStructure_RendersEveryContentField guards the shared builder: it is the one
// place all four templates and all three languages pass through, so a field it forgets to render is
// a field that is silently missing from twelve emails.
func TestAutomationEmailMJMLStructure_RendersEveryContentField(t *testing.T) {
	svc := newAutomationTemplatesTestService()

	contents := automationEmailContents{
		lang:        "en",
		title:       "TITLE-MARKER",
		preview:     "PREVIEW-MARKER",
		heading:     "HEADING-MARKER",
		mainContent: "MAIN-MARKER",
		buttonText:  "BUTTON-MARKER",
		buttonHref:  "https://example.com/href-marker",
		footerText:  "FOOTER-MARKER",
	}

	tree := svc.createAutomationEmailMJMLStructure(contents)
	require.NotNil(t, tree)
	assert.Equal(t, "en", tree.GetAttributes()["lang"])

	html := svc.compileTemplateToHTML("ws1", "builder-test", tree, domain.MapOfAny{})
	require.NotEqual(t, svc.createFallbackHTML(), html, "the shared builder produces uncompilable MJML")

	for _, marker := range []string{
		"TITLE-MARKER",
		"PREVIEW-MARKER",
		"HEADING-MARKER",
		"MAIN-MARKER",
		"BUTTON-MARKER",
		"https://example.com/href-marker",
		"FOOTER-MARKER",
	} {
		assert.Contains(t, html, marker, "the shared builder drops %s", marker)
	}
}

// TestAutomationEmailMJMLStructure_ButtonHrefIsPerTemplate is the reason buttonHref exists as a
// field at all: the welcome builder this one is modelled on hardcodes its destination, and four
// templates sharing one builder must not share one link.
func TestAutomationEmailMJMLStructure_ButtonHrefIsPerTemplate(t *testing.T) {
	svc := newAutomationTemplatesTestService()

	hrefs := map[string]string{}
	for _, tmpl := range svc.demoAutomationTemplates("ws1") {
		href := findAutomationButtonHref(t, tmpl.Email.VisualEditorTree)
		require.NotEmpty(t, href, "template %s has no CTA destination", tmpl.ID)
		assert.True(t, strings.HasPrefix(href, "https://"), "template %s links to %s", tmpl.ID, href)

		if other, clash := hrefs[href]; clash {
			t.Errorf("templates %s and %s send the reader to the same URL %s", other, tmpl.ID, href)
		}
		hrefs[href] = tmpl.ID

		// Every language of a template keeps that template's destination.
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
		),
	}

	// The seed reports success even though a template was rejected — which is why the assertions
	// above exist rather than a test that only checks the returned error.
	require.NoError(t, svc.createAutomationTemplates(context.Background(), "ws1"))

	assert.NotContains(t, created, demoTemplateCartRecoveryA)
	for _, want := range []string{demoTemplateCartRecoveryB, demoTemplateOrderThankYou, demoTemplateWinbackOffer} {
		assert.Contains(t, created, want, "%s was skipped after an earlier template failed", want)
	}
}

// TestDemoAutomationTemplates_CreatesEveryTemplate asserts the happy path reaches the repository for
// each definition, since createAutomationTemplates returns nil either way.
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
