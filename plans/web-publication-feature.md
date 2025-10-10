# Web Publication Feature for Broadcasts

## Overview

This plan implements a comprehensive web publication system for broadcasts, enabling content to be published both via email and as blog posts on a custom domain. The system includes block-level visibility controls, SEO optimization, and architectural changes to serve publications at the root path while moving the console to `/console`.

## Database Schema Changes (Migration v7.0)

### Broadcasts Table
Add new fields to support web publishing:

```sql
ALTER TABLE broadcasts
  ADD COLUMN IF NOT EXISTS channels JSONB DEFAULT '{"email": true, "web": false}'::jsonb,
  ADD COLUMN IF NOT EXISTS web_settings JSONB;
```

**channels structure**: `{"email": boolean, "web": boolean}`

**web_settings structure**:
```json
{
  "slug": "my-post-slug",
  "meta_title": "SEO Title",
  "meta_description": "SEO Description",
  "og_title": "Open Graph Title",
  "og_description": "Open Graph Description",
  "og_image": "https://...",
  "canonical_url": "https://...",
  "keywords": ["keyword1", "keyword2"],
  "published_at": "2025-01-15T10:00:00Z"
}
```

### Templates Table
Add visibility field to support block-level channel filtering. Templates already store `visual_editor_tree` as JSONB, and we'll extend the EmailBlock attributes to include visibility.

**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/migrations/v7.go` (create new)

## Backend Domain Changes

### 1. Broadcast Domain Model
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/domain/broadcast.go`

Add new types and fields:

```go
// BroadcastChannels defines which channels are enabled
type BroadcastChannels struct {
    Email bool `json:"email"`
    Web   bool `json:"web"`
}

// WebSettings contains web publication configuration
type WebSettings struct {
    Slug            string    `json:"slug"`
    MetaTitle       string    `json:"meta_title,omitempty"`
    MetaDescription string    `json:"meta_description,omitempty"`
    OGTitle         string    `json:"og_title,omitempty"`
    OGDescription   string    `json:"og_description,omitempty"`
    OGImage         string    `json:"og_image,omitempty"`
    CanonicalURL    string    `json:"canonical_url,omitempty"`
    Keywords        []string  `json:"keywords,omitempty"`
    PublishedAt     *time.Time `json:"published_at,omitempty"`
}

// Update Broadcast struct
type Broadcast struct {
    // ... existing fields ...
    Channels    BroadcastChannels `json:"channels"`
    WebSettings *WebSettings      `json:"web_settings,omitempty"`
}
```

Add Value() and Scan() methods for database serialization.

### 2. MJML Block Visibility
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/pkg/notifuse_mjml/model.go`

Extend `MJSectionAttributes` to include visibility:

```go
type MJSectionAttributes struct {
    // ... existing fields ...
    Visibility *string `json:"visibility,omitempty"` // "all" | "email_only" | "web_only"
}
```

### 3. Web Publication Service
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/service/web_publication_service.go` (create new)

```go
type WebPublicationService struct {
    broadcastRepo domain.BroadcastRepository
    templateRepo  domain.TemplateRepository
    workspaceRepo domain.WorkspaceRepository
    logger        logger.Logger
}

// Key methods:
// - GetPublishedPosts(workspaceID, page, pageSize)
// - GetPostBySlugAndID(slug, broadcastID)
// - RenderWebContent(broadcast, template) - filters blocks by visibility
// - PublishPost(broadcastID)
```

### 4. Broadcast Service Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/service/broadcast_service.go`

Update validation to:
- Only allow web channel for draft broadcasts
- Validate slug uniqueness per workspace
- Ensure custom_endpoint is set if web channel is enabled

### 5. Repository Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/repository/broadcast_postgres.go`

Update queries to include new `channels` and `web_settings` columns:
- CreateBroadcastTx
- UpdateBroadcast
- GetBroadcast
- ListBroadcasts
- Add new methods: GetPublishedWebBroadcasts, GetBySlugAndID

## Backend HTTP Layer

### 1. Web Publication Handler
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/web_publication_handler.go` (create new)

```go
type WebPublicationHandler struct {
    webPublicationService *service.WebPublicationService
    workspaceRepo         domain.WorkspaceRepository
    logger                logger.Logger
}

