package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Notifuse/notifuse/internal/domain"
)

// toolHandler wraps a tool in the steps every call goes through, in this
// order. Step 1, before it, is the server's: the SDK finds the tool by name
// (a name outside the catalogue is a JSON-RPC error) and the receiving
// middleware applies the mcp limit (server.go).
//
//  2. for a tool that is not a read, a new audit row, with the actor from the
//     MCPCaller and the key's own workspace until a call names another;
//  3. argument validation against the tool's schema;
//  4. for every tool but list_workspaces, AuthenticateUserForWorkspace on the
//     workspace_id, which applies the MCP reachability rule;
//  5. the permission pre-check against a fresh copy of the key's permissions,
//     refusing any account that is not an API key holding a member role;
//  6. the mcp_send limit for send and open-world write tools;
//  7. the tool's handler;
//  8. the outcome, as a result and in the audit row.
//
// Every result is built here, so an error carries its object in both
// structuredContent and the text content, with a stable code.
func (s *Server) toolHandler(tool *Tool) sdk.ToolHandler {
	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		started := time.Now()
		call := &Call{Tool: tool, Caller: callerFromContext(ctx)}

		if tool.Class != ClassRead && call.Caller != nil {
			call.Audit = s.newAuditRecord(ctx, call)
			ctx = domain.WithAuditRecord(ctx, call.Audit)
		}

		var rawArgs json.RawMessage
		if req != nil && req.Params != nil {
			rawArgs = req.Params.Arguments
		}
		out, toolErr := s.runTool(ctx, call, rawArgs)

		if call.Audit != nil {
			s.recordAudit(ctx, call, toolErr)
		}
		s.logCall(call, toolErr, time.Since(started))
		if toolErr != nil {
			return errorResult(toolErr), nil
		}
		return successResult(out), nil
	}
}

// runTool runs steps 3 to 7.
func (s *Server) runTool(ctx context.Context, call *Call, rawArgs json.RawMessage) (any, *ToolError) {
	tool := call.Tool
	if call.Caller == nil {
		return nil, &ToolError{Code: CodePermissionDenied, Message: "no MCP credential"}
	}

	args, raw, toolErr := decodeArguments(tool, rawArgs)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Args, call.RawArgs = args, raw

	if !tool.WithoutWorkspace {
		call.WorkspaceID, _ = args["workspace_id"].(string)
		authCtx, user, membership, err := s.deps.Auth.AuthenticateUserForWorkspace(ctx, call.WorkspaceID)
		if err != nil {
			return nil, call.fail(err)
		}
		ctx = authCtx
		call.User, call.Membership = user, membership

		effective, toolErr := precheck(tool, args, user, membership)
		if toolErr != nil {
			return nil, toolErr
		}
		call.Effective = effective
	}

	if tool.countsAsSend() && !s.deps.RateLimiter.Allow(SendRateLimitNamespace, call.Caller.APIKeyUserID) {
		return nil, rateLimitedError()
	}

	if !tool.WithoutWorkspace {
		workspace, err := s.deps.Workspaces.GetByID(ctx, call.WorkspaceID)
		if err != nil {
			return nil, call.fail(err)
		}
		call.Workspace = workspace
	}

	out, err := tool.Handler(ctx, call)
	if err != nil {
		return nil, call.fail(err)
	}
	return out, nil
}

// decodeArguments reads the arguments as a JSON object and validates them
// against the tool's schema. Absent arguments are an empty object.
func decodeArguments(tool *Tool, rawArgs json.RawMessage) (map[string]any, json.RawMessage, *ToolError) {
	if len(rawArgs) == 0 || string(rawArgs) == "null" {
		rawArgs = json.RawMessage("{}")
	}
	var decoded any
	if err := json.Unmarshal(rawArgs, &decoded); err != nil {
		return nil, nil, validationError("arguments are not valid JSON")
	}
	args, ok := decoded.(map[string]any)
	if !ok {
		return nil, nil, validationError("arguments must be a JSON object")
	}
	if err := tool.resolved.Validate(args); err != nil {
		return nil, nil, validationError("invalid arguments: %v", err)
	}
	return args, rawArgs, nil
}

