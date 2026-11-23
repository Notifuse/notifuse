package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Notifuse/notifuse/internal/domain"
)

const searchContactsName = "search_contacts"

// searchContactsTool pages through a workspace's contacts, newest first, with
// the search and filters of contacts.list.
func (s *Server) searchContactsTool() *Tool {
	return &Tool{
		Name:  searchContactsName,
		Title: "Search contacts",
		Description: "Finds contacts in a workspace, newest first, by text search, list, list status or segment. Returns a summary of each contact. " +
			"get_contact has every field. To get the next page, pass next_cursor back as cursor with the same filters.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"search":       map[string]any{"type": "string", "description": "Part of the email, first name, last name, full name or external ID to match, case-insensitive. At most 100 characters."},
				"list_id":      map[string]any{"type": "string", "description": "Only contacts on this list (removed memberships excluded)."},
				"list_status": map[string]any{
					"type":        "string",
					"enum":        []string{"active", "pending", "unsubscribed", "bounced", "complained"},
					"description": "Only contacts with this status on a list. With list_id, the status must be on that list.",
				},
				"segment_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Only contacts in at least one of these segments."},
				"include_lists": map[string]any{"type": "boolean", "description": "Include each contact's list memberships. Default false."},
				"limit":         map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":        map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same filters."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.searchContacts,
	}
}

type searchContactsArgs struct {
	WorkspaceID  string   `json:"workspace_id"`
	Search       string   `json:"search"`
	ListID       string   `json:"list_id"`
	ListStatus   string   `json:"list_status"`
	SegmentIDs   []string `json:"segment_ids"`
	IncludeLists bool     `json:"include_lists"`
	Limit        *int     `json:"limit"`
	Cursor       string   `json:"cursor"`
}

// filters are the arguments a cursor is bound to, empty ones left out so a
// page that spells a default differently still matches.
func (a *searchContactsArgs) filters() map[string]any {
	filters := map[string]any{}
	if a.Search != "" {
		filters["search"] = a.Search
	}
	if a.ListID != "" {
		filters["list_id"] = a.ListID
	}
	if a.ListStatus != "" {
		filters["list_status"] = a.ListStatus
	}
	if len(a.SegmentIDs) > 0 {
		filters["segment_ids"] = a.SegmentIDs
	}
	if a.IncludeLists {
		filters["include_lists"] = true
	}
	return filters
}

type contactListSummary struct {
	ListID   string `json:"list_id"`
	ListName string `json:"list_name"`
	Status   string `json:"status"`
}

// contactSummary is the projection search_contacts returns. email_hmac signs
// notification-center links, so it is never part of it.
type contactSummary struct {
	Email      string                `json:"email"`
	ExternalID string                `json:"external_id,omitempty"`
	FirstName  string                `json:"first_name,omitempty"`
	LastName   string                `json:"last_name,omitempty"`
	FullName   string                `json:"full_name,omitempty"`
	Language   string                `json:"language,omitempty"`
	Timezone   string                `json:"timezone,omitempty"`
	Country    string                `json:"country,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	SegmentIDs []string              `json:"segment_ids"`
	Lists      *[]contactListSummary `json:"lists,omitempty"`
	Truncation
}

type searchContactsOutput struct {
	Workspace  WorkspaceRef     `json:"workspace"`
	Contacts   []contactSummary `json:"contacts"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (s *Server) searchContacts(ctx context.Context, call *Call) (any, error) {
	var args searchContactsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	args.Search = strings.TrimSpace(args.Search)
	if !utf8.ValidString(args.Search) || strings.ContainsRune(args.Search, 0) {
		return nil, validationError("search must be valid UTF-8 text")
	}
	if utf8.RuneCountInString(args.Search) > domain.ContactSearchMaxLength {
		return nil, validationError("search must be at most %d characters", domain.ContactSearchMaxLength)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}

	filters := args.filters()
	var apiCursor string
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, searchContactsName, call.WorkspaceID, filters, &apiCursor); toolErr != nil {
			return nil, toolErr
		}
		// contacts.list answers 500 on a cursor it cannot read, so it is read here
		// first.
		if !validContactsCursor(apiCursor) {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	// The request is decoded from JSON, as every service request built from
	// tool arguments is.
	body, err := json.Marshal(map[string]any{
		"workspace_id":        call.WorkspaceID,
		"search":              args.Search,
		"list_id":             args.ListID,
		"contact_list_status": args.ListStatus,
		"segments":            args.SegmentIDs,
		"with_contact_lists":  args.IncludeLists,
		"limit":               limit,
		"cursor":              apiCursor,
	})
	if err != nil {
		return nil, err
	}
	var req domain.GetContactsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}

	resp, err := s.deps.Contacts.GetContacts(ctx, &req)
	if err != nil {
		return nil, err
	}

	out := searchContactsOutput{Workspace: call.WorkspaceRef(), Contacts: make([]contactSummary, 0, len(resp.Contacts))}
	for _, contact := range resp.Contacts {
		out.Contacts = append(out.Contacts, summarizeContact(contact, args.IncludeLists))
	}
	if resp.NextCursor != "" {
		next, err := EncodeCursor(searchContactsName, call.WorkspaceID, filters, resp.NextCursor)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

// validContactsCursor reads contacts.list's own cursor: base64 of a
// RFC 3339 time and an email joined by "~".
func validContactsCursor(cursor string) bool {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), "~", 2)
	if len(parts) != 2 {
		return false
	}
	_, err = time.Parse(time.RFC3339Nano, parts[0])
	return err == nil
}

