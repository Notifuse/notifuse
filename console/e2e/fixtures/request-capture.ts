import { Request } from '@playwright/test'

/**
 * Represents a captured API request with its payload
 */
export interface CapturedRequest {
  url: string
  method: string
  body: Record<string, unknown> | null
  timestamp: number
}

/**
 * Store for capturing and retrieving API request payloads during tests.
 * This allows tests to verify that form data is correctly sent to the API.
 */
export class RequestCaptureStore {
  private requests: Map<string, CapturedRequest[]> = new Map()

  /**
   * Capture a request for a specific API pattern
   */
  capture(pattern: string, request: CapturedRequest): void {
    const existing = this.requests.get(pattern) || []
    existing.push(request)
    this.requests.set(pattern, existing)
  }

  /**
   * Get the last captured request for a pattern
   */
  getLastRequest(pattern: string): CapturedRequest | undefined {
    const requests = this.requests.get(pattern)
    return requests?.[requests.length - 1]
  }

  /**
   * Get all captured requests for a pattern
   */
  getAllRequests(pattern: string): CapturedRequest[] {
    return this.requests.get(pattern) || []
  }

  /**
   * Get the count of captured requests for a pattern
   */
  getRequestCount(pattern: string): number {
    return this.requests.get(pattern)?.length || 0
  }

  /**
   * Clear all captured requests (should be called before each test)
   */
  clear(): void {
    this.requests.clear()
  }

  /**
   * Get all patterns that have captured requests
   */
  getPatterns(): string[] {
    return Array.from(this.requests.keys())
  }
}

/**
 * Parse the request body as JSON
 */
export async function parseRequestBody(request: Request): Promise<Record<string, unknown> | null> {
  try {
    const postData = request.postData()
    if (postData) {
      return JSON.parse(postData)
    }
  } catch {
    // Body might not be JSON, return null
  }
  return null
}

/**
 * API endpoint patterns for request capture.
 * Use these constants when capturing and asserting requests.
 */
export const API_PATTERNS = {
  // Contacts
  CONTACT_UPSERT: 'contact.upsert',
  CONTACT_CREATE: 'contact.create',
  CONTACT_DELETE: 'contact.delete',
  CONTACT_IMPORT: 'contact.import',
  CONTACT_BULK: 'contact.bulk',

  // Lists
  LIST_CREATE: 'list.create',
  LIST_UPDATE: 'list.update',
  LIST_DELETE: 'list.delete',

  // Templates
  TEMPLATE_CREATE: 'template.create',
  TEMPLATE_UPDATE: 'template.update',
  TEMPLATE_DELETE: 'template.delete',

  // Broadcasts
  BROADCAST_CREATE: 'broadcast.create',
  BROADCAST_UPDATE: 'broadcast.update',
  BROADCAST_DELETE: 'broadcast.delete',
  BROADCAST_SCHEDULE: 'broadcast.schedule',
  BROADCAST_SEND: 'broadcast.send',

  // Segments
  SEGMENT_CREATE: 'segment.create',
  SEGMENT_UPDATE: 'segment.update',
  SEGMENT_DELETE: 'segment.delete',
  SEGMENT_PREVIEW: 'segment.preview',

  // Transactional
  TRANSACTIONAL_CREATE: 'transactional.create',
  TRANSACTIONAL_UPDATE: 'transactional.update',
  TRANSACTIONAL_DELETE: 'transactional.delete',

  // Blog Posts
  BLOG_POST_CREATE: 'blog.post.create',
  BLOG_POST_UPDATE: 'blog.post.update',
  BLOG_POST_DELETE: 'blog.post.delete',
  BLOG_POST_PUBLISH: 'blog.post.publish',

  // Blog Categories
  BLOG_CATEGORY_CREATE: 'blog.category.create',
  BLOG_CATEGORY_UPDATE: 'blog.category.update',
  BLOG_CATEGORY_DELETE: 'blog.category.delete',

  // Workspace
  WORKSPACE_UPDATE: 'workspace.update'
} as const

export type ApiPattern = (typeof API_PATTERNS)[keyof typeof API_PATTERNS]

/**
 * Map URL patterns to API_PATTERNS constants
 */
