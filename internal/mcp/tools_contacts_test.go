package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
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

func TestUpsertContact_Definition(t *testing.T) {
	tool := catalogueTool(t, "upsert_contact")
	assertWriteTool(t, tool, "upsert_contact", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		[]Pair{{Resource: "contacts", Verb: "write"}}, "contacts.upsert", []string{"workspace_id", "email"})
	properties := tool.InputSchema["properties"].(map[string]any)
	clearable := properties["clear_fields"].(map[string]any)["items"].(map[string]any)["enum"]
	assert.Equal(t, []string{
		"external_id", "first_name", "last_name", "full_name", "language", "timezone", "phone",
		"address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
		"tracking_opens_consent", "tracking_clicks_consent",
	}, clearable)
	for _, notExposed := range []string{"tracking_consent_source", "tracking_consent_updated_at", "email_hmac", "created_at"} {
		assert.NotContains(t, properties, notExposed)
	}
}

// The contact is decoded from the arguments as contacts.upsert decodes its
// body: a given field is set, a cleared one is null, an absent one is left
// nil so the stored value stays.
func TestUpsertContact_BuildsTheContactFromTheGivenKeysOnly(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	var sent *domain.Contact
	f.contacts.EXPECT().UpsertContact(gomock.Any(), testWorkspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, contact *domain.Contact) domain.UpsertContactOperation {
			sent = contact
			return domain.UpsertContactOperation{Email: contact.Email, Action: domain.UpsertContactOperationUpdate, Contact: fullContact()}
		})

	result := f.callTool("upsert_contact", map[string]any{
		"workspace_id":    testWorkspaceID,
		"email":           " Jane@Example.COM ",
		"first_name":      "Jane",
		"custom_number_2": 4.5,
		"custom_json_1":   "gold",
		"clear_fields":    []any{"phone", "custom_datetime_3"},
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	require.NotNil(t, sent)
	assert.Equal(t, "jane@example.com", sent.Email)
	assert.Equal(t, &domain.NullableString{String: "Jane"}, sent.FirstName)
	assert.Equal(t, &domain.NullableFloat64{Float64: 4.5}, sent.CustomNumber2)
	assert.Equal(t, &domain.NullableJSON{Data: "gold"}, sent.CustomJSON1)
	assert.Equal(t, &domain.NullableString{IsNull: true}, sent.Phone)
	assert.Equal(t, &domain.NullableTime{IsNull: true}, sent.CustomDatetime3)
	// Omitted, so kept as stored.
	assert.Nil(t, sent.LastName)
	assert.Nil(t, sent.Timezone)
	assert.Nil(t, sent.CustomString1)
	assert.Nil(t, sent.TrackingOpensConsent)
	assert.Nil(t, sent.TrackingConsentSource)
}

func TestUpsertContact_ReturnsTheStoredContact(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	f.contacts.EXPECT().UpsertContact(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{Email: "jane@example.com", Action: domain.UpsertContactOperationCreate, Contact: fullContact()})

	result := f.callTool("upsert_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"})
	var out struct {
		Workspace WorkspaceRef `json:"workspace"`
		Action    string       `json:"action"`
		Contact   contactFull  `json:"contact"`
	}
	result.decode(t, &out)
	assert.Equal(t, WorkspaceRef{ID: testWorkspaceID, Name: "Acme"}, out.Workspace)
	assert.Equal(t, "create", out.Action)
	assert.Equal(t, projectContact(fullContact()), out.Contact)
	assert.NotContains(t, result.Content[0].Text, "email_hmac")
	assert.NotContains(t, result.Content[0].Text, "signed-secret")
}

