import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
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
  const tabLabels = () => Array.from(document.querySelectorAll('.ant-tabs-tab')).map((tab) => tab.textContent)

  it('shows the text version between the HTML preview and the MJML source', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValueOnce({
      html: '<p>Hello</p>',
      mjml: '<mjml></mjml>',
      text: 'Hello Bob\n\nTrack (https://example.com/t)\n'
    })
    await openDrawer(undefined, codeTemplate)
    await waitFor(() => expect(screen.getByText('Text version')).toBeInTheDocument())

    expect(tabLabels().slice(0, 3)).toEqual(['HTML Preview', 'Text version', 'MJML Source'])
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

    fireEvent.click(await screen.findByText('French'))
    await waitFor(() => expect(lastRequest().text).toBe('Salut {{ contact.first_name }}'))
    vi.mocked(templatesApi.compile).mockReset()
    vi.mocked(templatesApi.compile).mockRejectedValue(new Error('no preview in tests'))
  })

  it('previews a translation without a text with the automatic version, never the default text', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValue({ html: '<p>Hello</p>', mjml: '<mjml></mjml>', text: 'x' })
    renderMultilingual(withText('Default text', french('Salut')))
    await waitFor(() => expect(lastRequest().text).toBe('Default text'))

    fireEvent.click(await screen.findByText('German'))
    await waitFor(() => expect(lastRequest().subject).toBe('Hallo'))
    expect(lastRequest()).not.toHaveProperty('text')
    vi.mocked(templatesApi.compile).mockReset()
    vi.mocked(templatesApi.compile).mockRejectedValue(new Error('no preview in tests'))
  })
})
