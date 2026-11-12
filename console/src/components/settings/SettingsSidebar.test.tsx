import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import {
  SettingsSidebar,
  SETTINGS_SECTIONS,
  SETTINGS_GROUPS,
  SettingsSection
} from './SettingsSidebar'

// Menu keys are strings until the onClick handler casts them to SettingsSection, so a key that no
// longer matches a section compiles, renders, and routes to a page that will never resolve. These
// tests walk the menu the way a user does — clicking, not reading the source — so the cast is
// exercised rather than trusted.
const renderSidebar = ({
  isOwner = true,
  isRoot = true,
  active = 'general' as SettingsSection
} = {}) => {
  const onSectionChange = vi.fn<(section: SettingsSection) => void>()
  render(
    <SettingsSidebar
      activeSection={active}
      onSectionChange={onSectionChange}
      isOwner={isOwner}
      isRoot={isRoot}
    />
  )
  return onSectionChange
}

describe('SETTINGS_GROUPS', () => {
  // Written out rather than derived: this list is what makes a settings URL valid.
  it('groups the sections, each once, in menu order', () => {
    expect(SETTINGS_GROUPS.map((group) => [...group.sections])).toEqual([
      ['general', 'team'],
      ['integrations', 'webhooks', 'smtp-bridge'],
      ['custom-fields', 'blog', 'web-analytics'],
      ['audit-logs', 'licence', 'danger-zone']
    ])
    expect(SETTINGS_SECTIONS).toEqual([
      'general',
      'team',
      'integrations',
      'webhooks',
      'smtp-bridge',
      'custom-fields',
      'blog',
      'web-analytics',
      'audit-logs',
      'licence',
      'danger-zone'
    ])
  })
})

describe('SettingsSidebar', () => {
  it('groups the entries under Workspace, Sending, Content and Security', () => {
    renderSidebar()
    for (const title of ['Workspace', 'Sending', 'Content', 'Security']) {
      expect(screen.getByText(title)).toBeInTheDocument()
    }
    const labels = screen.getAllByRole('button').map((button) => button.textContent)
    expect(labels).toEqual([
      'General',
      'Team & API keys',
      'Integrations',
      'Webhooks',
      'SMTP bridge',
      'Custom fields',
      'Blog',
      'Web analytics',
      'Audit log',
      'Licence',
      'Danger zone'
    ])
  })

  it('marks the active section as the current page, and draws Danger zone in the danger tone', () => {
    renderSidebar({ active: 'team' })
    expect(screen.getByRole('button', { name: 'Team & API keys' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('button', { name: 'Integrations' })).not.toHaveAttribute('aria-current')
    expect(screen.getByRole('button', { name: 'Danger zone' }).className).toContain(
      'text-danger-fg'
    )
  })

  it('reports a valid section for every entry it renders', () => {
    const onSectionChange = renderSidebar()
    const entries = screen.getAllByRole('button')
    entries.forEach((entry) => fireEvent.click(entry))
    const reported = onSectionChange.mock.calls.map(([section]) => section)
    expect(reported).toHaveLength(entries.length)
    expect(reported.filter((section) => !SETTINGS_SECTIONS.includes(section))).toEqual([])
  })

  it('reports the section of the entry that was clicked', () => {
    const onSectionChange = renderSidebar()
    fireEvent.click(screen.getByText('Integrations'))
    expect(onSectionChange).toHaveBeenCalledWith('integrations')
  })

  // Retention and deletion are an owner's decisions; the licence covers the deployment, root only.
  it('offers Audit log and Danger zone to owners, Licence to root', () => {
    renderSidebar({ isOwner: true, isRoot: false })
    expect(screen.getByText('Audit log')).toBeInTheDocument()
    expect(screen.getByText('Danger zone')).toBeInTheDocument()
    expect(screen.queryByText('Licence')).not.toBeInTheDocument()
  })

  it('offers Licence alone in Security to a root member who is not owner', () => {
    renderSidebar({ isOwner: false, isRoot: true })
    expect(screen.getByText('Security')).toBeInTheDocument()
    expect(screen.getByText('Licence')).toBeInTheDocument()
    expect(screen.queryByText('Audit log')).not.toBeInTheDocument()
  })

  it('drops the Security group for a member who is neither owner nor root', () => {
    renderSidebar({ isOwner: false, isRoot: false })
    expect(screen.queryByText('Security')).not.toBeInTheDocument()
    expect(screen.queryByText('Audit log')).not.toBeInTheDocument()
    expect(screen.queryByText('Danger zone')).not.toBeInTheDocument()
    expect(within(document.body).getByText('General')).toBeInTheDocument()
  })
})
