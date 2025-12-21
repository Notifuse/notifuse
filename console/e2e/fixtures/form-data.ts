/**
 * Complete test data definitions for all form types.
 * Used by form fillers to fill ALL fields and by assertions to verify payloads.
 */

// ============================================
// SEO Settings (shared across Blog Post & Category)
// ============================================
export interface SEOFormData {
  meta_title?: string
  meta_description?: string
  keywords?: string[]
  meta_robots?: string
  canonical_url?: string
  og_title?: string
  og_description?: string
  og_image?: string
}

export const testSEOData: SEOFormData = {
  meta_title: 'Test SEO Title for Search Engines',
  meta_description: 'This is a comprehensive meta description for testing SEO settings persistence in e2e tests.',
  keywords: ['test', 'e2e', 'seo', 'playwright'],
  meta_robots: 'index,follow',
  canonical_url: 'https://example.com/canonical-url',
  og_title: 'Test Open Graph Title',
  og_description: 'Open Graph description for social media sharing',
  og_image: 'https://example.com/og-image.jpg'
}

// ============================================
// Contact Form Data
// ============================================
export interface ContactFormData {
  email: string
  external_id?: string
  timezone?: string
  language?: string
  first_name?: string
  last_name?: string
  phone?: string
  address_line_1?: string
  address_line_2?: string
  country?: string
  postcode?: string
  state?: string
  job_title?: string
  custom_string_1?: string
  custom_string_2?: string
  custom_string_3?: string
  custom_string_4?: string
  custom_string_5?: string
  custom_number_1?: number
  custom_number_2?: number
  custom_number_3?: number
  custom_number_4?: number
  custom_number_5?: number
  custom_datetime_1?: string
  custom_datetime_2?: string
  custom_datetime_3?: string
  custom_datetime_4?: string
  custom_datetime_5?: string
}

// Minimal contact data for basic payload verification tests
export const testContactDataMinimal: ContactFormData = {
  email: 'e2e-test@example.com'
}

// Full contact data with all optional fields
export const testContactData: ContactFormData = {
  email: 'e2e-test@example.com',
  external_id: 'ext-123456',
  timezone: 'America/New_York',
  language: 'en',
  first_name: 'Test',
  last_name: 'Contact',
  phone: '+1-555-123-4567',
  address_line_1: '123 Test Street',
  address_line_2: 'Suite 456',
  country: 'US',
  postcode: '10001',
  state: 'NY',
  job_title: 'Test Engineer',
  custom_string_1: 'Custom String Value 1',
  custom_string_2: 'Custom String Value 2',
  custom_string_3: 'Custom String Value 3',
  custom_string_4: 'Custom String Value 4',
  custom_string_5: 'Custom String Value 5',
  custom_number_1: 100,
  custom_number_2: 200,
  custom_number_3: 300,
  custom_number_4: 400,
  custom_number_5: 500,
  custom_datetime_1: '2024-01-15T10:30:00Z',
  custom_datetime_2: '2024-02-20T14:45:00Z',
  custom_datetime_3: '2024-03-25T09:00:00Z',
  custom_datetime_4: '2024-04-30T16:15:00Z',
  custom_datetime_5: '2024-05-05T11:30:00Z'
}

// ============================================
// List Form Data
// ============================================
export interface ListFormData {
  id: string
  name: string
  description?: string
  is_double_optin: boolean
  is_public: boolean
  double_optin_template_id?: string
  double_optin_template_version?: number
}

export const testListData: ListFormData = {
  id: 'test-list-e2e',
  name: 'E2E Test List',
  description: 'A comprehensive test list created by e2e tests to verify all form fields are correctly submitted.',
  is_double_optin: true,
  is_public: true
  // Template IDs would be filled if templates exist in mock data
}

// ============================================
// Template Form Data
// ============================================
export interface TemplateFormData {
  id: string
  name: string
  channel: 'email' | 'web'
  category: string
  subject?: string
  subject_preview?: string
  sender_id?: string
  reply_to?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, unknown>
}

export const testTemplateData: TemplateFormData = {
  id: 'test-template-e2e',
  name: 'E2E Test Template',
  channel: 'email',
  category: 'marketing',
  subject: 'Test Email Subject for E2E',
  subject_preview: 'Preview text shown in email clients',
  utm_source: 'e2e-test',
  utm_medium: 'email',
  utm_campaign: 'test-campaign-2024',
  test_data: {
    first_name: 'Test',
    last_name: 'User',
    company: 'Test Corp'
  }
}

