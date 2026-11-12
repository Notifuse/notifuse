package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readableByContactsOnly is the filter for a member who may read contacts and nothing
// else: the entries carrying message or website data are dropped.
var readableByContactsOnly = domain.TimelineEntityTypeFilter{Except: []string{
	"message_history", "inbound_webhook_event", "webhook_event", "web_session", "web_page",
}}

func TestContactTimelineService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	service := NewContactTimelineService(mockRepo, mockAuthService)

	ctx := context.Background()
	workspaceID := "ws123"
	email := "user@example.com"
	limit := 50

	// role "member" (not "owner") so HasPermission actually consults the grants.
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
			UserID:      "user1",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: true},
			},
		}, nil).
		AnyTimes()

	t.Run("Success - List timeline entries", func(t *testing.T) {
		cursor := "cursor123"
		now := time.Now()
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:          "entry1",
				Email:       email,
				Operation:   "insert",
				EntityType:  "contact",
				Kind:        "insert_contact",
				Changes:     nil,
				CreatedAt:   now,
				DBCreatedAt: now,
			},
			{
				ID:         "entry2",
				Email:      email,
				Operation:  "update",
				EntityType: "contact",
				Kind:       "update_contact",
				Changes: map[string]interface{}{
					"first_name": map[string]interface{}{
						"old": "John",
						"new": "Jane",
					},
				},
				CreatedAt:   now.Add(-1 * time.Hour),
				DBCreatedAt: now.Add(-1 * time.Hour),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil), readableByContactsOnly).
			Return(expectedEntries, &cursor, nil)

		entries, nextCursor, err := service.List(ctx, workspaceID, email, limit, nil, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.NotNil(t, nextCursor)
		assert.Equal(t, "cursor123", *nextCursor)
		assert.Equal(t, "entry1", entries[0].ID)
		assert.Equal(t, "insert", entries[0].Operation)
		assert.Equal(t, "entry2", entries[1].ID)
		assert.Equal(t, "update", entries[1].Operation)
	})

	t.Run("Success - List with cursor", func(t *testing.T) {
		cursor := "existing_cursor"
		listID := "list123"
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry3",
				Email:      email,
				Operation:  "update",
				EntityType: "contact_list",
				EntityID:   &listID,
				Changes: map[string]interface{}{
					"status": map[string]interface{}{
						"old": "pending",
						"new": "active",
					},
				},
				CreatedAt: time.Now().Add(-2 * time.Hour),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, &cursor, readableByContactsOnly).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, limit, &cursor, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "entry3", entries[0].ID)
		assert.Equal(t, "contact_list", entries[0].EntityType)
		assert.NotNil(t, entries[0].EntityID)
		assert.Equal(t, "list123", *entries[0].EntityID)
	})

	t.Run("Success - Empty result", func(t *testing.T) {
		expectedEntries := []*domain.ContactTimelineEntry{}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil), readableByContactsOnly).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, limit, nil, nil)

		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("Success - Message history entry", func(t *testing.T) {
		messageID := "msg123"
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry4",
				Email:      email,
				Operation:  "insert",
				EntityType: "message_history",
				EntityID:   &messageID,
				Changes: map[string]interface{}{
					"template_id": map[string]interface{}{
						"new": "tpl123",
					},
					"channel": map[string]interface{}{
						"new": "email",
					},
				},
				CreatedAt: time.Now(),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil), readableByContactsOnly).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, limit, nil, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "message_history", entries[0].EntityType)
		assert.NotNil(t, entries[0].EntityID)
		assert.Equal(t, "msg123", *entries[0].EntityID)
	})

	t.Run("Error - Repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil), readableByContactsOnly).
			Return(nil, nil, assert.AnError)

		entries, nextCursor, err := service.List(ctx, workspaceID, email, limit, nil, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
	})

	t.Run("Success - With different limit", func(t *testing.T) {
		customLimit := 10
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry5",
				Email:      email,
				Operation:  "delete",
				EntityType: "contact",
				CreatedAt:  time.Now(),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, customLimit, (*string)(nil), readableByContactsOnly).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, customLimit, nil, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "delete", entries[0].Operation)
	})
}

func TestNewContactTimelineService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	service := NewContactTimelineService(mockRepo, mockAuthService)

	assert.NotNil(t, service)
	assert.IsType(t, &ContactTimelineService{}, service)
}

// TestContactTimelineService_PermissionEnforcement verifies that List enforces
// contacts:read. The member is granted the OPPOSITE permission (write, not read),
// so the test fails both if the check is missing AND if it is gated on the wrong
// permission type. No repo expectation is set, so gomock also fails if anything
// beyond the permission gate runs.
//
// The refusal has to name contacts:read specifically. The fixture grants a single
// resource, so a gate on any OTHER resource denies this member too and the error
// type alone cannot tell them apart — and the resource and verb are client-visible,
// since writePermissionError puts both on the wire.
func TestContactTimelineService_PermissionEnforcement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	service := NewContactTimelineService(mockRepo, mockAuthService)

	ctx := context.Background()

	// role "member" (not "owner") so HasPermission actually consults the grants.
	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(ctx, "ws123").
		Return(ctx, &domain.User{ID: "user1"}, &domain.UserWorkspace{
			UserID:      "user1",
			WorkspaceID: "ws123",
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Write: true},
			},
		}, nil)

	entries, nextCursor, err := service.List(ctx, "ws123", "user@example.com", 50, nil, nil)

	require.Error(t, err)

	var permErr *domain.PermissionError
	require.True(t, errors.As(err, &permErr), "expected a *domain.PermissionError, got %T: %v", err, err)
	assert.Equal(t, domain.PermissionResourceContacts, permErr.Resource)
	assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
	assert.Nil(t, entries)
	assert.Nil(t, nextCursor)
}

