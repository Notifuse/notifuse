import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactElement } from 'react'
import { ApiError } from '../../services/api/client'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import type { Template, Workspace } from '../../services/api/types'
import type { EmailAIAssistantProps } from '../email_builder/EmailAIAssistant'
import { templatesApi } from '../../services/api/template'
import { templateBlocksApi } from '../../services/api/template_blocks'
import { EmailBlockClass } from '../email_builder/EmailBlockClass'
import type {
  DeleteSavedBlockHandler,
  EmailBlock,
  RenameSavedBlockHandler,
  SaveBlockHandler
} from '../email_builder/types'

// The editor pulls in Monaco / the visual builder / AI assistant, none of which are
// relevant to the logic under test — stub them out. The assistant stub keeps the one
// prop these tests are about, the template category, readable from the DOM: it changes
// a render after the form field settles, so an attribute waitFor can see beats a
// captured variable.
// The visual builder stub records its last props, so the tests can call the handlers the drawer gives it
// (onSaveBlock, onCompile).
const builder = vi.hoisted(() => ({ props: undefined as undefined | Record<string, unknown> }))
const assistant = vi.hoisted(() => ({ props: undefined as undefined | Record<string, unknown> }))
const auth = vi.hoisted(() => ({
  refreshWorkspaces: vi.fn(),
  permissions: { templates: { read: true, write: true }, transactional: { read: true, write: true } } as Record<string, unknown>
}))
vi.mock('../email_builder/EmailBuilder', () => ({
  default: (props: Record<string, unknown>) => {
    builder.props = props
    return null
  }
}))
vi.mock('../email_builder/EmailAIAssistant', () => ({
  EmailAIAssistant: (props: EmailAIAssistantProps) => {
    assistant.props = props as unknown as Record<string, unknown>
    return <div data-testid="email-ai-assistant" data-category={props.category ?? ''} />
  }
}))
vi.mock('../email_builder/MjmlCodeEditor', () => ({
  default: () => null,
  STARTER_TEMPLATE: '<mjml></mjml>'
}))
vi.mock('./PhonePreview', () => ({ default: () => null }))
vi.mock('./ImportExportButton', () => ({ ImportExportButton: () => null }))
// The translations tab records what the drawer tells it.
const translationsTab = vi.hoisted(() => ({ props: undefined as Record<string, unknown> | undefined }))
vi.mock('./TemplateTranslationsTab', () => ({
  default: (props: Record<string, unknown>) => {
    translationsTab.props = props
    return null
  }
}))
// The start modal is tested on its own: here it only hands the drawer a choice, as it does.
vi.mock('./TemplateStartModal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./TemplateStartModal')>()
  const { buildStartingPoint } = await import('./gallery/buildStartingPoint')
  const { galleryModel } = await import('./gallery/models')
  return {
    ...actual,
    TemplateStartModal: (props: import('./TemplateStartModal').TemplateStartModalProps) => {
      const start = (id: string) =>
        props.onPickModel(
          buildStartingPoint(
            galleryModel(id)!,
            { defaultLanguage: props.workspace.settings.default_language, languages: props.workspace.settings.languages },
            props.assets,
            props.defaultTestData
          )
        )
      return (
        <div data-testid="start-modal" data-force-category={props.forceCategory ?? ''}>
          <button onClick={() => start('blank')}>pick blank</button>
          <button onClick={() => start('order')}>pick order</button>
          <button onClick={() => props.onPickTemplate({ ...startSource, name: 'Spring news' } as Template)}>pick template</button>
          <button onClick={props.onCancel}>cancel start</button>
        </div>
      )
    }
  }
})
const startSource = vi.hoisted(() => ({
  id: 'spring',
  name: 'Spring news',
  version: 2,
  channel: 'email',
  category: 'marketing',
  email: { editor_mode: 'code', subject: 'Spring', subject_preview: 'News', mjml_source: '<mjml></mjml>', sender_id: 'sender-9', reply_to: 'news@acme.io' }
}))

// The test send only shows which saved revision it was opened on.
vi.mock('./SendTemplateModal', () => ({
  default: (props: { isOpen: boolean; template: Template | null }) =>
    props.isOpen ? <div data-testid="send-test" data-version={props.template?.version} /> : null
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ refreshWorkspaces: auth.refreshWorkspaces }),
  useWorkspacePermissions: () => ({ permissions: auth.permissions })
}))
vi.mock('../../services/api/template_blocks', () => ({
  templateBlocksApi: { list: vi.fn(), get: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() }
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

// A new template opens the start modal first: Blank, unless a test picks something else.
async function pickBlank() {
  await userEvent.click(await screen.findByRole('button', { name: 'pick blank' }))
}

// The bar's save button says what it writes: "Create template" for a new one, "Save vN" otherwise.
// The bar's save button: the last one so named, the opener of an uncontrolled drawer is called Create template too.
const saveButton = () => screen.getAllByRole('button', { name: /^(Create template|Save v\d+)$/ }).at(-1)!
// A saved template has nothing to save until something changes: a character added to its name.
const editName = () => userEvent.type(screen.getByPlaceholderText('i.e: Welcome Email'), '!')

// Closes the drawer from its back button, discarding the changes when it asks.
async function closeDrawer() {
  await userEvent.click(screen.getByRole('button', { name: /^Templates$/ }))
  const discard = await screen.findByRole('button', { name: 'Discard' }, { timeout: 300 }).catch(() => null)
  if (discard) await userEvent.click(discard)
}

// The step the bar's switch shows as selected.
const selectedStep = () => document.querySelector('.ant-segmented-item-selected')?.textContent

// A step of the bar's switch (the Content card of the Settings step has the same title).
const stepButton = (step: 'settings' | 'template' | 'translations') =>
  document.querySelector<HTMLElement>(`[data-step="${step}"]`)!

describe('CreateTemplateDrawer conflict handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('freshens against the latest server revision when opening an existing template', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({
      template: makeTemplate(5)
    })

    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} />)

    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))

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

    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

    // Give any async open work a chance to run before asserting it did not fetch.
    await new Promise((r) => setTimeout(r, 0))
    expect(templatesApi.get).not.toHaveBeenCalled()
  })
})

