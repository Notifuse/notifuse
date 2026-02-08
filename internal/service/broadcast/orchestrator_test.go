package broadcast_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// Helper function to create a minimal valid MJML block with proper children
func createMinimalValidMJMLBlock(id string) *notifuse_mjml.MJMLBlock {
	bodyBase := notifuse_mjml.NewBaseBlock(id+"-body", notifuse_mjml.MJMLComponentMjBody)
	bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	rootBase := notifuse_mjml.NewBaseBlock(id, notifuse_mjml.MJMLComponentMjml)
	rootBase.Attributes["version"] = "4.0.0"
	rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

func TestBroadcastOrchestrator_CanProcess(t *testing.T) {
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
		nil, // abTestEvaluator not needed for tests
		mockLogger,
		nil, // Use default config
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Test cases
	tests := []struct {
		taskType string
		expected bool
	}{
		{"send_broadcast", true},
		{"other_task", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.taskType, func(t *testing.T) {
			result := orchestrator.CanProcess(tc.taskType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBroadcastOrchestrator_LoadTemplates(t *testing.T) {
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
		nil, // abTestEvaluator not needed for tests
		mockLogger,
		nil, // Use default config
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	ctx := context.Background()
	workspaceID := "workspace-123"
	templateIDs := []string{"template-1", "template-2"}

	// Mock template responses
	template1 := &domain.Template{
		ID: "template-1",
		Email: &domain.EmailTemplate{
			Subject:          "Test Subject 1",
			SenderID:         "sender-123",
			VisualEditorTree: createMinimalValidMJMLBlock("root1"),
		},
	}
	template2 := &domain.Template{
		ID: "template-2",
		Email: &domain.EmailTemplate{
			Subject:          "Test Subject 2",
			SenderID:         "sender-123",
			VisualEditorTree: createMinimalValidMJMLBlock("root2"),
		},
	}

	// Setup expectations
	mockTemplateRepo.EXPECT().
		GetTemplateByID(ctx, workspaceID, "template-1", int64(0)).
		Return(template1, nil)

	mockTemplateRepo.EXPECT().
		GetTemplateByID(ctx, workspaceID, "template-2", int64(0)).
		Return(template2, nil)

	// Execute
	templates, err := orchestrator.LoadTemplates(ctx, workspaceID, templateIDs)

	// Verify
	require.NoError(t, err)
	assert.Len(t, templates, 2)
	assert.Equal(t, template1, templates["template-1"])
	assert.Equal(t, template2, templates["template-2"])
}

func TestBroadcastOrchestrator_ValidateTemplates(t *testing.T) {
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

	// Setup logger expectations - ensure all possible calls are mocked
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
		nil, // abTestEvaluator not needed for tests
		mockLogger,
		nil, // Use default config
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Test cases
	tests := []struct {
		name        string
		templates   map[string]*domain.Template
		expectError bool
	}{
		{
			name: "Valid templates",
			templates: map[string]*domain.Template{
				"template-1": {
					ID: "template-1",
					Email: &domain.EmailTemplate{
						Subject:          "Test Subject",
						SenderID:         "sender-123",
						VisualEditorTree: createMinimalValidMJMLBlock("root1"),
					},
				},
			},
			expectError: false,
		},
		{
			name:        "Empty templates",
			templates:   map[string]*domain.Template{},
			expectError: true,
		},
		{
			name: "Missing email config",
			templates: map[string]*domain.Template{
				"template-1": {
					ID: "template-1",
				},
			},
			expectError: true,
		},
		{
			name: "Missing subject",
			templates: map[string]*domain.Template{
				"template-1": {
					ID: "template-1",
					Email: &domain.EmailTemplate{
						SenderID:         "sender-123",
						VisualEditorTree: createMinimalValidMJMLBlock("root1"),
					},
				},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := orchestrator.ValidateTemplates(tc.templates)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBroadcastOrchestrator_GetTotalRecipientCount(t *testing.T) {
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

	// Mock broadcast
	testBroadcast := &domain.Broadcast{
		Audience: domain.AudienceSettings{
			List: "list-1",
		},
	}

	// Setup expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(testBroadcast, nil)

	mockContactRepo.EXPECT().
		CountContactsForBroadcast(ctx, workspaceID, testBroadcast.Audience).
		Return(100, nil)

	// Execute
	count, err := orchestrator.GetTotalRecipientCount(ctx, workspaceID, broadcastID)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 100, count)
}

func TestBroadcastOrchestrator_FetchBatch(t *testing.T) {
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
	afterEmail := "" // Empty cursor for first batch
	limit := 50

	// Mock broadcast
	testBroadcast := &domain.Broadcast{
		Status: domain.BroadcastStatusProcessing,
		Audience: domain.AudienceSettings{
			List: "list-1",
		},
	}

	// Mock contacts
	expectedContacts := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "user1@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user2@example.com"}, ListID: "list-1"},
	}

	// Setup expectations
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(testBroadcast, nil)

	mockContactRepo.EXPECT().
		GetContactsForBroadcast(ctx, workspaceID, testBroadcast.Audience, limit, afterEmail).
		Return(expectedContacts, nil)

	// Execute
	contacts, err := orchestrator.FetchBatch(ctx, workspaceID, broadcastID, afterEmail, limit)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, expectedContacts, contacts)
}

func TestBroadcastOrchestrator_FetchBatch_CancelledBroadcast(t *testing.T) {
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
	afterEmail := "" // Empty cursor for first batch
	limit := 50

	// Mock cancelled broadcast
	cancelledBroadcast := &domain.Broadcast{
		Status: domain.BroadcastStatusCancelled,
		Audience: domain.AudienceSettings{
			List: "list-1",
		},
	}

	// Setup expectations - should return cancelled broadcast
	mockBroadcastRepository.EXPECT().
		GetBroadcast(ctx, workspaceID, broadcastID).
		Return(cancelledBroadcast, nil)

	// Should NOT call GetContactsForBroadcast since broadcast is cancelled

	// Execute
	contacts, err := orchestrator.FetchBatch(ctx, workspaceID, broadcastID, afterEmail, limit)

	// Verify
	require.Error(t, err)
	assert.Nil(t, contacts)

	// Check that it's the specific broadcast cancelled error
	broadcastErr, ok := err.(*broadcast.BroadcastError)
	require.True(t, ok, "Expected BroadcastError")
	assert.Equal(t, broadcast.ErrCodeBroadcastCancelled, broadcastErr.Code)
	assert.Equal(t, "broadcast has been cancelled", broadcastErr.Message)
	assert.False(t, broadcastErr.Retryable)
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes_and_seconds", 2*time.Minute + 30*time.Second, "2m 30s"},
		{"hours_and_minutes", 1*time.Hour + 30*time.Minute, "1h 30m"},
		{"zero_duration", 0, "0s"},
		{"large_duration", 25*time.Hour + 90*time.Minute, "26h 30m"},
		{"exact_seconds", 60 * time.Second, "1m 0s"},
		{"exact_minute", 60 * time.Minute, "1h 0m"},
		{"exact_hour", 1 * time.Hour, "1h 0m"},
		{"milliseconds_rounded", 1500 * time.Millisecond, "1s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := broadcast.FormatDuration(tc.duration)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCalculateProgress(t *testing.T) {
	tests := []struct {
		name      string
		processed int
		total     int
		expected  float64
	}{
		{"zero_total", 0, 0, 100.0},
		{"zero_processed", 0, 100, 0.0},
		{"half_processed", 50, 100, 50.0},
		{"fully_processed", 100, 100, 100.0},
		{"more_than_total_processed", 150, 100, 100.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := broadcast.CalculateProgress(tc.processed, tc.total)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFormatProgressMessage(t *testing.T) {
	tests := []struct {
		name      string
		processed int
		total     int
		elapsed   time.Duration
		expected  string
	}{
		{"initial_progress_no_ETA", 0, 100, 5 * time.Second, "Processed 0/100 recipients (0.0%)"},
		{"progress_with_ETA", 25, 100, 1 * time.Minute, "Processed 25/100 recipients (25.0%), ETA: 3m 0s"},
		{"completed", 100, 100, 2 * time.Minute, "Processed 100/100 recipients (100.0%)"},
		{"zero_total", 0, 0, 30 * time.Second, "Processed 0/0 recipients (100.0%)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := broadcast.FormatProgressMessage(tc.processed, tc.total, tc.elapsed)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSaveProgressState(t *testing.T) {
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
	taskID := "task-123"
	broadcastID := "broadcast-123"
	totalRecipients := 100
	sentCount := 25
	failedCount := 5
	processedCount := 30
	lastSaveTime := time.Now().Add(-10 * time.Second)
	startTime := time.Now().Add(-1 * time.Minute)

	// Mock time provider
	currentTime := time.Now()
	mockTimeProvider.EXPECT().Now().Return(currentTime).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(5 * time.Second).AnyTimes()

	// Setup expectations
	mockTaskRepo.EXPECT().
		SaveState(ctx, workspaceID, taskID, gomock.Any(), gomock.Any()).
		Return(nil)

	// Create broadcast state
	broadcastState := &domain.SendBroadcastState{
		BroadcastID:               broadcastID,
		TotalRecipients:           totalRecipients,
		EnqueuedCount:             sentCount,
		FailedCount:               failedCount,
		ChannelType:               "email",
		RecipientOffset:           int64(processedCount),
		Phase:                     "test",
		TestPhaseCompleted:        false,
		TestPhaseRecipientCount:   50,
		WinnerPhaseRecipientCount: 50,
	}

	// Execute
	newSaveTime, err := orchestrator.SaveProgressState(
		ctx, workspaceID, taskID, broadcastState,
		sentCount, failedCount, processedCount,
		lastSaveTime, startTime,
	)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, currentTime, newSaveTime)
}

// TestBroadcastOrchestrator_Process tests the main Process method covering lines 594-795
func TestBroadcastOrchestrator_Process(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider)
		task          *domain.Task
		expectedDone  bool
		expectedError bool
		errorContains string
	}{
		{
			name: "successful_process_with_recipients",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()
				mockTimeProvider.EXPECT().Since(gomock.Any()).Return(10 * time.Second).AnyTimes()

				// Mock workspace with email provider (this is called first in lines 594-795)
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey:                "secret-key",
						EmailTrackingEnabled:     true,
						MarketingEmailProviderID: "marketing-provider-id",
					},
					Integrations: []domain.Integration{
						{
							ID:   "marketing-provider-id",
							Name: "Marketing Provider",
							Type: domain.IntegrationTypeEmail,
							EmailProvider: domain.EmailProvider{
								Kind: domain.EmailProviderKindSES,
								SES: &domain.AmazonSESSettings{
									AccessKey: "access-key",
									SecretKey: "secret-key",
									Region:    "us-east-1",
								},
							},
						},
					},
				}
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

				// Mock broadcast for template loading
				broadcast := &domain.Broadcast{
					ID: "broadcast-123",
					TestSettings: domain.BroadcastTestSettings{
						Variations: []domain.BroadcastVariation{
							{TemplateID: "template-1"},
						},
					},
					Audience: domain.AudienceSettings{
						List: "list-1",
					},
					Status: domain.BroadcastStatusProcessing,
				}
				// Initial calls for template loading and later status updates; allow additional refresh calls
				mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(broadcast, nil).AnyTimes()

				// Mock broadcast status update on completion
				mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, b *domain.Broadcast) error {
					// Verify the broadcast status was updated to processed
					assert.Equal(t, domain.BroadcastStatusProcessed, b.Status)
					assert.NotNil(t, b.CompletedAt)
					return nil
				})

				// Mock template
				template := &domain.Template{
					ID: "template-1",
					Email: &domain.EmailTemplate{
						Subject:  "Test Subject",
						SenderID: "sender-123",
						VisualEditorTree: &notifuse_mjml.MJMLBlock{
							BaseBlock: notifuse_mjml.NewBaseBlock("root1", notifuse_mjml.MJMLComponentMjml),
						},
					},
				}
				mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(template, nil)

				// Mock recipients - return fewer than batch size to indicate completion
				recipients := []*domain.ContactWithList{
					{Contact: &domain.Contact{Email: "user1@example.com"}, ListID: "list-1"},
					{Contact: &domain.Contact{Email: "user2@example.com"}, ListID: "list-1"},
				}
				// Expect batch size of 2 because remainingInPhase (2) < FetchBatchSize (50)
				mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", broadcast.Audience, 2, "").Return(recipients, nil)

				// Mock message sending
				mockMessageSender.EXPECT().SendBatch(
					gomock.Any(),
					"workspace-123",
					"marketing-provider-id",
					"secret-key",
					gomock.Any(),
					true,
					"broadcast-123",
					recipients,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(2, 0, nil)

				// Mock task state saving
				mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-123"),
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "broadcast-123",
						TotalRecipients: 2, // Set to non-zero to skip recipient counting phase
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  true,
			expectedError: false,
		},
		{
			name: "broadcast_status_update_failure",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()
				mockTimeProvider.EXPECT().Since(gomock.Any()).Return(10 * time.Second).AnyTimes()

				// Mock workspace with email provider
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey:                "secret-key",
						EmailTrackingEnabled:     true,
						MarketingEmailProviderID: "marketing-provider-id",
					},
					Integrations: []domain.Integration{
						{
							ID:   "marketing-provider-id",
							Name: "Marketing Provider",
							Type: domain.IntegrationTypeEmail,
							EmailProvider: domain.EmailProvider{
								Kind: domain.EmailProviderKindSES,
								SES: &domain.AmazonSESSettings{
									AccessKey: "access-key",
									SecretKey: "secret-key",
									Region:    "us-east-1",
								},
							},
						},
					},
				}
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

				// Mock broadcast for template loading
				broadcast := &domain.Broadcast{
					ID: "broadcast-123",
					TestSettings: domain.BroadcastTestSettings{
						Variations: []domain.BroadcastVariation{
							{TemplateID: "template-1"},
						},
					},
					Audience: domain.AudienceSettings{
						List: "list-1",
					},
					Status: domain.BroadcastStatusProcessing,
				}
				// Initial calls for template loading and later status updates; allow additional refresh calls
				mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(broadcast, nil).AnyTimes()

				// Mock broadcast status update failure on completion
				mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(fmt.Errorf("database error"))

				// Mock template
				template := &domain.Template{
					ID: "template-1",
					Email: &domain.EmailTemplate{
						Subject:  "Test Subject",
						SenderID: "sender-123",
						VisualEditorTree: &notifuse_mjml.MJMLBlock{
							BaseBlock: notifuse_mjml.NewBaseBlock("root1", notifuse_mjml.MJMLComponentMjml),
						},
					},
				}
				mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(template, nil)

				// Mock recipients - return fewer than batch size to indicate completion
				recipients := []*domain.ContactWithList{
					{Contact: &domain.Contact{Email: "user1@example.com"}, ListID: "list-1"},
				}
				// Expect batch size of 1 because remainingInPhase (1) < FetchBatchSize (50)
				mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", broadcast.Audience, 1, "").Return(recipients, nil)

				// Mock message sending
				mockMessageSender.EXPECT().SendBatch(
					gomock.Any(),
					"workspace-123",
					"marketing-provider-id", "secret-key",
					gomock.Any(),
					true,
					"broadcast-123",
					recipients,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(1, 0, nil)

				// Mock task state saving
				mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-123"),
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "broadcast-123",
						TotalRecipients: 1, // Set to non-zero to skip recipient counting phase
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  false,
			expectedError: true,
			errorContains: "failed to update broadcast status to processed",
		},
		{
			name: "workspace_not_found",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()

				// Workspace not found
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(nil, fmt.Errorf("workspace not found"))

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-123"),
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "broadcast-123",
						TotalRecipients: 2,
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  false,
			expectedError: true,
			errorContains: "failed to get workspace",
		},
		{
			name: "no_email_provider_configured",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()

				// Mock workspace without email provider
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey:            "secret-key",
						EmailTrackingEnabled: true,
						// No MarketingEmailProviderID configured
					},
				}
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-123"),
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "broadcast-123",
						TotalRecipients: 2,
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  false,
			expectedError: true,
			errorContains: "no email provider configured for marketing emails",
		},
		{
			name: "template_loading_failure",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()

				// Mock workspace with email provider
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey:                "secret-key",
						EmailTrackingEnabled:     true,
						MarketingEmailProviderID: "marketing-provider-id",
					},
					Integrations: []domain.Integration{
						{
							ID:   "marketing-provider-id",
							Name: "Marketing Provider",
							Type: domain.IntegrationTypeEmail,
							EmailProvider: domain.EmailProvider{
								Kind: domain.EmailProviderKindSES,
								SES: &domain.AmazonSESSettings{
									AccessKey: "access-key",
									SecretKey: "secret-key",
									Region:    "us-east-1",
								},
							},
						},
					},
				}
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

				// Mock broadcast for template loading
				broadcast := &domain.Broadcast{
					ID: "broadcast-123",
					TestSettings: domain.BroadcastTestSettings{
						Variations: []domain.BroadcastVariation{
							{TemplateID: "template-1"},
						},
					},
					Audience: domain.AudienceSettings{
						List: "list-1",
					},
				}
				mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(broadcast, nil)

				// Template loading failure
				mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(nil, fmt.Errorf("template not found"))

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-123"),
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "broadcast-123",
						TotalRecipients: 2,
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  false,
			expectedError: true,
			errorContains: "no valid templates found for broadcast",
		},
		{
			name: "recipient_fetch_failure",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()

				// Mock workspace with email provider
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey:                "secret-key",
						EmailTrackingEnabled:     true,
						MarketingEmailProviderID: "marketing-provider-id",
					},
					Integrations: []domain.Integration{
						{
							ID:   "marketing-provider-id",
							Name: "Marketing Provider",
							Type: domain.IntegrationTypeEmail,
							EmailProvider: domain.EmailProvider{
								Kind: domain.EmailProviderKindSES,
								SES: &domain.AmazonSESSettings{
									AccessKey: "access-key",
									SecretKey: "secret-key",
									Region:    "us-east-1",
								},
							},
						},
					},
				}
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

				// Mock broadcast for template loading
				broadcast := &domain.Broadcast{
					ID: "broadcast-123",
					TestSettings: domain.BroadcastTestSettings{
						Variations: []domain.BroadcastVariation{
							{TemplateID: "template-1"},
						},
					},
					Audience: domain.AudienceSettings{
						List: "list-1",
					},
				}
				mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(broadcast, nil).AnyTimes()

				// Mock template
				template := &domain.Template{
					ID: "template-1",
					Email: &domain.EmailTemplate{
						Subject:  "Test Subject",
						SenderID: "sender-123",
						VisualEditorTree: &notifuse_mjml.MJMLBlock{
							BaseBlock: notifuse_mjml.NewBaseBlock("root1", notifuse_mjml.MJMLComponentMjml),
						},
					},
				}
				mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(template, nil)

				// Recipient fetch failure - expect batch size of 2 because remainingInPhase (2) < FetchBatchSize (50)
				mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", broadcast.Audience, 2, "").Return(nil, fmt.Errorf("database error"))

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-123"),
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "broadcast-123",
						TotalRecipients: 2,
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  false,
			expectedError: true,
			errorContains: "failed to fetch recipients",
		},
		{
			name: "broadcast_id_from_task_field",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockMessageSender, *domainmocks.MockBroadcastRepository, *domainmocks.MockTemplateRepository, *domainmocks.MockContactRepository, *domainmocks.MockTaskRepository, *domainmocks.MockWorkspaceRepository, *pkgmocks.MockLogger, *mocks.MockTimeProvider) {
				mockMessageSender := mocks.NewMockMessageSender(ctrl)
				mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
				mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
				mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
				mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
				mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
				mockLogger := pkgmocks.NewMockLogger(ctrl)
				mockTimeProvider := mocks.NewMockTimeProvider(ctrl)

				// Setup logger expectations
				mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
				mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
				mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

				// Setup time provider
				baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
				mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()
				mockTimeProvider.EXPECT().Since(gomock.Any()).Return(10 * time.Second).AnyTimes()

				// Mock workspace with email provider
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey:                "secret-key",
						EmailTrackingEnabled:     true,
						MarketingEmailProviderID: "marketing-provider-id",
					},
					Integrations: []domain.Integration{
						{
							ID:   "marketing-provider-id",
							Name: "Marketing Provider",
							Type: domain.IntegrationTypeEmail,
							EmailProvider: domain.EmailProvider{
								Kind: domain.EmailProviderKindSES,
								SES: &domain.AmazonSESSettings{
									AccessKey: "access-key",
									SecretKey: "secret-key",
									Region:    "us-east-1",
								},
							},
						},
					},
				}
				mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

				// Mock broadcast for template loading
				broadcast := &domain.Broadcast{
					ID: "broadcast-456",
					TestSettings: domain.BroadcastTestSettings{
						Variations: []domain.BroadcastVariation{
							{TemplateID: "template-1"},
						},
					},
					Audience: domain.AudienceSettings{
						List: "list-1",
					},
					Status: domain.BroadcastStatusProcessing,
				}
				// Initial calls for template loading and later status updates; allow additional refresh calls
				mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-456").Return(broadcast, nil).AnyTimes()

				// Mock broadcast status update on completion
				mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, b *domain.Broadcast) error {
					// Verify the broadcast status was updated to sent
					assert.Equal(t, domain.BroadcastStatusProcessed, b.Status)
					assert.NotNil(t, b.CompletedAt)
					return nil
				})

				// Mock template
				template := &domain.Template{
					ID: "template-1",
					Email: &domain.EmailTemplate{
						Subject:  "Test Subject",
						SenderID: "sender-123",
						VisualEditorTree: &notifuse_mjml.MJMLBlock{
							BaseBlock: notifuse_mjml.NewBaseBlock("root1", notifuse_mjml.MJMLComponentMjml),
						},
					},
				}
				mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(template, nil)

				// Mock recipients - return fewer than batch size to indicate completion
				recipients := []*domain.ContactWithList{
					{Contact: &domain.Contact{Email: "user1@example.com"}, ListID: "list-1"},
				}
				// Expect batch size of 1 because remainingInPhase (1) < FetchBatchSize (50)
				mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", broadcast.Audience, 1, "").Return(recipients, nil)

				// Mock message sending
				mockMessageSender.EXPECT().SendBatch(
					gomock.Any(),
					"workspace-123",
					"marketing-provider-id", "secret-key",
					gomock.Any(),
					true,
					"broadcast-456",
					recipients,
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(1, 0, nil)

				// Mock task state saving
				mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider
			},
			task: &domain.Task{
				ID:          "task-123",
				WorkspaceID: "workspace-123",
				Type:        "send_broadcast",
				BroadcastID: stringPtr("broadcast-456"), // Broadcast ID in task field
				State: &domain.TaskState{
					Progress: 0,
					Message:  "Starting broadcast",
					SendBroadcast: &domain.SendBroadcastState{
						BroadcastID:     "", // Empty broadcast ID in state
						TotalRecipients: 1,
						EnqueuedCount:   0,
						FailedCount:     0,
						RecipientOffset: 0,
					},
				},
				RetryCount: 0,
				MaxRetries: 3,
			},
			expectedDone:  true,
			expectedError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider := tc.setupMocks(ctrl)
			mockEventBus := domainmocks.NewMockEventBus(ctrl)

			config := &broadcast.Config{
				FetchBatchSize:      100,
				MaxProcessTime:      30 * time.Second,
				ProgressLogInterval: 5 * time.Second,
			}

			orchestrator := broadcast.NewBroadcastOrchestrator(
				mockMessageSender,
				mockBroadcastRepo,
				mockTemplateRepo,
				mockContactRepo,
				mockTaskRepo,
				mockWorkspaceRepo,
				nil, // abTestEvaluator not needed for tests,
				mockLogger,
				config,
				mockTimeProvider,
				"https://api.example.com",
				mockEventBus,
			)

			// Execute
			ctx := context.Background()
			timeoutAt := time.Now().Add(30 * time.Second)
			done, err := orchestrator.Process(ctx, tc.task, timeoutAt)

			// Verify
			assert.Equal(t, tc.expectedDone, done)
			if tc.expectedError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBroadcastOrchestrator_Process_ABTestStartSetsTestingAndCompletesTestPhase(t *testing.T) {
	// Covers phase initialization when A/B testing is enabled (lines 594-622) and recipient limit selection (713-722)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Time provider
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(5 * time.Second).AnyTimes()

	// Workspace with email provider
	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{ID: "marketing-provider-id", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

	// Broadcast with A/B testing enabled, status sending so phase becomes test, and must update status to testing
	bcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: "workspace-123",
		Audience:    domain.AudienceSettings{List: "list-1"},
		Status:      domain.BroadcastStatusProcessing,
		TestSettings: domain.BroadcastTestSettings{
			Enabled:    true,
			Variations: []domain.BroadcastVariation{{TemplateID: "template-1"}},
			// sample percentage set to ensure testRecipientCount calculation
			SamplePercentage: 100,
		},
	}
	// GetBroadcast can be called multiple times
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(bcast, nil).AnyTimes()

	// Expect status update to testing, then to test_completed
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
		if b.Status == domain.BroadcastStatusTesting || b.Status == domain.BroadcastStatusTestCompleted {
			return nil
		}
		return fmt.Errorf("unexpected status: %s", b.Status)
	}).Times(2)

	// Template
	tpl := &domain.Template{ID: "template-1", Email: &domain.EmailTemplate{Subject: "S", SenderID: "s", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(tpl, nil)

	// Contacts - since sample 100% and totalRecipients preset below = 1, expect limit 1
	recipients := []*domain.ContactWithList{{Contact: &domain.Contact{Email: "a@b.com"}, ListID: "list-1"}}
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", bcast.Audience, 1, "").Return(recipients, nil)

	// Send batch
	mockMessageSender.EXPECT().SendBatch(gomock.Any(), "workspace-123", "marketing-provider-id", "secret-key", gomock.Any(), true, "broadcast-123", recipients, gomock.Any(), gomock.Any(), gomock.Any()).Return(1, 0, nil)

	// Save state
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State:       &domain.TaskState{Progress: 0, Message: "Starting", SendBroadcast: &domain.SendBroadcastState{BroadcastID: "broadcast-123", TotalRecipients: 1, RecipientOffset: 0}},
		RetryCount:  0,
		MaxRetries:  3,
	}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	// Test phase completes and waits for winner selection
	assert.False(t, done)
}