func summarizeContact(contact *domain.Contact, includeLists bool) contactSummary {
	summary := contactSummary{
		CreatedAt:  contact.CreatedAt,
		UpdatedAt:  contact.UpdatedAt,
		SegmentIDs: []string{},
	}
	text := func(path string, value *domain.NullableString) string {
		if value == nil || value.IsNull {
			return ""
		}
		return summary.Text(path, value.String)
	}
	summary.Email = summary.Text("email", contact.Email)
	summary.ExternalID = text("external_id", contact.ExternalID)
	summary.FirstName = text("first_name", contact.FirstName)
	summary.LastName = text("last_name", contact.LastName)
	summary.FullName = text("full_name", contact.FullName)
	summary.Language = text("language", contact.Language)
	summary.Timezone = text("timezone", contact.Timezone)
	summary.Country = text("country", contact.Country)
	for _, segment := range contact.ContactSegments {
		if segment != nil {
			summary.SegmentIDs = append(summary.SegmentIDs, segment.SegmentID)
		}
	}
	if includeLists {
		lists := make([]contactListSummary, 0, len(contact.ContactLists))
		for _, membership := range contact.ContactLists {
			if membership == nil {
				continue
			}
			lists = append(lists, contactListSummary{
				ListID:   membership.ListID,
				ListName: summary.Text(fmt.Sprintf("lists[%d].list_name", len(lists)), membership.ListName),
				Status:   string(membership.Status),
			})
		}
		summary.Lists = &lists
	}
	return summary
}

// getContactTool reads one contact whole, with its list memberships and
// segments.
func (s *Server) getContactTool() *Tool {
	return &Tool{
		Name:  "get_contact",
		Title: "Get contact",
		Description: "Reads one contact with every field, its list memberships (list, status and dates), and the IDs of the segments it is in. " +
			"Give either email or external_id, not both. external_id is not unique: when several contacts share one, the first match is returned. " +
			"Long text fields are cut at 2000 characters and named in truncated_fields. A custom JSON field longer than that is null.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The contact's email address. Give email or external_id, not both."},
				"external_id":  map[string]any{"type": "string", "description": "The contact's ID in your own system, matched exactly. Give email or external_id, not both."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.getContact,
	}
}

type getContactArgs struct {
	Email      string `json:"email"`
	ExternalID string `json:"external_id"`
}

