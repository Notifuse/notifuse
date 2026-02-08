package broadcast_test

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/internal/service/broadcast/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastOrchestrator_Process_CancelledBroadcast(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepository := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepository,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil, // abTestEvaluator not needed for tests,
		mockLogger,
		nil, // Use default config
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"

	// Mock task with broadcast state
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: workspaceID,
		BroadcastID: &broadcastID,
		State: &domain.TaskState{
			Progress: 0,
			Message:  "Starting broadcast",
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     broadcastID,
				TotalRecipients: 100,
				EnqueuedCount:   0,
				FailedCount:     0,
				RecipientOffset: 0,
				Phase:           "single",
				ChannelType:     "email",
			},
		},
	}

	// Mock workspace with email provider
	workspace := &domain.Workspace{
		ID: workspaceID,
		Settings: domain.WorkspaceSettings{
			EmailTrackingEnabled:         true,
			MarketingEmailProviderID:     "ses-integration-1",
			TransactionalEmailProviderID: "ses-integration-1",
		},
	}
	// Add an SES integration
	sesIntegration := domain.Integration{
		ID:   "ses-integration-1",
		Name: "SES Provider",
		Type: domain.IntegrationTypeEmail,
		EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSES,
			SES: &domain.AmazonSESSettings{
				Region: "us-east-1",
			},
			Senders: []domain.EmailSender{
				{
					Name:  "Test Sender",
					Email: "test@example.com",
				},
			},
		},
	}
	workspace.AddIntegration(sesIntegration)

	// Mock cancelled broadcast
	cancelledBroadcast := &domain.Broadcast{
		ID:     broadcastID,
		Status: domain.BroadcastStatusCancelled,
		Audience: domain.AudienceSettings{
			List: "list-1",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{TemplateID: "template-1"},
			},
		},
	}

	// Setup expectations
	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	// GetBroadcast will be called multiple times - first for the main process, then for batch fetch
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(cancelledBroadcast, nil).
		AnyTimes()

	// Mock UpdateBroadcast - the orchestrator may try to update broadcast status
	mockBroadcastRepository.EXPECT().
		UpdateBroadcast(ctx, gomock.Any()).
		Return(nil).
		AnyTimes()

	mockWorkspaceRepo.EXPECT().
		GetByID(ctx, workspaceID).
		Return(workspace, nil).
		AnyTimes()

	// Mock template repository - the orchestrator will try to load templates before detecting cancellation
	mockTemplate := &domain.Template{
		ID:   "template-1",
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject: "Test Subject",
			VisualEditorTree: func() notifuse_mjml.EmailBlock {
				bodyBase := notifuse_mjml.NewBaseBlock("body-1", notifuse_mjml.MJMLComponentMjBody)
				bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
				rootBase := notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml)
				rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
				return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
			}(),
		},
	}
	mockTemplateRepo.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, "template-1", int64(0)).
		Return(mockTemplate, nil).
		AnyTimes()

	// When FetchBatch is called (indirectly through Process), it should detect cancellation
	// This happens in the orchestrator.FetchBatch method which calls GetBroadcast

	// Execute
	timeoutAt := time.Now().Add(5 * time.Minute)
	allDone, err := orchestrator.Process(ctx, task, timeoutAt)

	// Verify - should complete successfully without error because cancellation is handled gracefully
	require.NoError(t, err)
	assert.True(t, allDone, "Task should be marked as complete when broadcast is cancelled")
}

// Note: The fix for broadcast cancellation race condition has been implemented.
// When a broadcast is cancelled during processing, the orchestrator now tracks
// this state and avoids attempting to update the broadcast status at the end,
// preventing the "Broadcast not found with ID" error.
