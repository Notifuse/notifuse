package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureIsValid(t *testing.T) {
	testCases := []struct {
		name    string
		feature Feature
		want    bool
	}{
		{"rbac is a feature this build gates", FeatureRBAC, true},
		{"ses_tenant is a feature this build gates", FeatureSESTenant, true},
		{"sso is a feature this build gates", FeatureSSO, true},
		{"audit_logs is a feature this build gates", FeatureAuditLogs, true},
		{"a feature name this build does not know is rejected", Feature("web_analytics"), false},
		{"the empty feature is rejected", Feature(""), false},
		{"a known name in the wrong case is rejected", Feature("RBAC"), false},
		{"a known name with surrounding space is rejected", Feature(" rbac"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.feature.IsValid())
		})
	}
}

func TestFeatureWireStrings(t *testing.T) {
	// These strings travel inside signed keys that are already in customers' hands and
	// cannot be re-minted without a phone-home. Renaming one silently un-licenses every
	// key carrying it, so the values are pinned here rather than left to a refactor.
	t.Run("the wire value of every feature is frozen", func(t *testing.T) {
		assert.Equal(t, "rbac", string(FeatureRBAC))
		assert.Equal(t, "ses_tenant", string(FeatureSESTenant))
		assert.Equal(t, "sso", string(FeatureSSO))
		assert.Equal(t, "audit_logs", string(FeatureAuditLogs))
	})

	t.Run("the wire value of every licence state is frozen", func(t *testing.T) {
		assert.Equal(t, "none", string(LicenseStateNone))
		assert.Equal(t, "active", string(LicenseStateActive))
		assert.Equal(t, "grace", string(LicenseStateGrace))
		assert.Equal(t, "expired", string(LicenseStateExpired))
	})
}

func TestEntitlementsHas(t *testing.T) {
	granted := Entitlements{
		State:    LicenseStateActive,
		Features: []Feature{FeatureRBAC, FeatureSESTenant},
	}

	t.Run("reports a feature the licence grants", func(t *testing.T) {
		assert.True(t, granted.Has(FeatureRBAC))
		assert.True(t, granted.Has(FeatureSESTenant))
	})

	t.Run("reports false for a feature the licence omits", func(t *testing.T) {
		assert.False(t, granted.Has(FeatureSSO))
		assert.False(t, granted.Has(FeatureAuditLogs))
	})

	t.Run("reports false for a feature name this build does not know", func(t *testing.T) {
		assert.False(t, granted.Has(Feature("web_analytics")))
		assert.False(t, granted.Has(Feature("")))
	})

	t.Run("reports false on a nil feature slice", func(t *testing.T) {
		empty := Entitlements{State: LicenseStateActive}
		require.Nil(t, empty.Features)

		assert.False(t, empty.Has(FeatureRBAC))
		assert.False(t, empty.Has(FeatureSSO))
	})

	t.Run("grants nothing on the zero value", func(t *testing.T) {
		// The zero Entitlements is never a valid answer, and the reason it must not be
		// handed out is that a gate reading it has to stay shut: absence restricts.
		var zero Entitlements

		assert.False(t, zero.Has(FeatureRBAC))
		assert.False(t, zero.Has(FeatureSESTenant))
		assert.False(t, zero.Has(FeatureSSO))
		assert.False(t, zero.Has(FeatureAuditLogs))
		assert.False(t, zero.Licensed())
	})
}

func TestEntitlementsLicensed(t *testing.T) {
	testCases := []struct {
		name  string
		state LicenseState
		want  bool
	}{
		{"an installation with no key is not licensed", LicenseStateNone, false},
		{"an active key is licensed", LicenseStateActive, true},
		{"a key inside its grace period is still licensed", LicenseStateGrace, true},
		{"a key past its grace period is no longer licensed", LicenseStateExpired, false},
		{"a state this build does not know is not licensed", LicenseState("frozen"), false},
		{"the empty state is not licensed", LicenseState(""), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ent := Entitlements{State: tc.state}
			assert.Equal(t, tc.want, ent.Licensed())
		})
	}

	t.Run("says nothing about which features are granted", func(t *testing.T) {
		// Licensed answers the console's "is there a key here"; a gate must still ask Has,
		// because an active key can be licensed and carry no right to the feature at hand.
		studio := Entitlements{
			State:    LicenseStateActive,
			Features: []Feature{FeatureRBAC, FeatureSESTenant},
		}

		require.True(t, studio.Licensed())
		assert.False(t, studio.Has(FeatureSSO))
	})
}

