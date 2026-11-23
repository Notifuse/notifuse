package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

const (
	testKeyID       = "key-1"
	testWorkspaceID = "ws1"
)

// fixture is one MCP server over HTTP with mocked services. Every request
// carries the MCPCaller of testKeyID, as the /mcp handler would set it.
type fixture struct {
	t           *testing.T
	auth        *mocks.MockAuthService
	workspaces  *mocks.MockWorkspaceRepository
	contacts    *mocks.MockContactService
	contactRows *mocks.MockContactRepository
	memberships *mocks.MockContactListRepository
	contactList *mocks.MockContactListService
	events      *mocks.MockCustomEventService
	subscribe   *subscriptionStub
	wsService   *mocks.MockWorkspaceServiceInterface
	timeline    *mocks.MockContactTimelineService
	lists       *mocks.MockListService
	segments    *mocks.MockSegmentService
	segmentRows *mocks.MockSegmentRepository
	templates   *mocks.MockTemplateService
	broadcasts  *mocks.MockBroadcastService
	bcastRows   *mocks.MockBroadcastRepository
	notifs      *mocks.MockTransactionalNotificationService
	notifRows   *mocks.MockTransactionalNotificationRepository
	messages    *mocks.MockMessageHistoryService
	analytics   *mocks.MockAnalyticsService
	automations *mocks.MockAutomationService
	autoRows    *mocks.MockAutomationRepository
	blog        *mocks.MockBlogService
	categories  *mocks.MockBlogCategoryRepository
	recorder    *mocks.MockAuditLogRecorder
	limiter     *ratelimiter.RateLimiter
	server      *Server
	url         string
	keyID       string
}

// newFixture builds the server with the given tools, or the catalogue when
// none is given.
func newFixture(t *testing.T, tools ...*Tool) *fixture {
	t.Helper()
	ctrl := gomock.NewController(nonFatalReporter{t})
	f := &fixture{
		t:           t,
		auth:        mocks.NewMockAuthService(ctrl),
		workspaces:  mocks.NewMockWorkspaceRepository(ctrl),
		contacts:    mocks.NewMockContactService(ctrl),
		contactRows: mocks.NewMockContactRepository(ctrl),
		memberships: mocks.NewMockContactListRepository(ctrl),
		contactList: mocks.NewMockContactListService(ctrl),
		events:      mocks.NewMockCustomEventService(ctrl),
		subscribe:   &subscriptionStub{t: t},
		wsService:   mocks.NewMockWorkspaceServiceInterface(ctrl),
		timeline:    mocks.NewMockContactTimelineService(ctrl),
		lists:       mocks.NewMockListService(ctrl),
		segments:    mocks.NewMockSegmentService(ctrl),
		segmentRows: mocks.NewMockSegmentRepository(ctrl),
		templates:   mocks.NewMockTemplateService(ctrl),
		broadcasts:  mocks.NewMockBroadcastService(ctrl),
		bcastRows:   mocks.NewMockBroadcastRepository(ctrl),
		notifs:      mocks.NewMockTransactionalNotificationService(ctrl),
		notifRows:   mocks.NewMockTransactionalNotificationRepository(ctrl),
		messages:    mocks.NewMockMessageHistoryService(ctrl),
		analytics:   mocks.NewMockAnalyticsService(ctrl),
		automations: mocks.NewMockAutomationService(ctrl),
		autoRows:    mocks.NewMockAutomationRepository(ctrl),
		blog:        mocks.NewMockBlogService(ctrl),
		categories:  mocks.NewMockBlogCategoryRepository(ctrl),
		recorder:    mocks.NewMockAuditLogRecorder(ctrl),
		limiter:     ratelimiter.NewRateLimiter(),
		keyID:       testKeyID,
	}
	t.Cleanup(f.limiter.Stop)
	f.limiter.SetPolicy(RateLimitNamespace, 120, time.Minute)
	f.limiter.SetPolicy(SendRateLimitNamespace, 30, time.Minute)

	deps := Deps{
		Auth:             f.auth,
		Workspaces:       f.workspaces,
		WorkspaceService: f.wsService,
		Contacts:         f.contacts,
		ContactRows:      f.contactRows,
		Timeline:         f.timeline,
		Lists:            f.lists,
		Subscriptions:    f.subscribe,
		ContactLists:     f.contactList,
		Memberships:      f.memberships,
		CustomEvents:     f.events,
		Segments:         f.segments,
		SegmentRows:      f.segmentRows,
		Templates:        f.templates,
		Broadcasts:       f.broadcasts,
		BroadcastRows:    f.bcastRows,
		Transactional:    f.notifs,
		NotificationRows: f.notifRows,
		Messages:         f.messages,
		Analytics:        f.analytics,
		Automations:      f.automations,
		AutomationRows:   f.autoRows,
		Blog:             f.blog,
		BlogCategories:   f.categories,
		Recorder:         f.recorder,
		RateLimiter:      f.limiter,
		Version:          "test",
	}
	if len(tools) == 0 {
		f.server = NewServer(deps)
	} else {
		f.server = newBareServer(deps)
		f.server.register(tools...)
	}

	handler := f.server.Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := domain.WithMCPCaller(r.Context(), &domain.MCPCaller{Kind: domain.MCPCredentialHeader, APIKeyUserID: f.keyID})
		ctx = WithRequestMeta(ctx, RequestMeta{IP: "203.0.113.7", UserAgent: "test-agent", RequestID: "req-1"})
		handler.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

// subscriptionStub stands in for the list service's
// SubscribeToListsWithResults, which domain.ListService and its generated mock
// do not declare. It records each request and answers with respond; a call
// without respond fails the test.
type subscriptionStub struct {
	t        *testing.T
	mu       sync.Mutex
	requests []domain.SubscribeToListsRequest
	bearer   []bool
	respond  func(req *domain.SubscribeToListsRequest) ([]*domain.ContactList, error)
}

func (s *subscriptionStub) SubscribeToListsWithResults(_ context.Context, req *domain.SubscribeToListsRequest, hasBearerToken bool) ([]*domain.ContactList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, *req)
	s.bearer = append(s.bearer, hasBearerToken)
	if s.respond == nil {
		s.t.Errorf("unexpected call to SubscribeToListsWithResults")
		return nil, errors.New("unexpected call")
	}
	return s.respond(req)
}

