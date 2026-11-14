import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MessageColumnsSelector } from './MessageColumnsSelector'

describe('MessageColumnsSelector', () => {
  it('names its icon button', () => {
    render(
      <MessageColumnsSelector
        storageKey="columns"
        columns={[{ key: 'id', title: 'Message ID', visible: true }]}
        onColumnVisibilityChange={vi.fn()}
      />
    )
    expect(screen.getByRole('button', { name: 'Select columns' })).toBeInTheDocument()
  })

  it('is not described by its tooltip, which repeats its name', async () => {
    render(
      <MessageColumnsSelector
        storageKey="columns"
        columns={[{ key: 'id', title: 'Message ID', visible: true }]}
        onColumnVisibilityChange={vi.fn()}
      />
    )
    const button = screen.getByRole('button', { name: 'Select columns' })
    fireEvent.mouseEnter(button)
    expect(await screen.findByRole('tooltip')).toBeInTheDocument()
    expect(button).not.toHaveAttribute('aria-describedby')
  })
})
