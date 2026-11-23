package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Class is what a tool does, which decides its annotations, whether it writes
// an audit row and whether it counts in the mcp_send limit.
type Class string

const (
	ClassRead           Class = "read"
	ClassWrite          Class = "write"
	ClassWriteOpenWorld Class = "write_open_world"
	ClassSend           Class = "send"
	ClassDestructive    Class = "destructive"
)

// classAnnotations are the annotation values of each class. A write tool's
// idempotentHint has no default worth stating: each one sets Idempotent.
var classAnnotations = map[Class]struct {
	readOnly, destructive, openWorld, idempotent bool
}{
	ClassRead:           {readOnly: true, destructive: false, openWorld: false, idempotent: true},
	ClassWrite:          {readOnly: false, destructive: false, openWorld: false, idempotent: false},
	ClassWriteOpenWorld: {readOnly: false, destructive: false, openWorld: true, idempotent: false},
	ClassSend:           {readOnly: false, destructive: true, openWorld: true, idempotent: false},
	ClassDestructive:    {readOnly: false, destructive: true, openWorld: false, idempotent: true},
}

// Pair is one permission a tool needs.
type Pair struct {
	Resource domain.PermissionResource
	Verb     domain.PermissionType
}

// Tool is one entry of the MCP catalogue.
type Tool struct {
	Name        string
	Title       string
	Description string
	Class       Class

	// Pairs are the permissions the tool always needs. tools/list shows the
	// tool only to a key that holds them all.
	Pairs []Pair
	// ArgumentPairs, when set, gives the permissions some arguments add
	// (a cc that reads contacts, a template that changes). Checked at call
	// time only.
	ArgumentPairs func(args map[string]any) []Pair

	// Idempotent overrides the class's idempotentHint.
	Idempotent *bool

	// InputSchema is the JSON schema of the arguments: type, properties,
	// required, items, enum and description, with additionalProperties false.
	InputSchema map[string]any

	// WithoutWorkspace marks the one tool that takes no workspace_id
	// (list_workspaces). Every other tool requires it.
	WithoutWorkspace bool

	// AuditAction is the API route whose action name the audit row of a tool
	// that is not a read carries.
	AuditAction string

	// Handler runs the tool once the wrapper has validated the arguments,
	// authenticated the workspace and checked the permissions. It returns the
	// object of structuredContent, or an error the wrapper maps to a code.
	Handler func(ctx context.Context, call *Call) (any, error)

	resolved *jsonschema.Resolved
}

// Call is what the wrapper hands a tool handler.
type Call struct {
	Tool   *Tool
	Caller *domain.MCPCaller

	// Args are the validated arguments as decoded JSON; RawArgs the same bytes
	// for a handler that decodes them into its own struct.
	Args    map[string]any
	RawArgs json.RawMessage

	// Set for every tool that takes a workspace_id.
	WorkspaceID string
	Workspace   *domain.Workspace
	User        *domain.User
	Membership  *domain.UserWorkspace
	// Effective is a fresh copy of the key's permissions in the workspace.
	Effective domain.UserPermissions

	// Audit is the row of a tool that is not a read; nil for a read.
	Audit *domain.AuditRecord

	// failure is the error behind an internal outcome, for the log line.
	failure error
}

// DecodeArgs decodes the validated arguments into v.
func (c *Call) DecodeArgs(v any) error {
	return json.Unmarshal(c.RawArgs, v)
}

// WorkspaceRef names the workspace of the call, as every workspace-scoped
// result does.
func (c *Call) WorkspaceRef() WorkspaceRef {
	ref := WorkspaceRef{ID: c.WorkspaceID}
	if c.Workspace != nil {
		ref.Name = c.Workspace.Name
	}
	return ref
}

// WorkspaceRef is the {id, name} of a workspace in a tool result.
type WorkspaceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Annotations are the hints the class gives, with the tool's own
// idempotentHint when it sets one.
func (t *Tool) Annotations() *sdk.ToolAnnotations {
	values := classAnnotations[t.Class]
	idempotent := values.idempotent
	if t.Idempotent != nil {
		idempotent = *t.Idempotent
	}
	destructive := values.destructive
	openWorld := values.openWorld
	return &sdk.ToolAnnotations{
		Title:           t.Title,
		ReadOnlyHint:    values.readOnly,
		DestructiveHint: &destructive,
		OpenWorldHint:   &openWorld,
		IdempotentHint:  idempotent,
	}
}

