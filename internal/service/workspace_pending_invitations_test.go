package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// The signed-in side of an invitation: listing what the caller's email was invited to,
// and joining it without the emailed link.

type pendingInvitationMocks struct {
	repo *mocks.MockWorkspaceRepository
	auth *mocks.MockAuthService
}

func newPendingInvitationService(t *testing.T, maxUsers int) (*WorkspaceService, *pendingInvitationMocks) {
	t.Helper()
	ctrl := gomock.NewController(t)
	m := &pendingInvitationMocks{
		repo: mocks.NewMockWorkspaceRepository(ctrl),
		auth: mocks.NewMockAuthService(ctrl),
	}
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	svc := NewWorkspaceService(
		m.repo, mocks.NewMockUserRepository(ctrl), mocks.NewMockTaskRepository(ctrl),
		mockLogger, mocks.NewMockUserServiceInterface(ctrl), m.auth, pkgmocks.NewMockMailer(ctrl),
		&config.Config{MaxUsers: maxUsers, Environment: "development"},
		mocks.NewMockContactService(ctrl), mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl), mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key", &SupabaseService{}, &DNSVerificationService{}, &BlogService{},
		fullyLicensedProvider(ctrl),
	)
	return svc, m
}

// As AuthenticateUserFromContext returns it: the users row it loads carries no type, so
// whether the caller is a person or an API key comes from the token, in the context.
var invitee = &domain.User{ID: "user-1", Email: "invitee@example.com"}

func sessionCtx() context.Context {
	return context.WithValue(context.Background(), domain.UserTypeKey, string(domain.UserTypeUser))
}

func apiKeyCtx() context.Context {
	return context.WithValue(context.Background(), domain.UserTypeKey, string(domain.UserTypeAPIKey))
}

const invitationUUID = "6f1d2c3b-4a5e-4f70-8a9b-0c1d2e3f4a5b"

func liveInvitation() *domain.WorkspaceInvitation {
	return &domain.WorkspaceInvitation{
		ID:          invitationUUID,
		WorkspaceID: "wsacme",
		Email:       "Invitee@Example.com", // as the owner typed it
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true},
		},
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
}

// listed is what ListPendingInvitationsForEmail answers when the invitation is addressed to
// the caller and live: entitlement to join is being in that list.
func listed(ids ...string) []*domain.PendingInvitation {
	out := []*domain.PendingInvitation{}
	for _, id := range ids {
		out = append(out, &domain.PendingInvitation{ID: id, WorkspaceID: "wsacme"})
	}
	return out
}

func TestWorkspaceService_ListPendingInvitations(t *testing.T) {
	t.Run("lists the invitations for workspaces not joined yet", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return([]*domain.PendingInvitation{
			{ID: "inv-1", WorkspaceID: "wsacme", WorkspaceName: "Acme"},
			{ID: "inv-2", WorkspaceID: "wsbeta", WorkspaceName: "Beta"},
		}, nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(false, nil)
		// Already joined another way since: the invitation has nothing to offer.
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsbeta").Return(true, nil)

		invitations, err := svc.ListPendingInvitations(sessionCtx())
		require.NoError(t, err)
		require.Len(t, invitations, 1)
		assert.Equal(t, "wsacme", invitations[0].WorkspaceID)
	})

	t.Run("an API key has none", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).
			Return(&domain.User{ID: "key-1", Email: "invitee@example.com"}, nil)

		invitations, err := svc.ListPendingInvitations(apiKeyCtx())
		require.NoError(t, err)
		assert.NotNil(t, invitations)
		assert.Empty(t, invitations)
	})

	t.Run("authentication failure", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(nil, errors.New("session expired"))

		_, err := svc.ListPendingInvitations(sessionCtx())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("repository errors propagate", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(nil, errors.New("connection reset"))

		_, err := svc.ListPendingInvitations(sessionCtx())
		require.Error(t, err)
	})

	t.Run("membership check errors propagate", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).
			Return([]*domain.PendingInvitation{{ID: "inv-1", WorkspaceID: "wsacme"}}, nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(false, errors.New("connection reset"))

		_, err := svc.ListPendingInvitations(sessionCtx())
		require.Error(t, err)
	})
}

