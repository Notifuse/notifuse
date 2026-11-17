import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { generateMessageId } from '@lingui/message-utils/generateMessageId'
import type { ReactNode } from 'react'
import type { Workspace } from '../../services/api/workspace'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ params, children }: { params: Record<string, string>; children: ReactNode }) => (
    <a href={`/settings/${params.section}`}>{children}</a>
  )
}))

import { SendingCard } from './SendingCard'

const workspace = (settings: Record<string, string>): Workspace =>
  ({
    id: 'ws1',
    name: 'Acme',
    settings,
    integrations: [
      {
        id: 'ses',
        name: 'Main SES',
        type: 'email',
        email_provider: {
          kind: 'ses',
          senders: [
            {
              id: 's1',
              email: 'first@acme.test',
              name: 'First',
              is_default: false
            },
            {
              id: 's2',
              email: 'news@acme.test',
              name: 'News',
              is_default: true
            }
          ]
        }
      },
      {
        id: 'smtp',
        name: 'Relay',
        type: 'email',
        email_provider: {
          kind: 'smtp',
          senders: [
            {
              id: 's3',
              email: 'hello@acme.test',
              name: 'Hello',
              is_default: false
            }
          ]
        }
      }
    ]
  }) as unknown as Workspace

const renderCard = (ws: Workspace, canConfigure = true) =>
  render(
    <I18nProvider i18n={i18n}>
      <SendingCard workspace={ws} canConfigure={canConfigure} />
    </I18nProvider>
  )

describe('SendingCard', () => {
  it('is titled as a section, not as a state in progress', () => {
    i18n.load('fr', { [generateMessageId('Sending', 'section')]: 'Envoi', Sending: 'Envoi en cours' })
    i18n.activate('fr')
    try {
      renderCard(workspace({ marketing_email_provider_id: 'ses' }))
      expect(screen.getByText('Envoi')).toBeInTheDocument()
      expect(screen.queryByText('Envoi en cours')).not.toBeInTheDocument()
    } finally {
      i18n.activate('en')
    }
  })

  it('shows the provider and default sender of each use', () => {
    renderCard(
      workspace({
        marketing_email_provider_id: 'ses',
        transactional_email_provider_id: 'smtp'
      })
    )

    expect(screen.getByText('Amazon SES')).toBeInTheDocument()
    expect(screen.getByText('news@acme.test')).toBeInTheDocument()
    expect(screen.queryByText('first@acme.test')).not.toBeInTheDocument()
    expect(screen.getByText('SMTP')).toBeInTheDocument()
    // Without a default sender, the first one.
    expect(screen.getByText('hello@acme.test')).toBeInTheDocument()
    expect(screen.queryByText('Not configured')).not.toBeInTheDocument()
  })

  it('links a use without provider to the integrations settings', () => {
    renderCard(workspace({ marketing_email_provider_id: 'ses' }))

    expect(screen.getByText('Not configured')).toBeInTheDocument()
    expect(screen.getByText('Configure').closest('a')).toHaveAttribute('href', '/settings/integrations')
  })

  it('shows no configure link to a member who cannot change the workspace', () => {
    renderCard(workspace({}), false)

    expect(screen.getAllByText('Not configured')).toHaveLength(2)
    expect(screen.queryByText('Configure')).not.toBeInTheDocument()
  })

  it('says nothing can be sent without any provider, with one link to configure', () => {
    renderCard(workspace({}))

    expect(
      screen.getByText(/^Nothing can be sent until an email provider is chosen for marketing or transactional emails\./)
    ).toBeInTheDocument()
    expect(screen.getAllByText('Configure')).toHaveLength(1)
    expect(screen.getByText('Configure').closest('a')).toHaveAttribute('href', '/settings/integrations')
  })

  it('counts a role pointing to a deleted integration as none', () => {
    renderCard(workspace({ marketing_email_provider_id: 'gone' }))
    expect(screen.getByText(/^Nothing can be sent/)).toBeInTheDocument()
  })

  it('does not say nothing can be sent once one use has a provider', () => {
    renderCard(workspace({ transactional_email_provider_id: 'smtp' }))

    expect(screen.queryByText(/Nothing can be sent/)).not.toBeInTheDocument()
    expect(screen.getByText('Configure')).toBeInTheDocument()
  })

  it('writes « Not configured » in a readable grey', () => {
    renderCard(workspace({ marketing_email_provider_id: 'ses' }))

    expect(screen.getByText('Not configured')).toHaveClass('text-ink-3')
    expect(screen.getByText('Not configured')).not.toHaveClass('text-ink-4')
  })
})
