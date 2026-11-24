import type React from 'react'
import { afterAll, describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import enUS from 'antd/locale/en_US'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AIAssistantSettings } from './AIAssistantSettings'
import type { Integration, Workspace } from '../../services/api/types'
import { api } from '../../services/api/client'

// Settings › AI › Assistant seen from the API client: the client is the only service mocked, so
// workspaceService and the forms run for real and the tests read the exact bodies posted.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))
// The page renders outside a router: a link shows the path it would open.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    params,
    className
  }: {
    children: React.ReactNode
    to: string
    params: { workspaceId: string }
    className?: string
  }) => (
    <a href={to.replace('$workspaceId', params.workspaceId)} className={className}>
      {children}
    </a>
  )
}))

// The connected day is read on the browser's clock: pinned to UTC so it is the same on any runner.
const runnerTimeZone = process.env.TZ
process.env.TZ = 'UTC'
afterAll(() => {
  if (runnerTimeZone === undefined) delete process.env.TZ
  else process.env.TZ = runnerTimeZone
})

i18n.loadAndActivate({ locale: 'en', messages: {} })

const anthropic = {
  id: 'int_ant',
  name: 'Claude',
  type: 'llm',
  llm_provider: { kind: 'anthropic', anthropic: { model: 'claude-sonnet-5-5', encrypted_api_key: 'e' } },
  credential_hints: { 'anthropic.api_key': 'a1B2' },
  created_at: '2026-11-12T10:00:00Z',
  updated_at: '2026-11-12T10:00:00Z'
} as unknown as Integration

const openai = {
  id: 'int_oai',
  name: 'Local Llama',
  type: 'llm',
  llm_provider: {
    kind: 'openai',
    openai: { model: 'llama3.3:70b', base_url: 'https://llm.acme.internal/v1', reasoning_effort: 'low', encrypted_api_key: 'e' }
  },
  credential_hints: { 'openai.api_key': 'r5Tn' },
  created_at: '2026-11-20T10:00:00Z',
  updated_at: '2026-11-20T10:00:00Z'
} as unknown as Integration

const firecrawl = {
  id: 'int_fc',
  name: 'Firecrawl',
  type: 'firecrawl',
  firecrawl_settings: { encrypted_api_key: 'e', base_url: 'https://crawl.acme.com' },
  credential_hints: { 'firecrawl.api_key': '3vTb' },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z'
} as unknown as Integration

const zapier = {
  id: 'int_zap',
  name: 'Marketing Zaps',
  type: 'zapier',
  zapier_settings: { api_key_email: 'zapier-marketing@api.acme.com' },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z'
} as unknown as Integration

const ses = {
  id: 'int_ses',
  name: 'Amazon SES',
  type: 'email',
  email_provider: { kind: 'ses', senders: [], ses: { region: 'eu-west-1' } },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z'
} as unknown as Integration

const workspaceWith = (integrations: Integration[]) =>
  ({ id: 'ws1', name: 'Acme', settings: {}, integrations }) as unknown as Workspace

let current: Workspace

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.get).mockImplementation((async (path: string) =>
    path.startsWith('/api/workspaces.get') ? { workspace: current } : {}) as typeof api.get)
  vi.mocked(api.post).mockImplementation((async (path: string) =>
    path === '/api/workspaces.createIntegration' ? { integration_id: 'int_new' } : {}) as typeof api.post)
})

const renderPage = (integrations: Integration[] = [], isOwner = true) => {
  current = workspaceWith(integrations)
  let rerender: (ui: React.ReactElement) => void = () => {}
  const tree = (workspace: Workspace) => (
    <I18nProvider i18n={i18n}>
      <ConfigProvider locale={enUS}>
        <App>
          <AIAssistantSettings workspace={workspace} onSave={onSave} isOwner={isOwner} />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  // Mirrors WorkspaceSettingsPage: onSave swaps the workspace prop.
  const onSave = vi.fn(async (workspace: Workspace) => rerender(tree(workspace)))
  const utils = render(tree(current))
  rerender = utils.rerender
  return { ...utils, onSave }
}

/** The bodies posted to one endpoint, in order. */
const posted = (path: string) =>
  vi
    .mocked(api.post)
    .mock.calls.filter(([p]) => p === path)
    .map(([, body]) => body as Record<string, unknown>)

const modelsBlock = () => document.querySelector('[data-ai-block="models"]') as HTMLElement
const webAccessBlock = () => document.querySelector('[data-ai-block="web-access"]') as HTMLElement
const drawerTitle = () => document.querySelector('[data-drawer-title]')?.textContent

const addModel = async (provider: string) => {
  fireEvent.click(within(modelsBlock()).getByRole('button', { name: /Add a model/ }))
  fireEvent.click(await screen.findByRole('menuitem', { name: new RegExp(`^${provider}`) }))
  await waitFor(() => expect(document.querySelector('[data-drawer-title]')).not.toBeNull())
}

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } })

const save = () =>
  fireEvent.click(within(document.querySelector('[data-drawer-bar]') as HTMLElement).getByRole('button', { name: 'Save' }))

const rowOf = (name: string) => screen.getByText(name).closest('tr') as HTMLElement

