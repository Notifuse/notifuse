import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LibraryPanel } from './LibraryPanel'
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
    expect(within(ctaCard).getByRole('button', { name: 'Insert' })).toBeDisabled()

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
    renderPanel({ highlightedId: 'header' })
    const headerCard = document.querySelector('[data-library-card="header"]') as HTMLElement
    expect(within(headerCard).getByText('New')).toBeInTheDocument()
    expect(screen.getAllByText('New')).toHaveLength(1)
  })
})
