package broadcast_test

import (
	"context"
	"errors"
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

// TestBroadcastOrchestrator_Process_PausedBroadcast tests that the orchestrator
// correctly detects when a broadcast is paused during processing and stops execution.
func TestBroadcastOrchestrator_Process_PausedBroadcast(t *testing.T) {
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
		nil, // emailQueueRepo not needed for this test
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

	// Mock task with broadcast state already in progress
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: workspaceID,
		BroadcastID: &broadcastID,
		State: &domain.TaskState{
			Progress: 0.25, // 25% complete
			Message:  "Processing broadcast",
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     broadcastID,
				TotalRecipients: 100,
				EnqueuedCount:   25,
				FailedCount:     0,
				RecipientOffset: 25,
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

	// Mock paused broadcast
	pausedBroadcast := &domain.Broadcast{
		ID:     broadcastID,
		Status: domain.BroadcastStatusPaused,
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

	// GetBroadcast will be called during processing loop - return paused status
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(pausedBroadcast, nil).
		AnyTimes()

	mockWorkspaceRepo.EXPECT().
		GetByID(ctx, workspaceID).
		Return(workspace, nil).
		AnyTimes()

	// Mock template repository - the orchestrator will try to load templates before detecting pause
	bodyBlock := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("body-1", notifuse_mjml.MJMLComponentMjBody),
	}

	mjmlBlock := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml),
	}
	mjmlBlock.Children = []notifuse_mjml.EmailBlock{bodyBlock}

	mockTemplate := &domain.Template{
		ID:   "template-1",
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject:          "Test Subject",
			VisualEditorTree: mjmlBlock,
		},
	}
	mockTemplateRepo.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, "template-1", int64(0)).
		Return(mockTemplate, nil).
		AnyTimes()

	// Execute
	timeoutAt := time.Now().Add(5 * time.Minute)
	allDone, err := orchestrator.Process(ctx, task, timeoutAt)

	// The task is parked, not completed: a resume wakes it.
	assertParkedForTheResume(t, err)
	assert.False(t, allDone, "Task should not be marked as complete when broadcast is paused - it should be resumable")
}

// TestBroadcastOrchestrator_Process_PausedDuringProcessing tests that a broadcast
// that transitions from sending to paused while processing is detected correctly.
func TestBroadcastOrchestrator_Process_PausedDuringProcessing(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepository := domainmocks.NewMockBroadcastRepository(ctrl)
	// The first sending run records the start date (orchestrator_started_at_test.go).
	mockBroadcastRepository.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// The run that begins sending announces itself; this test is not about that event.
	mockEventBus.EXPECT().Publish(gomock.Any(), eventOfType(domain.EventBroadcastSendingStarted)).AnyTimes()

	// Setup logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	config := &broadcast.Config{
		FetchBatchSize: 10, // Small batch size to force multiple iterations
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepository,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil, // emailQueueRepo not needed for this test
		nil, // abTestEvaluator not needed for tests,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"

	// Mock task starting fresh
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: workspaceID,
		BroadcastID: &broadcastID,
		State: &domain.TaskState{
			Progress: 0,
			Message:  "Starting broadcast",
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     broadcastID,
				TotalRecipients: 50,
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
			SecretKey:                    "test-secret-key",
		},
	}
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

	// Initially sending broadcast
	sendingBroadcast := &domain.Broadcast{
		ID:     broadcastID,
		Status: domain.BroadcastStatusProcessing,
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

	// Later becomes paused
	pausedBroadcast := &domain.Broadcast{
		ID:     broadcastID,
		Status: domain.BroadcastStatusPaused,
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

	// First call returns sending, subsequent calls return paused
	callCount := 0
	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), workspaceID, broadcastID).
		DoAndReturn(func(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error) {
			callCount++
			if callCount <= 1 {
				return sendingBroadcast, nil
			}
			return pausedBroadcast, nil
		}).
		AnyTimes()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		Return(workspace, nil).
		AnyTimes()

	// Mock template
	bodyBlock := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("body-1", notifuse_mjml.MJMLComponentMjBody),
	}

	mjmlBlock := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml),
	}
	mjmlBlock.Children = []notifuse_mjml.EmailBlock{bodyBlock}

	mockTemplate := &domain.Template{
		ID:   "template-1",
		Name: "Test Template",
		Email: &domain.EmailTemplate{
			Subject:          "Test Subject",
			VisualEditorTree: mjmlBlock,
		},
	}
	mockTemplateRepo.EXPECT().
		GetTemplateByID(gomock.Any(), workspaceID, "template-1", int64(0)).
		Return(mockTemplate, nil).
		AnyTimes()

	// Mock contacts for first batch - should be fetched before pause is detected
	mockContacts := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "test1@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "test2@example.com"}, ListID: "list-1"},
	}
	mockContactRepo.EXPECT().
		GetContactsForBroadcast(gomock.Any(), workspaceID, sendingBroadcast.Audience, 10, "").
		Return(mockContacts, nil).
		MaxTimes(1)

	// Mock message sender - may or may not be called before pause is detected
	mockMessageSender.EXPECT().
		SendBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(len(mockContacts), 0, nil).
		MaxTimes(1)

	// Execute
	timeoutAt := time.Now().Add(5 * time.Minute)
	allDone, err := orchestrator.Process(ctx, task, timeoutAt)

	// The pause is detected and the task parked for the resume.
	assertParkedForTheResume(t, err)
	assert.False(t, allDone, "Task should not be complete when broadcast is paused during processing")
}

