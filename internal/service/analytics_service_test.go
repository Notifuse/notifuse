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

type testContextKey string

const testKey testContextKey = "test-key"
const authKey testContextKey = "auth-key"

// analyticsMember builds a non-owner membership granting read on the given
// resources, so the analytics gates have to consult the grants rather than
// short-circuit on the owner role.
func analyticsMember(resources ...domain.PermissionResource) *domain.UserWorkspace {
	permissions := domain.UserPermissions{}
	for _, resource := range resources {
		permissions[resource] = domain.ResourcePermissions{Read: true}
	}
	return &domain.UserWorkspace{
		WorkspaceID: "test-workspace",
		Role:        "member",
		Permissions: permissions,
	}
}

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
				userWorkspace := analyticsMember(domain.PermissionResourceMessageHistory)
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
				userWorkspace := analyticsMember(domain.PermissionResourceMessageHistory)
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
				userWorkspace := analyticsMember(domain.PermissionResourceMessageHistory)
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
				userWorkspace := analyticsMember(domain.PermissionResourceMessageHistory)
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
				userWorkspace := analyticsMember(
					domain.PermissionResourceMessageHistory,
					domain.PermissionResourceContacts,
					domain.PermissionResourceBroadcasts,
				)
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
				userWorkspace := analyticsMember(domain.PermissionResourceMessageHistory)
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
	userWorkspace := analyticsMember(domain.PermissionResourceMessageHistory)

	// Create a context with a value to verify it's propagated
	originalCtx := context.WithValue(context.Background(), testKey, "test-value")
	modifiedCtx := context.WithValue(originalCtx, authKey, "auth-value")

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

// TestAnalyticsService_SchemaPermissionEnforcement pins the schema → resource
// mapping from both sides: a member holding every permission EXCEPT read on the
// resource that owns the schema is refused, and the same member with that one
// grant restored is served. The refused member holds WRITE on the owning
// resource, so the test also fails if the gate asks for the wrong verb, and the
// case table is checked against domain.PredefinedSchemas so a schema added
// without a mapping fails here rather than shipping ungated.
func TestAnalyticsService_SchemaPermissionEnforcement(t *testing.T) {
	owning := map[string]domain.PermissionResource{
		"message_history":            domain.PermissionResourceMessageHistory,
		"contacts":                   domain.PermissionResourceContacts,
		"broadcasts":                 domain.PermissionResourceBroadcasts,
		"webhook_deliveries":         domain.PermissionResourceWebhookSubscriptions,
		"email_queue":                domain.PermissionResourceMessageHistory,
		"automation_node_executions": domain.PermissionResourceAutomations,
		"web_sessions":               domain.PermissionResourceWebAnalytics,
		"web_pages":                  domain.PermissionResourceWebAnalytics,
		"web_goals":                  domain.PermissionResourceWebAnalytics,
	}

	for schema := range domain.PredefinedSchemas {
		_, covered := owning[schema]
		assert.True(t, covered, "predefined schema %q is not pinned to an owning resource here", schema)
	}

	// role "member" (not "owner") so HasPermission actually consults the grants.
	newService := func(t *testing.T, permissions domain.UserPermissions) (*AnalyticsService, *mocks.MockAnalyticsRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
		mockAuth := mocks.NewMockAuthService(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
			Return(context.Background(), &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				WorkspaceID: "test-workspace",
				Role:        "member",
				Permissions: permissions,
			}, nil)

		return NewAnalyticsService(mockRepo, mockAuth, logger.NewLogger()), mockRepo
	}

	countSchema := func(name string) map[string]analytics.SchemaDefinition {
		return map[string]analytics.SchemaDefinition{
			name: {
				Name:     name,
				Measures: map[string]analytics.MeasureDefinition{"count": {Type: "count", SQL: "COUNT(*)"}},
			},
		}
	}

	for schema, resource := range owning {
		t.Run(schema+" is refused without read on "+string(resource), func(t *testing.T) {
			// Every grant except read on the owning resource, which is granted
			// write instead.
			permissions := domain.NewFullPermissions()
			permissions[resource] = domain.ResourcePermissions{Read: false, Write: true}

			// No repo expectations: the query must be refused before any read.
			svc, _ := newService(t, permissions)

			_, err := svc.Query(context.Background(), "test-workspace", analytics.Query{
				Schema: schema, Measures: []string{"count"},
			})
			require.Error(t, err)
			assert.IsType(t, &domain.PermissionError{}, err)

			var permErr *domain.PermissionError
			require.ErrorAs(t, err, &permErr)
			assert.Equal(t, resource, permErr.Resource)
			assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
		})

		t.Run(schema+" is served with read on "+string(resource), func(t *testing.T) {
			svc, mockRepo := newService(t, domain.UserPermissions{
				resource: domain.ResourcePermissions{Read: true},
			})
			mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").
				Return(countSchema(schema), nil)
			mockRepo.EXPECT().Query(gomock.Any(), "test-workspace", gomock.Any()).
				Return(&analytics.Response{Data: []map[string]interface{}{{"count": 1}}}, nil)

			response, err := svc.Query(context.Background(), "test-workspace", analytics.Query{
				Schema: schema, Measures: []string{"count"},
			})
			require.NoError(t, err)
			assert.NotNil(t, response)
		})
	}

	t.Run("a schema with no mapping is refused to a member holding every permission", func(t *testing.T) {
		svc, _ := newService(t, domain.NewFullPermissions())

		_, err := svc.Query(context.Background(), "test-workspace", analytics.Query{
			Schema: "schema_added_without_a_mapping", Measures: []string{"count"},
		})
		require.Error(t, err)
		assert.IsType(t, &domain.PermissionError{}, err)
	})

	t.Run("a schema with no mapping is refused to an owner too", func(t *testing.T) {
		// Owners short-circuit HasPermission, so the mapping — not the grant —
		// has to be what refuses here.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
		mockAuth := mocks.NewMockAuthService(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
			Return(context.Background(), &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				WorkspaceID: "test-workspace", Role: "owner",
			}, nil)

		svc := NewAnalyticsService(mockRepo, mockAuth, logger.NewLogger())

		_, err := svc.Query(context.Background(), "test-workspace", analytics.Query{
			Schema: "schema_added_without_a_mapping", Measures: []string{"count"},
		})
		require.Error(t, err)
		assert.IsType(t, &domain.PermissionError{}, err)
	})
}

