import React from 'react'
import { describe, it, expect, onTestFinished, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DndContext } from '@dnd-kit/core'
import { LibraryPanel, type LibraryManage } from './LibraryPanel'
import type { SavedBlock } from '../types'
import { b, email, text } from '../utils/libraryTestTrees'

const saved = (
  id: string,
  name: string,
  type: string,
  dates: Partial<SavedBlock> = {}
): SavedBlock => ({
  id,
  name,
  block: b(`${id}-block`, type, type === 'mj-section' ? [] : undefined),
  ...dates
})

const header = saved('header', 'Header', 'mj-section', { created: '2026-09-01T00:00:00Z' })
const footer = saved('footer', 'Pied de page', 'mj-section', {
  created: '2026-07-01T00:00:00Z',
  updated: '2026-10-01T00:00:00Z'
})
const cta = saved('cta', 'Élan CTA', 'mj-button', { created: '2026-08-01T00:00:00Z' })
// Enough older blocks for the search to show, matched by none of the searches below
const others = Array.from({ length: 10 }, (_, i) =>
  saved(`block-${i + 1}`, `Block ${i + 1}`, 'mj-text', { created: '2020-01-01T00:00:00Z' })
)
const searchable = (...blocks: SavedBlock[]) => [...blocks, ...others]

const tree = email([b('section-1', 'mj-section', [b('column-1', 'mj-column', [text('text-1')])])])

const renderPanel = (props: Partial<React.ComponentProps<typeof LibraryPanel>> = {}) => {
  const handlers = { onInsert: vi.fn(), onOpenSaveDialog: vi.fn() }
  render(
    <LibraryPanel
      emailTree={tree}
      savedBlocks={[header, cta, footer]}
      selectedBlockId={null}
      {...handlers}
      {...props}
    />
  )
  return handlers
}

const cardNames = () =>
  Array.from(document.querySelectorAll('[data-library-card]')).map((card) =>
    card.getAttribute('data-library-card')
  )