func TestBroadcastOrchestrator_Process_WinnerPhaseMissingTemplate_Error(t *testing.T) {
	// Covers winner phase with no winning template selected (lines 650-662)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "k",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{ID: "marketing-provider-id", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

	bcast := &domain.Broadcast{ID: "broadcast-123", WorkspaceID: "workspace-123", Audience: domain.AudienceSettings{}, Status: domain.BroadcastStatusWinnerSelected, TestSettings: domain.BroadcastTestSettings{Enabled: true}}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(bcast, nil)

	// Expect the UpdateBroadcast call when the error occurs and status is set to failed
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{ID: "task-123", WorkspaceID: "workspace-123", Type: "send_broadcast", BroadcastID: stringPtr("broadcast-123"), State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{BroadcastID: "broadcast-123", TotalRecipients: 1, Phase: "winner"}}}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(10*time.Second))
	assert.False(t, done)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "winner phase but no winning template selected")
}

func TestBroadcastOrchestrator_Process_ValidateTemplatesFailure(t *testing.T) {
	// Covers template validation failure path (lines 687-697)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "k",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{ID: "marketing-provider-id", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "a", SecretKey: "b", Region: "us-east-1"}}},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

	bcast := &domain.Broadcast{ID: "broadcast-123", WorkspaceID: "workspace-123", Audience: domain.AudienceSettings{}, Status: domain.BroadcastStatusProcessing, TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{TemplateID: "tpl1"}}}}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(bcast, nil).AnyTimes()

	// Expect the UpdateBroadcast call when the error occurs and status is set to failed
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Load template that will fail validation (missing subject)
	badTpl := &domain.Template{ID: "tpl1", Email: &domain.EmailTemplate{SenderID: "s", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "tpl1", int64(0)).Return(badTpl, nil)

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{ID: "task-123", WorkspaceID: "workspace-123", Type: "send_broadcast", BroadcastID: stringPtr("broadcast-123"), State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{BroadcastID: "broadcast-123", TotalRecipients: 2}}}

	_, err := orchestrator.Process(ctx, task, time.Now().Add(10*time.Second))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template missing subject")
}

