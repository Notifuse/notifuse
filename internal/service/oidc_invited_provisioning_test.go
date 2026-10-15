package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// A first SSO sign-in for an address with no account: a pending invitation to that
// address provisions it — but only when the provider is authoritative for the address,
// and when it is not, nothing may create the account (see resolveOrProvisionUser).

type fakeInvitationFinder struct {
	invitations []*domain.PendingInvitation
	err         error
	asked       []string
}

func (f *fakeInvitationFinder) ListPendingInvitationsForEmail(_ context.Context, email string) ([]*domain.PendingInvitation, error) {
	f.asked = append(f.asked, email)
	return f.invitations, f.err
}

func onePendingInvitation() *fakeInvitationFinder {
	return &fakeInvitationFinder{invitations: []*domain.PendingInvitation{{ID: "inv-1", WorkspaceID: "wsacme"}}}
}

func TestEmailIsAuthoritative(t *testing.T) {
	const entra = "https://login.microsoftonline.com/0b5e6f5a-tenant/v2.0"
	cases := []struct {
		name                   string
		issuer, email, hd, idp string
		verified, edov         *bool
		authoritative          bool
	}{
		{name: "verified", issuer: testIssuer, email: "a@corp.com", verified: boolPtr(true), authoritative: true},
		// OIDC_ALLOW_UNVERIFIED_EMAIL lets this one sign in; it is still not proof of the address.
		{name: "claim absent", issuer: testIssuer, email: "a@corp.com", authoritative: false},
		{name: "claim false", issuer: testIssuer, email: "a@corp.com", verified: boolPtr(false), authoritative: false},
		// xms_edov is Entra's: from any other issuer it is just a key someone put in a token.
		{name: "xms_edov outside Entra", issuer: testIssuer, email: "a@corp.com", edov: boolPtr(true), authoritative: false},
		{name: "entra tenant member", issuer: entra, email: "a@corp.com", edov: boolPtr(true), authoritative: true},
		{name: "entra member, idp is the issuer", issuer: entra, email: "a@corp.com", idp: entra, edov: boolPtr(true), authoritative: true},
		{name: "entra domain owner not verified", issuer: entra, email: "a@corp.com", edov: boolPtr(false), authoritative: false},
		{name: "entra without xms_edov", issuer: entra, email: "a@corp.com", authoritative: false},
		// xms_edov is also true for consumer accounts signing in as guests — the weak case.
		{name: "entra guest with a Microsoft account", issuer: entra, email: "a@corp.com", idp: "live.com", edov: boolPtr(true), authoritative: false},
		{name: "entra guest with a Google account", issuer: entra, email: "a@corp.com", idp: "google.com", edov: boolPtr(true), authoritative: false},
		{name: "entra v1 issuer", issuer: "https://sts.windows.net/0b5e6f5a-tenant/", email: "a@corp.com", edov: boolPtr(true), authoritative: true},
		{name: "entra ignores email_verified", issuer: entra, email: "a@corp.com", verified: boolPtr(true), authoritative: false},
		{name: "google gmail", issuer: googleIssuer, email: "a@gmail.com", verified: boolPtr(true), authoritative: true},
		{name: "google workspace", issuer: googleIssuer, email: "a@corp.com", hd: "corp.com", verified: boolPtr(true), authoritative: true},
		// A consumer Google account on someone else's domain: verified once, at creation.
		{name: "google consumer account on a third-party domain", issuer: googleIssuer, email: "a@corp.com", verified: boolPtr(true), authoritative: false},
		{name: "google gmail unverified", issuer: googleIssuer, email: "a@gmail.com", verified: boolPtr(false), authoritative: false},
		{name: "google ignores xms_edov", issuer: googleIssuer, email: "a@corp.com", verified: boolPtr(true), edov: boolPtr(true), authoritative: false},
		{name: "gmail suffix needs Google", issuer: testIssuer, email: "a@gmail.com", authoritative: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.authoritative, emailIsAuthoritative(tc.issuer, tc.email, tc.verified, tc.edov, tc.hd, tc.idp))
		})
	}
}