// precheck refuses any account but an API key holding a member role, then
// checks the tool's pairs (static and argument-dependent) against a fresh
// copy of the membership's permissions, which it returns.
func precheck(tool *Tool, args map[string]any, user *domain.User, membership *domain.UserWorkspace) (domain.UserPermissions, *ToolError) {
	if !isMemberKey(user, membership) {
		return nil, &ToolError{Code: CodePermissionDenied, Message: "MCP serves API keys with the member role only"}
	}
	effective := maps.Clone(membership.Permissions)
	if missing := missingPair(effective, tool.requiredPairs(args)); missing != nil {
		return nil, permissionDenied(missing.Resource, missing.Verb)
	}
	return effective, nil
}

// newAuditRecord opens the row of a tool that is not a read. The workspace is
// the key's own until AuthenticateUserForWorkspace names the one called, so
// a call refused before that is still recorded where the key's owner looks.
func (s *Server) newAuditRecord(ctx context.Context, call *Call) *domain.AuditRecord {
	meta := RequestMetaFromContext(ctx)
	record := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
	if call.Caller.Kind == domain.MCPCredentialOAuth {
		// Set before the actor: an API key actor keeps an OAuth method.
		record.SetAuthMethod(domain.AuditAuthOAuth)
	}
	record.SetActorClaims(call.Caller.APIKeyUserID, string(domain.UserTypeAPIKey))
	record.AddMetadata("channel", "mcp")
	record.AddMetadata("mcp_tool", call.Tool.Name)
	if call.Caller.Kind == domain.MCPCredentialOAuth && call.Caller.ClientName != "" {
		record.AddMetadata("oauth_client", call.Caller.ClientName)
	}
	if memberships, err := s.deps.Workspaces.GetUserWorkspaces(ctx, call.Caller.APIKeyUserID); err == nil && len(memberships) > 0 {
		record.SetWorkspace(memberships[0].WorkspaceID)
	}
	return record
}

// recordAudit finalizes the row with the outcome and writes it.
func (s *Server) recordAudit(ctx context.Context, call *Call, toolErr *ToolError) {
	status := http.StatusOK
	if toolErr != nil {
		status = toolErr.auditStatus()
	}
	event, skipped := call.Audit.Finalize(status)
	if skipped {
		return
	}
	spec, _ := auditSpec(call.Tool.AuditAction)
	event.Action = call.Tool.AuditAction
	event.Category = spec.Category
	if event.TargetType == "" {
		event.TargetType = spec.TargetType
	}
	if toolErr != nil {
		if event.Metadata == nil {
			event.Metadata = make(map[string]any)
		}
		event.Metadata["error_code"] = toolErr.Code
		for key, value := range map[string]string{
			"resource":      toolErr.Resource,
			"permission":    toolErr.Permission,
			"feature":       toolErr.Feature,
			"required_tier": toolErr.RequiredTier,
		} {
			if value != "" {
				event.Metadata[key] = value
			}
		}
		if toolErr.Code == CodeLicenceRequired {
			event.Metadata["licence_refused"] = true
		}
	}
	s.deps.Recorder.Record(ctx, &event)
}

// logCall writes one line per call: the tool, the workspace, the outcome and
// the duration. Never an argument value, a token or an error's text, which
// can quote arguments.
func (s *Server) logCall(call *Call, toolErr *ToolError, elapsed time.Duration) {
	if s.deps.Logger == nil {
		return
	}
	outcome := "success"
	if toolErr != nil {
		outcome = toolErr.Code
	}
	entry := s.deps.Logger.WithFields(map[string]interface{}{
		"mcp_tool":     call.Tool.Name,
		"workspace_id": call.WorkspaceID,
		"outcome":      outcome,
		"duration_ms":  elapsed.Milliseconds(),
	})
	if toolErr != nil && toolErr.Code == CodeInternal {
		if call.failure != nil {
			entry = entry.WithField("error_type", errorType(call.failure))
		}
		entry.Error("MCP tool call failed")
		return
	}
	entry.Debug("MCP tool call")
}

// fail maps a service error to a tool error, keeping the error behind an
// internal one for the log line.
func (c *Call) fail(err error) *ToolError {
	toolErr := toToolError(err)
	if toolErr.Code == CodeInternal {
		c.failure = err
	}
	return toolErr
}

// errorType names an error's Go type for a log line, without its text.
func errorType(err error) string {
	return fmt.Sprintf("%T", err)
}
