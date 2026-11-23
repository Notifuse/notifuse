// Package mcp serves the Model Context Protocol endpoint. It is a delivery
// layer: every tool calls the services, never an HTTP handler, and the /mcp
// HTTP handler in internal/http resolves the credential before any JSON-RPC
// message reaches this package. A few readers reach repositories directly
// (ContactReader, MembershipReader, SegmentReader, BroadcastReader,
// QueueReader, NotificationReader, AutomationReader, BlogCategoryReader), for the state a write tool reads first;
// they run only after the wrapper's pre-check.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Rate-limit namespaces. Their policies are set in internal/app.
const (
	// RateLimitNamespace bounds tools/list and tools/call per API key.
	RateLimitNamespace = "mcp"
	// SendRateLimitNamespace bounds the send and open-world write tools per
	// API key.
	SendRateLimitNamespace = "mcp_send"
)

// rateLimitedRPCCode is the JSON-RPC error code of a refused tools/list.
const rateLimitedRPCCode = -32000

// AuthService authenticates the caller for one workspace, applying the MCP
// reachability rule when the context carries an MCPCaller.
type AuthService interface {
	AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error)
}

// WorkspaceReader reads a workspace and the memberships of an account.
type WorkspaceReader interface {
	GetByID(ctx context.Context, id string) (*domain.Workspace, error)
	GetUserWorkspaces(ctx context.Context, userID string) ([]*domain.UserWorkspace, error)
}

// ContactService is the part of the contact service the tools call.
type ContactService interface {
	GetContacts(ctx context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error)
	GetContactByEmail(ctx context.Context, workspaceID string, email string) (*domain.Contact, error)
	GetContactByExternalID(ctx context.Context, workspaceID string, externalID string) (*domain.Contact, error)
	CountContacts(ctx context.Context, workspaceID string) (int, error)
	UpsertContact(ctx context.Context, workspaceID string, contact *domain.Contact) domain.UpsertContactOperation
	DeleteContact(ctx context.Context, workspaceID string, email string, opts domain.DeleteContactOptions) error
}

// ContactReader reads a contact row without a permission check of its own.
// delete_contact calls it, once the pre-check has granted contacts:write, to
// answer not_found for a contact that is already gone: the service deletes
// nothing then and still answers success.
type ContactReader interface {
	GetContactByEmail(ctx context.Context, workspaceID string, email string) (*domain.Contact, error)
}

// MembershipReader reads a contact's row on a list, a removed one included,
// without a permission check of its own. The subscription tools call it once
// the pre-check has granted lists:write and contacts:write, for the status
// before the change that their audit row records, and remove_contact_from_list
// to answer not_found when there is nothing to remove.
type MembershipReader interface {
	GetContactListIncludingRemoved(ctx context.Context, workspaceID string, email, listID string) (*domain.ContactList, error)
}

// ContactListService is the part of the contact list service the tools call.
type ContactListService interface {
	UpdateContactListStatus(ctx context.Context, workspaceID string, email, listID string, status domain.ContactListStatus, resubscribe bool) (*domain.UpdateContactListStatusResult, error)
	RemoveContactFromList(ctx context.Context, workspaceID string, email, listID string) error
}

// CustomEventService is the part of the custom event service the tools call.
type CustomEventService interface {
	UpsertEvent(ctx context.Context, req *domain.UpsertCustomEventRequest) (*domain.CustomEvent, error)
}

// TimelineService is the part of the contact timeline service the tools call.
type TimelineService interface {
	List(ctx context.Context, workspaceID string, email string, limit int, cursor *string, entityTypes []string) ([]*domain.ContactTimelineEntry, *string, error)
}

// ListService is the part of the list service the tools call.
type ListService interface {
	GetLists(ctx context.Context, workspaceID string, withCounts bool) ([]*domain.List, error)
	GetListStats(ctx context.Context, workspaceID string, id string) (*domain.ListStats, error)
	GetListByID(ctx context.Context, workspaceID string, id string) (*domain.List, error)
	CreateList(ctx context.Context, workspaceID string, list *domain.List) error
	UpdateList(ctx context.Context, workspaceID string, list *domain.List) error
	DeleteList(ctx context.Context, workspaceID string, id string) error
}