func TestBroadcastOrchestrator_Process_BatchSizeZeroTriggersPhaseCompletion(t *testing.T) {
	// Covers lines 787-811 by configuring FetchBatchSize=0 so batchSize<=0 path is taken
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	workspace := &domain.Workspace{ID: "w", Settings: domain.WorkspaceSettings{SecretKey: "k", MarketingEmailProviderID: "pid"}, Integrations: []domain.Integration{{ID: "pid", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "a", SecretKey: "b", Region: "us-east-1"}}}}}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "w").Return(workspace, nil)

	bcast := &domain.Broadcast{ID: "b", WorkspaceID: "w", Audience: domain.AudienceSettings{}, Status: domain.BroadcastStatusProcessing, TestSettings: domain.BroadcastTestSettings{Enabled: true, Variations: []domain.BroadcastVariation{{TemplateID: "tpl"}}, SamplePercentage: 100}}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "w", "b").Return(bcast, nil).AnyTimes()

	// First UpdateBroadcast to testing during phase init, second to test_completed in handleTestPhaseCompletion
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	tpl := &domain.Template{ID: "tpl", Email: &domain.EmailTemplate{Subject: "s", SenderID: "x", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "w", "tpl", int64(0)).Return(tpl, nil)

	// No SaveState expectations needed; allow any
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "w", "t", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 0, MaxProcessTime: 30 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{ID: "t", WorkspaceID: "w", Type: "send_broadcast", BroadcastID: stringPtr("b"), State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{BroadcastID: "b", TotalRecipients: 1}}}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(10*time.Second))
	// Not done because test phase completes and waits for winner selection
	assert.False(t, done)
	require.NoError(t, err)
}

