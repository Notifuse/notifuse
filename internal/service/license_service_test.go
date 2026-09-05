package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/license"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLicenseStore stands in for the settings table and records what was asked of it, so a
// test can assert that the environment key made the database read unnecessary rather than
// merely that it produced the same answer.
type fakeLicenseStore struct {
	value  string
	getErr error
	// When set, Get parks on it before doing anything else — the shape of a database
	// that is up but not answering, which is what the request path must never wait on.
	block  chan struct{}
	setErr error

	gets     int
	sets     int
	setValue string
}

func (f *fakeLicenseStore) Get(_ context.Context, key string) (*domain.Setting, error) {
	if f.block != nil {
		<-f.block
	}
	f.gets++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.value == "" {
		return nil, &domain.ErrSettingNotFound{Key: key}
	}
	return &domain.Setting{Key: key, Value: f.value}, nil
}

func (f *fakeLicenseStore) Set(_ context.Context, _, value string) error {
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	f.setValue = value
	f.value = value
	return nil
}

// quietLogger accepts anything. Licence handling logs one line at startup and one on
// degradation, and most tests are asserting the grant rather than the prose.
func quietLogger(t *testing.T) *pkgmocks.MockLogger {
	t.Helper()
	ctrl := gomock.NewController(t)
	l := pkgmocks.NewMockLogger(ctrl)
	l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().Info(gomock.Any()).AnyTimes()
	l.EXPECT().Warn(gomock.Any()).AnyTimes()
	l.EXPECT().Error(gomock.Any()).AnyTimes()
	l.EXPECT().Debug(gomock.Any()).AnyTimes()
	return l
}

// fixedClock freezes time so the boundaries a licence draws can be asserted to the second.
func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

// alwaysTrusted stands in for license.HasTrustedKey in every test that is not about the
// placeholder signing key. It is deliberately explicit at each call site rather than a
// default: a dependency that can be omitted is a dependency that will be omitted, and this
// one decides whether a deployment is told its binary can never accept a licence.
func alwaysTrusted() bool { return true }

// noTrustedKey is what a build still carrying the pubkey_prod.go placeholder answers.
func noTrustedKey() bool { return false }

// movableClock is a clock a test can wind forward, for the lazy retry: the whole point of
// the backoff is that time has to pass, and no test may pass by sleeping.
type movableClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *movableClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *movableClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func parserReturning(claims *license.Claims, err error) func(string) (*license.Claims, error) {
	return func(string) (*license.Claims, error) { return claims, err }
}

// testClaims is a plausible Agency key: two features, a fifteen-workspace ceiling, and an
// expiry the caller chooses.
func testClaims(expiresAt time.Time) *license.Claims {
	return &license.Claims{
		V:     license.SchemaVersion,
		LID:   "lic_7f3a9c21",
		Org:   "ACME SAS",
		Sub:   "billing@acme.com",
		Tier:  "agency",
		Feat:  []string{"rbac", "ses_tenant"},
		MaxWS: 15,
		IAT:   expiresAt.Add(-365 * 24 * time.Hour).Unix(),
		Exp:   expiresAt.Unix(),
	}
}

func TestDeriveLicenseState(t *testing.T) {
	expiresAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		now  time.Time
		want domain.LicenseState
	}{
		{
			name: "a second before expiry is active",
			now:  expiresAt.Add(-time.Second),
			want: domain.LicenseStateActive,
		},
		{
			name: "the expiry second itself is still active",
			// Inclusive on purpose: whether a key checked in the very second it lapses
			// is honoured must not depend on which side of a tick a request landed.
			now:  expiresAt,
			want: domain.LicenseStateActive,
		},
		{
			name: "a second after expiry is grace",
			now:  expiresAt.Add(time.Second),
			want: domain.LicenseStateGrace,
		},
		{
			name: "a second before the grace period ends is still grace",
			now:  expiresAt.Add(GracePeriod).Add(-time.Second),
			want: domain.LicenseStateGrace,
		},
		{
			name: "the last second of the grace period is still grace",
			now:  expiresAt.Add(GracePeriod),
			want: domain.LicenseStateGrace,
		},
		{
			name: "a second after the grace period is expired",
			now:  expiresAt.Add(GracePeriod).Add(time.Second),
			want: domain.LicenseStateExpired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deriveLicenseState(expiresAt, tc.now))
		})
	}
}

func TestGracePeriodIsThirtyDays(t *testing.T) {
	// Pinned because the number is a policy decision, not an implementation detail: it is
	// sized to outlast Stripe's dunning schedule, and shrinking it would silently degrade
	// customers whose renewal payment is still being retried.
	assert.Equal(t, 30*24*time.Hour, GracePeriod)
}

