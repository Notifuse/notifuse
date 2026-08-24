// Mock data for E2E tests

export const mockUser = {
  id: 'test-user-id',
  email: 'test@example.com',
  timezone: 'UTC'
}

export const mockWorkspace = {
  id: 'test-workspace',
  name: 'Test Workspace',
  settings: {
    timezone: 'UTC',
    custom_fields: {
      company: { type: 'string', label: 'Company' },
      plan: { type: 'string', label: 'Plan' }
    }
  },
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z'
}

export const mockWorkspaces = [mockWorkspace]

export const mockUserMeResponse = {
  user: mockUser,
  workspaces: mockWorkspaces
}

// ============================================
// CONTACTS
// ============================================

// Shaped after the Contact interface in src/services/api/contacts.ts: `contact_lists`
// is a required field there, and the contacts table reads `record.contact_lists.map`
// unconditionally, so a contact without it crashes the whole table.
export const mockContacts = [
  {
    id: 'contact-1',
    email: 'john@example.com',
    external_id: 'ext-1',
    first_name: 'John',
    last_name: 'Doe',
    phone: '+1234567890',
    address_line_1: '123 Main St',
    address_line_2: 'Apt 4',
    city: 'New York',
    state: 'NY',
    country: 'US',
    postcode: '10001',
    language: 'en',
    timezone: 'America/New_York',
    custom_string_1: 'Acme Corp',
    custom_string_2: 'Pro',
    created_at: '2024-01-15T10:30:00Z',
    updated_at: '2024-01-20T14:00:00Z',
    contact_lists: [
      {
        email: 'john@example.com',
        list_id: 'list-1',
        status: 'active',
        created_at: '2024-01-15T10:30:00Z',
        updated_at: '2024-01-15T10:30:00Z'
      }
    ]
  },
  {
    id: 'contact-2',
    email: 'jane@example.com',
    external_id: 'ext-2',
    first_name: 'Jane',
    last_name: 'Smith',
    phone: '+0987654321',
    address_line_1: '456 Oak Ave',
    address_line_2: null,
    city: 'Los Angeles',
    state: 'CA',
    country: 'US',
    postcode: '90001',
    language: 'en',
    timezone: 'America/Los_Angeles',
    custom_string_1: 'TechCo',
    custom_string_2: 'Enterprise',
    created_at: '2024-01-10T08:00:00Z',
    updated_at: '2024-01-18T09:30:00Z',
    contact_lists: [
      {
        email: 'jane@example.com',
        list_id: 'list-1',
        status: 'active',
        created_at: '2024-01-10T08:00:00Z',
        updated_at: '2024-01-10T08:00:00Z'
      },
      {
        email: 'jane@example.com',
        list_id: 'list-2',
        status: 'unsubscribed',
        created_at: '2024-01-10T08:00:00Z',
        updated_at: '2024-01-18T09:30:00Z'
      }
    ]
  },
  {
    id: 'contact-3',
    email: 'bob@example.com',
    external_id: 'ext-3',
    first_name: 'Bob',
    last_name: 'Wilson',
    phone: '+1122334455',
    address_line_1: '789 Pine Rd',
    address_line_2: 'Suite 100',
    city: 'Chicago',
    state: 'IL',
    country: 'US',
    postcode: '60601',
    language: 'en',
    timezone: 'America/Chicago',
    custom_string_1: 'StartupInc',
    custom_string_2: 'Free',
    created_at: '2024-01-05T12:00:00Z',
    updated_at: '2024-01-16T16:45:00Z',
    contact_lists: []
  }
]

export const mockContactsResponse = {
  contacts: mockContacts,
  total: 3,
  next_cursor: null
}

export const mockEmptyContacts = {
  contacts: [],
  total: 0,
  next_cursor: null
}

export const mockTotalContacts = {
  total_contacts: 3
}

// ============================================
// LISTS
// ============================================

