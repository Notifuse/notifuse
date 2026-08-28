import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { zapierProvider, getZapierIcon } from './ZapierProviders'

describe('zapierProvider', () => {
  it('serves the mark from under the console base path', () => {
    // vite.config.ts sets base: '/console/', so a bare '/zapier.svg' resolves against the site
    // root and 404s. The failure is a broken image in the integration catalogue, which no test
    // that only looks for an <img> would notice.
    render(<>{zapierProvider.getIcon()}</>)

    expect(screen.getByAltText('Zapier')).toHaveAttribute('src', '/console/zapier.svg')
  })

  it('sizes the mark by name or by pixel count', () => {
    const { rerender } = render(<>{zapierProvider.getIcon('', 'large')}</>)
    expect(screen.getByAltText('Zapier').style.height).toBe('18px')

    rerender(<>{zapierProvider.getIcon('', 14)}</>)
    expect(screen.getByAltText('Zapier').style.height).toBe('14px')
  })

  it('passes a class through, which is how the dropdown entry is sized', () => {
    render(<>{zapierProvider.getIcon('h-6 w-12 object-contain mr-1')}</>)

    expect(screen.getByAltText('Zapier')).toHaveClass('h-6', 'w-12', 'object-contain', 'mr-1')
  })

  it('names the integration and its type', () => {
    expect(zapierProvider.name).toBe('Zapier')
    expect(zapierProvider.type).toBe('zapier')
  })

  it('renders the same mark through the standalone helper', () => {
    render(<>{getZapierIcon('large')}</>)

    expect(screen.getByAltText('Zapier')).toHaveAttribute('src', '/console/zapier.svg')
  })
})
