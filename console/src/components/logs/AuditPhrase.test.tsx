import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AuditPhrase } from './AuditPhrase'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderPhrase = (entry: { action: string; target_name?: string; target_id?: string }) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <AuditPhrase entry={entry} />
      </App>
    </I18nProvider>
  )

describe('AuditPhrase', () => {
  it('names the target in bold', () => {
    const { container } = renderPhrase({
      action: 'templates.create',
      target_name: 'Welcome',
      target_id: 't1'
    })
    expect(container).toHaveTextContent('Created the template Welcome')
    expect(container.querySelector('strong')).toHaveTextContent('Welcome')
  })

  it('shows the target id in the code typeface without a name', () => {
    const { container } = renderPhrase({ action: 'broadcasts.schedule', target_id: 'bc_42' })
    expect(container).toHaveTextContent('Scheduled the newsletter bc_42')
    expect(container.querySelector('strong code')).toHaveTextContent('bc_42')
  })

  it('says the target is unknown when the entry has none', () => {
    const { container } = renderPhrase({ action: 'workspaces.inviteMember' })
    expect(container).toHaveTextContent('Invited (unknown)')
  })

  it('writes an action without target as a plain sentence', () => {
    const { container } = renderPhrase({ action: 'contacts.export' })
    expect(container).toHaveTextContent('Exported contacts')
    expect(container.querySelector('strong')).toBeNull()
  })

  it('shows an action the console does not know by its route name', () => {
    renderPhrase({ action: 'contacts.merge', target_name: 'Ann' })
    expect(screen.getByText('contacts.merge').tagName).toBe('CODE')
  })
})
