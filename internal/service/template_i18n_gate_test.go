package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// notifuseRoot is the smallest tree EmailTemplate.Validate accepts.
func notifuseRoot() notifuse_mjml.EmailBlock {
	bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
	bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
	rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
	rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

// workspaceWithDutch configures nl so validateTranslationLanguages passes and the only thing
// left to refuse a save is the licence.
func workspaceWithDutch() *domain.Workspace {
	return &domain.Workspace{
		ID:   "ws1",
		Name: "Test",
		Settings: domain.WorkspaceSettings{
			DefaultLanguage: "en",
			Languages:       []string{"en", "nl"},
		},
	}
}

// G5, the template-translations gate.
//
// It refuses authoring a multilingual variant — adding a language, or editing the content of
// one already stored — and nothing else. What it must NOT refuse is the larger half of these
// tests, because that is where a gate on a capability turns into a gate on the product:
// sending, editing a template that merely carries translations, and removing a language are
// all untouched in every licence state.

func communityProvider(ctrl *gomock.Controller) domain.EntitlementProvider {
	p := domainmocks.NewMockEntitlementProvider(ctrl)
	p.EXPECT().Entitlements().Return(domain.CommunityEntitlements()).AnyTimes()
	return p
}

func i18nProvider(ctrl *gomock.Controller) domain.EntitlementProvider {
	p := domainmocks.NewMockEntitlementProvider(ctrl)
	p.EXPECT().Entitlements().Return(domain.Entitlements{
		Tier:          "studio",
		MaxWorkspaces: 5,
		Features:      []domain.Feature{domain.FeatureRBAC, domain.FeatureTemplateI18n},
		State:         domain.LicenseStateActive,
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
	}).AnyTimes()
	return p
}

// gatedTemplateService builds the service with an explicit provider and an authenticated
// owner. Nothing else is stubbed: a test that reaches the repository has passed the gate,
// which is exactly the distinction being asserted.
func gatedTemplateService(t *testing.T, ctrl *gomock.Controller, ent domain.EntitlementProvider) (
	*service.TemplateService, *domainmocks.MockTemplateRepository, *domainmocks.MockWorkspaceRepository,
) {
	t.Helper()

	repo := domainmocks.NewMockTemplateRepository(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	auth := domainmocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()

	auth.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: "ws1", Role: "owner",
			}, nil
		}).AnyTimes()

	svc := service.NewTemplateService(repo, workspaceRepo, auth, log, "https://api.example.com", ent)
	return svc, repo, workspaceRepo
}

func translated(subject string) map[string]domain.TemplateTranslation {
	return map[string]domain.TemplateTranslation{
		"nl": {Email: &domain.EmailTemplate{
			Subject:          subject,
			SenderID:         "sender-1",
			CompiledPreview:  "<p>Test</p>",
			VisualEditorTree: notifuseRoot(),
		}},
	}
}

func stored(translations map[string]domain.TemplateTranslation) *domain.Template {
	return &domain.Template{
		ID:      "tpl1",
		Name:    "Welcome",
		Version: 3,
		Channel: "email",
		Email: &domain.EmailTemplate{
			Subject:          "Welcome",
			SenderID:         "sender-1",
			CompiledPreview:  "<p>Test</p>",
			VisualEditorTree: notifuseRoot(),
		},
		Category:     "welcome",
		Translations: translations,
	}
}

func TestTemplateI18nGate_Create(t *testing.T) {
	t.Run("an unlicensed deployment cannot author a variant", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		svc, repo, _ := gatedTemplateService(t, ctrl, communityProvider(ctrl))
		// No repository expectation: the refusal must happen before anything is written.
		repo.EXPECT().CreateTemplate(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		tmpl := stored(translated("Hallo"))
		err := svc.CreateTemplate(context.Background(), "ws1", tmpl)

		var notLicensed *domain.ErrFeatureNotLicensed
		require.ErrorAs(t, err, &notLicensed)
		assert.Equal(t, domain.FeatureTemplateI18n, notLicensed.Feature)
	})

	// The gate is on the variant, not on templates. An unlicensed deployment keeps every
	// template it can build today.
	t.Run("an unlicensed deployment creates a template with no variant", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		svc, repo, _ := gatedTemplateService(t, ctrl, communityProvider(ctrl))
		repo.EXPECT().CreateTemplate(gomock.Any(), "ws1", gomock.Any()).Return(nil).Times(1)

		require.NoError(t, svc.CreateTemplate(context.Background(), "ws1", stored(nil)))
	})
}

