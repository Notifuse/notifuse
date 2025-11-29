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
    updated_at: '2024-01-20T14:00:00Z'
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
    updated_at: '2024-01-18T09:30:00Z'
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
    updated_at: '2024-01-16T16:45:00Z'
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
    double_opt_in: true,
    welcome_template_id: 'tpl-1',
    unsubscribe_template_id: null,
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
    double_opt_in: false,
    welcome_template_id: null,
    unsubscribe_template_id: 'tpl-3',
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
    double_opt_in: true,
    welcome_template_id: 'tpl-2',
    unsubscribe_template_id: 'tpl-3',
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

// ============================================
// TEMPLATES
// ============================================

export const mockTemplates = [
  {
    id: 'tpl-1',
    name: 'Welcome Email',
    description: 'Sent when a new subscriber joins',
    category: 'welcome',
    subject: 'Welcome to {{workspace.name}}!',
    mjml: `<mjml>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-text>Welcome, {{contact.first_name}}!</mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`,
    html: '<html><body><p>Welcome!</p></body></html>',
    from_email: 'hello@example.com',
    from_name: 'Test Workspace',
    reply_to: 'support@example.com',
    utm_source: 'email',
    utm_medium: 'newsletter',
    utm_campaign: 'welcome',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'tpl-2',
    name: 'Monthly Newsletter',
    description: 'Monthly product updates',
    category: 'marketing',
    subject: '{{workspace.name}} Newsletter - {{date}}',
    mjml: `<mjml>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-text>Hello {{contact.first_name}}, here are our updates!</mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`,
    html: '<html><body><p>Newsletter</p></body></html>',
    from_email: 'newsletter@example.com',
    from_name: 'Test Workspace',
    reply_to: null,
    utm_source: 'email',
    utm_medium: 'newsletter',
    utm_campaign: 'monthly',
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z'
  },
  {
    id: 'tpl-3',
    name: 'Unsubscribe Confirmation',
    description: 'Confirms unsubscription',
    category: 'transactional',
    subject: "You've been unsubscribed",
    mjml: `<mjml>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-text>We're sorry to see you go!</mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`,
    html: '<html><body><p>Unsubscribed</p></body></html>',
    from_email: 'hello@example.com',
    from_name: 'Test Workspace',
    reply_to: null,
    utm_source: null,
    utm_medium: null,
    utm_campaign: null,
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

export const mockBroadcasts = [
  {
    id: 'bc-1',
    name: 'January Newsletter',
    description: 'Monthly newsletter for January',
    status: 'draft',
    template_id: 'tpl-2',
    audience: {
      type: 'list',
      list_ids: ['list-1']
    },
    schedule: null,
    ab_test: null,
    stats: {
      recipients: 0,
      sent: 0,
      delivered: 0,
      opened: 0,
      clicked: 0,
      bounced: 0,
      unsubscribed: 0
    },
    created_at: '2024-01-20T00:00:00Z',
    updated_at: '2024-01-20T00:00:00Z'
  },
  {
    id: 'bc-2',
    name: 'Product Launch',
    description: 'New feature announcement',
    status: 'sent',
    template_id: 'tpl-2',
    audience: {
      type: 'segment',
      segment_ids: ['seg-1']
    },
    schedule: {
      scheduled_at: '2024-01-15T10:00:00Z',
      timezone: 'UTC'
    },
    ab_test: null,
    stats: {
      recipients: 500,
      sent: 500,
      delivered: 485,
      opened: 250,
      clicked: 75,
      bounced: 15,
      unsubscribed: 3
    },
    sent_at: '2024-01-15T10:00:00Z',
    created_at: '2024-01-10T00:00:00Z',
    updated_at: '2024-01-15T10:30:00Z'
  },
  {
    id: 'bc-3',
    name: 'A/B Test Campaign',
    description: 'Testing subject lines',
    status: 'scheduled',
    template_id: 'tpl-2',
    audience: {
      type: 'list',
      list_ids: ['list-2']
    },
    schedule: {
      scheduled_at: '2024-02-01T09:00:00Z',
      timezone: 'UTC'
    },
    ab_test: {
      enabled: true,
      test_percentage: 20,
      variations: [
        { id: 'var-a', subject: 'Check out our new features!', weight: 50 },
        { id: 'var-b', subject: 'You wont believe what we just launched', weight: 50 }
      ],
      winner_criteria: 'open_rate',
      winner_wait_hours: 4
    },
    stats: {
      recipients: 0,
      sent: 0,
      delivered: 0,
      opened: 0,
      clicked: 0,
      bounced: 0,
      unsubscribed: 0
    },
    created_at: '2024-01-25T00:00:00Z',
    updated_at: '2024-01-25T00:00:00Z'
  }
]

export const mockBroadcastsResponse = {
  broadcasts: mockBroadcasts,
  total: 3
}

export const mockEmptyBroadcasts = {
  broadcasts: [],
  total: 0
}

// ============================================
// TRANSACTIONAL NOTIFICATIONS
// ============================================

export const mockTransactionalNotifications = [
  {
    id: 'transactional-1',
    name: 'Password Reset',
    description: 'Sent when user requests password reset',
    template_id: 'tpl-1',
    tracking: {
      opens: true,
      clicks: true
    },
    utm_source: 'email',
    utm_medium: 'transactional',
    utm_campaign: 'password-reset',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z'
  },
  {
    id: 'transactional-2',
    name: 'Order Confirmation',
    description: 'Sent after successful order',
    template_id: 'tpl-2',
    tracking: {
      opens: true,
      clicks: false
    },
    utm_source: 'email',
    utm_medium: 'transactional',
    utm_campaign: 'order-confirmation',
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'transactional-3',
    name: 'Account Verification',
    description: 'Email verification for new accounts',
    template_id: 'tpl-1',
    tracking: {
      opens: false,
      clicks: true
    },
    utm_source: null,
    utm_medium: null,
    utm_campaign: null,
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
    name: 'Engineering',
    slug: 'engineering',
    description: 'Technical articles and tutorials',
    seo_title: 'Engineering Blog',
    seo_description: 'Technical articles about our engineering practices',
    post_count: 5,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z'
  },
  {
    id: 'cat-2',
    name: 'Product Updates',
    slug: 'product-updates',
    description: 'New features and improvements',
    seo_title: 'Product Updates',
    seo_description: 'Latest product news and feature announcements',
    post_count: 8,
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z'
  },
  {
    id: 'cat-3',
    name: 'Company News',
    slug: 'company-news',
    description: 'Company announcements and news',
    seo_title: 'Company News',
    seo_description: 'Stay updated with company announcements',
    post_count: 3,
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-12T00:00:00Z'
  }
]

export const mockBlogPosts = [
  {
    id: 'post-1',
    title: 'Getting Started with Email Marketing',
    slug: 'getting-started-email-marketing',
    excerpt: 'Learn the basics of email marketing and how to get started.',
    content: '<h1>Getting Started</h1><p>Email marketing is a powerful tool...</p>',
    category_id: 'cat-1',
    status: 'published',
    author_id: 'test-user-id',
    author_name: 'Test User',
    featured_image: 'https://example.com/images/post-1.jpg',
    seo_title: 'Getting Started with Email Marketing - Guide',
    seo_description: 'Complete guide to getting started with email marketing',
    published_at: '2024-01-10T10:00:00Z',
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-10T10:00:00Z'
  },
  {
    id: 'post-2',
    title: 'New Feature: A/B Testing',
    slug: 'new-feature-ab-testing',
    excerpt: 'Introducing our powerful new A/B testing capabilities.',
    content: '<h1>A/B Testing</h1><p>Test your subject lines...</p>',
    category_id: 'cat-2',
    status: 'published',
    author_id: 'test-user-id',
    author_name: 'Test User',
    featured_image: null,
    seo_title: 'New Feature: A/B Testing for Email Campaigns',
    seo_description: 'Learn about our new A/B testing feature',
    published_at: '2024-01-15T14:00:00Z',
    created_at: '2024-01-12T00:00:00Z',
    updated_at: '2024-01-15T14:00:00Z'
  },
  {
    id: 'post-3',
    title: 'Draft Post',
    slug: 'draft-post',
    excerpt: 'This is a draft post that is not yet published.',
    content: '<h1>Draft</h1><p>Work in progress...</p>',
    category_id: 'cat-3',
    status: 'draft',
    author_id: 'test-user-id',
    author_name: 'Test User',
    featured_image: null,
    seo_title: null,
    seo_description: null,
    published_at: null,
    created_at: '2024-01-20T00:00:00Z',
    updated_at: '2024-01-22T00:00:00Z'
  },
  {
    id: 'post-4',
    title: 'Scheduled Post',
    slug: 'scheduled-post',
    excerpt: 'This post is scheduled for future publication.',
    content: '<h1>Coming Soon</h1><p>Stay tuned...</p>',
    category_id: 'cat-2',
    status: 'scheduled',
    author_id: 'test-user-id',
    author_name: 'Test User',
    featured_image: 'https://example.com/images/post-4.jpg',
    seo_title: 'Upcoming Feature Announcement',
    seo_description: 'Big news coming soon',
    scheduled_at: '2024-02-01T09:00:00Z',
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
  total: 4
}

export const mockEmptyBlogPosts = {
  posts: [],
  total: 0
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
