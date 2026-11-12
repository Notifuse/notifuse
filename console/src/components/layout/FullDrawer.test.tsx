import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { FullDrawer, type FullDrawerProps } from './FullDrawer'
import * as clipboard from '../../lib/copyToClipboard'

function renderDrawer(props: Partial<FullDrawerProps> = {}) {
  const onClose = vi.fn()
  const utils = render(
    <App>
      <FullDrawer open onClose={onClose} backLabel="Broadcasts" {...props}>
        <p>Drawer body</p>
      </FullDrawer>
    </App>
  )
  return { onClose, ...utils }
}

const drawerRoot = () => document.querySelector<HTMLElement>('.ant-drawer')!

describe('FullDrawer', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens full width with no antd header, its root class and its content padding', () => {
    renderDrawer()
    expect(screen.getByText('Drawer body')).toBeInTheDocument()
    expect(drawerRoot().className).toContain('app-full-drawer')
    expect(document.querySelector<HTMLElement>('.ant-drawer-content-wrapper')!.style.width).toBe('100%')
    expect(document.querySelector('.ant-drawer-header')).toBeNull()
    expect(document.querySelector('.ant-drawer-close')).toBeNull()
    const content = document.querySelector('[data-drawer-content]')!
    expect(content.className).toContain('px-5')
    expect(content.className).toContain('gap-3')
  })

  it('closes from the back button and from the close button', () => {
    const { onClose } = renderDrawer()
    fireEvent.click(screen.getByRole('button', { name: /Broadcasts/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('labels the back button "Back" without a backLabel', () => {
    renderDrawer({ backLabel: undefined })
    expect(screen.getByRole('button', { name: /Back/ })).toBeInTheDocument()
  })

  it('disables previous and next without their handler', () => {
    const onNext = vi.fn()
    renderDrawer({ nav: { onNext } })
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    const next = screen.getByRole('button', { name: 'Next' })
    expect(next).toBeEnabled()
    fireEvent.click(next)
    expect(onNext).toHaveBeenCalledTimes(1)
  })

  it('shows the position only with both position and total, then the context', () => {
    const { unmount } = renderDrawer({ nav: { position: 4, total: 12, context: 'Pro plan' } })
    expect(screen.getByText('4 of 12')).toBeInTheDocument()
    expect(screen.getByText('Pro plan')).toBeInTheDocument()
    unmount()
    renderDrawer({ nav: { position: 4, context: 'Pro plan' } })
    expect(document.querySelector('[data-drawer-position]')).toBeNull()
    expect(screen.getByText('Pro plan')).toBeInTheDocument()
  })

  it('draws no previous or next without nav', () => {
    renderDrawer()
    expect(screen.queryByRole('button', { name: 'Previous' })).toBeNull()
  })

  it('goes to the next item on J and the previous on K', () => {
    const onNext = vi.fn()
    const onPrev = vi.fn()
    renderDrawer({ nav: { onNext, onPrev } })
    fireEvent.keyDown(document.body, { key: 'j' })
    fireEvent.keyDown(document.body, { key: 'k' })
    expect(onNext).toHaveBeenCalledTimes(1)
    expect(onPrev).toHaveBeenCalledTimes(1)
  })

  it('ignores J while closed', () => {
    const onNext = vi.fn()
    renderDrawer({ open: false, nav: { onNext } })
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('copies the page URL and says so', async () => {
    const copy = vi.spyOn(clipboard, 'copyToClipboard').mockResolvedValue(true)
    renderDrawer()
    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    expect(copy).toHaveBeenCalledWith(window.location.href)
    expect(await screen.findByText('Link copied')).toBeInTheDocument()
  })

  it('says when the copy fails', async () => {
    vi.spyOn(clipboard, 'copyToClipboard').mockResolvedValue(false)
    renderDrawer()
    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    await waitFor(() => expect(screen.getByText('Failed to copy to clipboard')).toBeInTheDocument())
  })

  it('has no copy button with copyLink false, and renders the top actions', () => {
    renderDrawer({ copyLink: false, topActions: <button type="button">Edit</button> })
    expect(screen.queryByRole('button', { name: 'Copy link' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  })
})