func TestTemplateI18nGate_Update(t *testing.T) {
	// Everything the gate must NOT refuse. This is the half that decides whether it gates a
	// capability or gates the product.
	t.Run("an unlicensed deployment", func(t *testing.T) {
		existing := stored(translated("Hallo"))

		t.Run("edits a template that carries variants, leaving them alone", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc, repo, workspaceRepo := gatedTemplateService(t, ctrl, communityProvider(ctrl))
			repo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl1", int64(0)).Return(existing, nil)
			workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(workspaceWithDutch(), nil).AnyTimes()
			repo.EXPECT().UpdateTemplate(gomock.Any(), "ws1", gomock.Any(), gomock.Any()).Return(nil).Times(1)

			// The console sends the whole template on every save: same variants, new subject.
			incoming := stored(translated("Hallo"))
			incoming.Email.Subject = "Welcome aboard"
			incoming.Version = 0

			require.NoError(t, svc.UpdateTemplate(context.Background(), "ws1", incoming))
		})

		t.Run("removes a variant, which is the way back inside the licence", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc, repo, workspaceRepo := gatedTemplateService(t, ctrl, communityProvider(ctrl))
			repo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl1", int64(0)).Return(existing, nil)
			workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(workspaceWithDutch(), nil).AnyTimes()
			repo.EXPECT().UpdateTemplate(gomock.Any(), "ws1", gomock.Any(), gomock.Any()).Return(nil).Times(1)

			cleared := stored(map[string]domain.TemplateTranslation{})
			cleared.Version = 0

			require.NoError(t, svc.UpdateTemplate(context.Background(), "ws1", cleared))
		})

		t.Run("saves without mentioning translations at all", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc, repo, workspaceRepo := gatedTemplateService(t, ctrl, communityProvider(ctrl))
			repo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl1", int64(0)).Return(existing, nil)
			workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(workspaceWithDutch(), nil).AnyTimes()
			repo.EXPECT().UpdateTemplate(gomock.Any(), "ws1", gomock.Any(), gomock.Any()).Return(nil).Times(1)

			// A nil map preserves what is stored — the gate must not read that as authoring.
			untouched := stored(nil)
			untouched.Version = 0

			require.NoError(t, svc.UpdateTemplate(context.Background(), "ws1", untouched))
		})
	})

	t.Run("an unlicensed deployment cannot edit a variant", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		svc, repo, _ := gatedTemplateService(t, ctrl, communityProvider(ctrl))
		repo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl1", int64(0)).
			Return(stored(translated("Hallo")), nil)
		repo.EXPECT().UpdateTemplate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		edited := stored(translated("Goedendag"))
		edited.Version = 0

		var notLicensed *domain.ErrFeatureNotLicensed
		require.ErrorAs(t, svc.UpdateTemplate(context.Background(), "ws1", edited), &notLicensed)
		assert.Equal(t, domain.FeatureTemplateI18n, notLicensed.Feature)
	})

	t.Run("a licensed deployment authors freely", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		svc, repo, workspaceRepo := gatedTemplateService(t, ctrl, i18nProvider(ctrl))
		repo.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "tpl1", int64(0)).
			Return(stored(nil), nil)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws1").Return(workspaceWithDutch(), nil).AnyTimes()
		repo.EXPECT().UpdateTemplate(gomock.Any(), "ws1", gomock.Any(), gomock.Any()).Return(nil).Times(1)

		added := stored(translated("Hallo"))
		added.Version = 0

		require.NoError(t, svc.UpdateTemplate(context.Background(), "ws1", added))
	})
}

// The send path carries no licence check of any kind, and this is the assertion that says so.
// A deployment whose key lapsed keeps delivering every message in the language it already
// stored: the gate took the ability to author a variant, never the variant itself.
func TestTemplateI18nGate_ResolvingAVariantIsNeverLicensed(t *testing.T) {
	tmpl := stored(translated("Hallo"))

	resolved := tmpl.ResolveEmailContent("nl", "en")
	require.NotNil(t, resolved)
	assert.Equal(t, "Hallo", resolved.Subject,
		"resolving a stored translation must not depend on the licence")
}
