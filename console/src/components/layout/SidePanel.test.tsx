import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { SidePanel, type SidePanelProps } from './SidePanel'
import * as clipboard from '../../lib/copyToClipboard'

function renderPanel(props: Partial<SidePanelProps> = {}) {
  const onClose = vi.fn()
  const utils = render(
    <App>
      <SidePanel open onClose={onClose} title="jane@example.com" {...props}>
        <p>Panel body</p>
      </SidePanel>
    </App>
  )
  return { onClose, ...utils }
}

describe('SidePanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('is 520px wide, with no antd header and an unblurred mask', () => {
    // A blur set console-wide must not reach the panel: the list behind stays readable.
    render(
      <ConfigProvider drawer={{ mask: { blur: true } }}>
        <App>
          <SidePanel open onClose={vi.fn()} title="jane@example.com">
            <p>Panel body</p>
          </SidePanel>
        </App>
      </ConfigProvider>
    )
    expect(document.querySelector('.ant-drawer-mask')).not.toBeNull()
    expect(document.querySelector<HTMLElement>('.ant-drawer-content-wrapper')!.style.width).toBe('520px')
    expect(document.querySelector('.ant-drawer-header')).toBeNull()
    expect(document.querySelector('.ant-drawer-mask-blur')).toBeNull()
    // data-unreset: antd's reset margin would push the title off the centre of its 40px header.
    expect(screen.getByRole('heading', { level: 2, name: 'jane@example.com' })).toHaveAttribute('data-unreset')
    expect(screen.getByText('Panel body')).toBeInTheDocument()
  })

  it('renders the status, the meta line, the actions and the footer', () => {
    renderPanel({
      status: <span>Active</span>,
      meta: ['Created today', 'France'],
      actions: <button type="button">Edit</button>,
      footer: <button type="button">Delete</button>
    })
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('France')).toBeInTheDocument()
    expect(document.querySelectorAll('[data-meta-separator]')).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
    expect(document.querySelector('[data-sidepanel-footer]')).toHaveTextContent('Delete')
  })

  it('closes from the close button', () => {
    const { onClose } = renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('renders the tabs and shows the active tab in the body', () => {
    const onChange = vi.fn()
    renderPanel({
      tabs: {
        items: [
          { key: 'profile', label: 'Profile', children: <p>Profile body</p> },
          { key: 'timeline', label: 'Timeline', children: <p>Timeline body</p> }
        ],
        onChange
      }
    })
    expect(screen.getByRole('tab', { name: 'Profile' })).toBeInTheDocument()
    // In line with the title and the body: the padding is on a wrapper, where antd cannot override it.
    expect(document.querySelector('.ant-tabs')!.parentElement!.className).toContain('px-4')
    expect(screen.getByText('Profile body')).toBeInTheDocument()
    expect(screen.queryByText('Panel body')).toBeNull()
    fireEvent.click(screen.getByRole('tab', { name: 'Timeline' }))
    expect(onChange).toHaveBeenCalledWith('timeline')
    expect(screen.getByText('Timeline body')).toBeInTheDocument()
    expect(screen.queryByText('Profile body')).toBeNull()
  })

  it('disables previous and next without their handler, and answers J and K', () => {
    const onPrev = vi.fn()
    renderPanel({ nav: { onPrev } })
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Previous' }))
    fireEvent.keyDown(document.body, { key: 'k' })
    expect(onPrev).toHaveBeenCalledTimes(2)
  })

  it('ignores J and K typed in a field of the panel', () => {
    const onNext = vi.fn()
    render(
      <App>
        <SidePanel open onClose={vi.fn()} title="Contact" nav={{ onNext }}>
          <input aria-label="note" />
        </SidePanel>
      </App>
    )
    fireEvent.keyDown(screen.getByLabelText('note'), { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('copies the page URL', async () => {
    const copy = vi.spyOn(clipboard, 'copyToClipboard').mockResolvedValue(true)
    renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    expect(copy).toHaveBeenCalledWith(window.location.href)
    expect(await screen.findByText('Link copied')).toBeInTheDocument()
  })
})