export function getPatternFromUrl(url: string): ApiPattern | null {
  // Contact endpoints
  if (url.includes('/api/contact.upsert') || url.includes('/api/contacts.upsert')) {
    return API_PATTERNS.CONTACT_UPSERT
  }
  if (url.includes('/api/contact.create') || url.includes('/api/contacts.create')) {
    return API_PATTERNS.CONTACT_CREATE
  }
  if (url.includes('/api/contact.delete') || url.includes('/api/contacts.delete')) {
    return API_PATTERNS.CONTACT_DELETE
  }
  if (url.includes('/api/contact.import') || url.includes('/api/contacts.import')) {
    return API_PATTERNS.CONTACT_IMPORT
  }
  if (url.includes('/api/contact.bulk') || url.includes('/api/contacts.bulk')) {
    return API_PATTERNS.CONTACT_BULK
  }

  // List endpoints
  if (url.includes('/api/list.create') || url.includes('/api/lists.create')) {
    return API_PATTERNS.LIST_CREATE
  }
  if (url.includes('/api/list.update') || url.includes('/api/lists.update')) {
    return API_PATTERNS.LIST_UPDATE
  }
  if (url.includes('/api/list.delete') || url.includes('/api/lists.delete')) {
    return API_PATTERNS.LIST_DELETE
  }

  // Template endpoints
  if (url.includes('/api/template.create') || url.includes('/api/templates.create')) {
    return API_PATTERNS.TEMPLATE_CREATE
  }
  if (url.includes('/api/template.update') || url.includes('/api/templates.update')) {
    return API_PATTERNS.TEMPLATE_UPDATE
  }
  if (url.includes('/api/template.delete') || url.includes('/api/templates.delete')) {
    return API_PATTERNS.TEMPLATE_DELETE
  }

  // Broadcast endpoints
  if (url.includes('/api/broadcast.create') || url.includes('/api/broadcasts.create')) {
    return API_PATTERNS.BROADCAST_CREATE
  }
  if (url.includes('/api/broadcast.update') || url.includes('/api/broadcasts.update')) {
    return API_PATTERNS.BROADCAST_UPDATE
  }
  if (url.includes('/api/broadcast.delete') || url.includes('/api/broadcasts.delete')) {
    return API_PATTERNS.BROADCAST_DELETE
  }
  if (url.includes('/api/broadcast.schedule') || url.includes('/api/broadcasts.schedule')) {
    return API_PATTERNS.BROADCAST_SCHEDULE
  }
  if (url.includes('/api/broadcast.send') || url.includes('/api/broadcasts.send')) {
    return API_PATTERNS.BROADCAST_SEND
  }

  // Segment endpoints
  if (url.includes('/api/segment.create') || url.includes('/api/segments.create')) {
    return API_PATTERNS.SEGMENT_CREATE
  }
  if (url.includes('/api/segment.update') || url.includes('/api/segments.update')) {
    return API_PATTERNS.SEGMENT_UPDATE
  }
  if (url.includes('/api/segment.delete') || url.includes('/api/segments.delete')) {
    return API_PATTERNS.SEGMENT_DELETE
  }
  if (url.includes('/api/segment.preview') || url.includes('/api/segments.preview')) {
    return API_PATTERNS.SEGMENT_PREVIEW
  }

  // Transactional endpoints
  if (url.includes('/api/transactional.create')) {
    return API_PATTERNS.TRANSACTIONAL_CREATE
  }
  if (url.includes('/api/transactional.update')) {
    return API_PATTERNS.TRANSACTIONAL_UPDATE
  }
  if (url.includes('/api/transactional.delete')) {
    return API_PATTERNS.TRANSACTIONAL_DELETE
  }

  // Blog post endpoints
  if (url.includes('/api/blog.post.create') || url.includes('/api/blog_post.create')) {
    return API_PATTERNS.BLOG_POST_CREATE
  }
  if (url.includes('/api/blog.post.update') || url.includes('/api/blog_post.update')) {
    return API_PATTERNS.BLOG_POST_UPDATE
  }
  if (url.includes('/api/blog.post.delete') || url.includes('/api/blog_post.delete')) {
    return API_PATTERNS.BLOG_POST_DELETE
  }
  if (url.includes('/api/blog.post.publish') || url.includes('/api/blog_post.publish')) {
    return API_PATTERNS.BLOG_POST_PUBLISH
  }

  // Blog category endpoints
  if (url.includes('/api/blog.category.create') || url.includes('/api/blog_category.create')) {
    return API_PATTERNS.BLOG_CATEGORY_CREATE
  }
  if (url.includes('/api/blog.category.update') || url.includes('/api/blog_category.update')) {
    return API_PATTERNS.BLOG_CATEGORY_UPDATE
  }
  if (url.includes('/api/blog.category.delete') || url.includes('/api/blog_category.delete')) {
    return API_PATTERNS.BLOG_CATEGORY_DELETE
  }

  // Workspace endpoints
  if (url.includes('/api/workspace.update')) {
    return API_PATTERNS.WORKSPACE_UPDATE
  }

  return null
}