// ============================================
// Broadcast Form Data
// ============================================
export interface BroadcastFormData {
  name: string
  // Audience
  list?: string
  segments?: string[]
  exclude_unsubscribed: boolean
  // Schedule
  is_scheduled: boolean
  scheduled_date?: string
  scheduled_time?: string
  timezone?: string
  use_recipient_timezone: boolean
  // UTM
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  utm_term?: string
  utm_content?: string
  // A/B Testing
  test_enabled: boolean
  test_sample_percentage?: number
  test_auto_send_winner?: boolean
  test_auto_send_winner_metric?: 'open_rate' | 'click_rate'
  test_duration_hours?: number
  variations?: Array<{
    variation_name: string
    template_id: string
  }>
}

export const testBroadcastData: BroadcastFormData = {
  name: 'E2E Test Broadcast Campaign',
  exclude_unsubscribed: true,
  is_scheduled: false,
  use_recipient_timezone: false,
  utm_source: 'newsletter',
  utm_medium: 'email',
  utm_campaign: 'e2e-test-campaign',
  utm_term: 'test-term',
  utm_content: 'test-content-variation',
  test_enabled: false
}

export const testBroadcastWithABData: BroadcastFormData = {
  ...testBroadcastData,
  name: 'E2E Test A/B Broadcast',
  test_enabled: true,
  test_sample_percentage: 20,
  test_auto_send_winner: true,
  test_auto_send_winner_metric: 'open_rate',
  test_duration_hours: 4
}

// ============================================
// Segment Form Data
// ============================================
export interface SegmentFormData {
  name: string
  description?: string
  // Tree structure for conditions would be complex
  // We'll use a simplified representation for tests
}

export const testSegmentData: SegmentFormData = {
  name: 'E2E Test Segment',
  description: 'A test segment to verify segment creation with conditions'
}

// ============================================
// Transactional Notification Form Data
// ============================================
export interface TransactionalFormData {
  id: string
  name: string
  description?: string
  email_template_id?: string
  tracking_enabled: boolean
  tracking_opens: boolean
  tracking_clicks: boolean
}

export const testTransactionalData: TransactionalFormData = {
  id: 'test-transactional-e2e',
  name: 'E2E Test Transactional Notification',
  description: 'A transactional notification for order confirmations, created by e2e tests.',
  tracking_enabled: true,
  tracking_opens: true,
  tracking_clicks: true
}

// ============================================
// Blog Author
// ============================================
export interface BlogAuthorFormData {
  name: string
  avatar_url?: string
}

export const testBlogAuthorData: BlogAuthorFormData = {
  name: 'Test Author',
  avatar_url: 'https://example.com/avatar.jpg'
}

// ============================================
// Blog Post Form Data
// ============================================
export interface BlogPostFormData {
  title: string
  slug: string
  category_id?: string
  excerpt?: string
  featured_image_url?: string
  authors: BlogAuthorFormData[]
  reading_time_minutes: number
  seo: SEOFormData
}

export const testBlogPostData: BlogPostFormData = {
  title: 'E2E Test Blog Post with Full SEO Settings',
  slug: 'e2e-test-blog-post-with-seo',
  excerpt: 'This is a comprehensive test excerpt for the blog post. It tests the excerpt field persistence along with all other fields.',
  featured_image_url: 'https://example.com/featured-image.jpg',
  authors: [testBlogAuthorData],
  reading_time_minutes: 5,
  seo: testSEOData
}

// ============================================
// Blog Category Form Data
// ============================================
export interface BlogCategoryFormData {
  name: string
  slug: string
  description?: string
  seo: SEOFormData
}

export const testBlogCategoryData: BlogCategoryFormData = {
  name: 'E2E Test Category',
  slug: 'e2e-test-category',
  description: 'A test category created by e2e tests to verify all form fields including SEO settings.',
  seo: testSEOData
}

// ============================================
// Workspace Settings Form Data
// ============================================
export interface WorkspaceSettingsFormData {
  name: string
  timezone?: string
  custom_endpoint_url?: string
  // Add more fields as needed
}

export const testWorkspaceSettingsData: WorkspaceSettingsFormData = {
  name: 'E2E Test Workspace',
  timezone: 'America/New_York',
  custom_endpoint_url: 'https://custom.example.com'
}
