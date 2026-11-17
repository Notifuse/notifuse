import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ContactColumnsSelector } from './ContactColumnsSelector'

const columns = [
  { key: 'first_name', title: 'First Name', visible: true },
  { key: 'country', title: 'Country', visible: false }
]

const checklist = () => document.querySelector('[data-columns-checklist]')
const isShown = (el: Element | null) => !!el && !el.closest('.ant-popover-hidden, .ant-tooltip-hidden')
const hoverLabel = () => screen.queryByText('Select columns')

describe('ContactColumnsSelector', () => {
  it('closes on Escape and gives the focus back to its button', async () => {
    render(<ContactColumnsSelector columns={columns} onColumnVisibilityChange={vi.fn()} />)
    const button = screen.getByRole('button', { name: 'Select columns' })
    fireEvent.click(button)
    await waitFor(() => expect(isShown(checklist())).toBe(true))
    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })
    await waitFor(() => expect(isShown(checklist())).toBe(false))
    expect(document.activeElement).toBe(button)
  })

  it('hides its hover label while the columns are open', async () => {
    render(<ContactColumnsSelector columns={columns} onColumnVisibilityChange={vi.fn()} />)
    const button = screen.getByRole('button', { name: 'Select columns' })
    fireEvent.mouseEnter(button)
    await waitFor(() => expect(isShown(hoverLabel())).toBe(true))
    fireEvent.click(button)
    await waitFor(() => expect(isShown(checklist())).toBe(true))
    await waitFor(() => expect(isShown(hoverLabel())).toBe(false))
  })

  it('reports a column switched on', async () => {
    const onChange = vi.fn()
    render(<ContactColumnsSelector columns={columns} onColumnVisibilityChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Select columns' }))
    fireEvent.click(await screen.findByRole('checkbox', { name: 'Country' }))
    expect(onChange).toHaveBeenCalledWith('country', true)
  })
})