// nonFatalReporter reports an unexpected mock call as a failure without
// stopping the goroutine: the mocks are called from the SDK's goroutines,
// where a fatal stop would leave the HTTP request unanswered and the test
// hung instead of failed.
type nonFatalReporter struct{ t *testing.T }

func (r nonFatalReporter) Errorf(format string, args ...any) { r.t.Errorf(format, args...) }
func (r nonFatalReporter) Fatalf(format string, args ...any) { r.t.Errorf(format, args...) }
func (r nonFatalReporter) Helper()                           { r.t.Helper() }
func (r nonFatalReporter) Cleanup(f func())                  { r.t.Cleanup(f) }

func apiKeyUser() *domain.User {
	return &domain.User{ID: testKeyID, Email: "key@example.com", Name: "Agent key", Type: domain.UserTypeAPIKey}
}

func testWorkspace() *domain.Workspace {
	return &domain.Workspace{ID: testWorkspaceID, Name: "Acme", Settings: domain.WorkspaceSettings{Timezone: "Europe/Paris"}}
}

// reachable makes testWorkspaceID reachable for the key, as a member holding
// these permissions.
func (f *fixture) reachable(permissions domain.UserPermissions) {
	f.membership(&domain.UserWorkspace{UserID: testKeyID, WorkspaceID: testWorkspaceID, Role: "member", Permissions: permissions}, apiKeyUser())
}

// membership makes testWorkspaceID reachable with this membership and account.
func (f *fixture) membership(membership *domain.UserWorkspace, user *domain.User) {
	f.membershipIn(membership, user, testWorkspace())
}

// reachableIn is reachable with the workspace the wrapper reads, for a tool
// that depends on its settings or integrations.
func (f *fixture) reachableIn(permissions domain.UserPermissions, workspace *domain.Workspace) {
	f.membershipIn(&domain.UserWorkspace{UserID: testKeyID, WorkspaceID: testWorkspaceID, Role: "member", Permissions: permissions}, apiKeyUser(), workspace)
}

