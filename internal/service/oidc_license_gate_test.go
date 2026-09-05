package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// The SSO gate.
//
// It is the narrowest of the four: it removes the sign-in button and nothing else. No
// console is walled, no data is withheld, no send is stopped, and every user of a
// deployment that loses it can still sign in — resolveOrProvisionUser refuses an identity
// with no email address, so an SSO account is always an ordinary users row with a reachable
// address, and magic-code login knows nothing about federated identities.
//
// Sessions already minted survive: the JWT records user_id, type, session_id and email, and
// not which method issued it. Turning SSO off therefore costs nobody their session — the
// effect appears at the next sign-in, as an absent button.

// gatedOIDCService builds an OIDCService with an explicit entitlement provider. It does not
// extend newTestOIDCService, because a gate whose dependency is optional at the call site is
// how this feature shipped inert the first time.
func gatedOIDCService(t *testing.T, ctrl *gomock.Controller, cfg config.OIDCConfig, ent domain.EntitlementProvider) (*OIDCService, cache.Cache) {
	t.Helper()
	c := cache.NewInMemoryCache(time.Hour)
	svc := NewOIDCService(OIDCServiceConfig{
		UserRepo:              mocks.NewMockUserRepository(ctrl),
		FederatedIdentityRepo: mocks.NewMockFederatedIdentityRepository(ctrl),
		AuthService:           mocks.NewMockAuthService(ctrl),
		OIDCConfig:            cfg,
		SessionExpiry:         30 * 24 * time.Hour,
		ExchangeCache:         c,
		IsRootEmail:           func(string) bool { return false },
		Entitlements:          ent,
		Logger:                logger.NewLogger(),
	})
	return svc, c
}

// provider answers one fixed grant, however often it is asked. AnyTimes rather than a count:
// what matters is the answer, not how many gates consulted it.
func provider(ctrl *gomock.Controller, ent domain.Entitlements) domain.EntitlementProvider {
	p := mocks.NewMockEntitlementProvider(ctrl)
	p.EXPECT().Entitlements().Return(ent).AnyTimes()
	return p
}

func withSSO() domain.Entitlements {
	return domain.Entitlements{
		Tier:          "enterprise",
		MaxWorkspaces: domain.UnlimitedWorkspaces,
		Features:      []domain.Feature{domain.FeatureRBAC, domain.FeatureSESTenant, domain.FeatureSSO},
		State:         domain.LicenseStateActive,
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
	}
}

// A perfectly valid, perfectly paid Studio key. It carries RBAC and SES tenants and does not
// carry SSO, which is the case the gate exists for — not an unlicensed deployment, but a
// licensed one reaching past what it bought.
func withoutSSO() domain.Entitlements {
	return domain.Entitlements{
		Tier:          "studio",
		MaxWorkspaces: 5,
		Features:      []domain.Feature{domain.FeatureRBAC, domain.FeatureSESTenant},
		State:         domain.LicenseStateActive,
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
	}
}

func TestOIDCIsEnabledAsksTheLicence(t *testing.T) {
	disabled := func() config.OIDCConfig { c := enabledCfg(); c.Enabled = false; return c }

	cases := []struct {
		name string
		cfg  config.OIDCConfig
		ent  func(*gomock.Controller) domain.EntitlementProvider
		want bool
	}{
		{
			name: "switched on and licensed for sso",
			cfg:  enabledCfg(),
			ent:  func(c *gomock.Controller) domain.EntitlementProvider { return provider(c, withSSO()) },
			want: true,
		},
		{
			name: "switched on but the licence does not carry sso",
			cfg:  enabledCfg(),
			ent:  func(c *gomock.Controller) domain.EntitlementProvider { return provider(c, withoutSSO()) },
			want: false,
		},
		{
			name: "switched on with no licence at all",
			cfg:  enabledCfg(),
			ent: func(c *gomock.Controller) domain.EntitlementProvider {
				return provider(c, domain.CommunityEntitlements())
			},
			want: false,
		},
		{
			// The licence never switches SSO ON. An Enterprise key on a deployment that never
			// configured an issuer must not make the console offer a button that leads nowhere.
			name: "licensed for sso but switched off",
			cfg:  disabled(),
			ent:  func(c *gomock.Controller) domain.EntitlementProvider { return provider(c, withSSO()) },
			want: false,
		},
		{
			// Grace keeps everything the key ever granted — see entitlementsFrom. A customer
			// whose card is mid-dunning must not lose their logins over it.
			name: "in the grace period after expiry",
			cfg:  enabledCfg(),
			ent: func(c *gomock.Controller) domain.EntitlementProvider {
				ent := withSSO()
				ent.State = domain.LicenseStateGrace
				ent.ExpiresAt = time.Now().UTC().Add(-24 * time.Hour)
				return provider(c, ent)
			},
			want: true,
		},
		{
			// Fail-safe, and the direction is deliberate: an unwired licence subsystem must
			// never be what removes a capability. app_license_wiring_test.go is what keeps
			// this branch from being the production one.
			name: "no provider wired at all",
			cfg:  enabledCfg(),
			ent:  func(*gomock.Controller) domain.EntitlementProvider { return nil },
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc, _ := gatedOIDCService(t, ctrl, tc.cfg, tc.ent(ctrl))
			assert.Equal(t, tc.want, svc.IsEnabled())
		})
	}
}

