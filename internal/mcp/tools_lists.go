package mcp

import (
	"cmp"
	"context"
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
