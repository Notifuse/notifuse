import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SavedBlockCard, type CardActions } from './SavedBlockCard'
import { headKeyOf, type LibraryItem } from '../utils/library'
import { b, email, text } from '../utils/libraryTestTrees'

const tree = email([])
const item: LibraryItem = {
  kind: 'saved',
  key: 'header-id',
  name: 'Header',
  block: b('h', 'mj-section', [b('c', 'mj-column', [text('t', '<p>Welcome</p>')])])
}

const renderCard = (props: Partial<React.ComponentProps<typeof SavedBlockCard>> = {}) => {
  const onInsert = vi.fn()
  render(
    <SavedBlockCard
      item={item}
      typeLabel="Section"
      date="2026-10-05T09:00:00Z"
      locale="en"
      emailTree={tree}
      headKey={headKeyOf(tree)}
      refusal={null}
      onInsert={onInsert}
      {...props}
    />
  )
  return onInsert
}

describe('SavedBlockCard', () => {
  it('shows the name, the type and the date', () => {
    renderCard()
    expect(screen.getByText('Header')).toBeInTheDocument()
    expect(screen.getByText(/Section/)).toHaveTextContent('Section · Oct 5, 2026')
    expect(screen.getByText('Welcome')).toBeInTheDocument()
  })

  it("writes the date in the console's language", () => {
    renderCard({ locale: 'fr' })
    expect(screen.getByText(/Section/)).toHaveTextContent('Section · 5 oct. 2026')
  })

  it('inserts the block', () => {
    const onInsert = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Insert' }))
    expect(onInsert).toHaveBeenCalledWith(item)
  })

  it('disables "Insert" and gives the reason when the block cannot go there', async () => {
    const onInsert = renderCard({ refusal: 'Select a column' })
    const insert = screen.getByRole('button', { name: 'Insert' })
    expect(insert).toBeDisabled()

    await userEvent.hover(insert.parentElement as HTMLElement)
    expect(await screen.findByText('Select a column')).toBeInTheDocument()
    fireEvent.click(insert)
    expect(onInsert).not.toHaveBeenCalled()
  })

  it('hides "Insert" with opacity only, so it stays in the tab order', async () => {
    renderCard()
    const actions = screen.getByTestId('library-card-actions')
    expect(actions.className).toContain('opacity-0')
    expect(actions.className).toContain('focus-within:opacity-100')
    expect(actions.className).not.toMatch(/\b(hidden|invisible)\b/)
    expect(actions.style.display).toBe('')
    expect(actions.style.visibility).toBe('')

    await userEvent.tab()
    expect(screen.getByRole('button', { name: 'Insert' })).toHaveFocus()
  })

  it('puts the drag handle on a plain element, not on a button around "Insert"', () => {
    renderCard()
    const handle = document.querySelector('[data-library-drag-handle]') as HTMLElement
    expect(handle.getAttribute('role')).toBeNull()
    expect(handle.contains(screen.getByRole('button', { name: 'Insert' }))).toBe(false)
  })

  it('carries a "New" tag for a new block, and only a border for an updated one', () => {
    const { unmount } = render(
      <SavedBlockCard
        item={item}
        typeLabel="Section"
        locale="en"
        emailTree={tree}
        headKey={headKeyOf(tree)}
        refusal={null}
        highlight="new"
        onInsert={vi.fn()}
      />
    )
    expect(screen.getByText('New')).toBeInTheDocument()
    unmount()
    renderCard({ highlight: 'updated' })
    expect(screen.queryByText('New')).toBeNull()
    expect(document.querySelector('[data-library-card]')!.className).toContain('border-[#7763F1]')
  })
})

