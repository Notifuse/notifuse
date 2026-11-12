import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Template, Workspace } from '../services/api/types'
import type { GetTemplateUsageResponse } from '../services/api/template'
import type { TemplatesSearch } from '../lib/searchParams'

const state = vi.hoisted(() => ({
  search: {} as Record<string, unknown>,
  permissions: {} as Record<string, { read: boolean; write: boolean }> | null
}))
const navigate = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => navigate
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: {
    timezone: 'UTC',
    marketing_email_provider_id: 'mp',
    transactional_email_provider_id: 'tp'
  },
  integrations: [{ id: 'supa', name: 'Supabase', type: 'supabase' }]
} as unknown as Workspace

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [workspace] }),
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: false })
}))

// The drawers are tested on their own; here they only say how the page opened them.
vi.mock('../components/templates/CreateTemplateDrawer', () => ({
  CreateTemplateDrawer: (props: {
    open?: boolean
    template?: Template
    fromTemplate?: Template
    initialName?: string
    initialTree?: unknown
    canManageSavedBlocks?: boolean
  }) =>
    props.open ? (
      <div
        data-testid="template-drawer"
        data-template={props.template?.id ?? ''}
        data-from={props.fromTemplate?.id ?? ''}
        data-name={props.initialName ?? ''}
        data-tree={props.initialTree ? 'yes' : ''}
        data-can-manage={String(props.canManageSavedBlocks)}
      />
    ) : null
}))
vi.mock('../components/templates/TemplatePreviewDrawer', () => ({
  default: (props: { open?: boolean; record: Template }) =>
    props.open ? <div data-testid="preview-drawer" data-template={props.record.id} /> : null
}))
vi.mock('../components/templates/SendTemplateModal', () => ({
  default: (props: { isOpen: boolean; template: Template | null }) =>
    props.isOpen ? <div data-testid="send-test" data-template={props.template?.id} /> : null
}))

vi.mock('../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
vi.mock('../lib/copyToClipboard', () => ({ copyToClipboard: vi.fn(async () => true) }))
vi.mock('../services/api/template', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api/template')>()
  return {
    ...actual,
    templatesApi: { list: vi.fn(), get: vi.fn(), delete: vi.fn(), usage: vi.fn() }
  }
})

import { TemplatesPage } from './TemplatesPage'
import { templatesApi } from '../services/api/template'
import { ApiError } from '../services/api/errors'
import { copyToClipboard } from '../lib/copyToClipboard'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const make = (overrides: Partial<Template> & { id: string }): Template =>
  ({
    name: overrides.id,
    version: 1,
    channel: 'email',
    category: 'marketing',
    email: { subject: `Subject of ${overrides.id}`, compiled_preview: '', visual_editor_tree: {} },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
    ...overrides
  }) as Template

const templates = [
  make({ id: 'newsletter', name: 'Newsletter' }),
  make({
    id: 'promo',
    name: 'Promo',
    email: {
      subject: 'Spring sale',
      subject_preview: 'Up to 50% off',
      compiled_preview: '',
      visual_editor_tree: {} as never,
      editor_mode: 'code'
    }
  }),
  make({ id: 'receipt', name: 'Receipt', category: 'transactional', integration_id: 'supa' })
]

const usage = (response: Partial<GetTemplateUsageResponse> = {}): GetTemplateUsageResponse => ({
  usage: {},
  hidden_types: [],
  ...response
})

const writer = {
  templates: { read: true, write: true },
  transactional: { read: true, write: true },
  lists: { read: true, write: true },
  contacts: { read: true, write: true }
}

