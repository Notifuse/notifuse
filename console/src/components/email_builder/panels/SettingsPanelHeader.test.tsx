import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { App } from 'antd'
import type { EmailBlock } from '../types'
import { SettingsPanelHeader } from './SettingsPanelHeader'

const b = (id: string, type: string, children?: EmailBlock[]) =>
  ({ id, type, attributes: {}, ...(children ? { children } : {}) }) as unknown as EmailBlock

const button = b('button', 'mj-button')
const font = b('font', 'mj-font')
const tree = b('root', 'mjml', [
  b('head', 'mj-head', [b('attrs', 'mj-attributes'), font]),
  b('body', 'mj-body', [b('section', 'mj-section', [b('column', 'mj-column', [button])])])
])

const renderHeader = (block: EmailBlock) => {
  const handlers = { onSelectBlock: vi.fn(), onCloneBlock: vi.fn(), onDeleteBlock: vi.fn(), onSaveBlock: vi.fn() }
  render(
    <App>
      <SettingsPanelHeader block={block} emailTree={tree} {...handlers} />
    </App>
  )
  return handlers
}

describe('SettingsPanelHeader', () => {
  it("shows the block's name and its path, the ancestors as buttons that select them", () => {
    const { onSelectBlock } = renderHeader(button)
    const header = screen.getByTestId('settings-panel-header')
    // The darker rule of the panel, not the soft one between rows
    expect(header).toHaveClass('border-line')
    expect(within(header).getAllByText('Button')[0]).toBeInTheDocument()
    const path = screen.getByRole('navigation', { name: 'Block path' })
    expect(path).toHaveTextContent('Body›Section›Column›Button')
    expect(within(path).getByText('Button', { selector: '[aria-current="location"]' })).toBeInTheDocument()
    fireEvent.click(within(path).getByRole('button', { name: 'Column' }))
    expect(onSelectBlock).toHaveBeenCalledWith('column')
  })

  it('clones and opens the save dialog for the block', () => {
    const { onCloneBlock, onSaveBlock } = renderHeader(button)
    fireEvent.click(screen.getByRole('button', { name: 'Clone Block' }))
    expect(onCloneBlock).toHaveBeenCalledWith('button')
    fireEvent.click(screen.getByRole('button', { name: 'Save to library' }))
    expect(onSaveBlock).toHaveBeenCalledWith('button')
  })

  it('deletes the block once confirmed', async () => {
    const { onDeleteBlock } = renderHeader(button)
    fireEvent.click(screen.getByRole('button', { name: 'Delete Block' }))
    expect(onDeleteBlock).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'OK' }))
    expect(onDeleteBlock).toHaveBeenCalledWith('button')
  })

  it('offers only delete on a head block', () => {
    renderHeader(font)
    expect(screen.queryByRole('button', { name: 'Clone Block' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Save to library' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Delete Block' })).toBeInTheDocument()
  })

  it('offers no action on the body', () => {
    renderHeader(tree.children![1])
    expect(screen.queryByRole('button', { name: /Clone Block|Save to library|Delete Block/ })).toBeNull()
    // The body is its own path: nothing to show under its name
    expect(screen.queryByRole('navigation', { name: 'Block path' })).toBeNull()
  })
})
