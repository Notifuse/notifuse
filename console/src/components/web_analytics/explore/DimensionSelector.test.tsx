import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'

vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => ({ customDimensionLabels: {} }) }))

import { DimensionSelector } from './DimensionSelector'

describe('DimensionSelector', () => {
  it('adds the dimension picked in the menu after the selected ones', async () => {
    const onChange = vi.fn()
    render(<DimensionSelector value={['device']} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /Add dimension/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Browser' }))
    expect(onChange).toHaveBeenCalledWith(['device', 'browser'])
  })

  it('removes a dimension from its chip', () => {
    const onChange = vi.fn()
    render(<DimensionSelector value={['device', 'browser']} onChange={onChange} />)
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove dimension' })[0])
    expect(onChange).toHaveBeenCalledWith(['browser'])
  })
})
