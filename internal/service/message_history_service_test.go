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
)

func TestMessageHistoryService_ListMessages(t *testing.T) {
	// Create fixed timestamps for testing
	now := time.Now()
	twoHoursAgo := now.Add(-2 * time.Hour)

	// Define test cases
	testCases := []struct {
		name           string
		workspaceID    string
		params         domain.MessageListParams
		setupMocks     func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService)
		expectedResult *domain.MessageListResult
		expectedError  error
	}{
		{
			name:        "Success with messages and next cursor",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit: 10,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				messages := []*domain.MessageHistory{
					{
						ID:           "msg-1",
						ContactEmail: "user@example.com",
						TemplateID:   "template-1",
						Channel:      "email",
					},
					{
						ID:           "msg-2",
						ContactEmail: "user2@example.com",
						TemplateID:   "template-1",
						Channel:      "email",
					},
				}
				nextCursor := "cursor-value"
				mockRepo.EXPECT().
					ListMessages(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any()).
					Return(messages, nextCursor, nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages: []*domain.MessageHistory{
					{
						ID:           "msg-1",
						ContactEmail: "user@example.com",
						TemplateID:   "template-1",
						Channel:      "email",
					},
					{
						ID:           "msg-2",
						ContactEmail: "user2@example.com",
						TemplateID:   "template-1",
						Channel:      "email",
					},
				},
				NextCursor: "cursor-value",
				HasMore:    true,
			},
			expectedError: nil,
		},
		{
			name:        "Success with empty result",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit: 10,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				mockRepo.EXPECT().
					ListMessages(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any()).
					Return([]*domain.MessageHistory{}, "", nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages:   []*domain.MessageHistory{},
				NextCursor: "",
				HasMore:    false,
			},
			expectedError: nil,
		},
		{
			name:        "Authentication error",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit: 10,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(nil, nil, nil, errors.New("authentication failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("failed to authenticate user: authentication failed"),
		},
		{
			name:        "Repository error",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit: 10,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				mockRepo.EXPECT().
					ListMessages(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any()).
					Return(nil, "", errors.New("database error"))

				// We expect the error to be logged
				mockLogger.EXPECT().
					Error(gomock.Any())
			},
			expectedResult: nil,
			expectedError:  errors.New("database error"),
		},
		{
			name:        "With filters",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit:        10,
				Channel:      "email",
				ContactEmail: "user@example.com",
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				messages := []*domain.MessageHistory{
					{
						ID:           "msg-1",
						ContactEmail: "user@example.com",
						TemplateID:   "template-1",
						Channel:      "email",
					},
				}
				mockRepo.EXPECT().
					ListMessages(
						gomock.Any(),
						"workspace-123",
						gomock.Any(),
						gomock.Any(),
					).
					Do(func(_ context.Context, _ string, _ string, params domain.MessageListParams) {
						assert.Equal(t, "email", params.Channel)
						assert.Equal(t, "user@example.com", params.ContactEmail)
						assert.Equal(t, 10, params.Limit)
					}).
					Return(messages, "", nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages: []*domain.MessageHistory{
					{
						ID:           "msg-1",
						ContactEmail: "user@example.com",
						TemplateID:   "template-1",
						Channel:      "email",
					},
				},
				NextCursor: "",
				HasMore:    false,
			},
			expectedError: nil,
		},
		{
			name:        "With cursor-based pagination",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Cursor: "previous-cursor",
				Limit:  10,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				messages := []*domain.MessageHistory{
					{
						ID:           "msg-3",
						ContactEmail: "user3@example.com",
						TemplateID:   "template-2",
						Channel:      "email",
					},
					{
						ID:           "msg-4",
						ContactEmail: "user4@example.com",
						TemplateID:   "template-2",
						Channel:      "email",
					},
				}

				mockRepo.EXPECT().
					ListMessages(
						gomock.Any(),
						"workspace-123",
						gomock.Any(),
						gomock.Any(),
					).
					Do(func(_ context.Context, _ string, _ string, params domain.MessageListParams) {
						assert.Equal(t, "previous-cursor", params.Cursor)
						assert.Equal(t, 10, params.Limit)
					}).
					Return(messages, "next-cursor", nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages: []*domain.MessageHistory{
					{
						ID:           "msg-3",
						ContactEmail: "user3@example.com",
						TemplateID:   "template-2",
						Channel:      "email",
					},
					{
						ID:           "msg-4",
						ContactEmail: "user4@example.com",
						TemplateID:   "template-2",
						Channel:      "email",
					},
				},
				NextCursor: "next-cursor",
				HasMore:    true,
			},
			expectedError: nil,
		},
		{
			name:        "With time filters",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit:      10,
				SentAfter:  &twoHoursAgo,
				SentBefore: &now,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				messages := []*domain.MessageHistory{
					{
						ID:           "msg-5",
						ContactEmail: "user5@example.com",
						TemplateID:   "template-3",
						Channel:      "email",
						SentAt:       twoHoursAgo.Add(time.Hour),
					},
				}

				mockRepo.EXPECT().
					ListMessages(
						gomock.Any(),
						"workspace-123",
						gomock.Any(),
						gomock.Any(),
					).
					Do(func(_ context.Context, _ string, _ string, params domain.MessageListParams) {
						assert.Equal(t, twoHoursAgo, *params.SentAfter)
						assert.Equal(t, now, *params.SentBefore)
					}).
					Return(messages, "", nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages: []*domain.MessageHistory{
					{
						ID:           "msg-5",
						ContactEmail: "user5@example.com",
						TemplateID:   "template-3",
						Channel:      "email",
						SentAt:       twoHoursAgo.Add(time.Hour),
					},
				},
				NextCursor: "",
				HasMore:    false,
			},
			expectedError: nil,
		},
		{
			name:        "Multiple filter combinations",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Limit:       10,
				Channel:     "email",
				BroadcastID: "broadcast-123",
				TemplateID:  "template-5",
				IsSent:      boolPtr(true),
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				broadcastID := "broadcast-123"
				errMsg := "template error"
				messages := []*domain.MessageHistory{
					{
						ID:           "msg-7",
						ContactEmail: "user7@example.com",
						BroadcastID:  &broadcastID,
						TemplateID:   "template-5",
						Channel:      "email",
						StatusInfo:   &errMsg,
					},
				}

				mockRepo.EXPECT().
					ListMessages(
						gomock.Any(),
						"workspace-123",
						gomock.Any(),
						gomock.Any(),
					).
					Do(func(_ context.Context, _ string, _ string, params domain.MessageListParams) {
						assert.Equal(t, "email", params.Channel)
						assert.Equal(t, "broadcast-123", params.BroadcastID)
						assert.Equal(t, "template-5", params.TemplateID)
						assert.True(t, *params.IsSent)
					}).
					Return(messages, "", nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages: []*domain.MessageHistory{
					{
						ID:           "msg-7",
						ContactEmail: "user7@example.com",
						BroadcastID:  strPtr("broadcast-123"),
						TemplateID:   "template-5",
						Channel:      "email",
						StatusInfo:   strPtr("template error"),
					},
				},
				NextCursor: "",
				HasMore:    false,
			},
			expectedError: nil,
		},
		{
			name:        "Last page of results",
			workspaceID: "workspace-123",
			params: domain.MessageListParams{
				Cursor: "last-page-cursor",
				Limit:  10,
			},
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(&domain.Workspace{ID: "workspace-123", Settings: domain.WorkspaceSettings{SecretKey: "test-secret"}}, nil)

				messages := []*domain.MessageHistory{
					{
						ID:           "msg-8",
						ContactEmail: "user8@example.com",
						TemplateID:   "template-6",
						Channel:      "email",
					},
				}
				// Empty next cursor indicates no more pages
				mockRepo.EXPECT().
					ListMessages(gomock.Any(), "workspace-123", gomock.Any(), gomock.Any()).
					Return(messages, "", nil)
			},
			expectedResult: &domain.MessageListResult{
				Messages: []*domain.MessageHistory{
					{
						ID:           "msg-8",
						ContactEmail: "user8@example.com",
						TemplateID:   "template-6",
						Channel:      "email",
					},
				},
				NextCursor: "",
				HasMore:    false,
			},
			expectedError: nil,
		},
	}

	// Execute test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			tc.setupMocks(mockRepo, mockWorkspaceRepo, mockLogger, mockAuthService)

			// Create service with mocks
			service := NewMessageHistoryService(mockRepo, mockWorkspaceRepo, mockLogger, mockAuthService)

			// Call the method under test
			result, err := service.ListMessages(context.Background(), tc.workspaceID, tc.params)

			// Verify expectations
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestMessageHistoryService_GetBroadcastStats(t *testing.T) {
	testCases := []struct {
		name          string
		workspaceID   string
		broadcastID   string
		setupMocks    func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService)
		expectedStats *domain.MessageHistoryStatusSum
		expectedError error
	}{
		{
			name:        "Success with stats",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				stats := &domain.MessageHistoryStatusSum{
					TotalSent:         100,
					TotalDelivered:    95,
					TotalBounced:      2,
					TotalComplained:   1,
					TotalFailed:       2,
					TotalOpened:       80,
					TotalClicked:      60,
					TotalUnsubscribed: 3,
				}
				mockRepo.EXPECT().
					GetBroadcastStats(gomock.Any(), "workspace-123", "broadcast-123").
					Return(stats, nil)
			},
			expectedStats: &domain.MessageHistoryStatusSum{
				TotalSent:         100,
				TotalDelivered:    95,
				TotalBounced:      2,
				TotalComplained:   1,
				TotalFailed:       2,
				TotalOpened:       80,
				TotalClicked:      60,
				TotalUnsubscribed: 3,
			},
			expectedError: nil,
		},
		{
			name:        "Authentication error",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(nil, nil, nil, errors.New("authentication failed"))
			},
			expectedStats: nil,
			expectedError: errors.New("failed to authenticate user: authentication failed"),
		},
		{
			name:        "Repository error",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					GetBroadcastStats(gomock.Any(), "workspace-123", "broadcast-123").
					Return(nil, errors.New("database error"))
			},
			expectedStats: nil,
			expectedError: errors.New("failed to get broadcast stats: database error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			tc.setupMocks(mockRepo, mockAuthService)

			// Create service with mocks
			service := NewMessageHistoryService(mockRepo, mockWorkspaceRepo, mockLogger, mockAuthService)

			// Call the method under test
			stats, err := service.GetBroadcastStats(context.Background(), tc.workspaceID, tc.broadcastID)

			// Verify expectations
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedStats, stats)
			}
		})
	}
}