// The hint claims only matter on an invitee's first sign-in, so a value of an unexpected
// type must read as absent instead of failing the claims decode — and every login with it.
func TestLenientHintClaims(t *testing.T) {
	var claims struct {
		HostedDomain        lenientString `json:"hd"`
		DomainOwnerVerified lenientBool   `json:"xms_edov"`
		IdentityProvider    lenientString `json:"idp"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"hd":123,"xms_edov":1,"idp":["x"]}`), &claims))
	assert.Empty(t, string(claims.HostedDomain))
	assert.Nil(t, claims.DomainOwnerVerified.ptr())
	assert.Empty(t, string(claims.IdentityProvider))

	require.NoError(t, json.Unmarshal([]byte(`{"hd":"corp.com","xms_edov":"1","idp":"live.com"}`), &claims))
	assert.Equal(t, "corp.com", string(claims.HostedDomain))
	require.NotNil(t, claims.DomainOwnerVerified.ptr())
	assert.True(t, *claims.DomainOwnerVerified.ptr())
	assert.Equal(t, "live.com", string(claims.IdentityProvider))

	var absent struct {
		DomainOwnerVerified lenientBool `json:"xms_edov"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{}`), &absent))
	assert.Nil(t, absent.DomainOwnerVerified.ptr())
}

func TestResolve_PendingInvitee_InvitedOnly_ProvisionsAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg() // AutoCreateUsers=false: invited-only
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	finder := onePendingInvitation()
	svc.invitations = finder

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "invitee@gmail.com").Return(nil, notFoundUsr())
	m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u *domain.User) error {
			assert.Equal(t, "invitee@gmail.com", u.Email)
			assert.Equal(t, domain.UserTypeUser, u.Type)
			return nil
		})
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, fi *domain.FederatedIdentity) error {
			assert.Equal(t, "sub-1", fi.IDPSub)
			return nil
		})

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "invitee@gmail.com", boolPtr(true), "Invitee", true)
	require.NoError(t, err)
	assert.Equal(t, "invitee@gmail.com", u.Email)
	// The account only: the invitation stays pending for the user to join from the console,
	// and OIDCService has no way to write a membership at all.
	assert.Equal(t, []string{"invitee@gmail.com"}, finder.asked)
}

func TestResolve_PendingInvitee_OutsideDomainAllowlist_ProvisionsAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	svc.invitations = onePendingInvitation()

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "guest@gmail.com").Return(nil, notFoundUsr())
	m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil)
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "guest@gmail.com", boolPtr(true), "", true)
	require.NoError(t, err)
	assert.Equal(t, "guest@gmail.com", u.Email)
}

// The account would be able to claim the invitation from the console, so a provider that
// is not authoritative for the address may not create it — even where JIT alone would.
func TestResolve_PendingInvitee_NotAuthoritative_NoAccountEvenWhenJITAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	cfg.AllowUnverifiedEmail = true
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	svc.invitations = onePendingInvitation()

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr())
	// No CreateUser, no link.

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", nil, "", false)
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
}

func TestResolve_PendingInvitee_NotAuthoritative_InvitedOnly_NoAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AllowUnverifiedEmail = true
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	svc.invitations = onePendingInvitation()

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "invitee@corp.com").Return(nil, notFoundUsr())

	// email_verified absent: signed-in tolerated, not authoritative.
	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "invitee@corp.com", nil, "", false)
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
}

// Outside the allowlist an invited address with a weak token gets the answer an uninvited
// outsider gets: the reason must not tell a prober which addresses have an invitation waiting.
func TestResolve_PendingInvitee_NotAuthoritative_OutsideAllowlist_SameAnswerAsOutsider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	cfg.AllowUnverifiedEmail = true
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	svc.invitations = onePendingInvitation()

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "guest@partner.com").Return(nil, notFoundUsr())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "guest@partner.com", nil, "", false)
	assert.ErrorIs(t, err, domain.ErrOIDCDomainNotAllowed)
}

