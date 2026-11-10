import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { BlockToolbar } from './BlockToolbar'
import type { EmailBlock, SaveBlockHandler, SavedBlock } from '../types'

// The selected section. Attribute keys in the same order as the saved "Header" below, which differs only in a
// long-hand padding: the shape the old "is this block already saved?" guess matched.
const section = {
  id: 'products',
  type: 'mj-section',
  attributes: { backgroundColor: '#ffffff', paddingTop: '8px', paddingBottom: '8px' },
  children: []
} as unknown as EmailBlock

const header: SavedBlock = {
  id: 'header-id',
  name: 'Header',
  block: {
    id: 'h',
    type: 'mj-section',
    attributes: { backgroundColor: '#ffffff', paddingTop: '24px', paddingBottom: '8px' },
    children: []
  } as unknown as EmailBlock,
  created: '2026-09-01T00:00:00Z'
}
const footer: SavedBlock = {
  id: 'footer-id',
  name: 'Footer',
  block: {
    id: 'f',
    type: 'mj-section',
    attributes: { backgroundColor: '#f3efe8' },
    children: []
  } as unknown as EmailBlock,
  created: '2026-10-01T00:00:00Z'
}
const cta: SavedBlock = {
  id: 'cta-id',
  name: 'CTA',
  block: { id: 'c', type: 'mj-button', attributes: {}, content: 'Shop' } as unknown as EmailBlock,
  created: '2026-11-01T00:00:00Z'
}

function renderToolbar(onSave: SaveBlockHandler, savedBlocks: SavedBlock[]) {
  render(
    <App>
      <BlockToolbar
        blockId={section.id}
        block={section}
        onClone={vi.fn()}
        onDelete={vi.fn()}
        onSave={onSave}
        savedBlocks={savedBlocks}
      />
    </App>
  )
}

const openDialog = async () => {
  const save = document.querySelector('svg[data-icon="floppy-disk"]')!.closest('button')!
  await userEvent.click(save)
}

// The test setup reports every element as displayed and antd keeps a closed dialog in the DOM, so whether the
// dialog is open is read from the inline display antd sets on its wrapper.
const dialogDisplay = () => (document.querySelector('.ant-modal-wrap') as HTMLElement).style.display
const nameInput = () => screen.getByPlaceholderText('Enter a name for this block') as HTMLInputElement
const saveButton = () => screen.getByRole('button', { name: 'Save' })
const toasts = () => document.querySelectorAll('.ant-message-notice')

const deferred = () => {
  let resolve!: () => void
  let reject!: (error: Error) => void
  const promise = new Promise<void>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('BlockToolbar save dialog', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  it('opens in "Save as new block" mode even when a saved section looks alike', async () => {
    renderToolbar(vi.fn().mockResolvedValue(undefined), [header, footer, cta])
    await openDialog()

    expect(screen.getByRole('radio', { name: 'Save as new block' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Update existing block' })).not.toBeChecked()
    expect(nameInput().value).toBe('section Block')
    expect(screen.queryByText('Select Block to Update')).not.toBeInTheDocument()
  })

  it("lists only saved blocks of the selected block's type when updating, newest first", async () => {
    renderToolbar(vi.fn().mockResolvedValue(undefined), [header, cta, footer])
    await openDialog()

    await userEvent.click(screen.getByRole('radio', { name: 'Update existing block' }))
    await userEvent.click(screen.getByRole('combobox'))
    await screen.findAllByText('Footer (mj-section)', { selector: '.ant-select-item-option-content' })

    const options = Array.from(document.querySelectorAll('.ant-select-item-option-content')).map(
      (option) => option.textContent
    )
    expect(options).toEqual(['Footer (mj-section)', 'Header (mj-section)'])
  })

  it("disables \"Update existing block\" when no saved block has the selected block's type", async () => {
    renderToolbar(vi.fn().mockResolvedValue(undefined), [cta])
    await openDialog()

    expect(screen.getByRole('radio', { name: 'Update existing block' })).toBeDisabled()
  })

  it('does not reorder the savedBlocks prop', async () => {
    const savedBlocks = [header, cta, footer]
    renderToolbar(vi.fn().mockResolvedValue(undefined), savedBlocks)
    await openDialog()
    await userEvent.click(screen.getByRole('radio', { name: 'Update existing block' }))
    await userEvent.click(screen.getByRole('combobox'))

    expect(savedBlocks.map((saved) => saved.id)).toEqual(['header-id', 'cta-id', 'footer-id'])
  })

  it('saves a new block and closes once the save resolves', async () => {
    const pending = deferred()
    const onSave = vi.fn().mockReturnValue(pending.promise)
    renderToolbar(onSave, [header])
    await openDialog()

    await userEvent.clear(nameInput())
    await userEvent.type(nameInput(), 'Products')
    await userEvent.click(saveButton())

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith(section, 'create', 'Products')
    expect(dialogDisplay()).toBe('')

    pending.resolve()
    await waitFor(() => expect(dialogDisplay()).toBe('none'))
    expect(toasts()).toHaveLength(0)
  })

  it('keeps the dialog open with the typed name when the save fails', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('boom'))
    renderToolbar(onSave, [])
    await openDialog()

    await userEvent.clear(nameInput())
    await userEvent.type(nameInput(), 'Products')
    await userEvent.click(saveButton())

    await waitFor(() => expect(saveButton()).not.toHaveClass('ant-btn-loading'))
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(dialogDisplay()).toBe('')
    expect(nameInput().value).toBe('Products')
  })

  it('cannot be dismissed while a save is pending', async () => {
    const pending = deferred()
    renderToolbar(vi.fn().mockReturnValue(pending.promise), [])
    await openDialog()
    expect(document.querySelector('.ant-modal-close')).not.toBeNull()

    await userEvent.click(saveButton())

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(document.querySelector('.ant-modal-close')).toBeNull()
    await userEvent.keyboard('{Escape}')
    await userEvent.click(document.querySelector('.ant-modal-wrap') as HTMLElement)
    expect(dialogDisplay()).toBe('')
    pending.resolve()
    await waitFor(() => expect(dialogDisplay()).toBe('none'))
  })

  // Characterisation: also held before the fix
  it('updates the chosen saved block', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderToolbar(onSave, [header, footer])
    await openDialog()

    await userEvent.click(screen.getByRole('radio', { name: 'Update existing block' }))
    await userEvent.click(screen.getByRole('combobox'))
    const footerOption = await screen.findAllByText('Footer (mj-section)', {
      selector: '.ant-select-item-option-content'
    })
    await userEvent.click(footerOption[footerOption.length - 1])
    await userEvent.click(screen.getByRole('button', { name: 'Update' }))

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith(section, 'update', 'footer-id')
  })

  // Guards the awaited save: before the fix the reset name stopped the second Enter instead. The input ignores
  // a second Enter until the key is released, so each press is a keydown and a keyup.
  it('ignores Enter while a save is pending', async () => {
    const pending = deferred()
    const onSave = vi.fn().mockReturnValue(pending.promise)
    renderToolbar(onSave, [])
    await openDialog()

    const pressEnter = () => {
      fireEvent.keyDown(nameInput(), { key: 'Enter', code: 'Enter', keyCode: 13 })
      fireEvent.keyUp(nameInput(), { key: 'Enter', code: 'Enter', keyCode: 13 })
    }
    pressEnter()
    pressEnter()

    expect(onSave).toHaveBeenCalledTimes(1)
    pending.resolve()
    await waitFor(() => expect(dialogDisplay()).toBe('none'))
  })
})
