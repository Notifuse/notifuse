package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	testIssuer   = "https://idp.example.com"
	testClientID = "client-abc"
)

type oidcMocks struct {
	userRepo *mocks.MockUserRepository
	fedRepo  *mocks.MockFederatedIdentityRepository
	authSvc  *mocks.MockAuthService
	cache    cache.Cache
}

func newTestOIDCService(t *testing.T, ctrl *gomock.Controller, cfg config.OIDCConfig, isRoot func(string) bool) (*OIDCService, *oidcMocks) {
	t.Helper()
	if isRoot == nil {
		isRoot = func(string) bool { return false }
	}
	m := &oidcMocks{
		userRepo: mocks.NewMockUserRepository(ctrl),
		fedRepo:  mocks.NewMockFederatedIdentityRepository(ctrl),
		authSvc:  mocks.NewMockAuthService(ctrl),
		cache:    cache.NewInMemoryCache(time.Hour),
	}
	svc := NewOIDCService(OIDCServiceConfig{
		UserRepo:              m.userRepo,
		FederatedIdentityRepo: m.fedRepo,
		AuthService:           m.authSvc,
		OIDCConfig:            cfg,
		SessionExpiry:         30 * 24 * time.Hour,
		ExchangeCache:         m.cache,
		IsRootEmail:           isRoot,
		Logger:                logger.NewLogger(),
	})
	return svc, m
}

func enabledCfg() config.OIDCConfig {
	return config.OIDCConfig{
		Enabled:   true,
		IssuerURL: testIssuer,
		ClientID:  testClientID,
		Scopes:    []string{"openid", "email"},
	}
}

func notFoundFI() error  { return &domain.ErrFederatedIdentityNotFound{Message: "nf"} }
func notFoundUsr() error { return &domain.ErrUserNotFound{Message: "nf"} }

// --- pure helpers -----------------------------------------------------------

func TestRandToken(t *testing.T) {
	a, err := randToken(32)
	require.NoError(t, err)
	b, err := randToken(32)
	require.NoError(t, err)
	assert.NotEmpty(t, a)
	assert.NotEqual(t, a, b, "tokens must be unique")
	raw, err := base64.RawURLEncoding.DecodeString(a)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
}

func TestDomainAllowed(t *testing.T) {
	allow := []string{"corp.com", "Sub.Corp.com"}
	assert.True(t, domainAllowed("alice@corp.com", allow))
	assert.True(t, domainAllowed("bob@CORP.com", allow))
	assert.True(t, domainAllowed("c@sub.corp.com", allow))
	assert.False(t, domainAllowed("x@other.com", allow))
	assert.False(t, domainAllowed("no-at", allow))
}

func TestHasDistinctSecondAudience(t *testing.T) {
	assert.False(t, hasDistinctSecondAudience(audience{testClientID}, testClientID), "single aud")
	assert.False(t, hasDistinctSecondAudience(audience{testClientID, testClientID}, testClientID), "duplicate of self")
	assert.True(t, hasDistinctSecondAudience(audience{testClientID, "other"}, testClientID), "genuine second audience")
	assert.True(t, hasDistinctSecondAudience(audience{"other"}, testClientID), "only a foreign audience")
}

func TestRejectAccessTokenTyp(t *testing.T) {
	mk := func(typ string) string {
		hdr := map[string]string{"alg": "RS256"}
		if typ != "" {
			hdr["typ"] = typ
		}
		hb, _ := json.Marshal(hdr)
		return base64.RawURLEncoding.EncodeToString(hb) + ".payload.sig"
	}
	assert.NoError(t, rejectAccessTokenTyp(mk("JWT")))
	assert.NoError(t, rejectAccessTokenTyp(mk("")))
	assert.Error(t, rejectAccessTokenTyp(mk("at+jwt")))
	assert.Error(t, rejectAccessTokenTyp(mk("AT+JWT")), "case-insensitive")
	assert.Error(t, rejectAccessTokenTyp(mk("application/at+jwt")))
	assert.Error(t, rejectAccessTokenTyp("not-a-jwt"))
}