func TestResolve_NoPendingInvitation_ExistingPolicyApplies(t *testing.T) {
	t.Run("invited-only", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)
		finder := &fakeInvitationFinder{}
		svc.invitations = finder

		m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "ghost@corp.com").Return(nil, notFoundUsr())

		_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "ghost@corp.com", boolPtr(true), "", true)
		assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
		assert.Equal(t, []string{"ghost@corp.com"}, finder.asked)
	})

	t.Run("JIT outside the allowlist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cfg := enabledCfg()
		cfg.AutoCreateUsers = true
		cfg.AllowedDomains = []string{"corp.com"}
		svc, m := newTestOIDCService(t, ctrl, cfg, nil)
		svc.invitations = &fakeInvitationFinder{}

		m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "x@evil.com").Return(nil, notFoundUsr())

		_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "x@evil.com", boolPtr(true), "", true)
		assert.ErrorIs(t, err, domain.ErrOIDCDomainNotAllowed)
	})

	t.Run("JIT inside the allowlist, provider not authoritative", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cfg := enabledCfg()
		cfg.AutoCreateUsers = true
		cfg.AllowedDomains = []string{"corp.com"}
		svc, m := newTestOIDCService(t, ctrl, cfg, nil)
		svc.invitations = &fakeInvitationFinder{}

		m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
		m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr())
		m.userRepo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(nil)
		m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

		// Nothing is waiting for this address, so JIT is unchanged by the stricter rule.
		_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", false)
		require.NoError(t, err)
	})
}

func TestResolve_PendingInvitee_RootEmail_RefusedBeforeLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), func(e string) bool { return e == "root@corp.com" })
	finder := onePendingInvitation()
	svc.invitations = finder

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "root@corp.com").Return(nil, notFoundUsr())

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "root@corp.com", boolPtr(true), "", true)
	assert.ErrorIs(t, err, domain.ErrOIDCAccountNotProvisioned)
	assert.Empty(t, finder.asked, "an invitation must never stand in for a ROOT_EMAIL account")
}

func TestResolve_PendingInvitationLookupError_FailsClosed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := enabledCfg()
	cfg.AutoCreateUsers = true
	cfg.AllowedDomains = []string{"corp.com"}
	svc, m := newTestOIDCService(t, ctrl, cfg, nil)
	svc.invitations = &fakeInvitationFinder{err: errors.New("connection reset")}

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "new@corp.com").Return(nil, notFoundUsr())
	// No CreateUser: without knowing whether the address is invited, the stricter rule
	// cannot be applied, so nothing is created.

	_, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "new@corp.com", boolPtr(true), "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc pending invitation lookup")
}

// An existing account is never decided by invitations: the bridge links it as before.
func TestResolve_ExistingAccount_InvitationsNotConsulted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, m := newTestOIDCService(t, ctrl, enabledCfg(), nil)
	finder := onePendingInvitation()
	svc.invitations = finder

	m.fedRepo.EXPECT().GetByIssuerSubject(gomock.Any(), testIssuer, "sub-1").Return(nil, notFoundFI())
	m.userRepo.EXPECT().GetUserByEmailInsensitive(gomock.Any(), "jane@corp.com").Return(&domain.User{ID: "u1", Email: "jane@corp.com"}, nil)
	m.fedRepo.EXPECT().GetByUserAndIssuer(gomock.Any(), "u1", testIssuer).Return(nil, notFoundFI())
	m.fedRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	u, err := svc.resolveOrProvisionUser(context.Background(), testIssuer, "sub-1", "jane@corp.com", boolPtr(true), "", false)
	require.NoError(t, err)
	assert.Equal(t, "u1", u.ID)
	assert.Empty(t, finder.asked)
}