describe('CreateTemplateDrawer editor height', () => {
  // The drawer's top bar is 48 px (FullDrawer, h-12): an editor any taller scrolls the drawer
  it("gives the editor the screen's height under the drawer's 48 px bar", async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(builder.props?.height).toBe('calc(100vh - 48px)'))
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

    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))

    await waitFor(() => {
      expect(screen.getByTestId('email-ai-assistant')).toHaveAttribute(
        'data-category',
        'transactional'
      )
    })
  })

  it('passes the forced category to the AI assistant when creating from a fixed category', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} forceCategory="marketing" />)

    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

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

    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

    await waitFor(() => {
      expect(screen.getByTestId('email-ai-assistant')).toHaveAttribute('data-category', '')
    })
  })

  it('hands a category picked in the Settings tab to the AI assistant', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)

    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()
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

describe('CreateTemplateDrawer AI assistant column widths', () => {
  const col = (id: string, width: string) => ({ id, type: 'mj-column', attributes: { width }, children: [] })
  const withSection = (columns: unknown[]): Template => {
    const template = makeTemplate(3)
    return {
      ...template,
      email: {
        ...template.email,
        visual_editor_tree: {
          id: 'root',
          type: 'mjml',
          children: [
            {
              id: 'body',
              type: 'mj-body',
              attributes: {},
              children: [{ id: 'sec', type: 'mj-section', attributes: {}, children: columns }]
            }
          ]
        }
      }
    } as unknown as Template
  }
  const widths = () => {
    const tree = builder.props!.tree as EmailBlock
    return (EmailBlockClass.findBlockById(tree, 'sec')?.children ?? []).map(
      (child) => (child.attributes as Record<string, unknown>).width
    )
  }
  const callbacks = () =>
    assistant.props!.callbacks as Required<NonNullable<EmailAIAssistantProps['callbacks']>>

  const openEditor = async (template: Template) => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={template} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(builder.props?.tree).toBeDefined())
    await waitFor(() => expect(widths().length).toBeGreaterThan(0))
  }

  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    assistant.props = undefined
  })

  it('gives a column the assistant adds its share, the others keeping their ratio, as the editor does', async () => {
    await openEditor(withSection([col('a', '30%'), col('b', '70%')]))

    act(() => callbacks().onAddBlock('sec', 'mj-column', 2))

    await waitFor(() => expect(widths()).toEqual(['20%', '46.67%', '33.33%']))
  })

  it('scales the columns left when the assistant deletes one', async () => {
    await openEditor(withSection([col('a', '20%'), col('b', '30%'), col('c', '50%')]))

    act(() => callbacks().onDeleteBlock('c'))

    await waitFor(() => expect(widths()).toEqual(['40%', '60%']))
  })

  it('rebalances both sections when the assistant moves a column between them, not on a reorder', async () => {
    const template = withSection([col('a', '50%'), col('b', '50%')])
    const body = (template.email!.visual_editor_tree as unknown as EmailBlock).children![0]
    ;(body.children as EmailBlock[]).push({
      id: 'sec2',
      type: 'mj-section',
      attributes: {},
      children: [col('d', '100%')]
    } as unknown as EmailBlock)
    await openEditor(template)

    act(() => callbacks().onMoveBlock('b', 'sec', 0))
    await waitFor(() => expect(widths()).toEqual(['50%', '50%']))

    act(() => callbacks().onMoveBlock('b', 'sec2', 1))
    await waitFor(() => expect(widths()).toEqual(['100%']))
    const second = EmailBlockClass.findBlockById(builder.props!.tree as EmailBlock, 'sec2')
    expect(second?.children?.map((child) => (child.attributes as Record<string, unknown>).width)).toEqual([
      '50%',
      '50%'
    ])
  })
})

describe('CreateTemplateDrawer template ID and editor mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  async function typeTemplateId(id: string) {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()
    await userEvent.type(screen.getByPlaceholderText('i.e: welcome-email'), id)
  }

  // A deleted template keeps its id (sent messages render from it) and no list shows it,
  // so "already exists" alone leaves the author hunting for a template they deleted.
  it('says an id belongs to a deleted template', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({
      template: { ...makeTemplate(2), deleted_at: '2026-09-27T13:00:00Z' }
    })

    await typeTemplateId('tmpl1')

    expect(
      await screen.findByText("This ID belongs to a deleted template and can't be reused")
    ).toBeInTheDocument()
    expect(screen.queryByText('Template ID already exists')).not.toBeInTheDocument()
  })

  it('keeps the plain message for an id taken by a live template', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(2) })

    await typeTemplateId('tmpl1')

    expect(await screen.findByText('Template ID already exists')).toBeInTheDocument()
    expect(
      screen.queryByText("This ID belongs to a deleted template and can't be reused")
    ).not.toBeInTheDocument()
  })

  it('accepts an id the server does not know', async () => {
    ;(templatesApi.get as Mock).mockRejectedValue(new Error('Template not found'))

    await typeTemplateId('fresh-id')

    await waitFor(() => expect(templatesApi.get).toHaveBeenCalled())
    expect(screen.queryByText('Template ID already exists')).not.toBeInTheDocument()
    expect(
      screen.queryByText("This ID belongs to a deleted template and can't be reused")
    ).not.toBeInTheDocument()
  })

  // The mode is locked once saved; the field's help says so, and how to switch anyway.
  it('explains in the editor mode help that the mode is permanent', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

    const label = screen.getByText('Editor mode').closest('label')
    expect(label).not.toBeNull()
    await userEvent.hover(within(label as HTMLElement).getByRole('img', { name: 'question-circle' }))

    expect(
      await screen.findByText(/The editor mode can't be changed after the template is created/)
    ).toBeInTheDocument()
  })
})