func TestAudienceUnmarshalJSON(t *testing.T) {
	var single audience
	require.NoError(t, json.Unmarshal([]byte(`"abc"`), &single))
	assert.Equal(t, audience{"abc"}, single)

	var multi audience
	require.NoError(t, json.Unmarshal([]byte(`["a","b"]`), &multi))
	assert.Equal(t, audience{"a", "b"}, multi)

	var bad audience
	assert.Error(t, json.Unmarshal([]byte(`123`), &bad))
}

// --- IsEnabled / ExchangeCode ----------------------------------------------

func TestOIDCService_IsEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	on, _ := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	assert.True(t, on.IsEnabled())
	off, _ := newTestOIDCService(t, ctrl, config.OIDCConfig{Enabled: false}, nil)
	assert.False(t, off.IsEnabled())
}

func TestOIDCService_ExchangeCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _ := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	resp := &domain.AuthResponse{Token: "jwt", User: domain.User{ID: "u1"}}
	svc.exchangeCache.Set(oidcExchangeKeyPrefix+"code1", resp, oidcExchangeTTL)

	got, err := svc.ExchangeCode(context.Background(), "code1")
	require.NoError(t, err)
	assert.Equal(t, "jwt", got.Token)

	// single-use: second call fails
	_, err = svc.ExchangeCode(context.Background(), "code1")
	assert.Error(t, err)

	// empty / unknown
	_, err = svc.ExchangeCode(context.Background(), "")
	assert.Error(t, err)
	_, err = svc.ExchangeCode(context.Background(), "nope")
	assert.Error(t, err)
}

func TestOIDCService_ExchangeCode_DisabledShortCircuits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _ := newTestOIDCService(t, ctrl, config.OIDCConfig{Enabled: false}, nil)
	_, err := svc.ExchangeCode(context.Background(), "x")
	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
}

// --- flow-state seal/open (AEAD) -------------------------------------------

func TestOIDCService_SealOpenFlowState_RoundTrip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	svc, _ := newTestOIDCService(t, ctrl, cfg, nil)
	svc.secretKey = "test-secret-key-1234567890123456"

	fs := domain.OIDCFlowState{State: "s-val", Nonce: "n-val", Verifier: "v-val"}
	enc, err := svc.SealFlowState(fs)
	require.NoError(t, err)
	assert.NotEmpty(t, enc)
	assert.NotContains(t, enc, "s-val", "sealed blob must be opaque ciphertext")

	got, err := svc.OpenFlowState(enc)
	require.NoError(t, err)
	assert.Equal(t, fs, got)

	// Tampered/garbage ciphertext must fail, not silently return a zero value.
	_, err = svc.OpenFlowState("deadbeef")
	assert.Error(t, err)
}

func TestOIDCService_OpenFlowState_RejectsTamperedCiphertext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _ := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	svc.secretKey = "test-secret-key-1234567890123456"

	sealed, err := svc.SealFlowState(domain.OIDCFlowState{State: "s-val", Nonce: "n-val", Verifier: "v-val"})
	require.NoError(t, err)

	raw, err := hex.DecodeString(sealed)
	require.NoError(t, err)
	// Layout is [12-byte GCM nonce || ciphertext+tag]. Flip a byte PAST the nonce so we
	// exercise the AES-GCM authentication tag, not just the length guard — this would
	// PASS if SealFlowState ever swapped to an unauthenticated cipher (CBC/CTR).
	require.Greater(t, len(raw), 13)
	raw[12] ^= 0xFF
	tampered := hex.EncodeToString(raw)

	_, err = svc.OpenFlowState(tampered)
	assert.Error(t, err, "AES-GCM must reject a flipped ciphertext byte (authenticated encryption)")
}

// --- mintSession ------------------------------------------------------------

