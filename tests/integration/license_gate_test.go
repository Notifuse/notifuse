package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The integration harness straddles the licence boundary. Every other suite runs on the
// enterprise grant tests/testutil/license.go mints, so a gate that closed on the free tier
// would be invisible to all of them, and a grant that quietly stopped verifying would show
// up as a wall of 402s in suites that have nothing to do with licensing. The subtests here
// pin both sides: that the grant is really in force, and that without it the gates the
// unit tests describe fire end-to-end — through the HTTP layer, against a real database,
// with the body the console switches on.
//
// The unlicensed subtests are the regression test for the failure that produced the
// harness licence. Gating shipped with no integration coverage, the suite ran on the free
// tier, and the fourth workspace of seven unrelated suites answered 402.
func TestLicenseGates(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	const rootEmail = "test@example.com" // RootEmail in the harness config

	t.Run("the harness runs on an enterprise grant with every feature and no ceiling", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
			return app.NewApp(cfg)
		})
		defer suite.Cleanup()

		client := suite.APIClient
		rootToken := performCompleteSignInFlow(t, client, rootEmail)
		client.SetToken(rootToken)

		ent := fetchEntitlements(t, client)
		assert.Equal(t, domain.LicenseStateActive, ent.State)
		assert.Equal(t, domain.UnlimitedWorkspaces, ent.MaxWorkspaces)
		for _, f := range []domain.Feature{
			domain.FeatureRBAC,
			domain.FeatureSESTenant,
			domain.FeatureSSO,
			domain.FeatureAuditLogs,
			domain.FeatureTemplateI18n,
		} {
			assert.True(t, ent.Has(f), "the harness licence must carry %q", f)
		}

		// Lifted in fact, not just in the report: one more workspace than the free tier
		// allows, which is what the workspace suites need.
		for i := 0; i <= domain.CommunityMaxWorkspaces; i++ {
			createTestWorkspaceWithToken(t, client, rootToken, fmt.Sprintf("Licensed %d", i))
		}
		count, err := suite.ServerManager.GetApp().GetWorkspaceRepository().CountWorkspaces(context.Background())
		require.NoError(t, err)
		assert.Greater(t, count, domain.CommunityMaxWorkspaces)
	})

	t.Run("an unlicensed server refuses the workspace past the community ceiling and provisions nothing", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, unlicensed(nil))
		defer suite.Cleanup()

		client := suite.APIClient
		rootToken := performCompleteSignInFlow(t, client, rootEmail)
		client.SetToken(rootToken)

		ent := fetchEntitlements(t, client)
		assert.Equal(t, domain.LicenseStateNone, ent.State)
		assert.Equal(t, domain.CommunityMaxWorkspaces, ent.MaxWorkspaces)
		assert.Empty(t, ent.Features, "the free tier reports no features")

		ctx := context.Background()
		repo := suite.ServerManager.GetApp().GetWorkspaceRepository()
		count, err := repo.CountWorkspaces(ctx)
		require.NoError(t, err)
		require.LessOrEqual(t, count, domain.CommunityMaxWorkspaces,
			"a fresh database already sits over the free-tier ceiling")

		// Everything up to the ceiling is allowed.
		for count < domain.CommunityMaxWorkspaces {
			createTestWorkspaceWithToken(t, client, rootToken, fmt.Sprintf("Community %d", count))
			count++
		}

		// The next one is refused with the machine-readable body the console renders a
		// purchase prompt from — not a 403, which has nothing for sale, and not a 500.
		refusedID := "test" + uuid.New().String()[:8]
		resp, err := client.Post("/api/workspaces.create", domain.CreateWorkspaceRequest{
			ID:   refusedID,
			Name: "One past the ceiling",
			Settings: domain.WorkspaceSettings{
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusPaymentRequired, resp.StatusCode)

		var refusal map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&refusal))
		assert.Equal(t, "license_required", refusal["error"])
		assert.Equal(t, "workspaces", refusal["feature"])
		assert.NotEmpty(t, refusal["message"])
		assert.NotEmpty(t, refusal["docs"])

		// Refused means refused: the count is unchanged and the id was never provisioned.
		after, err := repo.CountWorkspaces(ctx)
		require.NoError(t, err)
		assert.Equal(t, domain.CommunityMaxWorkspaces, after)
		_, err = repo.GetByID(ctx, refusedID)
		var notFound *domain.ErrWorkspaceNotFound
		assert.ErrorAs(t, err, &notFound)
	})

	t.Run("an unlicensed server keeps SSO off the sign-in page even with OIDC configured", func(t *testing.T) {
		idp := newFakeOIDP(t)
		suite := testutil.NewIntegrationTestSuite(t, unlicensed(func(cfg *config.Config) {
			cfg.OIDC = config.OIDCConfig{
				Enabled:      true,
				IssuerURL:    idp.server.URL,
				ClientID:     oidcTestClientID,
				ClientSecret: "integration-secret",
				RedirectURI:  "http://localhost/api/user.oidc.callback",
				Scopes:       []string{"openid", "email", "profile"},
				ButtonLabel:  "Sign in with SSO",
			}
		}))
		defer suite.Cleanup()

		// The licensed OIDC suite asserts the mirror image of this line against the same
		// configuration; together they pin that the licence, and only the licence, is
		// what flips it.
		resp, err := http.Get(suite.ServerManager.GetURL() + "/config.js")
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "window.OIDC_ENABLED = false;")

		// And the start of the SSO flow is refused outright, so a bookmarked link cannot
		// bypass the missing button. The client must not follow redirects: a 302 to the
		// IdP is the failure being asserted against, and following it would hide it.
		noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		resp, err = noFollow.Get(suite.ServerManager.GetURL() + "/api/user.oidc.start")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "an unlicensed server must not redirect to the IdP")
	})
}

// unlicensed builds an app on the free tier: the harness licence is blanked before the
// App reads its config, which is exactly the state every suite ran in before the harness
// was licensed. mutate, when non-nil, runs after the blank so a test can layer other
// configuration on top.
func unlicensed(mutate func(*config.Config)) func(*config.Config) testutil.AppInterface {
	return func(cfg *config.Config) testutil.AppInterface {
		cfg.LicenseKey = ""
		if mutate != nil {
			mutate(cfg)
		}
		return app.NewApp(cfg)
	}
}

// fetchEntitlements reads what the server says it is licensed for, through the same
// endpoint the console's banner and gate notices use.
func fetchEntitlements(t *testing.T, client *testutil.APIClient) domain.Entitlements {
	t.Helper()
	var body struct {
		Entitlements domain.Entitlements `json:"entitlements"`
	}
	require.NoError(t, client.GetJSON("/api/licence.get", &body))
	return body.Entitlements
}
