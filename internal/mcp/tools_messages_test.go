package mcp

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func messagesReadPermission() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}}
}

var messagesEpoch = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

// fullMessage carries every field of a message, the rendering data included.
func fullMessage() *domain.MessageHistory {
	delivered := messagesEpoch.Add(time.Minute)
	return &domain.MessageHistory{
		ID: "msg-1", ExternalID: strPtr("order-981"), SMTPMessageID: strPtr("<smtp-id@acme>"),
		ContactEmail:                "ada@example.com",
		TransactionalNotificationID: strPtr("password_reset"),
		TemplateID:                  "tpl_reset", TemplateVersion: 3, Channel: "email",
		StatusInfo: strPtr("250 accepted"),
		MessageData: domain.MessageData{
			Data:     map[string]interface{}{"contact": map[string]interface{}{"email_hmac": "hmac-secret"}, "unsubscribe_url": "https://acme.com/unsub?token=signed-link"},
			Metadata: map[string]interface{}{"rendered_language": "fr"},
		},
		ChannelOptions: &domain.ChannelOptions{
			FromName: strPtr("Acme support"), Subject: strPtr("Reset your password"), SubjectPreview: strPtr("preview-text"),
			CC: []string{"cc@example.com"}, ReplyTo: "reply@acme.com",
		},
		Attachments:  []domain.AttachmentMetadata{{Filename: "invoice.pdf"}},
		SentAt:       messagesEpoch,
		DeliveredAt:  &delivered,
		OpensTracked: boolPtr(false),
		CreatedAt:    messagesEpoch, UpdatedAt: messagesEpoch.Add(time.Hour),
	}
}

func TestSearchMessages_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "search_messages"), "search_messages", []Pair{{Resource: "message_history", Verb: "read"}}, []string{"workspace_id"})
}

