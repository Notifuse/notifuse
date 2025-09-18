package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnalyticsService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := logger.NewLogger()

	service := NewAnalyticsService(mockRepo, mockAuth, mockLogger)

	assert.NotNil(t, service)
	assert.IsType(t, &AnalyticsService{}, service)
}

func TestAnalyticsService_Query(t *testing.T) {
	tests := []struct {
		name          string
		workspaceID   string
		query         analytics.Query
		setupMocks    func(*mocks.MockAnalyticsRepository, *mocks.MockAuthService)
		expectedError string
		expectedData  []map[string]interface{}
	}{
		{
			name:        "successful query execution",
			workspaceID: "test-workspace",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				user := &domain.User{ID: "user-123", Email: "test@example.com"}
				userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}
				ctx := context.Background()

				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(ctx, user, userWorkspace, nil)

				schemas := map[string]analytics.SchemaDefinition{
					"message_history": domain.PredefinedSchemas["message_history"],
				}
				mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return(schemas, nil)

				response := &analytics.Response{
					Data: []map[string]interface{}{
						{"count": 42},
					},
					Meta: analytics.Meta{
						Query:  "SELECT COUNT(*) AS count FROM message_history",
						Params: []interface{}{},
					},
				}
				mockRepo.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
					Return(response, nil)
			},
			expectedData: []map[string]interface{}{
				{"count": 42},
			},
		},
		{
			name:        "authentication failure",
			workspaceID: "test-workspace",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(context.Background(), (*domain.User)(nil), (*domain.UserWorkspace)(nil), assert.AnError)
			},
			expectedError: "failed to authenticate user",
		},
		{
			name:        "schema retrieval failure",
			workspaceID: "test-workspace",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				user := &domain.User{ID: "user-123", Email: "test@example.com"}
				userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}
				ctx := context.Background()

				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(ctx, user, userWorkspace, nil)

				mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return((map[string]analytics.SchemaDefinition)(nil), assert.AnError)
			},
			expectedError: "failed to get schemas",
		},
		{
			name:        "query validation failure",
			workspaceID: "test-workspace",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"invalid_measure"},
			},
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				user := &domain.User{ID: "user-123", Email: "test@example.com"}
				userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}
				ctx := context.Background()

				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(ctx, user, userWorkspace, nil)

				schemas := map[string]analytics.SchemaDefinition{
					"message_history": domain.PredefinedSchemas["message_history"],
				}
				mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return(schemas, nil)
			},
			expectedError: "query validation failed",
		},
		{
			name:        "repository query execution failure",
			workspaceID: "test-workspace",
			query: analytics.Query{
				Schema:   "message_history",
				Measures: []string{"count"},
			},
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				user := &domain.User{ID: "user-123", Email: "test@example.com"}
				userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}
				ctx := context.Background()

				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(ctx, user, userWorkspace, nil)

				schemas := map[string]analytics.SchemaDefinition{
					"message_history": domain.PredefinedSchemas["message_history"],
				}
				mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return(schemas, nil)

				mockRepo.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
					Return((*analytics.Response)(nil), assert.AnError)
			},
			expectedError: "failed to execute query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
			mockAuth := mocks.NewMockAuthService(ctrl)
			mockLogger := logger.NewLogger()

			// Setup mocks
			tt.setupMocks(mockRepo, mockAuth)

			// Create service
			service := NewAnalyticsService(mockRepo, mockAuth, mockLogger)

			// Execute query
			ctx := context.Background()
			response, err := service.Query(ctx, tt.workspaceID, tt.query)

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, tt.expectedData, response.Data)
			}

			// Expectations are automatically verified by gomock
		})
	}
}

func TestAnalyticsService_GetSchemas(t *testing.T) {
	tests := []struct {
		name          string
		workspaceID   string
		setupMocks    func(*mocks.MockAnalyticsRepository, *mocks.MockAuthService)
		expectedError string
		expectSchemas bool
	}{
		{
			name:        "successful schema retrieval",
			workspaceID: "test-workspace",
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				user := &domain.User{ID: "user-123", Email: "test@example.com"}
				userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}
				ctx := context.Background()

				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(ctx, user, userWorkspace, nil)

				schemas := map[string]analytics.SchemaDefinition{
					"message_history": domain.PredefinedSchemas["message_history"],
					"contacts":        domain.PredefinedSchemas["contacts"],
					"broadcasts":      domain.PredefinedSchemas["broadcasts"],
				}
				mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return(schemas, nil)
			},
			expectSchemas: true,
		},
		{
			name:        "authentication failure",
			workspaceID: "test-workspace",
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(context.Background(), (*domain.User)(nil), (*domain.UserWorkspace)(nil), assert.AnError)
			},
			expectedError: "failed to authenticate user",
		},
		{
			name:        "repository failure",
			workspaceID: "test-workspace",
			setupMocks: func(mockRepo *mocks.MockAnalyticsRepository, mockAuth *mocks.MockAuthService) {
				user := &domain.User{ID: "user-123", Email: "test@example.com"}
				userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}
				ctx := context.Background()

				mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
					Return(ctx, user, userWorkspace, nil)

				mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
					Return((map[string]analytics.SchemaDefinition)(nil), assert.AnError)
			},
			expectedError: "failed to get schemas",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
			mockAuth := mocks.NewMockAuthService(ctrl)
			mockLogger := logger.NewLogger()

			// Setup mocks
			tt.setupMocks(mockRepo, mockAuth)

			// Create service
			service := NewAnalyticsService(mockRepo, mockAuth, mockLogger)

			// Execute GetSchemas
			ctx := context.Background()
			schemas, err := service.GetSchemas(ctx, tt.workspaceID)

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, schemas)
			} else {
				assert.NoError(t, err)
				if tt.expectSchemas {
					assert.NotNil(t, schemas)
					assert.Contains(t, schemas, "message_history")
					assert.Contains(t, schemas, "contacts")
					assert.Contains(t, schemas, "broadcasts")
				}
			}

			// Expectations are automatically verified by gomock
		})
	}
}

func TestAnalyticsService_Interface(t *testing.T) {
	// Ensure AnalyticsService implements domain.AnalyticsService
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := logger.NewLogger()

	service := NewAnalyticsService(mockRepo, mockAuth, mockLogger)

	// This should compile without error
	var _ domain.AnalyticsService = service
}

func TestAnalyticsService_ContextPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
	mockAuth := mocks.NewMockAuthService(ctrl)
	mockLogger := logger.NewLogger()

	service := NewAnalyticsService(mockRepo, mockAuth, mockLogger)

	user := &domain.User{ID: "user-123", Email: "test@example.com"}
	userWorkspace := &domain.UserWorkspace{WorkspaceID: "test-workspace"}

	// Create a context with a value to verify it's propagated
	originalCtx := context.WithValue(context.Background(), "test-key", "test-value")
	modifiedCtx := context.WithValue(originalCtx, "auth-key", "auth-value")

	mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
		Return(modifiedCtx, user, userWorkspace, nil)

	schemas := map[string]analytics.SchemaDefinition{
		"message_history": domain.PredefinedSchemas["message_history"],
	}
	mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
		Return(schemas, nil)

	response := &analytics.Response{
		Data: []map[string]interface{}{{"count": 42}},
		Meta: analytics.Meta{Query: "SELECT COUNT(*) FROM test", Params: []interface{}{}},
	}
	mockRepo.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
		Return(response, nil)

	query := analytics.Query{
		Schema:   "message_history",
		Measures: []string{"count"},
	}

	result, err := service.Query(originalCtx, "test-workspace", query)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Expectations are automatically verified by gomock
}
