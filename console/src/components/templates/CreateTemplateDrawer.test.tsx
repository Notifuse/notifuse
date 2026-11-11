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
import type { EmailBlock, SaveBlockHandler } from '../email_builder/types'

// The editor pulls in Monaco / the visual builder / AI assistant, none of which are
// relevant to the logic under test — stub them out. The assistant stub keeps the one
// prop these tests are about, the template category, readable from the DOM: it changes
// a render after the form field settles, so an attribute waitFor can see beats a
// captured variable.
// The visual builder stub records its last props, so the tests can call the handlers the drawer gives it
// (onSaveBlock, onCompile).
const builder = vi.hoisted(() => ({ props: undefined as undefined | Record<string, unknown> }))
const auth = vi.hoisted(() => ({ refreshWorkspaces: vi.fn() }))
vi.mock('../email_builder/EmailBuilder', () => ({
  default: (props: Record<string, unknown>) => {
    builder.props = props
    return null
  }
}))
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
  useAuth: () => ({ refreshWorkspaces: auth.refreshWorkspaces })
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

describe('CreateTemplateDrawer template ID and editor mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  async function typeTemplateId(id: string) {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))
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
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

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
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

    expect(nameInput()).toHaveAttribute('maxlength', '255')
    expect(idInput()).toHaveAttribute('maxlength', '32')
  })

  it('cuts the ID filled in from a long name to 32 characters, without a trailing hyphen', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

    // kebab-cased, its first 32 characters end on a hyphen
    await userEvent.type(nameInput(), 'Welcome email for new customers in France')

    expect(idInput().value).toBe('welcome-email-for-new-customers')
  })

  it('keeps the -copy suffix within 32 characters when duplicating a long-named template', async () => {
    const source = { ...makeTemplate(1), name: 'Welcome email for new customers in France' }
    renderDrawer(<CreateTemplateDrawer workspace={workspace} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone Template/i }))

    expect(idInput().value).toBe('welcome-email-for-new-custo-copy')
  })

  it('gives the ID "copy" to the duplicate of a template whose name has no letters or digits', async () => {
    const source = { ...makeTemplate(1), name: '!!!' }
    renderDrawer(<CreateTemplateDrawer workspace={workspace} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone Template/i }))

    expect(idInput().value).toBe('copy')
  })

  it('does not save a duplicate whose name, with " copy" added, exceeds 255 characters', async () => {
    const source = { ...makeTemplate(1), name: 'a'.repeat(252) }
    renderDrawer(<CreateTemplateDrawer workspace={workspace} fromTemplate={source} />)
    await userEvent.click(screen.getByRole('button', { name: /Clone Template/i }))

    expect(nameInput().value).toHaveLength(257)
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

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
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))
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
      await screen.findByText('Block saved. Reload the page to see it in your saved blocks.')
    ).toBeInTheDocument()
    expect(toasts('error')).toHaveLength(0)
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
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))
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
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())
  }

  const save = async () => {
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
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
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())
    act(() => textVersion().custom.onChange('Leftover'))
    await waitFor(() => expect(textVersion().custom.value).toBe('Leftover'))

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))

    await waitFor(() => expect(textVersion().custom.value).toBeUndefined())
  })

  it('drops a text that arrives for a drawer already closed', async () => {
    renderDrawer(<CreateTemplateDrawer workspace={workspace} />)
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))
    await waitFor(() => expect(builder.props).toBeDefined())
    // The panel keeps the handlers of the render that started a seed.
    const startedWith = textVersion().custom

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await userEvent.click(screen.getByRole('button', { name: /Create Template/i }))
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
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
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
    await userEvent.click(screen.getByRole('button', { name: /Clone Template/i }))
    await waitFor(() => expect(textVersion().custom.value).toBe('Hi'))
    // Opened again after a Cancel, the copy starts from the source again.
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await userEvent.click(screen.getByRole('button', { name: /Clone Template/i }))
    await waitFor(() => expect(textVersion().custom.value).toBe('Hi'))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(templatesApi.create).toHaveBeenCalled())

    const payload = (templatesApi.create as Mock).mock.calls[0][0]
    expect(payload.email.text).toBe('Hi')
    expect(payload.translations.fr.email.text).toBe('Salut')
  })

  it('takes the text of a newer revision found on opening', async () => {
    ;(templatesApi.get as Mock).mockResolvedValue({ template: { ...withText('Newer text'), version: 5 } })
    renderDrawer(<CreateTemplateDrawer workspace={workspace} template={withText('Older text')} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))

    await waitFor(() => expect(textVersion().custom.value).toBe('Newer text'))
  })

  it('takes the translation texts of a newer revision found on opening', async () => {
    const newer = { ...withTranslationText('Texte plus récent'), version: 5 } as Template
    ;(templatesApi.get as Mock).mockResolvedValue({ template: newer })
    ;(templatesApi.update as Mock).mockResolvedValue({ template: { ...newer, version: 6 } })
    renderDrawer(<CreateTemplateDrawer workspace={multilingual} template={withTranslationText('Ancien texte')} />)
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))
    await waitFor(() => expect(templatesApi.get).toHaveBeenCalled())
    await screen.findByText(/Loaded the latest version/)
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
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
    await userEvent.click(screen.getByRole('button', { name: /Edit Template/i }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalled())
    expect(await screen.findByText(new RegExp(error.replace(/[{}%]/g, '.')))).toBeInTheDocument()
  }

  it('goes back to the email editor when the server refuses its text version', async () => {
    await refuseText('invalid email template: text version: Liquid syntax error')
    // The email editor's footer: back to the settings, on to the translations.
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Save' })).toBeNull())
    expect(screen.getByRole('button', { name: 'Previous' })).toBeInTheDocument()
  })

  it('stays on the translations when the refused text version is a translation', async () => {
    await refuseText("translation 'fr': invalid email template: text version: Liquid syntax error")
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
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
