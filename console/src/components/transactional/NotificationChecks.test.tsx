import { describe, expect, it, vi } from 'vitest'
import { render, screen, within, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { NotificationChecks, type NotificationChecksProps } from './NotificationChecks'
import { draftOf, type NotificationDraft } from './notificationForm'

const provider = (senders: unknown[]) => ({
  id: 'ses',
  name: 'Amazon SES',
  type: 'email',
  email_provider: { kind: 'ses', senders }
})
const workspaceWith = (senders: unknown[]) =>
  ({
    id: 'ws1',
    settings: { transactional_email_provider_id: 'ses' },
    integrations: [provider(senders)]
  }) as unknown as Workspace
const workspace = workspaceWith([{ id: 'd', email: 'hello@acme.com', name: 'Acme', is_default: true }])

const template = (extra: Partial<Template> = {}) =>
  ({ id: 'tpl', name: 'Receipt', category: 'transactional', email: { subject: 'Hi' }, ...extra }) as Template

const draft = (extra: Partial<NotificationDraft> = {}): NotificationDraft => ({
  ...draftOf(undefined, workspace),
  name: 'Receipt',
  id: 'receipt',
  channels: { email: { template_id: 'tpl' } },
  ...extra
})

function renderChecks(props: Partial<NotificationChecksProps> = {}) {
  const onShowField = vi.fn()
  const base = draft()
  render(
    <I18nProvider i18n={i18n}>
      <NotificationChecks
        workspace={workspace}
        editing={false}
        draft={base}
        baseline={base}
        idStatus="unknown"
        template={{ status: 'ok', template: template() }}
        templateLocked={false}
        onShowField={onShowField}
        {...props}
      />
    </I18nProvider>
  )
  return { onShowField }
}

const item = (key: string) => document.querySelector(`[data-check="${key}"]`) as HTMLElement

describe('NotificationChecks', () => {
  it('is ready when the notification has a name, an identifier, a template and a sender', () => {
    renderChecks({ idStatus: 'free' })
    expect(screen.getByText('Before you create')).toBeInTheDocument()
    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(within(item('identity')).getByText('receipt is free')).toBeInTheDocument()
    expect(within(item('sender')).getByText("hello@acme.com, the provider's default")).toBeInTheDocument()
  })

  it('brings the missing field into view', () => {
    const { onShowField } = renderChecks({ draft: draft({ channels: { email: { template_id: '' } } }), template: undefined })
    expect(screen.getByText('1 missing')).toBeInTheDocument()
    fireEvent.click(within(item('template')).getByRole('button', { name: 'Choose' }))
    expect(onShowField).toHaveBeenCalledWith(['channels', 'email', 'template_id'])
  })

  it('says a notification without a template cannot send, rather than cannot be created, once it exists', () => {
    const none = draft({ channels: { email: { template_id: '' } } })
    renderChecks({ editing: true, draft: none, baseline: draft(), template: undefined })
    expect(within(item('template')).getByText("Emails can't be sent until you choose one")).toBeInTheDocument()
    expect(within(item('template')).queryByText('Required to create the notification')).toBeNull()
    expect(screen.getByText('1 to check')).toBeInTheDocument()
  })

  it('offers no Choose for a template an integration sets', () => {
    renderChecks({ template: { status: 'notFound' }, templateLocked: true })
    expect(within(item('template')).getByText('Template not found')).toBeInTheDocument()
    expect(within(item('template')).queryByRole('button')).toBeNull()
  })

  it('warns when neither the template nor the provider gives a sender', () => {
    render(
      <I18nProvider i18n={i18n}>
        <NotificationChecks
          workspace={workspaceWith([{ id: 'x', email: 'x@acme.com', is_default: false }])}
          editing
          draft={draft()}
          baseline={draft()}
          idStatus="unknown"
          template={{ status: 'ok', template: template() }}
          templateLocked={false}
          onShowField={vi.fn()}
        />
      </I18nProvider>
    )
    expect(within(item('sender')).getByText('No sender')).toBeInTheDocument()
    expect(within(item('sender')).getByText('Amazon SES has no default sender')).toBeInTheDocument()
    expect(screen.getByText('1 to check')).toBeInTheDocument()
    // The notification exists: no identity check, and the checks read as its state
    expect(item('identity')).toBeNull()
    expect(screen.getByText('Sending')).toBeInTheDocument()
  })

  it('says All set for an existing notification with nothing to check, and names a template change', () => {
    renderChecks({ editing: true, draft: draft({ channels: { email: { template_id: 'other' } } }) })
    expect(screen.getByText('All set')).toBeInTheDocument()
    expect(within(item('changed-template')).getByText('Changed: Email template')).toBeInTheDocument()
    expect(within(item('changed-template')).getByText('Applies to the next sends')).toBeInTheDocument()
  })

  it('says a template that could not be read is not a missing one', () => {
    renderChecks({ template: { status: 'forbidden' } })
    expect(item('template')).toHaveAttribute('data-tone', 'ok')
    expect(within(item('template')).getByText('tpl')).toBeInTheDocument()
  })
})
