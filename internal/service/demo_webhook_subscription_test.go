package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// The demo webhook subscription must end up disabled: created enabled (Create
// has no say in it) and then switched off. If the switch-off fails, the
// subscription is deleted rather than left running against a placeholder
// URL — an enabled demo subscription would generate deliveries for every
// seeded event.

func demoWebhookSvc(t *testing.T) (*DemoService, *mocks.MockWebhookSubscriptionRepository, *mocks.MockAuthService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "root-1"},
			&domain.UserWorkspace{UserID: "root-1", WorkspaceID: "demo", Role: "owner"}, nil).AnyTimes()
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	deliveryRepo.EXPECT().DeleteBySubscriptionID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()
	webhookSvc := NewWebhookSubscriptionService(repo, deliveryRepo, authService,
		logger.NewLoggerWithLevel("disabled"), false)
	svc := &DemoService{
		logger:                     logger.NewLoggerWithLevel("disabled"),
		webhookSubscriptionService: webhookSvc,
	}
	return svc, repo, authService
}

func TestDemoService_CreateDemoWebhookSubscription(t *testing.T) {
	ctx := context.Background()

	t.Run("create failure is returned as-is", func(t *testing.T) {
		svc, repo, _ := demoWebhookSvc(t)
		sentinel := errors.New("insert failed")
		repo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).Return(sentinel).Times(1)

		err := svc.createDemoWebhookSubscription(ctx, "demo")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("disable failure deletes the subscription and reports the disable error", func(t *testing.T) {
		svc, repo, _ := demoWebhookSvc(t)
		var created *domain.WebhookSubscription
		repo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				created = sub
				return nil
			}).Times(1)
		repo.EXPECT().GetByID(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string) (*domain.WebhookSubscription, error) {
				return created, nil
			}).AnyTimes()
		disableErr := errors.New("update failed")
		repo.EXPECT().Update(gomock.Any(), "demo", gomock.Any()).Return(disableErr).Times(1)
		repo.EXPECT().Delete(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, id string) error {
				assert.Equal(t, created.ID, id, "the just-created subscription is the one removed")
				return nil
			}).Times(1)

		err := svc.createDemoWebhookSubscription(ctx, "demo")
		require.ErrorContains(t, err, "failed to disable the demo webhook subscription:")
		assert.ErrorIs(t, err, disableErr)
	})

	t.Run("created enabled, then switched off with all event types", func(t *testing.T) {
		svc, repo, _ := demoWebhookSvc(t)
		var created *domain.WebhookSubscription
		repo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				assert.True(t, sub.Enabled, "Create cannot disable: the switch-off is a second step")
				assert.Equal(t, domain.WebhookEventTypes, sub.Settings.EventTypes)
				created = sub
				return nil
			}).Times(1)
		repo.EXPECT().GetByID(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string) (*domain.WebhookSubscription, error) {
				return created, nil
			}).AnyTimes()
		repo.EXPECT().Update(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				assert.False(t, sub.Enabled)
				created = sub
				return nil
			}).Times(1)

		require.NoError(t, svc.createDemoWebhookSubscription(ctx, "demo"))
		require.NotNil(t, created)
		assert.False(t, created.Enabled)
	})
}