func TestBroadcastOrchestrator_Process_EmptyRecipientsTriggersTestCompletion(t *testing.T) {
	// Covers lines 828-859 where recipient fetch returns empty list
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	workspace := &domain.Workspace{ID: "w", Settings: domain.WorkspaceSettings{SecretKey: "k", MarketingEmailProviderID: "pid"}, Integrations: []domain.Integration{{ID: "pid", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "a", SecretKey: "b", Region: "us-east-1"}}}}}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "w").Return(workspace, nil)

	bcast := &domain.Broadcast{ID: "b", WorkspaceID: "w", Audience: domain.AudienceSettings{List: "l"}, Status: domain.BroadcastStatusProcessing, TestSettings: domain.BroadcastTestSettings{Enabled: true, Variations: []domain.BroadcastVariation{{TemplateID: "tpl"}}, SamplePercentage: 100}}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "w", "b").Return(bcast, nil).AnyTimes()

	// Update to testing then to test_completed
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	tpl := &domain.Template{ID: "tpl", Email: &domain.EmailTemplate{Subject: "s", SenderID: "x", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "w", "tpl", int64(0)).Return(tpl, nil)

	// Return empty recipients
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "w", bcast.Audience, 1, "").Return([]*domain.ContactWithList{}, nil)

	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "w", "t", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{ID: "t", WorkspaceID: "w", Type: "send_broadcast", BroadcastID: stringPtr("b"), State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{BroadcastID: "b", TotalRecipients: 1}}}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(10*time.Second))
	assert.False(t, done)
	require.NoError(t, err)
}

