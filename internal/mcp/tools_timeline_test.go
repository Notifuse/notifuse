package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

const timelineEntryID = "3f1c2a4e-8b7d-4c6e-9a01-2b3c4d5e6f70"

func strPtr(v string) *string { return &v }

// expectContact makes the contact lookup that precedes the timeline succeed.
func (f *fixture) expectContact(email string) {
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, email).Return(&domain.Contact{Email: email}, nil)
}

func messageEntry() *domain.ContactTimelineEntry {
	return &domain.ContactTimelineEntry{
		ID:          timelineEntryID,
		Email:       "jane@example.com",
		Operation:   "update",
		EntityType:  "message_history",
		Kind:        "email.clicked",
		Changes:     map[string]any{"clicked_at": map[string]any{"old": nil, "new": "2026-11-20T10:00:00Z"}},
		EntityID:    strPtr("msg_1"),
		CreatedAt:   time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
		DBCreatedAt: time.Date(2026, 11, 20, 10, 0, 1, 0, time.UTC),
		EntityData: map[string]any{
			"id": "msg_1", "template_id": "welcome", "template_version": float64(3), "template_name": "Welcome",
			"template_category": "marketing", "template_email": map[string]any{"from_address": "hi@acme.example"},
			"channel": "email", "sent_at": "2026-11-20T09:00:00Z", "delivered_at": nil, "opened_at": "2026-11-20T09:30:00Z",
			"clicked_at": "2026-11-20T10:00:00Z", "broadcast_id": "b1", "automation_id": nil, "transactional_notification_id": nil,
			"message_data": map[string]any{"data": map[string]any{"reset_link": "https://acme.example/reset?token=s3cret"}},
		},
	}
}

func TestGetContactTimeline_Definition(t *testing.T) {
	tool := catalogueTool(t, "get_contact_timeline")
	assertReadTool(t, tool, "get_contact_timeline", []Pair{{Resource: "contacts", Verb: "read"}}, []string{"workspace_id", "email"})
	items := tool.InputSchema["properties"].(map[string]any)["entity_types"].(map[string]any)["items"].(map[string]any)
	assert.Equal(t, []string{
		"contact", "contact_list", "contact_segment", "message_history", "custom_event",
		"inbound_webhook_event", "automation", "web_session", "web_page",
	}, items["enum"])
}

// Every timeline type is offered, but the legacy webhook_event, which the
// tool asks for along with inbound_webhook_event.
func TestTimelineEntityTypes_CoverTheDomain(t *testing.T) {
	for _, entityType := range domain.TimelineEntityTypes {
		if entityType == domain.TimelineEntityWebhookEvent {
			assert.NotContains(t, timelineEntityTypes, entityType)
			continue
		}
		assert.Contains(t, timelineEntityTypes, entityType)
	}
}