func TestSearchMessages_SendsTheFiltersAndDropsMessageData(t *testing.T) {
	f := newFixture(t)
	f.reachable(messagesReadPermission())
	sentAfter := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	sentBefore := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	f.messages.EXPECT().ListMessages(gomock.Any(), testWorkspaceID, domain.MessageListParams{
		Limit: 5, ContactEmail: "ada@example.com", ID: "msg-1", ExternalID: "order-981", Origin: "transactional",
		TransactionalNotificationID: "password_reset", TemplateID: "tpl_reset", ListID: "news",
		IsDelivered: boolPtr(true), SentAfter: &sentAfter, SentBefore: &sentBefore,
	}).Return(&domain.MessageListResult{Messages: []*domain.MessageHistory{fullMessage()}}, nil)

	result := f.callTool("search_messages", map[string]any{
		"workspace_id": testWorkspaceID, "limit": 5, "contact_email": " Ada@Example.com ", "message_id": "msg-1", "external_id": "order-981",
		"origin": "transactional", "transactional_notification_id": "password_reset", "template_id": "tpl_reset", "list_id": "news",
		"status": "delivered", "sent_after": "2026-11-01T01:00:00+01:00", "sent_before": "2026-11-30T00:00:00Z",
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"messages":[{
			"id":"msg-1","external_id":"order-981","contact_email":"ada@example.com","origin":"transactional",
			"broadcast_id":null,"automation_id":null,"transactional_notification_id":"password_reset","list_id":null,
			"template_id":"tpl_reset","template_version":3,"channel":"email",
			"subject":"Reset your password","cc":["cc@example.com"],"bcc":[],"status_info":"250 accepted",
			"sent_at":"2026-11-20T09:00:00Z","delivered_at":"2026-11-20T09:01:00Z","failed_at":null,"opened_at":null,"clicked_at":null,
			"bounced_at":null,"complained_at":null,"unsubscribed_at":null,
			"opens_tracked":false,"clicks_tracked":true,"created_at":"2026-11-20T09:00:00Z"
		}]
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"message_data", "hmac-secret", "signed-link", "rendered_language", "smtp-id", "invoice.pdf", "Acme support", "preview-text", "reply@acme.com", "updated_at"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestSearchMessages_Origin(t *testing.T) {
	for want, message := range map[string]*domain.MessageHistory{
		"broadcast":     {BroadcastID: strPtr("bc1")},
		"automation":    {AutomationID: strPtr("auto1")},
		"transactional": {TransactionalNotificationID: strPtr("reset")},
		"other":         {},
	} {
		assert.Equal(t, want, messageOrigin(message))
	}
}

func TestSearchMessages_PagesWithTheAPICursor(t *testing.T) {
	f := newFixture(t)
	f.reachable(messagesReadPermission())
	apiCursor := base64.StdEncoding.EncodeToString([]byte("2026-11-20T09:00:00Z~msg-1"))
	f.messages.EXPECT().ListMessages(gomock.Any(), testWorkspaceID, domain.MessageListParams{Limit: 1, BroadcastID: "bc1"}).
		Return(&domain.MessageListResult{Messages: []*domain.MessageHistory{fullMessage()}, NextCursor: apiCursor, HasMore: true}, nil)
	f.messages.EXPECT().ListMessages(gomock.Any(), testWorkspaceID, domain.MessageListParams{Limit: 1, BroadcastID: "bc1", Cursor: apiCursor}).
		Return(&domain.MessageListResult{Messages: []*domain.MessageHistory{}}, nil)

	var first struct {
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("search_messages", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "limit": 1}).decode(t, &first)
	require.NotEmpty(t, first.NextCursor)
	assert.NotEqual(t, apiCursor, first.NextCursor, "the API cursor is wrapped")

	result := f.callTool("search_messages", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "limit": 1, "cursor": first.NextCursor})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"messages":[]}`, string(result.StructuredContent))

	toolErr := f.callTool("search_messages", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc2", "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to its filters")
}

func TestSearchMessages_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(messagesReadPermission())
	unreadable, err := EncodeCursor("search_messages", testWorkspaceID, map[string]any{}, base64.StdEncoding.EncodeToString([]byte("yesterday~msg-1")))
	require.NoError(t, err)
	tooManyParts, err := EncodeCursor("search_messages", testWorkspaceID, map[string]any{}, base64.StdEncoding.EncodeToString([]byte("2026-11-20T09:00:00Z~a~b")))
	require.NoError(t, err)

	for name, args := range map[string]map[string]any{
		"a contact email that is not one":  {"contact_email": "not-an-email"},
		"clicked_url without broadcast_id": {"clicked_url": "https://acme.com"},
		"an unknown status":                {"status": "sent"},
		"an unknown origin":                {"origin": "api"},
		"a date that is not a time":        {"sent_after": "2026-11-01"},
		"a range in the wrong order":       {"sent_after": "2026-11-30T00:00:00Z", "sent_before": "2026-11-01T00:00:00Z"},
		"limit above 50":                   {"limit": 51},
		"a cursor the API cannot read":     {"cursor": unreadable},
		"a cursor of three parts":          {"cursor": tooManyParts},
		"a channel argument":               {"channel": "email"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("search_messages", args).errorOf(t).Code)
		})
	}
}

func TestSearchMessages_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(messagesReadPermission())
	message := fullMessage()
	message.StatusInfo = strPtr(strings.Repeat("s", 2001))
	message.ChannelOptions.Subject = strPtr("Hi\x07 " + strings.Repeat("x", 2000))
	f.messages.EXPECT().ListMessages(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(&domain.MessageListResult{Messages: []*domain.MessageHistory{message}}, nil)

	var out struct {
		Messages []struct {
			StatusInfo      string   `json:"status_info"`
			Subject         string   `json:"subject"`
			Truncated       bool     `json:"truncated"`
			TruncatedFields []string `json:"truncated_fields"`
		} `json:"messages"`
	}
	f.callTool("search_messages", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
	require.Len(t, out.Messages, 1)
	assert.Equal(t, strings.Repeat("s", 2000), out.Messages[0].StatusInfo)
	assert.True(t, strings.HasPrefix(out.Messages[0].Subject, "Hi x"), "the control character is removed")
	assert.True(t, out.Messages[0].Truncated)
	assert.Equal(t, []string{"status_info", "subject"}, out.Messages[0].TruncatedFields)
}

func TestSearchMessages_Errors(t *testing.T) {
	t.Run("a key without message_history:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Write: true}})
		toolErr := f.callTool("search_messages", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the message_history read permission", Resource: "message_history", Permission: "read"}, toolErr)
	})

	t.Run("a failed read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(messagesReadPermission())
		f.messages.EXPECT().ListMessages(gomock.Any(), testWorkspaceID, gomock.Any()).Return(nil, errors.New("invalid cursor encoding: pq"))
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"},
			f.callTool("search_messages", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
	})
}