// Names may hold 255 characters and IDs 32 (issue #463): the fields say so as the author
// types, and an ID filled in from a long name fits its limit instead of failing on save.
describe('CreateTemplateDrawer name and ID length', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Every ID is free, so the uniqueness check never gets in the way.
    ;(templatesApi.get as Mock).mockRejectedValue(new Error('Template not found'))
  })

  const idInput = () => screen.getByPlaceholderText('i.e: welcome-email') as HTMLInputElement
  const nameInput = () => screen.getByPlaceholderText('i.e: Welcome Email') as HTMLInputElement

  it('limits the name to 255 characters and the ID to 32', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

    expect(nameInput()).toHaveAttribute('maxlength', '255')
    expect(idInput()).toHaveAttribute('maxlength', '32')
  })

  it('cuts the ID filled in from a long name to 32 characters, without a trailing hyphen', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

    // kebab-cased, its first 32 characters end on a hyphen. Pasted rather than typed: each key
    // re-renders the whole drawer, which made this test slow enough to time out on CI.
    await userEvent.click(nameInput())
    await userEvent.paste('Welcome email for new customers in France')

    expect(idInput().value).toBe('welcome-email-for-new-customers')
  })

  it('keeps the -copy suffix within 32 characters when duplicating a long-named template', async () => {
    const source = { ...makeTemplate(1), name: 'Welcome email for new customers in France' }
    renderDrawer(<CreateTemplateDrawer workspace={workspace} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone template/i }))

    expect(idInput().value).toBe('welcome-email-for-new-custo-copy')
  })

  it('gives the ID "copy" to the duplicate of a template whose name has no letters or digits', async () => {
    const source = { ...makeTemplate(1), name: '!!!' }
    renderDrawer(<CreateTemplateDrawer workspace={workspace} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone template/i }))

    expect(idInput().value).toBe('copy')
  })

  it('does not save a duplicate whose name, with " copy" added, exceeds 255 characters', async () => {
    const source = { ...makeTemplate(1), name: 'a'.repeat(252) }
    renderDrawer(<CreateTemplateDrawer workspace={workspace} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone template/i }))

    expect(nameInput().value).toHaveLength(257)
    await userEvent.click(saveButton())

    expect(
      await screen.findByText('Template name must be at most 255 characters long')
    ).toBeInTheDocument()
    expect(templatesApi.create).not.toHaveBeenCalled()
  })
})

describe('CreateTemplateDrawer saving a block from the editor', () => {
  const block = { id: 'b1', type: 'mj-section', attributes: {}, children: [] } as unknown as EmailBlock
  const savedWorkspace = {
    ...workspace,
    settings: {
      ...workspace.settings,
      template_blocks: [{ id: 'header-id', name: 'Header', block }]
    }
  } as unknown as Workspace
  const toasts = (kind: string) => document.querySelectorAll(`.ant-message-notice-${kind}`)

  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    auth.refreshWorkspaces.mockResolvedValue(undefined)
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
  })

  const openEditor = async () => {
    renderDrawer(<CreateTemplateDrawer workspace={savedWorkspace} template={makeTemplate(3)} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())
    return builder.props!.onSaveBlock as SaveBlockHandler
  }

  // Characterisation: the duplicate success message came from the toolbar, not from here
  it('creates a template block, refreshes the workspace and shows one success message', async () => {
    ;(templateBlocksApi.create as Mock).mockResolvedValue({ block: { id: 'products-id' } })
    const onSaveBlock = await openEditor()

    // The created block's id, which the editor highlights in its library
    await expect(onSaveBlock(block, 'create', 'Products')).resolves.toBe('products-id')

    expect(templateBlocksApi.create).toHaveBeenCalledWith({ workspace_id: 'ws1', name: 'Products', block })
    expect(auth.refreshWorkspaces).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(toasts('success')).toHaveLength(1))
    expect(toasts('error')).toHaveLength(0)
  })

  it('rejects and shows the API error when creating a template block fails', async () => {
    ;(templateBlocksApi.create as Mock).mockRejectedValue(new Error('Name already used'))
    const onSaveBlock = await openEditor()

    await expect(onSaveBlock(block, 'create', 'Products')).rejects.toThrow()

    expect(await screen.findByText('Name already used')).toBeInTheDocument()
    expect(toasts('success')).toHaveLength(0)
    expect(auth.refreshWorkspaces).not.toHaveBeenCalled()
  })

  it('rejects without calling the API when updating a template block the workspace no longer has', async () => {
    const onSaveBlock = await openEditor()

    await expect(onSaveBlock(block, 'update', 'gone-id')).rejects.toThrow()

    expect(templateBlocksApi.update).not.toHaveBeenCalled()
    expect(await screen.findByText('Template block not found')).toBeInTheDocument()
  })

  it('resolves with a warning when the block is saved but refreshing the workspace fails', async () => {
    ;(templateBlocksApi.update as Mock).mockResolvedValue({ block: {} })
    auth.refreshWorkspaces.mockRejectedValue(new Error('network'))
    const onSaveBlock = await openEditor()

    await expect(onSaveBlock(block, 'update', 'header-id')).resolves.toBe('header-id')

    expect(templateBlocksApi.update).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      id: 'header-id',
      name: 'Header',
      block
    })
    await waitFor(() => expect(toasts('success')).toHaveLength(1))
    expect(
      await screen.findByText('Block replaced. Reload the page to see its new content in your saved blocks.')
    ).toBeInTheDocument()
    expect(toasts('error')).toHaveLength(0)
  })

  it('warns with the saved block when creating it is done but refreshing the workspace fails', async () => {
    ;(templateBlocksApi.create as Mock).mockResolvedValue({ block: { id: 'products-id' } })
    auth.refreshWorkspaces.mockRejectedValue(new Error('network'))
    const onSaveBlock = await openEditor()

    await expect(onSaveBlock(block, 'create', 'Products')).resolves.toBe('products-id')

    expect(
      await screen.findByText('Block saved. Reload the page to see it in your saved blocks.')
    ).toBeInTheDocument()
  })
})

