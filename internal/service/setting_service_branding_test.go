package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func brandingGrant(state domain.LicenseState, features ...domain.Feature) staticEntitlements {
	return staticEntitlements{entitlements: domain.Entitlements{
		Tier:          "agency",
		MaxWorkspaces: 15,
		Features:      features,
		State:         state,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}}
}

// installedRepoWithBranding returns an installed instance whose stored brand is the given JSON,
// or no brand row at all when stored is "".
func installedRepoWithBranding(stored string) *MockSettingRepository {
	repo := NewMockSettingRepository()
	repo.settings["is_installed"] = "true"
	if stored != "" {
		repo.settings[InstanceBrandingSettingKey] = stored
	}
	return repo
}

// countingSetRepo records every key a save writes, to assert that a refused save writes nothing.
type countingSetRepo struct {
	*MockSettingRepository
	written []string
}

func (r *countingSetRepo) Set(ctx context.Context, key, value string) error {
	r.written = append(r.written, key)
	return r.MockSettingRepository.Set(ctx, key, value)
}

func saveConfig(name, logo, icon string, hide bool) *SystemConfig {
	return &SystemConfig{
		IsInstalled:            true,
		AuditLogsRetentionDays: domain.DefaultAuditLogRetentionDays,
		BrandingName:           name,
		BrandingLogoURL:        logo,
		BrandingIconURL:        icon,
		BrandingHideHelpMenu:   hide,
	}
}

// withBackground sets the sign-in background of a save built by saveConfig.
func withBackground(cfg *SystemConfig, url string) *SystemConfig {
	cfg.BrandingBackgroundURL = url
	return cfg
}

const (
	storedAcme   = `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","icon_url":"","hide_help_menu":true}`
	acmeLogo     = "https://cdn.example.com/l.png"
	acmeBg       = "https://cdn.example.com/bg.jpg"
	storedAcmeBg = `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","icon_url":"","hide_help_menu":true,"background_url":"https://cdn.example.com/bg.jpg"}`
)

func TestSettingService_GetInstanceBranding(t *testing.T) {
	ctx := context.Background()

	t.Run("an absent row is the default identity", func(t *testing.T) {
		got, err := NewSettingService(installedRepoWithBranding(""), nil).GetInstanceBranding(ctx)
		require.NoError(t, err)
		assert.Equal(t, branding.Branding{}, got)
	})

	t.Run("reads a stored identity", func(t *testing.T) {
		got, err := NewSettingService(installedRepoWithBranding(storedAcme), nil).GetInstanceBranding(ctx)
		require.NoError(t, err)
		assert.Equal(t, branding.Branding{Name: "Acme", LogoURL: acmeLogo, HideHelpMenu: true}, got)
	})

	t.Run("a database error is returned, not read as the default identity", func(t *testing.T) {
		repo := installedRepoWithBranding(storedAcme)
		repo.getError = errors.New("db down")
		_, err := NewSettingService(repo, nil).GetInstanceBranding(ctx)
		require.Error(t, err)
		assert.False(t, errors.Is(err, branding.ErrInvalidStored))
	})

	t.Run("a corrupted row is a typed error", func(t *testing.T) {
		_, err := NewSettingService(installedRepoWithBranding("{not json"), nil).GetInstanceBranding(ctx)
		assert.ErrorIs(t, err, branding.ErrInvalidStored)
	})

	t.Run("reads no other setting, so no SMTP secret is decrypted", func(t *testing.T) {
		repo := installedRepoWithBranding(storedAcme)
		repo.settings["encrypted_smtp_password"] = "not-hex-and-would-fail-to-decrypt"
		_, err := NewSettingService(repo, nil).GetInstanceBranding(ctx)
		assert.NoError(t, err)
	})
}

func TestSettingService_GetSystemConfig_Branding(t *testing.T) {
	ctx := context.Background()

	t.Run("carries the stored identity", func(t *testing.T) {
		cfg, err := NewSettingService(installedRepoWithBranding(storedAcme), nil).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, "Acme", cfg.BrandingName)
		assert.Equal(t, acmeLogo, cfg.BrandingLogoURL)
		assert.Equal(t, "", cfg.BrandingIconURL)
		assert.True(t, cfg.BrandingHideHelpMenu)
		assert.Empty(t, cfg.BrandingBackgroundURL, "a brand saved before the background existed has none")
	})

	t.Run("carries the stored background", func(t *testing.T) {
		cfg, err := NewSettingService(installedRepoWithBranding(storedAcmeBg), nil).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.Equal(t, acmeBg, cfg.BrandingBackgroundURL)
	})

	t.Run("an unreadable row is flagged, and the other settings still load", func(t *testing.T) {
		repo := installedRepoWithBranding("{not json")
		repo.settings["smtp_host"] = "smtp.example.com"
		cfg, err := NewSettingService(repo, nil).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.True(t, cfg.BrandingUnreadable)
		assert.Empty(t, cfg.BrandingName)
		assert.Equal(t, "smtp.example.com", cfg.SMTPHost)
	})

	t.Run("a brand saved under older rules still reads", func(t *testing.T) {
		repo := installedRepoWithBranding(`{"name":"Acme","logo_url":"https://10.0.0.1/l.png"}`)
		cfg, err := NewSettingService(repo, nil).GetSystemConfig(ctx, testSecretKey)
		require.NoError(t, err)
		assert.False(t, cfg.BrandingUnreadable)
		assert.Equal(t, "https://10.0.0.1/l.png", cfg.BrandingLogoURL)
	})
}

