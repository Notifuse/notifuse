package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

const getContactTimelineName = "get_contact_timeline"

// timelineEntityTypes are the entity types get_contact_timeline accepts, in
// the order of domain.TimelineEntityTypes without the legacy webhook_event,
// which the tool asks for along with inbound_webhook_event.
var timelineEntityTypes = []string{
	domain.TimelineEntityContact,
	domain.TimelineEntityContactList,
	domain.TimelineEntityContactSegment,
	domain.TimelineEntityMessageHistory,
	domain.TimelineEntityCustomEvent,
	domain.TimelineEntityInboundWebhookEvent,
	domain.TimelineEntityAutomation,
	domain.TimelineEntityWebSession,
	domain.TimelineEntityWebPage,
}

// timelineEntityFields are the fields of each entity type's joined data the
// tool returns. Left out: message_data of a message (the data of the send,
// which holds reset links, codes and order details) and the template's
// version, category and sender.
var timelineEntityFields = map[string][]string{
	domain.TimelineEntityContact:     {"email", "first_name", "last_name", "external_id"},
	domain.TimelineEntityContactList: {"id", "name", "status"},
	domain.TimelineEntityMessageHistory: {
		"id", "template_id", "template_name", "channel", "sent_at", "delivered_at", "opened_at", "clicked_at",
		"broadcast_id", "automation_id", "transactional_notification_id",
	},
	domain.TimelineEntityInboundWebhookEvent: {
		"id", "type", "source", "message_id", "timestamp", "bounce_type", "bounce_category", "bounce_diagnostic",
		"complaint_feedback_type", "template_id", "template_name",
	},
	domain.TimelineEntityWebhookEvent: {
		"id", "type", "source", "message_id", "timestamp", "bounce_type", "bounce_category", "bounce_diagnostic",
		"complaint_feedback_type", "template_id", "template_name",
	},
	domain.TimelineEntityAutomation: {"id", "name", "status"},
}

// getContactTimelineTool pages through what happened to one contact, newest
// first.
func (s *Server) getContactTimelineTool() *Tool {
	return &Tool{
		Name:  getContactTimelineName,
		Title: "Get contact timeline",
		Description: "Lists what happened to one contact, newest first: " +
			"changes to the contact, list subscriptions, segment entries, emails sent and their opens, clicks, bounces and complaints, custom events, automations and website visits. " +
			"Each entry has its kind (such as email.clicked), the changed values and a short description of the object. " +
			"created_at is when the entry happened: a custom event is dated by its occurred_at, so an event that happened in the past sorts among older entries, even before the contact's creation. " +
			"Emails and provider events need the message_history read permission. Website visits need the web_analytics read permission. " +
			"Types the key cannot read are left out and named in omitted_entity_types. Returns a not_found error when no contact has this email. " +
			"To get the next page, pass next_cursor back as cursor with the same email and entity_types.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The contact's email address."},
				"entity_types": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": timelineEntityTypes},
					"description": "Keep only entries of these types. Default: every type the key may read.",
				},
				"limit":  map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor": map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same email and entity_types."},
			},
			"required":             []string{"workspace_id", "email"},
			"additionalProperties": false,
		},
		Handler: s.getContactTimeline,
	}
}

type getContactTimelineArgs struct {
	Email       string   `json:"email"`
	EntityTypes []string `json:"entity_types"`
	Limit       *int     `json:"limit"`
	Cursor      string   `json:"cursor"`
}