// ListSubscriptionService subscribes a contact to lists and reports the
// membership each list was left with. The list service implements it beside
// ListService; domain.ListService does not declare it.
type ListSubscriptionService interface {
	SubscribeToListsWithResults(ctx context.Context, payload *domain.SubscribeToListsRequest, hasBearerToken bool) ([]*domain.ContactList, error)
}

// SegmentService is the part of the segment service the tools call.
type SegmentService interface {
	ListSegments(ctx context.Context, req *domain.GetSegmentsRequest) ([]*domain.Segment, error)
	GetSegment(ctx context.Context, req *domain.GetSegmentRequest) (*domain.Segment, error)
	PreviewSegment(ctx context.Context, workspaceID string, tree *domain.TreeNode, limit int) (*domain.PreviewSegmentResponse, error)
	CreateSegment(ctx context.Context, req *domain.CreateSegmentRequest) (*domain.Segment, error)
	UpdateSegment(ctx context.Context, req *domain.UpdateSegmentRequest) (*domain.Segment, error)
	DeleteSegment(ctx context.Context, req *domain.DeleteSegmentRequest) error
}

// SegmentReader reads a segment row, a deleted one included, without a
// permission check of its own. update_segment and delete_segment call it once
// the pre-check has granted segments:write, to answer not_found for a deleted
// segment: the service updates one and deletes one again, and answers success.
type SegmentReader interface {
	GetSegmentByID(ctx context.Context, workspaceID string, id string) (*domain.Segment, error)
}

// TemplateService is the part of the template service the tools call.
type TemplateService interface {
	GetTemplates(ctx context.Context, workspaceID string, category string, channel string) ([]*domain.Template, error)
	GetTemplateByID(ctx context.Context, workspaceID string, id string, version int64) (*domain.Template, error)
	CompileTemplate(ctx context.Context, payload domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error)
	CreateTemplate(ctx context.Context, workspaceID string, template *domain.Template) error
	UpdateTemplate(ctx context.Context, workspaceID string, template *domain.Template) error
	DeleteTemplate(ctx context.Context, workspaceID string, id string) error
}

// BroadcastService is the part of the broadcast service the tools call.
type BroadcastService interface {
	ListBroadcasts(ctx context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error)
	GetBroadcast(ctx context.Context, workspaceID, id string, opts domain.GetBroadcastOptions) (*domain.Broadcast, error)
	CreateBroadcast(ctx context.Context, request *domain.CreateBroadcastRequest) (*domain.Broadcast, error)
	UpdateBroadcast(ctx context.Context, request *domain.UpdateBroadcastRequest) (*domain.Broadcast, error)
	ScheduleBroadcast(ctx context.Context, request *domain.ScheduleBroadcastRequest) error
	PauseBroadcast(ctx context.Context, request *domain.PauseBroadcastRequest) error
	ResumeBroadcast(ctx context.Context, request *domain.ResumeBroadcastRequest) error
	CancelBroadcast(ctx context.Context, request *domain.CancelBroadcastRequest) error
	DeleteBroadcast(ctx context.Context, request *domain.DeleteBroadcastRequest) error
	SendToIndividual(ctx context.Context, request *domain.SendToIndividualRequest) error
	SelectWinner(ctx context.Context, workspaceID, broadcastID, templateID string) error
}

// BroadcastReader reads a broadcast row without a permission check of its
// own. The broadcast write tools call it once the pre-check has granted
// broadcasts:write: update_broadcast to check the request against the stored
// broadcast before the service does, so a refusal comes back typed;
// delete_broadcast to refuse a broadcast still sending; and the send tools to
// report the status and audience the call left.
type BroadcastReader interface {
	GetBroadcast(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error)
}

// QueueReader counts a source's rows in the email queue without a permission
// check of its own. delete_broadcast calls it, once the pre-check has granted
// broadcasts:write, to refuse a broadcast whose queue still holds emails.
type QueueReader interface {
	GetSourceCounts(ctx context.Context, workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string) (*domain.EmailQueueSourceCounts, error)
}

