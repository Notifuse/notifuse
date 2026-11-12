import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { EditorSettings } from './EditorSettings'
import { NodePalette } from './NodePalette'

const ctx = vi.hoisted(() => ({
  value: {} as Record<string, unknown>
}))
vi.mock('./context', () => ({ useAutomation: () => ctx.value }))

const setFrequency = vi.fn()
const setListId = vi.fn()

const renderSettings = () =>
  render(
    <I18nProvider i18n={i18n}>
      <EditorSettings />
    </I18nProvider>
  )

describe('EditorSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ctx.value = {
      lists: [{ id: 'news', name: 'Newsletter' }],
      listId: 'news',
      setListId,
      frequency: 'once',
      setFrequency,
      exitOnReply: false,
      setExitOnReply: vi.fn(),
      workspace: { id: 'ws', integrations: [] }
    }
  })

  it('writes the frequency through the context', () => {
    renderSettings()
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Frequency' }))
    fireEvent.click(screen.getByTitle('Every time'))
    expect(setFrequency).toHaveBeenCalledWith('every_time', expect.anything())
  })

  it('locks exit on reply and explains it without an inbound provider', async () => {
    renderSettings()
    expect(screen.getByRole('switch', { name: 'Exit on reply' })).toBeDisabled()
    fireEvent.mouseEnter(screen.getByLabelText('About exit on reply'))
    expect(await screen.findByText(/Available once you connect an email provider/)).toBeInTheDocument()
  })

  it('offers exit on reply with an inbound provider', async () => {
    ctx.value = {
      ...ctx.value,
      workspace: { id: 'ws', integrations: [{ email_provider: { kind: 'mailgun' } }] }
    }
    renderSettings()
    expect(screen.getByRole('switch', { name: 'Exit on reply' })).not.toBeDisabled()
    fireEvent.mouseEnter(screen.getByLabelText('About exit on reply'))
    expect(await screen.findByText(/requires inbound reply forwarding/)).toBeInTheDocument()
  })
})

describe('NodePalette', () => {
  const renderPalette = (hasListSelected: boolean) =>
    render(
      <I18nProvider i18n={i18n}>
        <NodePalette hasListSelected={hasListSelected} />
      </I18nProvider>
    )

  it('shows Actions then Logic, each in its order', () => {
    renderPalette(true)
    const items = (name: string) =>
      Array.from(within(screen.getByRole('region', { name })).getAllByText(/./, { selector: '[data-palette-item] span:last-child' })).map(
        (el) => el.textContent
      )
    expect(items('Actions')).toEqual(['Email', 'Add to list', 'Remove from list', 'Webhook'])
    expect(items('Logic')).toEqual(['Delay', 'Filter', 'List status', 'A/B test'])
  })

  it('disables email without a list, with its reason', async () => {
    renderPalette(false)
    const email = document.querySelector('[data-palette-item="email"]')!
    expect(email.getAttribute('draggable')).toBe('false')
    expect(email.getAttribute('aria-disabled')).toBe('true')
    fireEvent.mouseEnter(email)
    expect(await screen.findByText('Select a list to enable email nodes')).toBeInTheDocument()
  })
})
