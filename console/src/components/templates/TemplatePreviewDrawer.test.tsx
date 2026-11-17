import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import TemplatePreviewDrawer from './TemplatePreviewDrawer'
import { templatesApi } from '../../services/api/template'
import type { Template, Workspace } from '../../services/api/types'
import type { MessageHistory } from '../../services/api/messages_history'

vi.mock('../../services/api/template', () => ({
  templatesApi: {
    // The compile call is irrelevant here: the Template Data and Metadata tabs are built
    // from props, and the tab bar renders whether or not a preview compiled.
    compile: vi.fn().mockRejectedValue(new Error('no preview in tests'))
  }
}))

const template = {
  id: 'tpl-1',
  name: 'Order Confirmation',
  version: 1,
  category: 'transactional',
  email: {}
} as unknown as Template

const workspace = {
  id: 'ws-1',
  name: 'Test Workspace',
  settings: { timezone: 'UTC' },
  integrations: []
} as unknown as Workspace

const message = (metadata?: Record<string, unknown>): MessageHistory =>
  ({
    id: 'msg-1',
    contact_email: 'bob@example.com',
    template_id: 'tpl-1',
    template_version: 1,
    channel: 'email',
    message_data: { data: {}, ...(metadata === undefined ? {} : { metadata }) },
    sent_at: '2026-08-01T10:00:00Z',
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z'
  }) as MessageHistory


// The language menu of the header: opens it, then picks a language
const pickLanguage = async (name: string) => {
  fireEvent.mouseDown(await screen.findByRole('combobox', { name: 'Language' }))
  fireEvent.click(await screen.findByTitle(name))
}

const openDrawer = async (messageHistory?: MessageHistory, record: Template = template) => {
  render(
    <ConfigProvider>
      <App>
        <TemplatePreviewDrawer record={record} workspace={workspace} messageHistory={messageHistory}>
          <button>open</button>
        </TemplatePreviewDrawer>
      </App>
    </ConfigProvider>
  )

  fireEvent.click(screen.getByText('open'))
  await waitFor(() => expect(screen.getByText('Template Data')).toBeInTheDocument())
}

describe('TemplatePreviewDrawer metadata tab', () => {
  it('adds a Metadata tab when the message carries metadata', async () => {
    await openDrawer(message({ order_id: '9912' }))

    expect(screen.getByText('Metadata')).toBeInTheDocument()
  })

  it('omits the tab when metadata is an empty object', async () => {
    await openDrawer(message({}))

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
  })

  it('omits the tab when the message has no metadata', async () => {
    // The broadcast and automation shape.
    await openDrawer(message())

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
  })

  it('omits the tab when previewing a template with no message at all', async () => {
    // The drawer is also opened straight from the templates list, with no send behind it.
    await openDrawer()

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
  })
})

describe('TemplatePreviewDrawer text version tab', () => {
  const codeTemplate = {
    ...template,
    email: { editor_mode: 'code', mjml_source: '<mjml></mjml>' }
  } as unknown as Template
  const viewLabels = () =>
    Array.from(document.querySelectorAll('.ant-drawer .ant-segmented-item-label')).map((view) => view.textContent)

  it('shows the views in the mockup order: HTML, MJML, text, data', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({
      html: '<p>Hello</p>',
      mjml: '<mjml></mjml>',
      text: 'Hello Bob\n\nTrack (https://example.com/t)\n'
    })
    await openDrawer(undefined, codeTemplate)
    await waitFor(() => expect(screen.getByText('Text version')).toBeInTheDocument())

    expect(viewLabels()).toEqual(['HTML Preview', 'MJML Source', 'Text version', 'Template Data'])
    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() =>
      expect(screen.getByTestId('preview-text-version').textContent).toBe(
        'Hello Bob\n\nTrack (https://example.com/t)\n'
      )
    )
  })

  it('says the email goes out as HTML only when its text is empty', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({ html: '<img src="x.gif">', mjml: '<mjml></mjml>', text: '' })
    await openDrawer(undefined, codeTemplate)
    await waitFor(() => expect(screen.getByText('Text version')).toBeInTheDocument())

    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() =>
      expect(screen.getByText('No text version: this email will be sent as HTML only.')).toBeInTheDocument()
    )
  })

  it('has no text version tab when the compile returns no text', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({ html: '<p>Hello</p>', mjml: '<mjml></mjml>' })
    await openDrawer(undefined, codeTemplate)
    await waitFor(() => expect(screen.getByText('HTML Preview')).toBeInTheDocument())

    expect(screen.queryByText('Text version')).not.toBeInTheDocument()
  })
})