func TestBroadcastOrchestrator_Process_AutoWinnerEvaluationPath(t *testing.T) {
	// Covers lines 574-591: auto winner evaluation when test completed and time passed
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)
	msgRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Fixed time to satisfy shouldEvaluateWinner
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(base).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(5 * time.Second).AnyTimes()

	workspace := &domain.Workspace{ID: "w", Settings: domain.WorkspaceSettings{SecretKey: "k", EmailTrackingEnabled: true, MarketingEmailProviderID: "pid"}, Integrations: []domain.Integration{{ID: "pid", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "a", SecretKey: "b", Region: "us-east-1"}}}}}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "w").Return(workspace, nil)

	bcast := &domain.Broadcast{
		ID:          "b",
		WorkspaceID: "w",
		Audience:    domain.AudienceSettings{List: "l"},
		Status:      domain.BroadcastStatusTestCompleted,
		TestSentAt:  &time.Time{},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:              true,
			AutoSendWinner:       true,
			AutoSendWinnerMetric: domain.TestWinnerMetricOpenRate,
			TestDurationHours:    1,
			Variations:           []domain.BroadcastVariation{{TemplateID: "tplA"}, {TemplateID: "tplB"}},
		},
	}
	// Set TestSentAt to 2 hours ago to satisfy shouldEvaluateWinner
	testSentAt := base.Add(-2 * time.Hour)
	bcast.TestSentAt = &testSentAt

	// Build evaluator and allow transaction update to winner selected
	abEval := broadcast.NewABTestEvaluator(msgRepo, mockBroadcastRepo, mockLogger)

	// Stats prefer tplB
	msgRepo.EXPECT().GetBroadcastVariationStats(gomock.Any(), "w", "b", "tplA").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 10}, nil)
	msgRepo.EXPECT().GetBroadcastVariationStats(gomock.Any(), "w", "b", "tplB").Return(&domain.MessageHistoryStatusSum{TotalDelivered: 100, TotalOpened: 60}, nil)

	// Transaction and update during evaluation
	mockBroadcastRepo.EXPECT().WithTransaction(gomock.Any(), "w", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
	mockBroadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// After evaluation, phase becomes winner; ensure we can proceed with winner phase template
	// Provide winning template id in refreshed broadcast
	bcastAfter := *bcast
	bcastAfter.Status = domain.BroadcastStatusWinnerSelected
	tplBID := "tplB"
	bcastAfter.WinningTemplate = &tplBID

	// Orchestrator loads broadcast initially, then refreshes after evaluation
	callCount := 0
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "w", "b").DoAndReturn(func(_ context.Context, _, _ string) (*domain.Broadcast, error) {
		callCount++
		if callCount <= 2 {
			return bcast, nil // Initial calls
		}
		return &bcastAfter, nil // After evaluation
	}).AnyTimes()

	// Template load for tplB
	tplB := &domain.Template{ID: "tplB", Email: &domain.EmailTemplate{Subject: "s", SenderID: "x", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "w", "tplB", int64(0)).Return(tplB, nil)

	// Recipient batch for winner phase (totalRecipients preset to 1 in task below)
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "w", bcast.Audience, 1, "").Return([]*domain.ContactWithList{{Contact: &domain.Contact{Email: "w@x.com"}}}, nil)

	// Send
	mockMessageSender.EXPECT().SendBatch(gomock.Any(), "w", "pid", "k", gomock.Any(), true, "b", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(1, 0, nil)

	// Save state
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "w", "t", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Final broadcast update to Sent
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil)

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, abEval, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{ID: "t", WorkspaceID: "w", Type: "send_broadcast", BroadcastID: stringPtr("b"), State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{BroadcastID: "b", TotalRecipients: 1, Phase: "test"}}}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.True(t, done)
}