// TransactionalService is the part of the transactional notification service
// the tools call.
type TransactionalService interface {
	ListNotifications(ctx context.Context, workspaceID string, filter map[string]interface{}, limit, offset int) ([]*domain.TransactionalNotification, int, error)
	GetNotification(ctx context.Context, workspaceID, id string) (*domain.TransactionalNotification, error)
	CreateNotification(ctx context.Context, workspaceID string, params domain.TransactionalNotificationCreateParams) (*domain.TransactionalNotification, error)
	UpdateNotification(ctx context.Context, workspaceID, id string, params domain.TransactionalNotificationUpdateParams) (*domain.TransactionalNotification, error)
	DeleteNotification(ctx context.Context, workspaceID, id string) error
	SendNotification(ctx context.Context, workspaceID string, params domain.TransactionalNotificationSendParams) (string, error)
	TestTemplate(ctx context.Context, workspaceID string, templateID string, integrationID string, senderID string, recipientEmail string, language string, listID string, omitListUnsubscribeHeader bool, options domain.EmailOptions) error
}

// NotificationReader reads a transactional notification row, deleted ones
// left out, without a permission check of its own.
// update_transactional_notification calls it once the pre-check has granted
// transactional:write, to keep the stored half of the tracking settings when a
// call changes only the other half: the service replaces them as a whole.
type NotificationReader interface {
	Get(ctx context.Context, workspaceID, id string) (*domain.TransactionalNotification, error)
}

// MessageHistoryService is the part of the message history service the tools
// call.
type MessageHistoryService interface {
	ListMessages(ctx context.Context, workspaceID string, params domain.MessageListParams) (*domain.MessageListResult, error)
	GetBroadcastStats(ctx context.Context, workspaceID, broadcastID string) (*domain.BroadcastDeliveryStats, error)
	GetBroadcastVariationStats(ctx context.Context, workspaceID, broadcastID, templateID string) (*domain.MessageHistoryStatusSum, error)
	GetBroadcastLinkStats(ctx context.Context, workspaceID, broadcastID, templateID string) ([]domain.LinkClickStats, error)
}

// AnalyticsService is the part of the analytics service the tools call.
type AnalyticsService interface {
	Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error)
	GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error)
}

// AutomationService is the part of the automation service the tools call.
type AutomationService interface {
	List(ctx context.Context, workspaceID string, filter domain.AutomationFilter) ([]*domain.Automation, int, error)
	Get(ctx context.Context, workspaceID, automationID string) (*domain.Automation, error)
	CountJourneys(ctx context.Context, workspaceID string, automationIDs []string) (map[string]domain.JourneyCounts, error)
	GetContactNodeExecutions(ctx context.Context, workspaceID, automationID, email string) (*domain.ContactAutomation, []*domain.NodeExecution, error)
	GetNodeStats(ctx context.Context, workspaceID, automationID string, since, until *time.Time) (*domain.NodeStatsResponse, error)
	Activate(ctx context.Context, workspaceID, automationID string) error
	Pause(ctx context.Context, workspaceID, automationID string) error
	Delete(ctx context.Context, workspaceID, automationID string) error
}

// AutomationReader reads an automation row, deleted ones left out, without a
// permission check of its own. The automation write tools call it once the
// pre-check has granted automations:write: pause_automation for the status
// before the call, which tells a replay, and activate_automation for the
// audience its audit row records.
type AutomationReader interface {
	GetByID(ctx context.Context, workspaceID, id string) (*domain.Automation, error)
}

// BlogService is the part of the blog service the tools call. It reads the
// workspace from the context, under domain.WorkspaceIDKey.
type BlogService interface {
	ListPosts(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error)
	GetPost(ctx context.Context, id string) (*domain.BlogPost, error)
	GetPostBySlug(ctx context.Context, slug string) (*domain.BlogPost, error)
	ListCategories(ctx context.Context) (*domain.BlogCategoryListResponse, error)
	CreatePost(ctx context.Context, request *domain.CreateBlogPostRequest) (*domain.BlogPost, error)
	UpdatePost(ctx context.Context, request *domain.UpdateBlogPostRequest) (*domain.BlogPost, error)
	DeletePost(ctx context.Context, request *domain.DeleteBlogPostRequest) error
	PublishPost(ctx context.Context, request *domain.PublishBlogPostRequest) error
	UnpublishPost(ctx context.Context, request *domain.UnpublishBlogPostRequest) error
}

