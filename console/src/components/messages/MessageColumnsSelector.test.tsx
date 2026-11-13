import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
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
})
