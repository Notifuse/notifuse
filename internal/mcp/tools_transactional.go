package mcp

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

const listTransactionalNotificationsName = "list_transactional_notifications"

var transactionalRead = Pair{Resource: domain.PermissionResourceTransactional, Verb: domain.PermissionTypeRead}

// listTransactionalNotificationsTool pages through a workspace's
// transactional notifications. transactional.list pages by offset on
// created_at alone, so the tool reads them all and pages them by offset on
// (created_at, id).
func (s *Server) listTransactionalNotificationsTool() *Tool {
	return &Tool{
		Name:  listTransactionalNotificationsName,
		Title: "List transactional notifications",
		Description: "Lists the transactional notifications of a workspace, newest first. " +
			"These are emails an application sends one at a time, such as a password reset. " +
			"Each has its ID (the value the application sends with), name, description, the template of each channel, and whether an integration manages it. " +
			"total counts every notification that matches the search. To get the next page, pass next_cursor back as cursor with the same search. " +
			"A notification created between two pages can show up twice.",
		Class: ClassRead,
		Pairs: []Pair{transactionalRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"search":       map[string]any{"type": "string", "description": "Part of the notification name or ID to match, case-insensitive."},
				"limit":        map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":       map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same search."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listTransactionalNotifications,
	}
}

type listTransactionalNotificationsArgs struct {
	Search string `json:"search"`
	Limit  *int   `json:"limit"`
	Cursor string `json:"cursor"`
}

// notificationSummary is the projection list_transactional_notifications
// returns. The integration's ID is reduced to managed_by_integration: no
// tool takes it.
type notificationSummary struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	TemplateIDs          map[string]string `json:"template_ids"`
	ManagedByIntegration bool              `json:"managed_by_integration"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	Truncation
}

type listTransactionalNotificationsOutput struct {
	Workspace     WorkspaceRef          `json:"workspace"`
	Notifications []notificationSummary `json:"notifications"`
	Total         int                   `json:"total"`
	NextCursor    string                `json:"next_cursor,omitempty"`
}

func (s *Server) listTransactionalNotifications(ctx context.Context, call *Call) (any, error) {
	var args listTransactionalNotificationsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	search := strings.TrimSpace(args.Search)
	filters := map[string]any{}
	var serviceFilter map[string]interface{}
	if search != "" {
		filters["search"] = search
		serviceFilter = map[string]interface{}{"search": search}
	}
	offset := 0
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listTransactionalNotificationsName, call.WorkspaceID, filters, &offset); toolErr != nil {
			return nil, toolErr
		}
		if offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	// A limit of 0 reads every notification: the page is cut here, on an order
	// with a tie-breaker.
	notifications, _, err := s.deps.Transactional.ListNotifications(ctx, call.WorkspaceID, serviceFilter, 0, 0)
	if err != nil {
		return nil, err
	}
	notifications = slices.DeleteFunc(notifications, func(n *domain.TransactionalNotification) bool {
		return n == nil || n.DeletedAt != nil
	})
	slices.SortFunc(notifications, func(a, b *domain.TransactionalNotification) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	out := listTransactionalNotificationsOutput{
		Workspace:     call.WorkspaceRef(),
		Notifications: []notificationSummary{},
		Total:         len(notifications),
	}
	if offset >= len(notifications) {
		return out, nil
	}
	end := min(offset+limit, len(notifications))
	for _, notification := range notifications[offset:end] {
		out.Notifications = append(out.Notifications, summarizeNotification(notification))
	}
	if end < len(notifications) {
		next, err := EncodeCursor(listTransactionalNotificationsName, call.WorkspaceID, filters, end)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

func summarizeNotification(notification *domain.TransactionalNotification) notificationSummary {
	out := notificationSummary{
		ID:                   notification.ID,
		TemplateIDs:          map[string]string{},
		ManagedByIntegration: notification.IntegrationID != nil && *notification.IntegrationID != "",
		CreatedAt:            notification.CreatedAt,
		UpdatedAt:            notification.UpdatedAt,
	}
	out.Name = out.Text("name", notification.Name)
	out.Description = out.Text("description", notification.Description)
	for channel, template := range notification.Channels {
		out.TemplateIDs[string(channel)] = template.TemplateID
	}
	return out
}

// getTransactionalNotificationTool reads one transactional notification.
func (s *Server) getTransactionalNotificationTool() *Tool {
	return &Tool{
		Name:  "get_transactional_notification",
		Title: "Get transactional notification",
		Description: "Reads one transactional notification: " +
			"name, description, the template of each channel, tracking and UTM settings, metadata, and whether an integration manages it. " +
			"Use get_template to read the template content. Metadata longer than 2000 characters is null and named in truncated_fields.",
		Class: ClassRead,
		Pairs: []Pair{transactionalRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"notification_id": map[string]any{"type": "string", "description": "Notification ID from list_transactional_notifications."},
			},
			"required":             []string{"workspace_id", "notification_id"},
			"additionalProperties": false,
		},
		Handler: s.getTransactionalNotification,
	}
}

type getTransactionalNotificationArgs struct {
	NotificationID string `json:"notification_id"`
}

type notificationChannel struct {
	TemplateID string `json:"template_id"`
}

// notificationTracking is the stored tracking settings without the fields a
// send fills in (endpoint, workspace and message IDs, the recipient's token).
type notificationTracking struct {
	EnableTracking bool   `json:"enable_tracking"`
	TrackingMode   string `json:"tracking_mode"`
	UTMSource      string `json:"utm_source"`
	UTMMedium      string `json:"utm_medium"`
	UTMCampaign    string `json:"utm_campaign"`
	UTMContent     string `json:"utm_content"`
	UTMTerm        string `json:"utm_term"`
}

type notificationDetail struct {
	ID                   string                         `json:"id"`
	Name                 string                         `json:"name"`
	Description          string                         `json:"description"`
	Channels             map[string]notificationChannel `json:"channels"`
	Tracking             notificationTracking           `json:"tracking"`
	Metadata             any                            `json:"metadata"`
	ManagedByIntegration bool                           `json:"managed_by_integration"`
	CreatedAt            time.Time                      `json:"created_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
	Truncation
}

