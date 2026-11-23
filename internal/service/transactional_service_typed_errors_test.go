package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// The repository's typed errors reach the handler through the service's wraps: a missing
// notification as ErrNotFound, a taken ID as AlreadyExistsError. The handler maps them by type,
// where it matched "not found" in the message before.
func TestTransactionalNotificationService_TypedErrorsReachTheCaller(t *testing.T) {
	const workspaceID = "w1"
	notFound := &domain.ErrNotFound{Entity: "transactional_notification", ID: "welcome"}

	setup := func(t *testing.T) (*TransactionalNotificationService, *mocks.MockTransactionalNotificationRepository) {
		ctrl := gomock.NewController(t)
		logger := pkgmocks.NewMockLogger(ctrl)
		logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
		logger.EXPECT().Debug(gomock.Any()).AnyTimes()
		logger.EXPECT().Info(gomock.Any()).AnyTimes()
		logger.EXPECT().Error(gomock.Any()).AnyTimes()

		authService := mocks.NewMockAuthService(ctrl)
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{
				UserID: "u1", WorkspaceID: workspaceID, Role: "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil).AnyTimes()

		repo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		svc := NewTransactionalNotificationService(repo, mocks.NewMockMessageHistoryRepository(ctrl),
			mocks.NewMockTemplateService(ctrl), mocks.NewMockContactService(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl), authService, logger,
			mocks.NewMockWorkspaceRepository(ctrl), mocks.NewMockListRepository(ctrl), "https://api.example.com")
		return svc, repo
	}

	t.Run("get of an unknown notification", func(t *testing.T) {
		svc, repo := setup(t)
		repo.EXPECT().Get(gomock.Any(), workspaceID, "welcome").Return(nil, notFound)

		_, err := svc.GetNotification(context.Background(), workspaceID, "welcome")
		requireNotFound(t, err, "transactional_notification", "welcome")
	})

	t.Run("update of an unknown notification", func(t *testing.T) {
		svc, repo := setup(t)
		repo.EXPECT().Get(gomock.Any(), workspaceID, "welcome").Return(nil, notFound)

		_, err := svc.UpdateNotification(context.Background(), workspaceID, "welcome",
			domain.TransactionalNotificationUpdateParams{Name: "Welcome"})
		requireNotFound(t, err, "transactional_notification", "welcome")
	})

	t.Run("delete of an unknown notification", func(t *testing.T) {
		svc, repo := setup(t)
		repo.EXPECT().Get(gomock.Any(), workspaceID, "welcome").Return(nil, notFound)

		err := svc.DeleteNotification(context.Background(), workspaceID, "welcome")
		requireNotFound(t, err, "transactional_notification", "welcome")
	})

	t.Run("create of an ID a deleted notification holds", func(t *testing.T) {
		svc, repo := setup(t)
		repo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any()).
			Return(domain.NewAlreadyExistsError("transactional_notification", "welcome", true))

		_, err := svc.CreateNotification(context.Background(), workspaceID,
			domain.TransactionalNotificationCreateParams{ID: "welcome", Name: "Welcome"})
		requireAlreadyExists(t, err, "transactional_notification", "welcome", true)
	})
}
