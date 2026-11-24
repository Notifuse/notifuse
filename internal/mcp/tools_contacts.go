package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"

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

// contactTextFields are the text fields of a contact that upsert_contact
// writes, in the order of its schema.
var contactTextFields = []string{
	"external_id", "first_name", "last_name", "full_name", "language", "timezone", "phone",
	"address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
	"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
}

var (
	contactNumberFields   = []string{"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5"}
	contactDatetimeFields = []string{"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5"}
	contactJSONFields     = []string{"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5"}
	contactConsentFields  = []string{"tracking_opens_consent", "tracking_clicks_consent"}
)

// contactClearableFields are the fields clear_fields may name: every field
// upsert_contact writes but the email.
func contactClearableFields() []string {
	return slices.Concat(contactTextFields, contactNumberFields, contactDatetimeFields, contactJSONFields, contactConsentFields)
}

// emailArgument normalizes an email argument and checks its form, before any
// service call: several paths compare addresses verbatim.
func emailArgument(raw string) (string, *ToolError) {
	email := domain.NormalizeEmail(raw)
	if email == "" || !govalidator.IsEmail(email) {
		return "", validationError("email must be a valid email address")
	}
	return email, nil
}

func (s *Server) upsertContactTool() *Tool {
	descriptions := map[string]string{
		"external_id": "Your own ID for the contact.",
		"language":    "Language code, for example en or fr.",
		"timezone":    "IANA time zone name, for example Europe/Paris.",
		"country":     "Country code, for example FR.",
	}
	properties := map[string]any{
		"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
		"email":        map[string]any{"type": "string", "description": "The contact's email address. A contact is created when none has it."},
	}
	for _, field := range contactTextFields {
		property := map[string]any{"type": "string"}
		if description, ok := descriptions[field]; ok {
			property["description"] = description
		}
		properties[field] = property
	}
	for _, field := range contactNumberFields {
		properties[field] = map[string]any{"type": "number"}
	}
	for _, field := range contactDatetimeFields {
		properties[field] = map[string]any{"type": "string", "description": "RFC 3339 date and time."}
	}
	for _, field := range contactJSONFields {
		properties[field] = map[string]any{"description": "Any JSON value."}
	}
	for _, field := range contactConsentFields {
		properties[field] = map[string]any{"type": "string", "enum": []string{"granted", "denied"}}
	}
	properties["clear_fields"] = map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string", "enum": contactClearableFields()},
		"description": "Fields to empty. A field cannot be both given and cleared.",
	}
	return &Tool{
		Name:  "upsert_contact",
		Title: "Create or update contact",
		Description: "Creates a contact, or updates the one with this email. " +
			"Only the fields you give change; the others keep their stored value. Use clear_fields to empty a field. " +
			"get_workspace_overview gives the labels of the custom fields in custom_field_labels. " +
			"Creating or changing a contact can start automations, which may send emails. " +
			"Calling it again with the same arguments changes nothing more. " +
			"Returns create or update, and the contact as stored.",
		Class:       ClassWriteOpenWorld,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite}},
		AuditAction: "contacts.upsert",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"workspace_id", "email"},
			"additionalProperties": false,
		},
		Handler: s.upsertContact,
	}
}

type upsertContactOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Action    string       `json:"action"`
	Contact   *contactFull `json:"contact,omitempty"`
}

func (s *Server) upsertContact(ctx context.Context, call *Call) (any, error) {
	rawEmail, _ := call.Args["email"].(string)
	email, toolErr := emailArgument(rawEmail)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.SetTarget(domain.AuditTargetContact, email, "")
	call.Audit.AddMetadata("email", email)

	// The contact is decoded from a JSON object, as contacts.upsert decodes
	// its body: a given field is written, a null one emptied, an absent one
	// kept.
	fields := make(map[string]any, len(call.Args))
	for key, value := range call.Args {
		if key != "workspace_id" && key != "clear_fields" {
			fields[key] = value
		}
	}
	fields["email"] = email
	cleared, _ := call.Args["clear_fields"].([]any)
	for _, item := range cleared {
		field, _ := item.(string)
		if _, given := fields[field]; given {
			return nil, validationError("%s is both given and in clear_fields", field)
		}
		fields[field] = nil
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	contact, err := domain.FromJSON(data)
	if err != nil {
		return nil, validationError("%v", err)
	}
	if err := contact.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	// The API stores any time zone text; a contact's sends are scheduled by it.
	if contact.Timezone != nil && !contact.Timezone.IsNull && !domain.IsValidTimezone(contact.Timezone.String) {
		return nil, validationError("timezone must be an IANA time zone name, for example Europe/Paris")
	}

	operation := s.deps.Contacts.UpsertContact(ctx, call.WorkspaceID, contact)
	if operation.Err != nil {
		return nil, operation.Err
	}
	if operation.Action == domain.UpsertContactOperationError {
		// The arguments were validated above, so what is left is a failure of
		// the write, which the operation reports as text only.
		return nil, errors.New("upsert failed")
	}
	call.Audit.AddMetadata("action", operation.Action)
	out := upsertContactOutput{Workspace: call.WorkspaceRef(), Action: operation.Action}
	if operation.Contact != nil {
		projected := projectContact(operation.Contact)
		out.Contact = &projected
	}
	return out, nil
}

func (s *Server) deleteContactTool() *Tool {
	return &Tool{
		Name:  "delete_contact",
		Title: "Delete contact",
		Description: "Deletes a contact and everything about it: its message history, timeline, list memberships, custom events, automation journeys and queued emails. " +
			"Its web analytics visits are kept without the address. " +
			"The contact's unsubscribes, bounces and complaints are kept as hashes, so a later import does not subscribe it again. " +
			"This cannot be undone: confirm the address with the user first. " +
			"Returns not_found when no contact has this email, which is also what a repeated call answers.",
		Class:       ClassDestructive,
		Pairs:       []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite}},
		AuditAction: "contacts.delete",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The email address of the contact to delete."},
			},
			"required":             []string{"workspace_id", "email"},
			"additionalProperties": false,
		},
		Handler: s.deleteContact,
	}
}

type deleteContactOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Deleted   bool         `json:"deleted"`
	Email     string       `json:"email"`
}

func (s *Server) deleteContact(ctx context.Context, call *Call) (any, error) {
	rawEmail, _ := call.Args["email"].(string)
	email, toolErr := emailArgument(rawEmail)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.SetTarget(domain.AuditTargetContact, email, "")

	var req domain.DeleteContactRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "email": email}, &req); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	// The service answers success for an address with no contact, so that an
	// interrupted erasure can be finished. A repeated call here answers
	// not_found instead, and deletes nothing.
	if _, err := s.deps.ContactRows.GetContactByEmail(ctx, call.WorkspaceID, email); err != nil {
		return nil, err
	}
	// The opt-outs stay as hashes: forgetting them is left to a person, in the
	// console or the API.
	if err := s.deps.Contacts.DeleteContact(ctx, call.WorkspaceID, email, domain.DeleteContactOptions{}); err != nil {
		return nil, err
	}
	return deleteContactOutput{Workspace: call.WorkspaceRef(), Deleted: true, Email: email}, nil
}

func (s *Server) trackCustomEventTool() *Tool {
	return &Tool{
		Name:  "track_custom_event",
		Title: "Track custom event",
		Description: "Records an event that happened to a contact, such as an order or a signup, and creates the contact when none has this email. " +
			"The event appears on the contact's timeline and can start automations, which may send emails. " +
			"event_name and external_id identify the event: a later call with both updates it, and only when its occurred_at is later than the stored one. " +
			"So a repeated call with the same occurred_at changes nothing. " +
			"The contact's timeline dates the event at occurred_at, not at the time of the call. " +
			"Returns the event as stored.",
		Class:       ClassWriteOpenWorld,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite}},
		AuditAction: "customEvents.upsert",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The contact's email address."},
				"event_name":   map[string]any{"type": "string", "description": "Lowercase letters, digits, underscores, dots, slashes and hyphens, at most 100 characters. For example order.completed."},
				"external_id":  map[string]any{"type": "string", "description": "The event's ID in your own system, at most 255 characters."},
				"occurred_at":  map[string]any{"type": "string", "description": "When the event happened, as an RFC 3339 date and time."},
				"properties":   map[string]any{"type": "object", "description": "The event's data. Replaces the stored properties; leave it out to keep them."},
				"goal_type": map[string]any{
					"type":        "string",
					"enum":        domain.ValidGoalTypes,
					"description": "Counts the event as a goal in the reports.",
				},
				"goal_name":  map[string]any{"type": "string", "description": "At most 100 characters."},
				"goal_value": map[string]any{"type": "number", "description": "The amount of a purchase or subscription, required for those two goal types. Negative for a refund."},
			},
			"required":             []string{"workspace_id", "email", "event_name", "external_id", "occurred_at"},
			"additionalProperties": false,
		},
		Handler: s.trackCustomEvent,
	}
}

type customEventOutput struct {
	EventName  string    `json:"event_name"`
	ExternalID string    `json:"external_id"`
	Email      string    `json:"email"`
	OccurredAt time.Time `json:"occurred_at"`
	Properties any       `json:"properties"`
	GoalType   *string   `json:"goal_type,omitempty"`
	GoalName   *string   `json:"goal_name,omitempty"`
	GoalValue  *float64  `json:"goal_value,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Truncation
}

type trackCustomEventOutput struct {
	Workspace WorkspaceRef      `json:"workspace"`
	Event     customEventOutput `json:"event"`
}

func (s *Server) trackCustomEvent(ctx context.Context, call *Call) (any, error) {
	rawEmail, _ := call.Args["email"].(string)
	email, toolErr := emailArgument(rawEmail)
	if toolErr != nil {
		return nil, toolErr
	}
	eventName, _ := call.Args["event_name"].(string)
	call.Audit.SetTarget(domain.AuditTargetContact, email, "")
	call.Audit.AddMetadata("email", email)
	call.Audit.AddMetadata("event_name", eventName)

	occurredAt, _ := call.Args["occurred_at"].(string)
	if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
		return nil, validationError("occurred_at must be an RFC 3339 date and time")
	}
	fields := make(map[string]any, len(call.Args))
	for key, value := range call.Args {
		fields[key] = value
	}
	fields["workspace_id"] = call.WorkspaceID
	fields["email"] = email
	var req domain.UpsertCustomEventRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}

	event, err := s.deps.CustomEvents.UpsertEvent(ctx, &req)
	if err != nil {
		return nil, err
	}
	return trackCustomEventOutput{Workspace: call.WorkspaceRef(), Event: projectCustomEvent(event)}, nil
}

func projectCustomEvent(event *domain.CustomEvent) customEventOutput {
	out := customEventOutput{
		OccurredAt: event.OccurredAt,
		GoalType:   event.GoalType,
		GoalValue:  event.GoalValue,
		CreatedAt:  event.CreatedAt,
		UpdatedAt:  event.UpdatedAt,
	}
	out.EventName = out.Text("event_name", event.EventName)
	out.ExternalID = out.Text("external_id", event.ExternalID)
	out.Email = out.Text("email", event.Email)
	out.GoalName = out.OptionalText("goal_name", event.GoalName)
	properties := map[string]any(event.Properties)
	if properties == nil {
		properties = map[string]any{}
	}
	out.Properties = out.JSON("properties", properties)
	return out
}