type getTransactionalNotificationOutput struct {
	Workspace    WorkspaceRef       `json:"workspace"`
	Notification notificationDetail `json:"notification"`
}

func (s *Server) getTransactionalNotification(ctx context.Context, call *Call) (any, error) {
	var args getTransactionalNotificationArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if args.NotificationID == "" {
		return nil, validationError("notification_id is required")
	}
	notification, err := s.deps.Transactional.GetNotification(ctx, call.WorkspaceID, args.NotificationID)
	if err != nil {
		return nil, err
	}
	// The repository filters deleted notifications; one that comes back deleted
	// is answered the same way.
	if notification == nil || notification.DeletedAt != nil {
		return nil, &domain.ErrNotFound{Entity: "transactional_notification", ID: args.NotificationID}
	}
	return getTransactionalNotificationOutput{Workspace: call.WorkspaceRef(), Notification: projectNotification(notification)}, nil
}

func projectNotification(notification *domain.TransactionalNotification) notificationDetail {
	tracking := notification.TrackingSettings
	mode := tracking.TrackingMode
	if mode == "" {
		mode = notifuse_mjml.TrackingModeInherit
	}
	out := notificationDetail{
		ID:       notification.ID,
		Channels: map[string]notificationChannel{},
		Tracking: notificationTracking{
			EnableTracking: tracking.EnableTracking,
			TrackingMode:   mode,
		},
		Metadata:             map[string]any{},
		ManagedByIntegration: notification.IntegrationID != nil && *notification.IntegrationID != "",
		CreatedAt:            notification.CreatedAt,
		UpdatedAt:            notification.UpdatedAt,
	}
	out.Name = out.Text("name", notification.Name)
	out.Description = out.Text("description", notification.Description)
	out.Tracking.UTMSource = out.Text("tracking.utm_source", tracking.UTMSource)
	out.Tracking.UTMMedium = out.Text("tracking.utm_medium", tracking.UTMMedium)
	out.Tracking.UTMCampaign = out.Text("tracking.utm_campaign", tracking.UTMCampaign)
	out.Tracking.UTMContent = out.Text("tracking.utm_content", tracking.UTMContent)
	out.Tracking.UTMTerm = out.Text("tracking.utm_term", tracking.UTMTerm)
	for channel, template := range notification.Channels {
		out.Channels[string(channel)] = notificationChannel{TemplateID: template.TemplateID}
	}
	if len(notification.Metadata) > 0 {
		out.Metadata = out.JSON("metadata", map[string]any(notification.Metadata))
	}
	return out
}