func TestSettingService_SetSystemConfig_BrandingRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := installedRepoWithBranding("")
	svc := NewSettingService(repo, brandingGrant(domain.LicenseStateActive, domain.FeatureBranding))

	require.NoError(t, svc.SetSystemConfig(ctx, saveConfig(" Acme ", acmeLogo, "", true), testSecretKey))
	got, err := svc.GetInstanceBranding(ctx)
	require.NoError(t, err)
	assert.Equal(t, branding.Branding{Name: "Acme", LogoURL: acmeLogo, HideHelpMenu: true}, got, "stored normalised")

	assert.JSONEq(t, storedAcme, repo.settings[InstanceBrandingSettingKey], "without a background, the row is written as 45.0 wrote it")

	require.NoError(t, svc.SetSystemConfig(ctx, withBackground(saveConfig("Acme", acmeLogo, "", true), " "+acmeBg+" "), testSecretKey))
	got, err = svc.GetInstanceBranding(ctx)
	require.NoError(t, err)
	assert.Equal(t, acmeBg, got.BackgroundURL, "the background is stored normalised")

	require.NoError(t, svc.SetSystemConfig(ctx, withBackground(saveConfig("", "", "", false), acmeBg), testSecretKey))
	got, err = svc.GetInstanceBranding(ctx)
	require.NoError(t, err)
	assert.Equal(t, branding.Branding{BackgroundURL: acmeBg}, got, "a background needs no name")

	require.NoError(t, svc.SetSystemConfig(ctx, saveConfig("", "", "", false), testSecretKey))
	got, err = svc.GetInstanceBranding(ctx)
	require.NoError(t, err)
	assert.Equal(t, branding.Branding{}, got, "clearing every field is durable")
}

func TestSettingService_SetSystemConfig_BrandingValidation(t *testing.T) {
	ctx := context.Background()
	licensed := brandingGrant(domain.LicenseStateActive, domain.FeatureBranding)

	cases := map[string]*SystemConfig{
		"a logo without a name":   saveConfig("", acmeLogo, "", false),
		"an http logo":            saveConfig("Acme", "http://cdn.example.com/l.png", "", false),
		"an icon on a private IP": saveConfig("Acme", "", "https://10.0.0.1/i.png", false),
		"a name with a newline":   saveConfig("Ac\nme", "", "", false),
		"an http background":      withBackground(saveConfig("", "", "", false), "http://cdn.example.com/bg.jpg"),
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &countingSetRepo{MockSettingRepository: installedRepoWithBranding("")}
			err := NewSettingService(repo, licensed).SetSystemConfig(ctx, cfg, testSecretKey)
			var fe *branding.FieldError
			require.ErrorAs(t, err, &fe)
			assert.Empty(t, repo.written, "validation runs before the first write")
		})
	}
}