describe('SavedBlockCard actions', () => {
  const actions = (over: Partial<CardActions> = {}): CardActions => ({
    busy: false,
    replaceRefusal: null,
    onRename: vi.fn().mockResolvedValue(undefined),
    onReplace: vi.fn(),
    onDuplicate: vi.fn(),
    onDelete: vi.fn(),
    ...over
  })
  const openMenu = async () => {
    await userEvent.click(screen.getByRole('button', { name: 'More actions for Header' }))
  }
  const menuItem = (name: string) => screen.getByRole('menuitem', { name: new RegExp(name) })
  const startRename = async () => {
    await openMenu()
    await userEvent.click(menuItem('Rename'))
    return (await screen.findByRole('textbox', { name: 'Block name' })) as HTMLInputElement
  }

  it('has no menu without actions', () => {
    renderCard()
    expect(screen.queryByRole('button', { name: /More actions/ })).toBeNull()
  })

  it('lists rename, replace, duplicate and delete', async () => {
    renderCard({ actions: actions() })
    await openMenu()
    for (const name of ['Rename', 'Replace with the selected block', 'Duplicate', 'Delete']) {
      expect(menuItem(name)).toBeInTheDocument()
    }
  })

  it('duplicates, replaces and deletes through its handlers', async () => {
    const handlers = actions()
    renderCard({ actions: handlers })
    await openMenu()
    await userEvent.click(menuItem('Duplicate'))
    expect(handlers.onDuplicate).toHaveBeenCalledTimes(1)
    await openMenu()
    await userEvent.click(menuItem('Replace with the selected block'))
    expect(handlers.onReplace).toHaveBeenCalledTimes(1)
    await openMenu()
    await userEvent.click(menuItem('Delete'))
    expect(handlers.onDelete).toHaveBeenCalledTimes(1)
  })

  it('disables replace with the reason when the selection does not fit', async () => {
    const handlers = actions({ replaceRefusal: 'Select a block of the same type' })
    renderCard({ actions: handlers })
    await openMenu()
    const replace = menuItem('Replace with the selected block')
    expect(replace).toHaveAttribute('aria-disabled', 'true')
    expect(replace).toHaveTextContent('Select a block of the same type')
    await userEvent.click(replace)
    expect(handlers.onReplace).not.toHaveBeenCalled()
  })

  it('disables every action while a library change runs', async () => {
    renderCard({ actions: actions({ busy: true }) })
    await openMenu()
    for (const name of ['Rename', 'Replace with the selected block', 'Duplicate', 'Delete']) {
      expect(menuItem(name)).toHaveAttribute('aria-disabled', 'true')
    }
  })

  it('renames with Enter, trimmed, once even when the input then loses focus', async () => {
    const handlers = actions()
    renderCard({ actions: handlers })
    const input = await startRename()
    expect(input).toHaveFocus()
    expect(input.value).toBe('Header')
    await userEvent.clear(input)
    await userEvent.type(input, '  Top banner  {Enter}')
    input.blur()
    expect(handlers.onRename).toHaveBeenCalledTimes(1)
    expect(handlers.onRename).toHaveBeenCalledWith('Top banner')
  })

  it('saves the name when the input loses focus', async () => {
    const handlers = actions()
    renderCard({ actions: handlers })
    const input = await startRename()
    await userEvent.clear(input)
    await userEvent.type(input, 'Banner')
    fireEvent.blur(input)
    expect(handlers.onRename).toHaveBeenCalledWith('Banner')
  })

  it('cancels with Escape, and saves nothing for an empty or unchanged name', async () => {
    const handlers = actions()
    renderCard({ actions: handlers })
    let input = await startRename()
    await userEvent.type(input, ' more{Escape}')
    fireEvent.blur(input)
    expect(screen.queryByRole('textbox', { name: 'Block name' })).toBeNull()
    expect(screen.getByText('Header')).toBeInTheDocument()

    input = await startRename()
    await userEvent.type(input, '{Enter}')
    input = await startRename()
    await userEvent.clear(input)
    await userEvent.type(input, '   {Enter}')
    expect(handlers.onRename).not.toHaveBeenCalled()
  })

  it('keeps a name within 255 bytes', async () => {
    renderCard({ actions: actions() })
    const input = await startRename()
    await userEvent.clear(input)
    fireEvent.change(input, { target: { value: '日'.repeat(100) } })
    expect(new TextEncoder().encode(input.value).length).toBeLessThanOrEqual(255)
  })

  it('stops the card from being dragged while its name is edited', async () => {
    renderCard({ actions: actions() })
    const handle = document.querySelector('[data-library-drag-handle]') as HTMLElement
    expect(handle.className).toContain('cursor-grab')
    await startRename()
    expect(handle.className).not.toContain('cursor-grab')
  })

  it('disables the input while the name is saved, and keeps the old name on failure', async () => {
    let fail: (error: Error) => void = () => {}
    const onRename = vi.fn(() => new Promise<void>((_, reject) => (fail = reject)))
    renderCard({ actions: actions({ onRename }) })
    const input = await startRename()
    await userEvent.clear(input)
    await userEvent.type(input, 'Banner{Enter}')
    expect(input).toBeDisabled()
    await act(async () => fail(new Error('nope')))
    expect(screen.queryByRole('textbox', { name: 'Block name' })).toBeNull()
    expect(screen.getByText('Header')).toBeInTheDocument()
  })

  it('keeps the actions visible while the menu is open', async () => {
    renderCard({ actions: actions() })
    await openMenu()
    expect(screen.getByTestId('library-card-actions').className).toContain('!opacity-100')
  })
})