func TestCommunityEntitlements(t *testing.T) {
	t.Run("grants three workspaces and no features", func(t *testing.T) {
		ent := CommunityEntitlements()

		assert.Equal(t, 3, ent.MaxWorkspaces)
		assert.Equal(t, CommunityMaxWorkspaces, ent.MaxWorkspaces)
		assert.Empty(t, ent.Features)
		assert.Equal(t, LicenseStateNone, ent.State)
		assert.False(t, ent.Licensed())

		// Restated one by one rather than ranged over a package list, so this keeps
		// failing if a feature is ever defaulted on for unlicensed installations.
		assert.False(t, ent.Has(FeatureRBAC))
		assert.False(t, ent.Has(FeatureSESTenant))
		assert.False(t, ent.Has(FeatureSSO))
		assert.False(t, ent.Has(FeatureAuditLogs))
	})

	t.Run("carries no licensee identity and no expiry", func(t *testing.T) {
		ent := CommunityEntitlements()

		assert.Empty(t, ent.Tier)
		assert.Empty(t, ent.Org)
		assert.Empty(t, ent.Sub)
		assert.True(t, ent.ExpiresAt.IsZero())
	})

	t.Run("returns a fresh value that a caller cannot corrupt for the next one", func(t *testing.T) {
		// FullPermissions is the cautionary tale: a package-level collection handed out by
		// reference was mutated by a caller and stayed mutated for the whole process.
		first := CommunityEntitlements()
		first.Features = append(first.Features, FeatureSSO, FeatureAuditLogs)
		first.MaxWorkspaces = 99
		first.State = LicenseStateActive

		second := CommunityEntitlements()

		assert.Empty(t, second.Features)
		assert.False(t, second.Has(FeatureSSO))
		assert.False(t, second.Has(FeatureAuditLogs))
		assert.Equal(t, CommunityMaxWorkspaces, second.MaxWorkspaces)
		assert.Equal(t, LicenseStateNone, second.State)
	})

	t.Run("marshals its feature list as an empty array rather than null", func(t *testing.T) {
		// The console reads this off /api/user.me and iterates the field; null would make
		// the banner's own code the thing that breaks on an unlicensed installation.
		encoded, err := json.Marshal(CommunityEntitlements())
		require.NoError(t, err)

		assert.Contains(t, string(encoded), `"features":[]`)
	})
}

func TestUnlimitedWorkspaces(t *testing.T) {
	t.Run("is negative so an unfilled quota never reads as unlimited", func(t *testing.T) {
		// Zero is what a struct holds when nobody filled it in. A sentinel of zero would
		// turn a forgotten field into an unlimited grant, which is absence widening.
		assert.Equal(t, -1, UnlimitedWorkspaces)
		assert.NotEqual(t, 0, UnlimitedWorkspaces)
	})
}

func TestErrFeatureNotLicensed(t *testing.T) {
	t.Run("Error returns the message the console shows", func(t *testing.T) {
		err := &ErrFeatureNotLicensed{
			Feature:      FeatureSSO,
			RequiredTier: "Enterprise",
			Message:      "Single sign-on requires a Notifuse Enterprise licence.",
		}

		assert.Equal(t, "Single sign-on requires a Notifuse Enterprise licence.", err.Error())
	})

	testCases := []struct {
		name         string
		feature      Feature
		requiredTier string
		message      string
	}{
		{
			name:         "rbac is sold from the entry plan",
			feature:      FeatureRBAC,
			requiredTier: "Studio",
			message:      "Custom permissions require a Notifuse licence (Studio or above).",
		},
		{
			name:         "ses tenant isolation is sold from the entry plan",
			feature:      FeatureSESTenant,
			requiredTier: "Studio",
			message:      "SES tenant isolation requires a Notifuse licence (Studio or above).",
		},
		{
			name:         "sso is sold from the top plan",
			feature:      FeatureSSO,
			requiredTier: "Enterprise",
			message:      "Single sign-on requires a Notifuse Enterprise licence.",
		},
		{
			name:         "audit logs are sold from the top plan",
			feature:      FeatureAuditLogs,
			requiredTier: "Enterprise",
			message:      "Audit logs require a Notifuse Enterprise licence.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewFeatureNotLicensedError(tc.feature)

			require.NotNil(t, err)
			assert.Equal(t, tc.feature, err.Feature)
			assert.Equal(t, tc.requiredTier, err.RequiredTier)
			assert.Equal(t, tc.message, err.Message)
			assert.Equal(t, tc.message, err.Error())
		})
	}

	t.Run("quotes the most expensive plan for a feature this build does not know", func(t *testing.T) {
		// Guessing low would advertise a licence that does not carry the capability. The
		// refusal has to stay a refusal, so unknown never resolves to a cheaper yes.
		err := NewFeatureNotLicensedError(Feature("time_travel"))

		require.NotNil(t, err)
		assert.Equal(t, Feature("time_travel"), err.Feature)
		assert.Equal(t, "Enterprise", err.RequiredTier)
		assert.NotEmpty(t, err.Message)
	})

	t.Run("matches with errors.As through the wrapping a service adds", func(t *testing.T) {
		wrapped := fmt.Errorf("failed to enable tenant isolation: %w",
			NewFeatureNotLicensedError(FeatureSESTenant))

		var notLicensed *ErrFeatureNotLicensed
		require.True(t, errors.As(wrapped, &notLicensed))
		assert.Equal(t, FeatureSESTenant, notLicensed.Feature)
		assert.Equal(t, "Studio", notLicensed.RequiredTier)
	})

	t.Run("is not matched as a permission error", func(t *testing.T) {
		// The two land on different statuses — 402 here, 403 for PermissionError — so a
		// handler that matched both would sell a plan to a user who merely lacks a right.
		err := error(NewFeatureNotLicensedError(FeatureRBAC))

		var permErr *PermissionError
		assert.False(t, errors.As(err, &permErr))

		var unauthorized *ErrUnauthorized
		assert.False(t, errors.As(err, &unauthorized))
	})

	t.Run("serializes the fields the 402 body carries", func(t *testing.T) {
		encoded, err := json.Marshal(NewFeatureNotLicensedError(FeatureSESTenant))
		require.NoError(t, err)

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, "ses_tenant", decoded["feature"])
		assert.Equal(t, "Studio", decoded["required_tier"])
		assert.Equal(t, "SES tenant isolation requires a Notifuse licence (Studio or above).", decoded["message"])
	})
}

