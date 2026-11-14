package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// fakeIdP is a minimal OIDC provider (discovery + JWKS + token endpoint) for
// exercising the real go-oidc verification path in HandleCallback.
type fakeIdP struct {
	server  *httptest.Server
	key     *rsa.PrivateKey
	kid     string
	idToken string // the id_token the /token endpoint returns on the next exchange
	// tokenHits counts the calls to the token endpoint.
	tokenHits atomic.Int32
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &fakeIdP{key: key, kid: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer":                 f.server.URL,
			"authorization_endpoint": f.server.URL + "/authorize",
			"token_endpoint":         f.server.URL + "/token",
			"jwks_uri":               f.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := f.key.PublicKey
		n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]interface{}{
				{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": f.kid, "n": n, "e": e},
			},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.tokenHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"id_token":     f.idToken,
		})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

type idTokenOpts struct {
	iss           string
	aud           interface{} // string or []string
	sub           string
	email         string
	emailVerified bool
	name          string
	nonce         string
	azp           string
	typ           string         // header typ; "" omits it
	hs256         bool           // sign with HS256 instead of RS256 (alg-confusion negative test)
	expired       bool
	extra         map[string]any // additional provider-specific claims (e.g. Google's "hd")
}

func (f *fakeIdP) mint(t *testing.T, o idTokenOpts) string {
	t.Helper()
	if o.iss == "" {
		o.iss = f.server.URL
	}
	if o.aud == nil {
		o.aud = testClientID
	}
	exp := time.Now().Add(time.Hour)
	if o.expired {
		exp = time.Now().Add(-time.Hour)
	}
	claims := jwt.MapClaims{
		"iss":            o.iss,
		"aud":            o.aud,
		"sub":            o.sub,
		"email":          o.email,
		"email_verified": o.emailVerified,
		"name":           o.name,
		"nonce":          o.nonce,
		"iat":            time.Now().Unix(),
		"exp":            exp.Unix(),
	}
	if o.azp != "" {
		claims["azp"] = o.azp
	}
	for k, v := range o.extra {
		claims[k] = v
	}

	if o.hs256 {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := tok.SignedString([]byte("symmetric-secret"))
		require.NoError(t, err)
		return signed
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	if o.typ != "" {
		tok.Header["typ"] = o.typ
	}
	signed, err := tok.SignedString(f.key)
	require.NoError(t, err)
	return signed
}

func newCallbackService(t *testing.T, ctrl *gomock.Controller, f *fakeIdP) (*OIDCService, *oidcMocks) {
	cfg := config.OIDCConfig{
		Enabled:      true,
		IssuerURL:    f.server.URL,
		ClientID:     testClientID,
		ClientSecret: "client-secret",
		RedirectURI:  "https://app.example.com/api/user.oidc.callback",
		Scopes:       []string{"openid", "email"},
	}
	return newTestOIDCService(t, ctrl, cfg, nil)
}

// validFlow returns a matching state/nonce flow-state and a callback input wired to it.
func validFlow(nonce string) (domain.OIDCFlowState, domain.OIDCCallbackInput) {
	fs := domain.OIDCFlowState{State: "the-state", Nonce: nonce, Verifier: oauth2Verifier()}
	in := domain.OIDCCallbackInput{Code: "auth-code", State: "the-state", FlowState: fs}
	return fs, in
}

func oauth2Verifier() string {
	// any non-empty PKCE verifier; the fake token endpoint ignores it
	return "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcd"
}

func TestHandleCallback_HappyPath_FoundIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, m := newCallbackService(t, ctrl, f)

	_, in := validFlow("nonce-123")
	f.idToken = f.mint(t, idTokenOpts{sub: "sub-1", email: "u1@corp.com", emailVerified: true, nonce: "nonce-123"})

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), f.server.URL, "sub-1").
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: f.server.URL, IDPSub: "sub-1"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u1").Return(&domain.User{ID: "u1", Email: "u1@corp.com"}, nil)
	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	m.authSvc.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("minted-jwt")

	code, err := svc.HandleCallback(context.Background(), in)
	require.NoError(t, err)
	assert.NotEmpty(t, code)

	// The one-time code resolves to the minted session, single-use.
	resp, err := svc.ExchangeCode(context.Background(), code)
	require.NoError(t, err)
	assert.Equal(t, "minted-jwt", resp.Token)
}

func TestHandleCallback_StateMismatch_NoExchange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	in := domain.OIDCCallbackInput{
		Code:      "auth-code",
		State:     "attacker-state",
		FlowState: domain.OIDCFlowState{State: "real-state", Nonce: "n", Verifier: oauth2Verifier()},
	}
	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state mismatch")
}

func TestHandleCallback_NonceMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("expected-nonce")
	f.idToken = f.mint(t, idTokenOpts{sub: "s", email: "e@corp.com", emailVerified: true, nonce: "WRONG-nonce"})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonce mismatch")
}