func TestEntitlementsFrom(t *testing.T) {
	expiresAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	t.Run("no claims is the community grant", func(t *testing.T) {
		ent := entitlementsFrom(nil, expiresAt)

		assert.Equal(t, domain.CommunityEntitlements(), ent)
		assert.Equal(t, domain.CommunityMaxWorkspaces, ent.MaxWorkspaces)
		assert.NotNil(t, ent.Features, "features must marshal as [] for the console, never null")
	})

	t.Run("an active key grants what it carries", func(t *testing.T) {
		ent := entitlementsFrom(testClaims(expiresAt), expiresAt.Add(-time.Hour))

		assert.Equal(t, domain.LicenseStateActive, ent.State)
		assert.Equal(t, "agency", ent.Tier)
		assert.Equal(t, "ACME SAS", ent.Org)
		assert.Equal(t, "billing@acme.com", ent.Sub)
		assert.Equal(t, 15, ent.MaxWorkspaces)
		assert.Equal(t, expiresAt, ent.ExpiresAt)
		assert.True(t, ent.Has(domain.FeatureRBAC))
		assert.True(t, ent.Has(domain.FeatureSESTenant))
		assert.False(t, ent.Has(domain.FeatureSSO))
		assert.True(t, ent.Licensed())
	})

	t.Run("grace grants exactly what active grants", func(t *testing.T) {
		claims := testClaims(expiresAt)

		active := entitlementsFrom(claims, expiresAt.Add(-time.Hour))
		grace := entitlementsFrom(claims, expiresAt.Add(24*time.Hour))

		assert.Equal(t, domain.LicenseStateGrace, grace.State)
		assert.True(t, grace.Licensed(), "a customer being dunned keeps everything they are paying for")

		// Compare on the fields a gate reads. Only State may differ between the two.
		assert.Equal(t, active.Features, grace.Features)
		assert.Equal(t, active.MaxWorkspaces, grace.MaxWorkspaces)
	})

	t.Run("an expired key grants exactly what no key grants", func(t *testing.T) {
		ent := entitlementsFrom(testClaims(expiresAt), expiresAt.Add(GracePeriod).Add(time.Second))
		community := domain.CommunityEntitlements()

		assert.Equal(t, domain.LicenseStateExpired, ent.State)
		assert.Equal(t, community.Features, ent.Features)
		assert.Equal(t, community.MaxWorkspaces, ent.MaxWorkspaces)
		assert.False(t, ent.Has(domain.FeatureRBAC))
		assert.False(t, ent.Licensed())

		// The licensee is still named, because the console has to be able to say "your
		// key ran out" rather than "you have no key".
		assert.Equal(t, "ACME SAS", ent.Org)
		assert.Equal(t, "agency", ent.Tier)
		assert.Equal(t, expiresAt, ent.ExpiresAt)
	})

	t.Run("a feature this build does not know is dropped", func(t *testing.T) {
		claims := testClaims(expiresAt)
		claims.Feat = []string{"rbac", "time_travel", ""}

		ent := entitlementsFrom(claims, expiresAt.Add(-time.Hour))

		assert.Equal(t, []domain.Feature{domain.FeatureRBAC}, ent.Features)
	})

	t.Run("a negative ceiling means unlimited", func(t *testing.T) {
		claims := testClaims(expiresAt)
		claims.MaxWS = -1

		assert.Equal(t, domain.UnlimitedWorkspaces, entitlementsFrom(claims, expiresAt).MaxWorkspaces)
	})
}

func TestLicenseService_KeySourcePrecedence(t *testing.T) {
	expiresAt := time.Now().Add(365 * 24 * time.Hour)
	envClaims := testClaims(expiresAt)
	envClaims.Org = "from env"
	dbClaims := testClaims(expiresAt)
	dbClaims.Org = "from db"

	parse := func(raw string) (*license.Claims, error) {
		switch raw {
		case "env-key":
			return envClaims, nil
		case "db-key":
			return dbClaims, nil
		default:
			return nil, license.ErrBadSignature
		}
	}

	t.Run("the environment wins and the database is not even read", func(t *testing.T) {
		store := &fakeLicenseStore{value: "db-key"}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			EnvKey:      "env-key",
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		assert.Equal(t, "from env", svc.Entitlements().Org)
		assert.Zero(t, store.gets, "a key declared in the environment must not depend on the database being up")
	})

	t.Run("surrounding whitespace in the environment value is tolerated", func(t *testing.T) {
		// A key pasted into a manifest, a .env file or a Docker secret arrives with a
		// trailing newline more often than not.
		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "  env-key\n",
			Logger: quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		assert.Equal(t, "from env", svc.Entitlements().Org)
	})

	t.Run("the database is used when the environment is unset", func(t *testing.T) {
		store := &fakeLicenseStore{value: "db-key"}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		assert.Equal(t, "from db", svc.Entitlements().Org)
		assert.Equal(t, 1, store.gets, "the key is read once, not on every entitlement check")

		svc.Entitlements()
		svc.Entitlements()
		assert.Equal(t, 1, store.gets)
	})

	t.Run("no key anywhere is the community grant", func(t *testing.T) {
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: &fakeLicenseStore{},
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})

	t.Run("a stored empty value is not a key", func(t *testing.T) {
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: &fakeLicenseStore{value: "   "},
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})
}

func TestLicenseService_FailsSafe(t *testing.T) {
	// Every failure pkg/license can report. They are all the same outcome — the free tier —
	// and the point of enumerating them is that no future error class can be added to the
	// verifier and accidentally become a panic or a boot failure here.
	parseFailures := map[string]error{
		"malformed envelope":               license.ErrMalformedEnvelope,
		"bad encoding":                     license.ErrBadEncoding,
		"no trusted key":                   license.ErrNoTrustedKey,
		"bad signature":                    license.ErrBadSignature,
		"malformed payload":                license.ErrMalformedPayload,
		"unknown version":                  license.ErrUnknownVersion,
		"future issued at":                 license.ErrFutureIssuedAt,
		"an error nobody has invented yet": errors.New("something else entirely"),
	}

	for name, parseErr := range parseFailures {
		t.Run(name+" degrades to community", func(t *testing.T) {
			svc := newLicenseService(LicenseServiceConfig{
				EnvKey: "whatever",
				Logger: quietLogger(t),
			}, parserReturning(nil, parseErr), time.Now, alwaysTrusted)

			assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
			assert.False(t, svc.Entitlements().Has(domain.FeatureSSO),
				"a broken key must not unlock anything")
		})
	}

	t.Run("a verifier that panics degrades to community instead of taking the process down", func(t *testing.T) {
		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "whatever",
			Logger: quietLogger(t),
		}, func(string) (*license.Claims, error) { panic("verifier exploded") }, time.Now, alwaysTrusted)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})

	t.Run("an unreadable settings table degrades to community", func(t *testing.T) {
		store := &fakeLicenseStore{getErr: errors.New("connection refused")}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parserReturning(testClaims(time.Now().Add(time.Hour)), nil), time.Now, alwaysTrusted)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})

	t.Run("no settings store at all degrades to community", func(t *testing.T) {
		svc := newLicenseService(LicenseServiceConfig{
			Logger: quietLogger(t),
		}, parserReturning(nil, license.ErrBadSignature), time.Now, alwaysTrusted)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})

	t.Run("a service with no logger still answers", func(t *testing.T) {
		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "whatever",
		}, parserReturning(nil, license.ErrBadSignature), time.Now, alwaysTrusted)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})

	t.Run("a rejected key is warned about exactly once, not on every read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		l := pkgmocks.NewMockLogger(ctrl)
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Warn(gomock.Any()).Times(1)

		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "whatever",
			Logger: l,
		}, parserReturning(nil, license.ErrBadSignature), time.Now, alwaysTrusted)

		for i := 0; i < 10; i++ {
			svc.Entitlements()
		}
	})

	t.Run("a missing settings row is ordinary and is not warned about", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		l := pkgmocks.NewMockLogger(ctrl)
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Warn(gomock.Any()).Times(0)

		newLicenseService(LicenseServiceConfig{
			SettingRepo: &fakeLicenseStore{},
			Logger:      l,
		}, parserReturning(nil, license.ErrBadSignature), time.Now, alwaysTrusted)
	})
}

