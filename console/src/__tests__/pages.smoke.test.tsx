import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactNode } from 'react'
import { App, ConfigProvider } from 'antd'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'

// Use vi.hoisted to define mock data before mocks are hoisted
const { mockWorkspace, mockUser, mockPermissions } = vi.hoisted(() => ({
  mockWorkspace: {
    id: 'test-workspace',
    name: 'Test Workspace',
    settings: {
      timezone: 'UTC',
      logo_url: '',
      custom_fields_labels: {}
    }
  },
  mockUser: {
    id: 'user-123',
    email: 'test@example.com'
  },
  mockPermissions: {
    contacts: { read: true, write: true },
    lists: { read: true, write: true },
    templates: { read: true, write: true },
    broadcasts: { read: true, write: true },
    transactional: { read: true, write: true },
    workspace: { read: true, write: true },
    message_history: { read: true, write: true },
    blog: { read: true, write: true },
    automations: { read: true, write: true }
  }
}))

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value
    },
    removeItem: (key: string) => {
      delete store[key]
    },
    clear: () => {
      store = {}
    }
  }
})()

Object.defineProperty(window, 'localStorage', { value: localStorageMock })

// Mock TanStack Router
vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useMatch: () => false,
    useParams: () => ({ workspaceId: 'test-workspace', section: 'team' }),
    useSearch: () => ({})
  }
})

// Mock AuthContext
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: mockUser,
    workspaces: [mockWorkspace],
    isAuthenticated: true,
    signin: vi.fn(),
    signout: vi.fn(),
    loading: false,
    refreshWorkspaces: vi.fn()
  }),
  useWorkspacePermissions: () => ({
    permissions: mockPermissions,
    loading: false
  }),
  AuthProvider: ({ children }: { children: ReactNode }) => children
}))

// Mock auth service
vi.mock('../services/api/auth', () => ({
  authService: {
    signIn: vi.fn().mockResolvedValue({}),
    verifyCode: vi.fn().mockResolvedValue({ token: 'test-token' }),
    getCurrentUser: vi.fn().mockResolvedValue({
      user: { id: 'user-123', email: 'test@example.com' },
      workspaces: [{ id: 'test-workspace', name: 'Test Workspace', settings: { timezone: 'UTC', logo_url: '', custom_fields_labels: {} } }]
    }),
    logout: vi.fn().mockResolvedValue({}),
    getSecretKey: vi.fn().mockResolvedValue({ secret_key: 'test-key' }),
    resetSecretKey: vi.fn().mockResolvedValue({ secret_key: 'new-key' })
  },
  isRootUser: () => true
}))

// Mock workspace service
vi.mock('../services/api/workspace', () => ({
  workspaceService: {
    list: vi.fn().mockResolvedValue({ workspaces: [{ id: 'test-workspace', name: 'Test Workspace', settings: { timezone: 'UTC', logo_url: '', custom_fields_labels: {} } }] }),
    get: vi.fn().mockResolvedValue({ workspace: { id: 'test-workspace', name: 'Test Workspace', settings: { timezone: 'UTC', logo_url: '', custom_fields_labels: {} } } }),
    create: vi.fn().mockResolvedValue({ workspace: { id: 'test-workspace', name: 'Test Workspace', settings: { timezone: 'UTC', logo_url: '', custom_fields_labels: {} } } }),
    update: vi.fn().mockResolvedValue({ workspace: { id: 'test-workspace', name: 'Test Workspace', settings: { timezone: 'UTC', logo_url: '', custom_fields_labels: {} } } }),
    getMembers: vi.fn().mockResolvedValue({ members: [] }),
    inviteMember: vi.fn().mockResolvedValue({}),
    removeMember: vi.fn().mockResolvedValue({}),
    updateMemberPermissions: vi.fn().mockResolvedValue({})
  }
}))