func TestWorkspaceService_JoinInvitation(t *testing.T) {
	t.Run("seats the user with the invitation's grant and clears it", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		invitation := liveInvitation()
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(invitation, nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(listed(invitationUUID), nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(false, nil)
		// Its own invitation is left out of the seat count: joining converts that seat.
		m.repo.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), "wsacme", invitation.Email).Return(2, nil)
		gomock.InOrder(
			m.repo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, uw *domain.UserWorkspace) error {
					assert.Equal(t, invitee.ID, uw.UserID)
					assert.Equal(t, "wsacme", uw.WorkspaceID)
					assert.Equal(t, "member", uw.Role)
					assert.Equal(t, invitation.Permissions, uw.Permissions)
					return nil
				}),
			m.repo.EXPECT().DeleteInvitationByEmail(gomock.Any(), "wsacme", invitation.Email).Return(nil),
		)

		record := domain.NewAuditRecord("", "", "")
		workspaceID, err := svc.JoinInvitation(domain.WithAuditRecord(sessionCtx(), record), invitationUUID)
		require.NoError(t, err)
		assert.Equal(t, "wsacme", workspaceID)

		// The route is not workspace-scoped, so nothing upstream names the workspace.
		event, _ := record.Finalize(200)
		assert.Equal(t, "wsacme", event.WorkspaceID)
		assert.Equal(t, domain.AuditTargetInvitation, event.TargetType)
		assert.Equal(t, invitationUUID, event.TargetID)
	})

	// Not a UUID can name no invitation, and asking the database would be a type error (a
	// 500), not a miss.
	t.Run("a malformed id is unavailable without a lookup", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)

		_, err := svc.JoinInvitation(sessionCtx(), "abc")
		assert.ErrorIs(t, err, domain.ErrInvitationUnavailable)
	})

	t.Run("not found is unavailable", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(nil, domain.ErrInvitationNotFound)

		_, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		assert.ErrorIs(t, err, domain.ErrInvitationUnavailable)
	})

	// Someone else's, expired, or a spelling the database does not fold to the caller's
	// address: all are "not in the caller's list". The refused attempt is still recorded
	// against the invitation's workspace, so its owner can see a member probing an id.
	t.Run("an invitation not in the caller's list is unavailable, and audited to its workspace", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(liveInvitation(), nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(listed("another-invitation"), nil)

		record := domain.NewAuditRecord("", "", "")
		_, err := svc.JoinInvitation(domain.WithAuditRecord(sessionCtx(), record), invitationUUID)
		assert.ErrorIs(t, err, domain.ErrInvitationUnavailable)

		event, _ := record.Finalize(404)
		assert.Equal(t, "wsacme", event.WorkspaceID)
		assert.Equal(t, invitationUUID, event.TargetID)
	})

	for name, ctx := range map[string]func() context.Context{
		"an API key with the same email": apiKeyCtx,
		// A caller that never went through the auth middleware has proven nothing.
		"a context without a token type": context.Background,
	} {
		t.Run(name+" is unavailable", func(t *testing.T) {
			svc, m := newPendingInvitationService(t, 3)
			m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
			m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(liveInvitation(), nil)
			// Refused before the list, the membership check or any write.

			_, err := svc.JoinInvitation(ctx(), invitationUUID)
			assert.ErrorIs(t, err, domain.ErrInvitationUnavailable)
		})
	}

	t.Run("a database failure is not reported as unavailable", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(nil, errors.New("connection reset"))

		_, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrInvitationUnavailable)
	})

	t.Run("a failed entitlement lookup is an error, not a refusal", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(liveInvitation(), nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(nil, errors.New("connection reset"))

		_, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrInvitationUnavailable)
	})

	// Writing the membership is an upsert: doing it for an existing member would reset an
	// owner to the invitation's member grant. The audit row says nothing changed.
	t.Run("already a member: the membership is left alone, the invitation cleared", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		invitation := liveInvitation()
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(invitation, nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(listed(invitationUUID), nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(true, nil)
		m.repo.EXPECT().DeleteInvitationByEmail(gomock.Any(), "wsacme", invitation.Email).Return(nil)

		record := domain.NewAuditRecord("", "", "")
		workspaceID, err := svc.JoinInvitation(domain.WithAuditRecord(sessionCtx(), record), invitationUUID)
		require.NoError(t, err)
		assert.Equal(t, "wsacme", workspaceID)

		event, _ := record.Finalize(200)
		assert.Equal(t, true, event.Metadata["already_member"])
	})

	t.Run("refused at the seat limit", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 3)
		invitation := liveInvitation()
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(invitation, nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(listed(invitationUUID), nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(false, nil)
		m.repo.EXPECT().CountWorkspaceMembersAndInvitations(gomock.Any(), "wsacme", invitation.Email).Return(3, nil)

		_, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		var limitErr *domain.ErrTeamMemberLimitReached
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, 3, limitErr.Limit)
	})

	t.Run("a failed membership write keeps the invitation", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(liveInvitation(), nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(listed(invitationUUID), nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(false, nil)
		m.repo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(errors.New("connection reset"))
		// No DeleteInvitationByEmail: the user can try again.

		_, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add user to workspace")
	})

	t.Run("a failed invitation cleanup does not fail the join", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(invitee, nil)
		m.repo.EXPECT().GetInvitationByID(gomock.Any(), invitationUUID).Return(liveInvitation(), nil)
		m.repo.EXPECT().ListPendingInvitationsForEmail(gomock.Any(), invitee.Email).Return(listed(invitationUUID), nil)
		m.repo.EXPECT().IsUserWorkspaceMember(gomock.Any(), invitee.ID, "wsacme").Return(false, nil)
		m.repo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil)
		m.repo.EXPECT().DeleteInvitationByEmail(gomock.Any(), "wsacme", "Invitee@Example.com").Return(errors.New("connection reset"))

		workspaceID, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		require.NoError(t, err)
		assert.Equal(t, "wsacme", workspaceID)
	})

	t.Run("authentication failure", func(t *testing.T) {
		svc, m := newPendingInvitationService(t, 0)
		m.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(nil, errors.New("session expired"))

		_, err := svc.JoinInvitation(sessionCtx(), invitationUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})
}