// What this service contributes to the SSO gate: whether the resolved grant carries the
// feature. Whether SSO is also switched on — and therefore whether the sign-in button appears
// — is OIDCService.IsEnabled's question, and it is answered in oidc_license_gate_test.go.
// Splitting it that way is the point of the redesign: the licence answers what was bought,
// and one capability decides what to do about it.
func TestLicenseService_SSOEntitlement(t *testing.T) {
	future := time.Now().Add(365 * 24 * time.Hour)

	withSSO := testClaims(future)
	withSSO.Feat = []string{"rbac", "ses_tenant", "sso", "audit_logs"}
	withoutSSO := testClaims(future)

	testCases := []struct {
		name     string
		claims   *license.Claims
		parseErr error
		want     bool
	}{
		{
			name:   "a key that carries sso",
			claims: withSSO,
			want:   true,
		},
		{
			name:   "a perfectly valid key that does not carry sso",
			claims: withoutSSO,
			want:   false,
		},
		{
			name:     "no usable key at all",
			parseErr: license.ErrNoTrustedKey,
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newLicenseService(LicenseServiceConfig{
				EnvKey: "a-key",
				Logger: quietLogger(t),
			}, parserReturning(tc.claims, tc.parseErr), time.Now, alwaysTrusted)

			assert.Equal(t, tc.want, svc.Entitlements().Has(domain.FeatureSSO))
		})
	}

	t.Run("an expired sso licence stops granting sso", func(t *testing.T) {
		// The grace period is generous, but it does end, and when it does the deployment
		// is in exactly the position it would be in with no key.
		expiresAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
		expired := testClaims(expiresAt)
		expired.Feat = []string{"sso"}

		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "a-key",
			Logger: quietLogger(t),
		}, parserReturning(expired, nil), fixedClock(expiresAt.Add(GracePeriod).Add(time.Second)), alwaysTrusted)

		assert.Equal(t, domain.LicenseStateExpired, svc.Entitlements().State)
		assert.False(t, svc.Entitlements().Has(domain.FeatureSSO))
	})

	t.Run("an sso licence inside its grace period still grants sso", func(t *testing.T) {
		// Thirty days of dunning must not cost a customer their logins.
		expiresAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
		lapsing := testClaims(expiresAt)
		lapsing.Feat = []string{"sso"}

		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "a-key",
			Logger: quietLogger(t),
		}, parserReturning(lapsing, nil), fixedClock(expiresAt.Add(GracePeriod)), alwaysTrusted)

		assert.Equal(t, domain.LicenseStateGrace, svc.Entitlements().State)
		assert.True(t, svc.Entitlements().Has(domain.FeatureSSO),
			"the last second of grace is still a licence")
	})
}

func TestLicenseService_SetKey(t *testing.T) {
	future := time.Now().Add(365 * 24 * time.Hour)
	goodClaims := testClaims(future)
	goodClaims.Org = "the new key"
	installedClaims := testClaims(future)
	installedClaims.Org = "the key already installed"

	parse := func(raw string) (*license.Claims, error) {
		switch raw {
		case "good-key":
			return goodClaims, nil
		case "installed-key":
			return installedClaims, nil
		default:
			return nil, license.ErrBadSignature
		}
	}

	newInstalled := func(t *testing.T) (*LicenseService, *fakeLicenseStore) {
		t.Helper()
		store := &fakeLicenseStore{value: "installed-key"}
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)
		require.Equal(t, "the key already installed", svc.Entitlements().Org)
		return svc, store
	}

	t.Run("a valid key is stored and becomes current", func(t *testing.T) {
		svc, store := newInstalled(t)

		require.NoError(t, svc.SetKey(context.Background(), "  good-key\n"))

		assert.Equal(t, "good-key", store.setValue, "the envelope is stored trimmed and verbatim")
		assert.Equal(t, "the new key", svc.Entitlements().Org, "the swap is in-memory: no restart is needed")
	})

	t.Run("an invalid key changes nothing", func(t *testing.T) {
		svc, store := newInstalled(t)

		err := svc.SetKey(context.Background(), "nonsense")

		require.Error(t, err)
		assert.ErrorIs(t, err, license.ErrBadSignature)
		assert.Zero(t, store.sets, "a key that does not verify must never reach the settings table")
		assert.Equal(t, "the key already installed", svc.Entitlements().Org)
	})

	t.Run("an empty key is refused rather than treated as a clear", func(t *testing.T) {
		svc, store := newInstalled(t)

		assert.ErrorIs(t, svc.SetKey(context.Background(), "   "), ErrLicenseKeyEmpty)
		assert.Zero(t, store.sets)
		assert.Equal(t, "the key already installed", svc.Entitlements().Org)
	})

	t.Run("a failed write leaves the previous licence in place", func(t *testing.T) {
		svc, store := newInstalled(t)
		store.setErr = errors.New("disk full")

		err := svc.SetKey(context.Background(), "good-key")

		require.Error(t, err)
		assert.Equal(t, "the key already installed", svc.Entitlements().Org,
			"a licence that could not be persisted must not silently apply until the next restart")
	})

	t.Run("the environment locks the key against console edits", func(t *testing.T) {
		store := &fakeLicenseStore{}
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			EnvKey:      "installed-key",
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		err := svc.SetKey(context.Background(), "good-key")

		assert.ErrorIs(t, err, ErrLicenseKeyLockedByEnv)
		assert.Zero(t, store.sets, "storing a key that could never take effect would report a success that is not one")
		assert.Equal(t, "the key already installed", svc.Entitlements().Org)
	})
}