// countsAsSend reports whether a call uses the mcp_send limit.
func (t *Tool) countsAsSend() bool {
	return t.Class == ClassSend || t.Class == ClassWriteOpenWorld
}

// requiredPairs are the static pairs plus those the arguments add.
func (t *Tool) requiredPairs(args map[string]any) []Pair {
	pairs := slices.Clone(t.Pairs)
	if t.ArgumentPairs != nil {
		pairs = append(pairs, t.ArgumentPairs(args)...)
	}
	return pairs
}

// sdkTool is the tool as tools/list describes it.
func (t *Tool) sdkTool() *sdk.Tool {
	return &sdk.Tool{
		Name:        t.Name,
		Title:       t.Title,
		Description: t.Description,
		InputSchema: t.InputSchema,
		Annotations: t.Annotations(),
	}
}

// prepare checks a tool definition and compiles its schema. A mistake here is
// a programming error, so it panics at startup rather than at the first call.
func (t *Tool) prepare() {
	if t.Name == "" || t.Handler == nil {
		panic(fmt.Sprintf("mcp: tool %q needs a name and a handler", t.Name))
	}
	if _, ok := classAnnotations[t.Class]; !ok {
		panic(fmt.Sprintf("mcp: tool %q has unknown class %q", t.Name, t.Class))
	}
	if (t.Class == ClassSend || t.Class == ClassDestructive) && !t.declaresWrite() {
		panic(fmt.Sprintf("mcp: %s tool %q must declare a write permission", t.Class, t.Name))
	}
	if t.Class != ClassRead {
		if _, ok := auditSpec(t.AuditAction); !ok {
			panic(fmt.Sprintf("mcp: tool %q names audit action %q, which is in neither AuditedActions nor MCPAuditedActions", t.Name, t.AuditAction))
		}
	}
	if t.InputSchema["type"] != "object" || t.InputSchema["additionalProperties"] != false {
		panic(fmt.Sprintf("mcp: tool %q input schema must be an object with additionalProperties false", t.Name))
	}
	if !t.WithoutWorkspace {
		required, _ := t.InputSchema["required"].([]string)
		if !slices.Contains(required, "workspace_id") {
			panic(fmt.Sprintf("mcp: tool %q must require workspace_id", t.Name))
		}
	}
	data, err := json.Marshal(t.InputSchema)
	if err != nil {
		panic(fmt.Sprintf("mcp: tool %q input schema: %v", t.Name, err))
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		panic(fmt.Sprintf("mcp: tool %q input schema: %v", t.Name, err))
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		panic(fmt.Sprintf("mcp: tool %q input schema: %v", t.Name, err))
	}
	t.resolved = resolved
}

func (t *Tool) declaresWrite() bool {
	for _, pair := range t.Pairs {
		if pair.Verb == domain.PermissionTypeWrite {
			return true
		}
	}
	return false
}

// auditSpec finds the category and target type of an action: the API's own
// spec when the route is audited there, else the MCP-only table.
func auditSpec(action string) (domain.AuditActionSpec, bool) {
	if action == "" {
		return domain.AuditActionSpec{}, false
	}
	_, audited := domain.AuditedActions[action]
	_, mcpOnly := domain.MCPAuditedActions[action]
	if audited == mcpOnly {
		return domain.AuditActionSpec{}, false
	}
	if audited {
		return domain.AuditedActions[action], true
	}
	return domain.MCPAuditedActions[action], true
}

func boolPtr(v bool) *bool { return &v }

// requestFromJSON builds a service request by decoding a JSON object made of
// the arguments, never by filling it field by field: a request type that tells
// an omitted key from a zero value then sees only the keys the caller gave.
func requestFromJSON(fields map[string]any, request any) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, request)
}
