package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// --- HandleCallback ---------------------------------------------------------

// RFC 9207: a callback naming another issuer is refused before the code is
// exchanged, so the code never reaches a token endpoint.
func TestHandleCallback_IssParameterMismatch_NoExchange(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	in.Iss = "https://evil.example.com"
	f.idToken = f.mint(t, idTokenOpts{sub: "s", email: "e@corp.com", emailVerified: true, nonce: "n"})

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc issuer mismatch")
	assert.Zero(t, f.tokenHits.Load(), "the code must not be exchanged")
}

// A matching ?iss= is no obstacle.
func TestHandleCallback_IssParameterMatches_Proceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newFakeIdP(t)
	svc, m := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	in.Iss = f.server.URL
	f.idToken = f.mint(t, idTokenOpts{sub: "sub-1", email: "u1@corp.com", emailVerified: true, nonce: "n"})
	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), f.server.URL, "sub-1").
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: f.server.URL, IDPSub: "sub-1"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u1").Return(&domain.User{ID: "u1", Email: "u1@corp.com"}, nil)
	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	m.authSvc.EXPECT().GenerateUserAuthToken(gomock.Any(), gomock.Any(), gomock.Any()).Return("minted-jwt")

	code, err := svc.HandleCallback(context.Background(), in)
	require.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.Equal(t, int32(1), f.tokenHits.Load())
}

// A token response without an id_token is refused before any user lookup
// (strict mocks: no repository expectation).
func TestHandleCallback_MissingIDToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newFakeIdP(t)
	svc, _ := newCallbackService(t, ctrl, f)

	_, in := validFlow("n")
	f.idToken = ""

	_, err := svc.HandleCallback(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id_token")
}

// --- resolveOrProvisionUser: every lookup failure fails closed ---------------

// Strict mocks throughout: a test that sets no expectation for a call asserts
// the call is never made.

func TestResolve_LinkedUserLoadFails_NoEmailLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "sub-1"}, nil)
	m.userRepo.EXPECT().GetUserByID(gomock.Any(), "u1").Return(nil, errors.New("db down"))

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "u1@corp.com", boolPtr(true), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc linked user load")
	assert.Nil(t, u)
}

func TestResolve_FederatedLookupFails_NoEmailLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, errors.New("db down"))

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc federated lookup")
	assert.Nil(t, u)
}

// The link landed between the two lookups (another login of the same person):
// the user is returned and no second link is written.
func TestResolve_Bridge_SameSubAlreadyLinked_NoNewLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	existing := &domain.User{ID: "u1", Email: "invited@corp.com", Type: domain.UserTypeUser}

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "invited@corp.com").Return(existing, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).
		Return(&domain.FederatedIdentity{UserID: "u1", IDPIssuer: testIssuer, IDPSub: "sub-1"}, nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "invited@corp.com", boolPtr(true), "", true)
	require.NoError(t, err)
	assert.Equal(t, "u1", u.ID)
}

func TestResolve_Bridge_UserIssuerLookupFails_NoLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	existing := &domain.User{ID: "u1", Email: "invited@corp.com", Type: domain.UserTypeUser}

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "invited@corp.com").Return(existing, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).Return(nil, errors.New("db down"))

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "invited@corp.com", boolPtr(true), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc user-issuer lookup")
	assert.Nil(t, u)
}

func TestResolve_UserByEmailLookupFails_NoCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, errors.New("db down"))

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc user-by-email lookup")
	assert.Nil(t, u)
}

// Boot refuses JIT without an allowlist; the service refuses it again on its own.
func TestResolve_JIT_EmptyAllowlist_NotProvisioned(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = nil
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr())

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", true)
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
	assert.Nil(t, u)
}

// Two first logins race on the same address: the loser re-reads the winner's row
// and links the identity to it, not to the ID it generated.
func TestResolve_JIT_CreateRace_LinksTheWinner(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	winner := &domain.User{ID: "winner-id", Email: "new@corp.com", Type: domain.UserTypeUser}

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	gomock.InOrder(
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr()),
		m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(&domain.ErrUserExists{Message: "dup"}),
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(winner, nil),
	)
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fi *domain.FederatedIdentity) error {
			assert.Equal(t, "winner-id", fi.UserID)
			assert.Equal(t, testIssuer, fi.IDPIssuer)
			assert.Equal(t, "sub-1", fi.IDPSub)
			return nil
		})

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "New", true)
	require.NoError(t, err)
	assert.Equal(t, "winner-id", u.ID)
}

func TestResolve_JIT_CreateRace_RefetchFails_NoLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	gomock.InOrder(
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr()),
		m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(&domain.ErrUserExists{Message: "dup"}),
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, errors.New("db down")),
	)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc jit re-fetch")
	assert.Nil(t, u)
}

func TestResolve_JIT_CreateFails_NoLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr())
	m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc jit create")
	assert.Nil(t, u)
}

// --- linkIdentity, mintSession, OpenFlowState -------------------------------

func TestLinkIdentity_CreateFails_NotAConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

	err := svc.linkIdentity(context.Background(), "u1", testIssuer, "sub-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc link create")
	assert.NotErrorIs(t, err, domain.ErrOIDCIdentityConflict)
}

// No session row, no token.
func TestOIDCService_MintSession_SessionCreateFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)

	m.userRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

	resp, err := svc.mintSession(context.Background(), &domain.User{ID: "u1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc create session")
	assert.Nil(t, resp)
}

func TestOIDCService_OpenFlowState_RejectsEmptyAndNonJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, _ := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	svc.secretKey = "test-secret-key-1234567890123456"

	_, err := svc.OpenFlowState("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	// Authentic ciphertext whose plaintext is not a flow state.
	sealed, err := crypto.EncryptString("not json", svc.secretKey)
	require.NoError(t, err)
	_, err = svc.OpenFlowState(sealed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}