func TestOIDCService_MintSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	user := &domain.User{ID: "u1", Email: "u1@corp.com"}

	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sess *domain.Session) error {
			assert.Equal(t, "u1", sess.UserID)
			assert.Nil(t, sess.MagicCode, "OIDC session must have no magic code")
			assert.Nil(t, sess.MagicCodeExpires)
			return nil
		})
	m.authSvc.EXPECT().GenerateUserAuthToken(user, gomock.Any(), gomock.Any()).Return("signed-jwt")

	resp, err := svc.mintSession(context.Background(), user)
	require.NoError(t, err)
	assert.Equal(t, "signed-jwt", resp.Token)
	assert.Equal(t, "u1", resp.User.ID)
}

func TestOIDCService_MintSession_EmptyTokenFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	user := &domain.User{ID: "u1"}
	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	m.authSvc.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("")
	_, err := svc.mintSession(context.Background(), user)
	assert.Error(t, err)
}

// --- resolveOrProvisionUser: the identity state machine ---------------------

func TestResolve_FoundByIssuerSub_NoEmailLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "sub-1"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u1").Return(&domain.User{ID: "u1"}, nil)
	// No GetUserByEmail* expected — would fail the controller if called.

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "u1@corp.com", boolPtr(true), "U1")
	require.NoError(t, err)
	assert.Equal(t, "u1", u.ID)
}

func TestResolve_FirstLogin_EmailNotVerified(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "u1@corp.com", boolPtr(false), "")
	assert.ErrorIs(t, err, domain.ErrOIDCEmailNotVerified)
}

func TestResolve_FirstLogin_BridgeInvitedUser_CaseInsensitive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	existing := &domain.User{ID: "u1", Email: "Jane@Corp.com"}
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	// lowercased IdP email must hit the case-insensitive lookup, not exact-match.
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "jane@corp.com").Return(existing, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).Return(nil, notFoundFI())
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fi *domain.FederatedIdentity) error {
			assert.Equal(t, "u1", fi.UserID)
			assert.Equal(t, "sub-1", fi.IDPSub)
			return nil
		})

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "jane@corp.com", boolPtr(true), "")
	require.NoError(t, err)
	assert.Equal(t, "u1", u.ID)
}

func TestFlexibleBool_UnmarshalJSON(t *testing.T) {
	// Some IdPs / claim-mapping bridges emit email_verified as the STRING
	// "true"/"false"; the decode must tolerate that instead of failing the whole
	// claims struct. Absent and null both leave the pointer nil.
	type claims struct {
		EmailVerified *flexibleBool `json:"email_verified"`
	}

	var c claims
	require.NoError(t, json.Unmarshal([]byte(`{"email_verified": true}`), &c))
	require.NotNil(t, c.EmailVerified)
	assert.True(t, bool(*c.EmailVerified))

	c = claims{}
	require.NoError(t, json.Unmarshal([]byte(`{"email_verified": "false"}`), &c))
	require.NotNil(t, c.EmailVerified)
	assert.False(t, bool(*c.EmailVerified))

	c = claims{}
	require.NoError(t, json.Unmarshal([]byte(`{"email_verified": "true"}`), &c))
	require.NotNil(t, c.EmailVerified)
	assert.True(t, bool(*c.EmailVerified))

	c = claims{}
	require.NoError(t, json.Unmarshal([]byte(`{}`), &c))
	assert.Nil(t, c.EmailVerified, "absent claim stays nil")

	c = claims{}
	require.NoError(t, json.Unmarshal([]byte(`{"email_verified": null}`), &c))
	assert.Nil(t, c.EmailVerified, "explicit null behaves like absent")

	c = claims{}
	assert.Error(t, json.Unmarshal([]byte(`{"email_verified": "maybe"}`), &c),
		"a non-boolean string must still fail loudly")
}

func TestResolve_FirstLogin_ExplicitFalse_FlagOn_StillRejected(t *testing.T) {
	// An explicit email_verified=false is rejected even with the tolerance flag on:
	// the flag only rescues an ABSENT claim, never an IdP-asserted "unverified".
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AllowUnverifiedEmail = true
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "u1@corp.com", boolPtr(false), "")
	assert.ErrorIs(t, err, domain.ErrOIDCEmailNotVerified)
}