type timelineEntry struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	EntityType string          `json:"entity_type"`
	Operation  string          `json:"operation"`
	EntityID   string          `json:"entity_id,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	Changes    json.RawMessage `json:"changes,omitempty"`
	Entity     map[string]any  `json:"entity,omitempty"`
	Truncation
}

type getContactTimelineOutput struct {
	Workspace          WorkspaceRef    `json:"workspace"`
	Email              string          `json:"email"`
	Entries            []timelineEntry `json:"entries"`
	OmittedEntityTypes []string        `json:"omitted_entity_types"`
	NextCursor         string          `json:"next_cursor,omitempty"`
}

func (s *Server) getContactTimeline(ctx context.Context, call *Call) (any, error) {
	var args getContactTimelineArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// timeline.list matches the email verbatim.
	email := domain.NormalizeEmail(args.Email)
	if email == "" {
		return nil, validationError("email is required")
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	asked := askedEntityTypes(args.EntityTypes)

	filters := map[string]any{"email": email}
	if len(asked) > 0 {
		filters["entity_types"] = asked
	}
	var apiCursor *string
	if args.Cursor != "" {
		var inner string
		if toolErr := DecodeCursor(args.Cursor, getContactTimelineName, call.WorkspaceID, filters, &inner); toolErr != nil {
			return nil, toolErr
		}
		// timeline.list answers 500 on a cursor it cannot read, so it is read here
		// first.
		if !validTimelineCursor(inner) {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
		apiCursor = &inner
	}

	// The timeline of an address nobody has is empty, which would read as a
	// contact with no history.
	if _, err := s.deps.Contacts.GetContactByEmail(ctx, call.WorkspaceID, email); err != nil {
		return nil, err
	}

	serviceTypes := slices.Clone(asked)
	if slices.Contains(serviceTypes, domain.TimelineEntityInboundWebhookEvent) {
		serviceTypes = append(serviceTypes, domain.TimelineEntityWebhookEvent)
	}
	entries, next, err := s.deps.Timeline.List(ctx, call.WorkspaceID, email, limit, apiCursor, serviceTypes)
	if err != nil {
		return nil, err
	}

	out := getContactTimelineOutput{
		Workspace:          call.WorkspaceRef(),
		Email:              email,
		Entries:            make([]timelineEntry, 0, len(entries)),
		OmittedEntityTypes: omittedEntityTypes(asked, call.Effective),
	}
	for _, entry := range entries {
		if entry != nil {
			out.Entries = append(out.Entries, projectTimelineEntry(entry))
		}
	}
	if next != nil && *next != "" {
		cursor, err := EncodeCursor(getContactTimelineName, call.WorkspaceID, filters, *next)
		if err != nil {
			return nil, err
		}
		out.NextCursor = cursor
	}
	return out, nil
}

// askedEntityTypes deduplicates the entity_types argument and puts it in the
// tool's order, so a cursor matches whatever order the types were given in.
func askedEntityTypes(types []string) []string {
	var asked []string
	for _, entityType := range timelineEntityTypes {
		if slices.Contains(types, entityType) {
			asked = append(asked, entityType)
		}
	}
	return asked
}

// omittedEntityTypes are the asked types (every type when none is asked) the
// key's permissions take out of the timeline.
func omittedEntityTypes(asked []string, effective domain.UserPermissions) []string {
	if len(asked) == 0 {
		asked = timelineEntityTypes
	}
	omitted := []string{}
	for _, entityType := range asked {
		if resource, gated := domain.TimelineEntityPermissions[entityType]; gated && !effective[resource].Read {
			omitted = append(omitted, entityType)
		}
	}
	return omitted
}

// validTimelineCursor reads timeline.list's own cursor: base64 of a RFC 3339
// time and the entry's UUID joined by "|".
func validTimelineCursor(cursor string) bool {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, parts[0]); err != nil {
		return false
	}
	_, err = uuid.Parse(parts[1])
	return err == nil
}

func projectTimelineEntry(entry *domain.ContactTimelineEntry) timelineEntry {
	out := timelineEntry{
		ID:         entry.ID,
		Kind:       entry.Kind,
		EntityType: entry.EntityType,
		Operation:  entry.Operation,
		CreatedAt:  entry.CreatedAt,
	}
	if entry.EntityID != nil {
		out.EntityID = out.Text("entity_id", *entry.EntityID)
	}
	if entry.Changes != nil {
		data, err := json.Marshal(out.JSON("changes", map[string]any(entry.Changes)))
		if err != nil {
			data = json.RawMessage("null")
		}
		out.Changes = data
	}
	if fields := timelineEntityFields[entry.EntityType]; len(fields) > 0 && entry.EntityData != nil {
		entity := map[string]any{}
		for _, field := range fields {
			value, ok := entry.EntityData[field]
			if !ok || value == nil {
				continue
			}
			path := "entity." + field
			switch v := value.(type) {
			case string:
				entity[field] = out.Text(path, v)
			case float64, bool:
				entity[field] = v
			default:
				entity[field] = out.JSON(path, v)
			}
		}
		if len(entity) > 0 {
			out.Entity = entity
		}
	}
	return out
}
