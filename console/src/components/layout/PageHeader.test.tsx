import { describe, it, expect, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { PageHeader } from './PageHeader'

describe('PageHeader', () => {
  afterEach(() => i18n.activate('en'))

  it('titles a page with an h1 and a section with an h2', () => {
    const { rerender } = render(<PageHeader title="Broadcasts" />)
    expect(screen.getByRole('heading', { level: 1, name: 'Broadcasts' })).toBeInTheDocument()
    rerender(<PageHeader title="RSS / Feeds" level="section" />)
    expect(screen.getByRole('heading', { level: 2, name: 'RSS / Feeds' })).toBeInTheDocument()
  })

  it("marks its heading data-unreset, so antd's reset margin does not lift it above the count", () => {
    render(<PageHeader title="Contacts" count={3} />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveAttribute('data-unreset')
  })

  it('formats its counter in the console language', () => {
    const { rerender } = render(<PageHeader title="Contacts" count={18342} />)
    expect(screen.getByText('18,342')).toBeInTheDocument()
    i18n.load('fr', {})
    i18n.activate('fr')
    rerender(<PageHeader title="Contacts" count={18343} />)
    // The default normalizer would turn the narrow no-break space into a plain one.
    expect(screen.getByText('18\u202f343', { normalizer: (text) => text })).toBeInTheDocument()
  })

  it('never wraps its actions: the title truncates instead', () => {
    render(<PageHeader title="A long title" actions={<button>Create</button>} />)
    const actions = screen.getByRole('button', { name: 'Create' }).parentElement!
    expect(actions.className).toMatch(/\bshrink-0\b/)
    expect(actions.className).toMatch(/\bflex-nowrap\b/)
    expect(screen.getByRole('heading').className).toMatch(/\btruncate\b/)
  })

  it('renders the status, the leading element, the description and the meta line', () => {
    render(
      <PageHeader
        title="Live"
        leading={<button>Back</button>}
        status={<span>3 live now</span>}
        description="Who is on the site"
        meta={['Sent on 4 Nov', 'List Newsletter']}
      />
    )
    expect(screen.getByRole('button', { name: 'Back' })).toBeInTheDocument()
    expect(screen.getByText('3 live now')).toBeInTheDocument()
    expect(screen.getByText('Who is on the site')).toBeInTheDocument()
    expect(screen.getByText('Sent on 4 Nov')).toBeInTheDocument()
    expect(screen.getByText('List Newsletter')).toBeInTheDocument()
    // One separator dot between two meta items.
    expect(document.querySelectorAll('[data-meta-separator]')).toHaveLength(1)
  })

  it('passes its className to the container', () => {
    const { container } = render(<PageHeader title="T" className="border-t" />)
    expect((container.firstChild as HTMLElement).className).toMatch(/\bborder-t\b/)
  })
})