describe('TemplatePreviewDrawer custom text version', () => {
  const withText = (text?: string, translations?: Template['translations']) =>
    ({
      ...template,
      email: { editor_mode: 'code', mjml_source: '<mjml></mjml>', ...(text === undefined ? {} : { text }) },
      ...(translations ? { translations } : {})
    }) as unknown as Template
  const lastRequest = () => vi.mocked(templatesApi.compile).mock.calls.at(-1)![0]

  it('previews the custom text of the template and says it is custom', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({
      html: '<p>Hello</p>',
      mjml: '<mjml></mjml>',
      text: 'Hi Bob',
      text_source: 'custom'
    })
    await openDrawer(undefined, withText('Hi {{ contact.first_name }}'))
    await waitFor(() => expect(screen.getByText('Text version')).toBeInTheDocument())

    expect(lastRequest().text).toBe('Hi {{ contact.first_name }}')
    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() => expect(screen.getByTestId('preview-text-version').textContent).toBe('Hi Bob'))
    expect(screen.getByText('Custom')).toBeInTheDocument()
    expect(screen.getByText(/Written by hand for this template/)).toBeInTheDocument()
  })

  it('asks for the automatic version when the template has no custom text', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({
      html: '<p>Hello</p>',
      mjml: '<mjml></mjml>',
      text: 'Hello\n',
      text_source: 'auto'
    })
    await openDrawer(undefined, withText(undefined))
    await waitFor(() => expect(screen.getByText('Text version')).toBeInTheDocument())

    expect(lastRequest()).not.toHaveProperty('text')
    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() => expect(screen.getByText('Automatic')).toBeInTheDocument())
    expect(screen.queryByText('Custom')).not.toBeInTheDocument()
  })

  it('says why the custom text was replaced by the automatic version', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({
      html: '<p>Hello</p>',
      mjml: '<mjml></mjml>',
      text: 'Hello\n',
      text_source: 'auto',
      text_error: 'Liquid syntax error: unclosed tag'
    })
    await openDrawer(undefined, withText('{% if %}'))
    await waitFor(() => expect(screen.getByText('Text version')).toBeInTheDocument())

    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() =>
      expect(
        screen.getByText(/The custom text could not be used, the automatic version is sent instead: Liquid syntax error: unclosed tag/)
      ).toBeInTheDocument()
    )
  })

  const renderMultilingual = (record: Template) => {
    render(
      <ConfigProvider>
        <App>
          <TemplatePreviewDrawer
            record={record}
            workspace={{ ...workspace, settings: { timezone: 'UTC', default_language: 'en' } } as unknown as Workspace}
          >
            <button>open</button>
          </TemplatePreviewDrawer>
        </App>
      </ConfigProvider>
    )
    fireEvent.click(screen.getByText('open'))
  }
  const french = (text?: string) =>
    ({
      fr: { email: { editor_mode: 'code', mjml_source: '<mjml></mjml>', subject: 'Bonjour', ...(text ? { text } : {}) } },
      de: { email: { editor_mode: 'code', mjml_source: '<mjml></mjml>', subject: 'Hallo' } }
    }) as unknown as Template['translations']

  it('previews a translation with its own text', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValue({ html: '<p>Hello</p>', mjml: '<mjml></mjml>', text: 'x' })
    renderMultilingual(withText('Default text', french('Salut {{ contact.first_name }}')))
    await waitFor(() => expect(lastRequest().text).toBe('Default text'))

    await pickLanguage('French')
    await waitFor(() => expect(lastRequest().text).toBe('Salut {{ contact.first_name }}'))
    vi.mocked(templatesApi.compile).mockReset()
    vi.mocked(templatesApi.compile).mockRejectedValue(new Error('no preview in tests'))
  })

  it('opens on the language it is given, and compiles that translation', async () => {
    // The template selector's Full preview opened in English after Spanish was picked there.
    vi.mocked(templatesApi.compile).mockResolvedValue({ html: '<p>Hello</p>', mjml: '<mjml></mjml>', text: 'x' })
    render(
      <ConfigProvider>
        <App>
          <TemplatePreviewDrawer
            record={withText('Default text', french('Salut'))}
            workspace={{ ...workspace, settings: { timezone: 'UTC', default_language: 'en' } } as unknown as Workspace}
            initialLanguage="fr"
            open
          />
        </App>
      </ConfigProvider>
    )
    await waitFor(() => expect(lastRequest().subject).toBe('Bonjour'))
    expect(vi.mocked(templatesApi.compile).mock.calls.some(([request]) => request.text === 'Default text')).toBe(false)
    vi.mocked(templatesApi.compile).mockReset()
    vi.mocked(templatesApi.compile).mockRejectedValue(new Error('no preview in tests'))
  })

  it('previews a translation without a text with the automatic version, never the default text', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValue({ html: '<p>Hello</p>', mjml: '<mjml></mjml>', text: 'x' })
    renderMultilingual(withText('Default text', french('Salut')))
    await waitFor(() => expect(lastRequest().text).toBe('Default text'))

    await pickLanguage('German')
    await waitFor(() => expect(lastRequest().subject).toBe('Hallo'))
    expect(lastRequest()).not.toHaveProperty('text')
    vi.mocked(templatesApi.compile).mockReset()
    vi.mocked(templatesApi.compile).mockRejectedValue(new Error('no preview in tests'))
  })
})