describe('CreateTemplateDrawer managing saved blocks', () => {
  const block = {
    id: 'b1',
    type: 'mj-section',
    attributes: {},
    children: []
  } as unknown as EmailBlock
  const savedWorkspace = {
    ...workspace,
    settings: {
      ...workspace.settings,
      template_blocks: [{ id: 'header-id', name: 'Header', block }]
    }
  } as unknown as Workspace
  const toasts = (kind: string) => document.querySelectorAll(`.ant-message-notice-${kind}`)

  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    auth.refreshWorkspaces.mockResolvedValue(undefined)
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
  })

  const openEditor = async (canManageSavedBlocks?: boolean) => {
    renderDrawer(
      <CreateTemplateDrawer
        workspace={savedWorkspace}
        template={makeTemplate(3)}
        canManageSavedBlocks={canManageSavedBlocks}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())
    return builder.props!
  }

  it('lets the editor change the library only when told the member may', async () => {
    expect((await openEditor()).canManageSavedBlocks).toBeFalsy()
  })

  it('hands the editor the right to change the library', async () => {
    expect((await openEditor(true)).canManageSavedBlocks).toBe(true)
  })

  it('renames a saved block, keeping its content as the server has it, then refreshes', async () => {
    // Replaced elsewhere since the workspace was loaded: the server's content is the one kept
    const replaced = { ...block, id: 'b2' } as EmailBlock
    ;(templateBlocksApi.get as Mock).mockResolvedValue({
      block: { id: 'header-id', name: 'Header', block: replaced }
    })
    ;(templateBlocksApi.update as Mock).mockResolvedValue({ block: {} })
    const props = await openEditor(true)
    const rename = props.onRenameSavedBlock as RenameSavedBlockHandler

    await expect(rename('header-id', 'Top')).resolves.toBeUndefined()

    expect(templateBlocksApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'header-id' })
    expect(templateBlocksApi.update).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      id: 'header-id',
      name: 'Top',
      block: replaced
    })
    expect(auth.refreshWorkspaces).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('Block renamed to "Top"')).toBeInTheDocument()
  })

  it('rejects and shows the error when renaming fails', async () => {
    ;(templateBlocksApi.get as Mock).mockResolvedValue({ block: { id: 'header-id', name: 'Header', block } })
    ;(templateBlocksApi.update as Mock).mockRejectedValue(new Error('Server down'))
    const rename = (await openEditor(true)).onRenameSavedBlock as RenameSavedBlockHandler

    await expect(rename('header-id', 'Top')).rejects.toThrow()

    expect(await screen.findByText('Server down')).toBeInTheDocument()
    expect(auth.refreshWorkspaces).not.toHaveBeenCalled()
  })

  it('refreshes when the block to rename is already gone from the server', async () => {
    ;(templateBlocksApi.get as Mock).mockRejectedValue(new ApiError('Template block not found', 404))
    const rename = (await openEditor(true)).onRenameSavedBlock as RenameSavedBlockHandler

    await expect(rename('header-id', 'Top')).rejects.toThrow()
    expect(templateBlocksApi.update).not.toHaveBeenCalled()
    expect(auth.refreshWorkspaces).toHaveBeenCalledTimes(1)
  })

  it('refreshes when the block to rename goes between the read and the update', async () => {
    ;(templateBlocksApi.get as Mock).mockResolvedValue({ block: { id: 'header-id', name: 'Header', block } })
    ;(templateBlocksApi.update as Mock).mockRejectedValue(new ApiError('Template block not found', 404))
    const rename = (await openEditor(true)).onRenameSavedBlock as RenameSavedBlockHandler

    await expect(rename('header-id', 'Top')).rejects.toThrow()
    expect(auth.refreshWorkspaces).toHaveBeenCalledTimes(1)
  })

  it('deletes a saved block, then refreshes', async () => {
    ;(templateBlocksApi.delete as Mock).mockResolvedValue({ success: true })
    const remove = (await openEditor(true)).onDeleteSavedBlock as DeleteSavedBlockHandler

    await expect(remove('header-id')).resolves.toBeUndefined()

    expect(templateBlocksApi.delete).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'header-id' })
    expect(auth.refreshWorkspaces).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('Block "Header" deleted')).toBeInTheDocument()
  })

  it('counts a block already gone from the server as deleted', async () => {
    ;(templateBlocksApi.delete as Mock).mockRejectedValue(
      new ApiError('Template block not found', 404)
    )
    const remove = (await openEditor(true)).onDeleteSavedBlock as DeleteSavedBlockHandler

    await expect(remove('header-id')).resolves.toBeUndefined()

    expect(auth.refreshWorkspaces).toHaveBeenCalledTimes(1)
    expect(toasts('error')).toHaveLength(0)
  })

  it('rejects and shows the error when deleting fails', async () => {
    ;(templateBlocksApi.delete as Mock).mockRejectedValue(new ApiError('Server down', 500))
    const remove = (await openEditor(true)).onDeleteSavedBlock as DeleteSavedBlockHandler

    await expect(remove('header-id')).rejects.toThrow()

    expect(await screen.findByText('Server down')).toBeInTheDocument()
    expect(auth.refreshWorkspaces).not.toHaveBeenCalled()
  })

  it('warns about the operation done when the refresh after it fails', async () => {
    ;(templateBlocksApi.delete as Mock).mockResolvedValue({ success: true })
    auth.refreshWorkspaces.mockRejectedValue(new Error('network'))
    const remove = (await openEditor(true)).onDeleteSavedBlock as DeleteSavedBlockHandler

    await expect(remove('header-id')).resolves.toBeUndefined()

    expect(
      await screen.findByText('Block deleted. Reload the page to update your saved blocks.')
    ).toBeInTheDocument()
  })
})

describe('CreateTemplateDrawer text version', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
  })

  it('turns on the text version mode and hands the compiled text to the builder', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    ;(templatesApi.compile as Mock).mockResolvedValue({
      html: '<p>Hi</p>',
      mjml: '<mjml></mjml>',
      text: 'Hi\n'
    })

    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())

    expect(builder.props?.textVersion).toMatchObject({ enabled: true })
    const onCompile = builder.props?.onCompile as (tree: unknown) => Promise<{ text?: string }>
    await expect(onCompile({ id: 'root', type: 'mjml', children: [] })).resolves.toMatchObject({
      html: '<p>Hi</p>',
      text: 'Hi\n'
    })
  })
})

interface BuilderTextVersion {
  enabled: true
  custom: {
    value?: string
    onChange: (v?: string) => void
    onSeed: () => Promise<{ text?: string; error?: string; textError?: string; lostLiquid?: boolean }>
    category?: string
  }
}