func TestGetContactTimeline_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}, domain.PermissionResourceMessageHistory: {Read: true}})
	f.expectContact("jane@example.com")
	f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 20, nil, nil).
		Return([]*domain.ContactTimelineEntry{messageEntry(), {
			ID: "2a", Email: "jane@example.com", Operation: "insert", EntityType: "contact_list", Kind: "list.subscribed",
			Changes: map[string]any{"status": map[string]any{"new": "active"}}, EntityID: strPtr("news"),
			CreatedAt:  time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC),
			EntityData: map[string]any{"id": "news", "name": "Newsletter", "status": "active"},
		}, {
			ID: "1a", Email: "jane@example.com", Operation: "insert", EntityType: "custom_event", Kind: "custom_event.signup",
			CreatedAt: time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC),
		}}, nil, nil)

	result := f.callTool("get_contact_timeline", map[string]any{"workspace_id": testWorkspaceID, "email": "Jane@Example.com"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"email":"jane@example.com",
		"entries":[
			{"id":"`+timelineEntryID+`","kind":"email.clicked","entity_type":"message_history","operation":"update","entity_id":"msg_1",
			 "created_at":"2026-11-20T10:00:00Z",
			 "changes":{"clicked_at":{"old":null,"new":"2026-11-20T10:00:00Z"}},
			 "entity":{"id":"msg_1","template_id":"welcome","template_name":"Welcome","channel":"email","sent_at":"2026-11-20T09:00:00Z",
			           "opened_at":"2026-11-20T09:30:00Z","clicked_at":"2026-11-20T10:00:00Z","broadcast_id":"b1"}},
			{"id":"2a","kind":"list.subscribed","entity_type":"contact_list","operation":"insert","entity_id":"news",
			 "created_at":"2026-11-19T08:00:00Z","changes":{"status":{"new":"active"}},
			 "entity":{"id":"news","name":"Newsletter","status":"active"}},
			{"id":"1a","kind":"custom_event.signup","entity_type":"custom_event","operation":"insert","created_at":"2026-11-18T08:00:00Z"}
		],
		"omitted_entity_types":["web_session","web_page"]
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"message_data", "s3cret", "template_email", "hi@acme.example", "template_version", "template_category", "db_created_at"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// The service drops the entries a key may not read without saying so; the
// tool names the types it left out.
func TestGetContactTimeline_OmittedEntityTypes(t *testing.T) {
	for name, tc := range map[string]struct {
		permissions domain.UserPermissions
		asked       []string
		omitted     []string
	}{
		"contacts only, every type":  {contactsRead(), nil, []string{"message_history", "inbound_webhook_event", "web_session", "web_page"}},
		"contacts only, some types":  {contactsRead(), []string{"web_page", "custom_event"}, []string{"web_page"}},
		"every resource, every type": {domain.UserPermissions{"contacts": {Read: true}, "message_history": {Read: true}, "web_analytics": {Read: true}}, nil, []string{}},
		"web analytics, every type":  {domain.UserPermissions{"contacts": {Read: true}, "web_analytics": {Read: true}}, nil, []string{"message_history", "inbound_webhook_event"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(tc.permissions)
			f.expectContact("jane@example.com")
			f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 20, nil, gomock.Any()).Return([]*domain.ContactTimelineEntry{}, nil, nil)

			args := map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}
			if tc.asked != nil {
				args["entity_types"] = tc.asked
			}
			var out struct {
				Entries []json.RawMessage `json:"entries"`
				Omitted []string          `json:"omitted_entity_types"`
			}
			f.callTool("get_contact_timeline", args).decode(t, &out)
			assert.Equal(t, tc.omitted, out.Omitted)
			assert.NotNil(t, out.Entries)
		})
	}
}

// entity_types are deduplicated and put in one order, and asking for inbound
// webhook events asks for their legacy name too.
func TestGetContactTimeline_EntityTypesSentToTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.expectContact("jane@example.com")
	f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 5, nil,
		[]string{"contact_list", "custom_event", "inbound_webhook_event", "webhook_event"}).
		Return([]*domain.ContactTimelineEntry{}, nil, nil)

	f.callTool("get_contact_timeline", map[string]any{
		"workspace_id": testWorkspaceID, "email": "jane@example.com", "limit": 5,
		"entity_types": []string{"inbound_webhook_event", "custom_event", "contact_list", "custom_event"},
	}).decode(t, &struct{}{})
}

func TestGetContactTimeline_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	for name, args := range map[string]map[string]any{
		"no email":            {},
		"blank email":         {"email": "  "},
		"unknown entity type": {"email": "jane@example.com", "entity_types": []string{"broadcast"}},
		"legacy entity type":  {"email": "jane@example.com", "entity_types": []string{"webhook_event"}},
		"limit above 50":      {"email": "jane@example.com", "limit": 51},
		"garbage cursor":      {"email": "jane@example.com", "cursor": "%%%"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("get_contact_timeline", args).errorOf(t).Code)
		})
	}
}

func TestGetContactTimeline_UnknownContactIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "ghost@example.com").Return(nil, domain.ErrContactNotFound)

	toolErr := f.callTool("get_contact_timeline", map[string]any{"workspace_id": testWorkspaceID, "email": "ghost@example.com"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "contact not found"}, toolErr)
}

