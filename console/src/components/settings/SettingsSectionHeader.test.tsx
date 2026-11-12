import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SettingsSectionHeader } from './SettingsSectionHeader'

describe('SettingsSectionHeader', () => {
  it('titles a settings page with an h1 and its description', () => {
    render(<SettingsSectionHeader title="Team" description="Manage your workspace members" />)
    expect(screen.getByRole('heading', { level: 1, name: 'Team' })).toBeInTheDocument()
    expect(screen.getByText('Manage your workspace members')).toBeInTheDocument()
  })

  it('titles a section with an h2 under a rule', () => {
    const { container } = render(
      <SettingsSectionHeader title="RSS / Feeds" description="Feeds" level="section" />
    )
    expect(screen.getByRole('heading', { level: 2, name: 'RSS / Feeds' })).toBeInTheDocument()
    const root = container.firstElementChild!
    expect(root.className).toContain('border-t')
    expect(root.className).toContain('pt-4')
  })

  it('passes its class to the container', () => {
    const { container } = render(<SettingsSectionHeader title="Blog" description="d" className="mb-2" />)
    expect(container.firstElementChild!.className).toContain('mb-2')
    expect(container.firstElementChild!.className).not.toContain('border-t')
  })

  it('draws no divider', () => {
    const { container } = render(<SettingsSectionHeader title="Blog" description="d" />)
    expect(container.querySelector('.ant-divider')).toBeNull()
  })
})