// TestBroadcastOrchestrator_Process_ABTestWinnerPhaseProcessesRemainingRecipients
// This test reproduces and verifies the fix for the bug where the winner phase
// would not process remaining recipients after test phase completion.
// Scenario: 2 recipients, 50% test phase (1 recipient), winner selection, then winner phase should process remaining 1 recipient.
func TestBroadcastOrchestrator_Process_ABTestWinnerPhaseProcessesRemainingRecipients(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Mock time provider
	base := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(base).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(time.Minute).AnyTimes()

	// Setup workspace with email provider
	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{
				ID:   "marketing-provider-id",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSES,
					SES: &domain.AmazonSESSettings{
						AccessKey: "access-key",
						SecretKey: "secret-key",
						Region:    "us-east-1",
					},
				},
			},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil).AnyTimes()

	// Setup broadcast with A/B testing enabled
	winningTemplateB := "template-B"
	bcast := &domain.Broadcast{
		ID:              "broadcast-123",
		WorkspaceID:     "workspace-123",
		Audience:        domain.AudienceSettings{List: "list-1"},
		Status:          domain.BroadcastStatusWinnerSelected, // Winner already selected
		WinningTemplate: &winningTemplateB,                    // Winner is template B
		TestSettings: domain.BroadcastTestSettings{
			Enabled:          true,
			SamplePercentage: 50, // 50% test phase
			Variations: []domain.BroadcastVariation{
				{TemplateID: "template-A"},
				{TemplateID: "template-B"},
			},
		},
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(bcast, nil).AnyTimes()

	// Setup templates - we need to mock both in case the phase transition doesn't work initially
	templateA := &domain.Template{
		ID: "template-A",
		Email: &domain.EmailTemplate{
			Subject:  "Test Subject A",
			SenderID: "sender-1",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
		},
	}
	templateB := &domain.Template{
		ID: "template-B",
		Email: &domain.EmailTemplate{
			Subject:  "Test Subject B",
			SenderID: "sender-1",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
		},
	}

	// Mock template loading - might load all variations first, then just winner
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).Return(templateA, nil).AnyTimes()
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-B", int64(0)).Return(templateB, nil).AnyTimes()

	// Setup recipients: winner phase should fetch using cursor (after test phase processed 1 recipient)
	// This is the key part of the test - ensuring the winner phase processes the remaining recipient
	// With cursor-based pagination, we use LastProcessedEmail instead of offset
	recipient := &domain.ContactWithList{
		Contact: &domain.Contact{
			Email: "recipient2@example.com",
		},
		ListID: "list-1",
	}
	mockContactRepo.EXPECT().GetContactsForBroadcast(
		gomock.Any(),
		"workspace-123",
		bcast.Audience,
		1,                        // limit: remaining recipients in phase
		"recipient1@example.com", // cursor: last email processed in test phase
	).Return([]*domain.ContactWithList{recipient}, nil)

	// Mock successful sending
	mockMessageSender.EXPECT().SendBatch(
		gomock.Any(),
		"workspace-123",
		"marketing-provider-id", "secret-key",
		gomock.Any(), // custom endpoint
		true,
		"broadcast-123",
		[]*domain.ContactWithList{recipient},
		gomock.Any(), // templates
		gomock.Any(), // email provider
		gomock.Any(), // timeout
	).Return(1, 0, nil) // 1 sent, 0 failed

	// Mock task state saving
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Mock final broadcast status update to "sent"
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
		assert.Equal(t, domain.BroadcastStatusProcessed, b.Status)
		return nil
	})

	// Mock logger calls
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create orchestrator
	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}
	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil, // abTestEvaluator not needed for this test
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Create task that simulates resuming after test phase completion
	// This is the key: RecipientOffset=1 means test phase already processed 1 recipient
	// Phase="test" but winner is already selected, so should transition to "winner"
	// LastProcessedEmail stores the cursor for DB pagination
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:               "broadcast-123",
				TotalRecipients:           2,                        // Total of 2 recipients
				TestPhaseRecipientCount:   1,                        // Test phase processes 1 recipient (50% of 2)
				WinnerPhaseRecipientCount: 1,                        // Winner phase should process remaining 1 recipient
				RecipientOffset:           1,                        // Test phase already processed 1 recipient (progress counter)
				LastProcessedEmail:        "recipient1@example.com", // Cursor for DB pagination
				EnqueuedCount:             1,                        // Test phase sent to 1 recipient
				FailedCount:               0,
				Phase:                     "test", // Should transition to "winner" when processing starts
				TestPhaseCompleted:        true,
			},
		},
	}

	// Process the task
	ctx := context.Background()
	timeout := time.Now().Add(30 * time.Second)
	done, err := orchestrator.Process(ctx, task, timeout)

	// Verify results
	require.NoError(t, err)
	assert.True(t, done, "Task should be marked as done")
	assert.Equal(t, "winner", task.State.SendBroadcast.Phase, "Phase should be 'winner'")
	assert.Equal(t, int64(2), task.State.SendBroadcast.RecipientOffset, "Should have processed 2 recipients total")
	assert.Equal(t, 2, task.State.SendBroadcast.EnqueuedCount, "Should have sent to 2 recipients total")
	assert.Equal(t, 100.0, task.Progress, "Task progress should be 100%")
}

// TestBroadcastOrchestrator_Process_NoRecipientsUpdatesBroadcastStatus tests that
// when a broadcast has no recipients, both the task and broadcast are marked as completed
func TestBroadcastOrchestrator_Process_NoRecipientsUpdatesBroadcastStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
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

	// Setup time provider
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()

	// Mock broadcast
	testBroadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: "workspace-123",
		Status:      domain.BroadcastStatusProcessing,
		Audience: domain.AudienceSettings{
			List: "empty-list",
		},
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{
				{TemplateID: "template-1"},
			},
		},
	}

	// Mock GetBroadcast - called twice: once for counting recipients, once for updating status
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(testBroadcast, nil).Times(2)

	// Mock CountContactsForBroadcast to return 0 recipients
	mockContactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "workspace-123", testBroadcast.Audience).Return(0, nil)

	// Mock UpdateBroadcast - THIS IS THE KEY ASSERTION
	// The broadcast status should be updated to "sent" with a CompletedAt timestamp
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
		// Verify the broadcast was properly updated
		assert.Equal(t, domain.BroadcastStatusProcessed, b.Status, "Broadcast status should be 'sent'")
		assert.NotNil(t, b.CompletedAt, "CompletedAt should be set")
		return nil
	})

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil, // abTestEvaluator not needed
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Create task with no recipients counted yet
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State: &domain.TaskState{
			Progress: 0,
			Message:  "Starting broadcast",
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     "broadcast-123",
				TotalRecipients: 0, // Not counted yet
				EnqueuedCount:   0,
				FailedCount:     0,
				RecipientOffset: 0,
			},
		},
		RetryCount: 0,
		MaxRetries: 3,
	}

	// Execute
	ctx := context.Background()
	timeout := time.Now().Add(30 * time.Second)
	done, err := orchestrator.Process(ctx, task, timeout)

	// Verify
	require.NoError(t, err)
	assert.True(t, done, "Task should be marked as done")
	assert.Equal(t, 100.0, task.Progress, "Task progress should be 100%")
	assert.Equal(t, "Broadcast completed: No recipients found", task.State.Message)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// TestNewBroadcastOrchestrator_DefaultConfig tests that NewBroadcastOrchestrator properly handles nil config
func TestNewBroadcastOrchestrator_DefaultConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Test with nil config - should use default config
	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		nil, // nil config
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	assert.NotNil(t, orchestrator)
}

// TestNewBroadcastOrchestrator_DefaultTimeProvider tests that NewBroadcastOrchestrator properly handles nil timeProvider
func TestNewBroadcastOrchestrator_DefaultTimeProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	// Test with nil timeProvider - should use default time provider
	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		nil, // nil timeProvider
		"https://api.example.com",
		mockEventBus,
	)

	assert.NotNil(t, orchestrator)
}

// TestBroadcastOrchestrator_ValidateTemplates_EmptyTemplates tests ValidateTemplates with empty template map
func TestBroadcastOrchestrator_ValidateTemplates_EmptyTemplates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Test with empty templates map
	err := orchestrator.ValidateTemplates(map[string]*domain.Template{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no templates provided for validation")
}

// TestBroadcastOrchestrator_ValidateTemplates_NilTemplate tests ValidateTemplates with nil template
func TestBroadcastOrchestrator_ValidateTemplates_NilTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Test with nil template
	templates := map[string]*domain.Template{
		"template-1": nil,
	}
	err := orchestrator.ValidateTemplates(templates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template is nil")
}

// TestBroadcastOrchestrator_ValidateTemplates_MissingEmailConfig tests ValidateTemplates with missing email config
func TestBroadcastOrchestrator_ValidateTemplates_MissingEmailConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations for error logging
	mockLogger.EXPECT().WithField("template_id", "template-1").Return(mockLogger)
	mockLogger.EXPECT().Error("Template missing email configuration")

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Test with template missing email config
	templates := map[string]*domain.Template{
		"template-1": {
			ID:    "template-1",
			Email: nil, // Missing email config
		},
	}
	err := orchestrator.ValidateTemplates(templates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template missing email configuration")
}

// TestBroadcastOrchestrator_ValidateTemplates_MissingContent tests ValidateTemplates with missing content
func TestBroadcastOrchestrator_ValidateTemplates_MissingContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations for error logging
	mockLogger.EXPECT().WithField("template_id", "template-1").Return(mockLogger)
	mockLogger.EXPECT().Error("Template missing content")

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Test with template missing content
	emptyBase := notifuse_mjml.NewBaseBlock("", "")
	templates := map[string]*domain.Template{
		"template-1": {
			ID: "template-1",
			Email: &domain.EmailTemplate{
				Subject:          "Test Subject",
				SenderID:         "sender-1",
				VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: emptyBase},
			},
		},
	}
	err := orchestrator.ValidateTemplates(templates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template missing content")
}