// The read-back is best effort in the service: a committed write without it
// still succeeds, with no contact.
func TestUpsertContact_WithoutTheReadBack(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	f.contacts.EXPECT().UpsertContact(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{Email: "jane@example.com", Action: domain.UpsertContactOperationUpdate})

	result := f.callTool("upsert_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"action":"update"}`, string(result.StructuredContent))
}

func TestUpsertContact_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	for name, args := range map[string]map[string]any{
		"invalid email":           {"email": "not-an-email"},
		"given and cleared":       {"email": "jane@example.com", "first_name": "Jane", "clear_fields": []any{"first_name"}},
		"unknown time zone":       {"email": "jane@example.com", "timezone": "Mars/Olympus"},
		"date not in RFC 3339":    {"email": "jane@example.com", "custom_datetime_1": "01/02/2026"},
		"consent off its enum":    {"email": "jane@example.com", "tracking_opens_consent": "maybe"},
		"clearing the email":      {"email": "jane@example.com", "clear_fields": []any{"email"}},
		"consent source not open": {"email": "jane@example.com", "tracking_consent_source": "import"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("upsert_contact", args).errorOf(t).Code)
		})
	}
}

func TestUpsertContact_OperationErrorsAreMapped(t *testing.T) {
	t.Run("a typed refusal", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(contactsWrite())
		f.captureAudit()
		f.contacts.EXPECT().UpsertContact(gomock.Any(), testWorkspaceID, gomock.Any()).Return(domain.UpsertContactOperation{
			Action: domain.UpsertContactOperationError,
			Error:  "no",
			Err:    domain.NewPermissionError(domain.PermissionResourceContacts, domain.PermissionTypeWrite, "no"),
		})
		toolErr := f.callTool("upsert_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
		assert.Equal(t, CodePermissionDenied, toolErr.Code)
	})

	// The arguments were validated first, so an error with no typed cause is
	// the write failing: internal, never validation, and its text is not sent.
	t.Run("a failed write", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(contactsWrite())
		f.captureAudit()
		f.contacts.EXPECT().UpsertContact(gomock.Any(), testWorkspaceID, gomock.Any()).Return(domain.UpsertContactOperation{
			Action: domain.UpsertContactOperationError,
			Error:  "pq: deadlock detected on contacts",
		})
		toolErr := f.callTool("upsert_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, toolErr)
	})
}

