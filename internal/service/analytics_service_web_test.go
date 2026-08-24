package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/analytics"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// Web analytics rows carry visitor-level data (paths, geo, first-party user
// ids). The console hides the section behind the web_analytics permission, so
// the query endpoint must enforce it too — otherwise the permission is
// cosmetic and any workspace member can read the data through the API.
func TestAnalyticsService_WebSchemaRequiresWebAnalyticsPermission(t *testing.T) {
	newService := func(t *testing.T, permissions domain.UserPermissions) (*AnalyticsService, *mocks.MockAnalyticsRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockRepo := mocks.NewMockAnalyticsRepository(ctrl)
		mockAuth := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		mockAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").
			Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{
				WorkspaceID: "ws1", Permissions: permissions,
			}, nil)

		return NewAnalyticsService(mockRepo, mockAuth, mockLogger), mockRepo
	}

	for _, schema := range []string{"web_sessions", "web_pages", "web_goals"} {
		t.Run(schema+" is refused without the permission", func(t *testing.T) {
			// A member with unrelated permissions only.
			svc, mockRepo := newService(t, domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: true, Write: true},
			})
			// No repo expectations: the query must be rejected before any read.
			_ = mockRepo

			_, err := svc.Query(context.Background(), "ws1", analytics.Query{
				Schema: schema, Measures: []string{"sessions"},
			})
			require.Error(t, err)
			var permErr *domain.PermissionError
			assert.ErrorAs(t, err, &permErr)
		})
	}

	t.Run("non-web schemas are gated on their own resource", func(t *testing.T) {
		svc, mockRepo := newService(t, domain.UserPermissions{
			domain.PermissionResourceMessageHistory: {Read: true},
		})
		mockRepo.EXPECT().GetSchemas(gomock.Any(), "ws1").
			Return(map[string]analytics.SchemaDefinition{
				"message_history": {
					Name:     "message_history",
					Measures: map[string]analytics.MeasureDefinition{"count": {Type: "count", SQL: "COUNT(*)"}},
				},
			}, nil)
		mockRepo.EXPECT().Query(gomock.Any(), "ws1", gomock.Any()).
			Return(&analytics.Response{Data: []map[string]interface{}{{"count": 1}}}, nil)

		_, err := svc.Query(context.Background(), "ws1", analytics.Query{
			Schema: "message_history", Measures: []string{"count"},
		})
		assert.NoError(t, err)
	})

	t.Run("granted read permission allows the query", func(t *testing.T) {
		svc, mockRepo := newService(t, domain.UserPermissions{
			domain.PermissionResourceWebAnalytics: {Read: true},
		})
		mockRepo.EXPECT().GetSchemas(gomock.Any(), "ws1").
			Return(map[string]analytics.SchemaDefinition{
				"web_sessions": {
					Name:     "web_sessions",
					Measures: map[string]analytics.MeasureDefinition{"sessions": {Type: "count", SQL: "COUNT(*)"}},
				},
			}, nil)
		mockRepo.EXPECT().Query(gomock.Any(), "ws1", gomock.Any()).
			Return(&analytics.Response{Data: []map[string]interface{}{{"sessions": 3}}}, nil)

		_, err := svc.Query(context.Background(), "ws1", analytics.Query{
			Schema: "web_sessions", Measures: []string{"sessions"},
		})
		assert.NoError(t, err)
	})
}