// Mock contacts API
vi.mock('../services/api/contacts', () => ({
  contactsApi: {
    list: vi.fn().mockResolvedValue({ contacts: [], next_cursor: null }),
    get: vi.fn().mockResolvedValue({ contact: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    getTotalContacts: vi.fn().mockResolvedValue({ total_contacts: 0 })
  }
}))

// Mock lists API
vi.mock('../services/api/list', () => ({
  listsApi: {
    list: vi.fn().mockResolvedValue({ lists: [] }),
    get: vi.fn().mockResolvedValue({ list: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({})
  }
}))

// Mock segment API
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn().mockResolvedValue({ segments: [] }),
  getSegment: vi.fn().mockResolvedValue({ segment: null }),
  createSegment: vi.fn().mockResolvedValue({}),
  updateSegment: vi.fn().mockResolvedValue({}),
  deleteSegment: vi.fn().mockResolvedValue({})
}))

// Mock templates API
vi.mock('../services/api/templates', () => ({
  templatesApi: {
    list: vi.fn().mockResolvedValue({ templates: [] }),
    get: vi.fn().mockResolvedValue({ template: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({})
  }
}))

// Mock broadcasts API
vi.mock('../services/api/broadcasts', () => ({
  broadcastsApi: {
    list: vi.fn().mockResolvedValue({ broadcasts: [] }),
    get: vi.fn().mockResolvedValue({ broadcast: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    schedule: vi.fn().mockResolvedValue({}),
    send: vi.fn().mockResolvedValue({})
  }
}))

// Mock automations API
vi.mock('../services/api/automations', () => ({
  automationsApi: {
    list: vi.fn().mockResolvedValue({ automations: [] }),
    get: vi.fn().mockResolvedValue({ automation: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({})
  }
}))

// Mock transactional notifications API
vi.mock('../services/api/transactional', () => ({
  transactionalApi: {
    list: vi.fn().mockResolvedValue({ notifications: [] }),
    get: vi.fn().mockResolvedValue({ notification: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({})
  }
}))

// Mock message history API
vi.mock('../services/api/message_history', () => ({
  messageHistoryApi: {
    list: vi.fn().mockResolvedValue({ messages: [] }),
    get: vi.fn().mockResolvedValue({ message: null })
  }
}))

// Mock files API
vi.mock('../services/api/files', () => ({
  filesApi: {
    list: vi.fn().mockResolvedValue({ files: [], folders: [] }),
    upload: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    createFolder: vi.fn().mockResolvedValue({})
  }
}))

// Mock blog API
vi.mock('../services/api/blog', () => ({
  blogApi: {
    list: vi.fn().mockResolvedValue({ posts: [] }),
    get: vi.fn().mockResolvedValue({ post: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    listCategories: vi.fn().mockResolvedValue({ categories: [] })
  }
}))

// Mock email integration API
vi.mock('../services/api/email_integration', () => ({
  emailIntegrationApi: {
    list: vi.fn().mockResolvedValue({ integrations: [] }),
    get: vi.fn().mockResolvedValue({ integration: null }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({})
  }
}))

// Mock analytics API
vi.mock('../services/api/analytics', () => ({
  analyticsApi: {
    getOverview: vi.fn().mockResolvedValue({
      broadcasts_sent: 0,
      transactional_sent: 0,
      total_contacts: 0,
      total_lists: 0
    }),
    getBroadcastStats: vi.fn().mockResolvedValue({ stats: [] }),
    getTransactionalStats: vi.fn().mockResolvedValue({ stats: [] })
  },
  analyticsService: {
    getEmailMetrics: vi.fn().mockResolvedValue({ metrics: [] }),
    getFailedMessages: vi.fn().mockResolvedValue({ messages: [] }),
    query: vi.fn().mockResolvedValue({ data: [] })
  }
}))

// Mock system API
vi.mock('../services/api/system', () => ({
  systemApi: {
    getStatus: vi.fn().mockResolvedValue({ setup_completed: true }),
    getSettings: vi.fn().mockResolvedValue({ settings: {} }),
    updateSettings: vi.fn().mockResolvedValue({}),
    completeSetup: vi.fn().mockResolvedValue({})
  }
}))

// Mock invitation API
vi.mock('../services/api/invitation', () => ({
  invitationApi: {
    accept: vi.fn().mockResolvedValue({})
  }
}))

// Mock CSV upload provider
vi.mock('../components/contacts/ContactsCsvUploadProvider', () => ({
  useContactsCsvUpload: () => ({
    openDrawer: vi.fn()
  }),
  ContactsCsvUploadProvider: ({ children }: { children: ReactNode }) => children
}))

// Mock router module for route references
vi.mock('../router', () => ({
  workspaceContactsRoute: { id: '/console/workspace/$workspaceId/contacts', to: '/console/workspace/$workspaceId/contacts' },
  workspaceFileManagerRoute: { id: '/console/workspace/$workspaceId/file-manager' }
}))

// Pages - imported after mocks
import { SignInPage } from '../pages/SignInPage'
import { LogoutPage } from '../pages/LogoutPage'
import SetupWizard from '../pages/SetupWizard'
import { AcceptInvitationPage } from '../pages/AcceptInvitationPage'
import { DashboardPage } from '../pages/DashboardPage'
import { CreateWorkspacePage } from '../pages/CreateWorkspacePage'
import { AnalyticsPage } from '../pages/AnalyticsPage'
import { BroadcastsPage } from '../pages/BroadcastsPage'
import { ContactsPage } from '../pages/ContactsPage'
import { ListsPage } from '../pages/ListsPage'
import { TemplatesPage } from '../pages/TemplatesPage'
import { AutomationsPage } from '../pages/AutomationsPage'
import { TransactionalNotificationsPage } from '../pages/TransactionalNotificationsPage'
import { LogsPage } from '../pages/LogsPage'
import { WorkspaceSettingsPage } from '../pages/WorkspaceSettingsPage'
import { FileManagerPage } from '../pages/FileManagerPage'
import { BlogPage } from '../pages/BlogPage'
import { DebugSegmentPage } from '../pages/DebugSegmentPage'

// Create a wrapper with all required providers
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0
      }
    }
  })

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <I18nProvider i18n={i18n}>
          <ConfigProvider>
            <App>{children}</App>
          </ConfigProvider>
        </I18nProvider>
      </QueryClientProvider>
    )
  }
}

describe('Page Smoke Tests', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorageMock.clear()
  })

  describe('Public Pages', () => {
    it('SignInPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<SignInPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('LogoutPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<LogoutPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('SetupWizard renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<SetupWizard />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('AcceptInvitationPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<AcceptInvitationPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })
  })

  describe('Authenticated Pages', () => {
    it('DashboardPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<DashboardPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('CreateWorkspacePage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<CreateWorkspacePage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })
  })

  describe('Workspace Pages', () => {
    it('AnalyticsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<AnalyticsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('BroadcastsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<BroadcastsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('ContactsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<ContactsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('ListsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<ListsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('TemplatesPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<TemplatesPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('AutomationsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<AutomationsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('TransactionalNotificationsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<TransactionalNotificationsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('LogsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<LogsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('WorkspaceSettingsPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<WorkspaceSettingsPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('FileManagerPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<FileManagerPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('BlogPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<BlogPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })

    it('DebugSegmentPage renders without error', async () => {
      const Wrapper = createWrapper()
      expect(() => render(<DebugSegmentPage />, { wrapper: Wrapper })).not.toThrow()
      await waitFor(() => {
        expect(document.body).toBeTruthy()
      })
    })
  })

  describe('Page Content Verification', () => {
    it('SignInPage shows sign in form', async () => {
      const Wrapper = createWrapper()
      render(<SignInPage />, { wrapper: Wrapper })
      await waitFor(() => {
        expect(screen.getByText(/Sign In/i)).toBeTruthy()
      })
    })

    it('DashboardPage shows workspace selection', async () => {
      const Wrapper = createWrapper()
      render(<DashboardPage />, { wrapper: Wrapper })
      await waitFor(() => {
        expect(screen.getByText(/Select workspace/i)).toBeTruthy()
      })
    })

    it('ContactsPage shows contacts title', async () => {
      const Wrapper = createWrapper()
      render(<ContactsPage />, { wrapper: Wrapper })
      await waitFor(() => {
        expect(screen.getByText(/Contacts/i)).toBeTruthy()
      })
    })
  })
})
