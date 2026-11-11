import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { BlockToolbar } from './BlockToolbar'

const icon = (name: string) =>
  document.querySelector(`svg[data-icon="${name}"]`)!.closest('button')!

function renderToolbar() {
  const handlers = { onClone: vi.fn(), onDelete: vi.fn(), onSave: vi.fn() }
  render(
    <App>
      <BlockToolbar blockId="products" {...handlers} />
    </App>
  )
  return handlers
}

describe('BlockToolbar', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  // The dialog belongs to the editor (SaveBlockModal), which can also open it from the library
  it('asks the editor to open the save dialog for its block, and opens none of its own', () => {
    const { onSave } = renderToolbar()
    fireEvent.click(icon('floppy-disk'))

    expect(onSave).toHaveBeenCalledWith('products')
    expect(document.querySelector('.ant-modal')).toBeNull()
  })

  it('shows "Save to library" on the save button', async () => {
    renderToolbar()
    await userEvent.hover(icon('floppy-disk'))
    expect(await screen.findByText('Save to library')).toBeInTheDocument()
  })

  it('duplicates its block', () => {
    const { onClone } = renderToolbar()
    fireEvent.click(icon('clone'))
    expect(onClone).toHaveBeenCalledWith('products')
  })

  it('deletes its block once confirmed', async () => {
    const { onDelete } = renderToolbar()
    fireEvent.click(icon('trash-can'))
    expect(onDelete).not.toHaveBeenCalled()

    await userEvent.click(await screen.findByRole('button', { name: /ok|yes/i }))
    expect(onDelete).toHaveBeenCalledWith('products')
  })
})