// Endpoints:
// GET  / - List published posts (detect workspace by Host header)
// GET  /{slug}-{id} - View single post
```

### 2. Broadcast Handler Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/broadcast_handler.go`

Update create/update endpoints to handle new fields:
- channels
- web_settings

Add validation endpoint:
- POST /api/broadcast.validate_slug - Check slug availability

### 3. Root Handler Refactor
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/root_handler.go`

Major refactor to handle routing:

```go
func (h *RootHandler) Handle(w http.ResponseWriter, r *http.Request) {
    // 1. Handle /config.js
    if r.URL.Path == "/config.js" { ... }
    
    // 2. Handle /console/* - serve console
    if strings.HasPrefix(r.URL.Path, "/console") { 
        h.serveConsole(w, r)
        return 
    }
    
    // 3. Handle /notification-center/*
    if strings.HasPrefix(r.URL.Path, "/notification-center") { ... }
    
    // 4. Handle /api/*
    if strings.HasPrefix(r.URL.Path, "/api") { 
        // API routes handled by other handlers
        return
    }
    
    // 5. Root path - detect workspace by host and serve web publications
    // Strip port from host for matching
    host := strings.Split(r.Host, ":")[0]
    workspace := h.detectWorkspaceByHost(host)
    
    // Only serve web publications if:
    // - A workspace was found matching this domain
    // - The workspace has custom_endpoint_url configured
    if workspace != nil && workspace.Settings.CustomEndpointURL != nil {
        h.webPublicationHandler.Handle(w, r)
        return
    }
    
    // 6. Default - No workspace found for this domain, redirect to /console
    // This handles:
    // - Accessing via main API domain (not a custom domain)
    // - Accessing via unknown/unconfigured custom domain
    // - Any other root path access that doesn't match a workspace
    http.Redirect(w, r, "/console", http.StatusTemporaryRedirect)
}

func (h *RootHandler) detectWorkspaceByHost(host string) *domain.Workspace {
    // Query workspace where custom_endpoint_url matches the host
    // Extract hostname from custom_endpoint_url and compare with request host
    // Example:
    //   Request: blog.example.com
    //   Workspace custom_endpoint_url: "https://blog.example.com"
    //   Match: yes, return workspace
    //
    //   Request: notifuse.com (main API domain)
    //   No workspace matches this
    //   Match: no, return nil
}
```

Update serveConsole to strip `/console` prefix before serving files.

### 4. App Initialization
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/app/app.go`

- Initialize WebPublicationService
- Initialize WebPublicationHandler
- Update RootHandler initialization with webPublicationHandler

## Frontend Console Changes

### 1. Broadcast API Types
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/services/api/broadcast.ts`

```typescript
export interface BroadcastChannels {
  email: boolean
  web: boolean
}

export interface WebSettings {
  slug: string
  meta_title?: string
  meta_description?: string
  og_title?: string
  og_description?: string
  og_image?: string
  canonical_url?: string
  keywords?: string[]
  published_at?: string
}

export interface Broadcast {
  // ... existing fields ...
  channels: BroadcastChannels
  web_settings?: WebSettings
}

export interface CreateBroadcastRequest {
  // ... existing fields ...
  channels: BroadcastChannels
  web_settings?: WebSettings
}
```

Add API method: `validateSlug(workspaceId: string, slug: string)`

### 2. Email Builder Types
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/email_builder/types.ts`

```typescript
export interface MJSectionAttributes {
  // ... existing attributes ...
  visibility?: 'all' | 'email_only' | 'web_only'
}
```

