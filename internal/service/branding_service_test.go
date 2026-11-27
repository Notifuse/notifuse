package service

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/stretchr/testify/assert"
)

// switchableEntitlements answers whatever grant the test sets, to show that the service asks on
// every call rather than once.
type switchableEntitlements struct{ grant domain.Entitlements }

func (s *switchableEntitlements) Entitlements() domain.Entitlements { return s.grant }

func TestBrandingService_Effective(t *testing.T) {
	stored := branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png", IconURL: "https://cdn.example.com/i.png", HideHelpMenu: true, BackgroundURL: "https://cdn.example.com/bg.jpg"}
	defaults := branding.Resolve(branding.Branding{})

	t.Run("shows the stored brand while an active licence covers it", func(t *testing.T) {
		svc := NewBrandingService(stored, brandingGrant(domain.LicenseStateActive, domain.FeatureBranding))
		assert.Equal(t, branding.Resolve(stored), svc.Effective())
		assert.Equal(t, "Acme", svc.Name())
		assert.Equal(t, "https://cdn.example.com/bg.jpg", svc.Effective().BackgroundURL)
	})

	t.Run("keeps showing it during the grace period", func(t *testing.T) {
		svc := NewBrandingService(stored, brandingGrant(domain.LicenseStateGrace, domain.FeatureBranding))
		assert.True(t, svc.Effective().Custom)
	})

	t.Run("shows Notifuse without a key, with an expired key and with a key lacking the feature", func(t *testing.T) {
		for name, provider := range map[string]domain.EntitlementProvider{
			"no key":              staticEntitlements{entitlements: domain.CommunityEntitlements()},
			"key without feature": brandingGrant(domain.LicenseStateActive, domain.FeatureRBAC, domain.FeatureSSO),
			"nil provider":        nil,
		} {
			t.Run(name, func(t *testing.T) {
				got := NewBrandingService(stored, provider).Effective()
				assert.False(t, got.Custom)
				assert.Equal(t, branding.DefaultName, got.Name)
				assert.Empty(t, got.LogoURL)
				assert.Empty(t, got.IconURL)
				assert.False(t, got.HideHelpMenu, "hiding the Help menu is part of the brand")
				assert.Empty(t, got.BackgroundURL, "the background is part of the brand: the Notifuse photo comes back")
				assert.Equal(t, defaults.SplashLogoSrc, got.SplashLogoSrc)
				assert.Equal(t, defaults.FaviconSrc, got.FaviconSrc)
			})
		}
	})

	t.Run("publishes the stored revision whatever it shows", func(t *testing.T) {
		licensed := NewBrandingService(stored, brandingGrant(domain.LicenseStateActive, domain.FeatureBranding))
		unlicensed := NewBrandingService(stored, nil)
		assert.Equal(t, stored.Revision(), licensed.Effective().Revision)
		assert.Equal(t, stored.Revision(), unlicensed.Effective().Revision)
		assert.Equal(t, stored.Revision(), unlicensed.Revision())
	})

	t.Run("asks the licence on every call, so an expiry shows at once", func(t *testing.T) {
		provider := &switchableEntitlements{grant: domain.Entitlements{State: domain.LicenseStateActive, Features: []domain.Feature{domain.FeatureBranding}}}
		svc := NewBrandingService(stored, provider)
		assert.True(t, svc.Effective().Custom)

		provider.grant = domain.CommunityEntitlements()
		assert.False(t, svc.Effective().Custom)

		provider.grant = domain.Entitlements{State: domain.LicenseStateActive, Features: []domain.Feature{domain.FeatureBranding}}
		assert.Equal(t, "Acme", svc.Name(), "the stored brand comes back untouched")
	})

	t.Run("a nil service answers the Notifuse identity", func(t *testing.T) {
		var svc *BrandingService
		assert.Equal(t, branding.DefaultName, svc.Name())
		assert.Equal(t, defaults, svc.Effective())
		assert.Equal(t, branding.Branding{}.Revision(), svc.Revision())
	})

	t.Run("the help switch alone is shown without a custom identity", func(t *testing.T) {
		svc := NewBrandingService(branding.Branding{HideHelpMenu: true}, brandingGrant(domain.LicenseStateActive, domain.FeatureBranding))
		got := svc.Effective()
		assert.False(t, got.Custom)
		assert.True(t, got.HideHelpMenu)
	})

	t.Run("a background alone is shown without a custom identity, and only with the licence", func(t *testing.T) {
		bgOnly := branding.Branding{BackgroundURL: "https://cdn.example.com/bg.jpg"}
		got := NewBrandingService(bgOnly, brandingGrant(domain.LicenseStateActive, domain.FeatureBranding)).Effective()
		assert.False(t, got.Custom)
		assert.Equal(t, branding.DefaultName, got.Name)
		assert.Equal(t, "https://cdn.example.com/bg.jpg", got.BackgroundURL)

		assert.Empty(t, NewBrandingService(bgOnly, nil).Effective().BackgroundURL)
	})
}
