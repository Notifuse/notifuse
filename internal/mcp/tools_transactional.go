package mcp

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

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

var transactionalWrite = Pair{Resource: domain.PermissionResourceTransactional, Verb: domain.PermissionTypeWrite}

// notificationTrackingModes are the tracking modes a transactional
// notification takes.
var notificationTrackingModes = []string{notifuse_mjml.TrackingModeInherit, notifuse_mjml.TrackingModeNoTracking, notifuse_mjml.TrackingModeDisabled}

const notificationTrackingModeHelp = "inherit follows the workspace and each contact's consent. no_tracking adds no open pixel and no click redirect but keeps the UTM parameters. disabled changes no link at all, for single-use links such as a password reset."

// utmFieldNames maps the utm_parameters keys to the tracking settings fields.
var utmFieldNames = map[string]string{
	"source": "utm_source", "medium": "utm_medium", "campaign": "utm_campaign", "content": "utm_content", "term": "utm_term",
}

// notificationOutput is what create_transactional_notification and
// update_transactional_notification return.
type notificationOutput struct {
	Workspace    WorkspaceRef       `json:"workspace"`
	Notification notificationDetail `json:"notification"`
}

func (s *Server) createTransactionalNotificationTool() *Tool {
	return &Tool{
		Name:  "create_transactional_notification",
		Title: "Create transactional notification",
		Description: "Creates a transactional notification: an email an application sends one at a time with send_transactional, such as an order confirmation. " +
			"It sends nothing. The template must exist. " +
			"The ID is yours to choose and cannot be reused, even after the notification is deleted, so a repeated call answers conflict.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(false),
		Pairs:       []Pair{transactionalWrite, readTemplates},
		AuditAction: "transactional.create",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":      map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"notification_id":   map[string]any{"type": "string", "description": "The ID applications send it with, 1 to 32 characters, such as order_confirmation."},
				"name":              map[string]any{"type": "string", "description": "1 to 255 characters."},
				"description":       map[string]any{"type": "string"},
				"email_template_id": map[string]any{"type": "string", "description": "Template ID from list_templates, sent by email."},
				"tracking_mode":     map[string]any{"type": "string", "enum": notificationTrackingModes, "description": notificationTrackingModeHelp + " Default inherit."},
				"utm_parameters":    utmSchema("UTM parameters added to the links of the email."),
				"metadata":          map[string]any{"type": "object", "description": "Your own key and value pairs stored on the notification, at most 32 KiB."},
			},
			"required":             []string{"workspace_id", "notification_id", "name", "email_template_id"},
			"additionalProperties": false,
		},
		Handler: s.createTransactionalNotification,
	}
}

// trackingFields composes the tracking settings of a notification request:
// the stored ones, with the tracking_mode and utm_parameters given replacing
// theirs. The block replaces the stored one as a whole, so the half not given
// is written back as it was.
func trackingFields(args map[string]any, stored notifuse_mjml.TrackingSettings) map[string]any {
	fields := map[string]any{
		"enable_tracking": stored.EnableTracking,
		"tracking_mode":   stored.TrackingMode,
		"utm_source":      stored.UTMSource,
		"utm_medium":      stored.UTMMedium,
		"utm_campaign":    stored.UTMCampaign,
		"utm_content":     stored.UTMContent,
		"utm_term":        stored.UTMTerm,
	}
	if mode, ok := args["tracking_mode"]; ok {
		fields["tracking_mode"] = mode
	}
	if utm, ok := args["utm_parameters"].(map[string]any); ok {
		for key, field := range utmFieldNames {
			fields[field] = utm[key]
			if utm[key] == nil {
				fields[field] = ""
			}
		}
	}
	return fields
}

func (s *Server) createTransactionalNotification(ctx context.Context, call *Call) (any, error) {
	args := call.Args
	id, _ := args["notification_id"].(string)
	name, _ := args["name"].(string)
	call.Audit.SetTarget(domain.AuditTargetNotification, id, name)
	metadata, _ := args["metadata"].(map[string]any)
	if toolErr := freeFormArgument("metadata", metadata); toolErr != nil {
		return nil, toolErr
	}
	notification := map[string]any{
		"id":                id,
		"name":              name,
		"channels":          map[string]any{string(domain.TransactionalChannelEmail): map[string]any{"template_id": args["email_template_id"]}},
		"tracking_settings": trackingFields(args, notifuse_mjml.TrackingSettings{}),
	}
	copyArgs(args, notification, map[string]string{"description": "description", "metadata": "metadata"})
	var req domain.CreateTransactionalRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "notification": notification}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// transactional.create validates in its handler, not in the service.
	if err := req.Validate(); err != nil {
		return nil, err
	}
	created, err := s.deps.Transactional.CreateNotification(ctx, call.WorkspaceID, req.Notification)
	if err != nil {
		return nil, err
	}
	return notificationOutput{Workspace: call.WorkspaceRef(), Notification: projectNotification(created)}, nil
}

