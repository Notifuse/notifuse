import { useState } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, Button } from 'antd'
import { SaveBlockModal } from './SaveBlockModal'
import type { EmailBlock, SaveBlockHandler, SavedBlock } from '../types'
import { email } from '../utils/libraryTestTrees'

// The selected section. Attribute keys in the same order as the saved "Header" below, which differs only in a
// long-hand padding: the shape the old "is this block already saved?" guess matched.
const section = {
  id: 'products',
  type: 'mj-section',
  attributes: { backgroundColor: '#ffffff', paddingTop: '8px', paddingBottom: '8px' },
  children: [
    {
      id: 'products-column',
      type: 'mj-column',
      attributes: {},
      children: [
        { id: 'products-text', type: 'mj-text', attributes: {}, content: '<h2>Our products</h2>' }
      ]
    }
  ]
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
  created: '2026-08-01T00:00:00Z',
  updated: '2026-10-01T00:00:00Z'
}
const cta: SavedBlock = {
  id: 'cta-id',
  name: 'CTA',
  block: { id: 'c', type: 'mj-button', attributes: {}, content: 'Shop' } as unknown as EmailBlock,
  created: '2026-11-01T00:00:00Z'
}

const tree = email([section])

// Opens the dialog the way the editor does: a button sets the block to save
function Harness({
  onSave,
  savedBlocks,
  onSaved
}: {
  onSave: SaveBlockHandler
  savedBlocks: SavedBlock[]
  onSaved?: (id: string, operation: string) => void
}) {
  const [block, setBlock] = useState<EmailBlock | null>(null)
  return (
    <App>
      <Button onClick={() => setBlock(section)}>open</Button>
      <SaveBlockModal
        block={block}
        emailTree={tree}
        savedBlocks={savedBlocks}
        onSave={onSave}
        onSaved={onSaved}
        onClose={() => setBlock(null)}
      />
    </App>
  )
}

const renderDialog = (
  onSave: SaveBlockHandler,
  savedBlocks: SavedBlock[],
  onSaved?: (id: string, operation: string) => void
) => render(<Harness onSave={onSave} savedBlocks={savedBlocks} onSaved={onSaved} />)

const openDialog = () => fireEvent.click(screen.getByRole('button', { name: 'open' }))

// The test setup reports every element as displayed and antd keeps a closed dialog in the DOM, so whether the
// dialog is open is read from the inline display antd sets on its wrapper.
const dialogDisplay = () => (document.querySelector('.ant-modal-wrap') as HTMLElement).style.display
const nameInput = () =>
  screen.getByPlaceholderText('Enter a name for this block') as HTMLInputElement
const saveButton = () => screen.getByRole('button', { name: 'Save' })
const replaceRadio = () => screen.getByRole('radio', { name: /Replace an existing block/ })
const toasts = () => document.querySelectorAll('.ant-message-notice')