func TestErrWorkspaceQuotaReached(t *testing.T) {
	t.Run("Error names how many exist and what the ceiling is", func(t *testing.T) {
		err := &ErrWorkspaceQuotaReached{Limit: 3, Current: 3}

		assert.Equal(t, "workspace quota reached: 3 workspaces exist (limit: 3)", err.Error())
	})

	t.Run("matches with errors.As through the wrapping a service adds", func(t *testing.T) {
		wrapped := fmt.Errorf("failed to create workspace: %w",
			&ErrWorkspaceQuotaReached{Limit: 5, Current: 8})

		var quotaErr *ErrWorkspaceQuotaReached
		require.True(t, errors.As(wrapped, &quotaErr))
		assert.Equal(t, 5, quotaErr.Limit)
		assert.Equal(t, 8, quotaErr.Current)
	})

	t.Run("is distinct from the operator's own workspace limit error", func(t *testing.T) {
		// ErrWorkspaceLimitReached is the self-hosted PLAN_MAX_WORKSPACES ceiling and maps
		// to 403, where nothing is for sale. This one maps to 402. If errors.As confused
		// them, either the purchase prompt or the plain refusal would land on the wrong one.
		quota := error(&ErrWorkspaceQuotaReached{Limit: 3, Current: 3})
		limit := error(&ErrWorkspaceLimitReached{Limit: 3, Current: 3})

		var asLimit *ErrWorkspaceLimitReached
		assert.False(t, errors.As(quota, &asLimit))

		var asQuota *ErrWorkspaceQuotaReached
		assert.False(t, errors.As(limit, &asQuota))
	})

	t.Run("serializes the numbers the 402 body carries", func(t *testing.T) {
		encoded, err := json.Marshal(&ErrWorkspaceQuotaReached{Limit: 5, Current: 5})
		require.NoError(t, err)

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, float64(5), decoded["limit"])
		assert.Equal(t, float64(5), decoded["current"])
	})
}

func TestEntitlementsSerialization(t *testing.T) {
	t.Run("round-trips the fields the console reads off user.me", func(t *testing.T) {
		expiry := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
		original := Entitlements{
			Tier:          "agency",
			Org:           "ACME SAS",
			Sub:           "billing@acme.com",
			MaxWorkspaces: 15,
			Features:      []Feature{FeatureRBAC, FeatureSESTenant},
			State:         LicenseStateGrace,
			ExpiresAt:     expiry,
		}

		encoded, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded Entitlements
		require.NoError(t, json.Unmarshal(encoded, &decoded))

		assert.Equal(t, original, decoded)
		assert.True(t, decoded.Licensed())
		assert.True(t, decoded.Has(FeatureRBAC))
		assert.False(t, decoded.Has(FeatureSSO))
	})
}

