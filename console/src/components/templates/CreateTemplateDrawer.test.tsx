import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import type { Template, Workspace } from '../../services/api/types'
import type { EmailAIAssistantProps } from '../email_builder/EmailAIAssistant'
import { templatesApi } from '../../services/api/template'

// The editor pulls in Monaco / the visual builder / AI assistant, none of which are
// relevant to the logic under test — stub them out. The assistant stub keeps the one
// prop these tests are about, the template category, readable from the DOM: it changes
// a render after the form field settles, so an attribute waitFor can see beats a
// captured variable.
vi.mock('../email_builder/EmailBuilder', () => ({ default: () => null }))
vi.mock('../email_builder/EmailAIAssistant', () => ({
  EmailAIAssistant: ({ category }: EmailAIAssistantProps) => (
    <div data-testid="email-ai-assistant" data-category={category ?? ''} />
  )
}))
vi.mock('../email_builder/MjmlCodeEditor', () => ({
  default: () => null,
  STARTER_TEMPLATE: '<mjml></mjml>'
}))
vi.mock('./PhonePreview', () => ({ default: () => null }))
vi.mock('./ImportExportButton', () => ({ ImportExportButton: () => null }))
vi.mock('./TemplateTranslationsTab', () => ({ default: () => null }))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ refreshWorkspaces: vi.fn() })
}))

vi.mock('../../services/api/template', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/template')>()
  return {
    ...actual,
    templatesApi: {
      get: vi.fn(),
      update: vi.fn(),
      create: vi.fn(),
      list: vi.fn(),
      delete: vi.fn(),
      compile: vi.fn()
    }
  }
})

const workspace = {
  id: 'ws1',
  settings: {
    languages: ['en'],
    default_language: 'en',
    marketing_email_provider_id: undefined,
    transactional_email_provider_id: undefined
  },
  integrations: []
} as unknown as Workspace

const makeTemplate = (version: number): Template =>
  ({
    id: 'tmpl1',
    name: 'Welcome',
    version,
    channel: 'email',
    category: 'transactional',
    email: {
      editor_mode: 'visual',
      sender_id: 'sender-1',
      subject: 'Hello',
      subject_preview: 'Preview',
      compiled_preview: '<p>Hi</p>',
      visual_editor_tree: { id: 'root', kind: 'mjml', children: [] }
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z'
  }) as unknown as Template

function renderDrawer(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AntApp>{ui}</AntApp>
    </QueryClientProvider>
  )
}

describe('CreateTemplateDrawer conflict handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('freshens against the latest server revision when opening an existing template', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({
      template: makeTemplate(5)
    })

    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} />)

    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))

    // Opening the editor must re-read the latest revision (version: 0 => latest) so
    // the edit isn't based on a stale list snapshot.
    await waitFor(() => {
      expect(templatesApi.get).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        id: 'tmpl1',
        version: 0
      })
    })
  })

  it('does not freshen when creating a new template (no base to compare)', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)

    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

    // Give any async open work a chance to run before asserting it did not fetch.
    await new Promise((r) => setTimeout(r, 0))
    expect(templatesApi.get).not.toHaveBeenCalled()
  })
})

describe('CreateTemplateDrawer AI assistant context', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // The assistant designs to the category: a marketing email must carry an unsubscribe
  // footer, a transactional one must not, and it can only know which from this prop.
  it('passes the category of an existing template to the AI assistant', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })

    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} />)

    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))

    await waitFor(() => {
      expect(screen.getByTestId('email-ai-assistant')).toHaveAttribute(
        'data-category',
        'transactional'
      )
    })
  })

  it('passes the forced category to the AI assistant when creating from a fixed category', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} forceCategory="marketing" />)

    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

    await waitFor(() => {
      expect(screen.getByTestId('email-ai-assistant')).toHaveAttribute(
        'data-category',
        'marketing'
      )
    })
  })

  it('passes no category to the AI assistant until one is chosen on a new template', async () => {
    // The state the static compliance section of the prompt exists for: the assistant is
    // usable before the category field is filled in.
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)

    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

    await waitFor(() => {
      expect(screen.getByTestId('email-ai-assistant')).toHaveAttribute('data-category', '')
    })
  })

  it('hands a category picked in the Settings tab to the AI assistant', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)

    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))
    await userEvent.click(screen.getAllByRole('combobox')[0])
    await userEvent.click(await screen.findByText('Marketing'))

    await waitFor(() => {
      expect(screen.getByTestId('email-ai-assistant')).toHaveAttribute(
        'data-category',
        'marketing'
      )
    })
  })
})