describe('CreateTemplateDrawer custom text version', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
  })

  const withText = (text: string | undefined, category = 'transactional'): Template =>
    ({ ...makeTemplate(3), category, email: { ...makeTemplate(3).email, text } }) as unknown as Template
  const textVersion = () => builder.props?.textVersion as BuilderTextVersion

  const openEdit = async (tpl: Template) => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: tpl })
    ;(templatesApi.update as Mock).mockResolvedValue({ template: { ...tpl, version: 4 } })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={tpl} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())
  }

  const save = async () => {
    await editName()
    await userEvent.click(saveButton())
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalled())
    return (templatesApi.update as Mock).mock.calls[0][0] as { email: { text?: string } }
  }

  it('loads the custom text of the template and its category', async () => {
    await openEdit(withText('Hi {{ contact.first_name }}'))
    expect(textVersion().custom.value).toBe('Hi {{ contact.first_name }}')
    expect(textVersion().custom.category).toBe('transactional')
  })

  it('treats a blank stored text as automatic', async () => {
    await openEdit(withText('  \n '))
    expect(textVersion().custom.value).toBeUndefined()
  })

  it('saves the text written in the editor', async () => {
    await openEdit(withText(undefined))
    act(() => textVersion().custom.onChange('Hello {{ contact.first_name }}'))
    await waitFor(() => expect(textVersion().custom.value).toBe('Hello {{ contact.first_name }}'))
    const payload = await save()
    expect(payload.email.text).toBe('Hello {{ contact.first_name }}')
  })

  it('saves no text after going back to automatic', async () => {
    await openEdit(withText('Mine'))
    act(() => textVersion().custom.onChange(undefined))
    await waitFor(() => expect(textVersion().custom.value).toBeUndefined())
    const payload = await save()
    expect(payload.email).not.toHaveProperty('text')
  })

  it('never asks the editor compile for the custom text', async () => {
    ;(templatesApi.compile as Mock).mockResolvedValue({ html: '<p>Hi</p>', mjml: '', text: 'Hi\n' })
    await openEdit(withText('Mine'))
    const onCompile = builder.props?.onCompile as (tree: unknown) => Promise<unknown>
    await onCompile({ id: 'root', type: 'mjml', children: [] })
    expect((templatesApi.compile as Mock).mock.calls[0][0]).not.toHaveProperty('text')
  })

  it('starts from the automatic version with Liquid kept, no test data and no tracking', async () => {
    ;(templatesApi.compile as Mock).mockResolvedValue({
      html: '<p>Hi</p>',
      mjml: '<mjml>{% for i in items %}<mj-section></mj-section>{% endfor %}</mjml>',
      text: 'Hi {{ contact.first_name }}\n'
    })
    await openEdit(withText(undefined))
    // The Settings step compiles its own preview: the seed's request is the last one.
    ;(templatesApi.compile as Mock).mockClear()
    const seed = await textVersion().custom.onSeed()

    const request = (templatesApi.compile as Mock).mock.calls[0][0]
    expect(request.preserve_liquid).toBe(true)
    expect(request.test_data).toBeUndefined()
    expect(request.tracking_settings?.enable_tracking).toBe(false)
    expect(request).not.toHaveProperty('text')
    expect(seed).toMatchObject({ text: 'Hi {{ contact.first_name }}\n', lostLiquid: true })
  })

  it('reports a starting point that could not be generated', async () => {
    ;(templatesApi.compile as Mock).mockResolvedValue({ html: '', mjml: '', error: { message: 'XML syntax error' } })
    await openEdit(withText(undefined))
    await expect(textVersion().custom.onSeed()).resolves.toMatchObject({ error: 'XML syntax error' })
  })

  it('forgets the text of the previous template when creating a new one', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()
    await waitFor(() => expect(builder.props).toBeDefined())
    act(() => textVersion().custom.onChange('Leftover'))
    await waitFor(() => expect(textVersion().custom.value).toBe('Leftover'))

    await closeDrawer()
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()

    await waitFor(() => expect(textVersion().custom.value).toBeUndefined())
  })

  it('drops a text that arrives for a drawer already closed', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()
    await waitFor(() => expect(builder.props).toBeDefined())
    // The panel keeps the handlers of the render that started a seed.
    const startedWith = textVersion().custom

    await closeDrawer()
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()
    act(() => startedWith.onChange('Seed of the cancelled template'))

    await waitFor(() => expect(textVersion().custom.value).toBeUndefined())
    act(() => textVersion().custom.onChange('Mine'))
    await waitFor(() => expect(textVersion().custom.value).toBe('Mine'))
  })

  const multilingual = {
    ...workspace,
    settings: { ...workspace.settings, languages: ['en', 'fr'] }
  } as unknown as Workspace
  const withTranslationText = (frText: string): Template =>
    ({
      ...withText('Hi'),
      translations: {
        fr: { email: { ...makeTemplate(3).email, subject: 'Bonjour', subject_preview: 'Salut', text: frText } }
      }
    }) as unknown as Template

  it('sends back the text a translation holds', async () => {
    const tpl = withTranslationText('Salut {{ contact.first_name }}')
    ;(templatesApi.get as Mock).mockResolvedValue({ template: tpl })
    ;(templatesApi.update as Mock).mockResolvedValue({ template: { ...tpl, version: 4 } })
    renderDrawer(<CreateTemplateDrawer workspace={multilingual} template={tpl} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await editName()
    await userEvent.click(saveButton())
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalled())

    const payload = (templatesApi.update as Mock).mock.calls[0][0]
    expect(payload.email.text).toBe('Hi')
    expect(payload.translations.fr.email.text).toBe('Salut {{ contact.first_name }}')
  })

  it('copies the texts of the template it duplicates', async () => {
    const source = withTranslationText('Salut')
    // The copy's ID is free.
    ;(templatesApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
    ;(templatesApi.create as Mock).mockResolvedValue({ template: source })
    renderDrawer(<CreateTemplateDrawer workspace={multilingual} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone template/i }))
    await waitFor(() => expect(textVersion().custom.value).toBe('Hi'))
    // Opened again after a Cancel, the copy starts from the source again.
    await closeDrawer()
    await userEvent.click(screen.getByRole('button', { name: /Clone template/i }))
    await waitFor(() => expect(textVersion().custom.value).toBe('Hi'))
    await userEvent.click(saveButton())
    await waitFor(() => expect(templatesApi.create).toHaveBeenCalled())

    const payload = (templatesApi.create as Mock).mock.calls[0][0]
    expect(payload.email.text).toBe('Hi')
    expect(payload.translations.fr.email.text).toBe('Salut')
  })

  it('refuses to save a translation without preview text, and has the tab show its language', async () => {
    // The refusal was a short toast and a red dot on the step: the tab is told which language it
    // is about, to show it with its empty fields marked.
    const tpl = {
      ...withText('Hi'),
      translations: { fr: { email: { ...makeTemplate(3).email, subject: 'Bonjour', subject_preview: '' } } }
    } as unknown as Template
    ;(templatesApi.get as Mock).mockResolvedValue({ template: tpl })
    renderDrawer(<CreateTemplateDrawer workspace={multilingual} template={tpl} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await editName()
    await userEvent.click(saveButton())

    await waitFor(() => expect(translationsTab.props?.missing).toEqual({ lang: 'fr', attempt: expect.any(Number) }))
    expect(templatesApi.update).not.toHaveBeenCalled()
  })

  it('takes the text of a newer revision found on opening', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: { ...withText('Newer text'), version: 5 } })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={withText('Older text')} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))

    await waitFor(() => expect(textVersion().custom.value).toBe('Newer text'))
  })

  it('takes the translation texts of a newer revision found on opening', async () => {
    const newer = { ...withTranslationText('Texte plus récent'), version: 5 } as Template
    ;(templatesApi.get as Mock).mockResolvedValue({ template: newer })
    ;(templatesApi.update as Mock).mockResolvedValue({ template: { ...newer, version: 6 } })
    renderDrawer(<CreateTemplateDrawer workspace={multilingual} template={withTranslationText('Ancien texte')} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(templatesApi.get).toHaveBeenCalled())
    await screen.findByText(/Loaded the latest version/)
    await editName()
    await userEvent.click(saveButton())
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalled())

    expect((templatesApi.update as Mock).mock.calls[0][0].translations.fr.email.text).toBe('Texte plus récent')
  })

  const refuseText = async (error: string) => {
    const tpl = withText('Hi {% if %}')
    ;(templatesApi.get as Mock).mockResolvedValue({ template: tpl })
    ;(templatesApi.update as Mock).mockRejectedValue(new ApiError(error, 400))
    const multilingual = {
      ...workspace,
      settings: { ...workspace.settings, languages: ['en', 'fr'] }
    } as unknown as Workspace
    renderDrawer(<CreateTemplateDrawer workspace={multilingual} template={tpl} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await editName()
    await userEvent.click(saveButton())
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalled())
    expect(await screen.findByText(new RegExp(error.replace(/[{}%]/g, '.')))).toBeInTheDocument()
  }

  it('goes back to the email editor when the server refuses its text version', async () => {
    await refuseText('invalid email template: text version: Liquid syntax error')
    // The email editor's step opens, where its text version is written.
    await waitFor(() => expect(selectedStep()).toBe('Content'))
  })

  it('stays on the translations when the refused text version is a translation', async () => {
    await refuseText("translation 'fr': invalid email template: text version: Liquid syntax error")
    expect(selectedStep()).not.toBe('Content')
  })

  it('warns, without blocking, when a marketing text has no unsubscribe link', async () => {
    await openEdit(withText('Hi there', 'marketing'))
    await save()
    expect(await screen.findByText(/has no unsubscribe link/)).toBeInTheDocument()
  })

  it('does not warn a transactional text about an unsubscribe link', async () => {
    await openEdit(withText('Hi there', 'transactional'))
    await save()
    expect(screen.queryByText(/has no unsubscribe link/)).toBeNull()
  })
})