// BlogCategoryReader reads blog categories, deleted ones included, without a
// permission check of its own, from the workspace in the context.
// create_blog_post and update_blog_post call it once the pre-check has granted
// blog:write, to refuse a category that does not exist before the body
// template is written: the service answers that case untyped.
type BlogCategoryReader interface {
	GetCategoriesByIDs(ctx context.Context, ids []string) ([]*domain.BlogCategory, error)
}

// WorkspaceService is the part of the workspace service the tools call.
type WorkspaceService interface {
	GetWorkspace(ctx context.Context, id string) (*domain.Workspace, error)
}

// RateLimiter is the process's rate limiter.
type RateLimiter interface {
	Allow(namespace, key string) bool
}

// Deps are the services the server calls.
type Deps struct {
	Auth             AuthService
	Workspaces       WorkspaceReader
	WorkspaceService WorkspaceService
	Contacts         ContactService
	ContactRows      ContactReader
	Timeline         TimelineService
	Lists            ListService
	Subscriptions    ListSubscriptionService
	ContactLists     ContactListService
	Memberships      MembershipReader
	CustomEvents     CustomEventService
	Segments         SegmentService
	SegmentRows      SegmentReader
	Templates        TemplateService
	Broadcasts       BroadcastService
	BroadcastRows    BroadcastReader
	Queue            QueueReader
	Transactional    TransactionalService
	NotificationRows NotificationReader
	Messages         MessageHistoryService
	Analytics        AnalyticsService
	Automations      AutomationService
	AutomationRows   AutomationReader
	Blog             BlogService
	BlogCategories   BlogCategoryReader
	Recorder         domain.AuditLogRecorder
	RateLimiter      RateLimiter
	Logger           logger.Logger
	Version          string
}

// Server is the one MCP server of the process. Its tools are registered once;
// tools/list is filtered per caller by a receiving middleware.
type Server struct {
	deps   Deps
	sdk    *sdk.Server
	tools  []*Tool
	byName map[string]*Tool
}

// NewServer builds the server with the whole catalogue.
func NewServer(deps Deps) *Server {
	s := newBareServer(deps)
	s.register(s.catalogue()...)
	return s
}

// newBareServer builds a server without tools.
func newBareServer(deps Deps) *Server {
	if deps.Recorder == nil {
		deps.Recorder = domain.NoopAuditLogRecorder{}
	}
	s := &Server{deps: deps, byName: make(map[string]*Tool)}
	s.sdk = sdk.NewServer(&sdk.Implementation{Name: "notifuse", Version: deps.Version}, &sdk.ServerOptions{
		// Set explicitly: the default advertises tools.listChanged, which this
		// server never sends.
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{ListChanged: false}},
		SetCacheable: setCacheable,
	})
	s.sdk.AddReceivingMiddleware(s.middleware)
	return s
}

