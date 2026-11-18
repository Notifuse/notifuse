import { describe, it, expect } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SettingsSection, SettingsSectionsProvider } from './SettingsSections'

const Section = (props: Partial<React.ComponentProps<typeof SettingsSection>>) => (
  <SettingsSection sectionKey="mj-button:font" title="Font" summary="Inter · 15 px" {...props}>
    <input aria-label="Font family" />
  </SettingsSection>
)

describe('SettingsSection', () => {
  it('opens the first time when it is the first section or holds a set field', () => {
    const { unmount } = render(<Section first />)
    expect(screen.getByLabelText('Font family')).toBeInTheDocument()
    unmount()
    render(<Section setCount={2} />)
    expect(screen.getByLabelText('Font family')).toBeInTheDocument()
  })

  it('stays closed otherwise, with its summary and the count of set fields', () => {
    const { rerender } = render(<Section />)
    expect(screen.queryByLabelText('Font family')).toBeNull()
    expect(screen.getByText('Inter · 15 px')).toBeInTheDocument()
    rerender(
      <SettingsSectionsProvider>
        <Section setCount={0} />
      </SettingsSectionsProvider>
    )
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument()
  })

  it('opens and closes, and shows the count once closed', () => {
    render(<Section setCount={2} />)
    fireEvent.click(screen.getByRole('button', { expanded: true }))
    expect(screen.queryByLabelText('Font family')).toBeNull()
    expect(screen.getByLabelText('2 set')).toBeInTheDocument()
    expect(screen.getByText('Inter · 15 px')).toBeInTheDocument()
  })

  it("is a band in the Segmented's track colour and radius once closed, flat on the panel when open", () => {
    render(<Section setCount={2} />)
    const section = screen.getByTestId('settings-section')
    expect(section).not.toHaveClass('bg-primary-100')
    expect(section).not.toHaveClass('rounded-md')
    fireEvent.click(screen.getByRole('button', { expanded: true }))
    expect(section).toHaveClass('bg-primary-100', 'rounded-md', 'mx-2')
    // The count stays a pill on that colour: white
    expect(screen.getByLabelText('2 set')).toHaveClass('bg-surface')
  })

  it('remembers a toggle for the next block of the same type, through the editor', () => {
    const { rerender } = render(
      <SettingsSectionsProvider>
        <Section key="a" first />
      </SettingsSectionsProvider>
    )
    fireEvent.click(screen.getByRole('button', { expanded: true }))
    // The panel remounts for the next button
    rerender(
      <SettingsSectionsProvider>
        <Section key="b" first />
      </SettingsSectionsProvider>
    )
    expect(screen.queryByLabelText('Font family')).toBeNull()
  })

  it('keeps the state a section had when first shown, while its fields are set and reset', () => {
    const { rerender } = render(
      <SettingsSectionsProvider>
        <Section setCount={1} />
      </SettingsSectionsProvider>
    )
    expect(screen.getByLabelText('Font family')).toBeInTheDocument()
    // A reset brings the count back to 0: the section stays open
    rerender(
      <SettingsSectionsProvider>
        <Section setCount={0} />
      </SettingsSectionsProvider>
    )
    expect(screen.getByLabelText('Font family')).toBeInTheDocument()
  })
})