// TestBroadcastOrchestrator_Process_PausedVsCancelled verifies the difference
// between paused (allDone=false) and cancelled (allDone=true) broadcasts.
func TestBroadcastOrchestrator_Process_PausedVsCancelled(t *testing.T) {
	tests := []struct {
		name            string
		broadcastStatus domain.BroadcastStatus
		expectedAllDone bool
		expectedError   bool
		testDescription string
	}{
		{
			name:            "paused_broadcast_returns_false",
			broadcastStatus: domain.BroadcastStatusPaused,
			expectedAllDone: false,
			expectedError:   false,
			testDescription: "Paused broadcasts should return allDone=false so they can be resumed",
		},
		{
			name:            "cancelled_broadcast_returns_true",
			broadcastStatus: domain.BroadcastStatusCancelled,
			expectedAllDone: true,
			expectedError:   false,
			testDescription: "Cancelled broadcasts should return allDone=true as they won't be resumed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
				nil, // emailQueueRepo not needed for this test
				nil,
				mockLogger,
				nil,
				mockTimeProvider,
				"https://api.example.com",
				mockEventBus,
			)

			ctx := context.Background()
			workspaceID := "workspace-123"
			broadcastID := "broadcast-123"

			task := &domain.Task{
				ID:          "task-123",
				WorkspaceID: workspaceID,
				BroadcastID: &broadcastID,
				State: &domain.TaskState{
					Progress: 0.5,
					Message:  "Processing broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     broadcastID,
						TotalRecipients: 100,
						EnqueuedCount:   50,
						FailedCount:     0,
						RecipientOffset: 50,
						Phase:           "single",
						ChannelType:     "email",
					},
				},
			}

			workspace := &domain.Workspace{
				ID: workspaceID,
				Settings: domain.WorkspaceSettings{
					EmailTrackingEnabled:         true,
					MarketingEmailProviderID:     "ses-integration-1",
					TransactionalEmailProviderID: "ses-integration-1",
				},
			}
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
						{Name: "Test Sender", Email: "test@example.com"},
					},
				},
			}
			workspace.AddIntegration(sesIntegration)

			broadcast := &domain.Broadcast{
				ID:     broadcastID,
				Status: tc.broadcastStatus,
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

			mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()
			mockBroadcastRepository.EXPECT().
				GetBroadcast(ctx, workspaceID, broadcastID).
				Return(broadcast, nil).
				AnyTimes()
			mockBroadcastRepository.EXPECT().
				UpdateBroadcast(ctx, gomock.Any()).
				Return(nil).
				AnyTimes()
			mockWorkspaceRepo.EXPECT().
				GetByID(ctx, workspaceID).
				Return(workspace, nil).
				AnyTimes()

			bodyBlock := &notifuse_mjml.MJBodyBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("body-1", notifuse_mjml.MJMLComponentMjBody),
			}

			mjmlBlock := &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml),
			}
			mjmlBlock.Children = []notifuse_mjml.EmailBlock{bodyBlock}

			mockTemplate := &domain.Template{
				ID:   "template-1",
				Name: "Test Template",
				Email: &domain.EmailTemplate{
					Subject:          "Test Subject",
					VisualEditorTree: mjmlBlock,
				},
			}
			mockTemplateRepo.EXPECT().
				GetTemplateByID(gomock.Any(), workspaceID, "template-1", int64(0)).
				Return(mockTemplate, nil).
				AnyTimes()

			// Execute
			timeoutAt := time.Now().Add(5 * time.Minute)
			allDone, err := orchestrator.Process(ctx, task, timeoutAt)

			// Verify
			switch {
			case tc.expectedError:
				require.Error(t, err)
			case tc.broadcastStatus == domain.BroadcastStatusPaused:
				assertParkedForTheResume(t, err)
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expectedAllDone, allDone, tc.testDescription)
		})
	}
}

