package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageHistoryService_CountMessages(t *testing.T) {
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	filter := domain.MessageFilter{Origin: domain.MessageOriginAutomation, CreatedAfter: &after}

	setup := func(t *testing.T) (*MessageHistoryService, *mocks.MockMessageHistoryRepository, *mocks.MockAuthService) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)
		repo := mocks.NewMockMessageHistoryRepository(ctrl)
		auth := mocks.NewMockAuthService(ctrl)
		svc := NewMessageHistoryService(repo, mocks.NewMockWorkspaceRepository(ctrl), pkgmocks.NewMockLogger(ctrl), auth, nil)
		return svc, repo, auth
	}
	as := func(auth *mocks.MockAuthService, perms domain.UserPermissions) {
		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{},
			&domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws1", Role: "member", Permissions: perms}, nil)
	}

	t.Run("the repository gets the filter received", func(t *testing.T) {
		svc, repo, auth := setup(t)
		as(auth, domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
		want := &domain.MessageCounts{Total: 5, Opened: 2}
		repo.EXPECT().CountMessages(gomock.Any(), "ws1", filter).Return(want, nil)

		got, err := svc.CountMessages(context.Background(), "ws1", filter)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("refused without message history read", func(t *testing.T) {
		svc, _, auth := setup(t)
		as(auth, domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: false}})

		_, err := svc.CountMessages(context.Background(), "ws1", filter)
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
	})

	t.Run("authentication failure returned", func(t *testing.T) {
		svc, _, auth := setup(t)
		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), nil, nil, errors.New("bad token"))

		_, err := svc.CountMessages(context.Background(), "ws1", filter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad token")
	})

	t.Run("repository error returned", func(t *testing.T) {
		svc, repo, auth := setup(t)
		as(auth, domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
		repo.EXPECT().CountMessages(gomock.Any(), "ws1", filter).Return(nil, errors.New("db down"))

		_, err := svc.CountMessages(context.Background(), "ws1", filter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}