// The answer moves without a restart, in both directions. cfg.Enabled is resolved once at
// boot, but a key pasted into the console takes effect immediately, and a key that lapses
// stops granting immediately. A snapshot taken at construction would get both wrong.
func TestOIDCIsEnabledFollowsTheLicenceWithoutARestart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	current := domain.CommunityEntitlements()
	p := mocks.NewMockEntitlementProvider(ctrl)
	p.EXPECT().Entitlements().DoAndReturn(func() domain.Entitlements { return current }).AnyTimes()

	svc, _ := gatedOIDCService(t, ctrl, enabledCfg(), p)

	assert.False(t, svc.IsEnabled(), "community: no sso")

	current = withSSO()
	assert.True(t, svc.IsEnabled(), "a key pasted at runtime must reach the gate")

	current = withoutSSO()
	assert.False(t, svc.IsEnabled(), "and a downgrade must reach it too")
}

// Every path that talks to the IdP goes through ensureProvider, which is why the gate sits
// there and not only on IsEnabled.
//
// Asserting the error alone would prove nothing: ErrOIDCNotConfigured is ALSO what an
// unreachable issuer returns, so a test pointed at a hostname that does not resolve passes
// whether the gate is there or not. The assertion that means something is that the issuer was
// never contacted — the gate refuses before a single discovery request leaves the process.
func TestOIDCRefusesBeforeTouchingTheIdPWithoutTheLicence(t *testing.T) {
	reached := func(t *testing.T) (config.OIDCConfig, *int32) {
		t.Helper()
		var hits int32
		idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(idp.Close)

		cfg := enabledCfg()
		cfg.IssuerURL = idp.URL
		return cfg, &hits
	}

	t.Run("BuildAuthURL", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cfg, hits := reached(t)

		svc, _ := gatedOIDCService(t, ctrl, cfg, provider(ctrl, withoutSSO()))

		req, err := svc.BuildAuthURL(context.Background())
		assert.Nil(t, req)
		assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
		assert.Zero(t, atomic.LoadInt32(hits), "the gate must refuse before discovery")
	})

	t.Run("HandleCallback", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cfg, hits := reached(t)

		svc, _ := gatedOIDCService(t, ctrl, cfg, provider(ctrl, withoutSSO()))

		code, err := svc.HandleCallback(context.Background(), domain.OIDCCallbackInput{
			Code:      "irrelevant",
			State:     "s",
			FlowState: domain.OIDCFlowState{State: "s", Nonce: "n", Verifier: "v"},
		})
		assert.Empty(t, code)
		assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
		assert.Zero(t, atomic.LoadInt32(hits), "the gate must refuse before the token exchange")
	})

	// The control. With SSO licensed, the SAME refusal arrives — but only after the issuer
	// has been contacted and answered 404. Without this the two assertions above would still
	// pass if the gate were replaced by something that never talks to an IdP at all.
	t.Run("licensed, the issuer IS contacted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cfg, hits := reached(t)

		svc, _ := gatedOIDCService(t, ctrl, cfg, provider(ctrl, withSSO()))

		_, err := svc.BuildAuthURL(context.Background())
		assert.Error(t, err)
		assert.NotZero(t, atomic.LoadInt32(hits), "a licensed deployment must reach discovery")
	})
}

// A one-time code lives for oidcExchangeTTL. A licence that lapses inside that window must
// close it: otherwise the last sixty seconds of a licence mint sessions that outlive it by
// thirty days.
func TestOIDCExchangeCodeRefusesAfterTheLicenceGoes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	current := withSSO()
	p := mocks.NewMockEntitlementProvider(ctrl)
	p.EXPECT().Entitlements().DoAndReturn(func() domain.Entitlements { return current }).AnyTimes()

	svc, c := gatedOIDCService(t, ctrl, enabledCfg(), p)

	const oneTimeCode = "code-minted-while-licensed"
	c.Set(oidcExchangeKeyPrefix+oneTimeCode, &domain.AuthResponse{Token: "t"}, oidcExchangeTTL)

	current = withoutSSO()

	resp, err := svc.ExchangeCode(context.Background(), oneTimeCode)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)

	// And the refusal must not have consumed it: GetAndDelete comes after the gate, so a
	// licence restored a moment later still redeems the code the customer is holding.
	current = withSSO()
	resp, err = svc.ExchangeCode(context.Background(), oneTimeCode)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "t", resp.Token)
}
