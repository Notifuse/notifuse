package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

func transactionalWritePermissions(extra ...domain.PermissionResource) domain.UserPermissions {
	permissions := domain.UserPermissions{domain.PermissionResourceTransactional: {Write: true}}
	for _, resource := range extra {
		permissions[resource] = domain.ResourcePermissions{Read: true}
	}
	return permissions
}

func transactionalWriteFixture(t *testing.T, permissions domain.UserPermissions) (*fixture, *auditLog) {
	f := newFixture(t)
	f.reachableIn(permissions, sendingWorkspace())
	return f, f.captureAudit()
}

// storedNotification is a notification an application sends, with tracking
// settings and channel settings that no argument names.
func storedNotification() *domain.TransactionalNotification {
	return &domain.TransactionalNotification{
		ID: "order_confirmation", Name: "Order confirmation", Description: "After checkout",
		Channels: domain.ChannelTemplates{domain.TransactionalChannelEmail: {TemplateID: "tpl_order", Settings: domain.MapOfAny{"kept": true}}},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: true, TrackingMode: notifuse_mjml.TrackingModeNoTracking, UTMSource: "shop", UTMCampaign: "orders",
		},
		Metadata:  domain.MapOfAny{"team": "shop"},
		CreatedAt: notificationsEpoch, UpdatedAt: notificationsEpoch,
	}
}

func TestTransactionalWriteTools_Definitions(t *testing.T) {
	write := Pair{Resource: "transactional", Verb: "write"}
	templates := Pair{Resource: "templates", Verb: "read"}
	contacts := Pair{Resource: "contacts", Verb: "read"}
	lists := Pair{Resource: "lists", Verb: "read"}

	assertWriteToolDefinition(t, catalogueTool(t, "create_transactional_notification"), "create_transactional_notification", ClassWrite,
		writeHints{destructive: false, openWorld: false, idempotent: false}, []Pair{write, templates}, "transactional.create",
		[]string{"workspace_id", "notification_id", "name", "email_template_id"})
	assertWriteToolDefinition(t, catalogueTool(t, "update_transactional_notification"), "update_transactional_notification", ClassWriteOpenWorld,
		writeHints{destructive: false, openWorld: true, idempotent: true}, []Pair{write}, "transactional.update", []string{"workspace_id", "notification_id"})
	assertWriteToolDefinition(t, catalogueTool(t, "delete_transactional_notification"), "delete_transactional_notification", ClassDestructive,
		writeHints{destructive: true, openWorld: false, idempotent: true}, []Pair{write}, "transactional.delete", []string{"workspace_id", "notification_id"})
	assertWriteToolDefinition(t, catalogueTool(t, "send_transactional"), "send_transactional", ClassSend,
		writeHints{destructive: true, openWorld: true, idempotent: true}, []Pair{write}, "transactional.send",
		[]string{"workspace_id", "notification_id", "external_id", "email"})
	assertWriteToolDefinition(t, catalogueTool(t, "send_template_test"), "send_template_test", ClassSend,
		writeHints{destructive: true, openWorld: true, idempotent: false}, []Pair{write, templates}, "transactional.testTemplate",
		[]string{"workspace_id", "template_id", "recipient_email"})

	for _, name := range []string{"create_transactional_notification", "delete_transactional_notification"} {
		assert.Nil(t, catalogueTool(t, name).ArgumentPairs, name)
	}
	update := catalogueTool(t, "update_transactional_notification")
	assert.Empty(t, update.requiredPairs(map[string]any{"name": "X"})[1:])
	assert.Equal(t, []Pair{write, templates}, update.requiredPairs(map[string]any{"email_template_id": "tpl"}))

	send := catalogueTool(t, "send_transactional")
	assert.Equal(t, []Pair{write}, send.requiredPairs(map[string]any{"cc": []any{}}))
	assert.Equal(t, []Pair{write, contacts}, send.requiredPairs(map[string]any{"cc": []any{"a@example.com"}}))
	assert.Equal(t, []Pair{write, contacts}, send.requiredPairs(map[string]any{"bcc": []any{"a@example.com"}}))

	test := catalogueTool(t, "send_template_test")
	assert.Equal(t, []Pair{write, templates}, test.requiredPairs(map[string]any{}))
	assert.Equal(t, []Pair{write, templates, lists}, test.requiredPairs(map[string]any{"list_id": "news"}))
	assert.Equal(t, []Pair{write, templates, contacts, lists}, test.requiredPairs(map[string]any{"list_id": "news", "bcc": []any{"a@example.com"}}))

	// Profile fields, attachments and a chosen sender are not offered.
	properties := send.InputSchema["properties"].(map[string]any)
	assert.Equal(t, []string{"bcc", "cc", "data", "email", "external_id", "from_name", "metadata", "notification_id", "preheader", "reply_to", "subject", "workspace_id"},
		sortedKeys(properties))
}