func TestUpsertContact_AuditRow(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	audit := f.captureAudit()
	f.contacts.EXPECT().UpsertContact(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(domain.UpsertContactOperation{Email: "jane@example.com", Action: domain.UpsertContactOperationCreate})

	require.False(t, f.callTool("upsert_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "Jane@Example.com", "first_name": "Jane"}).IsError)
	row := audit.last(t)
	assert.Equal(t, "contacts.upsert", row.Action)
	assert.Equal(t, domain.AuditCategoryContacts, row.Category)
	assert.Equal(t, domain.AuditTargetContact, row.TargetType)
	assert.Equal(t, "jane@example.com", row.TargetID)
	assert.Equal(t, http.StatusOK, row.StatusCode)
	assert.Equal(t, map[string]any{"channel": "mcp", "mcp_tool": "upsert_contact", "email": "jane@example.com", "action": "create"}, row.Metadata)

	assert.True(t, f.callTool("upsert_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "timezone": "Nowhere"}).IsError)
	row = audit.last(t)
	assert.Equal(t, http.StatusBadRequest, row.StatusCode)
	assert.Equal(t, "jane@example.com", row.TargetID)
	assert.Equal(t, "validation", row.Metadata["error_code"])
}

func TestDeleteContact_Definition(t *testing.T) {
	tool := catalogueTool(t, "delete_contact")
	assertWriteTool(t, tool, "delete_contact", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		[]Pair{{Resource: "contacts", Verb: "write"}}, "contacts.delete", []string{"workspace_id", "email"})
	// Erasing the opt-out hashes too stays a person's decision.
	assert.Equal(t, []string{"email", "workspace_id"}, sortedKeys(tool.InputSchema["properties"].(map[string]any)))
}

func TestDeleteContact_KeepsTheOptOutHashes(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	f.contactRows.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").Return(sampleContact(), nil)
	f.contacts.EXPECT().DeleteContact(gomock.Any(), testWorkspaceID, "jane@example.com", domain.DeleteContactOptions{ForgetOptOuts: false}).Return(nil)

	result := f.callTool("delete_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "Jane@Example.com"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"deleted":true,"email":"jane@example.com"}`, string(result.StructuredContent))

	toolErr := f.callTool("delete_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "forget_opt_outs": true}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
}

// The service answers success for an address with no contact; the tool reads
// first, so a repeated call answers not_found and deletes nothing.
func TestDeleteContact_ReplayIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	audit := f.captureAudit()
	f.contactRows.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").Return(nil, domain.ErrContactNotFound)

	toolErr := f.callTool("delete_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "contact not found"}, toolErr)
	row := audit.last(t)
	assert.Equal(t, "contacts.delete", row.Action)
	assert.Equal(t, http.StatusNotFound, row.StatusCode)
	assert.Equal(t, domain.AuditTargetContact, row.TargetType)
	assert.Equal(t, "jane@example.com", row.TargetID)
}

func TestDeleteContact_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	audit := f.captureAudit()
	f.contactRows.EXPECT().GetContactByEmail(gomock.Any(), testWorkspaceID, "jane@example.com").Return(sampleContact(), nil)
	f.contacts.EXPECT().DeleteContact(gomock.Any(), testWorkspaceID, "jane@example.com", domain.DeleteContactOptions{}).
		Return(errors.New("failed to delete queued emails: timeout"))

	toolErr := f.callTool("delete_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com"}).errorOf(t)
	assert.Equal(t, CodeInternal, toolErr.Code)
	assert.Equal(t, http.StatusInternalServerError, audit.last(t).StatusCode)

	assert.Equal(t, CodeValidation, f.callTool("delete_contact", map[string]any{"workspace_id": testWorkspaceID, "email": "nobody"}).errorOf(t).Code)
}

func TestTrackCustomEvent_Definition(t *testing.T) {
	tool := catalogueTool(t, "track_custom_event")
	assertWriteTool(t, tool, "track_custom_event", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		[]Pair{{Resource: "contacts", Verb: "write"}}, "customEvents.upsert", []string{"workspace_id", "email", "event_name", "external_id", "occurred_at"})
	// A soft delete through a write tool would dodge destructiveHint.
	assert.Equal(t, []string{"email", "event_name", "external_id", "goal_name", "goal_type", "goal_value", "occurred_at", "properties", "workspace_id"},
		sortedKeys(tool.InputSchema["properties"].(map[string]any)))
}

func sampleEvent() *domain.CustomEvent {
	occurred := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	goalType, goalValue := "purchase", 59.9
	integration := "shopify"
	return &domain.CustomEvent{
		ExternalID: "ord_42", Email: "jane@example.com", EventName: "order.completed",
		Properties: map[string]interface{}{"total": 59.9}, OccurredAt: occurred, Source: "api", IntegrationID: &integration,
		GoalType: &goalType, GoalValue: &goalValue, CreatedAt: occurred, UpdatedAt: occurred,
	}
}

func TestTrackCustomEvent_BuildsTheRequest(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	var sent *domain.UpsertCustomEventRequest
	f.events.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.UpsertCustomEventRequest) (*domain.CustomEvent, error) {
		sent = req
		return sampleEvent(), nil
	})

	result := f.callTool("track_custom_event", map[string]any{
		"workspace_id": testWorkspaceID, "email": "Jane@Example.com", "event_name": "order.completed", "external_id": "ord_42",
		"occurred_at": "2026-11-20T11:00:00+01:00", "goal_type": "purchase", "goal_value": 59.9,
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	require.NotNil(t, sent)
	occurred := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	require.NotNil(t, sent.OccurredAt)
	assert.True(t, occurred.Equal(*sent.OccurredAt))
	assert.Equal(t, testWorkspaceID, sent.WorkspaceID)
	assert.Equal(t, "jane@example.com", sent.Email)
	assert.Equal(t, "order.completed", sent.EventName)
	assert.Equal(t, "ord_42", sent.ExternalID)
	assert.Equal(t, strPtr("purchase"), sent.GoalType)
	// Properties left out keep the stored ones: nil, not an empty map.
	assert.Nil(t, sent.Properties)
	assert.Nil(t, sent.DeletedAt)
	assert.Nil(t, sent.IntegrationID)
	assert.Nil(t, sent.GoalName)
}

func TestTrackCustomEvent_ReturnsTheStoredEvent(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	f.events.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).Return(sampleEvent(), nil)

	result := f.callTool("track_custom_event", map[string]any{
		"workspace_id": testWorkspaceID, "email": "jane@example.com", "event_name": "order.completed", "external_id": "ord_42",
		"occurred_at": "2026-11-20T10:00:00Z", "properties": map[string]any{"total": 59.9},
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"event":{
		"event_name":"order.completed","external_id":"ord_42","email":"jane@example.com","occurred_at":"2026-11-20T10:00:00Z",
		"properties":{"total":59.9},"goal_type":"purchase","goal_value":59.9,
		"created_at":"2026-11-20T10:00:00Z","updated_at":"2026-11-20T10:00:00Z"
	}}`, string(result.StructuredContent))
}

func TestTrackCustomEvent_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	f.captureAudit()
	valid := func() map[string]any {
		return map[string]any{
			"workspace_id": testWorkspaceID, "email": "jane@example.com", "event_name": "order.completed",
			"external_id": "ord_42", "occurred_at": "2026-11-20T10:00:00Z",
		}
	}
	for name, change := range map[string]func(map[string]any){
		"no occurred_at":                  func(args map[string]any) { delete(args, "occurred_at") },
		"occurred_at not RFC 3339":        func(args map[string]any) { args["occurred_at"] = "yesterday" },
		"invalid email":                   func(args map[string]any) { args["email"] = "jane" },
		"purchase without a value":        func(args map[string]any) { args["goal_type"] = "purchase" },
		"goal type off its enum":          func(args map[string]any) { args["goal_type"] = "donation" },
		"external_id too long":            func(args map[string]any) { args["external_id"] = strings.Repeat("x", 256) },
		"a soft delete is not offered":    func(args map[string]any) { args["deleted_at"] = "2026-11-20T10:00:00Z" },
		"an integration is not offered":   func(args map[string]any) { args["integration_id"] = "shopify" },
		"properties that are not objects": func(args map[string]any) { args["properties"] = []any{1} },
	} {
		t.Run(name, func(t *testing.T) {
			args := valid()
			change(args)
			assert.Equal(t, CodeValidation, f.callTool("track_custom_event", args).errorOf(t).Code)
		})
	}
}

func TestTrackCustomEvent_ServiceValidationAndAudit(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsWrite())
	audit := f.captureAudit()
	f.events.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).
		Return(nil, domain.NewValidationError("invalid custom event: event_name must contain only lowercase letters"))

	toolErr := f.callTool("track_custom_event", map[string]any{
		"workspace_id": testWorkspaceID, "email": "jane@example.com", "event_name": "Order Completed",
		"external_id": "ord_42", "occurred_at": "2026-11-20T10:00:00Z",
	}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
	row := audit.last(t)
	assert.Equal(t, "customEvents.upsert", row.Action)
	assert.Equal(t, domain.AuditCategoryContacts, row.Category)
	assert.Equal(t, domain.AuditTargetContact, row.TargetType)
	assert.Equal(t, "jane@example.com", row.TargetID)
	assert.Equal(t, http.StatusBadRequest, row.StatusCode)
	assert.Equal(t, "jane@example.com", row.Metadata["email"])
	assert.Equal(t, "Order Completed", row.Metadata["event_name"])

	f.events.EXPECT().UpsertEvent(gomock.Any(), gomock.Any()).Return(sampleEvent(), nil)
	require.False(t, f.callTool("track_custom_event", map[string]any{
		"workspace_id": testWorkspaceID, "email": "jane@example.com", "event_name": "order.completed",
		"external_id": "ord_42", "occurred_at": "2026-11-20T10:00:00Z",
	}).IsError)
	assert.Equal(t, http.StatusOK, audit.last(t).StatusCode)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
