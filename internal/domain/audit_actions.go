package domain

import "sort"

// The audit catalogue. Every /api/ route is named in exactly one of the three
// maps below — audited, logged-read or excluded — and a test in internal/http
// walks the registered routes to enforce it, so a route added later has to be
// classified before the build is green. The action name recorded for an
// audited route is the route itself (workspaces.inviteMember); the console and
// the docs read this file for their event lists.
//
// What is audited is the control plane: who changed configuration, who was
// granted what, who moved data in bulk. What is excluded is the data plane —
// contact upserts, sends, tracking — which can run at thousands of requests a
// minute and is a firehose, not a trail — and every read.

const (
	AuditCategoryAuth          = "auth"
	AuditCategoryLicence       = "licence"
	AuditCategorySystem        = "system"
	AuditCategoryWorkspaces    = "workspaces"
	AuditCategoryMembers       = "members"
	AuditCategoryAPIKeys       = "api_keys"
	AuditCategoryIntegrations  = "integrations"
	AuditCategoryWebhooks      = "webhooks"
	AuditCategoryTemplates     = "templates"
	AuditCategoryBroadcasts    = "broadcasts"
	AuditCategoryAutomations   = "automations"
	AuditCategoryLists         = "lists"
	AuditCategorySegments      = "segments"
	AuditCategoryTransactional = "transactional"
	AuditCategoryBlog          = "blog"
	AuditCategoryContacts      = "contacts"
	AuditCategoryAudit         = "audit"
)

// AuditCategories is every category, in the order the console groups them.
var AuditCategories = []string{
	AuditCategoryAuth,
	AuditCategoryLicence,
	AuditCategorySystem,
	AuditCategoryWorkspaces,
	AuditCategoryMembers,
	AuditCategoryAPIKeys,
	AuditCategoryIntegrations,
	AuditCategoryWebhooks,
	AuditCategoryTemplates,
	AuditCategoryBroadcasts,
	AuditCategoryAutomations,
	AuditCategoryLists,
	AuditCategorySegments,
	AuditCategoryTransactional,
	AuditCategoryBlog,
	AuditCategoryContacts,
	AuditCategoryAudit,
}

// Target types, so the console can link a row to what it names.
const (
	AuditTargetWorkspace    = "workspace"
	AuditTargetUser         = "user"
	AuditTargetAPIKey       = "api_key"
	AuditTargetInvitation   = "invitation"
	AuditTargetIntegration  = "integration"
	AuditTargetWebhook      = "webhook_subscription"
	AuditTargetTemplate     = "template"
	AuditTargetBlock        = "template_block"
	AuditTargetBroadcast    = "broadcast"
	AuditTargetAutomation   = "automation"
	AuditTargetList         = "list"
	AuditTargetSegment      = "segment"
	AuditTargetNotification = "transactional_notification"
	AuditTargetBlogPost     = "blog_post"
	AuditTargetBlogCategory = "blog_category"
	AuditTargetBlogTheme    = "blog_theme"
	AuditTargetContact      = "contact"
	AuditTargetAnnotation   = "annotation"
	AuditTargetLicence      = "licence"
	AuditTargetSettings     = "settings"
	AuditTargetAuditLog     = "audit_log"
)

// AuditActionSpec describes an audited POST route. TargetKey names the key of
// the JSON request body that carries the target's id (dotted for a nested
// object: "automation.id"); ResponseKey names it in the success response, for
// the creates that mint their own id. Either fills target_id for routes no
// service enriches; a service that knows better overrides it.
type AuditActionSpec struct {
	Category    string
	TargetType  string
	TargetKey   string
	ResponseKey string
}

