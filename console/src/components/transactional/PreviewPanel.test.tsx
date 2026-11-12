import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'

const { compile } = vi.hoisted(() => ({ compile: vi.fn() }))
vi.mock('../templates/compileTemplatePreview', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../templates/compileTemplatePreview')>()
  return { ...actual, compileTemplatePreview: compile }
})

import { PreviewPanel, type TemplateState } from './PreviewPanel'
import { TemplatePreviewError } from '../templates/compileTemplatePreview'

const workspace = { id: 'ws1', name: 'Acme', settings: {} } as unknown as Workspace

const template = (extra: Partial<Template> = {}): Template =>
  ({
    id: 'reset',
    name: 'Password reset',
    version: 3,
    channel: 'email',
    category: 'transactional',
    email: { subject: 'Reset {{ name }}', subject_preview: 'Your link', editor_mode: 'code', mjml_source: '<mjml/>' },
    test_data: { name: 'Ada' },
    ...extra
  }) as Template

const renderPanel = (state: TemplateState, onOpen = vi.fn()) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <PreviewPanel workspace={workspace} template={state} onOpen={onOpen} />
      </I18nProvider>
    </QueryClientProvider>
  )

/** Records the observers the panel creates, so a test can report a width. */
const observers: { callback: ResizeObserverCallback; element?: Element; disconnected: boolean }[] = []
class FakeResizeObserver {
  private readonly record: (typeof observers)[number]
  constructor(callback: ResizeObserverCallback) {
    this.record = { callback, disconnected: false }
    observers.push(this.record)
  }
  observe(element: Element) {
    this.record.element = element
  }
  disconnect() {
    this.record.disconnected = true
  }
  unobserve() {}
}

beforeEach(() => {
  compile.mockReset()
  observers.length = 0
  vi.stubGlobal('ResizeObserver', FakeResizeObserver)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PreviewPanel', () => {
  it('compiles the template with its test data and shows the rendered subject', async () => {
    compile.mockResolvedValue({ html: '<p>Hi Ada</p>', mjml: '', subject: 'Reset Ada', subject_preview: 'Your link' })
    const loaded = template()
    renderPanel({ status: 'ok', template: loaded })

    expect(await screen.findByText('Reset Ada')).toBeInTheDocument()
    expect(screen.getByText('Your link')).toBeInTheDocument()
    expect(compile).toHaveBeenCalledWith({ workspace, email: loaded.email, testData: { name: 'Ada' } })
    const frame = screen.getByTitle('Preview of Password reset')
    expect(frame).toHaveAttribute('srcdoc', '<p>Hi Ada</p>')
    // No script of the email runs in the console.
    expect(frame).toHaveAttribute('sandbox', '')
  })

  it('scales the email down to the width of the column, once the preview has compiled', async () => {
    compile.mockResolvedValue({ html: '<p>Hi</p>', mjml: '', subject: 'Reset' })
    renderPanel({ status: 'ok', template: template() })

    const frame = await screen.findByTitle('Preview of Password reset')
    const observer = observers.find((record) => record.element === frame.parentElement)
    expect(observer).toBeDefined()
    act(() => {
      observer?.callback([{ contentRect: { width: 300 } } as ResizeObserverEntry], {} as ResizeObserver)
    })
    expect(frame.style.transform).toBe('scale(0.5)')
    expect(frame.parentElement?.style.height).toBe('210px')
  })

  it('opens the preview drawer', async () => {
    compile.mockResolvedValue({ html: '', mjml: '' })
    const onOpen = vi.fn()
    renderPanel({ status: 'ok', template: template() }, onOpen)

    fireEvent.click(await screen.findByRole('button', { name: 'Open' }))
    expect(onOpen).toHaveBeenCalledTimes(1)
  })

  it('flags a deleted template the notification still sends with', async () => {
    compile.mockResolvedValue({ html: '', mjml: '' })
    renderPanel({ status: 'ok', template: template({ deleted_at: '2026-11-01T00:00:00Z' }) })

    expect(await screen.findByText('Deleted template')).toBeInTheDocument()
  })

  it('says when the template has no content yet', async () => {
    compile.mockRejectedValue(new TemplatePreviewError('missing_content'))
    renderPanel({ status: 'ok', template: template() })

    expect(await screen.findByText('This template has no content to preview yet')).toBeInTheDocument()
  })

  it('offers a retry when the compile fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    compile.mockResolvedValueOnce({ html: '', mjml: '', error: { message: 'bad mjml' } })
    compile.mockResolvedValueOnce({ html: '<p>ok</p>', mjml: '', subject: 'Fixed' })
    renderPanel({ status: 'ok', template: template() })

    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Fixed')).toBeInTheDocument()
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })

  it('says what blocks the preview without compiling', () => {
    const { unmount } = renderPanel({ status: 'forbidden' })
    expect(screen.getByText('You need access to templates')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open' })).toBeDisabled()
    unmount()

    renderPanel({ status: 'notFound' })
    expect(screen.getByText('Template not found')).toBeInTheDocument()
    expect(compile).not.toHaveBeenCalled()
  })

  it('retries loading the template', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const retry = vi.fn()
    renderPanel({ status: 'error', error: new Error('boom'), retry })

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledTimes(1)
    warn.mockRestore()
  })
})