// assertParkedForTheResume checks that a run which found its broadcast paused parks its task
// (paused, a day ahead) instead of leaving it due at once, where every scheduler tick ran it
// again for nothing.
func assertParkedForTheResume(t *testing.T, err error) {
	t.Helper()
	var requeue *domain.TaskRequeueError
	require.True(t, errors.As(err, &requeue), "expected the task to be parked, got %v", err)
	assert.True(t, requeue.Park)
	assert.Equal(t, 24*time.Hour, requeue.After)
	assert.NotNil(t, requeue.AfterRequeue, "a resume committed while the run held the task must still wake it")
}

// Once the task is parked, the run reads the broadcast again: a resume committed while the
// run still held the task could not wake it, so the run wakes it.
func TestPausedRun_WakesItsTaskWhenTheBroadcastWasResumedMeanwhile(t *testing.T) {
	for _, tc := range []struct {
		status domain.BroadcastStatus
		wake   bool
	}{
		{domain.BroadcastStatusPaused, false},
		{domain.BroadcastStatusProcessing, true},
		{domain.BroadcastStatusCancelled, true},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			broadcasts := domainmocks.NewMockBroadcastRepository(ctrl)
			taskRepo := domainmocks.NewMockTaskRepository(ctrl)
			logger := pkgmocks.NewMockLogger(ctrl)
			logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
			logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
			logger.EXPECT().Info(gomock.Any()).AnyTimes()
			logger.EXPECT().Warn(gomock.Any()).AnyTimes()
			o := broadcast.NewBroadcastOrchestrator(mocks.NewMockMessageSender(ctrl), broadcasts, domainmocks.NewMockTemplateRepository(ctrl),
				domainmocks.NewMockContactRepository(ctrl), taskRepo, domainmocks.NewMockWorkspaceRepository(ctrl), nil, nil, logger,
				nil, mocks.NewMockTimeProvider(ctrl), "https://api.example.com", domainmocks.NewMockEventBus(ctrl))

			broadcasts.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
				Return(&domain.Broadcast{ID: "broadcast-123", Status: tc.status}, nil)
			if tc.wake {
				taskRepo.EXPECT().WakeTask(gomock.Any(), "workspace-123", "task-123", gomock.Any()).Return(true, nil)
			}

			err := broadcast.ParkPausedTaskForTest(o, &domain.Task{ID: "task-123", WorkspaceID: "workspace-123"}, "broadcast-123")
			var requeue *domain.TaskRequeueError
			require.True(t, errors.As(err, &requeue))
			requeue.AfterRequeue(context.Background())
		})
	}
}