func (s *Server) updateTransactionalNotificationTool() *Tool {
	return &Tool{
		Name:  "update_transactional_notification",
		Title: "Update transactional notification",
		Description: "Changes the fields you give on a transactional notification and keeps the others. " +
			"Every later send of the notification uses the change, so confirm it with the user first. " +
			"A new email_template_id needs permission to read templates. " +
			"tracking_mode alone keeps the UTM parameters, and utm_parameters alone keeps the tracking mode. " +
			"A notification managed by an integration only takes description, tracking and metadata changes. " +
			"The notification is read then written, so a change saved by someone else in between can be overwritten. " +
			"Calling it again with the same arguments changes nothing more.",
		Class:      ClassWriteOpenWorld,
		Idempotent: boolPtr(true),
		Pairs:      []Pair{transactionalWrite},
		ArgumentPairs: func(args map[string]any) []Pair {
			if _, ok := args["email_template_id"]; ok {
				return []Pair{readTemplates}
			}
			return nil
		},
		AuditAction: "transactional.update",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":      map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"notification_id":   map[string]any{"type": "string", "description": "Notification ID from list_transactional_notifications."},
				"name":              map[string]any{"type": "string", "description": "1 to 255 characters."},
				"description":       map[string]any{"type": "string", "description": "An empty text clears it."},
				"email_template_id": map[string]any{"type": "string", "description": "Template ID from list_templates, sent by email from now on."},
				"tracking_mode":     map[string]any{"type": "string", "enum": notificationTrackingModes, "description": notificationTrackingModeHelp},
				"utm_parameters":    utmSchema("Replaces all UTM parameters. Keys you leave out are cleared."),
				"metadata":          map[string]any{"type": "object", "description": "Replaces the stored key and value pairs, at most 32 KiB."},
			},
			"required":             []string{"workspace_id", "notification_id"},
			"additionalProperties": false,
		},
		Handler: s.updateTransactionalNotification,
	}
}

func (s *Server) updateTransactionalNotification(ctx context.Context, call *Call) (any, error) {
	args := call.Args
	id, _ := args["notification_id"].(string)
	call.Audit.SetTarget(domain.AuditTargetNotification, id, "")
	if name, ok := args["name"].(string); ok && name == "" {
		return nil, validationError("name cannot be empty")
	}
	metadata, _ := args["metadata"].(map[string]any)
	if toolErr := freeFormArgument("metadata", metadata); toolErr != nil {
		return nil, toolErr
	}
	// The stored notification gives the half of the tracking settings a call
	// leaves out, and the channel settings a new template keeps.
	stored, err := s.deps.NotificationRows.Get(ctx, call.WorkspaceID, id)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, &domain.ErrNotFound{Entity: "transactional_notification", ID: id}
	}
	call.Audit.SetTarget("", "", stored.Name)

	// Only the keys given are sent: the service keeps the others.
	updates := map[string]any{}
	copyArgs(args, updates, map[string]string{"name": "name", "description": "description", "metadata": "metadata"})
	if templateID, ok := args["email_template_id"]; ok {
		channel := map[string]any{"template_id": templateID}
		if settings := stored.Channels[domain.TransactionalChannelEmail].Settings; len(settings) > 0 {
			channel["settings"] = map[string]any(settings)
		}
		updates["channels"] = map[string]any{string(domain.TransactionalChannelEmail): channel}
	}
	_, withMode := args["tracking_mode"]
	_, withUTM := args["utm_parameters"]
	if withMode || withUTM {
		updates["tracking_settings"] = trackingFields(args, stored.TrackingSettings)
	}
	var req domain.UpdateTransactionalRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id, "updates": updates}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// transactional.update validates in its handler, not in the service.
	if err := req.Validate(); err != nil {
		return nil, err
	}
	updated, err := s.deps.Transactional.UpdateNotification(ctx, call.WorkspaceID, id, req.Updates)
	if err != nil {
		return nil, err
	}
	call.Audit.SetTarget("", "", updated.Name)
	return notificationOutput{Workspace: call.WorkspaceRef(), Notification: projectNotification(updated)}, nil
}