// TestBroadcastOrchestrator_FetchBatch_BroadcastNotFound tests FetchBatch when broadcast is not found
func TestBroadcastOrchestrator_FetchBatch_BroadcastNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations for error logging
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Mock GetBroadcast to return an error
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(nil, errors.New("broadcast not found"))

	// Execute
	ctx := context.Background()
	contacts, err := orchestrator.FetchBatch(ctx, "workspace-123", "broadcast-123", "", 10)

	// Verify
	assert.Nil(t, contacts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast not found")
}

// TestBroadcastOrchestrator_FetchBatch_BroadcastCancelled tests FetchBatch when broadcast is cancelled
func TestBroadcastOrchestrator_FetchBatch_BroadcastCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations for debug logging
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Mock GetBroadcast to return a cancelled broadcast
	cancelledBroadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: "workspace-123",
		Status:      domain.BroadcastStatusCancelled,
		Audience:    domain.AudienceSettings{},
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(cancelledBroadcast, nil)

	// Execute
	ctx := context.Background()
	contacts, err := orchestrator.FetchBatch(ctx, "workspace-123", "broadcast-123", "", 10)

	// Verify
	assert.Nil(t, contacts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast has been cancelled")
}

// TestBroadcastOrchestrator_FetchBatch_ContactRepoError tests FetchBatch when contact repo returns error
func TestBroadcastOrchestrator_FetchBatch_ContactRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations for error and debug logging
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Mock GetBroadcast to return a valid broadcast
	broadcast := &domain.Broadcast{
		ID:          "broadcast-123",
		WorkspaceID: "workspace-123",
		Status:      domain.BroadcastStatusProcessing,
		Audience:    domain.AudienceSettings{},
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(broadcast, nil)

	// Mock GetContactsForBroadcast to return an error (with empty cursor for first batch)
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", broadcast.Audience, 10, "").Return(nil, errors.New("database connection error"))

	// Execute
	ctx := context.Background()
	contacts, err := orchestrator.FetchBatch(ctx, "workspace-123", "broadcast-123", "", 10)

	// Verify
	assert.Nil(t, contacts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch recipients")
}

// TestBroadcastOrchestrator_SaveProgressState_SaveStateError tests SaveProgressState when SaveState fails
func TestBroadcastOrchestrator_SaveProgressState_SaveStateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Setup logger expectations for error logging
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Setup time provider
	testTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(testTime)

	config := &broadcast.Config{
		FetchBatchSize:      100,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	// Mock SaveState to return an error
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(errors.New("database error"))

	// Execute
	ctx := context.Background()
	broadcastState := &domain.SendBroadcastState{
		BroadcastID:     "broadcast-123",
		TotalRecipients: 100,
		EnqueuedCount:   0,
		FailedCount:     0,
	}
	lastSaveTime := time.Date(2023, 1, 1, 11, 59, 0, 0, time.UTC)
	startTime := time.Date(2023, 1, 1, 11, 58, 0, 0, time.UTC)

	newSaveTime, err := orchestrator.SaveProgressState(ctx, "workspace-123", "task-123", broadcastState, 10, 2, 12, lastSaveTime, startTime)

	// Verify
	assert.Equal(t, lastSaveTime, newSaveTime) // Should return original lastSaveTime on error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save task state")
}

// TestBroadcastOrchestrator_Process_PartialBatchCursorUpdate verifies that when SendBatch
// processes fewer contacts than were fetched (e.g., due to timeout), the cursor is correctly
// updated to the last PROCESSED contact, not the last FETCHED contact.
// This test ensures no contacts are skipped when resuming after a partial batch.
func TestBroadcastOrchestrator_Process_PartialBatchCursorUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
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

	// Setup time provider - return a fixed time that's always before timeout
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	sendBatchCalled := false
	mockTimeProvider.EXPECT().Now().DoAndReturn(func() time.Time {
		// After SendBatch is called (which processes partial batch), simulate timeout
		if sendBatchCalled {
			return baseTime.Add(35 * time.Second) // Past timeout
		}
		return baseTime
	}).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(10 * time.Second).AnyTimes()

	// Mock workspace
	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{
				ID:   "marketing-provider-id",
				Name: "Marketing Provider",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSES,
					SES: &domain.AmazonSESSettings{
						AccessKey: "access-key",
						SecretKey: "secret-key",
						Region:    "us-east-1",
					},
				},
			},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

	// Mock broadcast
	testBroadcast := &domain.Broadcast{
		ID: "broadcast-123",
		TestSettings: domain.BroadcastTestSettings{
			Enabled: false,
			Variations: []domain.BroadcastVariation{
				{TemplateID: "template-1"},
			},
		},
		Audience: domain.AudienceSettings{
			List: "list-1",
		},
		Status: domain.BroadcastStatusProcessing,
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(testBroadcast, nil).AnyTimes()
	// Mock broadcast status update on completion
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Mock template
	template := &domain.Template{
		ID: "template-1",
		Email: &domain.EmailTemplate{
			Subject:  "Test Subject",
			SenderID: "sender-123",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root1", notifuse_mjml.MJMLComponentMjml),
			},
		},
	}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(template, nil)

	// Mock recipients - first batch: 5 contacts fetched
	recipients1 := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "user1@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user2@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user3@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user4@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user5@example.com"}, ListID: "list-1"},
	}
	// Second batch: remaining 2 contacts (user4, user5) - fetched after cursor user3
	recipients2 := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "user4@example.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user5@example.com"}, ListID: "list-1"},
	}

	// First fetch: empty cursor, get 5 contacts
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", testBroadcast.Audience, 5, "").Return(recipients1, nil)
	// Second fetch: cursor at user3 (after partial first batch), get remaining 2 contacts
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", testBroadcast.Audience, 2, "user3@example.com").Return(recipients2, nil)

	// Mock SendBatch - first call: simulates partial completion (3 of 5 sent)
	mockMessageSender.EXPECT().SendBatch(
		gomock.Any(),
		"workspace-123",
		"marketing-provider-id",
		"secret-key",
		gomock.Any(),
		true,
		"broadcast-123",
		recipients1,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _, _, _, _ interface{}, _ bool, _ string, _ []*domain.ContactWithList, _, _, _ interface{}) (int, int, error) {
		sendBatchCalled = true
		return 3, 0, nil // Only 3 sent due to internal timeout
	})
	// Second call: sends remaining 2 contacts
	mockMessageSender.EXPECT().SendBatch(
		gomock.Any(),
		"workspace-123",
		"marketing-provider-id",
		"secret-key",
		gomock.Any(),
		true,
		"broadcast-123",
		recipients2,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(2, 0, nil)

	// Capture the saved state to verify cursor
	var savedState *domain.TaskState
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, workspaceID, taskID string, progress float64, state *domain.TaskState) error {
			savedState = state
			return nil
		}).AnyTimes()

	config := &broadcast.Config{
		FetchBatchSize:      50,
		MaxProcessTime:      30 * time.Second,
		ProgressLogInterval: 5 * time.Second,
	}

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepo,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil,
		mockLogger,
		config,
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State: &domain.TaskState{
			Progress: 0,
			Message:  "Starting broadcast",
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     "broadcast-123",
				TotalRecipients: 5,
				EnqueuedCount:   0,
				FailedCount:     0,
				RecipientOffset: 0,
				Phase:           "single",
			},
		},
		RetryCount: 0,
		MaxRetries: 3,
	}

	// Execute with a real future timeout (orchestrator uses time.Now() directly for timeout check)
	ctx := context.Background()
	timeoutAt := time.Now().Add(30 * time.Second)
	done, err := orchestrator.Process(ctx, task, timeoutAt)

	// Verify
	require.NoError(t, err)
	// Task should be done since all contacts were eventually sent
	assert.True(t, done, "Task should be complete after all contacts sent")

	// The key test is that the second GetContactsForBroadcast call received cursor "user3@example.com"
	// This proves the fix works: after partial batch (3 of 5 sent), cursor was correctly set to user3
	// (the last PROCESSED contact), not user5 (the last FETCHED contact).
	// If the bug was still present, the second call would have received cursor "user5@example.com"
	// and would have failed because there's no mock for that.

	// Also verify final state
	require.NotNil(t, savedState, "State should have been saved")
	require.NotNil(t, savedState.SendBroadcast, "SendBroadcast state should exist")
	// Final cursor should be at user5 (last contact)
	assert.Equal(t, "user5@example.com", savedState.SendBroadcast.LastProcessedEmail,
		"Final cursor should be at the last processed contact (user5)")
	// Total processed should be 5 (3 from first batch + 2 from second batch)
	assert.Equal(t, int64(5), savedState.SendBroadcast.RecipientOffset,
		"RecipientOffset should reflect all processed contacts (5)")
}

