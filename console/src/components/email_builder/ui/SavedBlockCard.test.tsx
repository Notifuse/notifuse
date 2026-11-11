import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SavedBlockCard } from './SavedBlockCard'
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

  it('carries a "New" tag when highlighted', () => {
    renderCard({ highlighted: true })
    expect(screen.getByText('New')).toBeInTheDocument()
  })
})