// TestLicenseService_RealEnvelopes exercises the production path — license.Parse, the public
// keys compiled into this build — rather than an injected verifier.
func TestLicenseService_RealEnvelopes(t *testing.T) {
	t.Run("a well-formed key from an untrusted authority degrades to community", func(t *testing.T) {
		// Minted here, in this test, with a key pair generated on the spot. It is a
		// genuine NFUSE1 envelope with a valid signature; what it is not is signed by an
		// authority this binary trusts, which is the shape of both a forgery and a key
		// issued by somebody else's Notifuse.
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		raw, err := license.Mint(priv, &license.Claims{
			V:     license.SchemaVersion,
			LID:   "lic_untrusted",
			Org:   "Someone Else",
			Sub:   "someone@example.com",
			Tier:  "enterprise",
			Feat:  []string{"rbac", "ses_tenant", "sso", "audit_logs"},
			MaxWS: -1,
			IAT:   time.Now().Add(-time.Hour).Unix(),
			Exp:   time.Now().Add(365 * 24 * time.Hour).Unix(),
		})
		require.NoError(t, err)

		svc := NewLicenseService(LicenseServiceConfig{
			EnvKey:      raw,
			OIDCEnabled: true,
			Logger:      quietLogger(t),
		})

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
		assert.False(t, svc.Entitlements().Has(domain.FeatureSSO),
			"an unverifiable key must not unlock sso")
	})

	t.Run("a key minted with the committed dev signing key", func(t *testing.T) {
		// The fixture is pkg/license/testdata/dev_signing_key.json — the key pair
		// licensegen generated for development, committed on purpose and matching the
		// public key in pubkey_dev.go. This test therefore means two different things,
		// both of them worth asserting:
		//
		//   go test ./internal/service/...              the dev key is NOT trusted, so
		//                                               this is the fail-safe path
		//   go test -tags licdev ./internal/service/... the dev key IS trusted, so this is
		//                                               a real end-to-end grant
		priv := devSigningKey(t)
		expiresAt := time.Now().Add(365 * 24 * time.Hour).Truncate(time.Second)

		raw, err := license.Mint(priv, &license.Claims{
			V:     license.SchemaVersion,
			LID:   "lic_devfixture",
			Org:   "ACME SAS",
			Sub:   "billing@acme.com",
			Tier:  "enterprise",
			Feat:  []string{"rbac", "ses_tenant", "sso"},
			MaxWS: 15,
			IAT:   time.Now().Add(-time.Hour).Unix(),
			Exp:   expiresAt.Unix(),
		})
		require.NoError(t, err)

		svc := NewLicenseService(LicenseServiceConfig{
			EnvKey:      raw,
			OIDCEnabled: true,
			Logger:      quietLogger(t),
		})
		ent := svc.Entitlements()

		if _, parseErr := license.Parse(raw); parseErr != nil {
			assert.Equal(t, domain.CommunityEntitlements(), ent,
				"a build that does not trust the dev key must fall back, not fail")
			assert.False(t, ent.Has(domain.FeatureSSO))
			return
		}

		assert.Equal(t, domain.LicenseStateActive, ent.State)
		assert.Equal(t, "enterprise", ent.Tier)
		assert.Equal(t, "ACME SAS", ent.Org)
		assert.Equal(t, 15, ent.MaxWorkspaces)
		assert.Equal(t, expiresAt.UTC(), ent.ExpiresAt)
		assert.True(t, ent.Has(domain.FeatureSSO),
			"the sign-in button is what this grant buys")
	})
}

// devSigningKey loads the committed development key pair. It is a fixture and not a secret:
// it signs nothing a release binary will ever accept.
func devSigningKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()

	path := filepath.Join("..", "..", "pkg", "license", "testdata", "dev_signing_key.json")
	contents, err := os.ReadFile(path)
	require.NoError(t, err, "the dev signing fixture is what makes an end-to-end licence test possible")

	var fixture struct {
		Private string `json:"private"`
	}
	require.NoError(t, json.Unmarshal(contents, &fixture))

	decoded, err := base64.StdEncoding.DecodeString(fixture.Private)
	require.NoError(t, err)
	require.Len(t, decoded, ed25519.PrivateKeySize)

	return ed25519.PrivateKey(decoded)
}