func TestProcessBroadcastTask_RecipientFeedFailure_PausesBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Time provider
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(5 * time.Second).AnyTimes()

	// Workspace with email provider
	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{ID: "marketing-provider-id", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

	// Broadcast with single variation
	bcast := &domain.Broadcast{
		ID:     "broadcast-123",
		Status: domain.BroadcastStatusProcessing,
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{TemplateID: "template-1"}},
		},
		Audience: domain.AudienceSettings{List: "list-1"},
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(bcast, nil).AnyTimes()

	// Template
	tpl := &domain.Template{ID: "template-1", Email: &domain.EmailTemplate{Subject: "S", SenderID: "s", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(tpl, nil)

	// Contacts
	recipients := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "user1@test.com"}, ListID: "list-1"},
		{Contact: &domain.Contact{Email: "user2@test.com"}, ListID: "list-1"},
	}
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", bcast.Audience, 2, "").Return(recipients, nil)

	// SendBatch returns ErrBroadcastShouldPause
	mockMessageSender.EXPECT().SendBatch(
		gomock.Any(), "workspace-123", "marketing-provider-id", "secret-key", gomock.Any(), true, "broadcast-123",
		recipients, gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(0, 0, fmt.Errorf("%w: recipient feed failed for user1@test.com: server error", broadcast.ErrBroadcastShouldPause))

	// UpdateBroadcast should set status to paused
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
		assert.Equal(t, domain.BroadcastStatusPaused, b.Status)
		assert.NotNil(t, b.PausedAt)
		require.NotNil(t, b.PauseReason)
		assert.Contains(t, *b.PauseReason, "Recipient feed failed")
		return nil
	})

	// EventBus should publish a paused event
	mockEventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, event domain.EventPayload) {
		assert.Equal(t, domain.EventBroadcastPaused, event.Type)
		assert.Equal(t, "workspace-123", event.WorkspaceID)
		assert.Equal(t, "broadcast-123", event.EntityID)
		assert.Equal(t, "recipient_feed_failed", event.Data["reason"])
	})

	// Save state
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State:       &domain.TaskState{Progress: 0, Message: "Starting", SendBroadcast: &domain.SendBroadcastState{BroadcastID: "broadcast-123", TotalRecipients: 2, RecipientOffset: 0}},
		RetryCount:  0,
		MaxRetries:  3,
	}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))
	assert.False(t, done)
	require.Error(t, err)
	assert.True(t, errors.Is(err, broadcast.ErrBroadcastShouldPause))
}

func TestProcessBroadcastTask_RecipientFeedFailure_NotMarkedAsFailed(t *testing.T) {
	// When isLastRetry is true, the defer block should skip marking as failed
	// because the broadcast was already paused due to recipient feed failure.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageSender := mocks.NewMockMessageSender(ctrl)
	mockBroadcastRepo := domainmocks.NewMockBroadcastRepository(ctrl)
	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTimeProvider := mocks.NewMockTimeProvider(ctrl)
	mockEventBus := domainmocks.NewMockEventBus(ctrl)

	// Logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	// Time provider
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	mockTimeProvider.EXPECT().Now().Return(baseTime).AnyTimes()
	mockTimeProvider.EXPECT().Since(gomock.Any()).Return(5 * time.Second).AnyTimes()

	// Workspace with email provider
	workspace := &domain.Workspace{
		ID: "workspace-123",
		Settings: domain.WorkspaceSettings{
			SecretKey:                "secret-key",
			EmailTrackingEnabled:     true,
			MarketingEmailProviderID: "marketing-provider-id",
		},
		Integrations: []domain.Integration{
			{ID: "marketing-provider-id", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}},
		},
	}
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil)

	// Broadcast with single variation
	bcast := &domain.Broadcast{
		ID:     "broadcast-123",
		Status: domain.BroadcastStatusProcessing,
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{TemplateID: "template-1"}},
		},
		Audience: domain.AudienceSettings{List: "list-1"},
	}
	mockBroadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").Return(bcast, nil).AnyTimes()

	// Template
	tpl := &domain.Template{ID: "template-1", Email: &domain.EmailTemplate{Subject: "S", SenderID: "s", VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}}}
	mockTemplateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(tpl, nil)

	// Contacts
	recipients := []*domain.ContactWithList{
		{Contact: &domain.Contact{Email: "user1@test.com"}, ListID: "list-1"},
	}
	mockContactRepo.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", bcast.Audience, 1, "").Return(recipients, nil)

	// SendBatch returns ErrBroadcastShouldPause
	mockMessageSender.EXPECT().SendBatch(
		gomock.Any(), "workspace-123", "marketing-provider-id", "secret-key", gomock.Any(), true, "broadcast-123",
		recipients, gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(0, 0, fmt.Errorf("%w: recipient feed failed for user1@test.com: server error", broadcast.ErrBroadcastShouldPause))

	// UpdateBroadcast should be called exactly ONCE (for pause), NOT for failed
	mockBroadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
		// Must be paused, not failed
		assert.Equal(t, domain.BroadcastStatusPaused, b.Status, "broadcast should be paused, not failed")
		assert.NotNil(t, b.PausedAt)
		return nil
	}).Times(1) // Exactly once — defer must NOT call UpdateBroadcast again

	// EventBus publish for paused event
	mockEventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).AnyTimes()

	// Save state
	mockTaskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	orchestrator := broadcast.NewBroadcastOrchestrator(mockMessageSender, mockBroadcastRepo, mockTemplateRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, nil, mockLogger, config, mockTimeProvider, "https://api.example.com", mockEventBus)

	ctx := context.Background()
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Type:        "send_broadcast",
		BroadcastID: stringPtr("broadcast-123"),
		State:       &domain.TaskState{Progress: 0, Message: "Starting", SendBroadcast: &domain.SendBroadcastState{BroadcastID: "broadcast-123", TotalRecipients: 1, RecipientOffset: 0}},
		RetryCount:  2, // isLastRetry = true (2 >= 3-1)
		MaxRetries:  3,
	}

	done, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))
	assert.False(t, done)
	require.Error(t, err)
	assert.True(t, errors.Is(err, broadcast.ErrBroadcastShouldPause))
}
