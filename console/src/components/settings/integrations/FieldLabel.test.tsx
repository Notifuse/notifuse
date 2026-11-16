import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { OptionalLabel, RequiredLabel } from './FieldLabel'

i18n.loadAndActivate({ locale: 'en', messages: {} })

describe('field labels', () => {
  // The marks are CSS: the text a label query and htmlFor read stays the field's name.
  it('marks a required field without changing its text', () => {
    render(
      <label htmlFor="host">
        <RequiredLabel>Host</RequiredLabel>
      </label>
    )
    const mark = screen.getByText('Host')
    expect(mark.textContent).toBe('Host')
    expect(mark.className).toContain("after:content-['*']")
  })

  it('marks an optional field with the translated word', () => {
    render(
      <I18nProvider i18n={i18n}>
        <OptionalLabel>EHLO hostname</OptionalLabel>
      </I18nProvider>
    )
    const mark = screen.getByText('EHLO hostname')
    expect(mark.textContent).toBe('EHLO hostname')
    expect(mark).toHaveAttribute('data-optional', 'optional')
  })
})