// TestALicensedDeploymentIsNeverWorseOffThanAnUnlicensedOne is the invariant the whole
// scheme rests on, and it gets its own name because it is the one a reviewer should be able
// to find by searching for it.
//
// A key is immutable and there is no revocation, so a key minted with a nonsensical ceiling
// cannot be recalled — it is in the customer's hands until its exp. licensegen refuses to
// mint max_ws 0, which stops the mistake being made again; this floor is what makes the ones
// that already exist harmless. Both ends are needed: the mint-time check does not reach a key
// already issued, and the floor does not stop a second signer from issuing another.
func TestALicensedDeploymentIsNeverWorseOffThanAnUnlicensedOne(t *testing.T) {
	expiresAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	community := domain.CommunityEntitlements()

	// Every ceiling a signer could put in a key, including the ones nobody would mean.
	for _, maxWS := range []int{-100, -2, -1, 0, 1, 2, 3, 4, 5, 15, 1000} {
		t.Run(fmt.Sprintf("a key carrying max_ws %d", maxWS), func(t *testing.T) {
			claims := testClaims(expiresAt)
			claims.MaxWS = maxWS

			for _, tc := range []struct {
				state string
				now   time.Time
			}{
				{state: "active", now: expiresAt.Add(-time.Hour)},
				{state: "grace", now: expiresAt.Add(24 * time.Hour)},
				{state: "expired", now: expiresAt.Add(GracePeriod).Add(time.Second)},
			} {
				ent := entitlementsFrom(claims, tc.now)

				if ent.MaxWorkspaces == domain.UnlimitedWorkspaces {
					continue
				}

				assert.GreaterOrEqual(t, ent.MaxWorkspaces, community.MaxWorkspaces,
					"in state %s a licensed deployment resolved to %d workspaces, fewer than the %d an unlicensed one gets: a paying customer must never be worse off than a free one",
					tc.state, ent.MaxWorkspaces, community.MaxWorkspaces)
			}
		})
	}
}

func TestNormalizeWorkspaceQuota(t *testing.T) {
	testCases := []struct {
		name  string
		maxWS int
		want  int
	}{
		{
			name:  "the unlimited sentinel passes through",
			maxWS: -1,
			want:  domain.UnlimitedWorkspaces,
		},
		{
			name: "a value nobody designed still means unlimited rather than something arbitrary",
			// Only -1 is ever minted, but a gate handed -5 would compare against it
			// and behave in a way nobody chose.
			maxWS: -5,
			want:  domain.UnlimitedWorkspaces,
		},
		{
			name: "zero is raised to the community floor rather than locking the licensee out",
			// The defect: read literally, a ceiling of zero makes `count >= limit`
			// true on an installation with no workspaces at all, so the customer who
			// just paid cannot create their first one.
			maxWS: 0,
			want:  domain.CommunityMaxWorkspaces,
		},
		{
			name:  "a ceiling below the community grant is raised to it",
			maxWS: 1,
			want:  domain.CommunityMaxWorkspaces,
		},
		{
			name:  "the community grant itself is unchanged",
			maxWS: domain.CommunityMaxWorkspaces,
			want:  domain.CommunityMaxWorkspaces,
		},
		{
			name:  "an ordinary paid ceiling passes through untouched",
			maxWS: 15,
			want:  15,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeWorkspaceQuota(tc.maxWS))
		})
	}
}