### 3. Email Builder Settings Panel
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/email_builder/panels/EditPanel.tsx`

Add visibility selector for mj-section blocks with Liquid tag detection:

```tsx
{selectedBlock?.type === 'mj-section' && (
  <>
    <Form.Item label="Visibility">
      <Select
        value={attributes.visibility || 'all'}
        onChange={(value) => updateAttribute('visibility', value)}
        options={[
          { value: 'all', label: 'All Channels' },
          { value: 'email_only', label: 'Email Only' },
          { value: 'web_only', label: 'Web Only' }
        ]}
      />
    </Form.Item>
    
    {/* Warning for Liquid tags in web-visible blocks */}
    {hasLiquidTagsInSection(selectedBlock) && 
     attributes.visibility !== 'email_only' && (
      <Alert
        type="warning"
        message="Personalization Not Available for Web"
        description="This section contains Liquid template tags (e.g., {{ contact.name }}). Web publications don't have access to contact data, so these tags will not render properly. Consider marking this section as 'Email Only' or remove personalization tags."
        showIcon
      />
    )}
  </>
)}
```

Add helper function to detect Liquid tags:

```tsx
const hasLiquidTagsInSection = (section: EmailBlock): boolean => {
  // Regex to detect Liquid tags: {{ ... }} or {% ... %}
  const liquidRegex = /\{\{.*?\}\}|\{%.*?%\}/
  
  // Recursively check section content and all children
  const checkBlock = (block: EmailBlock): boolean => {
    // Check content field if present
    if (block.content && liquidRegex.test(block.content)) {
      return true
    }
    
    // Recursively check children
    if (block.children) {
      return block.children.some(child => checkBlock(child))
    }
    
    return false
  }
  
  return checkBlock(section)
}
```

### 4. Email Builder Preview Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/email_builder/panels/Preview.tsx`

Add channel selector:

```tsx
<Segmented
  value={previewChannel}
  onChange={setPreviewChannel}
  options={[
    { label: 'Email Preview', value: 'email' },
    { label: 'Web Preview', value: 'web' }
  ]}
/>
```

Filter blocks before compilation based on selected channel and visibility attribute.

### 5. Broadcast Creation Drawer
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`

Major updates:

```tsx
// Add channel selection
<Form.Item label="Publishing Channels">
  <Space>
    <Form.Item name={['channels', 'email']} valuePropName="checked" noStyle>
      <Checkbox>Email</Checkbox>
    </Form.Item>
    <Form.Item name={['channels', 'web']} valuePropName="checked" noStyle>
      <Checkbox disabled={broadcast?.status !== 'draft'}>
        Web Publication
      </Checkbox>
    </Form.Item>
  </Space>
</Form.Item>

// Show web settings section when web is enabled
{webEnabled && customEndpointSet && (
  <>
    <Form.Item
      name={['web_settings', 'slug']}
      label="URL Slug"
      rules={[
        { required: true },
        { pattern: /^[a-z0-9-]+$/, message: 'Only lowercase letters, numbers, and hyphens' }
      ]}
    >
      <Input 
        placeholder="my-blog-post"
        addonBefore={`${customEndpoint}/`}
        addonAfter={`-${broadcast?.id || 'ID'}`}
      />
    </Form.Item>
    
    <Form.Item name={['web_settings', 'meta_title']} label="Meta Title">
      <Input maxLength={60} />
    </Form.Item>
    
    <Form.Item name={['web_settings', 'meta_description']} label="Meta Description">
      <Input.TextArea maxLength={160} rows={3} />
    </Form.Item>
    
    <Form.Item name={['web_settings', 'og_title']} label="Open Graph Title">
      <Input maxLength={60} />
    </Form.Item>
    
    <Form.Item name={['web_settings', 'og_description']} label="Open Graph Description">
      <Input.TextArea maxLength={160} rows={2} />
    </Form.Item>
    
    <Form.Item name={['web_settings', 'og_image']} label="Open Graph Image URL">
      <Input placeholder="https://..." />
    </Form.Item>
    
    <Form.Item name={['web_settings', 'keywords']} label="Keywords">
      <Select mode="tags" placeholder="Add keywords..." />
    </Form.Item>
  </>
)}

{webEnabled && !customEndpointSet && (
  <Alert
    type="warning"
    message="Web publications require a custom endpoint"
    description="Go to workspace settings and configure a custom endpoint URL to enable web publications."
  />
)}
```

### 6. Router Configuration
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/main.tsx` or router config

Update all routes to be prefixed with `/console`:
- `/console` - Dashboard
- `/console/signin` - Sign in
- `/console/signup` - Sign up
- `/console/workspaces/:id` - Workspace pages
- etc.