export const mockLists = [
  {
    id: 'list-1',
    name: 'Newsletter',
    description: 'Monthly newsletter subscribers',
    is_double_optin: true,
    is_public: true,
    stats: {
      active: 150,
      pending: 25,
      unsubscribed: 10
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z'
  },
  {
    id: 'list-2',
    name: 'Marketing Updates',
    description: 'Product updates and marketing campaigns',
    is_double_optin: false,
    is_public: true,
    stats: {
      active: 320,
      pending: 0,
      unsubscribed: 45
    },
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-20T00:00:00Z'
  },
  {
    id: 'list-3',
    name: 'Beta Testers',
    description: 'Early access beta testing group',
    is_double_optin: true,
    is_public: false,
    stats: {
      active: 50,
      pending: 5,
      unsubscribed: 2
    },
    created_at: '2024-01-10T00:00:00Z',
    updated_at: '2024-01-18T00:00:00Z'
  }
]

export const mockListsResponse = {
  lists: mockLists
}

export const mockEmptyLists = {
  lists: []
}

// Per-list subscriber counts served by /api/lists.stats, which the list cards query
// separately from /api/lists.list.
export const mockListStats: Record<string, Record<string, number>> = {
  'list-1': {
    total_active: 150,
    total_pending: 25,
    total_unsubscribed: 10,
    total_bounced: 3,
    total_complained: 1
  },
  'list-2': {
    total_active: 320,
    total_pending: 0,
    total_unsubscribed: 45,
    total_bounced: 8,
    total_complained: 2
  },
  'list-3': {
    total_active: 50,
    total_pending: 5,
    total_unsubscribed: 2,
    total_bounced: 0,
    total_complained: 0
  }
}

export const emptyListStats = {
  total_active: 0,
  total_pending: 0,
  total_unsubscribed: 0,
  total_bounced: 0,
  total_complained: 0
}

// ============================================
// TEMPLATES
// ============================================

// The smallest tree the email builder accepts: an mjml root wrapping a single text
// block, mirroring the EmailBlock union in src/components/email_builder/types.ts.
// EmailTemplate.visual_editor_tree is required, and the drawer reads it to seed the
// editor, so every mocked email carries a real one instead of a placeholder.
const mockEmailTree = (slug: string, text: string) => ({
  id: `mjml-${slug}`,
  type: 'mjml',
  children: [
    {
      id: `body-${slug}`,
      type: 'mj-body',
      children: [
        {
          id: `section-${slug}`,
          type: 'mj-section',
          children: [
            {
              id: `column-${slug}`,
              type: 'mj-column',
              children: [
                {
                  id: `text-${slug}`,
                  type: 'mj-text',
                  content: text
                }
              ]
            }
          ]
        }
      ]
    }
  ]
})

// Shaped after the Template interface in src/services/api/template.ts: the email payload
// is nested under `email` (subject, subject_preview, reply_to, compiled_preview,
// visual_editor_tree, editor_mode), which is what the templates table's Subject column and
// CreateTemplateDrawer.showDrawer() read. A template carrying a top-level `subject` lists
// with an empty Subject column and edits as if it had no email at all.
export const mockTemplates = [
  {
    id: 'tpl-1',
    name: 'Welcome Email',
    version: 1,
    channel: 'email',
    category: 'welcome',
    email: {
      editor_mode: 'visual',
      subject: 'Welcome to {{workspace.name}}!',
      subject_preview: 'Your account is ready',
      reply_to: 'support@example.com',
      compiled_preview: '<html><body><p>Welcome!</p></body></html>',
      visual_editor_tree: mockEmailTree('welcome', 'Welcome, {{contact.first_name}}!')
    },
    utm_source: 'email',
    utm_medium: 'newsletter',
    utm_campaign: 'welcome',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'tpl-2',
    name: 'Monthly Newsletter',
    version: 1,
    channel: 'email',
    category: 'marketing',
    email: {
      editor_mode: 'visual',
      subject: '{{workspace.name}} Newsletter - {{date}}',
      subject_preview: 'This month at a glance',
      compiled_preview: '<html><body><p>Newsletter</p></body></html>',
      visual_editor_tree: mockEmailTree(
        'newsletter',
        'Hello {{contact.first_name}}, here are our updates!'
      )
    },
    utm_source: 'email',
    utm_medium: 'newsletter',
    utm_campaign: 'monthly',
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z'
  },
  {
    id: 'tpl-3',
    name: 'Unsubscribe Confirmation',
    version: 1,
    channel: 'email',
    category: 'transactional',
    email: {
      editor_mode: 'visual',
      subject: "You've been unsubscribed",
      subject_preview: 'You will not hear from us again',
      compiled_preview: '<html><body><p>Unsubscribed</p></body></html>',
      visual_editor_tree: mockEmailTree('unsubscribe', "We're sorry to see you go!")
    },
    created_at: '2024-01-08T00:00:00Z',
    updated_at: '2024-01-12T00:00:00Z'
  }
]

export const mockTemplatesResponse = {
  templates: mockTemplates
}

export const mockEmptyTemplates = {
  templates: []
}

export const mockCompiledTemplate = {
  html: '<html><body><p>Welcome, John!</p></body></html>'
}

// ============================================
// BROADCASTS
// ============================================

// Shaped after the Broadcast interface in src/services/api/broadcast.ts: the page
// reads test_settings.variations unconditionally, so a broadcast without
// test_settings crashes the whole list.
export const mockBroadcasts = [
  {
    id: 'bc-1',
    workspace_id: 'test-workspace',
    name: 'January Newsletter',
    channel_type: 'email',
    status: 'draft',
    audience: {
      list: 'list-1',
      segments: [],
      exclude_unsubscribed: true
    },
    schedule: {
      is_scheduled: false,
      use_recipient_timezone: false
    },
    test_settings: {
      enabled: false,
      sample_percentage: 0,
      auto_send_winner: false,
      variations: [{ variation_name: 'A', template_id: 'tpl-2' }]
    },
    utm_parameters: {
      source: 'notifuse',
      medium: 'email',
      campaign: 'january-newsletter'
    },
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    created_at: '2024-01-20T00:00:00Z',
    updated_at: '2024-01-20T00:00:00Z'
  },
  {
    id: 'bc-2',
    workspace_id: 'test-workspace',
    name: 'Product Launch',
    channel_type: 'email',
    status: 'processed',
    audience: {
      list: 'list-2',
      segments: ['seg-1'],
      exclude_unsubscribed: true
    },
    schedule: {
      is_scheduled: true,
      scheduled_date: '2024-01-15',
      scheduled_time: '10:00',
      timezone: 'UTC',
      use_recipient_timezone: false
    },
    test_settings: {
      enabled: false,
      sample_percentage: 0,
      auto_send_winner: false,
      variations: [{ variation_name: 'A', template_id: 'tpl-2' }]
    },
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 500,
    created_at: '2024-01-10T00:00:00Z',
    updated_at: '2024-01-15T10:30:00Z',
    started_at: '2024-01-15T10:00:00Z',
    completed_at: '2024-01-15T10:30:00Z'
  },
  {
    id: 'bc-3',
    workspace_id: 'test-workspace',
    name: 'A/B Test Campaign',
    channel_type: 'email',
    status: 'scheduled',
    audience: {
      list: 'list-2',
      segments: [],
      exclude_unsubscribed: true
    },
    schedule: {
      is_scheduled: true,
      scheduled_date: '2024-02-01',
      scheduled_time: '09:00',
      timezone: 'UTC',
      use_recipient_timezone: false
    },
    test_settings: {
      enabled: true,
      sample_percentage: 20,
      auto_send_winner: true,
      auto_send_winner_metric: 'open_rate',
      test_duration_hours: 4,
      variations: [
        { variation_name: 'A', template_id: 'tpl-1' },
        { variation_name: 'B', template_id: 'tpl-2' }
      ]
    },
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    created_at: '2024-01-25T00:00:00Z',
    updated_at: '2024-01-25T00:00:00Z'
  }
]

export const mockBroadcastsResponse = {
  broadcasts: mockBroadcasts,
  total_count: 3
}

export const mockEmptyBroadcasts = {
  broadcasts: [],
  total_count: 0
}

// ============================================
// TRANSACTIONAL NOTIFICATIONS
// ============================================

// Shaped after the TransactionalNotification interface in
// src/services/api/transactional_notifications.ts: the template lives under
// channels.email.template_id and the UTM/tracking fields under tracking_settings.
export const mockTransactionalNotifications = [
  {
    id: 'transactional-1',
    name: 'Password Reset',
    description: 'Sent when user requests password reset',
    channels: {
      email: { template_id: 'tpl-1' }
    },
    tracking_settings: {
      enable_tracking: true,
      tracking_mode: 'inherit',
      utm_source: 'notifuse',
      utm_medium: 'email',
      utm_campaign: 'transactional',
      utm_content: 'password_reset'
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z'
  },
  {
    id: 'transactional-2',
    name: 'Order Confirmation',
    description: 'Sent after successful order',
    channels: {
      email: { template_id: 'tpl-2' }
    },
    tracking_settings: {
      enable_tracking: true,
      tracking_mode: 'inherit',
      utm_source: 'notifuse',
      utm_medium: 'email',
      utm_campaign: 'transactional',
      utm_content: 'order_confirmation'
    },
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'transactional-3',
    name: 'Account Verification',
    description: 'Email verification for new accounts',
    channels: {
      email: { template_id: 'tpl-1' }
    },
    tracking_settings: {
      enable_tracking: false,
      tracking_mode: 'disabled'
    },
    created_at: '2024-01-08T00:00:00Z',
    updated_at: '2024-01-08T00:00:00Z'
  }
]

export const mockTransactionalResponse = {
  notifications: mockTransactionalNotifications
}

export const mockEmptyTransactional = {
  notifications: []
}

// ============================================
// SEGMENTS
// ============================================

export const mockSegments = [
  {
    id: 'seg-1',
    name: 'Active Users',
    description: 'Users who opened email in last 30 days',
    contact_count: 150,
    status: 'ready',
    rules: {
      operator: 'and',
      conditions: [
        {
          field: 'last_opened_at',
          operator: 'greater_than',
          value: '30_days_ago'
        }
      ]
    },
    last_built_at: '2024-01-20T00:00:00Z',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-20T00:00:00Z'
  },
  {
    id: 'seg-2',
    name: 'US Customers',
    description: 'Contacts located in the United States',
    contact_count: 250,
    status: 'ready',
    rules: {
      operator: 'and',
      conditions: [
        {
          field: 'country',
          operator: 'equals',
          value: 'US'
        }
      ]
    },
    last_built_at: '2024-01-19T00:00:00Z',
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-19T00:00:00Z'
  },
  {
    id: 'seg-3',
    name: 'Enterprise Plans',
    description: 'Contacts on enterprise plans',
    contact_count: 45,
    status: 'building',
    rules: {
      operator: 'or',
      conditions: [
        {
          field: 'custom_string_2',
          operator: 'equals',
          value: 'Enterprise'
        },
        {
          field: 'custom_string_2',
          operator: 'equals',
          value: 'Pro'
        }
      ]
    },
    last_built_at: null,
    created_at: '2024-01-15T00:00:00Z',
    updated_at: '2024-01-22T00:00:00Z'
  }
]

export const mockSegmentsResponse = {
  segments: mockSegments
}

export const mockEmptySegments = {
  segments: []
}

// ============================================
// BLOG
// ============================================

export const mockBlogCategories = [
  {
    id: 'cat-1',
    slug: 'engineering',
    settings: {
      name: 'Engineering',
      description: 'Technical articles and tutorials',
      seo: {
        meta_title: 'Engineering Blog',
        meta_description: 'Technical articles about our engineering practices'
      }
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'cat-2',
    slug: 'product-updates',
    settings: {
      name: 'Product Updates',
      description: 'New features and improvements',
      seo: {
        meta_title: 'Product Updates',
        meta_description: 'Latest product news and feature announcements'
      }
    },
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z'
  },
  {
    id: 'cat-3',
    slug: 'company-news',
    settings: {
      name: 'Company News',
      description: 'Company announcements and news',
      seo: {
        meta_title: 'Company News',
        meta_description: 'Stay updated with company announcements'
      }
    },
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-12T00:00:00Z'
  }
]

export const mockBlogPosts = [
  {
    id: 'post-1',
    slug: 'getting-started-email-marketing',
    category_id: 'cat-1',
    settings: {
      title: 'Getting Started with Email Marketing',
      excerpt: 'Learn the basics of email marketing and how to get started.',
      featured_image_url: 'https://example.com/images/post-1.jpg',
      authors: [{ name: 'Test User', avatar_url: null }],
      reading_time_minutes: 5,
      template: { template_id: 'tpl-1', template_version: 1 },
      seo: {
        meta_title: 'Getting Started with Email Marketing - Guide',
        meta_description: 'Complete guide to getting started with email marketing'
      }
    },
    published_at: '2024-01-10T10:00:00Z',
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-10T10:00:00Z'
  },
  {
    id: 'post-2',
    slug: 'new-feature-ab-testing',
    category_id: 'cat-2',
    settings: {
      title: 'New Feature: A/B Testing',
      excerpt: 'Introducing our powerful new A/B testing capabilities.',
      featured_image_url: null,
      authors: [{ name: 'Test User', avatar_url: null }],
      reading_time_minutes: 3,
      template: { template_id: 'tpl-1', template_version: 1 },
      seo: {
        meta_title: 'New Feature: A/B Testing for Email Campaigns',
        meta_description: 'Learn about our new A/B testing feature'
      }
    },
    published_at: '2024-01-15T14:00:00Z',
    created_at: '2024-01-12T00:00:00Z',
    updated_at: '2024-01-15T14:00:00Z'
  },
  {
    id: 'post-3',
    slug: 'draft-post',
    category_id: 'cat-3',
    settings: {
      title: 'Draft Post',
      excerpt: 'This is a draft post that is not yet published.',
      featured_image_url: null,
      authors: [{ name: 'Test User', avatar_url: null }],
      reading_time_minutes: 2,
      template: { template_id: 'tpl-1', template_version: 1 },
      seo: {}
    },
    published_at: null,
    created_at: '2024-01-20T00:00:00Z',
    updated_at: '2024-01-22T00:00:00Z'
  },
  {
    id: 'post-4',
    slug: 'scheduled-post',
    category_id: 'cat-2',
    settings: {
      title: 'Scheduled Post',
      excerpt: 'This post is scheduled for future publication.',
      featured_image_url: 'https://example.com/images/post-4.jpg',
      authors: [{ name: 'Test User', avatar_url: null }],
      reading_time_minutes: 4,
      template: { template_id: 'tpl-1', template_version: 1 },
      seo: {
        meta_title: 'Upcoming Feature Announcement',
        meta_description: 'Big news coming soon'
      }
    },
    published_at: null,
    created_at: '2024-01-25T00:00:00Z',
    updated_at: '2024-01-25T00:00:00Z'
  }
]

export const mockBlogCategoriesResponse = {
  categories: mockBlogCategories
}

export const mockBlogPostsResponse = {
  posts: mockBlogPosts,
  // The blog list endpoint names its count `total_count` (internal/http/blog_handler.go),
  // and BlogPage reads that key to decide between the empty state and the posts table.
  total_count: 4
}

export const mockEmptyBlogPosts = {
  posts: [],
  total_count: 0
}

export const mockBlogThemes = [
  {
    id: 'theme-1',
    name: 'Default Theme',
    version: 1,
    is_active: true,
    templates: {
      home: '<html>{{posts}}</html>',
      post: '<html>{{post.title}}</html>',
      category: '<html>{{category.name}}</html>'
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  }
]

export const mockBlogThemesResponse = {
  themes: mockBlogThemes
}

// ============================================
// ANALYTICS
// ============================================

export const mockAnalyticsData = {
  data: [
    { date: '2024-01-01', sent: 100, delivered: 95, opened: 50, clicked: 20 },
    { date: '2024-01-02', sent: 150, delivered: 145, opened: 75, clicked: 30 },
    { date: '2024-01-03', sent: 200, delivered: 190, opened: 100, clicked: 45 }
  ],
  total_sent: 450,
  total_delivered: 430,
  total_opened: 225,
  total_clicked: 95
}

// ============================================
// LOGS
// ============================================

export const mockLogs = [
  {
    id: 'log-1',
    type: 'email_sent',
    contact_email: 'john@example.com',
    message: 'Email sent successfully',
    metadata: { broadcast_id: 'bc-2' },
    created_at: '2024-01-15T10:00:00Z'
  },
  {
    id: 'log-2',
    type: 'email_opened',
    contact_email: 'john@example.com',
    message: 'Email opened',
    metadata: { broadcast_id: 'bc-2' },
    created_at: '2024-01-15T11:30:00Z'
  },
  {
    id: 'log-3',
    type: 'email_clicked',
    contact_email: 'jane@example.com',
    message: 'Link clicked',
    metadata: { broadcast_id: 'bc-2', url: 'https://example.com' },
    created_at: '2024-01-15T12:00:00Z'
  }
]

export const mockLogsResponse = {
  logs: mockLogs,
  total: 3
}

export const mockEmptyLogs = {
  logs: [],
  total: 0
}

// ============================================
// FILES
// ============================================

export const mockFiles = [
  {
    id: 'file-1',
    name: 'header-image.png',
    url: 'https://cdn.example.com/files/header-image.png',
    mime_type: 'image/png',
    size: 102400,
    created_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'file-2',
    name: 'logo.svg',
    url: 'https://cdn.example.com/files/logo.svg',
    mime_type: 'image/svg+xml',
    size: 5120,
    created_at: '2024-01-05T00:00:00Z'
  }
]

export const mockFilesResponse = {
  files: mockFiles,
  total: 2
}

export const mockEmptyFiles = {
  files: [],
  total: 0
}

// ============================================
// WORKSPACE MEMBERS
// ============================================

export const mockWorkspaceMembers = {
  members: [
    {
      user_id: mockUser.id,
      email: mockUser.email,
      role: 'owner',
      type: 'user',
      created_at: '2024-01-15T10:00:00Z',
      permissions: {
        contacts: { read: true, write: true },
        lists: { read: true, write: true },
        templates: { read: true, write: true },
        broadcasts: { read: true, write: true },
        transactional: { read: true, write: true },
        workspace: { read: true, write: true },
        message_history: { read: true, write: true },
        blog: { read: true, write: true }
      }
    }
  ]
}

// ============================================
// API MUTATION RESPONSES
// ============================================

export const mockSuccessResponse = {
  success: true
}

export const mockContactUpsertResponse = {
  contact: mockContacts[0]
}

export const mockContactImportResponse = {
  imported: 10,
  errors: [],
  duplicates: 2
}

export const mockListCreateResponse = {
  list: mockLists[0]
}

export const mockTemplateCreateResponse = {
  template: mockTemplates[0]
}

export const mockBroadcastCreateResponse = {
  broadcast: mockBroadcasts[0]
}

export const mockSegmentCreateResponse = {
  segment: mockSegments[0]
}

export const mockTransactionalCreateResponse = {
  notification: mockTransactionalNotifications[0]
}

export const mockBlogPostCreateResponse = {
  post: mockBlogPosts[0]
}

export const mockBlogCategoryCreateResponse = {
  category: mockBlogCategories[0]
}

export const mockTestEmailResponse = {
  sent: true,
  message_id: 'test-message-id-123'
}
