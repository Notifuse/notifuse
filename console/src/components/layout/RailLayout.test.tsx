import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactNode } from 'react'
import { Settings } from 'lucide-react'
import { RailLayout, RailSection, RailItem } from './RailLayout'

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
    // Actions show on hover and focus, and stay reachable by keyboard (no display:none).
    expect(edit.parentElement!.className).toMatch(/opacity-0/)
    expect(edit.parentElement!.className).toMatch(/group-focus-within:opacity-100/)
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