// TestLicenseService_RetriesAnUnansweredRead covers the difference between a settled answer
// and a failure to get one.
//
// The defect it guards: a single non-ErrSettingNotFound error from the settings table was
// cached as "no licence" for the lifetime of the process. A connection refused during a
// rolling restart therefore cost a paying customer every paid capability until somebody
// restarted them again — and with OIDC enabled, that is a permanently read-only console.
func TestLicenseService_RetriesAnUnansweredRead(t *testing.T) {
	future := time.Date(2027, 9, 3, 12, 0, 0, 0, time.UTC)
	claims := testClaims(future)
	claims.Org = "ACME SAS"
	parse := parserReturning(claims, nil)

	t.Run("a database that comes back restores the licence without a restart", func(t *testing.T) {
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{getErr: errors.New("connection refused")}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)
		// The read runs off the request goroutine and holds reloadMu until it is done, so
		// taking the lock is how a test waits for it.
		settle := func() { svc.reloadMu.Lock(); svc.reloadMu.Unlock() }

		require.Equal(t, domain.CommunityEntitlements(), svc.Entitlements(),
			"an unreadable settings table degrades to the free tier; it must not fail the boot")
		require.Equal(t, 1, store.gets)

		// Nothing before the backoff elapses: an unreachable database must not turn
		// into one connection attempt per request.
		for i := 0; i < 50; i++ {
			svc.Entitlements()
		}
		settle()
		assert.Equal(t, 1, store.gets, "the retry must be backed off, not a hot loop")

		clock.advance(licenseRetryInitialBackoff - time.Second)
		svc.Entitlements()
		settle()
		assert.Equal(t, 1, store.gets, "a second before the backoff elapses is still too early")

		// The database comes back.
		store.getErr = nil
		store.value = "a-key"
		clock.advance(2 * time.Second)

		// The call that notices the backoff has elapsed is answered from the state it
		// found and starts the read; the next call sees what the read installed.
		svc.Entitlements()
		settle()
		ent := svc.Entitlements()
		assert.Equal(t, 2, store.gets)
		assert.Equal(t, "ACME SAS", ent.Org,
			"a transient database error must not cost a paying customer their licence for the lifetime of the process")
		assert.Equal(t, 15, ent.MaxWorkspaces)
		assert.True(t, ent.Has(domain.FeatureRBAC))
	})

	t.Run("the backoff grows so a database that stays down costs a handful of attempts", func(t *testing.T) {
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{getErr: errors.New("connection refused")}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)
		settle := func() { svc.reloadMu.Lock(); svc.reloadMu.Unlock() }
		require.Equal(t, 1, store.gets)

		// One backoff window buys exactly one attempt, and the window doubles.
		clock.advance(licenseRetryInitialBackoff)
		svc.Entitlements()
		settle()
		require.Equal(t, 2, store.gets)

		clock.advance(licenseRetryInitialBackoff)
		svc.Entitlements()
		settle()
		assert.Equal(t, 2, store.gets, "the second window is twice as long as the first")

		clock.advance(licenseRetryInitialBackoff)
		svc.Entitlements()
		settle()
		assert.Equal(t, 3, store.gets)

		// However long the outage lasts, the wait never grows past the cap, so a
		// database that comes back is picked up within minutes rather than hours.
		clock.advance(24 * time.Hour)
		svc.Entitlements()
		settle()
		require.Equal(t, 4, store.gets)
		clock.advance(licenseRetryMaxBackoff)
		svc.Entitlements()
		settle()
		assert.Equal(t, 5, store.gets)
	})

	t.Run("the caller that triggers a read is answered without waiting for it", func(t *testing.T) {
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{getErr: errors.New("connection refused")}
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)
		settle := func() { svc.reloadMu.Lock(); svc.reloadMu.Unlock() }

		// A database that is up but not answering: Get parks until released.
		release := make(chan struct{})
		store.block = release
		clock.advance(licenseRetryInitialBackoff)

		answered := make(chan domain.Entitlements, 1)
		go func() { answered <- svc.Entitlements() }()
		select {
		case ent := <-answered:
			assert.Equal(t, domain.CommunityEntitlements(), ent,
				"answered from the state it found, not from a read that has not finished")
		case <-time.After(2 * time.Second):
			t.Fatal("Entitlements() waited on the database; a gate must never block on it")
		}

		// The read completes later, and the next caller sees it.
		store.getErr = nil
		store.value = "a-key"
		close(release)
		settle()
		assert.Equal(t, "ACME SAS", svc.Entitlements().Org)
	})

	t.Run("no stored row is an answer and is never re-read", func(t *testing.T) {
		// The ordinary state of every Community installation. Retrying it would be a
		// query per backoff window, forever, on every deployment that never bought
		// anything — which is most of them.
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)

		clock.advance(48 * time.Hour)
		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
		assert.Equal(t, 1, store.gets)
	})

	t.Run("a stored key that does not verify is an answer and is never re-read", func(t *testing.T) {
		// The bytes will not verify any better on a second reading, and retrying them
		// would repeat the same WARN line forever.
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{value: "a-key"}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parserReturning(nil, license.ErrBadSignature), clock.now, alwaysTrusted)

		clock.advance(48 * time.Hour)
		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
		assert.Equal(t, 1, store.gets)
	})

	t.Run("a key from the environment never touches the database, so there is nothing to retry", func(t *testing.T) {
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{getErr: errors.New("connection refused")}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			EnvKey:      "a-key",
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)

		clock.advance(48 * time.Hour)
		assert.Equal(t, "ACME SAS", svc.Entitlements().Org)
		assert.Zero(t, store.gets)
	})

	t.Run("concurrent readers never wait on the database and never race", func(t *testing.T) {
		// Entitlements is called inline on request paths by every gate. A retry must
		// never be something a request queues behind.
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := &fakeLicenseStore{getErr: errors.New("connection refused")}

		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			OIDCEnabled: true,
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)

		clock.advance(licenseRetryInitialBackoff)

		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				svc.Entitlements()
			}()
		}
		wg.Wait()

		assert.NotNil(t, svc.Entitlements().Features)
	})
}

// TestLicenseService_PlaceholderSigningKeyIsAnnounced covers the state every build is in
// until a human generates the real signing pair.
//
// A binary carrying the pubkey_prod.go placeholder refuses every licence key that has ever
// been minted, with ErrNoTrustedKey. From the outside that is indistinguishable from "your
// key is wrong", and the two have completely different remedies — one is the customer's
// problem, the other is entirely ours. The startup line is what tells them apart.
func TestLicenseService_PlaceholderSigningKeyIsAnnounced(t *testing.T) {
	const marker = "PLACEHOLDER"

	t.Run("a key was supplied and can never be accepted: error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		l := pkgmocks.NewMockLogger(ctrl)
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Warn(gomock.Any()).AnyTimes()
		l.EXPECT().Error(gomock.Any()).Do(func(msg string) {
			assert.Contains(t, msg, marker)
			assert.Contains(t, msg, "never accept a licence key")
		}).Times(1)

		// The parser FAILS with ErrNoTrustedKey, which is the only thing a placeholder
		// build can do: pkg/license refuses every key in existence against a public-key
		// slot that verifies nothing. This subtest used to pair noTrustedKey with a parser
		// that SUCCEEDED — a combination the binary cannot produce — and it was green
		// while the branch it names was unreachable, because the source it tests was
		// overwritten to "none" by the very failure that makes this an error.
		//
		// The refusal is the binary's fault, not the key's, which is why this is ERROR:
		// a customer set NOTIFUSE_LICENSE_KEY and is being refused right now.
		newLicenseService(LicenseServiceConfig{
			EnvKey: "a-key",
			Logger: l,
		}, parserReturning(nil, license.ErrNoTrustedKey), time.Now, noTrustedKey)
	})

	t.Run("no key was supplied: still said, at warn", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		l := pkgmocks.NewMockLogger(ctrl)
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Error(gomock.Any()).Times(0)
		l.EXPECT().Warn(gomock.Any()).Do(func(msg string) {
			assert.Contains(t, msg, marker)
		}).Times(1)

		newLicenseService(LicenseServiceConfig{
			SettingRepo: &fakeLicenseStore{},
			Logger:      l,
		}, parserReturning(nil, license.ErrNoTrustedKey), time.Now, noTrustedKey)
	})

	t.Run("a build with a real signing key says nothing about placeholders", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		l := pkgmocks.NewMockLogger(ctrl)
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Error(gomock.Any()).Times(0)
		l.EXPECT().Warn(gomock.Any()).Times(0)

		newLicenseService(LicenseServiceConfig{
			SettingRepo: &fakeLicenseStore{},
			Logger:      l,
		}, parserReturning(nil, license.ErrNoTrustedKey), time.Now, alwaysTrusted)
	})

	t.Run("the announcement never costs the deployment its ability to run", func(t *testing.T) {
		svc := newLicenseService(LicenseServiceConfig{
			EnvKey: "a-key",
			Logger: nil,
		}, parserReturning(nil, license.ErrNoTrustedKey), time.Now, noTrustedKey)

		assert.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())
	})
}