const deferred = () => {
  let resolve!: (id: string) => void
  let reject!: (error: Error) => void
  const promise = new Promise<string>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('SaveBlockModal', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  it('is titled "Save to library" and shows the block and its type', () => {
    renderDialog(vi.fn().mockResolvedValue('id'), [])
    openDialog()

    expect(screen.getByText('Save to library')).toBeInTheDocument()
    expect(screen.getByTestId('block-thumb')).toBeInTheDocument()
    expect(
      screen.getByText('Our products', { selector: '[data-testid="block-thumb"] *' })
    ).toBeInTheDocument()
    expect(screen.getByTestId('save-block-type')).toHaveTextContent('Section')
  })

  it('opens as a new block with a suggested name, even when a saved section looks alike', () => {
    renderDialog(vi.fn().mockResolvedValue('id'), [header, footer, cta])
    openDialog()

    expect(screen.getByRole('radio', { name: 'New block' })).toBeChecked()
    expect(replaceRadio()).not.toBeChecked()
    expect(nameInput().value).toBe('Section · Our products')
    expect(screen.queryByText('Block to replace')).not.toBeInTheDocument()
  })

  it('counts the saved blocks of the same type it can replace', () => {
    renderDialog(vi.fn().mockResolvedValue('id'), [header, footer, cta])
    openDialog()

    expect(replaceRadio()).toHaveAccessibleName('Replace an existing block (2)')
    expect(replaceRadio()).not.toBeDisabled()
  })

  it('disables replacing when no saved block has the type of the block', () => {
    renderDialog(vi.fn().mockResolvedValue('id'), [cta])
    openDialog()

    expect(replaceRadio()).toHaveAccessibleName('Replace an existing block (0)')
    expect(replaceRadio()).toBeDisabled()
  })

  it("lists only saved blocks of the block's type to replace, newest first", async () => {
    renderDialog(vi.fn().mockResolvedValue('id'), [header, cta, footer])
    openDialog()

    await userEvent.click(replaceRadio())
    await userEvent.click(screen.getByRole('combobox'))
    await screen.findAllByText('Footer', { selector: '.ant-select-item-option-content' })

    const options = Array.from(document.querySelectorAll('.ant-select-item-option-content')).map(
      (option) => option.textContent
    )
    // Footer was modified after Header was created
    expect(options).toEqual(['Footer', 'Header'])
  })

  it('does not reorder the savedBlocks prop', async () => {
    const savedBlocks = [header, cta, footer]
    renderDialog(vi.fn().mockResolvedValue('id'), savedBlocks)
    openDialog()
    await userEvent.click(replaceRadio())
    await userEvent.click(screen.getByRole('combobox'))

    expect(savedBlocks.map((saved) => saved.id)).toEqual(['header-id', 'cta-id', 'footer-id'])
  })

  it('saves a new block, closes once the save resolves and hands over the saved id', async () => {
    const pending = deferred()
    const onSave = vi.fn().mockReturnValue(pending.promise)
    const onSaved = vi.fn()
    renderDialog(onSave, [header], onSaved)
    openDialog()

    await userEvent.clear(nameInput())
    await userEvent.type(nameInput(), 'Products')
    await userEvent.click(saveButton())

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith(section, 'create', 'Products')
    expect(dialogDisplay()).toBe('')
    expect(onSaved).not.toHaveBeenCalled()

    pending.resolve('new-id')
    await waitFor(() => expect(dialogDisplay()).toBe('none'))
    expect(onSaved).toHaveBeenCalledWith('new-id', 'create')
    expect(toasts()).toHaveLength(0)
  })

  it('keeps the dialog open with the typed name when the save fails', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('boom'))
    const onSaved = vi.fn()
    renderDialog(onSave, [], onSaved)
    openDialog()

    await userEvent.clear(nameInput())
    await userEvent.type(nameInput(), 'Products')
    await userEvent.click(saveButton())

    await waitFor(() => expect(saveButton()).not.toHaveClass('ant-btn-loading'))
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(dialogDisplay()).toBe('')
    expect(nameInput().value).toBe('Products')
    expect(onSaved).not.toHaveBeenCalled()
  })

  it('cannot be dismissed while a save is pending', async () => {
    const pending = deferred()
    renderDialog(vi.fn().mockReturnValue(pending.promise), [])
    openDialog()
    expect(document.querySelector('.ant-modal-close')).not.toBeNull()

    await userEvent.click(saveButton())

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(document.querySelector('.ant-modal-close')).toBeNull()
    await userEvent.keyboard('{Escape}')
    await userEvent.click(document.querySelector('.ant-modal-wrap') as HTMLElement)
    expect(dialogDisplay()).toBe('')
    pending.resolve('id')
    await waitFor(() => expect(dialogDisplay()).toBe('none'))
  })

  it('replaces the chosen saved block', async () => {
    const onSave = vi.fn().mockResolvedValue('footer-id')
    const onSaved = vi.fn()
    renderDialog(onSave, [header, footer], onSaved)
    openDialog()

    await userEvent.click(replaceRadio())
    await userEvent.click(screen.getByRole('combobox'))
    const footerOption = await screen.findAllByText('Footer', {
      selector: '.ant-select-item-option-content'
    })
    await userEvent.click(footerOption[footerOption.length - 1])
    await userEvent.click(screen.getByRole('button', { name: 'Replace' }))

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith(section, 'update', 'footer-id')
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('footer-id', 'update'))
  })

  // Guards the awaited save: a second Enter used to save the block twice. The input ignores a second Enter until
  // the key is released, so each press is a keydown and a keyup.
  it('ignores Enter while a save is pending', async () => {
    const pending = deferred()
    const onSave = vi.fn().mockReturnValue(pending.promise)
    renderDialog(onSave, [])
    openDialog()

    const pressEnter = () => {
      fireEvent.keyDown(nameInput(), { key: 'Enter', code: 'Enter', keyCode: 13 })
      fireEvent.keyUp(nameInput(), { key: 'Enter', code: 'Enter', keyCode: 13 })
    }
    pressEnter()
    pressEnter()

    expect(onSave).toHaveBeenCalledTimes(1)
    pending.resolve('id')
    await waitFor(() => expect(dialogDisplay()).toBe('none'))
  })
})
