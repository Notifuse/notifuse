import { test as base, Page, Route } from '@playwright/test'
import {
  RequestCaptureStore,
  parseRequestBody,
  getPatternFromUrl
} from './request-capture'
import {
  mockUserMeResponse,
  mockWorkspace,
  mockWorkspaceMembers,
  mockContactsResponse,
  mockEmptyContacts,
  mockListsResponse,
  mockEmptyLists,
  mockSegmentsResponse,
  mockEmptySegments,
  mockBroadcastsResponse,
  mockEmptyBroadcasts,
  mockTemplatesResponse,
  mockEmptyTemplates,
  mockTransactionalResponse,
  mockEmptyTransactional,
  mockEmptyLogs,
  mockEmptyFiles,
  mockTotalContacts,
  mockAnalyticsData,
  mockBlogPostsResponse,
  mockBlogCategoriesResponse,
  mockBlogThemesResponse,
  mockSuccessResponse,
  mockContactUpsertResponse,
  mockContactImportResponse,
  mockListCreateResponse,
  mockTemplateCreateResponse,
  mockBroadcastCreateResponse,
  mockSegmentCreateResponse,
  mockTransactionalCreateResponse,
  mockBlogPostCreateResponse,
  mockBlogCategoryCreateResponse,
  mockTestEmailResponse,
  mockCompiledTemplate
} from './mock-data'

// Helper to create JSON response
const jsonResponse = (data: unknown) => ({
  status: 200,
  contentType: 'application/json',
  body: JSON.stringify(data)
})

// Global request capture store - shared across tests
export const requestCapture = new RequestCaptureStore()

// Configuration options for API mocks
export interface MockConfig {
  // Set to true to return data instead of empty responses
  withData?: boolean
}

