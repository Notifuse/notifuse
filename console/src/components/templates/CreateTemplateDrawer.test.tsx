import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import type { Template, Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'

// The editor pulls in Monaco / the visual builder / AI assistant, none of which are
// relevant to the conflict-detection logic under test — stub them out.
vi.mock('../email_builder/EmailBuilder', () => ({ default: () => null }))
vi.mock('../email_builder/EmailAIAssistant', () => ({ EmailAIAssistant: () => null }))
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
    ;(templatesApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
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