func TestCreateTransactionalNotification_BuildsTheRequest(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates))
	f.notifs.EXPECT().CreateNotification(gomock.Any(), testWorkspaceID, domain.TransactionalNotificationCreateParams{
		ID: "order_confirmation", Name: "Order confirmation", Description: "After checkout",
		Channels:         domain.ChannelTemplates{domain.TransactionalChannelEmail: {TemplateID: "tpl_order"}},
		TrackingSettings: notifuse_mjml.TrackingSettings{TrackingMode: "disabled", UTMSource: "shop"},
		Metadata:         domain.MapOfAny{"team": "shop"},
	}).Return(storedNotification(), nil)

	result := f.callTool("create_transactional_notification", map[string]any{
		"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "name": "Order confirmation", "description": "After checkout",
		"email_template_id": "tpl_order", "tracking_mode": "disabled", "utm_parameters": map[string]any{"source": "shop"}, "metadata": map[string]any{"team": "shop"},
	})
	var out struct {
		Notification notificationDetail `json:"notification"`
	}
	result.decode(t, &out)
	assert.Equal(t, "order_confirmation", out.Notification.ID)

	row := audit.last(t)
	assert.Equal(t, "transactional.create", row.Action)
	assert.Equal(t, domain.AuditTargetNotification, row.TargetType)
	assert.Equal(t, "order_confirmation", row.TargetID)
	assert.Equal(t, "Order confirmation", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

func TestCreateTransactionalNotification_Refusals(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates))
	args := func(id string) map[string]any {
		return map[string]any{"workspace_id": testWorkspaceID, "notification_id": id, "name": "N", "email_template_id": "tpl"}
	}
	// A replay meets its own ID.
	f.notifs.EXPECT().CreateNotification(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(nil, fmt.Errorf("failed to create notification: %w", domain.NewAlreadyExistsError("transactional_notification", "taken", false)))
	assert.Equal(t, ToolError{Code: CodeConflict, Message: "transactional_notification taken already exists"}, f.callTool("create_transactional_notification", args("taken")).errorOf(t))
	assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)

	f.notifs.EXPECT().CreateNotification(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(nil, fmt.Errorf("invalid template for channel email: %w", &domain.ErrTemplateNotFound{Message: "template not found"}))
	assert.Equal(t, CodeNotFound, f.callTool("create_transactional_notification", args("fresh")).errorOf(t).Code)

	// The handler's validation runs before the service.
	assert.Equal(t, CodeValidation, f.callTool("create_transactional_notification", args(strings.Repeat("x", 33))).errorOf(t).Code)

	g, _ := transactionalWriteFixture(t, transactionalWritePermissions())
	toolErr := g.callTool("create_transactional_notification", args("fresh")).errorOf(t)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
	assert.Equal(t, "templates", toolErr.Resource)
}