func TestMessageHistoryService_GetBroadcastVariationStats(t *testing.T) {
	testCases := []struct {
		name          string
		workspaceID   string
		broadcastID   string
		templateID    string
		setupMocks    func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService)
		expectedStats *domain.MessageHistoryStatusSum
		expectedError error
	}{
		{
			name:        "Success with template stats",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			templateID:  "template-abc",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				// Let's assume the mock repository has a GetBroadcastVariationStats method
				mockRepo.EXPECT().
					GetBroadcastVariationStats(gomock.Any(), "workspace-123", "broadcast-123", "template-abc").
					Return(&domain.MessageHistoryStatusSum{
						TotalSent:         50,
						TotalDelivered:    48,
						TotalBounced:      1,
						TotalComplained:   0,
						TotalFailed:       1,
						TotalOpened:       40,
						TotalClicked:      30,
						TotalUnsubscribed: 2,
					}, nil)
			},
			expectedStats: &domain.MessageHistoryStatusSum{
				TotalSent:         50,
				TotalDelivered:    48,
				TotalBounced:      1,
				TotalComplained:   0,
				TotalFailed:       1,
				TotalOpened:       40,
				TotalClicked:      30,
				TotalUnsubscribed: 2,
			},
			expectedError: nil,
		},
		{
			name:        "Authentication error",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			templateID:  "template-abc",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(nil, nil, nil, errors.New("authentication failed"))
			},
			expectedStats: nil,
			expectedError: errors.New("failed to authenticate user: authentication failed"),
		},
		{
			name:        "Repository error",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			templateID:  "template-abc",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)

				mockRepo.EXPECT().
					GetBroadcastVariationStats(gomock.Any(), "workspace-123", "broadcast-123", "template-abc").
					Return(nil, errors.New("database error"))
			},
			expectedStats: nil,
			expectedError: errors.New("failed to get broadcast variation stats: database error"),
		},
		{
			name:        "Invalid template ID",
			workspaceID: "workspace-123",
			broadcastID: "broadcast-123",
			templateID:  "",
			setupMocks: func(mockRepo *mocks.MockMessageHistoryRepository, mockAuthService *mocks.MockAuthService) {
				mockAuthService.EXPECT().
					AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").
					Return(context.Background(), &domain.User{}, &domain.UserWorkspace{
						UserID:      "user123",
						WorkspaceID: "workspace-123",
						Role:        "member",
						Permissions: domain.UserPermissions{
							domain.PermissionResourceMessageHistory: {Read: true, Write: true},
						},
					}, nil)
			},
			expectedStats: nil,
			expectedError: errors.New("template ID cannot be empty"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			tc.setupMocks(mockRepo, mockAuthService)

			// Create service with mocks
			service := NewMessageHistoryService(mockRepo, mockWorkspaceRepo, mockLogger, mockAuthService)

			// Call the method under test
			stats, err := service.GetBroadcastVariationStats(context.Background(), tc.workspaceID, tc.broadcastID, tc.templateID)

			// Verify expectations
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedStats, stats)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