type contactMembership struct {
	ListID    string    `json:"list_id"`
	ListName  string    `json:"list_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// contactFull is the projection get_contact returns: every field of the
// contact but email_hmac, which signs notification-center links. Null fields
// are left out.
type contactFull struct {
	Email        string `json:"email"`
	ExternalID   string `json:"external_id,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	FullName     string `json:"full_name,omitempty"`
	Language     string `json:"language,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
	Country      string `json:"country,omitempty"`
	Phone        string `json:"phone,omitempty"`
	AddressLine1 string `json:"address_line_1,omitempty"`
	AddressLine2 string `json:"address_line_2,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	State        string `json:"state,omitempty"`
	JobTitle     string `json:"job_title,omitempty"`

	CustomString1 string `json:"custom_string_1,omitempty"`
	CustomString2 string `json:"custom_string_2,omitempty"`
	CustomString3 string `json:"custom_string_3,omitempty"`
	CustomString4 string `json:"custom_string_4,omitempty"`
	CustomString5 string `json:"custom_string_5,omitempty"`

	CustomNumber1 *float64 `json:"custom_number_1,omitempty"`
	CustomNumber2 *float64 `json:"custom_number_2,omitempty"`
	CustomNumber3 *float64 `json:"custom_number_3,omitempty"`
	CustomNumber4 *float64 `json:"custom_number_4,omitempty"`
	CustomNumber5 *float64 `json:"custom_number_5,omitempty"`

	CustomDatetime1 *time.Time `json:"custom_datetime_1,omitempty"`
	CustomDatetime2 *time.Time `json:"custom_datetime_2,omitempty"`
	CustomDatetime3 *time.Time `json:"custom_datetime_3,omitempty"`
	CustomDatetime4 *time.Time `json:"custom_datetime_4,omitempty"`
	CustomDatetime5 *time.Time `json:"custom_datetime_5,omitempty"`

	// A custom JSON field cut by the cap is null rather than absent.
	CustomJSON1 json.RawMessage `json:"custom_json_1,omitempty"`
	CustomJSON2 json.RawMessage `json:"custom_json_2,omitempty"`
	CustomJSON3 json.RawMessage `json:"custom_json_3,omitempty"`
	CustomJSON4 json.RawMessage `json:"custom_json_4,omitempty"`
	CustomJSON5 json.RawMessage `json:"custom_json_5,omitempty"`

	TrackingOpensConsent     string     `json:"tracking_opens_consent,omitempty"`
	TrackingClicksConsent    string     `json:"tracking_clicks_consent,omitempty"`
	TrackingConsentSource    string     `json:"tracking_consent_source,omitempty"`
	TrackingConsentUpdatedAt *time.Time `json:"tracking_consent_updated_at,omitempty"`

	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
	Lists      []contactMembership `json:"lists"`
	SegmentIDs []string            `json:"segment_ids"`
	Truncation
}

type getContactOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Contact   contactFull  `json:"contact"`
}

func (s *Server) getContact(ctx context.Context, call *Call) (any, error) {
	var args getContactArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	email := domain.NormalizeEmail(args.Email)
	if (email == "") == (args.ExternalID == "") {
		return nil, validationError("give either email or external_id, not both")
	}

	var contact *domain.Contact
	var err error
	if email != "" {
		contact, err = s.deps.Contacts.GetContactByEmail(ctx, call.WorkspaceID, email)
	} else {
		contact, err = s.deps.Contacts.GetContactByExternalID(ctx, call.WorkspaceID, args.ExternalID)
	}
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, domain.ErrContactNotFound
	}
	return getContactOutput{Workspace: call.WorkspaceRef(), Contact: projectContact(contact)}, nil
}

