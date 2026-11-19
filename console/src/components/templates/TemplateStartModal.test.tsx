import '../../__tests__/resizeObserverMock'
import { describe, expect, it, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import { TemplateStartModal, type TemplateStartModalProps } from './TemplateStartModal'

// The API client imports the router, and the router every page.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))
// The previews compile through the server: the frames are not under test here.
vi.mock('../broadcasts/detail/TemplatePreviewFrame', () => ({
  TemplatePreviewFrame: ({ template }: { template: Template }) => <div data-testid="frame" data-template={template.id} />
}))
vi.mock('../../services/api/template', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/template')>()),
  templatesApi: { list: vi.fn() }
}))

const workspace = { id: 'ws1', settings: { default_language: 'fr', languages: ['fr', 'en'] } } as unknown as Workspace
const assets = { baseUrl: 'https://api.example.com/console/gallery' }

function renderModal(props: Partial<TemplateStartModalProps> = {}) {
  const handlers = { onCancel: vi.fn(), onPickModel: vi.fn(), onPickTemplate: vi.fn() }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AntApp>
        <TemplateStartModal open workspace={workspace} assets={assets} defaultTestData={{}} canReadTemplates {...handlers} {...props} />
      </AntApp>
    </QueryClientProvider>
  )
  return handlers
}

const card = (id: string) => document.querySelector<HTMLElement>(`[data-model="${id}"]`)!
const cards = () => [...document.querySelectorAll<HTMLElement>('[data-model]')].map((el) => el.dataset.model)

