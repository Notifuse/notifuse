package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

const searchMessagesName = "search_messages"

// messageStatuses are the events the status argument filters on, each the
// is_<status> filter of messages.list.
var messageStatuses = []string{
	string(domain.MessageEventDelivered),
	string(domain.MessageEventFailed),
	string(domain.MessageEventOpened),
	string(domain.MessageEventClicked),
	string(domain.MessageEventBounced),
	string(domain.MessageEventComplained),
	string(domain.MessageEventUnsubscribed),
}

// searchMessagesTool pages through the message history, newest first.
// messages.list pages by a keyset cursor, which the tool's cursor carries.
func (s *Server) searchMessagesTool() *Tool {
	return &Tool{
		Name:  searchMessagesName,
		Title: "Search messages",
		Description: "Finds the emails a workspace sent, newest first. " +
			"You can filter by recipient, message ID or external ID, origin (broadcast, automation, transactional, other), broadcast, automation, transactional notification, template, list, clicked link, an event the message reached, and send time. " +
			"Each message has its recipient, origin, template, the time of each event (delivered, failed, opened, clicked, bounced, complained, unsubscribed) and the provider's status message. " +
			"The data the email was rendered with is never returned. To get the next page, pass next_cursor back as cursor with the same filters.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceMessageHistory, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":                  map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"contact_email":                 map[string]any{"type": "string", "description": "Only messages sent to this email address."},
				"message_id":                    map[string]any{"type": "string", "description": "Only the message with this ID."},
				"external_id":                   map[string]any{"type": "string", "description": "Only the message sent with this external ID (the idempotency key of a transactional send)."},
				"origin":                        map[string]any{"type": "string", "enum": domain.MessageOrigins, "description": "Only messages sent by a broadcast, an automation, a transactional notification, or anything else (template tests, double opt-in confirmations)."},
				"broadcast_id":                  map[string]any{"type": "string", "description": "Only messages of this broadcast."},
				"automation_id":                 map[string]any{"type": "string", "description": "Only messages of this automation."},
				"transactional_notification_id": map[string]any{"type": "string", "description": "Only messages of this transactional notification."},
				"template_id":                   map[string]any{"type": "string", "description": "Only messages sent with this template."},
				"list_id":                       map[string]any{"type": "string", "description": "Only messages sent for this list."},
				"clicked_url":                   map[string]any{"type": "string", "description": "Only messages whose recipient clicked this exact URL. Needs broadcast_id."},
				"status": map[string]any{
					"type":        "string",
					"enum":        messageStatuses,
					"description": "Only messages that reached this event.",
				},
				"sent_after":  map[string]any{"type": "string", "description": "RFC 3339 time. Only messages sent at or after it."},
				"sent_before": map[string]any{"type": "string", "description": "RFC 3339 time. Only messages sent at or before it."},
				"limit":       map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":      map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same filters."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.searchMessages,
	}
}

type searchMessagesArgs struct {
	ContactEmail                string `json:"contact_email"`
	MessageID                   string `json:"message_id"`
	ExternalID                  string `json:"external_id"`
	Origin                      string `json:"origin"`
	BroadcastID                 string `json:"broadcast_id"`
	AutomationID                string `json:"automation_id"`
	TransactionalNotificationID string `json:"transactional_notification_id"`
	TemplateID                  string `json:"template_id"`
	ListID                      string `json:"list_id"`
	ClickedURL                  string `json:"clicked_url"`
	Status                      string `json:"status"`
	SentAfter                   string `json:"sent_after"`
	SentBefore                  string `json:"sent_before"`
	Limit                       *int   `json:"limit"`
	Cursor                      string `json:"cursor"`
}

// messageSummary is the projection search_messages returns. message_data is
// never part of it: it holds the whole contact and the recipient's signed
// links (unsubscribe, notification center), which act for that contact.
type messageSummary struct {
	ID                          string     `json:"id"`
	ExternalID                  *string    `json:"external_id"`
	ContactEmail                string     `json:"contact_email"`
	Origin                      string     `json:"origin"`
	BroadcastID                 *string    `json:"broadcast_id"`
	AutomationID                *string    `json:"automation_id"`
	TransactionalNotificationID *string    `json:"transactional_notification_id"`
	ListID                      *string    `json:"list_id"`
	TemplateID                  string     `json:"template_id"`
	TemplateVersion             int64      `json:"template_version"`
	Channel                     string     `json:"channel"`
	Subject                     *string    `json:"subject"`
	CC                          []string   `json:"cc"`
	BCC                         []string   `json:"bcc"`
	StatusInfo                  *string    `json:"status_info"`
	SentAt                      time.Time  `json:"sent_at"`
	DeliveredAt                 *time.Time `json:"delivered_at"`
	FailedAt                    *time.Time `json:"failed_at"`
	OpenedAt                    *time.Time `json:"opened_at"`
	ClickedAt                   *time.Time `json:"clicked_at"`
	BouncedAt                   *time.Time `json:"bounced_at"`
	ComplainedAt                *time.Time `json:"complained_at"`
	UnsubscribedAt              *time.Time `json:"unsubscribed_at"`
	OpensTracked                bool       `json:"opens_tracked"`
	ClicksTracked               bool       `json:"clicks_tracked"`
	CreatedAt                   time.Time  `json:"created_at"`
	Truncation
}