Update any hardcoded path references throughout the console application.

### 7. Config Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/vite.config.ts`

Update base path if needed for console serving at `/console`.

## Web Publication Frontend (New)

### Create Simple Blog View
**Directory**: `/Users/pierre/Sites/notifuse3/code/notifuse/web_publications/` (create new)

Create a minimal HTML/CSS/JS frontend for displaying publications:

```html
<!-- index.html template -->
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{workspace.name}}</title>
  <!-- SEO meta tags injected server-side -->
</head>
<body>
  <main>
    <div class="posts-list">
      <!-- Posts rendered server-side -->
    </div>
    <div class="pagination">
      <!-- Pagination links -->
    </div>
  </main>
  <footer>
    <a href="/console">Admin Console</a>
  </footer>
</body>
</html>
```

**Approach**: Server-side HTML rendering in Go using html/template package. No build step required.

**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/web_publication_handler.go`

Implement template rendering:
- List view template
- Single post view template
- Include SEO meta tags
- Filter MJML blocks by visibility before rendering
- Render MJML to HTML for web display

## Testing Strategy

### Backend Tests

1. **Migration Tests** (`internal/migrations/v7_test.go`):
   - Test broadcast table schema changes
   - Verify default values for new columns

2. **Domain Tests** (`internal/domain/broadcast_test.go`):
   - Test BroadcastChannels serialization
   - Test WebSettings validation
   - Test visibility filtering logic

3. **Repository Tests** (`internal/repository/broadcast_postgres_test.go`):
   - Test create/update with new fields
   - Test GetPublishedWebBroadcasts query
   - Test GetBySlugAndID query

4. **Service Tests** (`internal/service/web_publication_service_test.go`):
   - Test post listing with pagination
   - Test content filtering by visibility
   - Test slug validation

5. **HTTP Tests** (`internal/http/web_publication_handler_test.go`):
   - Test workspace detection by host
   - Test post listing endpoint
   - Test single post endpoint

### Frontend Tests

1. **Component Tests** (`console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`):
   - Test channel selection UI
   - Test web settings form visibility
   - Test slug validation

2. **Email Builder Tests** (`console/src/components/email_builder/panels/EditPanel.test.tsx`):
   - Test visibility selector for sections
   - Test preview channel switching

## Critical Implementation Details

### MJML Block Filtering Utility
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/pkg/notifuse_mjml/filter.go` (create new)

Create utility function to filter blocks by channel visibility:

```go
// FilterBlocksByChannel returns a new tree with only blocks visible for the given channel
// channel should be "email" or "web"
func FilterBlocksByChannel(tree EmailBlock, channel string) EmailBlock {
    // Traverse tree and remove mj-section blocks where visibility doesn't match
    // - "all" or nil: keep in both channels
    // - "email_only": keep only if channel == "email"
    // - "web_only": keep only if channel == "web"
}
```

### CompileTemplate API Enhancement
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/pkg/notifuse_mjml/compile.go`

Add Channel field to CompileTemplateRequest:

```go
type CompileTemplateRequest struct {
    WorkspaceID      string
    MessageID        string
    VisualEditorTree EmailBlock
    TemplateData     MapOfAny
    TrackingSettings TrackingSettings
    Channel          string // "email" or "web" - NEW FIELD
}
```

Update CompileTemplate to:
- Apply FilterBlocksByChannel before compilation
- Skip tracking/unsubscribe link injection for web channel
- Adjust link handling for web vs email

### Frontend Configuration Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/vite.config.ts`

```typescript
export default defineConfig({
  base: '/console/', // ADD THIS for serving at /console path
  // ... rest of config
})
```

**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/router.tsx`

```typescript
export const router = createRouter({ 
  routeTree,
  basepath: '/console' // ADD THIS for all routes under /console
})
```

### Migration Registration and Version Update
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/migrations/v7.go`

Don't forget the init function:

```go
func init() {
    Register(&V7Migration{})
}
```

**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/config/config.go`

```go
const VERSION = "7.0" // Update from current version
```

### HTML Template Embedding
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/web_publication_handler.go`