// The templates page opens the drawer itself (row menu, New template, Import): no button of
// its own, open and close driven by the page.
describe('CreateTemplateDrawer controlled by the page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    ;(templatesApi.get as Mock).mockRejectedValue(new Error('Template not found'))
  })

  const nameInput = () => screen.getByPlaceholderText('i.e: Welcome Email') as HTMLInputElement
  const idInput = () => screen.getByPlaceholderText('i.e: welcome-email') as HTMLInputElement

  it('renders no button and opens when told to', async () => {
    const { rerender } = renderDrawer(
      <CreateTemplateDrawer workspace={workspace} open={false} onClose={vi.fn()} />
    )
    expect(screen.queryByRole('button', { name: /Create template/i })).not.toBeInTheDocument()
    expect(screen.queryByTestId('template-bar-name')).not.toBeInTheDocument()

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <AntApp>
          <CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />
        </AntApp>
      </QueryClientProvider>
    )
    await pickBlank()
    expect(await screen.findByTestId('template-bar-name')).toHaveTextContent('New template')
  })

  it('opens an existing template and freshens it like the button does', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    renderDrawer(
      <CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} open onClose={vi.fn()} />
    )
    expect(await screen.findByTestId('template-bar-name')).toHaveTextContent('Welcomev3')
    await waitFor(() => expect(nameInput().value).toBe('Welcome'))
    expect(templatesApi.get).toHaveBeenCalled()
  })

  it('calls onClose on the back button, for the page to close it', async () => {
    const onClose = vi.fn()
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={onClose} />)
    await pickBlank()
    await screen.findByTestId('template-bar-name')
    await closeDrawer()
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('starts a new template from an imported tree, name and test data, saving nothing', async () => {
    const importedTree = {
      id: 'imported',
      type: 'mjml',
      children: []
    } as unknown as EmailBlock
    renderDrawer(
      <CreateTemplateDrawer
        workspace={workspace}
        open
        onClose={vi.fn()}
        initialName="Spring sale"
        initialTree={importedTree}
        initialTestData={{ contact: { first_name: 'Ada' } }}
      />
    )
    await screen.findByTestId('template-bar-name')
    expect(nameInput().value).toBe('Spring sale')
    expect(idInput().value).toBe('spring-sale')
    await waitFor(() => expect(builder.props?.tree).toEqual(importedTree))
    expect(builder.props?.testData).toEqual({ contact: { first_name: 'Ada' } })
    expect(templatesApi.create).not.toHaveBeenCalled()
  })
})