// update_transactional_notification sends only the keys it was given: the
// service keeps the name, the description, the channels, the metadata and
// the tracking settings left out.
func TestUpdateTransactionalNotification_KeepsWhatItIsNotGiven(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions())
	f.notifRows.EXPECT().Get(gomock.Any(), testWorkspaceID, "order_confirmation").Return(storedNotification(), nil)
	var sent domain.TransactionalNotificationUpdateParams
	f.notifs.EXPECT().UpdateNotification(gomock.Any(), testWorkspaceID, "order_confirmation", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, params domain.TransactionalNotificationUpdateParams) (*domain.TransactionalNotification, error) {
			sent = params
			return storedNotification(), nil
		})

	result := f.callTool("update_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "description": ""})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.Equal(t, strPtr(""), sent.Description, "an empty description clears it")
	assert.Empty(t, sent.Name)
	assert.Nil(t, sent.Channels)
	assert.Nil(t, sent.Metadata)
	assert.False(t, sent.TrackingSettingsSpecified(), "tracking settings left out are kept")

	row := audit.last(t)
	assert.Equal(t, "transactional.update", row.Action)
	assert.Equal(t, "order_confirmation", row.TargetID)
	assert.Equal(t, "Order confirmation", row.TargetName)
}

func TestUpdateTransactionalNotification_KeepsTheHalfOfTheTrackingNotGiven(t *testing.T) {
	stored := storedNotification().TrackingSettings
	for name, tc := range map[string]struct {
		args map[string]any
		want notifuse_mjml.TrackingSettings
	}{
		"tracking_mode alone keeps the UTM parameters": {
			args: map[string]any{"tracking_mode": "inherit"},
			want: notifuse_mjml.TrackingSettings{EnableTracking: true, TrackingMode: "inherit", UTMSource: stored.UTMSource, UTMCampaign: stored.UTMCampaign},
		},
		"utm_parameters alone keeps the mode and replaces every UTM parameter": {
			args: map[string]any{"utm_parameters": map[string]any{"medium": "email"}},
			want: notifuse_mjml.TrackingSettings{EnableTracking: true, TrackingMode: "no_tracking", UTMMedium: "email"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := transactionalWriteFixture(t, transactionalWritePermissions())
			f.notifRows.EXPECT().Get(gomock.Any(), testWorkspaceID, "order_confirmation").Return(storedNotification(), nil)
			var sent domain.TransactionalNotificationUpdateParams
			f.notifs.EXPECT().UpdateNotification(gomock.Any(), testWorkspaceID, "order_confirmation", gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, params domain.TransactionalNotificationUpdateParams) (*domain.TransactionalNotification, error) {
					sent = params
					return storedNotification(), nil
				})
			tc.args["workspace_id"], tc.args["notification_id"] = testWorkspaceID, "order_confirmation"
			require.False(t, f.callTool("update_transactional_notification", tc.args).IsError)
			assert.True(t, sent.TrackingSettingsSpecified())
			assert.Equal(t, tc.want, sent.TrackingSettings)
		})
	}
}

func TestUpdateTransactionalNotification_ANewTemplateNeedsTemplatesRead(t *testing.T) {
	args := map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "email_template_id": "tpl_new"}

	f, audit := transactionalWriteFixture(t, transactionalWritePermissions())
	toolErr := f.callTool("update_transactional_notification", args).errorOf(t)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
	assert.Equal(t, "templates", toolErr.Resource)
	assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)

	g, _ := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates))
	g.notifRows.EXPECT().Get(gomock.Any(), testWorkspaceID, "order_confirmation").Return(storedNotification(), nil)
	g.notifs.EXPECT().UpdateNotification(gomock.Any(), testWorkspaceID, "order_confirmation", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, params domain.TransactionalNotificationUpdateParams) (*domain.TransactionalNotification, error) {
			assert.Equal(t, domain.ChannelTemplates{domain.TransactionalChannelEmail: {TemplateID: "tpl_new", Settings: domain.MapOfAny{"kept": true}}}, params.Channels)
			assert.False(t, params.TrackingSettingsSpecified())
			return storedNotification(), nil
		})
	require.False(t, g.callTool("update_transactional_notification", args).IsError)
}

