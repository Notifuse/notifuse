import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { EmailConfigForm } from './EmailConfigForm'
import type { Workspace } from '../../../services/api/types'

// The selector queries the API; here it only needs to show what it was given.
vi.mock('../../templates/TemplateSelectorInput', () => ({
  default: ({ value, canReadTemplates }: { value?: string | null; canReadTemplates?: boolean }) => (
    <span data-testid="template-selector" data-can-read-templates={String(canReadTemplates)}>
      template:{value ?? 'none'}
    </span>
  )
}))

const workspace = { id: 'ws1', integrations: [] } as unknown as Workspace

const renderForm = (canReadTemplates?: boolean) =>
  render(
    <I18nProvider i18n={i18n}>
      <EmailConfigForm
        config={{ template_id: 'tpl-welcome' }}
        onChange={vi.fn()}
        workspaceId="ws1"
        workspace={workspace}
        canReadTemplates={canReadTemplates}
      />
    </I18nProvider>
  )

// Without Templates read the email node's template shows the stored ID, locked.
describe('EmailConfigForm template', () => {
  it.each([
    { given: false, passed: 'false' },
    { given: true, passed: 'true' },
    { given: undefined, passed: 'true' }
  ])('gives the selector Templates read $given', ({ given, passed }) => {
    renderForm(given)

    const selector = screen.getByTestId('template-selector')
    expect(selector).toHaveTextContent('template:tpl-welcome')
    expect(selector.getAttribute('data-can-read-templates')).toBe(passed)
  })
})