// The drawer of the Nuit claire mockup: steps in the bar, open at any time; the save button says
// what it writes; a test sends the saved version; closing asks before losing changes.
describe('CreateTemplateDrawer bar, steps, test send and close', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    auth.permissions = { templates: { read: true, write: true }, transactional: { read: true, write: true } }
  })

  const nameInput = () => screen.getByPlaceholderText('i.e: Welcome Email') as HTMLInputElement

  const openNew = async () => {
    ;(templatesApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create template/i }))
    await pickBlank()
    await screen.findByTestId('template-bar-name')
  }

  const openSaved = async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    ;(templatesApi.update as Mock).mockResolvedValue({ template: makeTemplate(4) })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit template/i }))
    await waitFor(() => expect(nameInput().value).toBe('Welcome'))
  }

  it('opens the Content step of a new template straight away', async () => {
    await openNew()
    expect(selectedStep()).toBe('Settings')
    await userEvent.click(stepButton('template'))
    expect(selectedStep()).toBe('Content')
    expect(screen.queryByRole('button', { name: 'Next' })).toBeNull()
  })

  it('opens the Settings step and marks it when a field there blocks the save', async () => {
    await openNew()
    await userEvent.click(stepButton('template'))
    await userEvent.click(saveButton())

    await waitFor(() => expect(selectedStep()).toContain('Settings'))
    expect(document.querySelector('[data-step="settings"] [data-step-error]')).not.toBeNull()
    expect(templatesApi.create).not.toHaveBeenCalled()
  })

  it('cannot send a test before the template exists', async () => {
    await openNew()
    expect(screen.getByRole('button', { name: /Send test/ })).toBeDisabled()
  })

  it('cannot send a test without the permissions a test send needs', async () => {
    auth.permissions = { templates: { read: true, write: true }, transactional: { read: true, write: false } }
    await openSaved()
    expect(screen.getByRole('button', { name: /Send test/ })).toBeDisabled()
  })

  it('sends the saved version straight away when nothing changed', async () => {
    await openSaved()
    await userEvent.click(screen.getByRole('button', { name: /Send test/ }))
    expect(await screen.findByTestId('send-test')).toHaveAttribute('data-version', '3')
  })

  it('with changes waiting, sends the saved version when asked to', async () => {
    await openSaved()
    await userEvent.type(nameInput(), ' v2')
    await userEvent.click(screen.getByRole('button', { name: /Send test/ }))

    await userEvent.click(await screen.findByRole('button', { name: 'Send the saved version' }))
    expect(await screen.findByTestId('send-test')).toHaveAttribute('data-version', '3')
    expect(templatesApi.update).not.toHaveBeenCalled()
  })

  it('with changes waiting, saves them and sends the new version, keeping the drawer open', async () => {
    await openSaved()
    await userEvent.type(nameInput(), ' v2')
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Send test/ }))

    await userEvent.click(await screen.findByRole('button', { name: 'Save v4 and send' }))
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalled())
    expect(await screen.findByTestId('send-test')).toHaveAttribute('data-version', '4')
    expect(screen.getByTestId('template-bar-name')).toHaveTextContent('v4')
    expect(screen.queryByText('Unsaved changes')).toBeNull()
    // The next save is based on the revision just written, so it is not a false conflict.
    expect(screen.getByRole('button', { name: /Save v5/ })).toBeInTheDocument()
  })

  it('closes an untouched drawer at once', async () => {
    const onClose = vi.fn()
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} open onClose={onClose} />)
    await waitFor(() => expect(nameInput().value).toBe('Welcome'))
    await userEvent.click(screen.getByRole('button', { name: /^Templates$/ }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('asks before closing over changes, and keeps editing when told to', async () => {
    const onClose = vi.fn()
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} open onClose={onClose} />)
    await waitFor(() => expect(nameInput().value).toBe('Welcome'))
    await userEvent.type(nameInput(), '!')

    await userEvent.click(screen.getByRole('button', { name: /^Templates$/ }))
    await userEvent.click(await screen.findByRole('button', { name: 'Keep editing' }))
    expect(onClose).not.toHaveBeenCalled()
    expect(nameInput().value).toBe('Welcome!')

    await userEvent.click(screen.getByRole('button', { name: /^Templates$/ }))
    await userEvent.click(await screen.findByRole('button', { name: 'Discard' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

// A new template starts from the gallery, a template of the workspace or Blank; the drawer opens
// filled with the choice.
describe('CreateTemplateDrawer started from the gallery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    auth.permissions = { templates: { read: true, write: true }, transactional: { read: true, write: true } }
    ;(templatesApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
  })

  const field = (placeholder: string) => screen.getByPlaceholderText(placeholder) as HTMLInputElement

  it('asks where to start before opening, and creates nothing when cancelled', async () => {
    const onClose = vi.fn()
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={onClose} />)
    expect(await screen.findByTestId('start-modal')).toBeInTheDocument()
    expect(screen.queryByTestId('template-bar-name')).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: 'cancel start' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(templatesApi.create).not.toHaveBeenCalled()
  })

  it('fills the drawer with a gallery model and says where it came from', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick order' }))

    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    expect(field('i.e: welcome-email').value).toBe('order-confirmation')
    expect(screen.getByRole('textbox', { name: 'Subject' })).toHaveValue('Your order #{{ order.number }} is confirmed')
    expect(screen.getByTestId('starting-point')).toHaveTextContent('Started from “Order confirmation”')
    expect(screen.getByTestId('starting-point')).toHaveTextContent('Gallery · English')
    expect(JSON.stringify(builder.props?.tree)).toContain('order.items')
    expect((builder.props?.testData as { order?: unknown }).order).toBeDefined()
    // Nothing changed yet: closing does not ask.
    expect(screen.queryByText('Unsaved changes')).toBeNull()
  })

  it('takes the next free ID when the model was used before', async () => {
    ;(templatesApi.get as Mock).mockImplementation(({ id }: { id: string }) =>
      id === 'order-confirmation' ? Promise.resolve({ template: makeTemplate(1) }) : Promise.reject(new ApiError('not found', 404))
    )
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick order' }))
    await waitFor(() => expect(field('i.e: welcome-email').value).toBe('order-confirmation-2'))
  })

  it('keeps the category imposed by where the template is created', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} forceCategory="marketing" />)
    expect(await screen.findByTestId('start-modal')).toHaveAttribute('data-force-category', 'marketing')
    await userEvent.click(screen.getByRole('button', { name: 'pick order' }))
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    await waitFor(() => expect(assistant.props?.category).toBe('marketing'))
  })

  it('copies a template of the workspace, keeping its editor mode', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick template' }))

    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Spring news copy'))
    expect(screen.getByRole('radio', { name: /Code \(MJML\)/ })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: /Visual/ })).toBeDisabled()
    expect(screen.getByTestId('starting-point')).toHaveTextContent('Your templates')
  })

  it('asks before Change replaces content that was edited', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick order' }))
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    await userEvent.type(field('i.e: Welcome Email'), ' (EU)')

    await userEvent.click(within(screen.getByTestId('starting-point')).getByRole('button', { name: 'Change' }))
    expect(screen.queryByTestId('start-modal')).toBeNull()
    await userEvent.click(await screen.findByRole('button', { name: 'Choose another' }))
    expect(await screen.findByTestId('start-modal')).toBeInTheDocument()
  })

  it('opens an imported file straight away, without the start modal', async () => {
    const tree = { id: 'imported', type: 'mjml', children: [] } as unknown as EmailBlock
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} initialName="Imported" initialTree={tree} />)
    expect(await screen.findByTestId('template-bar-name')).toHaveTextContent('Imported')
    expect(screen.queryByTestId('start-modal')).toBeNull()
  })

  it('locks a gallery model to the visual editor, its content being visual', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick order' }))
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    expect(screen.getByRole('radio', { name: /Code \(MJML\)/ })).toBeDisabled()
  })

  it('leaves Blank free to switch to code', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick blank' }))
    expect(await screen.findByRole('radio', { name: /Code \(MJML\)/ })).not.toBeDisabled()
  })

  it('does not carry the sender of an earlier choice over to a model', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick template' }))
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Spring news copy'))
    expect(screen.getByRole('textbox', { name: 'Reply to' })).toHaveValue('news@acme.io')
    await userEvent.click(within(screen.getByTestId('starting-point')).getByRole('button', { name: 'Change' }))
    await userEvent.click(await screen.findByRole('button', { name: 'pick order' }))
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))

    ;(templatesApi.create as Mock).mockResolvedValue({ template: makeTemplate(1) })
    await userEvent.click(saveButton())
    await waitFor(() => expect(templatesApi.create).toHaveBeenCalled())
    const payload = (templatesApi.create as Mock).mock.calls[0][0]
    expect(payload.email.sender_id).toBeUndefined()
    expect(payload.email.reply_to).toBeUndefined()
    expect(payload.email.editor_mode).toBe('visual')
  })

  it('applies one pick when a model is chosen twice in a row', async () => {
    let release: () => void = () => undefined
    ;(templatesApi.get as Mock).mockImplementation(
      () => new Promise((_, reject) => (release = () => reject(new ApiError('not found', 404))))
    )
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    const pick = await screen.findByRole('button', { name: 'pick order' })
    await userEvent.click(pick)
    await userEvent.click(pick)
    act(() => release())
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    expect(templatesApi.get).toHaveBeenCalledTimes(1)
  })

  it('names the drawer for screen readers', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick blank' }))
    expect(await screen.findByRole('dialog', { name: 'Create an email template' })).toBeInTheDocument()
  })
})