type searchMessagesOutput struct {
	Workspace  WorkspaceRef     `json:"workspace"`
	Messages   []messageSummary `json:"messages"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (s *Server) searchMessages(ctx context.Context, call *Call) (any, error) {
	var args searchMessagesArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	args.ContactEmail = domain.NormalizeEmail(args.ContactEmail)

	// The request is decoded from JSON, as every service request built from
	// tool arguments is. Empty arguments are left out.
	request := map[string]any{"limit": limit}
	for key, value := range map[string]string{
		"contact_email":                 args.ContactEmail,
		"id":                            args.MessageID,
		"external_id":                   args.ExternalID,
		"origin":                        args.Origin,
		"broadcast_id":                  args.BroadcastID,
		"automation_id":                 args.AutomationID,
		"transactional_notification_id": args.TransactionalNotificationID,
		"template_id":                   args.TemplateID,
		"list_id":                       args.ListID,
		"clicked_url":                   args.ClickedURL,
	} {
		if value != "" {
			request[key] = value
		}
	}
	if args.Status != "" {
		request["is_"+args.Status] = true
	}
	for key, value := range map[string]string{"sent_after": args.SentAfter, "sent_before": args.SentBefore} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, validationError("%s must be an RFC 3339 time, such as 2026-11-01T00:00:00Z", key)
		}
		request[key] = parsed.UTC()
	}
	// The cursor is bound to every argument but limit and cursor.
	filters := make(map[string]any, len(request))
	for key, value := range request {
		if key != "limit" {
			filters[key] = value
		}
	}
	if args.Cursor != "" {
		var apiCursor string
		if toolErr := DecodeCursor(args.Cursor, searchMessagesName, call.WorkspaceID, filters, &apiCursor); toolErr != nil {
			return nil, toolErr
		}
		// messages.list answers 500 on a cursor it cannot read, so it is read
		// here first.
		if !validMessagesCursor(apiCursor) {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
		request["cursor"] = apiCursor
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var params domain.MessageListParams
	if err := json.Unmarshal(body, &params); err != nil {
		return nil, err
	}
	if err := params.Validate(); err != nil {
		return nil, validationError("%v", err)
	}

	resp, err := s.deps.Messages.ListMessages(ctx, call.WorkspaceID, params)
	if err != nil {
		return nil, err
	}
	out := searchMessagesOutput{Workspace: call.WorkspaceRef(), Messages: []messageSummary{}}
	if resp == nil {
		return out, nil
	}
	for _, message := range resp.Messages {
		if message != nil {
			out.Messages = append(out.Messages, summarizeMessage(message))
		}
	}
	if resp.NextCursor != "" {
		next, err := EncodeCursor(searchMessagesName, call.WorkspaceID, filters, resp.NextCursor)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

// validMessagesCursor reads messages.list's own cursor: base64 of a
// RFC 3339 time and a message ID joined by a single "~".
func validMessagesCursor(cursor string) bool {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), "~")
	if len(parts) != 2 {
		return false
	}
	_, err = time.Parse(time.RFC3339Nano, parts[0])
	return err == nil
}

// messageOrigin names what sent a message, as the origin filter does.
func messageOrigin(message *domain.MessageHistory) string {
	switch {
	case message.BroadcastID != nil && *message.BroadcastID != "":
		return domain.MessageOriginBroadcast
	case message.AutomationID != nil && *message.AutomationID != "":
		return domain.MessageOriginAutomation
	case message.TransactionalNotificationID != nil && *message.TransactionalNotificationID != "":
		return domain.MessageOriginTransactional
	default:
		return domain.MessageOriginOther
	}
}

func summarizeMessage(message *domain.MessageHistory) messageSummary {
	out := messageSummary{
		ID:                          message.ID,
		ExternalID:                  message.ExternalID,
		Origin:                      messageOrigin(message),
		BroadcastID:                 message.BroadcastID,
		AutomationID:                message.AutomationID,
		TransactionalNotificationID: message.TransactionalNotificationID,
		ListID:                      message.ListID,
		TemplateID:                  message.TemplateID,
		TemplateVersion:             message.TemplateVersion,
		Channel:                     message.Channel,
		CC:                          []string{},
		BCC:                         []string{},
		SentAt:                      message.SentAt,
		DeliveredAt:                 message.DeliveredAt,
		FailedAt:                    message.FailedAt,
		OpenedAt:                    message.OpenedAt,
		ClickedAt:                   message.ClickedAt,
		BouncedAt:                   message.BouncedAt,
		ComplainedAt:                message.ComplainedAt,
		UnsubscribedAt:              message.UnsubscribedAt,
		// A message sent before tracking was recorded counts as tracked.
		OpensTracked:  message.OpensTracked == nil || *message.OpensTracked,
		ClicksTracked: message.ClicksTracked == nil || *message.ClicksTracked,
		CreatedAt:     message.CreatedAt,
	}
	out.ContactEmail = out.Text("contact_email", message.ContactEmail)
	if message.ExternalID != nil {
		externalID := out.Text("external_id", *message.ExternalID)
		out.ExternalID = &externalID
	}
	if message.StatusInfo != nil {
		info := out.Text("status_info", *message.StatusInfo)
		out.StatusInfo = &info
	}
	if options := message.ChannelOptions; options != nil {
		if options.Subject != nil {
			subject := out.Text("subject", *options.Subject)
			out.Subject = &subject
		}
		for i, address := range options.CC {
			out.CC = append(out.CC, out.Text(fmt.Sprintf("cc[%d]", i), address))
		}
		for i, address := range options.BCC {
			out.BCC = append(out.BCC, out.Text(fmt.Sprintf("bcc[%d]", i), address))
		}
	}
	return out
}
