import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SettingsSidebar, SETTINGS_SECTIONS, SettingsSection } from './SettingsSidebar'

// Menu keys are strings until the onClick handler casts them to SettingsSection, so a key that no
// longer matches a section compiles, renders, and routes to a page that will never resolve. These
// tests walk the menu the way a user does — clicking, not reading the source — so the cast is
// exercised rather than trusted.
const renderSidebar = () => {
  const onSectionChange = vi.fn<(section: SettingsSection) => void>()
  render(<SettingsSidebar activeSection="team" onSectionChange={onSectionChange} isOwner={true} />)
  return onSectionChange
}

describe('SettingsSidebar', () => {
  it('reports a valid section for every entry it renders', () => {
    const onSectionChange = renderSidebar()
    // isOwner={true} so the conditional danger-zone entry is in the menu too.
    const entries = screen.getAllByRole('menuitem')

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
})