// TestLicenseService_ProductionWiringCarriesTheRealProbe asserts that the exported
// constructor hands the service the compiled-key probe rather than something that always
// says yes. A seam that is only ever exercised through its test double proves nothing about
// the binary that ships.
func TestLicenseService_ProductionWiringCarriesTheRealProbe(t *testing.T) {
	svc := NewLicenseService(LicenseServiceConfig{Logger: quietLogger(t)})

	require.NotNil(t, svc.hasTrustedKey)
	assert.Equal(t, license.HasTrustedKey(), svc.hasTrustedKey())
}

// blockingLicenseStore fails the first read, then holds the second open until a test releases
// it. The first failure is what leaves the question unresolved and schedules the retry; the
// second is the retry, and it is the read a paste has to be ordered against.
//
// It is not fakeLicenseStore with a channel bolted on: that one counts without a mutex and is
// shared by two dozen sequential tests, and racing it would make all of them suspect.
type blockingLicenseStore struct {
	mu    sync.Mutex
	value string
	gets  int

	entered chan struct{} // closed when the SECOND Get is entered — the retry
	release chan struct{} // that Get returns when this is closed
	setDone chan struct{} // closed when Set returns

	enteredOnce sync.Once
	setOnce     sync.Once
}

func newBlockingLicenseStore() *blockingLicenseStore {
	return &blockingLicenseStore{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		setDone: make(chan struct{}),
	}
}

func (b *blockingLicenseStore) Get(ctx context.Context, key string) (*domain.Setting, error) {
	// The value is captured on ENTRY and returned after the block. That is the whole point:
	// a read issued before a write returns the state from before the write, however long it
	// takes to come back. Reading it after unblocking would model a read that started after
	// the paste, which is the case that was never in question.
	b.mu.Lock()
	b.gets++
	n := b.gets
	value := b.value
	b.mu.Unlock()

	if n == 1 {
		// Not an answer: the question stays open and a retry is scheduled.
		return nil, errors.New("connection refused")
	}

	if n == 2 {
		b.enteredOnce.Do(func() { close(b.entered) })
		select {
		case <-b.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if value == "" {
		return nil, &domain.ErrSettingNotFound{Key: key}
	}
	return &domain.Setting{Key: key, Value: value}, nil
}

func (b *blockingLicenseStore) Set(_ context.Context, _, value string) error {
	b.mu.Lock()
	b.value = value
	b.mu.Unlock()
	b.setOnce.Do(func() { close(b.setDone) })
	return nil
}

func (b *blockingLicenseStore) stored() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.value
}

// A licence installed from the console must not be undone by a settings read that started
// before it.
//
// The two writers were ordered by nothing. The lazy retry holds reloadMu across a read bounded
// only by licenseLoadTimeout; SetKey took mu alone. A read issued BEFORE the paste could
// therefore land AFTER it and call markResolved(nil, none), overwriting the freshly verified
// claims with the row as it was a moment earlier. That stale answer also sets resolved = true,
// so no later retry ever repairs it: the deployment runs Community until somebody restarts the
// process, while the correct key sits in the settings table and the console has already been
// told, with a 200 carrying the right entitlements, that the paste worked.
//
// It is the worst failure this file has had, because of when it strikes: the customer has just
// paid, and the product tells them it worked.
func TestLicenseService_SetKeySurvivesAnInFlightRetry(t *testing.T) {
	future := time.Now().Add(365 * 24 * time.Hour)
	installed := testClaims(future)
	installed.Org = "the key that was bought"
	second := testClaims(future)
	second.Org = "the second key"

	parse := func(raw string) (*license.Claims, error) {
		switch raw {
		case "bought-key":
			return installed, nil
		case "second-key":
			return second, nil
		}
		return nil, license.ErrBadSignature
	}

	t.Run("a stale read must not overwrite the key that was just installed", func(t *testing.T) {
		clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
		store := newBlockingLicenseStore()

		// Boot: the first read fails for a reason that is not an answer, so the question
		// stays open and a retry is scheduled.
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, clock.now, alwaysTrusted)
		require.Equal(t, domain.CommunityEntitlements(), svc.Entitlements())

		// The backoff elapses. Any request now triggers the retry — and that read blocks
		// while holding reloadMu, which is the window the paste has to survive.
		clock.advance(licenseRetryInitialBackoff)

		retried := make(chan struct{})
		go func() {
			defer close(retried)
			svc.Entitlements()
		}()
		<-store.entered

		// The operator pastes their key while that read is still open.
		//
		// Waiting on setDone rather than sleeping keeps the reproduction deterministic in
		// the direction that matters: on the broken build the write really does land here
		// and the test proceeds the moment it does. The timeout is only how long the fixed
		// build spends proving it is parked behind the reader.
		installErr := make(chan error, 1)
		go func() { installErr <- svc.SetKey(context.Background(), "bought-key") }()

		select {
		case <-store.setDone:
		case <-time.After(2 * time.Second):
		}

		close(store.release) // the stale read completes and reports "no row stored"

		<-retried
		require.NoError(t, <-installErr)

		ent := svc.Entitlements()
		assert.Equal(t, "the key that was bought", ent.Org,
			"the paid key was overwritten by a settings read that started before it, and no retry will ever repair it")
		assert.Equal(t, domain.LicenseStateActive, ent.State)
		assert.Equal(t, "bought-key", store.stored(),
			"the row and the memory must name the same key")
	})

	// The same absence of serialisation with no retry in sight: two pastes racing can leave
	// the stored row naming one key and the process running on the other, permanently.
	t.Run("concurrent installs leave memory and the stored row agreeing", func(t *testing.T) {
		store := &fakeLicenseStore{}
		svc := newLicenseService(LicenseServiceConfig{
			SettingRepo: store,
			Logger:      quietLogger(t),
		}, parse, time.Now, alwaysTrusted)

		var wg sync.WaitGroup
		for _, key := range []string{"bought-key", "second-key"} {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				_ = svc.SetKey(context.Background(), k)
			}(key)
		}
		wg.Wait()

		orgOf := map[string]string{"bought-key": "the key that was bought", "second-key": "the second key"}
		assert.Equal(t, orgOf[store.setValue], svc.Entitlements().Org,
			"the process is running on a different key from the one it stored")
	})
}

