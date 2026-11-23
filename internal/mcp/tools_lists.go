package mcp

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

const listListsName = "list_lists"

// listListsTool pages through a workspace's lists with their subscription
// counts. lists.list returns every list at once, so the tool pages them by
// offset on a stable order.
func (s *Server) listListsTool() *Tool {
	return &Tool{
		Name:  listListsName,
		Title: "List lists",
		Description: "Lists the subscription lists of a workspace, newest first. " +
			"Each list has its description, whether it is public and uses double opt-in, the template of the confirmation email, and how many contacts are active, pending, unsubscribed, bounced and complained on it. " +
			"To get the next page, pass next_cursor back as cursor. A list created between two pages can show up twice.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"limit":        map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":       map[string]any{"type": "string", "description": "next_cursor of the previous page."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listLists,
	}
}

type listListsArgs struct {
	Limit  *int   `json:"limit"`
	Cursor string `json:"cursor"`
}

type listStats struct {
	Active       int `json:"active"`
	Pending      int `json:"pending"`
	Unsubscribed int `json:"unsubscribed"`
	Bounced      int `json:"bounced"`
	Complained   int `json:"complained"`
}

type templateRef struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

type listDetail struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description,omitempty"`
	IsPublic            bool         `json:"is_public"`
	IsDoubleOptin       bool         `json:"is_double_optin"`
	DoubleOptinTemplate *templateRef `json:"double_optin_template,omitempty"`
	Stats               listStats    `json:"stats"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	Truncation
}

type listListsOutput struct {
	Workspace  WorkspaceRef `json:"workspace"`
	Lists      []listDetail `json:"lists"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

func (s *Server) listLists(ctx context.Context, call *Call) (any, error) {
	var args listListsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	offset := 0
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listListsName, call.WorkspaceID, nil, &offset); toolErr != nil {
			return nil, toolErr
		}
		if offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	lists, err := s.deps.Lists.GetLists(ctx, call.WorkspaceID, false)
	if err != nil {
		return nil, err
	}
	lists = slices.DeleteFunc(lists, func(list *domain.List) bool { return list == nil })
	// lists.list orders by created_at alone; the ID breaks ties so an offset
	// always lands on the same list.
	slices.SortFunc(lists, func(a, b *domain.List) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	out := listListsOutput{Workspace: call.WorkspaceRef(), Lists: []listDetail{}}
	if offset >= len(lists) {
		return out, nil
	}
	end := min(offset+limit, len(lists))
	for _, list := range lists[offset:end] {
		stats, err := s.deps.Lists.GetListStats(ctx, call.WorkspaceID, list.ID)
		if err != nil {
			return nil, err
		}
		out.Lists = append(out.Lists, projectList(list, stats))
	}
	if end < len(lists) {
		next, err := EncodeCursor(listListsName, call.WorkspaceID, nil, end)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

func projectList(list *domain.List, stats *domain.ListStats) listDetail {
	out := listDetail{
		ID:            list.ID,
		IsPublic:      list.IsPublic,
		IsDoubleOptin: list.IsDoubleOptin,
		CreatedAt:     list.CreatedAt,
		UpdatedAt:     list.UpdatedAt,
	}
	out.Name = out.Text("name", list.Name)
	out.Description = out.Text("description", list.Description)
	if list.DoubleOptInTemplate != nil {
		out.DoubleOptinTemplate = &templateRef{ID: list.DoubleOptInTemplate.ID, Version: list.DoubleOptInTemplate.Version}
	}
	if stats != nil {
		out.Stats = listStats{
			Active:       stats.TotalActive,
			Pending:      stats.TotalPending,
			Unsubscribed: stats.TotalUnsubscribed,
			Bounced:      stats.TotalBounced,
			Complained:   stats.TotalComplained,
		}
	}
	return out
}

// listRecord is a list as the list write tools return it: list_lists's shape
// without the counts.
type listRecord struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description,omitempty"`
	IsPublic            bool         `json:"is_public"`
	IsDoubleOptin       bool         `json:"is_double_optin"`
	DoubleOptinTemplate *templateRef `json:"double_optin_template,omitempty"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	Truncation
}

func projectListRecord(list *domain.List) listRecord {
	detail := projectList(list, nil)
	return listRecord{
		ID:                  detail.ID,
		Name:                detail.Name,
		Description:         detail.Description,
		IsPublic:            detail.IsPublic,
		IsDoubleOptin:       detail.IsDoubleOptin,
		DoubleOptinTemplate: detail.DoubleOptinTemplate,
		CreatedAt:           detail.CreatedAt,
		UpdatedAt:           detail.UpdatedAt,
		Truncation:          detail.Truncation,
	}
}

type listOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	List      listRecord   `json:"list"`
}

func (s *Server) createListTool() *Tool {
	return &Tool{
		Name:  "create_list",
		Title: "Create list",
		Description: "Creates a subscription list with the ID you choose. " +
			"The ID cannot be reused, even after the list is deleted, so a repeated call answers conflict. " +
			"With double opt-in, people who subscribe through a public form get a confirmation email built from the template you name; the template is not checked here, so give the ID of an existing email template.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(false),
		Pairs:       []Pair{{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeWrite}},
		AuditAction: "lists.create",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":                  map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":                            map[string]any{"type": "string", "description": "Letters and digits only, at most 32."},
				"name":                          map[string]any{"type": "string", "description": "At most 255 characters."},
				"description":                   map[string]any{"type": "string"},
				"is_public":                     map[string]any{"type": "boolean", "description": "Shown on the public subscription pages. Default false."},
				"is_double_optin":               map[string]any{"type": "boolean", "description": "Default false. Requires double_optin_template_id."},
				"double_optin_template_id":      map[string]any{"type": "string", "description": "The email template of the confirmation email."},
				"double_optin_template_version": map[string]any{"type": "integer", "description": "Default 0."},
			},
			"required":             []string{"workspace_id", "id", "name"},
			"additionalProperties": false,
		},
		Handler: s.createList,
	}
}

type listWriteArgs struct {
	ID                         string  `json:"id"`
	Name                       *string `json:"name"`
	Description                *string `json:"description"`
	IsPublic                   *bool   `json:"is_public"`
	IsDoubleOptin              *bool   `json:"is_double_optin"`
	DoubleOptinTemplateID      *string `json:"double_optin_template_id"`
	DoubleOptinTemplateVersion *int64  `json:"double_optin_template_version"`
	RemoveDoubleOptinTemplate  bool    `json:"remove_double_optin_template"`
}

func (s *Server) createList(ctx context.Context, call *Call) (any, error) {
	var args listWriteArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	call.Audit.SetTarget(domain.AuditTargetList, args.ID, "")

	fields := map[string]any{"workspace_id": call.WorkspaceID}
	for _, key := range []string{"id", "name", "description", "is_public", "is_double_optin"} {
		if value, ok := call.Args[key]; ok {
			fields[key] = value
		}
	}
	if args.DoubleOptinTemplateID != nil {
		fields["double_optin_template"] = map[string]any{"id": *args.DoubleOptinTemplateID, "version": valueOr(args.DoubleOptinTemplateVersion, 0)}
	} else if args.DoubleOptinTemplateVersion != nil {
		return nil, validationError("double_optin_template_version needs double_optin_template_id")
	}
	var req domain.CreateListRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, err
	}
	list, _, err := req.Validate()
	if err != nil {
		return nil, validationError("%v", err)
	}

	if err := s.deps.Lists.CreateList(ctx, call.WorkspaceID, list); err != nil {
		var exists *domain.ErrListExists
		if errors.As(err, &exists) {
			return nil, &ToolError{Code: CodeConflict, Message: domain.NewAlreadyExistsError("list", list.ID, exists.Deleted).Error()}
		}
		return nil, err
	}
	call.Audit.SetTarget("", "", list.Name)
	return listOutput{Workspace: call.WorkspaceRef(), List: projectListRecord(list)}, nil
}

func (s *Server) updateListTool() *Tool {
	return &Tool{
		Name:  "update_list",
		Title: "Update list",
		Description: "Changes the fields you give on a list and keeps the others. " +
			"It can change who gets a confirmation email and which template it uses: with double opt-in, people who subscribe through a public form get one, and a contact who unsubscribed and fills in the form again gets one whenever the list has a template. " +
			"The list is read then written, so a change saved elsewhere in between can be lost. " +
			"Calling it again with the same arguments changes nothing more. " +
			"Returns the list as written.",
		Class:      ClassWriteOpenWorld,
		Idempotent: boolPtr(true),
		Pairs: []Pair{
			{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeWrite},
			{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeRead},
		},
		AuditAction: "lists.update",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":                  map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":                            map[string]any{"type": "string", "description": "The list's ID."},
				"name":                          map[string]any{"type": "string", "description": "At most 255 characters."},
				"description":                   map[string]any{"type": "string", "description": "An empty text clears it."},
				"is_public":                     map[string]any{"type": "boolean", "description": "Shown on the public subscription pages."},
				"is_double_optin":               map[string]any{"type": "boolean", "description": "Requires a confirmation template, given here or already set."},
				"double_optin_template_id":      map[string]any{"type": "string", "description": "The email template of the confirmation email."},
				"double_optin_template_version": map[string]any{"type": "integer", "description": "Default 0 when double_optin_template_id is given."},
				"remove_double_optin_template":  map[string]any{"type": "boolean", "description": "Removes the confirmation template. Not allowed while double opt-in stays on."},
			},
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.updateList,
	}
}

func (s *Server) updateList(ctx context.Context, call *Call) (any, error) {
	var args listWriteArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	call.Audit.SetTarget(domain.AuditTargetList, args.ID, "")
	if args.RemoveDoubleOptinTemplate && (args.DoubleOptinTemplateID != nil || args.DoubleOptinTemplateVersion != nil) {
		return nil, validationError("remove_double_optin_template cannot be combined with double_optin_template_id or double_optin_template_version")
	}

	// lists.update replaces the whole list, so the tool reads it and writes it
	// back with the given fields changed.
	stored, err := s.deps.Lists.GetListByID(ctx, call.WorkspaceID, args.ID)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{
		"workspace_id":    call.WorkspaceID,
		"id":              stored.ID,
		"name":            valueOr(args.Name, stored.Name),
		"description":     valueOr(args.Description, stored.Description),
		"is_public":       valueOr(args.IsPublic, stored.IsPublic),
		"is_double_optin": valueOr(args.IsDoubleOptin, stored.IsDoubleOptin),
	}
	var template *domain.TemplateReference
	if stored.DoubleOptInTemplate != nil {
		copied := *stored.DoubleOptInTemplate
		template = &copied
	}
	switch {
	case args.RemoveDoubleOptinTemplate:
		template = nil
	case args.DoubleOptinTemplateID != nil:
		template = &domain.TemplateReference{ID: *args.DoubleOptinTemplateID, Version: valueOr(args.DoubleOptinTemplateVersion, 0)}
	case args.DoubleOptinTemplateVersion != nil:
		if template == nil {
			return nil, validationError("double_optin_template_version needs double_optin_template_id: the list has no confirmation template")
		}
		template.Version = *args.DoubleOptinTemplateVersion
	}
	if template != nil {
		fields["double_optin_template"] = map[string]any{"id": template.ID, "version": template.Version}
	}
	var req domain.UpdateListRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, err
	}
	list, _, err := req.Validate()
	if err != nil {
		return nil, validationError("%v", err)
	}

	if err := s.deps.Lists.UpdateList(ctx, call.WorkspaceID, list); err != nil {
		return nil, err
	}
	list.CreatedAt = stored.CreatedAt
	call.Audit.SetTarget("", "", list.Name)
	return listOutput{Workspace: call.WorkspaceRef(), List: projectListRecord(list)}, nil
}

func (s *Server) deleteListTool() *Tool {
	return &Tool{
		Name:  "delete_list",
		Title: "Delete list",
		Description: "Deletes a list and ends every membership on it. Its ID cannot be used again. " +
			"This cannot be undone: confirm the list with the user first. " +
			"While live automations use the list, nothing is deleted and the tool answers conflict. " +
			"Returns not_found for a list that does not exist or is already deleted, which is also what a repeated call answers.",
		Class:       ClassDestructive,
		Pairs:       []Pair{{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeWrite}},
		AuditAction: "lists.delete",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":           map[string]any{"type": "string", "description": "The list's ID."},
			},
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.deleteList,
	}
}

type deleteListOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Deleted   bool         `json:"deleted"`
	ID        string       `json:"id"`
}

func (s *Server) deleteList(ctx context.Context, call *Call) (any, error) {
	id, _ := call.Args["id"].(string)
	call.Audit.SetTarget(domain.AuditTargetList, id, "")
	var req domain.DeleteListRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, err
	}
	if _, err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.deps.Lists.DeleteList(ctx, call.WorkspaceID, id); err != nil {
		return nil, err
	}
	return deleteListOutput{Workspace: call.WorkspaceRef(), Deleted: true, ID: id}, nil
}

// membershipStatusNone is the previous status of a contact who was not on the
// list: no row, or a removed row that kept no opt-out.
const membershipStatusNone = "none"

// previousMembershipStatus reads the contact's status on the list before a
// change, as lists.subscribe reads it: a removed row counts only for the
// opt-out it keeps. Read before the write, so a change made in between is not
// seen; the audit row records it as a best estimate.
func (s *Server) previousMembershipStatus(ctx context.Context, workspaceID, email, listID string) (status string, onList bool, err error) {
	row, err := s.deps.Memberships.GetContactListIncludingRemoved(ctx, workspaceID, email, listID)
	var missing *domain.ErrContactListNotFound
	if errors.As(err, &missing) {
		return membershipStatusNone, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if row.DeletedAt != nil {
		if domain.IsOptOutContactListStatus(row.Status) {
			return string(row.Status), false, nil
		}
		return membershipStatusNone, false, nil
	}
	return string(row.Status), true, nil
}

// subscriptionArgs are the arguments of the subscription tools.
type subscriptionArgs struct {
	Email       string `json:"email"`
	ListID      string `json:"list_id"`
	Status      string `json:"status"`
	Resubscribe bool   `json:"resubscribe"`
}

// subscriptionChange starts the audit row of a subscription tool and returns
// its normalized email.
func subscriptionChange(call *Call) (subscriptionArgs, *ToolError) {
	var args subscriptionArgs
	if err := call.DecodeArgs(&args); err != nil {
		return args, validationError("invalid arguments: %v", err)
	}
	email, toolErr := emailArgument(args.Email)
	if toolErr != nil {
		return args, toolErr
	}
	args.Email = email
	call.Audit.SetTarget(domain.AuditTargetContact, email, "")
	call.Audit.AddMetadata("email", email)
	call.Audit.AddMetadata("list_id", args.ListID)
	if _, given := call.Args["resubscribe"]; given {
		call.Audit.AddMetadata("resubscribe", args.Resubscribe)
	}
	return args, nil
}

const resubscribeDescription = "Set to true only when the contact has given their consent again, for example by asking to be subscribed. " +
	"It brings back a contact who unsubscribed from the list. Default false: the contact stays unsubscribed and the tool answers invalid_state."

func (s *Server) subscribeContactToListTool() *Tool {
	return &Tool{
		Name:  "subscribe_contact_to_list",
		Title: "Subscribe contact to list",
		Description: "Subscribes a contact to a list and creates the contact when none has this email. " +
			"The membership becomes active right away, even on a double opt-in list, and no confirmation email is sent: you vouch for the contact's consent. " +
			"A pending membership becomes active too. " +
			"A contact who unsubscribed stays unsubscribed unless resubscribe is true. A bounced or complained contact never changes. " +
			"In both cases the tool answers invalid_state with the contact's status. " +
			"Once active, the contact receives the list's broadcasts, and the subscription can start automations that send emails. " +
			"Calling it again with the same arguments changes nothing more.",
		Class:      ClassWriteOpenWorld,
		Idempotent: boolPtr(true),
		Pairs: []Pair{
			{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeWrite},
			{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite},
		},
		AuditAction: "lists.subscribe",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The contact's email address."},
				"list_id":      map[string]any{"type": "string", "description": "The list's ID."},
				"resubscribe":  map[string]any{"type": "boolean", "description": resubscribeDescription},
			},
			"required":             []string{"workspace_id", "email", "list_id"},
			"additionalProperties": false,
		},
		Handler: s.subscribeContactToList,
	}
}

type subscriptionOutput struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Email          string       `json:"email"`
	ListID         string       `json:"list_id"`
	ListName       string       `json:"list_name,omitempty"`
	Status         string       `json:"status,omitempty"`
	PreviousStatus string       `json:"previous_status,omitempty"`
	Dropped        bool         `json:"dropped,omitempty"`
	Truncation
}

func (s *Server) subscribeContactToList(ctx context.Context, call *Call) (any, error) {
	args, toolErr := subscriptionChange(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.SubscribeToListsRequest
	if err := requestFromJSON(map[string]any{
		"workspace_id": call.WorkspaceID,
		"contact":      map[string]any{"email": args.Email},
		"list_ids":     []string{args.ListID},
		"resubscribe":  args.Resubscribe,
	}, &req); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}

	previous, _, err := s.previousMembershipStatus(ctx, call.WorkspaceID, args.Email, args.ListID)
	if err != nil {
		return nil, err
	}
	call.Audit.AddMetadata("previous_status", previous)

	// As lists.subscribe: an authenticated caller, so a new or pending
	// membership becomes active with no confirmation email.
	memberships, err := s.deps.Subscriptions.SubscribeToListsWithResults(ctx, &req, true)
	if err != nil {
		return nil, err
	}
	out := subscriptionOutput{Workspace: call.WorkspaceRef(), Email: args.Email, ListID: args.ListID, PreviousStatus: previous}
	if len(memberships) == 0 {
		// A disposable address: dropped without a word, as the API does.
		out.Dropped = true
		call.Audit.AddMetadata("dropped", true)
		return out, nil
	}
	membership := memberships[0]
	call.Audit.AddMetadata("new_status", string(membership.Status))
	if domain.IsOptOutContactListStatus(membership.Status) {
		return nil, &domain.ErrContactListOptedOut{Status: membership.Status}
	}
	out.ListName = out.Text("list_name", membership.ListName)
	out.Status = string(membership.Status)
	return out, nil
}

func (s *Server) setContactListStatusTool() *Tool {
	return &Tool{
		Name:  "set_contact_list_status",
		Title: "Set contact list status",
		Description: "Changes a contact's status on a list it is already on. " +
			"An active or pending membership can take any status. The change sends no email, but an active contact receives the list's broadcasts. " +
			"An unsubscribed contact can only become bounced or complained, or active again when resubscribe is true. A bounced contact can only become complained, and a complained one never changes. " +
			"A refused change answers invalid_state with the contact's status, and a contact not on the list answers not_found. " +
			"Calling it again with the same arguments changes nothing more.",
		Class:      ClassWriteOpenWorld,
		Idempotent: boolPtr(true),
		Pairs: []Pair{
			{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeWrite},
			{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite},
		},
		AuditAction: "contactLists.updateStatus",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The contact's email address."},
				"list_id":      map[string]any{"type": "string", "description": "The list's ID."},
				"status": map[string]any{
					"type": "string",
					"enum": []string{"active", "pending", "unsubscribed", "bounced", "complained"},
				},
				"resubscribe": map[string]any{"type": "boolean", "description": resubscribeDescription + " Only with status active."},
			},
			"required":             []string{"workspace_id", "email", "list_id", "status"},
			"additionalProperties": false,
		},
		Handler: s.setContactListStatus,
	}
}

func (s *Server) setContactListStatus(ctx context.Context, call *Call) (any, error) {
	args, toolErr := subscriptionChange(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.UpdateContactListStatusRequest
	if err := requestFromJSON(map[string]any{
		"workspace_id": call.WorkspaceID,
		"email":        args.Email,
		"list_id":      args.ListID,
		"status":       args.Status,
		"resubscribe":  args.Resubscribe,
	}, &req); err != nil {
		return nil, err
	}
	_, membership, err := req.Validate()
	if err != nil {
		return nil, validationError("%v", err)
	}

	previous, onList, err := s.previousMembershipStatus(ctx, call.WorkspaceID, args.Email, args.ListID)
	if err != nil {
		return nil, err
	}
	call.Audit.AddMetadata("previous_status", previous)
	if !onList {
		return nil, contactNotOnList()
	}

	result, err := s.deps.ContactLists.UpdateContactListStatus(ctx, call.WorkspaceID, args.Email, args.ListID, membership.Status, req.Resubscribe)
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Found {
		// Removed since the read above.
		return nil, contactNotOnList()
	}
	call.Audit.AddMetadata("new_status", args.Status)
	return subscriptionOutput{
		Workspace:      call.WorkspaceRef(),
		Email:          args.Email,
		ListID:         args.ListID,
		Status:         args.Status,
		PreviousStatus: previous,
	}, nil
}

func contactNotOnList() *ToolError {
	return &ToolError{Code: CodeNotFound, Message: "the contact is not on this list"}
}

func (s *Server) removeContactFromListTool() *Tool {
	return &Tool{
		Name:  "remove_contact_from_list",
		Title: "Remove contact from list",
		Description: "Ends a contact's membership of a list. The contact stops receiving the list's broadcasts. " +
			"Its status on the list is kept, so a contact who unsubscribed, bounced or complained keeps that status if added back. " +
			"Confirm with the user first. " +
			"Returns not_found when the contact is not on the list, which is also what a repeated call answers.",
		Class: ClassDestructive,
		Pairs: []Pair{
			{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeWrite},
			{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite},
		},
		AuditAction: "contactLists.removeContact",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"email":        map[string]any{"type": "string", "description": "The contact's email address."},
				"list_id":      map[string]any{"type": "string", "description": "The list's ID."},
			},
			"required":             []string{"workspace_id", "email", "list_id"},
			"additionalProperties": false,
		},
		Handler: s.removeContactFromList,
	}
}

type removeContactFromListOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Removed   bool         `json:"removed"`
	Email     string       `json:"email"`
	ListID    string       `json:"list_id"`
	Status    string       `json:"status"`
}

func (s *Server) removeContactFromList(ctx context.Context, call *Call) (any, error) {
	args, toolErr := subscriptionChange(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.RemoveContactFromListRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "email": args.Email, "list_id": args.ListID}, &req); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	// The service answers success for a membership already removed, and
	// removes it again: read first, so a repeated call answers not_found.
	previous, onList, err := s.previousMembershipStatus(ctx, call.WorkspaceID, args.Email, args.ListID)
	if err != nil {
		return nil, err
	}
	call.Audit.AddMetadata("previous_status", previous)
	if !onList {
		return nil, contactNotOnList()
	}
	if err := s.deps.ContactLists.RemoveContactFromList(ctx, call.WorkspaceID, args.Email, args.ListID); err != nil {
		return nil, err
	}
	return removeContactFromListOutput{Workspace: call.WorkspaceRef(), Removed: true, Email: args.Email, ListID: args.ListID, Status: previous}, nil
}

// valueOr is the value of an optional argument, or fallback when it is absent.
func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