func (f *fixture) membershipIn(membership *domain.UserWorkspace, user *domain.User, workspace *domain.Workspace) {
	f.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), testKeyID).
		Return([]*domain.UserWorkspace{{UserID: testKeyID, WorkspaceID: testWorkspaceID}}, nil).AnyTimes()
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), testWorkspaceID).
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, user, membership, nil
		}).AnyTimes()
	f.workspaces.EXPECT().GetByID(gomock.Any(), testWorkspaceID).Return(workspace, nil).AnyTimes()
}

// switchedOff makes testWorkspaceID unreachable, as a workspace whose MCP
// switch is off answers.
func (f *fixture) switchedOff() {
	f.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), testKeyID).
		Return([]*domain.UserWorkspace{{UserID: testKeyID, WorkspaceID: testWorkspaceID}}, nil).AnyTimes()
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), testWorkspaceID).
		Return(nil, nil, nil, domain.ErrUserNotInWorkspace).AnyTimes()
}

func contactsRead() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}}
}

// rpcResponse is one JSON-RPC response.
type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *jsonrpc.Error  `json:"error"`
}

// rpc posts one request the way a 2026-07-28 client does: the protocol
// headers and the _meta fields.
func (f *fixture) rpc(method string, params map[string]any) rpcResponse {
	f.t.Helper()
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "test", "version": "1"},
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(f.t, err)
	req, err := http.NewRequest(http.MethodPost, f.url, bytes.NewReader(body))
	require.NoError(f.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		req.Header.Set("Mcp-Name", name)
	}
	// A mock that fails inside the SDK's goroutine never answers: the timeout
	// turns that into a failure instead of a hung test.
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(f.t, err)
	defer func() { _ = resp.Body.Close() }()
	return readRPCResponse(f.t, resp)
}

func readRPCResponse(t *testing.T, resp *http.Response) rpcResponse {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	payload := data
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		payload = nil
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 1<<20), 1<<22)
		for scanner.Scan() {
			if line, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				payload = []byte(line)
			}
		}
	}
	var out rpcResponse
	require.NoError(t, json.Unmarshal(payload, &out), "status %d: %s", resp.StatusCode, data)
	return out
}

// toolResult is a tools/call result as a client reads it.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
}

func (f *fixture) callTool(name string, args map[string]any) toolResult {
	f.t.Helper()
	resp := f.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	require.Nil(f.t, resp.Error, "tools/call %s answered a protocol error", name)
	var result toolResult
	require.NoError(f.t, json.Unmarshal(resp.Result, &result))
	return result
}

// errorOf returns the error object of an isError result, read from
// structuredContent.
func (r toolResult) errorOf(t *testing.T) ToolError {
	t.Helper()
	require.True(t, r.IsError, "expected an error result, got %s", r.StructuredContent)
	var body struct {
		Error ToolError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(r.StructuredContent, &body))
	return body.Error
}

// decode reads a successful result's structuredContent.
func (r toolResult) decode(t *testing.T, v any) {
	t.Helper()
	require.False(t, r.IsError, "expected success, got %s", r.StructuredContent)
	require.NoError(t, json.Unmarshal(r.StructuredContent, v))
}