func TestContactTimelineService_List_AuthenticationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	service := NewContactTimelineService(mockRepo, mockAuthService)

	ctx := context.Background()

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(ctx, "ws123").
		Return(ctx, nil, nil, assert.AnError)

	entries, nextCursor, err := service.List(ctx, "ws123", "user@example.com", 50, nil, nil)

	assert.Error(t, err)
	assert.Nil(t, entries)
	assert.Nil(t, nextCursor)
}

func TestContactTimelineService_List_EntityTypes(t *testing.T) {
	ctx := context.Background()
	full := domain.UserPermissions{
		domain.PermissionResourceContacts:       {Read: true},
		domain.PermissionResourceMessageHistory: {Read: true},
		domain.PermissionResourceWebAnalytics:   {Read: true},
	}
	without := func(resource domain.PermissionResource) domain.UserPermissions {
		perms := domain.UserPermissions{}
		for r, p := range full {
			if r != resource {
				perms[r] = p
			}
		}
		return perms
	}
	setup := func(t *testing.T, perms domain.UserPermissions) (*ContactTimelineService, *mocks.MockContactTimelineRepository) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockContactTimelineRepository(ctrl)
		auth := mocks.NewMockAuthService(ctrl)
		auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws").
			Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws", Role: "member", Permissions: perms}, nil)
		return NewContactTimelineService(repo, auth), repo
	}
	expect := func(repo *mocks.MockContactTimelineRepository, filter domain.TimelineEntityTypeFilter) {
		repo.EXPECT().List(ctx, "ws", "a@example.com", 20, (*string)(nil), filter).Return(nil, nil, nil)
	}

	t.Run("full permissions and no type: no filter", func(t *testing.T) {
		svc, repo := setup(t, full)
		expect(repo, domain.TimelineEntityTypeFilter{})
		_, _, err := svc.List(ctx, "ws", "a@example.com", 20, nil, nil)
		require.NoError(t, err)
	})

	t.Run("full permissions keep the requested types", func(t *testing.T) {
		svc, repo := setup(t, full)
		expect(repo, domain.TimelineEntityTypeFilter{Only: []string{"web_page", "message_history"}})
		_, _, err := svc.List(ctx, "ws", "a@example.com", 20, nil, []string{"web_page", "message_history"})
		require.NoError(t, err)
	})

	t.Run("without message_history.read and no type: message and provider entries denied", func(t *testing.T) {
		svc, repo := setup(t, without(domain.PermissionResourceMessageHistory))
		expect(repo, domain.TimelineEntityTypeFilter{Except: []string{"message_history", "inbound_webhook_event", "webhook_event"}})
		_, _, err := svc.List(ctx, "ws", "a@example.com", 20, nil, nil)
		require.NoError(t, err)
	})

	t.Run("without message_history.read, requested message entries removed", func(t *testing.T) {
		svc, repo := setup(t, without(domain.PermissionResourceMessageHistory))
		expect(repo, domain.TimelineEntityTypeFilter{Only: []string{"contact"}})
		_, _, err := svc.List(ctx, "ws", "a@example.com", 20, nil, []string{"message_history", "contact", "inbound_webhook_event"})
		require.NoError(t, err)
	})

	t.Run("without web_analytics.read, website entries removed", func(t *testing.T) {
		svc, repo := setup(t, without(domain.PermissionResourceWebAnalytics))
		expect(repo, domain.TimelineEntityTypeFilter{Except: []string{"web_session", "web_page"}})
		_, _, err := svc.List(ctx, "ws", "a@example.com", 20, nil, nil)
		require.NoError(t, err)
	})

	t.Run("every requested type removed: empty list, no cursor, repository not called", func(t *testing.T) {
		svc, _ := setup(t, without(domain.PermissionResourceWebAnalytics))
		entries, cursor, err := svc.List(ctx, "ws", "a@example.com", 20, nil, []string{"web_session", "web_page"})
		require.NoError(t, err)
		assert.NotNil(t, entries)
		assert.Empty(t, entries)
		assert.Nil(t, cursor)
	})

	t.Run("without contacts.read still refused, whatever the types", func(t *testing.T) {
		svc, _ := setup(t, without(domain.PermissionResourceContacts))
		_, _, err := svc.List(ctx, "ws", "a@example.com", 20, nil, []string{"contact"})
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
		assert.Equal(t, domain.PermissionResourceContacts, permErr.Resource)
	})
}
