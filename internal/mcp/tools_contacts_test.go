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

func nullable(value string) *domain.NullableString {
	return &domain.NullableString{String: value}
}

func sampleContact() *domain.Contact {
	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return &domain.Contact{
		Email:      "jane@example.com",
		ExternalID: nullable("u_123"),
		FirstName:  nullable("Jane"),
		LastName:   nullable("Doe"),
		FullName:   &domain.NullableString{IsNull: true},
		Language:   nullable("en"),
		Timezone:   nullable("Europe/Paris"),
		Country:    nullable("FR"),
		Phone:      nullable("+33 6 00 00 00 00"),
		CreatedAt:  created,
		UpdatedAt:  created.Add(time.Hour),
		EmailHMAC:  "signed-secret",
		ContactLists: []*domain.ContactList{
			{Email: "jane@example.com", ListID: "news", ListName: "Newsletter", Status: domain.ContactListStatusActive, CreatedAt: created},
		},
		ContactSegments: []*domain.ContactSegment{{Email: "jane@example.com", SegmentID: "vip", Version: 3}},
	}
}

func TestSearchContacts_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Return(&domain.GetContactsResponse{Contacts: []*domain.Contact{sampleContact()}}, nil)

	result := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"contacts":[{
			"email":"jane@example.com","external_id":"u_123","first_name":"Jane","last_name":"Doe",
			"language":"en","timezone":"Europe/Paris","country":"FR",
			"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-01T10:00:00Z",
			"segment_ids":["vip"]
		}]
	}`, string(result.StructuredContent))
	assert.NotContains(t, result.Content[0].Text, "email_hmac")
	assert.NotContains(t, result.Content[0].Text, "signed-secret")
}

func TestSearchContacts_ListsOnlyWhenAsked(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	contact := sampleContact()
	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
			assert.True(t, req.WithContactLists)
			return &domain.GetContactsResponse{Contacts: []*domain.Contact{contact, {Email: "solo@example.com"}}}, nil
		})

	var out struct {
		Contacts []map[string]json.RawMessage `json:"contacts"`
	}
	f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID, "include_lists": true}).decode(t, &out)
	require.Len(t, out.Contacts, 2)
	assert.JSONEq(t, `[{"list_id":"news","list_name":"Newsletter","status":"active"}]`, string(out.Contacts[0]["lists"]))
	assert.JSONEq(t, `[]`, string(out.Contacts[1]["lists"]), "asked for and empty, rather than absent")
}

func TestSearchContacts_BuildsTheServiceRequest(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	var got *domain.GetContactsRequest
	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
			got = req
			return &domain.GetContactsResponse{}, nil
		}).Times(2)

	f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID})
	assert.Equal(t, &domain.GetContactsRequest{WorkspaceID: testWorkspaceID, Limit: 20}, got)

	f.callTool("search_contacts", map[string]any{
		"workspace_id": testWorkspaceID, "search": "  jane  ", "list_id": "news", "list_status": "unsubscribed",
		"segment_ids": []string{"vip"}, "limit": 50,
	})
	assert.Equal(t, &domain.GetContactsRequest{
		WorkspaceID: testWorkspaceID, Search: "jane", ListID: "news", ContactListStatus: "unsubscribed",
		Segments: []string{"vip"}, Limit: 50,
	}, got)
}

func TestSearchContacts_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())

	for name, args := range map[string]map[string]any{
		"limit above 50":  {"limit": 51},
		"limit of 0":      {"limit": 0},
		"long search":     {"search": strings.Repeat("a", 101)},
		"NUL in search":   {"search": "a\x00b"},
		"garbage cursor":  {"cursor": "%%%"},
		"limit not whole": {"limit": 2.5},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			toolErr := f.callTool("search_contacts", args).errorOf(t)
			assert.Equal(t, CodeValidation, toolErr.Code)
		})
	}
}

// next_cursor wraps the API's cursor; passed back with the same filters it
// reaches the service as the API cursor, and with other filters it is refused.
func TestSearchContacts_Cursor(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	apiCursor := base64.StdEncoding.EncodeToString([]byte("2026-10-01T09:00:00Z~jane@example.com"))

	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Return(&domain.GetContactsResponse{Contacts: []*domain.Contact{sampleContact()}, NextCursor: apiCursor}, nil)
	var first struct {
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID, "search": "jane"}).decode(t, &first)
	require.NotEmpty(t, first.NextCursor)
	assert.NotEqual(t, apiCursor, first.NextCursor, "the API cursor is wrapped")

	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
			assert.Equal(t, apiCursor, req.Cursor)
			return &domain.GetContactsResponse{}, nil
		})
	var last map[string]json.RawMessage
	f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID, "search": "jane", "cursor": first.NextCursor}).decode(t, &last)
	assert.NotContains(t, last, "next_cursor", "absent on the last page")

	toolErr := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID, "search": "john", "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
}

// contacts.list answers 500 on a cursor it cannot read, so a well-formed
// envelope around a bad API cursor is refused before the service.
func TestSearchContacts_BadInnerCursorNeverReachesTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	for _, inner := range []string{"not base64!", base64.StdEncoding.EncodeToString([]byte("no-separator")), base64.StdEncoding.EncodeToString([]byte("yesterday~jane@example.com"))} {
		cursor, err := EncodeCursor("search_contacts", testWorkspaceID, map[string]any{}, inner)
		require.NoError(t, err)
		toolErr := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID, "cursor": cursor}).errorOf(t)
		assert.Equal(t, CodeValidation, toolErr.Code, inner)
	}
}

func TestSearchContacts_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	contact := sampleContact()
	contact.FirstName = nullable(strings.Repeat("J", 2500))
	contact.LastName = nullable("Do\x1be")
	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Return(&domain.GetContactsResponse{Contacts: []*domain.Contact{contact}}, nil)

	var out struct {
		Contacts []struct {
			FirstName       string   `json:"first_name"`
			LastName        string   `json:"last_name"`
			Truncated       bool     `json:"truncated"`
			TruncatedFields []string `json:"truncated_fields"`
		} `json:"contacts"`
	}
	f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
	require.Len(t, out.Contacts, 1)
	assert.Equal(t, strings.Repeat("J", 2000), out.Contacts[0].FirstName)
	assert.Equal(t, "Doe", out.Contacts[0].LastName)
	assert.True(t, out.Contacts[0].Truncated)
	assert.Equal(t, []string{"first_name"}, out.Contacts[0].TruncatedFields)
}

func TestSearchContacts_ServiceRefusalsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		Return(nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Insufficient permissions: read access to contacts required"))

	toolErr := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
	assert.Equal(t, "contacts", toolErr.Resource)
}

func TestSearchContacts_UnreachableWorkspaceIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.switchedOff()
	toolErr := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "workspace not found"}, toolErr)
}

// contacts.list joins the time and the email with "~", and an email may hold
// one: only the first "~" separates them.
func TestValidContactsCursor_EmailWithATilde(t *testing.T) {
	cursor := base64.StdEncoding.EncodeToString([]byte("2026-11-23T10:00:00Z~a~b@example.com"))
	assert.True(t, validContactsCursor(cursor))
	assert.False(t, validContactsCursor(base64.StdEncoding.EncodeToString([]byte("no-separator"))))
}

func TestGetContact_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "get_contact"), "get_contact", []Pair{{Resource: "contacts", Verb: "read"}}, []string{"workspace_id"})
}

// fullContact sets every kind of field get_contact projects.
func fullContact() *domain.Contact {
	contact := sampleContact()
	at := time.Date(2026, 11, 2, 8, 30, 0, 0, time.UTC)
	contact.AddressLine1 = nullable("1 rue de Rivoli")
	contact.AddressLine2 = &domain.NullableString{IsNull: true}
	contact.JobTitle = nullable("CTO")
	contact.CustomString1 = nullable("pro")
	contact.CustomNumber1 = &domain.NullableFloat64{Float64: 12.5}
	contact.CustomNumber2 = &domain.NullableFloat64{IsNull: true}
	contact.CustomDatetime1 = &domain.NullableTime{Time: at}
	contact.CustomJSON1 = &domain.NullableJSON{Data: map[string]any{"plan": "pro", "seats": float64(3)}}
	contact.CustomJSON2 = &domain.NullableJSON{IsNull: true}
	contact.TrackingOpensConsent = nullable("granted")
	contact.TrackingConsentUpdatedAt = &domain.NullableTime{Time: at}
	return contact
}

func TestGetContact_ByEmailProjection(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").Return(fullContact(), nil)

	result := f.callTool("get_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "  Jane@Example.COM "})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"contact":{
			"email":"jane@example.com","external_id":"u_123","first_name":"Jane","last_name":"Doe",
			"language":"en","timezone":"Europe/Paris","country":"FR","phone":"+33 6 00 00 00 00",
			"address_line_1":"1 rue de Rivoli","job_title":"CTO",
			"custom_string_1":"pro","custom_number_1":12.5,"custom_datetime_1":"2026-11-02T08:30:00Z",
			"custom_json_1":{"plan":"pro","seats":3},
			"tracking_opens_consent":"granted","tracking_consent_updated_at":"2026-11-02T08:30:00Z",
			"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-01T10:00:00Z",
			"lists":[{"list_id":"news","list_name":"Newsletter","status":"active","created_at":"2026-10-01T09:00:00Z","updated_at":"0001-01-01T00:00:00Z"}],
			"segment_ids":["vip"]
		}
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"email_hmac", "signed-secret", "deleted_at", "contact_lists", "contact_segments", "address_line_2", "custom_json_2"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestGetContact_ByExternalID(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	// Matched exactly, so neither trimmed nor lower-cased.
	f.contacts.EXPECT().GetContactByExternalID(gomock.Any(), testWorkspaceID, " U_123").Return(sampleContact(), nil)

	var out struct {
		Contact struct {
			Email string `json:"email"`
		} `json:"contact"`
	}
	f.callTool("get_contact", map[string]any{"workspace_id": testWorkspaceID, "external_id": " U_123"}).decode(t, &out)
	assert.Equal(t, "jane@example.com", out.Contact.Email)
}

func TestGetContact_EmailOrExternalIDNotBoth(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	for name, args := range map[string]map[string]any{
		"neither":      {},
		"both":         {"email": "jane@example.com", "external_id": "u_123"},
		"blank email":  {"email": "   "},
		"empty values": {"email": "", "external_id": ""},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("get_contact", args).errorOf(t).Code)
		})
	}
}

func TestGetContact_UnknownContactIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "ghost@example.com").Return(nil, domain.ErrContactNotFound)
	f.contacts.EXPECT().GetContactByExternalID(gomock.Any(), testWorkspaceID, "ghost").Return(nil, domain.ErrContactNotFound)

	for _, args := range []map[string]any{{"email": "ghost@example.com"}, {"external_id": "ghost"}} {
		args["workspace_id"] = testWorkspaceID
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "contact not found"}, f.callTool("get_contact", args).errorOf(t))
	}
}

func TestGetContact_CapsFreeTextAndLongJSON(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	contact := fullContact()
	contact.JobTitle = nullable(strings.Repeat("t", 2001))
	contact.CustomJSON3 = &domain.NullableJSON{Data: map[string]any{"blob": strings.Repeat("x", 3000)}}
	contact.ContactLists[0].ListName = strings.Repeat("n", 2500)
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").Return(contact, nil)

	var out struct {
		Contact map[string]json.RawMessage `json:"contact"`
	}
	f.callTool("get_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).decode(t, &out)
	assert.Equal(t, `"`+strings.Repeat("t", 2000)+`"`, string(out.Contact["job_title"]))
	assert.Equal(t, "null", string(out.Contact["custom_json_3"]), "a JSON field too long to cut is null")
	assert.JSONEq(t, `{"plan":"pro","seats":3}`, string(out.Contact["custom_json_1"]))
	assert.Equal(t, "true", string(out.Contact["truncated"]))
	assert.JSONEq(t, `["job_title","custom_json_3","lists[0].list_name"]`, string(out.Contact["truncated_fields"]))
}

func TestGetContact_ServiceRefusalIsMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").
		Return(nil, domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeRead, "Insufficient permissions: read access to contacts required"))
	toolErr := f.callTool("get_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
	assert.Equal(t, "contacts", toolErr.Resource)
}

func TestGetContact_PrecheckRefusesWithoutContactsRead(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceContacts: {Write: true}})
	toolErr := f.callTool("get_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the contacts read permission", Resource: "contacts", Permission: "read"}, toolErr)
}

func TestCountContacts_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "count_contacts"), "count_contacts", []Pair{{Resource: "contacts", Verb: "read"}}, []string{"workspace_id"})
}

func TestCountContacts(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().CountContacts(gomock.Any(), testWorkspaceID).Return(48210, nil)

	result := f.callTool("count_contacts", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"total_contacts":48210}`, string(result.StructuredContent))
}

func TestCountContacts_TakesNoFilter(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	toolErr := f.callTool("count_contacts", map[string]any{"workspace_id": testWorkspaceID, "list_id": "news"}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
}

func TestCountContacts_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().CountContacts(gomock.Any(), testWorkspaceID).Return(0, errors.New("failed to count contacts: pq: timeout on db-host-7"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("count_contacts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))

	g := newFixture(t)
	g.reachable(domain.UserPermissions{})
	assert.Equal(t, CodePermissionDenied, g.callTool("count_contacts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t).Code)
}