describe('AI assistant settings: the models', () => {
  it('lists the AI models of the workspace and no other integration', () => {
    renderPage([anthropic, zapier, openai, ses, firecrawl])

    const rows = within(modelsBlock()).getAllByRole('row').slice(1)
    expect(rows.map((row) => within(row).getAllByRole('cell')[0].textContent)).toEqual(['Claude', 'Local Llama'])
    expect(within(rowOf('Claude')).getByText('claude-sonnet-5-5')).toBeInTheDocument()
    expect(within(rowOf('Claude')).getByText('Anthropic')).toBeInTheDocument()
    expect(within(rowOf('Claude')).getByText('Nov 12, 2026')).toBeInTheDocument()
    expect(within(rowOf('Local Llama')).getByText('OpenAI')).toBeInTheDocument()
    expect(screen.queryByText('Marketing Zaps')).toBeNull()
    expect(screen.queryByText('Amazon SES')).toBeNull()
  })

  it('offers the three providers, each with what it covers', async () => {
    renderPage()
    expect(within(modelsBlock()).getByText('No model yet')).toBeInTheDocument()

    fireEvent.click(within(modelsBlock()).getByRole('button', { name: /Add a model/ }))
    const items = await screen.findAllByRole('menuitem')
    expect(items.map((item) => item.textContent)).toEqual([
      'AnthropicClaude models',
      'OpenAIOr any OpenAI-compatible endpoint',
      'Google GeminiGemini models'
    ])
  })

  it('says a model has no API key instead of leaving it unmarked', () => {
    const keyless = {
      ...anthropic,
      llm_provider: { kind: 'anthropic', anthropic: { model: 'claude-sonnet-4-6' } },
      credential_hints: undefined
    } as unknown as Integration
    renderPage([keyless])

    expect(within(rowOf('Claude')).getByText('No API key')).toBeInTheDocument()
  })

  it(
    'Anthropic: takes any model ID besides the suggestions, and requires the key on creation',
    async () => {
      renderPage()
      await addModel('Anthropic')
      expect(drawerTitle()).toBe('Add Anthropic integration')
      // The bar's way back names this page.
      expect(screen.getByRole('button', { name: /AI assistant/ })).toBeInTheDocument()

      expect(screen.getByLabelText('Name')).toHaveValue('Anthropic')
      const model = screen.getByLabelText('Model')
      expect(model).toHaveValue('claude-sonnet-5-5')
      fireEvent.mouseDown(model)
      fireEvent.change(model, { target: { value: '' } })
      for (const id of ['claude-opus-5-5', 'claude-sonnet-5-5', 'claude-haiku-5-5']) {
        expect(await screen.findByTitle(id)).toBeInTheDocument()
      }
      fireEvent.change(model, { target: { value: 'claude-future-5' } })
      save()
      expect(await screen.findByText('Please enter your API key')).toBeInTheDocument()
      type('API key', 'sk-ant-1')
      save()

      await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
      expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
        workspace_id: 'ws1',
        name: 'Anthropic',
        type: 'llm',
        llm_provider: { kind: 'anthropic', anthropic: { api_key: 'sk-ant-1', model: 'claude-future-5' } }
      })
      // Saved: the workspace is read again and the drawer closes.
      await waitFor(() => expect(document.querySelector('[data-drawer-title]')).toBeNull())
      expect(api.get).toHaveBeenCalledWith(expect.stringMatching(/^\/api\/workspaces\.get/))
    }
  )

  it('OpenAI: starts on gpt-6.1-sol and saves the base URL and the reasoning effort', async () => {
    renderPage()
    await addModel('OpenAI')
    expect(screen.getByLabelText('Model')).toHaveValue('gpt-6.1-sol')
    type('API key', 'sk-proj-1')
    type('Base URL', 'https://llm.acme.internal/v1')
    const effort = screen.getByLabelText('Reasoning effort')
    fireEvent.mouseDown(effort)
    fireEvent.click(await screen.findByTitle('High'))
    save()

    await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      name: 'OpenAI',
      type: 'llm',
      llm_provider: {
        kind: 'openai',
        openai: {
          api_key: 'sk-proj-1',
          model: 'gpt-6.1-sol',
          base_url: 'https://llm.acme.internal/v1',
          reasoning_effort: 'high'
        }
      }
    })
  })

  it('Gemini: suggests the Gemini 4 models and saves the default one', async () => {
    renderPage()
    await addModel('Google Gemini')
    const model = screen.getByLabelText('Model')
    expect(model).toHaveValue('gemini-4-argon')
    fireEvent.mouseDown(model)
    fireEvent.change(model, { target: { value: '' } })
    for (const id of ['gemini-4-carbon', 'gemini-4-argon', 'gemini-4-hydrogen']) {
      expect(await screen.findByTitle(id)).toBeInTheDocument()
    }
    fireEvent.change(model, { target: { value: 'gemini-4-argon' } })
    type('API key', 'AIza-1')
    save()
    await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      name: 'Google Gemini',
      type: 'llm',
      llm_provider: { kind: 'gemini', gemini: { api_key: 'AIza-1', model: 'gemini-4-argon' } }
    })
  })

  it('OpenAI edit: resends the effort and keeps the key blank, showing its end', async () => {
    renderPage([openai])

    fireEvent.click(within(rowOf('Local Llama')).getByRole('button', { name: 'Edit' }))
    expect(drawerTitle()).toBe('Edit OpenAI integration')
    expect(await screen.findByLabelText('API key')).toHaveAttribute('placeholder', '••••••••r5Tn')
    save()
    await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      integration_id: 'int_oai',
      name: 'Local Llama',
      llm_provider: {
        kind: 'openai',
        openai: { model: 'llama3.3:70b', base_url: 'https://llm.acme.internal/v1', reasoning_effort: 'low' }
      }
    })
  })

  it('removes a model once the deletion is confirmed', async () => {
    renderPage([anthropic, openai])

    fireEvent.click(within(rowOf('Claude')).getByRole('button', { name: 'Remove' }))
    const confirm = await screen.findByRole('dialog', { name: 'Delete this integration?' })
    expect(within(confirm).getByText('This action cannot be undone.')).toBeInTheDocument()
    expect(posted('/api/workspaces.deleteIntegration')).toHaveLength(0)
    fireEvent.click(within(confirm).getByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(posted('/api/workspaces.deleteIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.deleteIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      integration_id: 'int_ant'
    })
  })
})