func (s *Server) deleteTransactionalNotificationTool() *Tool {
	return &Tool{
		Name:  "delete_transactional_notification",
		Title: "Delete transactional notification",
		Description: "Deletes a transactional notification. Applications that still send it get an error from then on, and its ID cannot be used again. " +
			"Messages already sent keep their history. This cannot be undone, so confirm with the user first. " +
			"A notification managed by an integration answers validation. " +
			"Returns not_found for a notification that does not exist or is already deleted, which is also what a repeated call answers.",
		Class:       ClassDestructive,
		Pairs:       []Pair{transactionalWrite},
		AuditAction: "transactional.delete",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"notification_id": map[string]any{"type": "string", "description": "Notification ID from list_transactional_notifications."},
			},
			"required":             []string{"workspace_id", "notification_id"},
			"additionalProperties": false,
		},
		Handler: s.deleteTransactionalNotification,
	}
}

type deleteNotificationOutput struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Deleted        bool         `json:"deleted"`
	NotificationID string       `json:"notification_id"`
}

func (s *Server) deleteTransactionalNotification(ctx context.Context, call *Call) (any, error) {
	id, _ := call.Args["notification_id"].(string)
	call.Audit.SetTarget(domain.AuditTargetNotification, id, "")
	var req domain.DeleteTransactionalRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	// The service reads the notification first, deleted ones left out, so a
	// repeated call answers not_found without a read here.
	if err := s.deps.Transactional.DeleteNotification(ctx, call.WorkspaceID, id); err != nil {
		return nil, err
	}
	return deleteNotificationOutput{Workspace: call.WorkspaceRef(), Deleted: true, NotificationID: id}, nil
}

// maxExternalIDLength is the length of message_history.external_id.
const maxExternalIDLength = 255

// copyRecipients checks cc and bcc, records them in the audit row, and copies
// them into the email options.
func copyRecipients(call *Call, options map[string]any) *ToolError {
	for _, key := range []string{"cc", "bcc"} {
		items, _ := call.Args[key].([]any)
		if len(items) == 0 {
			continue
		}
		addresses := make([]string, 0, len(items))
		for _, item := range items {
			raw, _ := item.(string)
			address, toolErr := recipientArgument(key, raw)
			if toolErr != nil {
				return toolErr
			}
			addresses = append(addresses, address)
		}
		options[key] = addresses
		call.Audit.AddMetadata(key, addresses)
	}
	return nil
}

// withCopies is the argument-dependent pair of the send tools: a cc or a bcc
// reads the contact, whose whole record the subject's Liquid can quote.
func withCopies(args map[string]any) []Pair {
	for _, key := range []string{"cc", "bcc"} {
		if items, _ := args[key].([]any); len(items) > 0 {
			return []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}}
		}
	}
	return nil
}

func copiesSchema(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}

func (s *Server) sendTransactionalTool() *Tool {
	return &Tool{
		Name:  "send_transactional",
		Title: "Send transactional email",
		Description: "Sends a transactional notification's email to one address now, through the transactional email integration, and returns its message_id. " +
			"The address is added as a contact when it is not one; its other contact fields are not changed. " +
			"This emails a real person, so confirm the recipient and the content with the user first. " +
			"external_id is your key for this one send: a second call with the same key sends nothing and returns the first message_id. " +
			"That holds even when the first send failed, so retry a failed send with a new external_id. " +
			"cc and bcc need permission to read contacts.",
		Class:         ClassSend,
		Idempotent:    boolPtr(true),
		Pairs:         []Pair{transactionalWrite},
		ArgumentPairs: withCopies,
		AuditAction:   "transactional.send",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"notification_id": map[string]any{"type": "string", "description": "Notification ID from list_transactional_notifications."},
				"external_id":     map[string]any{"type": "string", "description": "Your unique key for this send, 1 to 255 characters, such as order-981-confirmation. Never reuse it for another send."},
				"email":           map[string]any{"type": "string", "description": "The recipient's address."},
				"data":            map[string]any{"type": "object", "description": "Template variables, such as {\"order\": {\"number\": \"981\"}}, at most 32 KiB."},
				"subject":         map[string]any{"type": "string", "description": "Replaces the template's subject, Liquid allowed, at most 255 characters."},
				"preheader":       map[string]any{"type": "string", "description": "Replaces the template's inbox preview text, at most 255 characters."},
				"from_name":       map[string]any{"type": "string", "description": "Replaces the sender's display name."},
				"reply_to":        map[string]any{"type": "string", "description": "Reply-to address."},
				"cc":              copiesSchema("Addresses that get a copy. Needs permission to read contacts."),
				"bcc":             copiesSchema("Addresses that get a hidden copy. Needs permission to read contacts."),
				"metadata":        map[string]any{"type": "object", "description": "Your own key and value pairs stored with the message, at most 32 KiB."},
			},
			"required":             []string{"workspace_id", "notification_id", "external_id", "email"},
			"additionalProperties": false,
		},
		Handler: s.sendTransactional,
	}
}

