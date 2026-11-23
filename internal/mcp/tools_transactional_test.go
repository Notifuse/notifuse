package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

func transactionalReadPermission() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceTransactional: {Read: true}}
}

var notificationsEpoch = time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)

// threeNotifications are returned as transactional.list does, created_at
// alone deciding the order: "b" and "c" share a creation time.
func threeNotifications() []*domain.TransactionalNotification {
	return []*domain.TransactionalNotification{
		{ID: "c", Name: "C", CreatedAt: notificationsEpoch.Add(time.Hour)},
		{ID: "b", Name: "B", CreatedAt: notificationsEpoch.Add(time.Hour)},
		{ID: "a", Name: "A", CreatedAt: notificationsEpoch},
	}
}

func fullNotification() *domain.TransactionalNotification {
	return &domain.TransactionalNotification{
		ID: "password_reset", Name: "Password reset", Description: "Sent from the app",
		Channels: domain.ChannelTemplates{domain.TransactionalChannelEmail: {TemplateID: "tpl_reset", Settings: domain.MapOfAny{"unused": "setting"}}},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: true, UTMSource: "app", Endpoint: "https://track.example.com", WorkspaceID: "ws-internal",
			MessageID: "msg-internal", IdentifyToken: "bearer-token",
		},
		Metadata:      domain.MapOfAny{"team": "auth"},
		IntegrationID: strPtr("supabase-integration-id"),
		CreatedAt:     notificationsEpoch, UpdatedAt: notificationsEpoch.Add(time.Hour),
	}
}

func TestTransactionalTools_Definitions(t *testing.T) {
	transactional := []Pair{{Resource: "transactional", Verb: "read"}}
	assertReadTool(t, catalogueTool(t, "list_transactional_notifications"), "list_transactional_notifications", transactional, []string{"workspace_id"})
	assertReadTool(t, catalogueTool(t, "get_transactional_notification"), "get_transactional_notification", transactional, []string{"workspace_id", "notification_id"})
}

func TestListTransactionalNotifications_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(transactionalReadPermission())
	f.notifs.EXPECT().ListNotifications(gomock.Any(), testWorkspaceID, map[string]interface{}{"search": "reset"}, 0, 0).
		Return([]*domain.TransactionalNotification{fullNotification()}, 1, nil)

	result := f.callTool("list_transactional_notifications", map[string]any{"workspace_id": testWorkspaceID, "search": " reset "})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"notifications":[{
			"id":"password_reset","name":"Password reset","description":"Sent from the app",
			"template_ids":{"email":"tpl_reset"},"managed_by_integration":true,
			"created_at":"2026-10-05T08:00:00Z","updated_at":"2026-10-05T09:00:00Z"
		}],
		"total":1
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"supabase-integration-id", "tracking", "metadata", "unused"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// transactional.list orders by created_at alone; the tool reads every
// notification and pages them by offset, newest first with the ID breaking
// ties.
func TestListTransactionalNotifications_PagesOnAStableOrder(t *testing.T) {
	f := newFixture(t)
	f.reachable(transactionalReadPermission())
	f.notifs.EXPECT().ListNotifications(gomock.Any(), testWorkspaceID, nil, 0, 0).
		DoAndReturn(func(any, string, map[string]interface{}, int, int) ([]*domain.TransactionalNotification, int, error) {
			return threeNotifications(), 3, nil
		}).Times(2)

	type page struct {
		Notifications []struct {
			ID string `json:"id"`
		} `json:"notifications"`
		Total      int    `json:"total"`
		NextCursor string `json:"next_cursor"`
	}
	var first page
	f.callTool("list_transactional_notifications", map[string]any{"workspace_id": testWorkspaceID, "limit": 2}).decode(t, &first)
	require.Len(t, first.Notifications, 2)
	assert.Equal(t, "b", first.Notifications[0].ID)
	assert.Equal(t, "c", first.Notifications[1].ID)
	assert.Equal(t, 3, first.Total)
	require.NotEmpty(t, first.NextCursor)

	var second page
	f.callTool("list_transactional_notifications", map[string]any{"workspace_id": testWorkspaceID, "limit": 2, "cursor": first.NextCursor}).decode(t, &second)
	require.Len(t, second.Notifications, 1)
	assert.Equal(t, "a", second.Notifications[0].ID)
	assert.Empty(t, second.NextCursor)

	toolErr := f.callTool("list_transactional_notifications", map[string]any{"workspace_id": testWorkspaceID, "search": "x", "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to its search")
}

func TestListTransactionalNotifications_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(transactionalReadPermission())
	negative, err := EncodeCursor("list_transactional_notifications", testWorkspaceID, map[string]any{}, -1)
	require.NoError(t, err)
	for name, args := range map[string]map[string]any{
		"limit of 0":         {"limit": 0},
		"garbage cursor":     {"cursor": "%%%"},
		"negative offset":    {"cursor": negative},
		"an offset argument": {"offset": 3},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_transactional_notifications", args).errorOf(t).Code)
		})
	}
}

