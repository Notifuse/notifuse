import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactNode } from 'react'
import { Settings } from 'lucide-react'
import { RailLayout, RailCard, RailSection, RailItem } from './RailLayout'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    params,
    ...rest
  }: {
    children: ReactNode
    to: string
    params?: Record<string, string>
  }) => (
    <a href={to.replace(/\$(\w+)/g, (_, name: string) => params?.[name] ?? '')} {...rest}>
      {children}
    </a>
  )
}))

describe('RailLayout', () => {
  it('renders the rail as a named navigation, with its title, its footer and the content', () => {
    render(
      <RailLayout title="Settings" rail={<span>items</span>} footer={<span>2.1 GB used</span>}>
        <p>content</p>
      </RailLayout>
    )
    const nav = screen.getByRole('navigation', { name: 'Settings' })
    expect(nav).toHaveTextContent('Settings')
    expect(nav).toHaveTextContent('items')
    expect(nav).toHaveTextContent('2.1 GB used')
    expect(screen.getByText('content').closest('[data-rail-content]')).not.toBeNull()
  })

  it('names the rail from ariaLabel when it has no title', () => {
    render(<RailLayout ariaLabel="Blog" rail={null}>x</RailLayout>)
    expect(screen.getByRole('navigation', { name: 'Blog' })).toBeInTheDocument()
  })

  it('keeps the rail on screen while the page scrolls, below the licence banner', () => {
    render(<RailLayout ariaLabel="Blog" rail={null}>x</RailLayout>)
    const nav = screen.getByRole('navigation', { name: 'Blog' })
    expect(nav.className).toMatch(/\bsticky\b/)
    expect(nav.style.top).toContain('--license-banner-height')
    expect(nav.style.height).toContain('--license-banner-height')
  })
})

describe('RailSection', () => {
  it('renders its title and an action reachable by its name', () => {
    const onClick = vi.fn()
    render(
      <RailSection title="Categories" action={{ label: 'New Category', icon: <span>+</span>, onClick }}>
        <span>child</span>
      </RailSection>
    )
    expect(screen.getByText('Categories')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'New Category' }))
    expect(onClick).toHaveBeenCalled()
  })

  it('shows its label in a tooltip on hover and a pointer cursor', async () => {
    render(
      <RailSection title="Lists" action={{ label: 'New list', icon: <span>+</span>, onClick: vi.fn() }}>
        <span>child</span>
      </RailSection>
    )
    const button = screen.getByRole('button', { name: 'New list' })
    expect(button.className).toMatch(/\bcursor-pointer\b/)
    fireEvent.mouseEnter(button)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('New list')
  })

  it('disables its action with the reason as tooltip', async () => {
    const onClick = vi.fn()
    render(
      <RailSection
        title="Folders"
        action={{
          label: 'New folder',
          icon: <span>+</span>,
          onClick,
          primary: true,
          disabledReason: 'New folder (Read-only mode)'
        }}
      >
        <span>child</span>
      </RailSection>
    )
    const button = screen.getByRole('button', { name: 'New folder' })
    expect(button).toBeDisabled()
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
    fireEvent.mouseEnter(button.parentElement as HTMLElement)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('New folder (Read-only mode)')
  })
})

describe('RailItem', () => {
  it('is a link marked as the current page when active', () => {
    render(
      <RailItem
        label="Team"
        to="/console/workspace/$workspaceId/settings/$section"
        params={{ workspaceId: 'w1', section: 'team' }}
        active
      />
    )
    const link = screen.getByRole('link', { name: 'Team' })
    expect(link).toHaveAttribute('href', '/console/workspace/w1/settings/team')
    expect(link).toHaveAttribute('aria-current', 'page')
  })

  it('is a button without a destination, current when active', () => {
    const onClick = vi.fn()
    const { rerender } = render(<RailItem label="General" onClick={onClick} />)
    const button = screen.getByRole('button', { name: 'General' })
    expect(button).not.toHaveAttribute('aria-current')
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalled()
    rerender(<RailItem label="General" onClick={onClick} active />)
    expect(screen.getByRole('button', { name: 'General' })).toHaveAttribute('aria-current', 'page')
  })

  it('puts its trailing controls beside the main element, never inside it', () => {
    render(
      <RailItem
        label="News"
        onClick={() => {}}
        trailing={<button aria-label="Edit">e</button>}
        trailingMode="actions"
      />
    )
    const main = screen.getByRole('button', { name: 'News' })
    const edit = screen.getByRole('button', { name: 'Edit' })
    expect(main.contains(edit)).toBe(false)
    expect(document.querySelectorAll('button button, a button, button a')).toHaveLength(0)
    // Actions show on hover and on a keyboard focus, and stay reachable by keyboard (no display:none).
    // A mouse click's focus leaves them hidden once the pointer has gone: focus-visible, not focus-within.
    expect(edit.parentElement!.className).toMatch(/opacity-0/)
    expect(edit.parentElement!.className).toMatch(/group-has-\[:focus-visible\]:opacity-100/)
    expect(main.parentElement!.className).toMatch(/has-\[:focus-visible\]:ring-2/)
    expect(document.querySelector('[data-rail-item]')!.className).not.toMatch(/focus-within/)
  })

  it('puts its leading controls before the main element, never inside it', () => {
    const onToggle = vi.fn()
    const onClick = vi.fn()
    render(
      <RailItem
        label="docs"
        onClick={onClick}
        leading={<button aria-label="Expand docs" onClick={onToggle} />}
      />
    )
    const main = screen.getByRole('button', { name: 'docs' })
    const expand = screen.getByRole('button', { name: 'Expand docs' })
    expect(main.contains(expand)).toBe(false)
    expect(expand.compareDocumentPosition(main) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    fireEvent.click(expand)
    expect(onToggle).toHaveBeenCalled()
    expect(onClick).not.toHaveBeenCalled()
  })

  it('always shows a trailing indicator', () => {
    render(<RailItem label="News" onClick={() => {}} trailing={<span>!</span>} />)
    expect(screen.getByText('!').parentElement!.className).not.toMatch(/opacity-0/)
  })

  it('indents by depth, formats its count and takes the danger tone', () => {
    render(
      <>
        <RailItem label="Child" depth={1} count={1204} onClick={() => {}} />
        <RailItem label="Danger Zone" tone="danger" icon={<Settings />} onClick={() => {}} />
      </>
    )
    const child = screen.getByRole('button', { name: /Child/ })
    expect(child.closest('[data-rail-item]')!.className).toMatch(/\bpl-5\.5\b/)
    expect(screen.getByText('1,204')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Danger Zone' }).className).toMatch(/text-danger-fg/)
  })
})

describe('RailCard', () => {
  it('is a named navigation landmark holding the rail', () => {
    render(
      <RailCard ariaLabel="Blog">
        <span>All posts</span>
      </RailCard>
    )
    const nav = screen.getByRole('navigation', { name: 'Blog' })
    expect(nav).toHaveAttribute('data-rail-card')
    expect(nav).toHaveTextContent('All posts')
  })

  it('puts its footer at the bottom of the card, after the rail', () => {
    render(
      <RailCard ariaLabel="Folders" footer={<span>media · 2 MB</span>}>
        <span>Root</span>
      </RailCard>
    )
    const nav = screen.getByRole('navigation', { name: 'Folders' })
    const footer = screen.getByText('media · 2 MB')
    expect(nav.contains(footer)).toBe(true)
    expect(
      screen.getByText('Root').compareDocumentPosition(footer) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })
})