describe('AI assistant settings: web access', () => {
  it('Firecrawl: connects with a required key and a base URL', async () => {
    renderPage()
    expect(within(webAccessBlock()).getByText('Not connected')).toBeInTheDocument()

    fireEvent.click(within(webAccessBlock()).getByRole('button', { name: 'Connect' }))
    await waitFor(() => expect(drawerTitle()).toBe('Add Firecrawl integration'))
    expect(screen.getByText('scrape_url')).toBeInTheDocument()
    expect(screen.getByText('search_web')).toBeInTheDocument()
    save()
    expect(await screen.findByText('Please enter your API key')).toBeInTheDocument()
    type('API key', 'fc-key')
    type('Base URL', 'https://crawl.acme.com')
    save()

    await waitFor(() => expect(posted('/api/workspaces.createIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.createIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      name: 'Firecrawl',
      type: 'firecrawl',
      firecrawl_settings: { api_key: 'fc-key', base_url: 'https://crawl.acme.com' }
    })
  })

  // The server only uses the first Firecrawl integration: a second one would never run.
  it('offers no second Firecrawl once one exists, and edits or removes it instead', async () => {
    renderPage([firecrawl])
    const block = webAccessBlock()

    expect(within(block).queryByRole('button', { name: 'Connect' })).toBeNull()
    expect(within(block).getByText('Connected')).toBeInTheDocument()
    expect(within(block).getByText('https://crawl.acme.com')).toBeInTheDocument()

    fireEvent.click(within(block).getByRole('button', { name: 'Edit' }))
    expect(await screen.findByLabelText('API key')).toHaveAttribute('placeholder', '••••••••3vTb')
    save()
    await waitFor(() => expect(posted('/api/workspaces.updateIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.updateIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      integration_id: 'int_fc',
      name: 'Firecrawl',
      firecrawl_settings: { base_url: 'https://crawl.acme.com' }
    })
  })

  it('removes Firecrawl once the deletion is confirmed', async () => {
    renderPage([firecrawl])

    fireEvent.click(within(webAccessBlock()).getByRole('button', { name: 'Remove' }))
    const confirm = await screen.findByRole('dialog', { name: 'Delete this integration?' })
    fireEvent.click(within(confirm).getByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(posted('/api/workspaces.deleteIntegration')).toHaveLength(1))
    expect(posted('/api/workspaces.deleteIntegration')[0]).toEqual({
      workspace_id: 'ws1',
      integration_id: 'int_fc'
    })
  })
})

describe('AI assistant settings: members', () => {
  it('shows a member the models and Firecrawl with no way to add, change or remove them', () => {
    renderPage([anthropic, firecrawl], false)

    expect(screen.getByText('Claude')).toBeInTheDocument()
    expect(within(webAccessBlock()).getByText('Firecrawl')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Add a model/ })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull()
    expect(screen.getByText(/Only owners can add, change or remove a model/)).toBeInTheDocument()
  })

  it('offers a member no Firecrawl connection', () => {
    renderPage([], false)

    expect(within(webAccessBlock()).getByText('Not connected')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Connect' })).toBeNull()
  })
})

describe('AI assistant settings: where the assistant appears', () => {
  it('links to the blog, the templates and Web Analytics of the workspace', () => {
    renderPage()
    const block = document.querySelector('[data-ai-block="uses"]') as HTMLElement

    expect(within(block).getByRole('link', { name: /^Blog/ })).toHaveAttribute('href', '/console/workspace/ws1/blog')
    expect(within(block).getByRole('link', { name: /^Email editor/ })).toHaveAttribute(
      'href',
      '/console/workspace/ws1/templates'
    )
    expect(within(block).getByRole('link', { name: /^Web Analytics/ })).toHaveAttribute(
      'href',
      '/console/workspace/ws1/web-analytics'
    )
  })
})