func (f *fixture) listToolNames() []string {
	f.t.Helper()
	resp := f.rpc("tools/list", nil)
	require.Nil(f.t, resp.Error)
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(f.t, json.Unmarshal(resp.Result, &list))
	names := []string{}
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func toolNames(tools []*Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

// fakeTool is a workspace tool for wrapper tests.
func fakeTool(name string, class Class, pairs []Pair, auditAction string, handler func(ctx context.Context, call *Call) (any, error)) *Tool {
	if handler == nil {
		handler = func(context.Context, *Call) (any, error) { return map[string]any{"ok": true}, nil }
	}
	return &Tool{
		Name:        name,
		Description: fmt.Sprintf("Test tool %s.", name),
		Class:       class,
		Pairs:       pairs,
		AuditAction: auditAction,
		InputSchema: objectSchema(map[string]any{
			"workspace_id": map[string]any{"type": "string"},
			"note":         map[string]any{"type": "string"},
		}, "workspace_id"),
		Handler: handler,
	}
}

// assertReadTool checks a read tool's definition as tools/list describes it:
// its name, class and pairs, the read annotations of spec §6.1, no
// argument-dependent pair, and the required arguments.
func assertReadTool(t *testing.T, tool *Tool, name string, pairs []Pair, required []string) {
	t.Helper()
	assert.Equal(t, name, tool.Name)
	assert.Equal(t, ClassRead, tool.Class)
	assert.Equal(t, pairs, tool.Pairs)
	assert.Nil(t, tool.ArgumentPairs)
	assert.Empty(t, tool.AuditAction)
	assert.NotEmpty(t, tool.Title)
	assert.NotEmpty(t, tool.Description)

	listed := tool.sdkTool()
	require.NotNil(t, listed.Annotations)
	assert.True(t, listed.Annotations.ReadOnlyHint)
	require.NotNil(t, listed.Annotations.DestructiveHint)
	assert.False(t, *listed.Annotations.DestructiveHint)
	require.NotNil(t, listed.Annotations.OpenWorldHint)
	assert.False(t, *listed.Annotations.OpenWorldHint)
	assert.True(t, listed.Annotations.IdempotentHint)

	assert.Equal(t, required, tool.InputSchema["required"])
	assert.Equal(t, false, tool.InputSchema["additionalProperties"])
	assert.NotPanics(t, tool.prepare)
}

// catalogueTool returns the catalogue's tool of this name.
func catalogueTool(t *testing.T, name string) *Tool {
	t.Helper()
	for _, tool := range newBareServer(Deps{}).catalogue() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool %s in the catalogue", name)
	return nil
}

// auditLog collects the audit rows a fixture's tools write.
type auditLog struct {
	mu   sync.Mutex
	rows []domain.AuditLog
}

// last returns the latest row; it fails the test when there is none.
func (a *auditLog) last(t *testing.T) domain.AuditLog {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	require.NotEmpty(t, a.rows, "no audit row was written")
	return a.rows[len(a.rows)-1]
}

func (a *auditLog) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.rows)
}

// captureAudit records every audit row the tools write.
func (f *fixture) captureAudit() *auditLog {
	log := &auditLog{}
	f.recorder.EXPECT().Record(gomock.Any(), gomock.Any()).Do(func(_ context.Context, row *domain.AuditLog) {
		log.mu.Lock()
		defer log.mu.Unlock()
		log.rows = append(log.rows, *row)
	}).AnyTimes()
	return log
}

// writeHints are the annotations spec §6.1 and §6.2 give a tool that is not
// a read, written out as literals per tool.
type writeHints struct {
	destructive, openWorld, idempotent bool
}

// assertWriteTool checks the definition of a tool that is not a read: its
// name, class, pairs, audit action, annotations and required arguments.
func assertWriteTool(t *testing.T, tool *Tool, name string, class Class, hints writeHints, pairs []Pair, auditAction string, required []string) {
	t.Helper()
	assert.Nil(t, tool.ArgumentPairs)
	assertWriteToolDefinition(t, tool, name, class, hints, pairs, auditAction, required)
}

// assertWriteToolDefinition is assertWriteTool for a tool whose arguments can
// add pairs, which its own test checks.
func assertWriteToolDefinition(t *testing.T, tool *Tool, name string, class Class, hints writeHints, pairs []Pair, auditAction string, required []string) {
	t.Helper()
	assert.Equal(t, name, tool.Name)
	assert.Equal(t, class, tool.Class)
	assert.Equal(t, pairs, tool.Pairs)
	assert.Equal(t, auditAction, tool.AuditAction)
	assert.NotEmpty(t, tool.Title)
	assert.NotEmpty(t, tool.Description)

	listed := tool.sdkTool()
	require.NotNil(t, listed.Annotations)
	assert.False(t, listed.Annotations.ReadOnlyHint)
	require.NotNil(t, listed.Annotations.DestructiveHint)
	assert.Equal(t, hints.destructive, *listed.Annotations.DestructiveHint, "destructiveHint")
	require.NotNil(t, listed.Annotations.OpenWorldHint)
	assert.Equal(t, hints.openWorld, *listed.Annotations.OpenWorldHint, "openWorldHint")
	assert.Equal(t, hints.idempotent, listed.Annotations.IdempotentHint, "idempotentHint")

	assert.Equal(t, required, tool.InputSchema["required"])
	assert.Equal(t, false, tool.InputSchema["additionalProperties"])
	assert.NotPanics(t, tool.prepare)
}

func contactsWrite() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceContacts: {Write: true}}
}

func listsAndContactsWrite() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceLists: {Write: true}, domain.PermissionResourceContacts: {Write: true}}
}