func TestGetContactTimeline_Cursor(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	apiCursor := base64.StdEncoding.EncodeToString([]byte("2026-11-20T10:00:00Z|" + timelineEntryID))
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").Return(&domain.Contact{Email: "jane@example.com"}, nil).Times(2)

	f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 1, nil, []string{"custom_event"}).
		Return([]*domain.ContactTimelineEntry{messageEntry()}, &apiCursor, nil)
	var first struct {
		NextCursor string `json:"next_cursor"`
	}
	args := map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "entity_types": []string{"custom_event"}, "limit": 1}
	f.callTool("get_contact_timeline", args).decode(t, &first)
	require.NotEmpty(t, first.NextCursor)
	assert.NotEqual(t, apiCursor, first.NextCursor, "the API cursor is wrapped")

	f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 1, gomock.Any(), []string{"custom_event"}).
		DoAndReturn(func(_ context.Context, _, _ string, _ int, cursor *string, _ []string) ([]*domain.ContactTimelineEntry, *string, error) {
			require.NotNil(t, cursor)
			assert.Equal(t, apiCursor, *cursor)
			return []*domain.ContactTimelineEntry{}, nil, nil
		})
	args["cursor"] = first.NextCursor
	var last map[string]json.RawMessage
	f.callTool("get_contact_timeline", args).decode(t, &last)
	assert.NotContains(t, last, "next_cursor", "absent on the last page")

	for name, other := range map[string]map[string]any{
		"another email":      {"email": "john@example.com"},
		"other entity types": {"entity_types": []string{"contact"}},
		"no entity types":    {"entity_types": nil},
		"more entity types":  {"email": "jane@example.com", "entity_types": []string{"custom_event", "contact"}},
	} {
		t.Run(name, func(t *testing.T) {
			mismatched := map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "entity_types": []string{"custom_event"}, "cursor": first.NextCursor}
			for key, value := range other {
				if value == nil {
					delete(mismatched, key)
					continue
				}
				mismatched[key] = value
			}
			assert.Equal(t, CodeValidation, f.callTool("get_contact_timeline", mismatched).errorOf(t).Code)
		})
	}
}

// timeline.list answers 500 on a cursor it cannot read, so a well-formed
// envelope around a bad API cursor is refused before any service call.
func TestGetContactTimeline_BadInnerCursorNeverReachesTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	for _, inner := range []string{
		"not base64!",
		base64.StdEncoding.EncodeToString([]byte("2026-11-20T10:00:00Z")),
		base64.StdEncoding.EncodeToString([]byte("yesterday|" + timelineEntryID)),
		base64.StdEncoding.EncodeToString([]byte("2026-11-20T10:00:00Z|not-a-uuid")),
		base64.StdEncoding.EncodeToString([]byte("2026-11-20T10:00:00Z|" + timelineEntryID + "|x")),
	} {
		cursor, err := EncodeCursor("get_contact_timeline", testWorkspaceID, map[string]any{"email": "jane@example.com"}, inner)
		require.NoError(t, err)
		toolErr := f.callTool("get_contact_timeline", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "cursor": cursor}).errorOf(t)
		assert.Equal(t, CodeValidation, toolErr.Code, inner)
	}
}

func TestGetContactTimeline_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}, domain.PermissionResourceMessageHistory: {Read: true}})
	f.expectContact("jane@example.com")
	bounce := &domain.ContactTimelineEntry{
		ID: "b1", EntityType: "inbound_webhook_event", Kind: "email.bounced", Operation: "insert",
		Changes: map[string]any{"properties": map[string]any{"body": strings.Repeat("p", 2500)}},
		EntityData: map[string]any{
			"type": "bounce", "bounce_diagnostic": "550 " + strings.Repeat("d", 2100), "template_version": float64(2),
		},
	}
	f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 20, nil, nil).Return([]*domain.ContactTimelineEntry{bounce}, nil, nil)

	var out struct {
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	f.callTool("get_contact_timeline", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).decode(t, &out)
	require.Len(t, out.Entries, 1)
	entry := out.Entries[0]
	assert.Equal(t, "null", string(entry["changes"]), "changes too long to cut are null")
	var entity map[string]string
	require.NoError(t, json.Unmarshal(entry["entity"], &entity))
	assert.Equal(t, "bounce", entity["type"])
	assert.Len(t, entity["bounce_diagnostic"], 2000)
	assert.NotContains(t, entity, "template_version")
	assert.JSONEq(t, `["changes","entity.bounce_diagnostic"]`, string(entry["truncated_fields"]))
	assert.Equal(t, "true", string(entry["truncated"]))
}

func TestGetContactTimeline_ServiceErrorsAreMapped(t *testing.T) {
	t.Run("a failed timeline read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(contactsRead())
		f.expectContact("jane@example.com")
		f.timeline.EXPECT().List(gomock.Any(), testWorkspaceID, "jane@example.com", 20, nil, nil).Return(nil, nil, errors.New("failed to query timeline on db-host-7"))
		toolErr := f.callTool("get_contact_timeline", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, toolErr)
	})

	t.Run("a key without contacts:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
		toolErr := f.callTool("get_contact_timeline", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the contacts read permission", Resource: "contacts", Permission: "read"}, toolErr)
	})
}