describe('LibraryPanel', () => {
  it('lists the saved blocks newest first, by modification date, else creation date', () => {
    renderPanel()
    expect(cardNames()).toEqual(['footer', 'header', 'cta'])
  })

  it('shows the search only above ten saved blocks', () => {
    renderPanel({ savedBlocks: others })
    expect(screen.queryByPlaceholderText('Search saved blocks')).toBeNull()
    cleanup()
    renderPanel({ savedBlocks: searchable(cta) })
    expect(screen.getByPlaceholderText('Search saved blocks')).toBeInTheDocument()
  })

  it('keeps a search under way when the library falls to ten blocks, and hides the box once it is cleared', async () => {
    const props = { emailTree: tree, selectedBlockId: null, onInsert: vi.fn(), onOpenSaveDialog: vi.fn() }
    const { rerender } = render(<LibraryPanel {...props} savedBlocks={searchable(cta)} />)
    await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'elan')
    expect(cardNames()).toEqual(['cta'])

    rerender(<LibraryPanel {...props} savedBlocks={[cta, ...others.slice(1)]} />)
    expect(screen.getByPlaceholderText('Search saved blocks')).toHaveValue('elan')
    expect(cardNames()).toEqual(['cta'])

    await userEvent.clear(screen.getByPlaceholderText('Search saved blocks'))
    expect(screen.queryByPlaceholderText('Search saved blocks')).toBeNull()
    expect(cardNames()).toHaveLength(10)
  })

  it('counts the blocks it shows', async () => {
    renderPanel({ savedBlocks: searchable(header, cta, footer) })
    expect(screen.getByText('Saved blocks').parentElement).toHaveTextContent('Saved blocks (13)')
    await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'head')
    expect(screen.getByText('Saved blocks').parentElement).toHaveTextContent('Saved blocks (1)')
  })

  it('searches names whatever their case and accents', async () => {
    renderPanel({ savedBlocks: searchable(header, cta, footer) })
    await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'elan')
    expect(cardNames()).toEqual(['cta'])
    await userEvent.clear(screen.getByPlaceholderText('Search saved blocks'))
    await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'PIED')
    expect(cardNames()).toEqual(['footer'])
  })

  it('filters sections and content', () => {
    renderPanel()
    fireEvent.click(screen.getByText('Sections'))
    expect(cardNames()).toEqual(['footer', 'header'])
    fireEvent.click(screen.getByText('Content'))
    expect(cardNames()).toEqual(['cta'])
    fireEvent.click(screen.getByText('All'))
    expect(cardNames()).toEqual(['footer', 'header', 'cta'])
  })

  it('refuses to insert a content block with nothing selected, and gives the reason', async () => {
    const { onInsert } = renderPanel()
    const ctaCard = document.querySelector('[data-library-card="cta"]') as HTMLElement
    const insert = within(ctaCard).getByRole('button', { name: 'Insert' })
    expect(insert).toBeDisabled()
    // The reason shows on the disabled button's wrapper, which takes the hover
    fireEvent.mouseEnter(insert.parentElement!)
    expect(await screen.findByText('Select a column')).toBeInTheDocument()

    const headerCard = document.querySelector('[data-library-card="header"]') as HTMLElement
    fireEvent.click(within(headerCard).getByRole('button', { name: 'Insert' }))
    expect(onInsert).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'saved', key: 'header', block: header.block })
    )
  })

  it('lets a content block be inserted once a column is selected', () => {
    renderPanel({ selectedBlockId: 'text-1' })
    const ctaCard = document.querySelector('[data-library-card="cta"]') as HTMLElement
    expect(within(ctaCard).getByRole('button', { name: 'Insert' })).not.toBeDisabled()
  })

  it('explains how to save a block when there is none, without a save button when nothing is selected', () => {
    renderPanel({ savedBlocks: [] })
    expect(screen.getByTestId('library-empty')).toHaveTextContent('No saved blocks yet')
    expect(
      screen.queryByRole('button', { name: 'Save the selected block' })
    ).not.toBeInTheDocument()
  })

  it('opens the save dialog for the selected block from the empty library', () => {
    const { onOpenSaveDialog } = renderPanel({ savedBlocks: [], selectedBlockId: 'section-1' })
    fireEvent.click(screen.getByRole('button', { name: 'Save the selected block' }))
    expect(onOpenSaveDialog).toHaveBeenCalledWith('section-1')
  })

  it('offers no save button with the body selected', () => {
    renderPanel({ savedBlocks: [], selectedBlockId: 'body' })
    expect(
      screen.queryByRole('button', { name: 'Save the selected block' })
    ).not.toBeInTheDocument()
  })

  it('inserts a basic block from its tile', () => {
    const { onInsert } = renderPanel()
    fireEvent.click(document.querySelector('[data-library-tile="mj-section"]') as HTMLElement)
    expect(onInsert).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'basic',
        block: expect.objectContaining({ type: 'mj-section' })
      })
    )
  })

  it('does not insert a basic block its tile cannot place', () => {
    const { onInsert } = renderPanel()
    const tile = document.querySelector('[data-library-tile="mj-text"]') as HTMLElement
    expect(tile).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(tile)
    expect(onInsert).not.toHaveBeenCalled()
  })

  it('tags the block just saved "New"', () => {
    renderPanel({ highlight: { id: 'header', kind: 'new' } })
    const headerCard = document.querySelector('[data-library-card="header"]') as HTMLElement
    expect(within(headerCard).getByText('New')).toBeInTheDocument()
    expect(screen.getAllByText('New')).toHaveLength(1)
  })

  describe('managing saved blocks', () => {
    const manage = (over: Partial<LibraryManage> = {}): LibraryManage => ({
      busy: false,
      onRename: vi.fn().mockResolvedValue(undefined),
      onReplace: vi.fn(),
      onDuplicate: vi.fn(),
      onDelete: vi.fn(),
      ...over
    })
    const menuOf = async (id: string, name: string) => {
      const card = document.querySelector(`[data-library-card="${id}"]`) as HTMLElement
      await userEvent.click(within(card).getByRole('button', { name: `More actions for ${name}` }))
    }
    // Menus closed earlier stay in the document, hidden: the open one is the last
    const item = (name: string) =>
      screen.getAllByRole('menuitem', { name: new RegExp(name) }).at(-1)!

    it('keeps a renamed card that leaves the search listed, until the search changes', async () => {
      const props = { emailTree: tree, selectedBlockId: null, onInsert: vi.fn(), onOpenSaveDialog: vi.fn(), manage: manage() }
      const { rerender } = render(<LibraryPanel {...props} savedBlocks={searchable(header, cta, footer)} />)
      await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'head')
      expect(cardNames()).toEqual(['header'])

      const renamed = { ...header, name: 'Banner', updated: '2026-11-01T00:00:00Z' }
      rerender(
        <LibraryPanel {...props} savedBlocks={searchable(renamed, cta, footer)} highlight={{ id: 'header', kind: 'updated' }} />
      )
      expect(cardNames()).toEqual(['header'])

      await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'e')
      expect(cardNames()).toEqual([])
    })

    it('moves the focus to the next card after a delete, else to the previous one, else to the list', async () => {
      const onDelete = vi.fn()
      const props = { emailTree: tree, selectedBlockId: null, onInsert: vi.fn(), onOpenSaveDialog: vi.fn(), manage: manage({ onDelete }) }
      const { rerender } = render(<LibraryPanel {...props} savedBlocks={[header, cta, footer]} />)
      // Newest first
      expect(cardNames()).toEqual(['footer', 'header', 'cta'])
      const moreOf = (id: string, name: string) =>
        within(document.querySelector(`[data-library-card="${id}"]`) as HTMLElement).getByRole('button', {
          name: `More actions for ${name}`
        })

      // In the middle: the next card
      await menuOf('header', 'Header')
      await userEvent.click(item('Delete'))
      expect(onDelete).toHaveBeenCalledWith(header)
      rerender(<LibraryPanel {...props} savedBlocks={[cta, footer]} />)
      expect(moreOf('cta', 'Élan CTA')).toHaveFocus()

      // The last card: the one before it
      await menuOf('cta', 'Élan CTA')
      await userEvent.click(item('Delete'))
      rerender(<LibraryPanel {...props} savedBlocks={[footer]} />)
      expect(moreOf('footer', 'Pied de page')).toHaveFocus()

      // The only card shown, others hidden by the search: the list
      rerender(<LibraryPanel {...props} savedBlocks={searchable(header, footer)} />)
      await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'pied')
      expect(cardNames()).toEqual(['footer'])
      await menuOf('footer', 'Pied de page')
      await userEvent.click(item('Delete'))
      rerender(<LibraryPanel {...props} savedBlocks={searchable(header)} />)
      expect(screen.getByTestId('library-saved-blocks')).toHaveFocus()
    })

    it('leaves the focus where it is after a delete cancelled in its confirmation', async () => {
      const onDelete = vi.fn()
      const props = { emailTree: tree, selectedBlockId: null, onInsert: vi.fn(), onOpenSaveDialog: vi.fn(), manage: manage({ onDelete }) }
      render(<LibraryPanel {...props} savedBlocks={searchable(header, cta, footer)} />)
      await menuOf('cta', 'Élan CTA')
      await userEvent.click(item('Delete'))
      // The confirmation is cancelled: the library does not change
      const searchBox = screen.getByPlaceholderText('Search saved blocks')
      await userEvent.type(searchBox, 'head')
      expect(cardNames()).toEqual(['header'])
      expect(searchBox).toHaveFocus()
    })

    it('shows no menu without the right to change the library', () => {
      renderPanel()
      expect(screen.queryByRole('button', { name: /More actions/ })).toBeNull()
    })

    it('hands the saved block to each action', async () => {
      const handlers = manage()
      renderPanel({ manage: handlers, selectedBlockId: 'section-1' })
      await menuOf('header', 'Header')
      await userEvent.click(item('Duplicate'))
      expect(handlers.onDuplicate).toHaveBeenCalledWith(header)
      await menuOf('header', 'Header')
      await userEvent.click(item('Replace with the selected block'))
      expect(handlers.onReplace).toHaveBeenCalledWith(header)
      await menuOf('cta', 'Élan CTA')
      await userEvent.click(item('Delete'))
      expect(handlers.onDelete).toHaveBeenCalledWith(cta)
    })

    it('replaces only with a block selected in the email', async () => {
      renderPanel({ manage: manage() })
      await menuOf('header', 'Header')
      expect(item('Replace with the selected block')).toHaveTextContent(
        'Select a block in the email'
      )
    })

    it('refuses a selected block of another type', async () => {
      renderPanel({ manage: manage(), selectedBlockId: 'text-1' })
      await menuOf('header', 'Header')
      const replace = item('Replace with the selected block')
      expect(replace).toHaveAttribute('aria-disabled', 'true')
      expect(replace).toHaveTextContent('Select a block of the same type')
    })

    it('marks an updated block with its border only', () => {
      renderPanel({ highlight: { id: 'header', kind: 'updated' } })
      expect(screen.queryByText('New')).toBeNull()
      const card = document.querySelector('[data-library-card="header"]') as HTMLElement
      expect(card.className).toContain('border-primary-500')
    })

    it('brings the changed card into view once, not again when the list changes', () => {
      const scroll = vi.fn()
      // jsdom has no scrollIntoView: it is set for this test only
      const original = Element.prototype.scrollIntoView
      Element.prototype.scrollIntoView = scroll
      onTestFinished(() => {
        Element.prototype.scrollIntoView = original
      })
      const highlight = { id: 'header', kind: 'updated' as const }
      const handlers = { onInsert: vi.fn() }
      const { rerender } = render(
        <LibraryPanel
          emailTree={tree}
          savedBlocks={[header, cta, footer]}
          selectedBlockId={null}
          highlight={highlight}
          {...handlers}
        />
      )
      expect(scroll).toHaveBeenCalledTimes(1)
      rerender(
        <LibraryPanel
          emailTree={tree}
          savedBlocks={[header, footer]}
          selectedBlockId={null}
          highlight={highlight}
          {...handlers}
        />
      )
      expect(scroll).toHaveBeenCalledTimes(1)
    })

    it('lists a changed card the search hides, and brings it into view once', async () => {
      const scroll = vi.fn()
      const original = Element.prototype.scrollIntoView
      Element.prototype.scrollIntoView = scroll
      onTestFinished(() => {
        Element.prototype.scrollIntoView = original
      })
      const props = { emailTree: tree, selectedBlockId: null, onInsert: vi.fn() }
      const { rerender } = render(<LibraryPanel {...props} savedBlocks={searchable(header, cta, footer)} />)
      await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'elan')
      const highlight = { id: 'header', kind: 'updated' as const }
      rerender(
        <LibraryPanel {...props} savedBlocks={searchable(header, cta, footer)} highlight={highlight} />
      )
      // Changed from this editor: listed despite the search, until the search changes
      expect(cardNames()).toEqual(['header', 'cta'])
      expect(scroll).toHaveBeenCalledTimes(1)
      await userEvent.clear(screen.getByPlaceholderText('Search saved blocks'))
      expect(scroll).toHaveBeenCalledTimes(1)
    })

    it('disables the menus of every card while a change runs', async () => {
      renderPanel({ manage: manage({ busy: true }), selectedBlockId: 'section-1' })
      await menuOf('cta', 'Élan CTA')
      expect(item('Duplicate')).toHaveAttribute('aria-disabled', 'true')
    })
  })

  it("links dnd-kit's instructions to each tile and saved card, the tile keeping its own refusal", () => {
    render(
      <DndContext accessibility={{ screenReaderInstructions: { draggable: 'Use Insert instead.' } }}>
        <LibraryPanel emailTree={tree} savedBlocks={[header]} selectedBlockId={null} onInsert={vi.fn()} onOpenSaveDialog={vi.fn()} />
      </DndContext>
    )
    const described = (element: Element | null) => {
      const id = element?.getAttribute('aria-describedby')
      return id ? document.getElementById(id)?.textContent : undefined
    }
    const tile = document.querySelector('[data-library-tile="mj-text"]')
    expect(described(tile)).toBe('Use Insert instead.')
    // Nothing selected: a content block cannot be inserted, which the tile says
    expect(tile).toHaveAttribute('aria-disabled', 'true')
    const handle = document.querySelector('[data-library-card] [data-library-drag-handle]')
    expect(described(handle)).toBe('Use Insert instead.')
    expect(handle).not.toHaveAttribute('tabindex')
  })
})