func TestHandleCallback_AtJwtTypRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	f.idToken = f.mint(t, idTokenOpts{sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n", typ: "at+jwt"})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access-token typ")
}

func TestHandleCallback_HS256Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	f.idToken = f.mint(t, idTokenOpts{sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n", hs256: true})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err, "HS256 token must be rejected (alg pinned to RS256/ES256)")
}

func TestHandleCallback_WrongIssuerRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	// Signed by our key but claims a different issuer.
	f.idToken = f.mint(t, idTokenOpts{iss: "https://evil.example.com", sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n"})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err, "issuer mismatch must be rejected")
}

func TestHandleCallback_AzpMismatchMultiAudience(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	// genuine second distinct audience + azp != clientID
	f.idToken = f.mint(t, idTokenOpts{
		aud: []string{testClientID, "another-client"}, azp: "another-client",
		sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n",
	})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "azp mismatch")
}

func TestHandleCallback_MultiAudienceWithCorrectAzpAccepted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, m := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	f.idToken = f.mint(t, idTokenOpts{
		aud: []string{testClientID, "another-client"}, azp: testClientID,
		sub: "s2", email: "u2@corp.com", emailVerified: true, nonce: "n",
	})
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), f.server.URL, "s2").
		Return(&domain.FederatedIdentity{UserID: "u2", IDPIssuer: f.server.URL, IDPSub: "s2"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u2").Return(&domain.User{ID: "u2"}, nil)
	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	m.authSvc.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("jwt2")

	code, err := svc.HandleCallback(context.Background(), in)
	require.NoError(t, err)
	assert.NotEmpty(t, code)
}

func TestHandleCallback_EmailNotVerified_FirstLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, m := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	f.idToken = f.mint(t, idTokenOpts{sub: "s", email: "e@corp.com", emailVerified: false, nonce: "n"})
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), f.server.URL, "s").Return(nil, notFoundFI())

	_, err := svc.HandleCallback(context.Background(), in)
	require.ErrorIs(t, err, domain.ErrOIDCEmailNotVerified)
}

func TestBuildAuthURL_WithProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	req, err := svc.BuildAuthURL(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, req.FlowState.State)
	assert.NotEmpty(t, req.FlowState.Nonce)
	assert.NotEqual(t, req.FlowState.State, req.FlowState.Nonce)
	for _, want := range []string{"code_challenge=", "code_challenge_method=S256", "nonce=", "state=", "response_type=code"} {
		assert.True(t, strings.Contains(req.AuthURL, want), "auth URL missing %q", want)
	}
}

// TestHandleCallback_GoogleWorkspaceShapedToken proves compatibility with the exact
// ID-token shape Google Workspace issues: single aud == client_id, an azp claim
// also == client_id, a numeric string sub, email_verified as a JSON bool, header
// typ "JWT" (must NOT trip the at+jwt access-token guard), and a Workspace-only "hd"
// (hosted domain) claim that the app does not consume but must tolerate.
func TestHandleCallback_GoogleWorkspaceShapedToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, m := newCallbackService(t, ctrl, f)

	_, in := validFlow("google-nonce")
	f.idToken = f.mint(t, idTokenOpts{
		aud:           testClientID, // Google: single audience == client_id
		azp:           testClientID, // Google always sets azp
		sub:           "108273645210987654321", // Google's stable numeric subject id
		email:         "alice@acme-corp.com",
		emailVerified: true,        // Google sends a JSON boolean
		name:          "Alice Example",
		nonce:         "google-nonce",
		typ:           "JWT",       // Google's id_token header typ — must pass the at+jwt guard
		extra:         map[string]any{"hd": "acme-corp.com"}, // Workspace hosted-domain claim, ignored
	})

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), f.server.URL, "108273645210987654321").
		Return(&domain.FederatedIdentity{UserID: "u-google", IDPIssuer: f.server.URL, IDPSub: "108273645210987654321"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u-google").Return(&domain.User{ID: "u-google", Email: "alice@acme-corp.com"}, nil)
	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	m.authSvc.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("google-jwt")

	code, err := svc.HandleCallback(context.Background(), in)
	require.NoError(t, err, "a Google-Workspace-shaped ID token must be accepted")
	require.NotEmpty(t, code)
}

func TestHandleCallback_ExpiredIDTokenRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	// Validly-signed but EXPIRED — go-oidc Verify must reject (Skip*Expiry left false).
	f.idToken = f.mint(t, idTokenOpts{sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n", expired: true})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err, "an expired id_token must be rejected; no session may be minted")
}

func TestHandleCallback_AudienceExcludesClientID_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	// aud is a different client; our ClientID is absent. With a forged azp==ClientID,
	// the azp check alone would pass — Verify's audience check must still reject.
	f.idToken = f.mint(t, idTokenOpts{
		aud: []string{"some-other-client"}, azp: testClientID,
		sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n",
	})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err, "an id_token whose aud excludes our ClientID must be rejected")
}