func TestGetTransactionalNotification_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(transactionalReadPermission())
	f.notifs.EXPECT().GetNotification(gomock.Any(), testWorkspaceID, "password_reset").Return(fullNotification(), nil)

	result := f.callTool("get_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "password_reset"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"notification":{
			"id":"password_reset","name":"Password reset","description":"Sent from the app",
			"channels":{"email":{"template_id":"tpl_reset"}},
			"tracking":{"enable_tracking":true,"tracking_mode":"inherit","utm_source":"app","utm_medium":"","utm_campaign":"","utm_content":"","utm_term":""},
			"metadata":{"team":"auth"},
			"managed_by_integration":true,
			"created_at":"2026-10-05T08:00:00Z","updated_at":"2026-10-05T09:00:00Z"
		}
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"supabase-integration-id", "track.example.com", "ws-internal", "msg-internal", "bearer-token", "unused", "deleted_at"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestGetTransactionalNotification_CapsMetadata(t *testing.T) {
	f := newFixture(t)
	f.reachable(transactionalReadPermission())
	notification := fullNotification()
	notification.Metadata = domain.MapOfAny{"blob": strings.Repeat("m", 2001)}
	f.notifs.EXPECT().GetNotification(gomock.Any(), testWorkspaceID, "password_reset").Return(notification, nil)

	var out struct {
		Notification struct {
			Metadata        json.RawMessage `json:"metadata"`
			TruncatedFields []string        `json:"truncated_fields"`
		} `json:"notification"`
	}
	f.callTool("get_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "password_reset"}).decode(t, &out)
	assert.Equal(t, "null", string(out.Notification.Metadata))
	assert.Equal(t, []string{"metadata"}, out.Notification.TruncatedFields)
}

func TestGetTransactionalNotification_Errors(t *testing.T) {
	t.Run("an unknown or deleted notification", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalReadPermission())
		f.notifs.EXPECT().GetNotification(gomock.Any(), testWorkspaceID, "gone").
			Return(nil, fmt.Errorf("failed to get notification: %w", &domain.ErrNotFound{Entity: "transactional_notification", ID: "gone"}))
		toolErr := f.callTool("get_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "gone"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "transactional_notification not found with ID: gone"}, toolErr)
	})

	t.Run("a notification that comes back deleted", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalReadPermission())
		deleted := fullNotification()
		deleted.DeletedAt = &notificationsEpoch
		f.notifs.EXPECT().GetNotification(gomock.Any(), testWorkspaceID, "password_reset").Return(deleted, nil)
		toolErr := f.callTool("get_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "password_reset"}).errorOf(t)
		assert.Equal(t, CodeNotFound, toolErr.Code)
	})

	t.Run("a key without transactional:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceTransactional: {Write: true}})
		toolErr := f.callTool("get_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "x"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the transactional read permission", Resource: "transactional", Permission: "read"}, toolErr)
	})

	t.Run("a failed list", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalReadPermission())
		f.notifs.EXPECT().ListNotifications(gomock.Any(), testWorkspaceID, nil, 0, 0).Return(nil, 0, errors.New("pq: db-host-7"))
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"},
			f.callTool("list_transactional_notifications", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
	})
}

// A notification's name, description and UTM parameters are free text, capped
// like the rest.
func TestTransactionalTools_CapTheNameDescriptionAndUTMParameters(t *testing.T) {
	long, capped := strings.Repeat("n", FreeTextLimit+1), strings.Repeat("n", FreeTextLimit)
	longNotification := func() *domain.TransactionalNotification {
		notification := fullNotification()
		notification.Name, notification.Description = long, long
		tracking := &notification.TrackingSettings
		tracking.UTMSource, tracking.UTMMedium, tracking.UTMCampaign, tracking.UTMContent, tracking.UTMTerm = long, long, long, long, long
		return notification
	}

	t.Run("list_transactional_notifications", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalReadPermission())
		f.notifs.EXPECT().ListNotifications(gomock.Any(), testWorkspaceID, nil, 0, 0).
			Return([]*domain.TransactionalNotification{longNotification()}, 1, nil)
		var out struct {
			Notifications []struct {
				Name            string   `json:"name"`
				Description     string   `json:"description"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"notifications"`
		}
		f.callTool("list_transactional_notifications", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
		require.Len(t, out.Notifications, 1)
		assert.Equal(t, capped, out.Notifications[0].Name)
		assert.Equal(t, capped, out.Notifications[0].Description)
		assert.Equal(t, []string{"name", "description"}, out.Notifications[0].TruncatedFields)
	})

	t.Run("get_transactional_notification", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalReadPermission())
		f.notifs.EXPECT().GetNotification(gomock.Any(), testWorkspaceID, "password_reset").Return(longNotification(), nil)
		var out struct {
			Notification struct {
				Name            string         `json:"name"`
				Description     string         `json:"description"`
				Tracking        map[string]any `json:"tracking"`
				TruncatedFields []string       `json:"truncated_fields"`
			} `json:"notification"`
		}
		f.callTool("get_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "password_reset"}).decode(t, &out)
		assert.Equal(t, capped, out.Notification.Name)
		assert.Equal(t, capped, out.Notification.Description)
		utm := []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term"}
		for _, key := range utm {
			assert.Equal(t, capped, out.Notification.Tracking[key], key)
		}
		want := []string{"name", "description"}
		for _, key := range utm {
			want = append(want, "tracking."+key)
		}
		assert.Equal(t, want, out.Notification.TruncatedFields)
	})
}