// Setup all API mocks for a page - only intercept XHR/fetch requests to the backend
async function setupApiMocks(page: Page, config: MockConfig = {}) {
  const { withData = false } = config

  // Mock config.js to provide app configuration
  await page.route('**/config.js', (route: Route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/javascript',
      body: `
        window.API_URL = "https://localapi.notifuse.com:4000";
        window.ROOT_EMAIL = "test@example.com";
        window.IS_INSTALLED = true;
      `
    })
  )

  // Intercept all requests to the API backend (localapi.notifuse.com:4000)
  // Only intercept XHR/fetch requests, not script/module requests
  await page.route('https://localapi.notifuse.com:4000/**', async (route: Route) => {
    const url = route.request().url()
    const method = route.request().method()
    const resourceType = route.request().resourceType()

    // Only mock fetch/xhr requests, let other resource types pass through
    if (resourceType !== 'fetch' && resourceType !== 'xhr') {
      return route.continue()
    }

    // Capture POST/PUT/PATCH requests for payload verification
    if (method === 'POST' || method === 'PUT' || method === 'PATCH') {
      const pattern = getPatternFromUrl(url)
      if (pattern) {
        const body = await parseRequestBody(route.request())
        requestCapture.capture(pattern, {
          url,
          method,
          body,
          timestamp: Date.now()
        })
      }
    }

    // ============================================
    // USER & WORKSPACE (READ)
    // ============================================
    if (url.includes('/api/user.me')) {
      return route.fulfill(jsonResponse(mockUserMeResponse))
    }
    if (url.includes('/api/workspace.get')) {
      return route.fulfill(jsonResponse({ workspace: mockWorkspace }))
    }
    if (url.includes('/api/workspace.members') || url.includes('/api/workspaces.members')) {
      return route.fulfill(jsonResponse(mockWorkspaceMembers))
    }

    // ============================================
    // CONTACTS (supports both singular and plural API routes)
    // ============================================
    if (url.includes('/api/contact.list') || url.includes('/api/contacts.list')) {
      return route.fulfill(jsonResponse(withData ? mockContactsResponse : mockEmptyContacts))
    }
    if (
      url.includes('/api/contact.total') ||
      url.includes('/api/contacts.total') ||
      url.includes('/api/contact.count') ||
      url.includes('/api/contacts.count')
    ) {
      return route.fulfill(jsonResponse(mockTotalContacts))
    }
    if (url.includes('/api/contact.get') || url.includes('/api/contacts.get')) {
      return route.fulfill(jsonResponse({ contact: mockContactsResponse.contacts[0] }))
    }
    if (
      url.includes('/api/contact.upsert') ||
      url.includes('/api/contact.create') ||
      url.includes('/api/contacts.upsert') ||
      url.includes('/api/contacts.create')
    ) {
      return route.fulfill(jsonResponse(mockContactUpsertResponse))
    }
    if (url.includes('/api/contact.delete') || url.includes('/api/contacts.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }
    if (url.includes('/api/contact.import') || url.includes('/api/contacts.import')) {
      return route.fulfill(jsonResponse(mockContactImportResponse))
    }
    if (url.includes('/api/contact.bulk') || url.includes('/api/contacts.bulk')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    // ============================================
    // LISTS (supports both singular and plural API routes)
    // ============================================
    if (url.includes('/api/list.list') || url.includes('/api/lists.list')) {
      return route.fulfill(jsonResponse(withData ? mockListsResponse : mockEmptyLists))
    }
    if (url.includes('/api/list.get') || url.includes('/api/lists.get')) {
      return route.fulfill(jsonResponse({ list: mockListsResponse.lists[0] }))
    }
    if (url.includes('/api/list.create') || url.includes('/api/lists.create')) {
      return route.fulfill(jsonResponse(mockListCreateResponse))
    }
    if (url.includes('/api/list.update') || url.includes('/api/lists.update')) {
      return route.fulfill(jsonResponse(mockListCreateResponse))
    }
    if (url.includes('/api/list.delete') || url.includes('/api/lists.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    // ============================================
    // SEGMENTS (supports both singular and plural API routes)
    // ============================================
    if (url.includes('/api/segment.list') || url.includes('/api/segments.list')) {
      return route.fulfill(jsonResponse(withData ? mockSegmentsResponse : mockEmptySegments))
    }
    if (url.includes('/api/segment.get') || url.includes('/api/segments.get')) {
      return route.fulfill(jsonResponse({ segment: mockSegmentsResponse.segments[0] }))
    }
    if (url.includes('/api/segment.create') || url.includes('/api/segments.create')) {
      return route.fulfill(jsonResponse(mockSegmentCreateResponse))
    }
    if (url.includes('/api/segment.update') || url.includes('/api/segments.update')) {
      return route.fulfill(jsonResponse(mockSegmentCreateResponse))
    }
    if (url.includes('/api/segment.delete') || url.includes('/api/segments.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }
    if (
      url.includes('/api/segment.build') ||
      url.includes('/api/segment.rebuild') ||
      url.includes('/api/segments.build') ||
      url.includes('/api/segments.rebuild')
    ) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    // ============================================
    // TEMPLATES (supports both singular and plural API routes)
    // ============================================
    if (url.includes('/api/template.list') || url.includes('/api/templates.list')) {
      return route.fulfill(jsonResponse(withData ? mockTemplatesResponse : mockEmptyTemplates))
    }
    if (url.includes('/api/template.get') || url.includes('/api/templates.get')) {
      // Check if it's a validation call for a new template (ID check)
      // If the ID matches a known template, return it; otherwise return 404 for validation to pass
      const urlObj = new URL(url)
      const templateId = urlObj.searchParams.get('id')

      // Return existing template only for known IDs (from mock data)
      if (templateId && mockTemplatesResponse.templates.some(t => t.id === templateId)) {
        return route.fulfill(jsonResponse({ template: mockTemplatesResponse.templates[0] }))
      }

      // For unknown IDs (new templates), return 404 so validation passes
      return route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Template not found' })
      })
    }
    if (url.includes('/api/template.create') || url.includes('/api/templates.create')) {
      return route.fulfill(jsonResponse(mockTemplateCreateResponse))
    }
    if (url.includes('/api/template.update') || url.includes('/api/templates.update')) {
      return route.fulfill(jsonResponse(mockTemplateCreateResponse))
    }
    if (url.includes('/api/template.delete') || url.includes('/api/templates.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }
    if (
      url.includes('/api/template.compile') ||
      url.includes('/api/template.preview') ||
      url.includes('/api/templates.compile') ||
      url.includes('/api/templates.preview')
    ) {
      return route.fulfill(jsonResponse(mockCompiledTemplate))
    }
    if (url.includes('/api/template.send_test') || url.includes('/api/templates.send_test')) {
      return route.fulfill(jsonResponse(mockTestEmailResponse))
    }

    // ============================================
    // BROADCASTS (supports both singular and plural API routes)
    // ============================================
    if (url.includes('/api/broadcast.list') || url.includes('/api/broadcasts.list')) {
      return route.fulfill(jsonResponse(withData ? mockBroadcastsResponse : mockEmptyBroadcasts))
    }
    if (url.includes('/api/broadcast.get') || url.includes('/api/broadcasts.get')) {
      return route.fulfill(jsonResponse({ broadcast: mockBroadcastsResponse.broadcasts[0] }))
    }
    if (url.includes('/api/broadcast.create') || url.includes('/api/broadcasts.create')) {
      return route.fulfill(jsonResponse(mockBroadcastCreateResponse))
    }
    if (url.includes('/api/broadcast.update') || url.includes('/api/broadcasts.update')) {
      return route.fulfill(jsonResponse(mockBroadcastCreateResponse))
    }
    if (url.includes('/api/broadcast.delete') || url.includes('/api/broadcasts.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }
    if (url.includes('/api/broadcast.schedule') || url.includes('/api/broadcasts.schedule')) {
      return route.fulfill(
        jsonResponse({
          broadcast: { ...mockBroadcastsResponse.broadcasts[0], status: 'scheduled' }
        })
      )
    }
    if (url.includes('/api/broadcast.send') || url.includes('/api/broadcasts.send')) {
      return route.fulfill(
        jsonResponse({
          broadcast: { ...mockBroadcastsResponse.broadcasts[0], status: 'sending' }
        })
      )
    }
    if (url.includes('/api/broadcast.cancel') || url.includes('/api/broadcasts.cancel')) {
      return route.fulfill(
        jsonResponse({
          broadcast: { ...mockBroadcastsResponse.broadcasts[0], status: 'cancelled' }
        })
      )
    }
    if (url.includes('/api/broadcast.pause') || url.includes('/api/broadcasts.pause')) {
      return route.fulfill(
        jsonResponse({
          broadcast: { ...mockBroadcastsResponse.broadcasts[0], status: 'paused' }
        })
      )
    }
    if (url.includes('/api/broadcast.resume') || url.includes('/api/broadcasts.resume')) {
      return route.fulfill(
        jsonResponse({
          broadcast: { ...mockBroadcastsResponse.broadcasts[0], status: 'sending' }
        })
      )
    }
    if (url.includes('/api/broadcast.send_test') || url.includes('/api/broadcasts.send_test')) {
      return route.fulfill(jsonResponse(mockTestEmailResponse))
    }

    // ============================================
    // AUTOMATIONS
    // ============================================
    if (url.includes('/api/automation.list') || url.includes('/api/automations.list')) {
      return route.fulfill(jsonResponse({ automations: [] }))
    }
    if (url.includes('/api/automation.get') || url.includes('/api/automations.get')) {
      return route.fulfill(jsonResponse({ automation: null }))
    }
    if (url.includes('/api/automation.create') || url.includes('/api/automations.create')) {
      return route.fulfill(jsonResponse({ automation: { id: 'new-automation' } }))
    }
    if (url.includes('/api/automation.update') || url.includes('/api/automations.update')) {
      return route.fulfill(jsonResponse({ automation: { id: 'updated-automation' } }))
    }
    if (url.includes('/api/automation.delete') || url.includes('/api/automations.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    // ============================================
    // TRANSACTIONAL NOTIFICATIONS
    // ============================================
    if (url.includes('/api/transactional.list')) {
      return route.fulfill(jsonResponse(withData ? mockTransactionalResponse : mockEmptyTransactional))
    }
    if (url.includes('/api/transactional.get')) {
      return route.fulfill(jsonResponse({ notification: mockTransactionalResponse.notifications[0] }))
    }
    if (url.includes('/api/transactional.create')) {
      return route.fulfill(jsonResponse(mockTransactionalCreateResponse))
    }
    if (url.includes('/api/transactional.update')) {
      return route.fulfill(jsonResponse(mockTransactionalCreateResponse))
    }
    if (url.includes('/api/transactional.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }
    if (url.includes('/api/transactional.send_test')) {
      return route.fulfill(jsonResponse(mockTestEmailResponse))
    }

    // ============================================
    // BLOG (supports blogPosts.*, blog.post.*, and blog_post.* patterns)
    // ============================================
    if (url.includes('/api/blogPosts.list') || url.includes('/api/blog.post.list') || url.includes('/api/blog_post.list')) {
      return route.fulfill(jsonResponse(mockBlogPostsResponse))
    }
    if (url.includes('/api/blogPosts.get') || url.includes('/api/blog.post.get') || url.includes('/api/blog_post.get')) {
      return route.fulfill(jsonResponse({ post: mockBlogPostsResponse.posts[0] }))
    }
    if (url.includes('/api/blogPosts.create') || url.includes('/api/blog.post.create') || url.includes('/api/blog_post.create')) {
      return route.fulfill(jsonResponse(mockBlogPostCreateResponse))
    }
    if (url.includes('/api/blogPosts.update') || url.includes('/api/blog.post.update') || url.includes('/api/blog_post.update')) {
      return route.fulfill(jsonResponse(mockBlogPostCreateResponse))
    }
    if (url.includes('/api/blogPosts.delete') || url.includes('/api/blog.post.delete') || url.includes('/api/blog_post.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }
    if (url.includes('/api/blogPosts.publish') || url.includes('/api/blog.post.publish') || url.includes('/api/blog_post.publish')) {
      return route.fulfill(
        jsonResponse({
          post: { ...mockBlogPostsResponse.posts[0], status: 'published' }
        })
      )
    }
    if (url.includes('/api/blogPosts.unpublish') || url.includes('/api/blog.post.unpublish') || url.includes('/api/blog_post.unpublish')) {
      return route.fulfill(
        jsonResponse({
          post: { ...mockBlogPostsResponse.posts[0], status: 'draft' }
        })
      )
    }

    if (url.includes('/api/blogCategories.list') || url.includes('/api/blog.category.list') || url.includes('/api/blog_category.list')) {
      return route.fulfill(jsonResponse(mockBlogCategoriesResponse))
    }
    if (url.includes('/api/blogCategories.get') || url.includes('/api/blog.category.get') || url.includes('/api/blog_category.get')) {
      return route.fulfill(jsonResponse({ category: mockBlogCategoriesResponse.categories[0] }))
    }
    if (url.includes('/api/blogCategories.create') || url.includes('/api/blog.category.create') || url.includes('/api/blog_category.create')) {
      return route.fulfill(jsonResponse(mockBlogCategoryCreateResponse))
    }
    if (url.includes('/api/blogCategories.update') || url.includes('/api/blog.category.update') || url.includes('/api/blog_category.update')) {
      return route.fulfill(jsonResponse(mockBlogCategoryCreateResponse))
    }
    if (url.includes('/api/blogCategories.delete') || url.includes('/api/blog.category.delete') || url.includes('/api/blog_category.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    if (url.includes('/api/blogThemes.list') || url.includes('/api/blog.theme.list') || url.includes('/api/blog_theme.list')) {
      return route.fulfill(jsonResponse(mockBlogThemesResponse))
    }
    if (url.includes('/api/blogThemes.get') || url.includes('/api/blogThemes.getPublished') || url.includes('/api/blog.theme.get') || url.includes('/api/blog_theme.get')) {
      return route.fulfill(jsonResponse({ theme: mockBlogThemesResponse.themes[0] }))
    }
    if (url.includes('/api/blogThemes.update') || url.includes('/api/blog.theme.update') || url.includes('/api/blog_theme.update')) {
      return route.fulfill(jsonResponse({ theme: mockBlogThemesResponse.themes[0] }))
    }
    if (url.includes('/api/blogThemes.publish') || url.includes('/api/blog.theme.publish') || url.includes('/api/blog_theme.publish')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    // Generic blog routes
    if (url.includes('/api/blog')) {
      return route.fulfill(jsonResponse(mockBlogPostsResponse))
    }

    // ============================================
    // LOGS & ANALYTICS (supports both singular and plural API routes)
    // ============================================
    if (
      url.includes('/api/log.list') ||
      url.includes('/api/logs.list') ||
      url.includes('/api/message.list') ||
      url.includes('/api/messages.list')
    ) {
      return route.fulfill(jsonResponse(mockEmptyLogs))
    }
    if (url.includes('/api/analytics')) {
      return route.fulfill(jsonResponse(mockAnalyticsData))
    }

    // ============================================
    // FILES
    // ============================================
    if (url.includes('/api/file.list')) {
      return route.fulfill(jsonResponse(mockEmptyFiles))
    }
    if (url.includes('/api/file.upload')) {
      return route.fulfill(
        jsonResponse({
          file: {
            id: 'new-file-id',
            name: 'uploaded-file.png',
            url: 'https://cdn.example.com/files/uploaded-file.png',
            mime_type: 'image/png',
            size: 50000
          }
        })
      )
    }
    if (url.includes('/api/file.delete')) {
      return route.fulfill(jsonResponse(mockSuccessResponse))
    }

    // ============================================
    // SETTINGS & INTEGRATIONS
    // ============================================
    if (url.includes('/api/email_provider')) {
      return route.fulfill(jsonResponse({ providers: [] }))
    }
    if (url.includes('/api/integration')) {
      return route.fulfill(jsonResponse({ integrations: [] }))
    }
    if (url.includes('/api/api_key')) {
      return route.fulfill(jsonResponse({ api_keys: [] }))
    }
    if (url.includes('/api/webhook')) {
      return route.fulfill(jsonResponse({ webhooks: [] }))
    }
    if (url.includes('/api/workspace.update')) {
      return route.fulfill(jsonResponse({ workspace: mockWorkspace }))
    }

    // ============================================
    // TASKS
    // ============================================
    if (url.includes('/api/task.list') || url.includes('/api/tasks.list')) {
      return route.fulfill(jsonResponse({ tasks: [] }))
    }

    // Default: return empty success for any unhandled API requests
    console.log(`[Mock] Unhandled backend API: ${url}`)
    return route.fulfill(jsonResponse({}))
  })
}

// Extend base test with authenticated page fixture
export const test = base.extend<{
  authenticatedPage: Page
  authenticatedPageWithData: Page
}>({
  authenticatedPage: async ({ page }, use) => {
    // Clear request capture store before each test
    requestCapture.clear()

    // Setup API mocks before any navigation (empty data)
    await setupApiMocks(page, { withData: false })

    // Set auth token in localStorage via page context
    await page.addInitScript(() => {
      localStorage.setItem('auth_token', 'test-token-for-e2e')
    })

    // eslint-disable-next-line react-hooks/rules-of-hooks -- use() is Playwright's fixture API, not a React hook
    await use(page)
  },

  authenticatedPageWithData: async ({ page }, use) => {
    // Clear request capture store before each test
    requestCapture.clear()

    // Setup API mocks before any navigation (with mock data)
    await setupApiMocks(page, { withData: true })

    // Set auth token in localStorage via page context
    await page.addInitScript(() => {
      localStorage.setItem('auth_token', 'test-token-for-e2e')
    })

    // eslint-disable-next-line react-hooks/rules-of-hooks -- use() is Playwright's fixture API, not a React hook
    await use(page)
  }
})

export { expect } from '@playwright/test'