```go
import "embed"

//go:embed templates/*.html
var templateFS embed.FS

func (h *WebPublicationHandler) loadTemplates() {
    // Parse embedded templates
}
```

### Helper Functions
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/service/broadcast_service.go`

Add helper functions:

```go
// GenerateSlug creates a URL-safe slug from broadcast name
func GenerateSlug(name string) string {
    // Convert to lowercase, replace spaces with hyphens, remove special chars
}

// ValidateSlug checks if slug is valid and unique
func (s *BroadcastService) ValidateSlug(ctx context.Context, workspaceID, slug string, excludeBroadcastID string) error
```

**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/internal/service/web_publication_service.go`

```go
type PaginationData struct {
    CurrentPage int
    TotalPages  int
    HasPrev     bool
    HasNext     bool
    PrevPage    int
    NextPage    int
}

func (s *WebPublicationService) BuildCanonicalURL(workspace *Workspace, slug, id string) string
func (s *WebPublicationService) BuildPostURL(workspace *Workspace, broadcast *Broadcast) string
```

### Default Values in Broadcast Creation
When creating a broadcast, set sensible defaults:

```go
if req.Channels.Email == false && req.Channels.Web == false {
    req.Channels.Email = true // Default to email if nothing selected
}
```

### Error Handling HTML Templates
**Files to create**:
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/error_404.html`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/error_workspace_not_found.html`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/error_web_disabled.html`

### Authentication Redirect Updates
**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/contexts/AuthContext.tsx`

Update all navigation to use `/console` prefix:
- `navigate('/console/signin')`
- `navigate('/console/workspace/create')`
- etc.

**File**: `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/layouts/RootLayout.tsx`

Update route checks and redirects to use `/console` prefix.

### Published Timestamp Logic
Set `web_settings.published_at` when:
- Broadcast with web channel enabled is sent (status changes to 'sent')
- Can be explicitly set via API for scheduling web publication separately from email

## Implementation Order

1. **Version Update** - Bump version constant to 7.0
2. **Database Migration** - Create and test v7 migration with init() registration
3. **Backend Domain Models** - Add types for channels and web settings with defaults
4. **MJML Visibility** - Extend section attributes with visibility
5. **MJML Filtering Utility** - Create FilterBlocksByChannel function
6. **CompileTemplate Enhancement** - Add Channel parameter and filtering logic
7. **Repository Layer** - Update queries and add new methods
8. **Helper Functions** - Add slug generation, validation, URL builders
9. **Service Layer** - Create WebPublicationService and update BroadcastService
10. **HTML Templates** - Create list, post, and error page templates with embedding
11. **HTTP Handler** - Create WebPublicationHandler and update RootHandler
12. **Frontend Config** - Update vite.config.ts and router basepath
13. **Frontend Types** - Update TypeScript interfaces
14. **Frontend Auth Updates** - Update all auth redirects to use /console
15. **Email Builder UI** - Add visibility controls to sections
16. **Email Builder Preview** - Add channel preview switcher with filtering
17. **Broadcast Form** - Add channel selection and web settings fields
18. **Console Routing** - Update all route paths to include /console prefix
19. **Integration Testing** - Test end-to-end flow
20. **Documentation** - Update docs with web publication feature

## Files to Create

- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/migrations/v7.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/pkg/notifuse_mjml/filter.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/service/web_publication_service.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/web_publication_handler.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/web_list.html`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/web_post.html`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/error_404.html`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/error_workspace_not_found.html`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/templates/error_web_disabled.html`

## Files to Modify

- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/domain/broadcast.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/pkg/notifuse_mjml/model.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/pkg/notifuse_mjml/compile.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/repository/broadcast_postgres.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/service/broadcast_service.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/broadcast_handler.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/http/root_handler.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/internal/app/app.go`
- `/Users/pierre/Sites/notifuse3/code/notifuse/config/config.go` (version bump to 7.0)
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/vite.config.ts`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/router.tsx`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/services/api/broadcast.ts`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/email_builder/types.ts`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/email_builder/panels/EditPanel.tsx`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/email_builder/panels/Preview.tsx`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/contexts/AuthContext.tsx`
- `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/layouts/RootLayout.tsx`

