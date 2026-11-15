import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MetaLine } from './MetaLine'

describe('MetaLine', () => {
  it('renders nothing without an item', () => {
    const { container: none } = render(<MetaLine />)
    expect(none).toBeEmptyDOMElement()
    const { container: empty } = render(<MetaLine items={[null, undefined, false]} />)
    expect(empty).toBeEmptyDOMElement()
  })

  it('separates the items it shows with a dot, skipping the empty ones', () => {
    render(<MetaLine items={['Created today', null, 'France', false, <b key="v">v2</b>]} />)
    expect(screen.getByText('Created today')).toBeInTheDocument()
    expect(screen.getByText('France')).toBeInTheDocument()
    expect(screen.getByText('v2')).toBeInTheDocument()
    const separators = document.querySelectorAll('[data-meta-separator]')
    expect(separators).toHaveLength(2)
    separators.forEach((separator) => expect(separator).toHaveAttribute('aria-hidden', 'true'))
  })

  it('draws no dot before a single item', () => {
    render(<MetaLine items={['Only']} />)
    expect(document.querySelectorAll('[data-meta-separator]')).toHaveLength(0)
  })
})