func projectContact(contact *domain.Contact) contactFull {
	out := contactFull{
		CreatedAt:  contact.CreatedAt,
		UpdatedAt:  contact.UpdatedAt,
		Lists:      []contactMembership{},
		SegmentIDs: []string{},
	}
	text := func(path string, value *domain.NullableString) string {
		if value == nil || value.IsNull {
			return ""
		}
		return out.Text(path, value.String)
	}
	number := func(value *domain.NullableFloat64) *float64 {
		if value == nil || value.IsNull {
			return nil
		}
		n := value.Float64
		return &n
	}
	datetime := func(value *domain.NullableTime) *time.Time {
		if value == nil || value.IsNull {
			return nil
		}
		t := value.Time
		return &t
	}
	jsonField := func(path string, value *domain.NullableJSON) json.RawMessage {
		if value == nil || value.IsNull || value.Data == nil {
			return nil
		}
		data, err := json.Marshal(out.JSON(path, value.Data))
		if err != nil {
			return json.RawMessage("null")
		}
		return data
	}

	out.Email = out.Text("email", contact.Email)
	out.ExternalID = text("external_id", contact.ExternalID)
	out.FirstName = text("first_name", contact.FirstName)
	out.LastName = text("last_name", contact.LastName)
	out.FullName = text("full_name", contact.FullName)
	out.Language = text("language", contact.Language)
	out.Timezone = text("timezone", contact.Timezone)
	out.Country = text("country", contact.Country)
	out.Phone = text("phone", contact.Phone)
	out.AddressLine1 = text("address_line_1", contact.AddressLine1)
	out.AddressLine2 = text("address_line_2", contact.AddressLine2)
	out.Postcode = text("postcode", contact.Postcode)
	out.State = text("state", contact.State)
	out.JobTitle = text("job_title", contact.JobTitle)

	out.CustomString1 = text("custom_string_1", contact.CustomString1)
	out.CustomString2 = text("custom_string_2", contact.CustomString2)
	out.CustomString3 = text("custom_string_3", contact.CustomString3)
	out.CustomString4 = text("custom_string_4", contact.CustomString4)
	out.CustomString5 = text("custom_string_5", contact.CustomString5)

	out.CustomNumber1 = number(contact.CustomNumber1)
	out.CustomNumber2 = number(contact.CustomNumber2)
	out.CustomNumber3 = number(contact.CustomNumber3)
	out.CustomNumber4 = number(contact.CustomNumber4)
	out.CustomNumber5 = number(contact.CustomNumber5)

	out.CustomDatetime1 = datetime(contact.CustomDatetime1)
	out.CustomDatetime2 = datetime(contact.CustomDatetime2)
	out.CustomDatetime3 = datetime(contact.CustomDatetime3)
	out.CustomDatetime4 = datetime(contact.CustomDatetime4)
	out.CustomDatetime5 = datetime(contact.CustomDatetime5)

	out.CustomJSON1 = jsonField("custom_json_1", contact.CustomJSON1)
	out.CustomJSON2 = jsonField("custom_json_2", contact.CustomJSON2)
	out.CustomJSON3 = jsonField("custom_json_3", contact.CustomJSON3)
	out.CustomJSON4 = jsonField("custom_json_4", contact.CustomJSON4)
	out.CustomJSON5 = jsonField("custom_json_5", contact.CustomJSON5)

	out.TrackingOpensConsent = text("tracking_opens_consent", contact.TrackingOpensConsent)
	out.TrackingClicksConsent = text("tracking_clicks_consent", contact.TrackingClicksConsent)
	out.TrackingConsentSource = text("tracking_consent_source", contact.TrackingConsentSource)
	out.TrackingConsentUpdatedAt = datetime(contact.TrackingConsentUpdatedAt)

	for _, membership := range contact.ContactLists {
		if membership == nil {
			continue
		}
		out.Lists = append(out.Lists, contactMembership{
			ListID:    membership.ListID,
			ListName:  out.Text(fmt.Sprintf("lists[%d].list_name", len(out.Lists)), membership.ListName),
			Status:    string(membership.Status),
			CreatedAt: membership.CreatedAt,
			UpdatedAt: membership.UpdatedAt,
		})
	}
	for _, segment := range contact.ContactSegments {
		if segment != nil {
			out.SegmentIDs = append(out.SegmentIDs, segment.SegmentID)
		}
	}
	return out
}

// countContactsTool counts every contact of a workspace.
func (s *Server) countContactsTool() *Tool {
	return &Tool{
		Name:  "count_contacts",
		Title: "Count contacts",
		Description: "Counts all contacts in a workspace. It takes no filter. " +
			"list_lists gives the subscriber counts of each list, and segments count the contacts that match conditions.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.countContacts,
	}
}

type countContactsOutput struct {
	Workspace     WorkspaceRef `json:"workspace"`
	TotalContacts int          `json:"total_contacts"`
}

func (s *Server) countContacts(ctx context.Context, call *Call) (any, error) {
	total, err := s.deps.Contacts.CountContacts(ctx, call.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return countContactsOutput{Workspace: call.WorkspaceRef(), TotalContacts: total}, nil
}
