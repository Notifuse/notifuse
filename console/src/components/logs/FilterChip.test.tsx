import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FilterChip } from './FilterChip'

const items = [
  { key: 'a', label: 'Alpha' },
  { key: 'b', label: 'Beta' }
]

describe('FilterChip', () => {
  it('shows its label and value', () => {
    render(<FilterChip label="Status" value="Failed" menu={{ items }} />)
    const chip = screen.getByRole('button', { name: /Status Failed/ })
    expect(chip).toBeInTheDocument()
    expect(chip).toHaveAttribute('aria-haspopup')
  })

  it('opens its menu on a click and reports the choice', async () => {
    const onClick = vi.fn()
    render(<FilterChip label="Status" value="All" menu={{ items, onClick }} />)
    await userEvent.click(screen.getByRole('button', { name: /Status All/ }))
    await userEvent.click(await screen.findByText('Beta'))
    expect(onClick).toHaveBeenCalledWith(expect.objectContaining({ key: 'b' }))
  })

  it('opens with Enter from the keyboard', async () => {
    render(<FilterChip label="Status" value="All" menu={{ items }} />)
    screen.getByRole('button', { name: /Status All/ }).focus()
    await userEvent.keyboard('{Enter}')
    expect(await screen.findByText('Alpha')).toBeInTheDocument()
  })

  it('clears with its cross without opening', async () => {
    const onClear = vi.fn()
    render(<FilterChip label="List" value="Customers" menu={{ items }} onClear={onClear} />)
    fireEvent.click(screen.getByRole('button', { name: 'Remove List filter' }))
    expect(onClear).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  })

  it('opens popover content instead of a menu', async () => {
    render(<FilterChip label="Target ID" value="Any" content={<div>Type an id</div>} />)
    await userEvent.click(screen.getByRole('button', { name: /Target ID Any/ }))
    expect(await screen.findByText('Type an id')).toBeInTheDocument()
  })

  it('has a dashed variant for adding a filter', () => {
    render(<FilterChip label="+ Filter" dashed menu={{ items }} />)
    expect(screen.getByRole('button', { name: '+ Filter' })).toHaveAttribute('data-dashed', 'true')
  })
})