// catalogue is every tool of the server.
func (s *Server) catalogue() []*Tool {
	return []*Tool{
		s.listWorkspacesTool(),
		s.getWorkspaceOverviewTool(),
		s.searchContactsTool(),
		s.getContactTool(),
		s.countContactsTool(),
		s.getContactTimelineTool(),
		s.upsertContactTool(),
		s.deleteContactTool(),
		s.trackCustomEventTool(),
		s.listListsTool(),
		s.createListTool(),
		s.updateListTool(),
		s.deleteListTool(),
		s.subscribeContactToListTool(),
		s.setContactListStatusTool(),
		s.removeContactFromListTool(),
		s.listSegmentsTool(),
		s.getSegmentTool(),
		s.previewSegmentTool(),
		s.createSegmentTool(),
		s.updateSegmentTool(),
		s.deleteSegmentTool(),
		s.listTemplatesTool(),
		s.getTemplateTool(),
		s.renderTemplateTool(),
		s.createTemplateTool(),
		s.updateTemplateTool(),
		s.deleteTemplateTool(),
		s.listBroadcastsTool(),
		s.getBroadcastTool(),
		s.getBroadcastReportTool(),
		s.createBroadcastTool(),
		s.updateBroadcastTool(),
		s.sendBroadcastToContactTool(),
		s.scheduleBroadcastTool(),
		s.selectBroadcastWinnerTool(),
		s.pauseBroadcastTool(),
		s.resumeBroadcastTool(),
		s.cancelBroadcastTool(),
		s.deleteBroadcastTool(),
		s.listTransactionalNotificationsTool(),
		s.getTransactionalNotificationTool(),
		s.createTransactionalNotificationTool(),
		s.updateTransactionalNotificationTool(),
		s.deleteTransactionalNotificationTool(),
		s.sendTransactionalTool(),
		s.sendTemplateTestTool(),
		s.searchMessagesTool(),
		s.listAnalyticsSchemasTool(),
		s.queryAnalyticsTool(),
		s.listAutomationsTool(),
		s.getAutomationTool(),
		s.getAutomationJourneyTool(),
		s.getAutomationStatsTool(),
		s.activateAutomationTool(),
		s.pauseAutomationTool(),
		s.deleteAutomationTool(),
		s.listBlogPostsTool(),
		s.getBlogPostTool(),
		s.listBlogCategoriesTool(),
		s.createBlogPostTool(),
		s.updateBlogPostTool(),
		s.publishBlogPostTool(),
		s.unpublishBlogPostTool(),
		s.deleteBlogPostTool(),
	}
}

// register adds tools to the server through the wrapper.
func (s *Server) register(tools ...*Tool) {
	for _, tool := range tools {
		tool.prepare()
		if _, dup := s.byName[tool.Name]; dup {
			panic("mcp: tool " + tool.Name + " registered twice")
		}
		s.tools = append(s.tools, tool)
		s.byName[tool.Name] = tool
		s.sdk.AddTool(tool.sdkTool(), s.toolHandler(tool))
	}
}

// Handler is the Streamable HTTP handler of the server: stateless, without
// the SDK's localhost guard (the /mcp handler checks the host itself), and
// without tying a call to its HTTP request, so a dropped connection never
// cancels a send in flight.
func (s *Server) Handler() http.Handler {
	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.sdk }, &sdk.StreamableHTTPOptions{
		Stateless:                    true,
		DisableLocalhostProtection:   true,
		PropagateRequestCancellation: false,
	})
}

// setCacheable makes tools/list cacheable by the caller's client only, for a
// minute: the list depends on the key's permissions.
func setCacheable(_ context.Context, req sdk.Request, c *sdk.Cacheable) {
	if _, ok := req.(*sdk.ListToolsRequest); ok {
		c.CacheScope = "private"
		c.TTLMs = 60000
	}
}

// middleware applies the mcp limit to tools/list and tools/call before any
// database work, and filters tools/list by the caller.
func (s *Server) middleware(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if method != "tools/list" && method != "tools/call" {
			return next(ctx, method, req)
		}
		caller := callerFromContext(ctx)
		if caller == nil {
			// The /mcp handler always sets one; without it nothing is listed or run.
			if method == "tools/list" {
				return &sdk.ListToolsResult{Tools: []*sdk.Tool{}}, nil
			}
			return errorResult(&ToolError{Code: CodePermissionDenied, Message: "no MCP credential"}), nil
		}
		if !s.deps.RateLimiter.Allow(RateLimitNamespace, caller.APIKeyUserID) {
			if method == "tools/list" {
				return nil, &jsonrpc.Error{
					Code:    rateLimitedRPCCode,
					Message: rateLimitedError().Message,
					Data:    json.RawMessage(`{"code":"rate_limited"}`),
				}
			}
			return errorResult(rateLimitedError()), nil
		}
		if method == "tools/call" {
			return next(ctx, method, req)
		}

		result, err := next(ctx, method, req)
		if err != nil {
			return result, err
		}
		list, ok := result.(*sdk.ListToolsResult)
		if !ok {
			return result, nil
		}
		grants, err := s.reachableGrants(ctx, caller)
		if err != nil {
			s.logError("tools/list", "", err)
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "internal error"}
		}
		visible := make([]*sdk.Tool, 0, len(list.Tools))
		for _, listed := range list.Tools {
			if tool, ok := s.byName[listed.Name]; ok && s.visibleWith(tool, grants) {
				visible = append(visible, listed)
			}
		}
		slices.SortFunc(visible, func(a, b *sdk.Tool) int { return strings.Compare(a.Name, b.Name) })
		list.Tools = visible
		return list, nil
	}
}

