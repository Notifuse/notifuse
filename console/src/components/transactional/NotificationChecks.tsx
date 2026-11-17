import type { ReactNode } from 'react'
import { Button } from 'antd'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { CircleAlert, CircleCheck, CircleMinus, Info, ListChecks } from 'lucide-react'
import { Mono, Panel } from '../layout'
import type { Workspace } from '../../services/api/types'
import { TEMPLATE_CATEGORY_LABELS, isTemplateCategory } from '../templates/templateCategories'
import type { TemplateState } from './PreviewPanel'
import { resolveSenderInfo, transactionalProvider } from './senderInfo'
import { API_ID_PATTERN, sameTracking, type NotificationDraft } from './notificationForm'
import { TRANSACTIONAL_ID_MAX_LENGTH } from '../../constants/limits'

/** ok: met. warn: missing, the notification cannot send. info: a change. off: a choice that turns something off. */
export type CheckTone = 'ok' | 'warn' | 'info' | 'off'

export interface NotificationCheck {
  key: string
  tone: CheckTone
  label: string
  why?: ReactNode
  action?: ReactNode
}

/** Whether the identifier typed is still free, read with transactional.get once typing stops. */
export type IdStatus = 'unknown' | 'checking' | 'free' | 'taken'

export interface NotificationChecksProps {
  workspace: Workspace
  /** The notification exists: the checks read as its state rather than as a pre-flight. */
  editing: boolean
  draft: NotificationDraft
  baseline: NotificationDraft
  idStatus: IdStatus
  /** The chosen template; undefined while none is chosen. */
  template: TemplateState | undefined
  /** The template is set by an integration: nothing here can change it. */
  templateLocked: boolean
  /** Brings a field into view and focuses it. */
  onShowField: (path: string[]) => void
}

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const TONE_ICON: Record<CheckTone, { className: string; icon: ReactNode }> = {
  ok: { className: 'text-success-fg', icon: <CircleCheck size={14} aria-hidden="true" /> },
  warn: { className: 'text-warning-fg', icon: <CircleAlert size={14} aria-hidden="true" /> },
  info: { className: 'text-primary-700', icon: <Info size={14} aria-hidden="true" /> },
  off: { className: 'text-ink-3', icon: <CircleMinus size={14} aria-hidden="true" /> }
}

/**
 * What the notification needs to send, as the sheet reads it: a name and an identifier (before it
 * is created), its template, and a provider with a sender. The provider and sender are those of the
 * sheet's « Sent from » (`transactionalProvider`, `resolveSenderInfo`): without them a send fails.
 */