// The self-check for the "# Call sites" list on EntitlementProvider.
//
// The plan makes that list the compensating control for having no ee/ directory: it is the
// honest answer to "where is licensing actually enforced", and the plan promises it is
// complete. A promise like that has to be mechanised, because a doc comment nobody can
// verify rots silently and is then worse than no comment at all — a reader who trusts it
// stops looking, which is exactly how a seventh consumer went unlisted.
const (
	// callSitesHeading and neverHeading bracket the region of the comment that must name
	// every consumer.
	callSitesHeading = "# Call sites"
	neverHeading     = "# Never"

	// entitlementProviderIdent is what a consumer mentions. Inside package domain it is
	// bare; everywhere else it is domain.EntitlementProvider, and both contain this.
	entitlementProviderIdent = "EntitlementProvider"

	// entitlementsCallIdent catches the consumers that never name the type.
	//
	// Two readers reach the grant through a port they declare themselves —
	// LicenseStateReader in user_handler.go, LicenseServiceInterface in
	// settings_handler.go — which is good house style and made both invisible to a walker
	// that searched for the type alone. They serve the grant to /api/user.me and
	// /api/licence.get respectively: the single value the whole console keys off. A
	// compensating control that could not see the console's own source of truth was not
	// one.
	//
	// Searching for the CALL rather than the type catches any consumer whatever port it
	// goes through, which is the property the list actually promises.
	entitlementsCallIdent = ".Entitlements()"

	// licenseSourceFile is the file carrying the list, relative to the repository root.
	licenseSourceFile = "internal/domain/license.go"
)

// notCallSites are the files that mention the interface without consuming it. Each one is
// exempt for a stated structural reason, and the list is deliberately tiny: an exemption is
// the one way this check could be defeated, so adding to it should feel expensive.
var notCallSites = map[string]string{
	// Declares the interface. Listing itself would be circular.
	licenseSourceFile: "the declaration",

	// Implements the interface rather than consuming it. It is the provider every listed
	// call site is handed; it asks nobody.
	"internal/service/license_service.go": "the implementation",
}

// repoRoot returns the repository root, from this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "go.mod"),
		"this check locates the repository root by walking up from internal/domain; if the package moved, fix the path rather than deleting the check")

	return root
}

// entitlementProviderConsumers walks the tree for non-generated, non-test Go files that name
// the interface OR call Entitlements() through a port of their own.
func entitlementProviderConsumers(t *testing.T, root string) []string {
	t.Helper()

	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "mocks":
				return fs.SkipDir
			}
			return nil
		}

		name := entry.Name()
		// Tests are not call sites, and generated mocks are not consumers: both would
		// pad the list with entries that enforce nothing.
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "mock_") {
			return nil
		}

		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(contents)
		if !strings.Contains(text, entitlementProviderIdent) &&
			!strings.Contains(text, entitlementsCallIdent) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)

	sort.Strings(found)
	return found
}

func TestEntitlementProviderCallSitesAreListed(t *testing.T) {
	root := repoRoot(t)

	source, err := os.ReadFile(filepath.Join(root, licenseSourceFile))
	require.NoError(t, err)

	text := string(source)
	start := strings.Index(text, callSitesHeading)
	require.GreaterOrEqual(t, start, 0, "the %q heading is gone from %s; it is the compensating control for having no ee/ directory, so it must be restored rather than dropped", callSitesHeading, licenseSourceFile)
	end := strings.Index(text[start:], neverHeading)
	require.Greater(t, end, 0, "the %q heading no longer follows %q, so the call-site region cannot be delimited", neverHeading, callSitesHeading)
	listed := text[start : start+end]

	consumers := entitlementProviderConsumers(t, root)
	require.NotEmpty(t, consumers, "the walker found no consumers at all, which means it is looking in the wrong place and would never fail")

	t.Run("every consumer of the interface is named in the list", func(t *testing.T) {
		for _, consumer := range consumers {
			if reason, exempt := notCallSites[consumer]; exempt {
				t.Logf("%s is exempt: %s", consumer, reason)
				continue
			}

			assert.Contains(t, listed, consumer,
				"%s consumes the entitlements (by naming domain.EntitlementProvider or by calling Entitlements()) but is not named in the %q list in %s.\n"+
					"That list is what the plan offers instead of an ee/ directory, and it promises to be complete.\n"+
					"Add the file — and the gate or reader it holds — to the comment. Do not weaken this test.",
				consumer, callSitesHeading, licenseSourceFile)
		}
	})

	t.Run("nothing in the list has stopped consuming the interface", func(t *testing.T) {
		// The other direction of rot: a gate that was removed or moved leaves an entry
		// promising an enforcement point that is no longer there, which is how a
		// licence check silently disappears.
		//
		// One deliberate omission. internal/http/root_handler.go is named in the list
		// because that is where OIDC_ENABLED is decided, but it receives a func() bool
		// rather than the interface — naming the file is a claim about where licensing
		// is visible, not about who imports the type.
		gateByRegistration := map[string]bool{
			"internal/http/root_handler.go": true,
		}

		present := make(map[string]bool, len(consumers))
		for _, consumer := range consumers {
			present[consumer] = true
		}

		for _, line := range strings.Split(listed, "\n") {
			for _, field := range strings.Fields(line) {
				candidate := strings.Trim(field, ",.;:()")
				// A repository-relative path, not a bare file name: the prose
				// around the list mentions this test file by name, and that is
				// not a claim about a call site.
				if !strings.HasSuffix(candidate, ".go") || !strings.Contains(candidate, "/") || gateByRegistration[candidate] {
					continue
				}
				assert.True(t, present[candidate],
					"%s is named in the %q list but no longer mentions %s. Either the gate moved and the comment must follow it, or the gate was deleted and the entry must go.",
					candidate, callSitesHeading, entitlementProviderIdent)
			}
		}
	})
}