// A form that opens the drawer (the template selector) chooses what was saved, and tells a save
// from a cancel by what onClose receives.
describe('CreateTemplateDrawer reporting what it saved', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    builder.props = undefined
    auth.permissions = { templates: { read: true, write: true }, transactional: { read: true, write: true } }
    ;(templatesApi.get as Mock).mockRejectedValue(new ApiError('not found', 404))
  })

  const field = (placeholder: string) => screen.getByPlaceholderText(placeholder) as HTMLInputElement

  it('hands the created template to onClose, with the message it is given', async () => {
    const onClose = vi.fn()
    const created = { ...makeTemplate(1), id: 'order-confirmation', name: 'Order confirmation' }
    ;(templatesApi.create as Mock).mockResolvedValue({ template: created })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={onClose} successMessage="Template created and selected" />)
    await userEvent.click(await screen.findByRole('button', { name: 'pick order' }))
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))

    await userEvent.click(saveButton())
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
    expect(onClose.mock.calls[0][0]).toEqual(created)
    expect(await screen.findByText('Template created and selected')).toBeInTheDocument()
    expect(screen.queryByText('Template created successfully')).toBeNull()
  })

  it('hands onClose nothing when closed without a save', async () => {
    const onClose = vi.fn()
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={onClose} />)
    await pickBlank()
    await screen.findByTestId('template-bar-name')
    await closeDrawer()
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(onClose.mock.calls[0][0]).toBeUndefined()
  })

  it('hands onClose nothing when the gallery is cancelled', async () => {
    const onClose = vi.fn()
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={onClose} />)
    await userEvent.click(await screen.findByRole('button', { name: 'cancel start' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(onClose.mock.calls[0][0]).toBeUndefined()
  })

  it('starts from the gallery model it is given, without the gallery', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} startModelId="order" forceCategory="transactional" />)
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    expect(screen.queryByTestId('start-modal')).toBeNull()
    expect(field('i.e: welcome-email').value).toBe('order-confirmation')
    expect(JSON.stringify(builder.props?.tree)).toContain('order.items')
    await waitFor(() => expect(assistant.props?.category).toBe('transactional'))
  })

  it('opens at once on a loading state while the model it is given is prepared', async () => {
    let release: () => void = () => undefined
    ;(templatesApi.get as Mock).mockImplementation(
      () => new Promise((_, reject) => (release = () => reject(new ApiError('not found', 404))))
    )
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={vi.fn()} startModelId="order" />)
    expect(await screen.findByRole('dialog', { name: 'Create an email template' })).toBeInTheDocument()
    expect(screen.getByTestId('template-preparing')).toHaveTextContent('Loading...')
    // Nothing to save while it loads
    expect(screen.queryByRole('button', { name: /^(Create template|Save v\d+)$/ })).toBeNull()

    await waitFor(() => expect(templatesApi.get).toHaveBeenCalled())
    act(() => release())
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Order confirmation'))
    expect(screen.queryByTestId('template-preparing')).toBeNull()
  })

  it('opens nothing later when closed while the model it is given is prepared', async () => {
    let release: () => void = () => undefined
    ;(templatesApi.get as Mock).mockImplementation(
      () => new Promise((_, reject) => (release = () => reject(new ApiError('not found', 404))))
    )
    const onClose = vi.fn()
    renderDrawer(<CreateTemplateDrawer workspace={workspace} open onClose={onClose} startModelId="order" />)
    await screen.findByTestId('template-preparing')
    await waitFor(() => expect(templatesApi.get).toHaveBeenCalled())

    await closeDrawer()
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(onClose.mock.calls[0][0]).toBeUndefined()
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Create an email template' })).toBeNull())

    act(() => release())
    // The lookup answered after the close: the drawer stays shut.
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(screen.queryByRole('dialog', { name: 'Create an email template' })).toBeNull()
    expect(screen.queryByPlaceholderText('i.e: Welcome Email')).toBeNull()
  })

  it('stays open on an edit once saved, says which version, and hands it over on close', async () => {
    // Saving used to close the editor: the author had to reopen the template to go on.
    const onClose = vi.fn()
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate(3) })
    ;(templatesApi.update as Mock).mockResolvedValue({ template: makeTemplate(4) })
    renderDrawer(
      <CreateTemplateDrawer workspace={workspace} template={makeTemplate(3)} open onClose={onClose} successMessage="Template created and selected" />
    )
    await waitFor(() => expect(field('i.e: Welcome Email').value).toBe('Welcome'))
    expect(saveButton()).toBeDisabled()
    await userEvent.type(field('i.e: Welcome Email'), '!')
    expect(saveButton()).toHaveTextContent('Save v4')
    await userEvent.click(saveButton())

    expect(await screen.findByText('Saved as v4')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
    // Still open, on the new revision, with nothing left to save.
    expect(field('i.e: Welcome Email').value).toBe('Welcome!')
    await waitFor(() => expect(saveButton()).toHaveTextContent('Save v5'))
    expect(saveButton()).toBeDisabled()

    await closeDrawer()
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
    expect(onClose.mock.calls[0][0]).toEqual(makeTemplate(4))
  })
})