type sendTransactionalOutput struct {
	Workspace      WorkspaceRef `json:"workspace"`
	MessageID      string       `json:"message_id"`
	NotificationID string       `json:"notification_id"`
	ExternalID     string       `json:"external_id"`
}

func (s *Server) sendTransactional(ctx context.Context, call *Call) (any, error) {
	args := call.Args
	id, _ := args["notification_id"].(string)
	call.Audit.SetTarget(domain.AuditTargetNotification, id, "")
	call.Audit.AddMetadata("notification_id", id)
	raw, _ := args["email"].(string)
	email, toolErr := recipientArgument("email", raw)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.AddMetadata("recipient_email", email)
	externalID, _ := args["external_id"].(string)
	if externalID == "" || utf8.RuneCountInString(externalID) > maxExternalIDLength {
		return nil, validationError("external_id must be 1 to %d characters", maxExternalIDLength)
	}
	call.Audit.AddMetadata("external_id", externalID)
	options := map[string]any{}
	if toolErr := copyRecipients(call, options); toolErr != nil {
		return nil, toolErr
	}
	copyArgs(args, options, map[string]string{"subject": "subject", "preheader": "subject_preview", "from_name": "from_name", "reply_to": "reply_to"})
	for _, key := range []string{"data", "metadata"} {
		value, _ := args[key].(map[string]any)
		if toolErr := freeFormArgument(key, value); toolErr != nil {
			return nil, toolErr
		}
	}
	// The service answers a missing integration untyped.
	provider, _, err := call.Workspace.GetEmailProviderWithIntegrationID(false)
	if err != nil || provider == nil || provider.Kind == "" {
		return nil, validationError("the workspace has no usable transactional email integration: set one in the console first")
	}

	notification := map[string]any{
		"id":            id,
		"external_id":   externalID,
		"contact":       map[string]any{"email": email},
		"channels":      []string{string(domain.TransactionalChannelEmail)},
		"email_options": options,
	}
	copyArgs(args, notification, map[string]string{"data": "data", "metadata": "metadata"})
	var req domain.SendTransactionalRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "notification": notification}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// transactional.send validates in its handler, not in the service.
	if err := req.Validate(); err != nil {
		return nil, err
	}
	messageID, err := s.deps.Transactional.SendNotification(ctx, call.WorkspaceID, req.Notification)
	if err != nil {
		return nil, err
	}
	call.Audit.AddMetadata("message_id", messageID)
	return sendTransactionalOutput{Workspace: call.WorkspaceRef(), MessageID: messageID, NotificationID: id, ExternalID: externalID}, nil
}

func (s *Server) sendTemplateTestTool() *Tool {
	return &Tool{
		Name:  "send_template_test",
		Title: "Send template test email",
		Description: "Sends a template's email to one address now, to check how it looks. " +
			"It goes through the transactional email integration, or the marketing one when there is none, from the template's sender or else the integration's default sender. " +
			"The address is added as a contact when it is not one. " +
			"Calling it again sends the email again, so confirm the address with the user first. " +
			"list_id needs permission to read lists, and cc or bcc permission to read contacts.",
		Class: ClassSend,
		Pairs: []Pair{transactionalWrite, readTemplates},
		ArgumentPairs: func(args map[string]any) []Pair {
			pairs := withCopies(args)
			if listID, _ := args["list_id"].(string); listID != "" {
				pairs = append(pairs, Pair{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeRead})
			}
			return pairs
		},
		AuditAction: "transactional.testTemplate",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"template_id":     map[string]any{"type": "string", "description": "Template ID from list_templates."},
				"recipient_email": map[string]any{"type": "string", "description": "The address that gets the email."},
				"language":        map[string]any{"type": "string", "description": "Translation to send, such as fr. Default: the workspace's default language."},
				"list_id":         map[string]any{"type": "string", "description": "List ID from list_lists: the email carries that list's unsubscribe links and header, as a broadcast to it does."},
				"omit_list_unsubscribe_header": map[string]any{
					"type":        "boolean",
					"description": "With list_id, leave out the one-click unsubscribe header, as a broadcast that keeps unsubscribed contacts does.",
				},
				"subject":   map[string]any{"type": "string", "description": "Replaces the template's subject, Liquid allowed."},
				"preheader": map[string]any{"type": "string", "description": "Replaces the template's inbox preview text."},
				"cc":        copiesSchema("Addresses that get a copy. Needs permission to read contacts."),
				"bcc":       copiesSchema("Addresses that get a hidden copy. Needs permission to read contacts."),
			},
			"required":             []string{"workspace_id", "template_id", "recipient_email"},
			"additionalProperties": false,
		},
		Handler: s.sendTemplateTest,
	}
}