func TestResolve_FirstLogin_AbsentClaim_FlagOff_Rejected(t *testing.T) {
	// Default posture: an id_token without email_verified is rejected on first login.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "u1@corp.com", nil, "")
	assert.ErrorIs(t, err, domain.ErrOIDCEmailNotVerified)
}

func TestResolve_FirstLogin_AbsentClaim_FlagOn_BridgesInvitedUser(t *testing.T) {
	// With OIDC_ALLOW_UNVERIFIED_EMAIL=true, an absent claim proceeds to the normal
	// invited-user bridge (Entra ID / Cloudflare Access never emit email_verified).
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AllowUnverifiedEmail = true
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	existing := &domain.User{ID: "u1", Email: "jane@corp.com"}
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "jane@corp.com").Return(existing, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).Return(nil, notFoundFI())
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "jane@corp.com", nil, "")
	require.NoError(t, err)
	assert.Equal(t, "u1", u.ID)
}

func TestResolve_ReLogin_AbsentClaim_GateNotReached(t *testing.T) {
	// An already-linked identity ((issuer,sub) hit) re-logs-in regardless of the
	// claim or the flag — the email_verified gate only guards first logins.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "sub-1"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u1").Return(&domain.User{ID: "u1"}, nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "u1@corp.com", nil, "U1")
	require.NoError(t, err)
	assert.Equal(t, "u1", u.ID)
}

func TestResolve_FirstLogin_AbsentClaim_FlagOn_EmptyEmail_Rejected(t *testing.T) {
	// The flag rescues a missing claim, not a missing email: an id_token with no
	// email at all still cannot link or provision anything.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AllowUnverifiedEmail = true
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "", nil, "")
	assert.ErrorIs(t, err, domain.ErrOIDCEmailNotVerified)
}

func TestResolve_Bridge_APIKeyIdentity_RefusedAndNotLinked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	// An api_key row carries a real, unique email, so an IdP asserting it reaches
	// the invited-user bridge exactly like a human would.
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "zapier@api.corp.com").
		Return(&domain.User{ID: "apikey-1", Email: "zapier@api.corp.com", Type: domain.UserTypeAPIKey}, nil)
	// MUST NOT link: no GetByUserAndIssuer, no fedRepo.Create expected.

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "zapier@api.corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned,
		"an API key identity must never be bridged onto a human session")
}

func TestResolve_LinkedAPIKeyIdentity_StillRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	// A link to an api_key can only predate the bridge guard; it must not keep
	// minting sessions through the (issuer,sub) re-login branch.
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").
		Return(&domain.FederatedIdentity{UserID: "apikey-1", IDPIssuer: testIssuer, IDPSub: "sub-1"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "apikey-1").
		Return(&domain.User{ID: "apikey-1", Email: "zapier@api.corp.com", Type: domain.UserTypeAPIKey}, nil)

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "zapier@api.corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
}

func TestResolve_FirstLogin_LinkConflict_DifferentSubSameIssuer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	existing := &domain.User{ID: "u1", Email: "jane@corp.com"}
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-NEW").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "jane@corp.com").Return(existing, nil)
	// user already has a DIFFERENT sub for this issuer → conflict (no Create).
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "sub-OLD"}, nil)

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-NEW", "jane@corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCIdentityConflict)
}

func TestResolve_InvitedOnly_NoAccount_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg() // AutoCreateUsers=false
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "ghost@corp.com").Return(nil, notFoundUsr())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "ghost@corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
}

func TestResolve_Bridge_RootEmailGuard_RefusesFirstTimeLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg() // JIT OFF — the most locked-down config
	// Case-insensitive root matcher (as wired in app.go for the OIDC service).
	svc, m := newTestOIDCService(t, ctrl, cfg, func(e string) bool { return strings.EqualFold(e, "root@corp.com") })

	// First login: (issuer,sub) unknown, verified email == ROOT_EMAIL, account exists.
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "attacker-sub").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "root@corp.com").
		Return(&domain.User{ID: "root-id", Email: "root@corp.com"}, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "root-id", testIssuer).Return(nil, notFoundFI())
	// MUST NOT link or mint: no fedRepo.Create expected.

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "attacker-sub", "root@corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned,
		"first-time linking a ROOT_EMAIL account must be refused (privilege escalation)")
}