func TestSettingService_SetSystemConfig_BrandingLicence(t *testing.T) {
	ctx := context.Background()
	unlicensed := brandingGrant(domain.LicenseStateActive, domain.FeatureRBAC) // a valid key without branding

	refused := []struct {
		name   string
		stored string
		save   *SystemConfig
	}{
		{"setting a name", "", saveConfig("Acme", "", "", false)},
		{"setting a logo", `{"name":"Acme"}`, saveConfig("Acme", acmeLogo, "", false)},
		{"changing the name", storedAcme, saveConfig("Acme Labs", acmeLogo, "", true)},
		{"replacing the logo", storedAcme, saveConfig("Acme", "https://cdn.example.com/m.png", "", true)},
		{"ticking Hide help menu", "", saveConfig("", "", "", true)},
		{"setting a background", "", withBackground(saveConfig("", "", "", false), acmeBg)},
		{"replacing the background", storedAcmeBg, withBackground(saveConfig("Acme", acmeLogo, "", true), "https://cdn.example.com/other.jpg")},
	}
	for _, tc := range refused {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			repo := &countingSetRepo{MockSettingRepository: installedRepoWithBranding(tc.stored)}
			err := NewSettingService(repo, unlicensed).SetSystemConfig(ctx, tc.save, testSecretKey)

			var notLicensed *domain.ErrFeatureNotLicensed
			require.ErrorAs(t, err, &notLicensed)
			assert.Equal(t, domain.FeatureBranding, notLicensed.Feature)
			assert.Equal(t, "Agency", notLicensed.RequiredTier)
			assert.Empty(t, repo.written, "a refused save writes nothing")
		})
	}

	allowed := []struct {
		name   string
		stored string
		save   *SystemConfig
	}{
		{"a save without any brand", "", saveConfig("", "", "", false)},
		{"sending the stored brand back unchanged", storedAcme, saveConfig("Acme", acmeLogo, "", true)},
		{"the same values with surrounding spaces", storedAcme, saveConfig("  Acme ", acmeLogo+" ", "", true)},
		{"clearing the logo and name together", storedAcme, saveConfig("", "", "", true)},
		{"unticking Hide help menu", storedAcme, saveConfig("Acme", acmeLogo, "", false)},
		{"resetting everything", storedAcme, saveConfig("", "", "", false)},
		{"sending the stored background back unchanged", storedAcmeBg, withBackground(saveConfig("Acme", acmeLogo, "", true), acmeBg)},
		{"clearing the background", storedAcmeBg, saveConfig("Acme", acmeLogo, "", true)},
	}
	for _, tc := range allowed {
		t.Run("allows "+tc.name, func(t *testing.T) {
			repo := installedRepoWithBranding(tc.stored)
			require.NoError(t, NewSettingService(repo, unlicensed).SetSystemConfig(ctx, tc.save, testSecretKey))
		})
	}

	t.Run("an SMTP-only save keeps a stored brand without a licence", func(t *testing.T) {
		repo := installedRepoWithBranding(storedAcme)
		cfg := saveConfig("Acme", acmeLogo, "", true)
		cfg.SMTPHost = "smtp.example.com"
		require.NoError(t, NewSettingService(repo, unlicensed).SetSystemConfig(ctx, cfg, testSecretKey))
		assert.Equal(t, "smtp.example.com", repo.settings["smtp_host"])
	})

	t.Run("a grace-period key still allows setting a brand", func(t *testing.T) {
		repo := installedRepoWithBranding("")
		svc := NewSettingService(repo, brandingGrant(domain.LicenseStateGrace, domain.FeatureBranding))
		assert.NoError(t, svc.SetSystemConfig(ctx, saveConfig("Acme", "", "", false), testSecretKey))
	})

	t.Run("an expired key is refused like no key", func(t *testing.T) {
		// An expired key resolves to the community grant: no features.
		repo := installedRepoWithBranding("")
		svc := NewSettingService(repo, staticEntitlements{entitlements: domain.CommunityEntitlements()})
		var notLicensed *domain.ErrFeatureNotLicensed
		assert.ErrorAs(t, svc.SetSystemConfig(ctx, saveConfig("Acme", "", "", false), testSecretKey), &notLicensed)
	})

	t.Run("a nil provider is refused, never granted", func(t *testing.T) {
		repo := installedRepoWithBranding("")
		var notLicensed *domain.ErrFeatureNotLicensed
		assert.ErrorAs(t, NewSettingService(repo, nil).SetSystemConfig(ctx, saveConfig("Acme", "", "", false), testSecretKey), &notLicensed)
	})

	t.Run("over an unreadable row, setting a brand still needs the licence", func(t *testing.T) {
		repo := &countingSetRepo{MockSettingRepository: installedRepoWithBranding("{not json")}
		err := NewSettingService(repo, unlicensed).SetSystemConfig(ctx, saveConfig("Acme", "", "", false), testSecretKey)
		var notLicensed *domain.ErrFeatureNotLicensed
		assert.ErrorAs(t, err, &notLicensed)
		assert.Empty(t, repo.written)
	})

	t.Run("over an unreadable row, clearing replaces it without a licence", func(t *testing.T) {
		repo := installedRepoWithBranding("{not json")
		require.NoError(t, NewSettingService(repo, unlicensed).SetSystemConfig(ctx, saveConfig("", "", "", false), testSecretKey))
		got, err := branding.Decode([]byte(repo.settings[InstanceBrandingSettingKey]))
		require.NoError(t, err)
		assert.Equal(t, branding.Branding{}, got)
	})

	t.Run("KeepStoredBranding leaves the row exactly as it is", func(t *testing.T) {
		repo := installedRepoWithBranding("{not json")
		cfg := saveConfig("", "", "", false)
		cfg.KeepStoredBranding = true
		cfg.SMTPHost = "smtp.example.com"
		require.NoError(t, NewSettingService(repo, unlicensed).SetSystemConfig(ctx, cfg, testSecretKey))
		assert.Equal(t, "{not json", repo.settings[InstanceBrandingSettingKey])
		assert.Equal(t, "smtp.example.com", repo.settings["smtp_host"])
	})

	t.Run("a brand saved under older rules does not block an unrelated save", func(t *testing.T) {
		old := `{"name":"Acme","logo_url":"https://10.0.0.1/l.png","icon_url":"","hide_help_menu":false}`
		repo := installedRepoWithBranding(old)
		cfg := saveConfig("Acme", "https://10.0.0.1/l.png", "", false)
		cfg.SMTPHost = "smtp.example.com"
		require.NoError(t, NewSettingService(repo, unlicensed).SetSystemConfig(ctx, cfg, testSecretKey))
	})
}
