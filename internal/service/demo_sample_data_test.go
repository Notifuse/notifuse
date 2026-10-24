package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Sample broadcasts arrive already "processed": status, send window and —
// for the A/B campaign — the winner fields are stamped by the seed itself,
// because no orchestrator ever ran. A regression that dropped the winner
// stamping (or marked a single-template campaign as a test) would show a
// Variations table with no winner highlighted.

func demoBroadcastSvc(t *testing.T) (*DemoService, *broadcastSvcDeps, *mocks.MockBroadcastRepository) {
	t.Helper()
	d := setupBroadcastSvc(t)
	ctrl := gomock.NewController(t)
	broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	svc := &DemoService{
		logger:           logger.NewLoggerWithLevel("disabled"),
		broadcastService: d.svc,
		broadcastRepo:    broadcastRepo,
	}
	return svc, d, broadcastRepo
}

func expectBroadcastCreation(d *broadcastSvcDeps) {
	userWorkspace := &domain.UserWorkspace{
		UserID:      "user1",
		WorkspaceID: "ws-1",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true, Write: true},
		},
	}
	d.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
		Return(context.Background(), &domain.User{ID: "user1"}, userWorkspace, nil).AnyTimes()
	d.workspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).
		Return(&domain.Workspace{ID: "ws-1", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}, nil).AnyTimes()
	d.templateSvc.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&domain.Template{ID: "tpl"}, nil).AnyTimes()
}

func TestDemoService_CreateSampleBroadcasts(t *testing.T) {
	ctx := context.Background()

	t.Run("all campaigns are created processed, the A/B winner is stamped", func(t *testing.T) {
		svc, d, broadcastRepo := demoBroadcastSvc(t)
		defer d.ctrl.Finish()
		expectBroadcastCreation(d)

		var created []*domain.Broadcast
		d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
				created = append(created, b)
				return nil
			}).Times(4)
		var updated []*domain.Broadcast
		broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
				cp := *b
				updated = append(updated, &cp)
				return nil
			}).Times(4)

		ids, err := svc.createSampleBroadcasts(ctx, "ws-1")
		require.NoError(t, err)
		require.Len(t, ids, 4)
		require.Len(t, updated, 4)

		for i, b := range updated {
			assert.Equal(t, domain.BroadcastStatusProcessed, b.Status, "campaign %d", i)
			require.NotNil(t, b.StartedAt)
			require.NotNil(t, b.CompletedAt)
			assert.Equal(t, b.StartedAt.Add(2*time.Hour), *b.CompletedAt,
				"completion is stamped two hours after the send started")
			assert.True(t, b.StartedAt.Before(time.Now()), "sends are backdated")
		}
		// Campaigns are backdated 10, 8, 6, 4 days in creation order.
		assert.True(t, updated[0].StartedAt.Before(*updated[3].StartedAt))

		ab := updated[3]
		require.NotNil(t, ab.WinningTemplate)
		assert.Equal(t, "newsletter-weekly-v2", *ab.WinningTemplate,
			"the last variation is the recorded winner")
		require.NotNil(t, ab.TestSentAt)
		require.NotNil(t, ab.WinnerSentAt)
		assert.Equal(t, ab.TestSentAt.Add(time.Hour), *ab.WinnerSentAt)
		for _, single := range updated[:3] {
			assert.Nil(t, single.WinningTemplate, "single-template campaigns have no winner")
		}
		assert.Equal(t, created[3].ID, ids[3])
	})

	t.Run("one failing campaign is skipped, the rest still seed", func(t *testing.T) {
		svc, d, broadcastRepo := demoBroadcastSvc(t)
		defer d.ctrl.Finish()
		expectBroadcastCreation(d)

		gomock.InOrder(
			d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil),
			d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).
				Return(errors.New("template deleted")),
			d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil),
			d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil),
		)
		broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(3)

		ids, err := svc.createSampleBroadcasts(ctx, "ws-1")
		require.NoError(t, err)
		assert.Len(t, ids, 3)
	})

	t.Run("every campaign failing is an error", func(t *testing.T) {
		svc, d, _ := demoBroadcastSvc(t)
		defer d.ctrl.Finish()
		expectBroadcastCreation(d)
		d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).
			Return(errors.New("nope")).Times(4)

		_, err := svc.createSampleBroadcasts(ctx, "ws-1")
		require.EqualError(t, err, "failed to create any sample broadcasts")
	})
}

func TestDemoService_CreateSampleTransactionalNotifications(t *testing.T) {
	ctx := context.Background()
	newSvc := func(t *testing.T) (*DemoService, *mocks.MockTransactionalNotificationRepository, *mocks.MockAuthService) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		authService := mocks.NewMockAuthService(ctrl)
		templateSvc := mocks.NewMockTemplateService(ctrl)
		templateSvc.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&domain.Template{ID: "password-reset"}, nil).AnyTimes()
		txSvc := NewTransactionalNotificationService(
			repo,
			mocks.NewMockMessageHistoryRepository(ctrl),
			templateSvc,
			mocks.NewMockContactService(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl),
			authService,
			logger.NewLoggerWithLevel("disabled"),
			mocks.NewMockWorkspaceRepository(ctrl),
			"",
		)
		svc := &DemoService{
			logger:                           logger.NewLoggerWithLevel("disabled"),
			transactionalNotificationService: txSvc,
		}
		return svc, repo, authService
	}
	owner := &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws-1", Role: "owner"}

	t.Run("the password-reset notification is created with its template", func(t *testing.T) {
		svc, repo, authService := newSvc(t)
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "u1"}, owner, nil).Times(1)
		repo.EXPECT().Create(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, n *domain.TransactionalNotification) error {
				assert.Equal(t, "password_reset", n.ID)
				assert.Equal(t, "password-reset",
					n.Channels[domain.TransactionalChannelEmail].TemplateID)
				return nil
			}).Times(1)

		require.NoError(t, svc.createSampleTransactionalNotifications(ctx, "ws-1"))
	})

	t.Run("a creation failure degrades to nil, never aborting the seed", func(t *testing.T) {
		svc, repo, authService := newSvc(t)
		authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
			Return(ctx, &domain.User{ID: "u1"}, owner, nil).Times(1)
		repo.EXPECT().Create(gomock.Any(), "ws-1", gomock.Any()).
			Return(errors.New("already exists")).Times(1)

		assert.NoError(t, svc.createSampleTransactionalNotifications(ctx, "ws-1"))
	})
}