func TestResolve_Bridge_RootEmailGuard_CaseInsensitive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	// ROOT_EMAIL configured with mixed case; guard must still fire.
	svc, m := newTestOIDCService(t, ctrl, cfg, func(e string) bool { return strings.EqualFold(e, "Root@Corp.com") })

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "s").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "root@corp.com").
		Return(&domain.User{ID: "root-id", Email: "Root@Corp.com"}, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "root-id", testIssuer).Return(nil, notFoundFI())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "s", "root@corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
}

func TestResolve_Bridge_AlreadyLinkedRoot_AllowsReLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	svc, m := newTestOIDCService(t, ctrl, cfg, func(e string) bool { return strings.EqualFold(e, "root@corp.com") })

	// An INTENTIONALLY pre-linked root identity (same sub) must still be able to log in.
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "root-sub").
		Return(&domain.FederatedIdentity{UserID: "root-id", IDPIssuer: testIssuer, IDPSub: "root-sub"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "root-id").Return(&domain.User{ID: "root-id", Email: "root@corp.com"}, nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "root-sub", "root@corp.com", boolPtr(true), "")
	require.NoError(t, err, "an already-linked root identity (found by issuer,sub) must re-login")
	assert.Equal(t, "root-id", u.ID)
}

func TestResolve_JIT_RootEmailGuard_Refused(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	// email matches a configured ROOT_EMAIL → must refuse, no CreateUser.
	svc, m := newTestOIDCService(t, ctrl, cfg, func(e string) bool { return e == "root@corp.com" })

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "root@corp.com").Return(nil, notFoundUsr())
	// No CreateUser expected.

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "root@corp.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
}

func TestResolve_JIT_DomainNotAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "x@evil.com").Return(nil, notFoundUsr())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "x@evil.com", boolPtr(true), "")
	assert.ErrorIs(t, err, domain.ErrOIDCDomainNotAllowed)
}

func TestResolve_JIT_CreatesUserAndLinks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr())
	m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u *domain.User) error {
			assert.Equal(t, "new@corp.com", u.Email)
			assert.Equal(t, domain.UserTypeUser, u.Type)
			assert.Equal(t, domain.DefaultLanguageCode, u.Language)
			return nil
		})
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "New User")
	require.NoError(t, err)
	assert.Equal(t, "new@corp.com", u.Email)
}

// --- linkIdentity conflict semantics ---------------------------------------

func TestLinkIdentity_ExactDuplicateRace_IdempotentSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(&domain.ErrFederatedIdentityExists{Message: "dup"})
	// re-read shows the SAME (user, issuer, sub) → benign race → success
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "sub-1"}, nil)

	err := svc.linkIdentity(context.Background(), "u1", testIssuer, "sub-1")
	assert.NoError(t, err)
}

func TestLinkIdentity_ConflictDifferentSub_Refused(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(&domain.ErrFederatedIdentityExists{Message: "dup"})
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "OTHER"}, nil)

	err := svc.linkIdentity(context.Background(), "u1", testIssuer, "sub-1")
	assert.ErrorIs(t, err, domain.ErrOIDCIdentityConflict)
}

// --- BuildAuthURL / ensureProvider degradation ------------------------------

func TestBuildAuthURL_DisabledReturnsNotConfigured(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, _ := newTestOIDCService(t, ctrl, config.OIDCConfig{Enabled: false}, nil)
	_, err := svc.BuildAuthURL(context.Background())
	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
}