// The one entry that is not a refusal.
//
// The Widenings heading in the ledger says "None", and that is a claim about behaviour, so
// it is asserted against the code rather than trusted. ConnectZapier used to widen the
// Zapier key to FullPermissions on an unlicensed deployment; the heading and the two
// assertions below are what keep that from quietly coming back — the reader asking "can an
// unlicensed deployment obtain admin scope" must find a checked answer, not a list of
// refusals that forgot to mention the exception.
func TestTheZapierWideningHasNotReturned(t *testing.T) {
	root := repoRoot(t)

	source, err := os.ReadFile(filepath.Join(root, licenseSourceFile))
	require.NoError(t, err)
	text := string(source)

	require.Contains(t, text, "# Widenings",
		"the Widenings heading is gone from %s: keep it, saying None, so the absence is a checkable claim", licenseSourceFile)
	heading := strings.Index(text, "# Widenings")
	section := text[heading:]
	if next := strings.Index(section[1:], "\n// # "); next != -1 {
		section = section[:next+1]
	}
	require.Contains(t, section, "None",
		"the Widenings section of %s no longer says None — if a widening was added, this test and the section must both change, deliberately", licenseSourceFile)

	service, err := os.ReadFile(
		filepath.Join(root, "internal", "service", "workspace_service.go"),
	)
	require.NoError(t, err)
	code := string(service)

	connect := strings.Index(code, "func (s *WorkspaceService) ConnectZapier(")
	require.NotEqual(t, -1, connect, "ConnectZapier has moved; update this test to follow it")
	body := code[connect:]
	if next := strings.Index(body[1:], "\nfunc "); next != -1 {
		body = body[:next+1]
	}
	assert.NotContains(t, body, "NewFullPermissions()",
		"ConnectZapier widens the Zapier key to full access again — an integration platform must never hold an admin credential for a workspace")
	assert.Contains(t, body, "scopeFixedByProduct",
		"ConnectZapier must mint through the product-scope path; through CreateAPIKey the licence gate refuses the narrow scope on an unlicensed deployment, and the temptation to widen returns with it")
}

// TestTheConsoleNamesTheSameTierTheServerDoes pins console/src/types/license.ts's
// LICENSE_REQUIRED_TIER to NewFeatureNotLicensedError.
//
// The console names the plan BEFORE the call, in a locked control; the server names it AFTER,
// in the 402. Two tables in two languages, and a plan renamed in one of them would send a buyer
// to the wrong tier from one side or the other. The console cannot import this package, so the
// agreement is checked here, where the server's table lives.
func TestTheConsoleNamesTheSameTierTheServerDoes(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(repoRoot(t), "console", "src", "types", "license.ts"))
	require.NoError(t, err)

	for _, f := range []Feature{
		FeatureRBAC, FeatureSESTenant, FeatureSSO, FeatureAuditLogs, FeatureTemplateI18n,
	} {
		entry := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(string(f)) + `: '([A-Za-z]+)'`)
		match := entry.FindSubmatch(source)
		require.NotNil(t, match, "console LICENSE_REQUIRED_TIER has no entry for %q", f)
		assert.Equal(t, NewFeatureNotLicensedError(f).RequiredTier, string(match[1]),
			"the console and the server name different plans for %q", f)
	}
}
