import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MenuEntryLabel } from './notificationParts'

describe('MenuEntryLabel', () => {
  it('writes the reason under the label, without hover', () => {
    const { container } = render(<MenuEntryLabel label="Delete" reason="You need Transactional write permission" />)
    expect(screen.getByText('Delete')).toBeInTheDocument()
    expect(screen.getByText('You need Transactional write permission')).toBeInTheDocument()
    expect(container.textContent).toBe('DeleteYou need Transactional write permission')
  })

  it('shows the label alone without a reason, or with an empty one', () => {
    const { container, rerender } = render(<MenuEntryLabel label="Delete" />)
    expect(container.textContent).toBe('Delete')
    rerender(<MenuEntryLabel label="Delete" reason="" />)
    expect(container.textContent).toBe('Delete')
  })
})