// AuditedActions: every POST /api/<route> recorded by the middleware, keyed by
// route name.
var AuditedActions = map[string]AuditActionSpec{
	// auth — deployment-level; the actor is whoever signed in (or tried to)
	"user.signin":        {Category: AuditCategoryAuth, TargetType: AuditTargetUser, TargetKey: "email"},
	"user.verify":        {Category: AuditCategoryAuth, TargetType: AuditTargetUser, TargetKey: "email"},
	"user.rootSignin":    {Category: AuditCategoryAuth, TargetType: AuditTargetUser, TargetKey: "email"},
	"user.oidc.exchange": {Category: AuditCategoryAuth, TargetType: AuditTargetUser},
	"user.logout":        {Category: AuditCategoryAuth, TargetType: AuditTargetUser},
	"setup.initialize":   {Category: AuditCategorySystem, TargetType: AuditTargetSettings},

	// licence and system settings — deployment-level, root only
	"licence.set":     {Category: AuditCategoryLicence, TargetType: AuditTargetLicence},
	"settings.update": {Category: AuditCategorySystem, TargetType: AuditTargetSettings},

	// workspaces
	"workspaces.create":                  {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "id"},
	"workspaces.update":                  {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "id"},
	"workspaces.delete":                  {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "id"},
	"workspaces.setBlogSettings":         {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "workspace_id"},
	"workspaces.setCustomFieldLabels":    {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "workspace_id"},
	"workspaces.setWebAnalyticsSettings": {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "workspace_id"},
	"workspaces.setAuditLogSettings":     {Category: AuditCategoryWorkspaces, TargetType: AuditTargetWorkspace, TargetKey: "workspace_id"},

	// members
	"workspaces.inviteMember":       {Category: AuditCategoryMembers, TargetType: AuditTargetUser, TargetKey: "email"},
	"workspaces.acceptInvitation":   {Category: AuditCategoryMembers, TargetType: AuditTargetInvitation},
	"workspaces.deleteInvitation":   {Category: AuditCategoryMembers, TargetType: AuditTargetInvitation, TargetKey: "invitation_id"},
	"workspaces.removeMember":       {Category: AuditCategoryMembers, TargetType: AuditTargetUser, TargetKey: "user_id"},
	"workspaces.setUserPermissions": {Category: AuditCategoryMembers, TargetType: AuditTargetUser, TargetKey: "user_id"},

	// api keys
	"workspaces.createAPIKey":  {Category: AuditCategoryAPIKeys, TargetType: AuditTargetAPIKey},
	"workspaces.connectZapier": {Category: AuditCategoryAPIKeys, TargetType: AuditTargetAPIKey},

	// integrations and providers
	"workspaces.createIntegration": {Category: AuditCategoryIntegrations, TargetType: AuditTargetIntegration},
	"workspaces.updateIntegration": {Category: AuditCategoryIntegrations, TargetType: AuditTargetIntegration, TargetKey: "integration_id"},
	"workspaces.deleteIntegration": {Category: AuditCategoryIntegrations, TargetType: AuditTargetIntegration, TargetKey: "integration_id"},
	"ses.enableTenantIsolation":    {Category: AuditCategoryIntegrations, TargetType: AuditTargetIntegration, TargetKey: "integration_id"},
	"webhooks.register":            {Category: AuditCategoryIntegrations, TargetType: AuditTargetIntegration, TargetKey: "integration_id"},

	// outbound webhook subscriptions
	"webhookSubscriptions.create":           {Category: AuditCategoryWebhooks, TargetType: AuditTargetWebhook, ResponseKey: "subscription.id"},
	"webhookSubscriptions.update":           {Category: AuditCategoryWebhooks, TargetType: AuditTargetWebhook, TargetKey: "id"},
	"webhookSubscriptions.delete":           {Category: AuditCategoryWebhooks, TargetType: AuditTargetWebhook, TargetKey: "id"},
	"webhookSubscriptions.toggle":           {Category: AuditCategoryWebhooks, TargetType: AuditTargetWebhook, TargetKey: "id"},
	"webhookSubscriptions.regenerateSecret": {Category: AuditCategoryWebhooks, TargetType: AuditTargetWebhook, TargetKey: "id"},

	// templates
	"templates.create":      {Category: AuditCategoryTemplates, TargetType: AuditTargetTemplate, TargetKey: "id"},
	"templates.update":      {Category: AuditCategoryTemplates, TargetType: AuditTargetTemplate, TargetKey: "id"},
	"templates.delete":      {Category: AuditCategoryTemplates, TargetType: AuditTargetTemplate, TargetKey: "id"},
	"templateBlocks.create": {Category: AuditCategoryTemplates, TargetType: AuditTargetBlock, ResponseKey: "block.id"},
	"templateBlocks.update": {Category: AuditCategoryTemplates, TargetType: AuditTargetBlock, TargetKey: "id"},
	"templateBlocks.delete": {Category: AuditCategoryTemplates, TargetType: AuditTargetBlock, TargetKey: "id"},

	// broadcasts
	"broadcasts.create":           {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, ResponseKey: "broadcast.id"},
	"broadcasts.update":           {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.schedule":         {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.pause":            {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.resume":           {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.cancel":           {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.delete":           {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.retryFailed":      {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.selectWinner":     {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},
	"broadcasts.sendToIndividual": {Category: AuditCategoryBroadcasts, TargetType: AuditTargetBroadcast, TargetKey: "id"},

	// automations
	"automations.create":   {Category: AuditCategoryAutomations, TargetType: AuditTargetAutomation, TargetKey: "automation.id", ResponseKey: "automation.id"},
	"automations.update":   {Category: AuditCategoryAutomations, TargetType: AuditTargetAutomation, TargetKey: "automation.id"},
	"automations.delete":   {Category: AuditCategoryAutomations, TargetType: AuditTargetAutomation, TargetKey: "automation_id"},
	"automations.activate": {Category: AuditCategoryAutomations, TargetType: AuditTargetAutomation, TargetKey: "automation_id"},
	"automations.pause":    {Category: AuditCategoryAutomations, TargetType: AuditTargetAutomation, TargetKey: "automation_id"},

	// lists and segments
	"lists.create":     {Category: AuditCategoryLists, TargetType: AuditTargetList, TargetKey: "id", ResponseKey: "list.id"},
	"lists.update":     {Category: AuditCategoryLists, TargetType: AuditTargetList, TargetKey: "id"},
	"lists.delete":     {Category: AuditCategoryLists, TargetType: AuditTargetList, TargetKey: "id"},
	"segments.create":  {Category: AuditCategorySegments, TargetType: AuditTargetSegment, TargetKey: "id", ResponseKey: "segment.id"},
	"segments.update":  {Category: AuditCategorySegments, TargetType: AuditTargetSegment, TargetKey: "id"},
	"segments.delete":  {Category: AuditCategorySegments, TargetType: AuditTargetSegment, TargetKey: "id"},
	"segments.rebuild": {Category: AuditCategorySegments, TargetType: AuditTargetSegment, TargetKey: "id"},

	// transactional notifications
	"transactional.create": {Category: AuditCategoryTransactional, TargetType: AuditTargetNotification, TargetKey: "notification.id", ResponseKey: "notification.id"},
	"transactional.update": {Category: AuditCategoryTransactional, TargetType: AuditTargetNotification, TargetKey: "id"},
	"transactional.delete": {Category: AuditCategoryTransactional, TargetType: AuditTargetNotification, TargetKey: "id"},

	// blog
	"blogPosts.create":      {Category: AuditCategoryBlog, TargetType: AuditTargetBlogPost, ResponseKey: "post.id"},
	"blogPosts.update":      {Category: AuditCategoryBlog, TargetType: AuditTargetBlogPost, TargetKey: "id"},
	"blogPosts.delete":      {Category: AuditCategoryBlog, TargetType: AuditTargetBlogPost, TargetKey: "id"},
	"blogPosts.publish":     {Category: AuditCategoryBlog, TargetType: AuditTargetBlogPost, TargetKey: "id"},
	"blogPosts.unpublish":   {Category: AuditCategoryBlog, TargetType: AuditTargetBlogPost, TargetKey: "id"},
	"blogCategories.create": {Category: AuditCategoryBlog, TargetType: AuditTargetBlogCategory, ResponseKey: "category.id"},
	"blogCategories.update": {Category: AuditCategoryBlog, TargetType: AuditTargetBlogCategory, TargetKey: "id"},
	"blogCategories.delete": {Category: AuditCategoryBlog, TargetType: AuditTargetBlogCategory, TargetKey: "id"},
	"blogThemes.create":     {Category: AuditCategoryBlog, TargetType: AuditTargetBlogTheme, ResponseKey: "theme.version"},
	"blogThemes.update":     {Category: AuditCategoryBlog, TargetType: AuditTargetBlogTheme, TargetKey: "version"},
	"blogThemes.publish":    {Category: AuditCategoryBlog, TargetType: AuditTargetBlogTheme, TargetKey: "version"},

	// contacts: only the destructive and the bulk operations
	"contacts.delete":             {Category: AuditCategoryContacts, TargetType: AuditTargetContact, TargetKey: "email"},
	"contacts.import":             {Category: AuditCategoryContacts},
	"customEvents.import":         {Category: AuditCategoryContacts},
	"annotations.create":          {Category: AuditCategoryContacts, TargetType: AuditTargetAnnotation, ResponseKey: "annotation.id"},
	"annotations.update":          {Category: AuditCategoryContacts, TargetType: AuditTargetAnnotation, TargetKey: "id"},
	"annotations.delete":          {Category: AuditCategoryContacts, TargetType: AuditTargetAnnotation, TargetKey: "id"},
	"webAnalytics.backfillStart":  {Category: AuditCategoryContacts},
	"webAnalytics.backfillCancel": {Category: AuditCategoryContacts},

	// the audit log itself
	"auditLogs.export": {Category: AuditCategoryAudit, TargetType: AuditTargetAuditLog},
}

// AuditLoggedRead is a GET route recorded under a different action name, and
// only under a condition the middleware can see in the query string.
type AuditLoggedRead struct {
	Action   string
	Category string
	// RequireExport records the request only when it carries export=true and no
	// cursor: the console's export modal pages the list endpoint, and the first
	// page is the export.
	RequireExport bool
}

// AuditLoggedReads: the GET routes worth a row.
var AuditLoggedReads = map[string]AuditLoggedRead{
	"contacts.list":      {Action: "contacts.export", Category: AuditCategoryContacts, RequireExport: true},
	"user.oidc.callback": {Action: "user.oidc.callback", Category: AuditCategoryAuth},
}

// AuditExcludedRoutes: every /api/ route deliberately not recorded. Listed one
// by one rather than by pattern so that the exclusion of each is a decision
// someone made, and so the route-classification test can hold every route to
// exactly one list.
var AuditExcludedRoutes = map[string]struct{}{
	// data plane
	"contacts.upsert":            {},
	"customEvents.upsert":        {},
	"transactional.send":         {},
	"transactional.testTemplate": {},
	"lists.subscribe":            {},
	"contactLists.removeContact": {},
	"contactLists.updateStatus":  {},
	// utilities and tests that change nothing durable
	"llm.chat":                     {},
	"email.testProvider":           {},
	"settings.testSmtp":            {},
	"setup.testSmtp":               {},
	"setup.status":                 {},
	"templates.compile":            {},
	"segments.preview":             {},
	"broadcasts.refreshGlobalFeed": {},
	"broadcasts.testRecipientFeed": {},
	"webhookSubscriptions.test":    {},
	"user.updateLanguage":          {},
	"user.oidc.start":              {},
	"demo.reset":                   {},
	"detect-favicon":               {},
	// task plumbing and the cron endpoint
	"cron":          {},
	"cron.status":   {},
	"tasks.create":  {},
	"tasks.delete":  {},
	"tasks.execute": {},
	"tasks.get":     {},
	"tasks.list":    {},
	"tasks.reset":   {},
	"tasks.trigger": {},
	// reads
	"analytics.query":                  {},
	"analytics.schemas":                {},
	"annotations.get":                  {},
	"annotations.list":                 {},
	"auditLogs.actions":                {},
	"auditLogs.get":                    {},
	"auditLogs.list":                   {},
	"automations.get":                  {},
	"automations.list":                 {},
	"automations.nodeExecutions":       {},
	"blogCategories.get":               {},
	"blogCategories.list":              {},
	"blogPosts.get":                    {},
	"blogPosts.list":                   {},
	"blogThemes.get":                   {},
	"blogThemes.getPublished":          {},
	"blogThemes.list":                  {},
	"broadcasts.get":                   {},
	"broadcasts.getTestResults":        {},
	"broadcasts.list":                  {},
	"contactLists.getByIDs":            {},
	"contactLists.getContactsByList":   {},
	"contactLists.getListsByContact":   {},
	"contacts.count":                   {},
	"contacts.getByEmail":              {},
	"contacts.getByExternalID":         {},
	"customEvents.get":                 {},
	"customEvents.list":                {},
	"inboundWebhookEvents.list":        {},
	"licence.get":                      {},
	"lists.get":                        {},
	"lists.list":                       {},
	"lists.stats":                      {},
	"messages.broadcastLinkStats":      {},
	"messages.broadcastStats":          {},
	"messages.broadcastVariationStats": {},
	"messages.list":                    {},
	"segments.contacts":                {},
	"segments.get":                     {},
	"segments.list":                    {},
	"ses.listConfigurationSets":        {},
	"ses.listTenants":                  {},
	"ses.verifyTenant":                 {},
	"settings.get":                     {},
	"templateBlocks.get":               {},
	"templateBlocks.list":              {},
	"templates.get":                    {},
	"templates.list":                   {},
	"timeline.list":                    {},
	"transactional.get":                {},
	"transactional.list":               {},
	"usage.get":                        {},
	"user.me":                          {},
	"webAnalytics.backfillStatus":      {},
	"webhooks.status":                  {},
	"webhookSubscriptions.deliveries":  {},
	"webhookSubscriptions.eventTypes":  {},
	"webhookSubscriptions.get":         {},
	"webhookSubscriptions.list":        {},
	"workspaces.get":                   {},
	"workspaces.list":                  {},
	"workspaces.members":               {},
	"workspaces.verifyInvitationToken": {},
}

// AuditSystemActions are written by the server itself, outside any request.
var AuditSystemActions = map[string]AuditActionSpec{
	"audit.purged":             {Category: AuditCategoryAudit, TargetType: AuditTargetAuditLog},
	"licence.recordingStopped": {Category: AuditCategoryLicence, TargetType: AuditTargetAuditLog},
	"licence.recordingResumed": {Category: AuditCategoryLicence, TargetType: AuditTargetAuditLog},
}

// Non-HTTP action names, so callers and tests share one spelling.
const (
	AuditActionPurged           = "audit.purged"
	AuditActionRecordingStopped = "licence.recordingStopped"
	AuditActionRecordingResumed = "licence.recordingResumed"
)

// AuditActionCatalogue lists every action the log can contain, sorted by
// name: audited routes, logged reads under their recorded name, and the
// system actions.
func AuditActionCatalogue() []AuditActionDescriptor {
	out := make([]AuditActionDescriptor, 0, len(AuditedActions)+len(AuditLoggedReads)+len(AuditSystemActions))
	for action, spec := range AuditedActions {
		out = append(out, AuditActionDescriptor{Action: action, Category: spec.Category, TargetType: spec.TargetType})
	}
	for _, read := range AuditLoggedReads {
		out = append(out, AuditActionDescriptor{Action: read.Action, Category: read.Category})
	}
	for action, spec := range AuditSystemActions {
		out = append(out, AuditActionDescriptor{Action: action, Category: spec.Category, TargetType: spec.TargetType})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}