const renderPage = (search: TemplatesSearch = {}) => {
  state.search = search as Record<string, unknown>
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>
          <TemplatesPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const rowOf = (name: string) => screen.getByText(name).closest('tr') as HTMLElement

const openMenu = async (name: string) => {
  fireEvent.click(within(rowOf(name)).getByRole('button', { name: 'More actions' }))
  return screen.findByRole('menu')
}

/** The search the last navigate call produced from the current one. */
const lastSearch = () => {
  const call = navigate.mock.calls.at(-1)?.[0] as {
    search: TemplatesSearch | ((prev: TemplatesSearch) => TemplatesSearch)
  }
  return typeof call.search === 'function' ? call.search(state.search) : call.search
}

describe('TemplatesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.permissions = writer
    vi.mocked(templatesApi.list).mockResolvedValue({ templates })
    vi.mocked(templatesApi.usage).mockResolvedValue(usage())
  })

  it('reads every email template once and counts each category in its tab', async () => {
    renderPage()
    await screen.findByText('Newsletter')
    expect(templatesApi.list).toHaveBeenCalledTimes(1)
    expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', channel: 'email' })

    const tabs = screen.getByRole('radiogroup')
    expect(within(tabs).getByText('All').parentElement).toHaveTextContent('All3')
    expect(within(tabs).getByText('Marketing').parentElement).toHaveTextContent('Marketing2')
    expect(within(tabs).getByText('Transactional').parentElement).toHaveTextContent(
      'Transactional1'
    )
    expect(within(tabs).queryByText('Bounce')).not.toBeInTheDocument()
    // Every template is on one page.
    expect(document.querySelector('.ant-pagination')).toBeNull()
  })

  it('filters the category of the URL in the browser, and shows it at 0 when empty', async () => {
    renderPage({ category: 'bounce' })
    await screen.findByText('No templates found for category "Bounce"')
    const tabs = screen.getByRole('radiogroup')
    expect(within(tabs).getByText('Bounce').parentElement).toHaveTextContent('Bounce0')

    fireEvent.click(screen.getByRole('button', { name: 'reset the filter' }))
    expect(lastSearch()).toEqual({})
  })

  it('keeps only the templates of the category tab', async () => {
    renderPage({ category: 'transactional' })
    await screen.findByText('Receipt')
    expect(screen.queryByText('Newsletter')).not.toBeInTheDocument()
  })

  it('searches the name and the subject, without changing the counts', async () => {
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.change(screen.getByPlaceholderText('Name or subject'), {
      target: { value: 'spring' }
    })
    expect(screen.getByText('Promo')).toBeInTheDocument()
    expect(screen.queryByText('Newsletter')).not.toBeInTheDocument()
    expect(within(screen.getByRole('radiogroup')).getByText('All').parentElement).toHaveTextContent(
      'All3'
    )

    fireEvent.change(screen.getByPlaceholderText('Name or subject'), {
      target: { value: 'nothing like it' }
    })
    expect(screen.getByText('No templates match your search')).toBeInTheDocument()
  })

  it('shows the subject with its preview text, and the Code mark', async () => {
    renderPage()
    await screen.findByText('Promo')
    const row = rowOf('Promo')
    expect(row).toHaveTextContent('Spring sale · Up to 50% off')
    expect(within(row).getByText('Code')).toBeInTheDocument()
  })

  it('marks a template managed by an integration, with a translated tooltip', async () => {
    renderPage()
    await screen.findByText('Receipt')
    const icon = within(rowOf('Receipt')).getByAltText('Supabase')
    fireEvent.mouseEnter(icon)
    expect(
      await screen.findByText('Managed by Supabase (supabase integration)')
    ).toBeInTheDocument()
  })

  it('sums up where each template is used', async () => {
    vi.mocked(templatesApi.usage).mockResolvedValue(
      usage({
        usage: {
          newsletter: { broadcasts: 2, lists: 0 },
          promo: { broadcasts: 1, automations: 2 }
        }
      })
    )
    renderPage()
    await screen.findByText('2 newsletters')
    expect(rowOf('Promo')).toHaveTextContent('3 uses')
    expect(rowOf('Receipt')).toHaveTextContent('Not used')
  })

  it('says in the column header that uses the member cannot see are not counted', async () => {
    vi.mocked(templatesApi.usage).mockResolvedValue(usage({ hidden_types: ['automations'] }))
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.mouseEnter(screen.getByText('Used in'))
    expect(await screen.findByText(/Uses you can't see aren't counted/)).toBeInTheDocument()
  })

  it('shows a dash when the uses could not be loaded', async () => {
    vi.mocked(templatesApi.usage).mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('Newsletter')
    await waitFor(() => expect(within(rowOf('Newsletter')).getByText('—')).toBeInTheDocument())
  })

  it('offers a retry when the templates could not be loaded', async () => {
    vi.mocked(templatesApi.list).mockRejectedValueOnce(new Error('database is down'))
    renderPage()
    expect(await screen.findByText('database is down')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Newsletter')).toBeInTheDocument()
  })

  it('opens the editor on a row click with Templates write', async () => {
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.click(rowOf('Newsletter'))
    expect(await screen.findByTestId('template-drawer')).toHaveAttribute(
      'data-template',
      'newsletter'
    )
  })

  it('opens the preview on a row click without Templates write', async () => {
    state.permissions = { templates: { read: true, write: false } }
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.click(rowOf('Newsletter'))
    expect(lastSearch()).toEqual({ preview: 'newsletter' })
  })

  it('opens the preview of the template the URL names', async () => {
    renderPage({ preview: 'promo' })
    expect(await screen.findByTestId('preview-drawer')).toHaveAttribute('data-template', 'promo')
  })

  it('reads a template to preview that the list does not hold', async () => {
    vi.mocked(templatesApi.get).mockResolvedValue({
      template: make({ id: 'other', name: 'Other' })
    })
    renderPage({ preview: 'other' })
    expect(await screen.findByTestId('preview-drawer')).toHaveAttribute('data-template', 'other')
  })

  it.each([
    [
      'unknown',
      () =>
        vi.mocked(templatesApi.get).mockRejectedValue(new ApiError('Template not found', 404, null))
    ],
    [
      'deleted',
      () =>
        vi.mocked(templatesApi.get).mockResolvedValue({
          template: make({ id: 'gone', deleted_at: '2026-11-01T00:00:00Z' })
        })
    ]
  ])('drops a preview of an %s template and says it no longer exists', async (_, arrange) => {
    arrange()
    renderPage({ preview: 'gone' })
    expect(await screen.findByText('This template no longer exists')).toBeInTheDocument()
    expect(lastSearch()).toEqual({})
    expect(screen.queryByTestId('preview-drawer')).not.toBeInTheDocument()
  })

  it('keeps the preview link when the template could not be read, and says so', async () => {
    vi.mocked(templatesApi.get).mockRejectedValue(new ApiError('Internal error', 500, null))
    renderPage({ preview: 'other' })
    expect(await screen.findByText("Couldn't load this template")).toBeInTheDocument()
    expect(screen.queryByText('This template no longer exists')).not.toBeInTheDocument()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('drops, without a message, the preview of a template that is not an email', async () => {
    vi.mocked(templatesApi.get).mockResolvedValue({
      template: make({ id: 'blog-tpl', channel: 'web' })
    })
    renderPage({ preview: 'blog-tpl' })
    await waitFor(() => expect(lastSearch()).toEqual({}))
    expect(screen.queryByText('This template no longer exists')).not.toBeInTheDocument()
    expect(screen.queryByTestId('preview-drawer')).not.toBeInTheDocument()
  })

  it('copies the ID of a template from the menu', async () => {
    renderPage()
    await screen.findByText('Newsletter')
    const menu = await openMenu('Newsletter')
    fireEvent.click(within(menu).getByText('Copy ID'))
    await waitFor(() => expect(copyToClipboard).toHaveBeenCalledWith('newsletter'))
    expect(await screen.findByText('ID copied')).toBeInTheDocument()
  })

  it('disables the writes of the menu without Templates write, and keeps the rest', async () => {
    state.permissions = { templates: { read: true, write: false } }
    renderPage()
    await screen.findByText('Newsletter')
    const menu = await openMenu('Newsletter')
    for (const label of ['Edit', 'Clone', 'Delete']) {
      expect(within(menu).getByText(label).closest('li')).toHaveAttribute('aria-disabled', 'true')
    }
    for (const label of ['Preview', 'Copy ID']) {
      expect(within(menu).getByText(label).closest('li')).not.toHaveAttribute(
        'aria-disabled',
        'true'
      )
    }
    expect(screen.getByRole('button', { name: /New template/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Import/ })).toBeDisabled()

    // A disabled write says why.
    fireEvent.mouseEnter(within(menu).getByText('Edit'))
    expect(
      (await screen.findAllByText("You don't have write permission for templates")).length
    ).toBeGreaterThan(0)
  })

  it('disables Delete for a template an integration manages', async () => {
    renderPage()
    await screen.findByText('Receipt')
    const menu = await openMenu('Receipt')
    expect(within(menu).getByText('Delete').closest('li')).toHaveAttribute('aria-disabled', 'true')
  })

  it('applies the test email rule to Send test', async () => {
    state.permissions = {
      templates: { read: true, write: true },
      transactional: { read: true, write: false }
    }
    renderPage()
    await screen.findByText('Newsletter')
    const menu = await openMenu('Newsletter')
    expect(within(menu).getByText('Send test').closest('li')).toHaveAttribute(
      'aria-disabled',
      'true'
    )
  })

  it('clones from the menu, with the saved blocks right of the member', async () => {
    renderPage()
    await screen.findByText('Newsletter')
    const menu = await openMenu('Newsletter')
    fireEvent.click(within(menu).getByText('Clone'))
    const drawer = await screen.findByTestId('template-drawer')
    expect(drawer).toHaveAttribute('data-from', 'newsletter')
    expect(drawer).toHaveAttribute('data-can-manage', 'true')
  })

  it('opens a new template with New template', async () => {
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.click(screen.getByRole('button', { name: /New template/ }))
    const drawer = await screen.findByTestId('template-drawer')
    expect(drawer).toHaveAttribute('data-template', '')
    expect(drawer).toHaveAttribute('data-from', '')
  })

  it('imports a file into a new template named after it', async () => {
    const { container } = renderPage()
    await screen.findByText('Newsletter')
    const input = container.querySelector('input[accept^=".json"]') as HTMLInputElement
    const tree = {
      id: 'root',
      type: 'mjml',
      children: [
        {
          id: 'body',
          type: 'mj-body',
          children: [
            {
              id: 's',
              type: 'mj-section',
              children: [{ id: 'c', type: 'mj-column', children: [] }]
            }
          ]
        }
      ]
    }
    fireEvent.change(input, {
      target: {
        files: [new File([JSON.stringify(tree)], 'spring-sale.json', { type: 'application/json' })]
      }
    })
    const drawer = await screen.findByTestId('template-drawer')
    expect(drawer).toHaveAttribute('data-name', 'spring-sale')
    expect(drawer).toHaveAttribute('data-tree', 'yes')
  })

  it('lists the uses of a template in its delete confirmation', async () => {
    vi.mocked(templatesApi.usage).mockImplementation(async ({ template_id }) =>
      template_id
        ? usage({
            usage: { newsletter: { broadcasts: 7, lists: 1 } },
            references: [
              ...Array.from({ length: 7 }, (_, i) => ({
                type: 'broadcast' as const,
                id: `b${i}`,
                name: `Weekly ${i}`,
                status: 'draft'
              })),
              { type: 'list' as const, id: 'l1', name: 'Customers' }
            ]
          })
        : usage()
    )
    renderPage()
    await screen.findByText('Newsletter')
    const menu = await openMenu('Newsletter')
    fireEvent.click(within(menu).getByText('Delete'))

    expect(await screen.findByText('This template is still used')).toBeInTheDocument()
    expect(screen.getByText('Weekly 0')).toBeInTheDocument()
    // The status of each broadcast, translated rather than the server's value
    expect(screen.getByText('Weekly 0').parentElement).toHaveTextContent('Weekly 0Draft')
    expect(screen.queryByText('Weekly 5')).not.toBeInTheDocument()
    expect(screen.getByText('and 2 more')).toBeInTheDocument()
    expect(screen.getByText('Customers')).toBeInTheDocument()
    expect(templatesApi.usage).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      template_id: 'newsletter'
    })
  })

  it('says some uses may be hidden by the permissions', async () => {
    vi.mocked(templatesApi.usage).mockImplementation(async ({ template_id }) =>
      template_id ? usage({ references: [], hidden_types: ['lists'] }) : usage()
    )
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.click(within(await openMenu('Newsletter')).getByText('Delete'))
    expect(
      await screen.findByText('Some uses may be hidden by your permissions.')
    ).toBeInTheDocument()
    expect(screen.queryByText('This template is still used')).not.toBeInTheDocument()
  })

  // An older server, or a proxy that strips empty fields, can leave the lists out.
  it('reads a usage answer without its usage map or hidden types', async () => {
    vi.mocked(templatesApi.usage).mockResolvedValue({} as GetTemplateUsageResponse)
    renderPage()
    await screen.findByText('Newsletter')
    expect(await screen.findAllByText('Not used')).not.toHaveLength(0)

    fireEvent.click(within(await openMenu('Newsletter')).getByText('Delete'))
    expect(
      await screen.findByText('Are you sure you want to delete this template?')
    ).toBeInTheDocument()
    expect(screen.queryByText('This template is still used')).not.toBeInTheDocument()
    expect(screen.queryByText("Couldn't check where this template is used")).not.toBeInTheDocument()
  })

  it('still deletes when the uses could not be checked', async () => {
    vi.mocked(templatesApi.usage).mockImplementation(async ({ template_id }) => {
      if (template_id) throw new Error('boom')
      return usage()
    })
    vi.mocked(templatesApi.delete).mockResolvedValue({ status: 'ok' })
    renderPage()
    await screen.findByText('Newsletter')
    fireEvent.click(within(await openMenu('Newsletter')).getByText('Delete'))
    expect(
      await screen.findByText("Couldn't check where this template is used")
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Yes, Delete' }))
    await waitFor(() =>
      expect(templatesApi.delete).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'newsletter' })
    )
  })

  it('shows the first-template empty state for a workspace without templates', async () => {
    vi.mocked(templatesApi.list).mockResolvedValue({ templates: [] })
    renderPage()
    expect(await screen.findByText('No templates found')).toBeInTheDocument()
    expect(screen.getByText('Create your first template to get started')).toBeInTheDocument()
  })
})