export function NotificationChecks(props: NotificationChecksProps) {
  const { t, i18n } = useLingui()
  const { workspace, editing, draft, baseline, idStatus, template, templateLocked, onShowField } = props
  const checks: NotificationCheck[] = []

  const link = (label: string, path: string[]) => (
    <Button type="link" size="small" onClick={() => onShowField(path)}>
      {label}
    </Button>
  )
  const settingsLink = (
    <Button
      type="link"
      size="small"
      href={`/console/workspace/${workspace.id}/settings/integrations`}
      target="_blank"
      rel="noopener noreferrer"
    >
      {t`Settings`}
    </Button>
  )

  if (!editing) {
    const name = draft.name.trim()
    const id = draft.id.trim()
    const validId = id !== '' && API_ID_PATTERN.test(id) && id.length <= TRANSACTIONAL_ID_MAX_LENGTH
    if (!name || !id) {
      checks.push({
        key: 'identity',
        tone: 'warn',
        label: t`Name and API identifier`,
        why: t`Required to create the notification`,
        action: link(t`Add`, name ? ['id'] : ['name'])
      })
    } else if (!validId) {
      // Typed but refused: the field says why, the check sends to it.
      checks.push({
        key: 'identity',
        tone: 'warn',
        label: t`Name and API identifier`,
        why: t`The API identifier is not valid`,
        action: link(t`Edit`, ['id'])
      })
    } else if (idStatus === 'taken') {
      checks.push({
        key: 'identity',
        tone: 'warn',
        label: t`Name and API identifier`,
        why: t`${id} is already used by another notification`,
        action: link(t`Edit`, ['id'])
      })
    } else {
      checks.push({
        key: 'identity',
        tone: 'ok',
        label: t`Name and API identifier`,
        why: idStatus === 'free' ? t`${id} is free` : undefined
      })
    }
  }

  const templatePath = ['channels', 'email', 'template_id']
  const choose = templateLocked ? undefined : link(t`Choose`, templatePath)
  const loaded = template?.status === 'ok' ? template.template : undefined
  if (!template) {
    checks.push({
      key: 'template',
      tone: 'warn',
      label: t`No email template`,
      why: editing ? t`Emails can't be sent until you choose one` : t`Required to create the notification`,
      action: choose
    })
  } else if (template.status === 'notFound') {
    checks.push({ key: 'template', tone: 'warn', label: t`Template not found`, action: choose })
  } else if (template.status === 'error') {
    checks.push({ key: 'template', tone: 'warn', label: t`Email template`, why: t`Couldn't load` })
  } else if (template.status === 'forbidden') {
    checks.push({
      key: 'template',
      tone: 'ok',
      label: t`Email template`,
      why: <Mono>{draft.channels.email.template_id}</Mono>
    })
  } else if (loaded?.deleted_at) {
    checks.push({
      key: 'template',
      tone: 'warn',
      label: t`Deleted template`,
      why: t`This notification still sends with it`,
      action: choose
    })
  } else {
    const category = loaded && isTemplateCategory(loaded.category) ? i18n._(TEMPLATE_CATEGORY_LABELS[loaded.category]) : undefined
    checks.push({
      key: 'template',
      tone: 'ok',
      label: t`Email template`,
      why: loaded ? (category ? `${loaded.name} · ${category}` : loaded.name) : undefined
    })
  }

  const provider = transactionalProvider(workspace)
  if (!provider) {
    checks.push({
      key: 'sender',
      tone: 'warn',
      label: t`No transactional provider`,
      why: t`Emails can't be sent until you choose one`,
      action: settingsLink
    })
  } else if (loaded) {
    const { sender } = resolveSenderInfo(workspace, loaded)
    const providerName = provider.name
    const fromTemplate = Boolean(sender && loaded.email?.sender_id && sender.id === loaded.email.sender_id)
    const email = sender?.email ?? ''
    checks.push(
      sender
        ? {
            key: 'sender',
            tone: 'ok',
            label: t`Sender`,
            why: fromTemplate ? t`${email}, the template's, on ${providerName}` : t`${email}, the provider's default`
          }
        : {
            key: 'sender',
            tone: 'warn',
            label: t`No sender`,
            why: t`${providerName} has no default sender`,
            action: settingsLink
          }
    )
  } else {
    // No template read yet: the provider and the sender a send uses without one.
    const { sender } = resolveSenderInfo(workspace, undefined)
    const providerName = provider.name
    const email = sender?.email ?? ''
    checks.push(
      sender
        ? {
            key: 'sender',
            tone: 'ok',
            label: t`Transactional provider`,
            why: t`${providerName} · default sender ${email}`
          }
        : {
            key: 'sender',
            tone: 'warn',
            label: t`Transactional provider`,
            why: t`${providerName} has no default sender`,
            action: settingsLink
          }
    )
  }

  if (editing) {
    const applies = t`Applies to the next sends`
    if (draft.channels.email.template_id !== baseline.channels.email.template_id) {
      const section = t`Email template`
      checks.push({ key: 'changed-template', tone: 'info', label: t`Changed: ${section}`, why: applies })
    }
    if (!sameTracking(draft.tracking_settings, baseline.tracking_settings)) {
      const section = t`Tracking`
      checks.push({ key: 'changed-tracking', tone: 'info', label: t`Changed: ${section}`, why: applies })
    }
  }

  if (draft.tracking_settings.tracking_mode === 'disabled') {
    checks.push({ key: 'tracking', tone: 'off', label: t`Tracking disabled`, why: t`Links are never rewritten` })
  }

  const warnings = checks.filter((check) => check.tone === 'warn').length
  const aside =
    warnings === 0
      ? editing
        ? t`All set`
        : t`Ready`
      : editing
        ? plural(warnings, { one: '# to check', other: '# to check' })
        : plural(warnings, { one: '# missing', other: '# missing' })

  return (
    <Panel
      title={editing ? t({ message: 'Sending', context: 'section' }) : t`Before you create`}
      icon={<ListChecks size={14} aria-hidden="true" />}
      extra={<span className="text-xs text-ink-3">{aside}</span>}
    >
      <ul data-testid="notification-checks" className="m-0 flex list-none flex-col p-0">
        {checks.map((check) => (
          <li
            key={check.key}
            data-check={check.key}
            data-tone={check.tone}
            className="flex items-start gap-2 border-t border-line-soft py-1.5 first:border-t-0 first:pt-0 last:pb-0"
          >
            <span className={`mt-0.5 flex shrink-0 ${TONE_ICON[check.tone].className}`}>{TONE_ICON[check.tone].icon}</span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="text-sm text-ink">{check.label}</span>
              {check.why ? <span className="min-w-0 text-xs break-words text-ink-3">{check.why}</span> : null}
            </span>
            {check.action ?? null}
          </li>
        ))}
      </ul>
    </Panel>
  )
}