describe('TemplateStartModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(templatesApi.list as Mock).mockResolvedValue({
      templates: [
        { id: 'old', name: 'Old newsletter', category: 'marketing', updated_at: '2026-01-01T00:00:00Z', email: { subject: 'Old' } },
        { id: 'pwd', name: 'Password reset', category: 'transactional', updated_at: '2026-03-01T00:00:00Z', email: { subject: 'Reset' } }
      ]
    })
  })

  it('opens on the gallery, Blank first, with the use cases and their counts', () => {
    renderModal()
    expect(cards()[0]).toBe('blank')
    // Named for what it holds (a logo, a paragraph, the footer): it is not empty.
    const first = document.querySelector<HTMLElement>('[data-model="blank"]')!
    expect(within(first).getByText('Simple')).toBeInTheDocument()
    expect(within(first).queryByText('Blank')).not.toBeInTheDocument()
    expect(cards()).toHaveLength(16)
    const nav = screen.getByRole('navigation', { name: 'Use cases' })
    expect(within(nav).getByRole('button', { name: /All\s*15/ })).toHaveAttribute('aria-pressed', 'true')
    expect(within(nav).getByRole('button', { name: /E-commerce\s*4/ })).toBeInTheDocument()
  })

  it('filters by use case and by search, Blank staying', async () => {
    renderModal()
    await userEvent.click(screen.getByRole('button', { name: /Billing/ }))
    expect(cards()).toEqual(['blank', 'receipt'])

    await userEvent.click(screen.getByRole('button', { name: /^All/ }))
    await userEvent.type(screen.getByRole('textbox', { name: 'Search a model' }), 'cart')
    expect(cards()).toEqual(['blank', 'cart'])
  })

  it('keeps only the models of a category imposed by where the template is created', () => {
    renderModal({ forceCategory: 'transactional' })
    expect(screen.queryByRole('navigation', { name: 'Use cases' })).toBeNull()
    expect(screen.getByText(/is imposed where this template is created/)).toBeInTheDocument()
    expect(cards()).toEqual(['blank', 'verify', 'reset', 'invite', 'receipt', 'order', 'shipping'])
  })

  it('starts from Blank at once', async () => {
    const { onPickModel } = renderModal()
    await userEvent.click(card('blank'))
    expect(onPickModel).toHaveBeenCalledWith(expect.objectContaining({ modelId: 'blank', language: 'fr' }))
  })

  it('previews a model, then uses it in the workspace language with its translations', async () => {
    const { onPickModel } = renderModal()
    await userEvent.click(card('order'))

    expect(await screen.findByText('The summary sent right after a purchase: items, totals, address.')).toBeInTheDocument()
    expect(screen.getByText('order.items')).toBeInTheDocument()
    expect(screen.queryByText('contact.first_name')).toBeNull()
    expect(screen.getByTestId('frame')).toHaveAttribute('data-template', 'gallery-order-fr-logo')

    await userEvent.click(screen.getByRole('button', { name: 'Use this model' }))
    const start = onPickModel.mock.calls[0][0]
    expect(start).toMatchObject({ modelId: 'order', category: 'transactional', language: 'fr' })
    expect(Object.keys(start.translations)).toEqual(['en'])
  })

  it('shows Blank under the imposed category, not a category to choose', () => {
    renderModal({ forceCategory: 'transactional' })
    expect(within(card('blank')).getByText('Transactional')).toBeInTheDocument()
    expect(screen.queryByText('Category to choose')).toBeNull()
  })

  it('goes back to the gallery with its filter kept', async () => {
    renderModal()
    await userEvent.click(screen.getByRole('button', { name: /E-commerce/ }))
    await userEvent.click(card('promo'))
    await userEvent.click(await screen.findByRole('button', { name: 'Gallery' }))
    expect(cards()).toEqual(['blank', 'order', 'shipping', 'cart', 'promo'])
  })

  it('uses a model on a double click', async () => {
    const { onPickModel } = renderModal()
    await userEvent.dblClick(card('newsletter'))
    expect(onPickModel).toHaveBeenCalledWith(expect.objectContaining({ modelId: 'newsletter' }))
    expect(onPickModel).toHaveBeenCalledTimes(1)
    // The double click cancelled the preview its first click would have opened.
    await new Promise((resolve) => setTimeout(resolve, 300))
    expect(screen.queryByRole('button', { name: 'Use this model' })).toBeNull()
  })

  it('lists the workspace templates, latest first, and starts from one', async () => {
    const { onPickTemplate } = renderModal()
    await userEvent.click(screen.getByText('Your templates'))
    await waitFor(() => expect(screen.getByText('Password reset')).toBeInTheDocument())
    const names = screen.getAllByText(/Old newsletter|Password reset/).map((el) => el.textContent)
    expect(names).toEqual(['Password reset', 'Old newsletter'])

    await userEvent.click(screen.getAllByRole('button', { name: 'Start from this template' })[1])
    expect(onPickTemplate).toHaveBeenCalledWith(expect.objectContaining({ id: 'old' }))
  })

  it('hides Your templates without the right to read templates', () => {
    renderModal({ canReadTemplates: false })
    expect(screen.queryByText('Your templates')).toBeNull()
    expect(templatesApi.list).not.toHaveBeenCalled()
  })

  it('waits for the workspace logo to load before building a pick, so it gets its own width', async () => {
    // Built at once, a pick made before the logo loads kept a square's 40px width.
    let release: (() => void) | undefined
    class SlowImage {
      naturalWidth = 300
      naturalHeight = 100
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      set src(_value: string) {
        release = () => this.onload?.()
      }
    }
    const original = globalThis.Image
    globalThis.Image = SlowImage as unknown as typeof Image
    try {
      const withLogo = {
        ...workspace,
        name: 'Acme',
        settings: { ...workspace.settings, logo_url: 'https://cdn.example.com/slow-logo.png' }
      } as unknown as Workspace
      const { onPickModel } = renderModal({ workspace: withLogo })
      await userEvent.click(card('blank'))
      expect(onPickModel).not.toHaveBeenCalled()
      release?.()
      await waitFor(() => expect(onPickModel).toHaveBeenCalledTimes(1))
      expect(JSON.stringify(onPickModel.mock.calls[0][0].tree)).toContain('"width":"120px"')
    } finally {
      globalThis.Image = original
    }
  })
})
