import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { ColorField } from './ColorField'

const open = (label = 'Background') => fireEvent.click(screen.getByRole('button', { name: label }))

describe('ColorField', () => {
  it('offers the email colours above the palette, and a click writes the colour once', async () => {
    const onChange = vi.fn()
    render(<ColorField aria-label="Background" emailColors={['#6a5af9', '#111827']} onChange={onChange} />)
    open()
    const inEmail = await screen.findByRole('group', { name: 'In this email' })
    const palette = screen.getByRole('group', { name: 'Palette' })
    // « In this email » comes first in the popover.
    expect(inEmail.compareDocumentPosition(palette) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(within(inEmail).getAllByRole('button').map((b) => b.getAttribute('aria-label'))).toEqual([
      '#6A5AF9',
      '#111827'
    ])

    fireEvent.click(within(inEmail).getByRole('button', { name: '#6A5AF9' }))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('#6a5af9')
  })

  it('hides « In this email » when the email has no colour yet', async () => {
    render(<ColorField aria-label="Background" onChange={vi.fn()} />)
    open()
    expect(await screen.findByRole('group', { name: 'Palette' })).toBeInTheDocument()
    expect(screen.queryByRole('group', { name: 'In this email' })).toBeNull()
  })

  it('keeps the Flowbite palette of the former picker', async () => {
    const onChange = vi.fn()
    render(<ColorField aria-label="Background" onChange={onChange} />)
    open()
    const palette = await screen.findByRole('group', { name: 'Palette' })
    expect(within(palette).getAllByRole('button')).toHaveLength(82)
    fireEvent.click(within(palette).getByRole('button', { name: '#E02424' }))
    expect(onChange).toHaveBeenCalledWith('#e02424')
  })

  it('writes transparent when cleared, from the field or from the popover', async () => {
    const onChange = vi.fn()
    render(<ColorField aria-label="Background" value="#ff0000" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Clear Background' }))
    expect(onChange).toHaveBeenLastCalledWith('transparent')

    open()
    fireEvent.click(await screen.findByRole('button', { name: 'None' }))
    expect(onChange).toHaveBeenLastCalledWith('transparent')
    expect(onChange).toHaveBeenCalledTimes(2)
  })

  it('shows transparent as no colour, with the placeholder and no clear button', () => {
    render(<ColorField aria-label="Background" value="transparent" placeholder="Transparent" onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Background' })).toHaveTextContent('Transparent')
    expect(screen.queryByRole('button', { name: 'Clear Background' })).toBeNull()
  })

  it('shows the inherited colour greyed while the block sets none', () => {
    const { rerender } = render(<ColorField aria-label="Text" inherited="#374151" onChange={vi.fn()} />)
    const text = screen.getByText('#374151')
    expect(text).toHaveClass('text-ink-4')
    expect(screen.getByTestId('color-swatch').style.background).toBe('rgb(55, 65, 81)')
    expect(screen.queryByRole('button', { name: 'Clear Text' })).toBeNull()

    rerender(<ColorField aria-label="Text" value="#ff0000" inherited="#374151" onChange={vi.fn()} />)
    expect(screen.getByText('#FF0000')).toHaveClass('text-ink')
    expect(screen.queryByText('#374151')).toBeNull()
  })

  it('shows the swatch alone in compact mode, without hex text nor clear button', () => {
    render(<ColorField compact aria-label="Border color" value="#123456" onChange={vi.fn()} />)
    expect(screen.queryByText('#123456')).toBeNull()
    expect(screen.queryByRole('button', { name: /Clear/ })).toBeNull()
    expect(screen.getByRole('button', { name: 'Border color' })).toBeInTheDocument()
  })
})
