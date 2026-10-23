import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WebPageDrawer } from './WebPageDrawer'

describe('WebPageDrawer', () => {
  it('frames and links an https page', () => {
    render(<WebPageDrawer open url="https://www.example.com/pricing" onClose={() => {}} />)

    expect(screen.getByTitle('Website preview').getAttribute('src')).toBe(
      'https://www.example.com/pricing'
    )
    expect(screen.getByRole('link', { name: /Open in new tab/ }).getAttribute('href')).toBe(
      'https://www.example.com/pricing'
    )
  })

  it.each([
    'javascript:window.__probe=1',
    'data:text/html,<p>x</p>',
    `${window.location.origin}/console/`
  ])('neither frames nor links %s', (url) => {
    render(<WebPageDrawer open url={url} onClose={() => {}} />)

    expect(screen.queryByTitle('Website preview')).toBeNull()
    expect(screen.queryByRole('link', { name: /Open in new tab/ })).toBeNull()
  })
})