func TestEnsureProvider_HungIssuer_FailsFastNotWedged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// An issuer that ACCEPTS the connection but never responds to discovery. The
	// handler returns when the client cancels (i.e. when our bounded Timeout fires) so
	// httptest's Close() doesn't block on a wedged connection during teardown.
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done(): // client gave up -> release the connection
		case <-time.After(30 * time.Second): // safety cap
		}
	}))
	defer idp.Close()

	cfg := enabledCfg()
	cfg.IssuerURL = idp.URL
	svc, _ := newTestOIDCService(t, ctrl, cfg, nil)
	svc.httpTimeout = 300 * time.Millisecond // bound well under any default

	start := time.Now()
	err := svc.ensureProvider(context.Background())
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured, "a hung issuer must degrade, not hang")
	assert.Less(t, elapsed, 3*time.Second,
		"ensureProvider must return within the bounded HTTP timeout, not hang on the issuer")
}

func TestEnsureProvider_UnreachableIssuer_SelfHealRetryWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	// Loopback port 1 → connection refused immediately (fast-fail, no 30s timeout).
	cfg.IssuerURL = "http://127.0.0.1:1/oidc"
	svc, _ := newTestOIDCService(t, ctrl, cfg, nil)

	err := svc.ensureProvider(context.Background())
	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
	assert.Nil(t, svc.provider)
	first := svc.lastAttempt
	assert.False(t, first.IsZero(), "lastAttempt must be recorded")

	// Within the retry window: short-circuits without re-dialing (lastAttempt unchanged).
	err = svc.ensureProvider(context.Background())
	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
	assert.Equal(t, first, svc.lastAttempt, "should not re-attempt within the retry window")

	// Simulate the window elapsing: it must RE-ATTEMPT (unlike a plain sync.Once),
	// advancing lastAttempt even though the issuer is still unreachable.
	svc.lastAttempt = time.Now().Add(-oidcInitRetryWindow - time.Second)
	err = svc.ensureProvider(context.Background())
	assert.ErrorIs(t, err, domain.ErrOIDCNotConfigured)
	assert.True(t, svc.lastAttempt.After(first), "must re-attempt after the retry window elapses")
}

// --- token-endpoint error sanitisation --------------------------------------
//
// oauth2.RetrieveError.Error() prints the endpoint's raw response body whenever the
// response is not a well-formed OAuth error object. That error used to be %w-wrapped
// and logged by the callback handler, so an unvetted IdP payload reached the log.

func retrieveErr(status int, code string, body string) error {
	return &oauth2.RetrieveError{
		Response:  &http.Response{Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), StatusCode: status},
		Body:      []byte(body),
		ErrorCode: code,
	}
}

func TestSanitizeTokenExchangeError_DropsTheRawResponseBody(t *testing.T) {
	const sentinel = "SENSITIVE-IDP-PAYLOAD-9f3a"
	// No ErrorCode → oauth2 takes the branch that prints Body verbatim.
	raw := retrieveErr(http.StatusBadGateway, "", "<html>gateway said "+sentinel+"</html>")
	require.Contains(t, raw.Error(), sentinel,
		"guard: if oauth2 stops printing the body this test is no longer testing anything")

	got := sanitizeTokenExchangeError(raw)

	assert.NotContains(t, got.Error(), sentinel, "the endpoint's response body must not survive into the error")
	assert.Contains(t, got.Error(), "502", "the status is the diagnostic worth keeping")
}

func TestSanitizeTokenExchangeError_DoesNotWrapTheOriginal(t *testing.T) {
	const sentinel = "SENSITIVE-IDP-PAYLOAD-9f3a"
	got := sanitizeTokenExchangeError(retrieveErr(http.StatusBadRequest, "", sentinel))

	// Sanitising the message is not enough: a wrapped original is still reachable by
	// anything that walks the chain and calls Error() on what it finds.
	var re *oauth2.RetrieveError
	assert.False(t, errors.As(got, &re),
		"the RetrieveError must not stay reachable through the error chain")
}

func TestSanitizeTokenExchangeError_KeepsARecognisableErrorCode(t *testing.T) {
	got := sanitizeTokenExchangeError(retrieveErr(http.StatusBadRequest, "invalid_grant", `{"error":"invalid_grant"}`))

	assert.Contains(t, got.Error(), "invalid_grant", "the RFC 6749 code is what tells an operator what broke")
	assert.Contains(t, got.Error(), "400")
}

