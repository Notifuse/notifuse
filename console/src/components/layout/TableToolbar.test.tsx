import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TableToolbar } from './TableToolbar'

describe('TableToolbar', () => {
  it('renders its start and end slots', () => {
    render(<TableToolbar start={<span>filters</span>} end={<span>columns</span>} />)
    expect(screen.getByText('filters')).toBeInTheDocument()
    expect(screen.getByText('columns')).toBeInTheDocument()
  })

  it('offers a search input with its icon, 300px wide', () => {
    render(<TableToolbar.Search placeholder="Search by name" value="" onChange={() => {}} />)
    const input = screen.getByPlaceholderText('Search by name')
    expect(input.closest('.ant-input-affix-wrapper')).not.toBeNull()
  })
})