func TestUpdateTransactionalNotification_Refusals(t *testing.T) {
	f, _ := transactionalWriteFixture(t, transactionalWritePermissions())
	assert.Equal(t, CodeValidation, f.callTool("update_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "x", "name": ""}).errorOf(t).Code)

	f.notifRows.EXPECT().Get(gomock.Any(), testWorkspaceID, "gone").Return(nil, &domain.ErrNotFound{Entity: "transactional_notification", ID: "gone"})
	assert.Equal(t, CodeNotFound, f.callTool("update_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "gone", "name": "X"}).errorOf(t).Code)

	f.notifRows.EXPECT().Get(gomock.Any(), testWorkspaceID, "order_confirmation").Return(storedNotification(), nil)
	toolErr := f.callTool("update_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeValidation, Message: "at least one field must be updated"}, toolErr)

	f.notifRows.EXPECT().Get(gomock.Any(), testWorkspaceID, "order_confirmation").Return(storedNotification(), nil)
	f.notifs.EXPECT().UpdateNotification(gomock.Any(), testWorkspaceID, "order_confirmation", gomock.Any()).
		Return(nil, domain.NewValidationError("cannot update name of integration-managed notification"))
	assert.Equal(t, CodeValidation, f.callTool("update_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "name": "X"}).errorOf(t).Code)
}

func TestDeleteTransactionalNotification(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions())
	f.notifs.EXPECT().DeleteNotification(gomock.Any(), testWorkspaceID, "order_confirmation").Return(nil)
	result := f.callTool("delete_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation"})
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"deleted":true,"notification_id":"order_confirmation"}`, string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "transactional.delete", row.Action)
	assert.Equal(t, domain.AuditTargetNotification, row.TargetType)
	assert.Equal(t, "order_confirmation", row.TargetID)

	// The service reads the notification first: a replay answers not_found.
	f.notifs.EXPECT().DeleteNotification(gomock.Any(), testWorkspaceID, "order_confirmation").
		Return(fmt.Errorf("failed to get notification: %w", &domain.ErrNotFound{Entity: "transactional_notification", ID: "order_confirmation"}))
	assert.Equal(t, CodeNotFound, f.callTool("delete_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation"}).errorOf(t).Code)
	assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)

	f.notifs.EXPECT().DeleteNotification(gomock.Any(), testWorkspaceID, "supabase_signup").
		Return(domain.NewValidationError("cannot delete integration-managed notification"))
	assert.Equal(t, CodeValidation, f.callTool("delete_transactional_notification", map[string]any{"workspace_id": testWorkspaceID, "notification_id": "supabase_signup"}).errorOf(t).Code)
}

func TestSendTransactional_BuildsTheRequest(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceContacts))
	externalID := "order-981"
	subject, preheader, fromName := "Your order", "It ships today", "Acme shop"
	f.notifs.EXPECT().SendNotification(gomock.Any(), testWorkspaceID, domain.TransactionalNotificationSendParams{
		ID: "order_confirmation", ExternalID: &externalID, Contact: &domain.Contact{Email: "jane@example.com"},
		Channels: []domain.TransactionalChannel{domain.TransactionalChannelEmail},
		Data:     domain.MapOfAny{"order": map[string]any{"number": "981"}},
		Metadata: domain.MapOfAny{"source": "assistant"},
		EmailOptions: domain.EmailOptions{
			Subject: &subject, SubjectPreview: &preheader, FromName: &fromName, ReplyTo: "help@acme.test",
			CC: []string{"boss@example.com"}, BCC: []string{"audit@example.com"},
		},
	}).Return("msg-1", nil)

	result := f.callTool("send_transactional", map[string]any{
		"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "external_id": externalID, "email": " Jane@Example.com",
		"data": map[string]any{"order": map[string]any{"number": "981"}}, "metadata": map[string]any{"source": "assistant"},
		"subject": subject, "preheader": preheader, "from_name": fromName, "reply_to": "help@acme.test",
		"cc": []string{"Boss@Example.com"}, "bcc": []string{"audit@example.com"},
	})
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"message_id":"msg-1","notification_id":"order_confirmation","external_id":"order-981"}`, string(result.StructuredContent))

	row := audit.last(t)
	assert.Equal(t, "transactional.send", row.Action)
	assert.Equal(t, domain.AuditCategoryTransactional, row.Category)
	assert.Equal(t, domain.AuditTargetNotification, row.TargetType)
	assert.Equal(t, "order_confirmation", row.TargetID)
	assert.Equal(t, http.StatusOK, row.StatusCode)
	assert.Equal(t, map[string]any{
		"channel": "mcp", "mcp_tool": "send_transactional", "notification_id": "order_confirmation", "recipient_email": "jane@example.com",
		"cc": []string{"boss@example.com"}, "bcc": []string{"audit@example.com"}, "external_id": "order-981", "message_id": "msg-1",
	}, row.Metadata)
}

// The service answers a replayed key with the first message's ID and sends
// nothing: the tool returns that ID as a success.
func TestSendTransactional_AReplayAnswersTheFirstMessage(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions())
	f.notifs.EXPECT().SendNotification(gomock.Any(), testWorkspaceID, gomock.Any()).Return("msg-first", nil).Times(2)
	args := map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "external_id": "order-981", "email": "jane@example.com"}
	for range 2 {
		var out sendTransactionalOutput
		f.callTool("send_transactional", args).decode(t, &out)
		assert.Equal(t, "msg-first", out.MessageID)
	}
	assert.Equal(t, 2, audit.count())
}

func TestSendTransactional_Refusals(t *testing.T) {
	base := func(extra map[string]any) map[string]any {
		args := map[string]any{"workspace_id": testWorkspaceID, "notification_id": "order_confirmation", "external_id": "k1", "email": "jane@example.com"}
		for key, value := range extra {
			args[key] = value
		}
		return args
	}
	t.Run("cc without contacts:read", func(t *testing.T) {
		f, audit := transactionalWriteFixture(t, transactionalWritePermissions())
		toolErr := f.callTool("send_transactional", base(map[string]any{"cc": []string{"boss@example.com"}})).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the contacts read permission", Resource: "contacts", Permission: "read"}, toolErr)
		row := audit.last(t)
		assert.Equal(t, http.StatusForbidden, row.StatusCode)
		assert.Equal(t, "transactional.send", row.Action)
	})
	t.Run("arguments", func(t *testing.T) {
		f, _ := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceContacts))
		for name, args := range map[string]map[string]any{
			"an empty key":        base(map[string]any{"external_id": ""}),
			"a key too long":      base(map[string]any{"external_id": strings.Repeat("k", 256)}),
			"a bad address":       base(map[string]any{"email": "jane"}),
			"a bad cc":            base(map[string]any{"cc": []string{"boss"}}),
			"a bad reply-to":      base(map[string]any{"reply_to": "help"}),
			"a subject too long":  base(map[string]any{"subject": strings.Repeat("s", 256)}),
			"data above 32 KiB":   base(map[string]any{"data": map[string]any{"blob": strings.Repeat("x", 33<<10)}}),
			"contact fields":      base(map[string]any{"first_name": "Jane"}),
			"attachments":         base(map[string]any{"attachments": []any{}}),
			"a sender of its own": base(map[string]any{"sender_id": "t-billing"}),
		} {
			assert.Equal(t, CodeValidation, f.callTool("send_transactional", args).errorOf(t).Code, name)
		}
	})
	t.Run("no transactional integration", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalWritePermissions())
		f.captureAudit()
		toolErr := f.callTool("send_transactional", base(nil)).errorOf(t)
		assert.Equal(t, CodeValidation, toolErr.Code)
		assert.Contains(t, toolErr.Message, "transactional email integration")
	})
	t.Run("service errors", func(t *testing.T) {
		f, _ := transactionalWriteFixture(t, transactionalWritePermissions())
		f.notifs.EXPECT().SendNotification(gomock.Any(), testWorkspaceID, gomock.Any()).
			Return("", fmt.Errorf("failed to get notification: %w", &domain.ErrNotFound{Entity: "transactional_notification", ID: "order_confirmation"}))
		assert.Equal(t, CodeNotFound, f.callTool("send_transactional", base(nil)).errorOf(t).Code)
		f.notifs.EXPECT().SendNotification(gomock.Any(), testWorkspaceID, gomock.Any()).
			Return("", fmt.Errorf("failed to send notification through any channel: %w", &domain.TemplateRenderError{Message: "liquid: unknown filter money"}))
		assert.Equal(t, ToolError{Code: CodeValidation, Message: "liquid: unknown filter money"}, f.callTool("send_transactional", base(nil)).errorOf(t))
		f.notifs.EXPECT().SendNotification(gomock.Any(), testWorkspaceID, gomock.Any()).Return("", errors.New("failed to send notification through any channel"))
		assert.Equal(t, CodeInternal, f.callTool("send_transactional", base(nil)).errorOf(t).Code)
	})
}