func TestSanitizeTokenExchangeError_RefusesAFreeTextErrorCode(t *testing.T) {
	// The error field is IdP-controlled and the spec allows almost any printable
	// character, so a payload can be smuggled through it just as easily as the body.
	const smuggled = `{"access_token":"SENSITIVE-IDP-PAYLOAD-9f3a"}`
	got := sanitizeTokenExchangeError(retrieveErr(http.StatusBadRequest, smuggled, ""))

	assert.NotContains(t, got.Error(), "SENSITIVE-IDP-PAYLOAD-9f3a")
	assert.Contains(t, got.Error(), "unrecognized")
}

func TestSafeOAuthErrorCode(t *testing.T) {
	cases := map[string]string{
		"":                      "",
		"invalid_grant":         "invalid_grant",
		"unauthorized_client":   "unauthorized_client",
		"server_error.sub-code": "server_error.sub-code", // extension codes still pass
		"has space":             "unrecognized",
		`{"error":"x"}`:         "unrecognized",
		"with\nnewline":         "unrecognized",
		strings.Repeat("a", 65): "unrecognized",
		strings.Repeat("a", 64): strings.Repeat("a", 64),
	}
	for in, want := range cases {
		assert.Equal(t, want, safeOAuthErrorCode(in), "input %q", in)
	}
}

func TestTruncateForLog(t *testing.T) {
	assert.Equal(t, "short", truncateForLog("short", 256), "an ordinary message passes through untouched")

	long := strings.Repeat("x", 300)
	got := truncateForLog(long, 256)
	assert.Len(t, []rune(got), 256+len([]rune("…(truncated)")))
	assert.Contains(t, got, "truncated", "a trimmed message must say so, or it reads as the whole thing")

	// Rune-based, so a multi-byte body cannot be cut mid-character into mojibake.
	assert.Equal(t, "éé…(truncated)", truncateForLog("ééé", 2))
}

func TestSanitizeTokenExchangeError_KeepsTransportErrorsIntact(t *testing.T) {
	// A dial/TLS/timeout failure carries Go's own text, not an endpoint payload, and
	// is the most useful thing an operator can see. It must not be flattened away.
	orig := errors.New(`Post "https://idp.example.com/token": dial tcp 10.0.0.1:443: connect: connection refused`)
	got := sanitizeTokenExchangeError(orig)

	assert.ErrorIs(t, got, orig, "a transport error must stay wrapped and inspectable")
	assert.Contains(t, got.Error(), "connection refused")
}

// The API-client view: drive the real callback against an IdP whose token endpoint
// answers with an unparseable body, and assert the caller never receives it. A unit
// test of the helper proves the helper works, not that HandleCallback uses it.
func TestHandleCallback_TokenEndpointBodyNeverReachesTheCaller(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const sentinel = "SENSITIVE-IDP-PAYLOAD-9f3a"

	var idp *httptest.Server
	idp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"issuer":                 idp.URL,
				"authorization_endpoint": idp.URL + "/auth",
				"token_endpoint":         idp.URL + "/token",
				"jwks_uri":               idp.URL + "/jwks",
			})
		case "/token":
			// A reverse proxy in front of the IdP answering instead of the IdP: HTML,
			// not an OAuth error object, so oauth2 falls back to printing it raw.
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>upstream failed: " + sentinel + "</html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer idp.Close()

	cfg := enabledCfg()
	cfg.IssuerURL = idp.URL
	svc, _ := newTestOIDCService(t, ctrl, cfg, nil)

	_, err := svc.HandleCallback(context.Background(), domain.OIDCCallbackInput{
		Code:      "authz-code",
		State:     "st",
		FlowState: domain.OIDCFlowState{State: "st", Nonce: "n", Verifier: "verifier"},
	})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), sentinel,
		"the token endpoint's response body must not reach the caller, which logs this error")
	assert.Contains(t, err.Error(), "502", "the status must survive so the failure is still diagnosable")
}
