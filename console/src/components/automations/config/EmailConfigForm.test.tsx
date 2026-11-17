import type React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { EmailConfigForm } from './EmailConfigForm'
import type { Workspace } from '../../../services/api/types'

// The selector queries the API; here it only needs to show what it was given.
vi.mock('../../templates/TemplateSelectorInput', () => ({
  default: ({
    value,
    canReadTemplates,
    subtitle,
    listStatusNotice,
    sendsWith
  }: {
    value?: string | null
    canReadTemplates?: boolean
    subtitle?: string
    listStatusNotice?: boolean
    sendsWith?: unknown
  }) => (
    <span
      data-testid="template-selector"
      data-sends-with={JSON.stringify(sendsWith ?? null)}
      data-can-read-templates={String(canReadTemplates)}
      data-subtitle={subtitle}
      data-list-status-notice={String(!!listStatusNotice)}
    >
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

  // The window says what the template's category changes for the automation's list, and what
  // the template is for.
  it('asks the selector to explain what the category changes for the list', () => {
    renderForm()
    const selector = screen.getByTestId('template-selector')
    expect(selector.dataset.listStatusNotice).toBe('true')
    expect(selector.dataset.subtitle).toBe('The email this step sends')
  })
})

// The window names the sender the step sends from: its own integration, else the marketing provider.
describe('EmailConfigForm sending provider', () => {
  it.each([
    { config: { template_id: 'tpl-welcome' }, sendsWith: 'marketing' },
    { config: { template_id: 'tpl-welcome', integration_id: 'int-smtp' }, sendsWith: { integrationId: 'int-smtp' } }
  ])('tells the selector the step sends with $sendsWith', ({ config, sendsWith }) => {
    render(
      <I18nProvider i18n={i18n}>
        <EmailConfigForm config={config} onChange={vi.fn()} workspaceId="ws1" workspace={workspace} />
      </I18nProvider>
    )
    expect(JSON.parse(screen.getByTestId('template-selector').dataset.sendsWith ?? '')).toEqual(sendsWith)
  })
})

describe('EmailConfigForm layout', () => {
  const smtp = { id: 'int-smtp', name: 'Demo SMTP', type: 'email', email_provider: { kind: 'smtp' } }
  const renderWith = (ws: Workspace, preview?: React.ReactNode) =>
    render(
      <I18nProvider i18n={i18n}>
        <EmailConfigForm
          config={{ template_id: 'tpl-welcome' }}
          onChange={vi.fn()}
          workspaceId="ws1"
          workspace={ws}
          preview={preview}
        />
      </I18nProvider>
    )

  it('shows the preview between the template field and the integration field', () => {
    renderWith({ id: 'ws1', integrations: [smtp] } as unknown as Workspace, <div data-testid="preview" />)
    const selector = screen.getByTestId('template-selector')
    const preview = screen.getByTestId('preview')
    const integration = screen.getByText('Email Integration')
    expect(selector.compareDocumentPosition(preview) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(preview.compareDocumentPosition(integration) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('names the provider the workspace default falls back to', () => {
    renderWith({
      id: 'ws1',
      integrations: [smtp],
      settings: { marketing_email_provider_id: 'int-smtp' }
    } as unknown as Workspace)
    expect(screen.getByText('Use workspace default · Demo SMTP')).toBeInTheDocument()
  })

  it('says only "Use workspace default" without a marketing provider', () => {
    renderWith({ id: 'ws1', integrations: [smtp], settings: {} } as unknown as Workspace)
    expect(screen.getByText('Use workspace default')).toBeInTheDocument()
  })

  it('keeps the help of each field in its label tooltip', async () => {
    renderWith({ id: 'ws1', integrations: [smtp] } as unknown as Workspace)
    expect(screen.queryByText('Select the email template to send')).not.toBeInTheDocument()
    const icons = document.querySelectorAll('.ant-form-item-tooltip')
    expect(icons).toHaveLength(2)
    fireEvent.mouseEnter(icons[0])
    expect(await screen.findByText('Select the email template to send')).toBeInTheDocument()
  })
})