// The schema catalogue is metadata, so it degrades instead of refusing: it lists
// exactly the schemas the caller could query, which keeps a narrowly scoped key
// with a usable catalogue and keeps an unmapped schema out of everyone's.
func TestAnalyticsService_GetSchemasFiltersOnTheSameMapping(t *testing.T) {
	repoSchemas := map[string]analytics.SchemaDefinition{
		"message_history":                domain.PredefinedSchemas["message_history"],
		"contacts":                       domain.PredefinedSchemas["contacts"],
		"web_sessions":                   {Name: "web_sessions"},
		"schema_added_without_a_mapping": {Name: "schema_added_without_a_mapping"},
	}

	newService := func(t *testing.T, userWorkspace *domain.UserWorkspace) *AnalyticsService {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
		mockAuth := mocks.NewMockAuthService(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "test-workspace").
			Return(context.Background(), &domain.User{ID: "user-123"}, userWorkspace, nil)
		mockRepo.EXPECT().GetSchemas(gomock.Any(), "test-workspace").Return(repoSchemas, nil)

		return NewAnalyticsService(mockRepo, mockAuth, logger.NewLogger())
	}

	t.Run("a member sees only the schemas it can query", func(t *testing.T) {
		svc := newService(t, analyticsMember(domain.PermissionResourceContacts))

		schemas, err := svc.GetSchemas(context.Background(), "test-workspace")
		require.NoError(t, err)
		assert.Contains(t, schemas, "contacts")
		assert.NotContains(t, schemas, "message_history")
		assert.NotContains(t, schemas, "web_sessions")
	})

	t.Run("an unmapped schema is listed to nobody", func(t *testing.T) {
		svc := newService(t, &domain.UserWorkspace{WorkspaceID: "test-workspace", Role: "owner"})

		schemas, err := svc.GetSchemas(context.Background(), "test-workspace")
		require.NoError(t, err)
		assert.Contains(t, schemas, "message_history")
		assert.Contains(t, schemas, "web_sessions")
		assert.NotContains(t, schemas, "schema_added_without_a_mapping")
	})
}