// The startup line has to say whether a key was SEEN, separately from whether it worked.
//
// It used to say neither: the field was set by markResolved, which is called with
// licenseSourceNone on every verification failure, so a deployment that set
// NOTIFUSE_LICENSE_KEY and had it refused logged source: "none". That reads as "your variable
// was never seen", which is exactly the confusion the constant's own doc comment says the
// field exists to prevent — and it is the first thing support looks at.
func TestLicenseService_ReportsWhereTheKeyWasFoundEvenWhenItFailed(t *testing.T) {
	capture := func(t *testing.T, cfg LicenseServiceConfig, parse func(string) (*license.Claims, error)) map[string]interface{} {
		t.Helper()

		ctrl := gomock.NewController(t)
		l := pkgmocks.NewMockLogger(ctrl)

		var resolved map[string]interface{}
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).DoAndReturn(func(fields map[string]interface{}) logger.Logger {
			if _, ok := fields["state"]; ok {
				resolved = fields
			}
			return l
		}).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Warn(gomock.Any()).AnyTimes()
		l.EXPECT().Error(gomock.Any()).AnyTimes()

		cfg.Logger = l
		newLicenseService(cfg, parse, time.Now, alwaysTrusted)
		require.NotNil(t, resolved, "no resolved line was logged")
		return resolved
	}

	t.Run("an environment key that does not verify still names the environment", func(t *testing.T) {
		fields := capture(t, LicenseServiceConfig{EnvKey: "a-key"},
			parserReturning(nil, license.ErrBadSignature))

		assert.Equal(t, licenseSourceEnvironment, fields["key_source"],
			"the operator set NOTIFUSE_LICENSE_KEY; reporting 'none' sends support to look for a variable that is there")
		// And the outcome is still reported honestly next to it.
		assert.Equal(t, string(domain.LicenseStateNone), fields["state"])
	})

	t.Run("a stored key that does not verify still names the database", func(t *testing.T) {
		fields := capture(t, LicenseServiceConfig{SettingRepo: &fakeLicenseStore{value: "a-key"}},
			parserReturning(nil, license.ErrBadSignature))

		assert.Equal(t, licenseSourceDatabase, fields["key_source"])
	})

	// "none" now means one thing only, which is what makes it worth logging.
	t.Run("no key anywhere is the only thing that reports none", func(t *testing.T) {
		fields := capture(t, LicenseServiceConfig{SettingRepo: &fakeLicenseStore{}},
			parserReturning(nil, license.ErrBadSignature))

		assert.Equal(t, licenseSourceNone, fields["key_source"])
	})

	t.Run("a key that works names its source too", func(t *testing.T) {
		fields := capture(t, LicenseServiceConfig{EnvKey: "a-key"},
			parserReturning(testClaims(time.Now().Add(365*24*time.Hour)), nil))

		assert.Equal(t, licenseSourceEnvironment, fields["key_source"])
		assert.Equal(t, string(domain.LicenseStateActive), fields["state"])
	})
}

// panicOnFields is a logger that blows up the moment licence code tries to attach a field —
// the shape of a logging hook that fails. Everything else is the quiet test logger.
type panicOnFields struct{ logger.Logger }

func (p panicOnFields) WithField(key string, value interface{}) logger.Logger {
	panic("the logger is broken")
}
func (p panicOnFields) WithFields(fields map[string]interface{}) logger.Logger {
	panic("the logger is broken")
}

func TestLicenseService_ABrokenLoggerDoesNotCostTheLicence(t *testing.T) {
	future := time.Date(2027, 9, 3, 12, 0, 0, 0, time.UTC)
	claims := testClaims(future)
	claims.Org = "ACME SAS"
	clock := &movableClock{at: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}

	// A valid key is loaded, then the boot-time log lines panic. The constructor used to
	// recover from that by marking the licence resolved with NO claims — a deployment on
	// the free tier for the life of the process, with nothing logged, because the logger
	// was the thing that broke.
	svc := newLicenseService(LicenseServiceConfig{
		SettingRepo: &fakeLicenseStore{value: "a-key"},
		Logger:      panicOnFields{quietLogger(t)},
	}, parserReturning(claims, nil), clock.now, alwaysTrusted)

	ent := svc.Entitlements()
	assert.Equal(t, "ACME SAS", ent.Org, "the claims load() installed must survive a panic in the logging that follows")
	assert.True(t, ent.Has(domain.FeatureRBAC))
}
