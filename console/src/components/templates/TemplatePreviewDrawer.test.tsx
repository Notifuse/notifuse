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