type sendTemplateTestOutput struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Sent           bool         `json:"sent"`
	TemplateID     string       `json:"template_id"`
	RecipientEmail string       `json:"recipient_email"`
	FromEmail      string       `json:"from_email"`
}

// testIntegration is the integration a template test goes through: the
// workspace's transactional one, else its marketing one.
func testIntegration(workspace *domain.Workspace) *domain.Integration {
	for _, id := range []string{workspace.Settings.TransactionalEmailProviderID, workspace.Settings.MarketingEmailProviderID} {
		if id == "" {
			continue
		}
		if integration := workspace.GetIntegrationByID(id); integration != nil {
			return integration
		}
	}
	return nil
}

func (s *Server) sendTemplateTest(ctx context.Context, call *Call) (any, error) {
	args := call.Args
	templateID, _ := args["template_id"].(string)
	call.Audit.SetTarget(domain.AuditTargetTemplate, templateID, "")
	call.Audit.AddMetadata("template_id", templateID)
	raw, _ := args["recipient_email"].(string)
	recipient, toolErr := recipientArgument("recipient_email", raw)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.AddMetadata("recipient_email", recipient)
	options := map[string]any{}
	if toolErr := copyRecipients(call, options); toolErr != nil {
		return nil, toolErr
	}
	copyArgs(args, options, map[string]string{"subject": "subject", "preheader": "subject_preview"})
	listID, _ := args["list_id"].(string)
	if listID != "" {
		call.Audit.AddMetadata("list_id", listID)
	}

	// templates.testTemplate takes an integration and a sender, which no tool
	// lists: the tool picks them as the console would.
	integration := testIntegration(call.Workspace)
	if integration == nil {
		return nil, validationError("the workspace has no email integration: set one in the console first")
	}
	// readTemplate answers not_found for a deleted template, which the
	// service would still send from.
	template, toolErr, err := s.readTemplate(ctx, call.WorkspaceID, templateID, nil)
	if toolErr != nil {
		return nil, toolErr
	}
	if err != nil {
		return nil, err
	}
	call.Audit.SetTarget("", "", template.Name)
	if template.Email == nil {
		return nil, validationError("template %s is not an email template", templateID)
	}
	language, _ := args["language"].(string)
	content := template.ResolveEmailContent(language, call.Workspace.Settings.DefaultLanguage)
	templateSender := ""
	if content != nil {
		templateSender = content.SenderID
	}
	sender, err := integration.EmailProvider.ResolveSender("", templateSender)
	if err != nil || sender == nil {
		return nil, validationError("the email integration %s has no default sender: set one in the console first", integration.Name)
	}

	fields := map[string]any{
		"workspace_id":    call.WorkspaceID,
		"template_id":     templateID,
		"integration_id":  integration.ID,
		"sender_id":       sender.ID,
		"recipient_email": recipient,
		"email_options":   options,
	}
	copyArgs(args, fields, map[string]string{"language": "language", "list_id": "list_id", "omit_list_unsubscribe_header": "omit_list_unsubscribe_header"})
	var req domain.TestTemplateRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// transactional.testTemplate validates in its handler, not in the service.
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.deps.Transactional.TestTemplate(ctx, call.WorkspaceID, req.TemplateID, req.IntegrationID, req.SenderID, req.RecipientEmail,
		req.Language, req.ListID, req.OmitListUnsubscribeHeader, req.EmailOptions); err != nil {
		return nil, err
	}
	return sendTemplateTestOutput{
		Workspace: call.WorkspaceRef(), Sent: true, TemplateID: templateID, RecipientEmail: recipient, FromEmail: sender.Email,
	}, nil
}