func templateWithSender(senderID string) *domain.Template {
	return &domain.Template{
		ID: "tpl_welcome", Name: "Welcome", Channel: domain.ChannelEmail,
		Email: &domain.EmailTemplate{Subject: "Hi", SenderID: senderID},
		Translations: map[string]domain.TemplateTranslation{
			"fr": {Email: &domain.EmailTemplate{Subject: "Salut", SenderID: "t-billing"}},
		},
	}
}

func TestSendTemplateTest_PicksTheIntegrationAndTheSender(t *testing.T) {
	marketingOnly := sendingWorkspace()
	marketingOnly.Settings.TransactionalEmailProviderID = ""
	for name, tc := range map[string]struct {
		workspace      *domain.Workspace
		templateSender string
		language       string
		integration    string
		sender         string
		from           string
	}{
		"the transactional integration and the template's sender": {sendingWorkspace(), "t-billing", "", "int-t", "t-billing", "billing@acme.test"},
		"a template sender the integration lacks":                 {sendingWorkspace(), "s-support", "", "int-t", "t-default", "no-reply@acme.test"},
		"no template sender":                                      {sendingWorkspace(), "", "", "int-t", "t-default", "no-reply@acme.test"},
		"the sender of the translation sent":                      {sendingWorkspace(), "", "fr", "int-t", "t-billing", "billing@acme.test"},
		"the marketing integration when there is no other":        {marketingOnly, "s-support", "", "int-m", "s-support", "support@acme.test"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachableIn(transactionalWritePermissions(domain.PermissionResourceTemplates), tc.workspace)
			audit := f.captureAudit()
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(templateWithSender(tc.templateSender), nil)
			f.notifs.EXPECT().TestTemplate(gomock.Any(), testWorkspaceID, "tpl_welcome", tc.integration, tc.sender, "jane@example.com", tc.language, "", false, domain.EmailOptions{}).Return(nil)

			args := map[string]any{"workspace_id": testWorkspaceID, "template_id": "tpl_welcome", "recipient_email": "Jane@example.com"}
			if tc.language != "" {
				args["language"] = tc.language
			}
			result := f.callTool("send_template_test", args)
			assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"sent":true,"template_id":"tpl_welcome","recipient_email":"jane@example.com","from_email":%q}`, tc.from),
				string(result.StructuredContent))
			row := audit.last(t)
			assert.Equal(t, "transactional.testTemplate", row.Action)
			assert.Equal(t, domain.AuditTargetTemplate, row.TargetType)
			assert.Equal(t, "tpl_welcome", row.TargetID)
			assert.Equal(t, "Welcome", row.TargetName)
			assert.Equal(t, map[string]any{"channel": "mcp", "mcp_tool": "send_template_test", "template_id": "tpl_welcome", "recipient_email": "jane@example.com"}, row.Metadata)
		})
	}
}

func TestSendTemplateTest_ForAListWithCopies(t *testing.T) {
	f, audit := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates, domain.PermissionResourceLists, domain.PermissionResourceContacts))
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(templateWithSender(""), nil)
	subject := "Test"
	f.notifs.EXPECT().TestTemplate(gomock.Any(), testWorkspaceID, "tpl_welcome", "int-t", "t-default", "jane@example.com", "", "news", true,
		domain.EmailOptions{Subject: &subject, CC: []string{"boss@example.com"}}).Return(nil)
	require.False(t, f.callTool("send_template_test", map[string]any{
		"workspace_id": testWorkspaceID, "template_id": "tpl_welcome", "recipient_email": "jane@example.com",
		"list_id": "news", "omit_list_unsubscribe_header": true, "subject": subject, "cc": []string{"boss@example.com"},
	}).IsError)
	row := audit.last(t)
	assert.Equal(t, "news", row.Metadata["list_id"])
	assert.Equal(t, []string{"boss@example.com"}, row.Metadata["cc"])
}

func TestSendTemplateTest_Refusals(t *testing.T) {
	args := func(extra map[string]any) map[string]any {
		out := map[string]any{"workspace_id": testWorkspaceID, "template_id": "tpl_welcome", "recipient_email": "jane@example.com"}
		for key, value := range extra {
			out[key] = value
		}
		return out
	}
	t.Run("a list without lists:read", func(t *testing.T) {
		f, audit := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates))
		toolErr := f.callTool("send_template_test", args(map[string]any{"list_id": "news"})).errorOf(t)
		assert.Equal(t, "lists", toolErr.Resource)
		assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
	})
	t.Run("no integration", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(transactionalWritePermissions(domain.PermissionResourceTemplates))
		f.captureAudit()
		assert.Equal(t, CodeValidation, f.callTool("send_template_test", args(nil)).errorOf(t).Code)
	})
	t.Run("a template that is not found or not an email", func(t *testing.T) {
		f, _ := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates))
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(nil, &domain.ErrTemplateNotFound{Message: "template not found"})
		assert.Equal(t, CodeNotFound, f.callTool("send_template_test", args(nil)).errorOf(t).Code)
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(&domain.Template{ID: "tpl_welcome", Channel: "web"}, nil)
		assert.Equal(t, CodeValidation, f.callTool("send_template_test", args(nil)).errorOf(t).Code)
	})
	// templates.get still returns a deleted template, since sent messages
	// render from it, and the service would send from it.
	t.Run("a deleted template", func(t *testing.T) {
		f, audit := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates))
		deleted := templateWithSender("")
		deletedAt := time.Now()
		deleted.DeletedAt = &deletedAt
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(deleted, nil)
		// No TestTemplate expectation: a send would fail the mock.
		assert.Equal(t, CodeNotFound, f.callTool("send_template_test", args(nil)).errorOf(t).Code)
		assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
	})
	t.Run("an integration without a sender", func(t *testing.T) {
		workspace := sendingWorkspace()
		workspace.Integrations[1].EmailProvider.Senders = nil
		f := newFixture(t)
		f.reachableIn(transactionalWritePermissions(domain.PermissionResourceTemplates), workspace)
		f.captureAudit()
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(templateWithSender(""), nil)
		assert.Equal(t, CodeValidation, f.callTool("send_template_test", args(nil)).errorOf(t).Code)
	})
	t.Run("addresses and list IDs", func(t *testing.T) {
		f, _ := transactionalWriteFixture(t, transactionalWritePermissions(domain.PermissionResourceTemplates, domain.PermissionResourceLists))
		assert.Equal(t, CodeValidation, f.callTool("send_template_test", args(map[string]any{"recipient_email": "jane"})).errorOf(t).Code)
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl_welcome", int64(0)).Return(templateWithSender(""), nil)
		assert.Equal(t, CodeValidation, f.callTool("send_template_test", args(map[string]any{"list_id": "not-alphanumeric"})).errorOf(t).Code)
	})
}
