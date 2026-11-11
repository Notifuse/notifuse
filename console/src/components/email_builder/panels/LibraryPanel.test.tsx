import React from 'react'
import { describe, it, expect, onTestFinished, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

  it('counts the blocks it shows', async () => {
    renderPanel()
    expect(screen.getByText('Saved blocks').parentElement).toHaveTextContent('Saved blocks (3)')
    await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'head')
    expect(screen.getByText('Saved blocks').parentElement).toHaveTextContent('Saved blocks (1)')
  })

  it('searches names whatever their case and accents', async () => {
    renderPanel()
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
      expect(card.className).toContain('border-[#7763F1]')
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

    it('disables the menus of every card while a change runs', async () => {
      renderPanel({ manage: manage({ busy: true }), selectedBlockId: 'section-1' })
      await menuOf('cta', 'Élan CTA')
      expect(item('Duplicate')).toHaveAttribute('aria-disabled', 'true')
    })
  })
})