describe('TemplatePreviewDrawer preview errors', () => {
  it('says the template data is missing when the editor mode has no content, without compiling', async () => {
    vi.mocked(templatesApi.compile).mockClear()
    await openDrawer(undefined, { ...template, email: { editor_mode: 'code' } } as unknown as Template)

    expect(await screen.findByText('Missing workspace ID or template data.')).toBeInTheDocument()
    expect(templatesApi.compile).not.toHaveBeenCalled()
  })

  it('says the structure is invalid when the tree does not parse, without compiling', async () => {
    vi.mocked(templatesApi.compile).mockClear()
    await openDrawer(undefined, { ...template, email: { visual_editor_tree: '{not json' } } as unknown as Template)

    expect(await screen.findByText('Invalid template structure data.')).toBeInTheDocument()
    expect(templatesApi.compile).not.toHaveBeenCalled()
  })
})

// The templates page opens the preview from its row menu and from the URL.
describe('TemplatePreviewDrawer controlled by the page', () => {
  const renderControlled = (open: boolean, onClose = vi.fn()) =>
    render(
      <ConfigProvider>
        <App>
          <TemplatePreviewDrawer record={template} workspace={workspace} open={open} onClose={onClose} />
        </App>
      </ConfigProvider>
    )

  it('opens without children when told to', async () => {
    renderControlled(true)
    expect(await screen.findByText('Template Data')).toBeInTheDocument()
  })

  it('stays closed while open is false', () => {
    renderControlled(false)
    expect(screen.queryByText('Template Data')).not.toBeInTheDocument()
  })

  it('asks the page to close it', async () => {
    const onClose = vi.fn()
    renderControlled(true, onClose)
    await screen.findByText('Template Data')
    // The cross on the right of the header, as in the mockup
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

describe('TemplatePreviewDrawer header and envelope', () => {
  it('names the version and the category after the name, a category it does not know as it is', async () => {
    await openDrawer()
    expect(screen.getByText('v1 · Transactional')).toBeInTheDocument()
    cleanup()
    await openDrawer(undefined, { ...template, version: 4, category: 'custom-kind' } as unknown as Template)
    expect(screen.getByText('v4 · custom-kind')).toBeInTheDocument()
  })

  it('always shows Reply to, Not set without one, and no Bcc row for a template alone', async () => {
    await openDrawer()
    expect(screen.getByText('Reply to')).toBeInTheDocument()
    expect(screen.getByText('Not set')).toBeInTheDocument()
    expect(screen.queryByText('BCC')).not.toBeInTheDocument()
  })

  it('shows the reply-to of a message and the template one it replaced, and its cc and bcc', async () => {
    const sent = {
      ...message(),
      channel_options: { reply_to: 'help@acme.io', cc: ['cc@acme.io'], bcc: ['bcc@acme.io'] }
    } as unknown as MessageHistory
    await openDrawer(sent, { ...template, email: { reply_to: 'team@acme.io' } } as unknown as Template)
    expect(screen.getByText('help@acme.io')).toBeInTheDocument()
    expect(screen.getByText('(original: team@acme.io)')).toBeInTheDocument()
    expect(screen.getByText('cc@acme.io')).toBeInTheDocument()
    expect(screen.getByText('bcc@acme.io')).toBeInTheDocument()
  })

  it('keeps every view mounted, so going back to HTML does not reload it', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({ html: '<p>Hello</p>', mjml: '<mjml></mjml>' })
    await openDrawer(undefined, {
      ...template,
      email: { editor_mode: 'code', mjml_source: '<mjml></mjml>' }
    } as unknown as Template)
    await waitFor(() => expect(screen.getByTitle(/HTML Preview of/)).toBeInTheDocument())
    const iframe = screen.getByTitle(/HTML Preview of/)

    fireEvent.click(screen.getByText('MJML Source'))
    expect(iframe).toBeInTheDocument()
    expect(iframe.closest('[hidden]')).not.toBeNull()
    fireEvent.click(screen.getByText('HTML Preview'))
    expect(screen.getByTitle(/HTML Preview of/)).toBe(iframe)
  })

  it('shows the MJML view when the compile fails, as there is no HTML to show', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({
      html: '',
      mjml: '<mjml><mj-bad /></mjml>',
      error: { message: 'bad tag', details: [] }
    } as never)
    await openDrawer(undefined, {
      ...template,
      email: { editor_mode: 'code', mjml_source: '<mjml></mjml>' }
    } as unknown as Template)
    await waitFor(() => expect(screen.getByText('MJML Source')).toBeInTheDocument())
    const selected = document.querySelector('.ant-drawer .ant-segmented-item-selected')
    expect(selected?.textContent).toBe('MJML Source')
  })
})