// visibleWith reports whether tools/list shows a tool to a key holding these
// grants: a tool without a workspace always, any other when one reachable
// workspace grants all its static pairs.
func (s *Server) visibleWith(tool *Tool, grants []domain.UserPermissions) bool {
	if tool.WithoutWorkspace {
		return true
	}
	for _, grant := range grants {
		if missingPair(grant, tool.Pairs) == nil {
			return true
		}
	}
	return false
}

// reachable is one workspace the caller may reach over MCP.
type reachable struct {
	workspaceID string
	effective   domain.UserPermissions
}

// reachableWorkspaces lists the workspaces the caller's key reaches: a
// membership whose workspace is switched on, held by the key as a member.
// AuthenticateUserForWorkspace applies the switch.
func (s *Server) reachableWorkspaces(ctx context.Context, caller *domain.MCPCaller) ([]reachable, error) {
	memberships, err := s.deps.Workspaces.GetUserWorkspaces(ctx, caller.APIKeyUserID)
	if err != nil {
		return nil, err
	}
	var out []reachable
	for _, membership := range memberships {
		_, user, userWorkspace, err := s.deps.Auth.AuthenticateUserForWorkspace(ctx, membership.WorkspaceID)
		if errors.Is(err, domain.ErrUserNotInWorkspace) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !isMemberKey(user, userWorkspace) {
			continue
		}
		out = append(out, reachable{workspaceID: membership.WorkspaceID, effective: maps.Clone(userWorkspace.Permissions)})
	}
	return out, nil
}

func (s *Server) reachableGrants(ctx context.Context, caller *domain.MCPCaller) ([]domain.UserPermissions, error) {
	workspaces, err := s.reachableWorkspaces(ctx, caller)
	if err != nil {
		return nil, err
	}
	grants := make([]domain.UserPermissions, 0, len(workspaces))
	for _, workspace := range workspaces {
		grants = append(grants, workspace.effective)
	}
	return grants, nil
}

// isMemberKey is the only account MCP serves: an API key holding a member
// role, so no owner-only operation is reachable through it.
func isMemberKey(user *domain.User, membership *domain.UserWorkspace) bool {
	return user != nil && user.Type == domain.UserTypeAPIKey && membership != nil && membership.Role == "member"
}

// missingPair returns the first pair the permissions do not grant.
func missingPair(permissions domain.UserPermissions, pairs []Pair) *Pair {
	for i, pair := range pairs {
		granted := permissions[pair.Resource]
		ok := granted.Read
		if pair.Verb == domain.PermissionTypeWrite {
			ok = granted.Write
		}
		if !ok {
			return &pairs[i]
		}
	}
	return nil
}

// catalogueResources are the resources the catalogue's static pairs name.
func (s *Server) catalogueResources() map[domain.PermissionResource]struct{} {
	resources := make(map[domain.PermissionResource]struct{})
	for _, tool := range s.tools {
		for _, pair := range tool.Pairs {
			resources[pair.Resource] = struct{}{}
		}
	}
	return resources
}

func (s *Server) logError(tool, workspaceID string, err error) {
	if s.deps.Logger == nil {
		return
	}
	s.deps.Logger.WithFields(map[string]interface{}{
		"mcp_tool":     tool,
		"workspace_id": workspaceID,
		"error_type":   errorType(err),
	}).Error("MCP call failed")
}
