import { useState, type ReactNode } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Skeleton } from 'antd'
import { Check, ChevronDown, ChevronUp, Plus } from 'lucide-react'
import { Trans, useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Mono, Panel, Pill, useNumberFormat } from '../layout'
import { emailProviders } from '../integrations/EmailProviders'
import { canCreateTransactionalNotification, type StoredPermissions } from '../../services/api/permissions'
import { parseRootEmails } from '../../services/api/auth'
import { emailTemplatesQuery, membersQuery } from '../../services/api/sharedQueries'
import type { Workspace } from '../../services/api/workspace'
import { ButtonLink } from './ButtonLink'
import { contactsTotalQuery, latestContactsQuery } from './dashboardQueries'
import { toNumber } from './dashboardKpisModel'
import {
  GET_STARTED_GRACE_DAYS,
  deriveGetStarted,
  getStartedVisibility,
  shouldCollapse,
  type GetStartedModel,
  type StepId,
  type StepState
} from './getStartedModel'
import { ImportContactsAction } from './ImportContactsAction'
import { useGetStartedHidden } from './useGetStartedHidden'

interface GetStartedChecklistProps {
  workspace: Workspace
  permissions: StoredPermissions
  /** Only an owner (or a platform admin) can connect a provider and choose its uses. */
  role: 'root' | 'owner' | 'member' | undefined
  user: { id: string; email: string }
  /** The latest message's creation; null when there is none, undefined while it loads. */
  latestAt: string | null | undefined
}

interface StepCopy {
  title: string
  description: ReactNode
  /** Where in the console the step is done. */
  where?: string
  /** The step's actions; `primary` is set on the first one of the next step. */
  actions: (primary: boolean) => ReactNode
}

const DAY = 86_400_000

/**
 * The dashboard's "Get started": the steps from an empty workspace to its first email, each ticked
 * from the workspace's data. Shown to owners only, who can do every step; it folds as steps are done
 * and goes once they all are, or once hidden.
 */
export function GetStartedChecklist({ workspace, permissions, role, user, latestAt }: GetStartedChecklistProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const navigate = useNavigate()
  const ws = workspace.id
  const timezone = workspace.settings?.timezone || 'UTC'
  const [hidden, hide] = useGetStartedHidden(user.id, ws)
  const [showDone, setShowDone] = useState(false)

  const isOwner = role === 'owner' || role === 'root'
  const can = (resource: keyof StoredPermissions, verb: 'read' | 'write' = 'read') =>
    Boolean(permissions[resource]?.[verb])
  // Owners hold every right; the reads are checked anyway, so no step can answer 403.
  const canAct = isOwner && can('contacts') && can('templates')

  // Whether the checklist may show at all, before any of its own requests: a workspace that sends
  // and is past its first days never asks for its templates or members.
  const created = Date.parse(workspace.created_at)
  const recent = Number.isFinite(created) && Date.now() - created < GET_STARTED_GRACE_DAYS * DAY
  const candidate = canAct && !hidden && (latestAt === null || (typeof latestAt === 'string' && recent))

  const latestContacts = useQuery({ ...latestContactsQuery(ws), enabled: candidate })
  const contactsTotal = useQuery({ ...contactsTotalQuery(ws, timezone), enabled: candidate })
  const templates = useQuery({ ...emailTemplatesQuery(ws), enabled: candidate })
  // A member's rights already load the members list under this key; a platform admin's skip it, so
  // the checklist asks for it here. It never holds the checklist back: the team step waits unticked.
  const members = useQuery({
    ...membersQuery(ws, user.id),
    enabled: candidate && role === 'root' && can('workspace')
  })

  const rootEmails = parseRootEmails(window.ROOT_EMAIL)
  const failed = latestContacts.isError || templates.isError
  const model: GetStartedModel | undefined =
    latestContacts.data && templates.data
      ? deriveGetStarted({
          workspace,
          rootEmails,
          me: user,
          latestContacts: latestContacts.data.contacts ?? [],
          contactsTotal: contactsTotal.data ? toNumber(contactsTotal.data.count) : undefined,
          templates: templates.data.templates?.length ?? 0,
          latestAt,
          members: members.data?.members
        })
      : undefined

  const visibility = getStartedVisibility({
    canAct: candidate,
    hidden,
    model,
    latestAt,
    createdAt: workspace.created_at,
    now: Date.now()
  })
  // A failed request leaves the dashboard as it was, rather than a checklist that cannot load.
  if (visibility === 'hidden' || failed) return null

  const title = t`Get started`

  if (visibility === 'loading' || !model) {
    return (
      <Panel title={title}>
        <div data-testid="get-started-loading">
          <Skeleton active title={false} paragraph={{ rows: 4 }} />
        </div>
      </Panel>
    )
  }

  const settings = (section: 'integrations' | 'web-analytics' | 'team') =>
    ({ to: '/console/workspace/$workspaceId/settings/$section', params: { workspaceId: ws, section } }) as const

  const workspaceName = workspace.name
  const copy: Record<StepId, StepCopy> = {
    workspace: {
      title: t`Create the workspace`,
      description: t`${workspaceName} is ready, with you as its first contact in the Test List.`,
      actions: () => null
    },
    provider: {
      title: t`Connect an email provider`,
      description: t`Nothing can be sent until one is connected: Amazon SES, Postmark, Mailgun, SendGrid, SparkPost, Mailjet or any SMTP server.`,
      where: t`Settings › Integrations`,
      actions: (primary) => (
        <ButtonLink {...settings('integrations')} primary={primary}>
          {t`Connect an email provider`}
        </ButtonLink>
      )
    },
    sender: {
      title: t`Choose the sender and what it sends`,
      description: t`Give the provider a default From address, and use it for both marketing and transactional emails.`,
      where: t`Settings › Integrations`,
      actions: (primary) => (
        <ButtonLink {...settings('integrations')} primary={primary}>
          {t`Open integrations`}
        </ButtonLink>
      )
    },
    contacts: {
      title: t`Add your contacts`,
      description: (
        <Trans>
          Import a CSV into a list, or send contacts from your app with the <Mono>contacts.upsert</Mono> API.
        </Trans>
      ),
      where: t`Contacts`,
      actions: (primary) =>
        can('contacts', 'write') ? (
          <ImportContactsAction
            workspaceId={ws}
            canReadLists={can('lists')}
            type={primary ? 'primary' : 'default'}
            size="middle"
            // The drawer is the layout's, so it stays open over the Contacts page it fills.
            beforeOpen={() =>
              void navigate({ to: '/console/workspace/$workspaceId/contacts', params: { workspaceId: ws } })
            }
          >
            {t`Import contacts`}
          </ImportContactsAction>
        ) : null
    },
    template: {
      title: t`Pick a template`,
      description: t`Start from a model in the gallery, then edit it in the visual editor. Every email, broadcast or transactional, uses one.`,
      where: t`Content › Templates`,
      actions: (primary) =>
        can('templates', 'write') ? (
          <ButtonLink
            to="/console/workspace/$workspaceId/templates"
            params={{ workspaceId: ws }}
            search={{ new: true }}
            primary={primary}
          >
            {t`Browse the gallery`}
          </ButtonLink>
        ) : null
    },
    send: {
      title: t`Send your first email`,
      description: (
        <Trans>
          A broadcast goes to a list; a transactional email is sent by your app through{' '}
          <Mono>transactional.send</Mono>. Try it on the Test List first: only you will receive it.
        </Trans>
      ),
      where: t`Broadcasts or Transactional`,
      actions: (primary) => (
        <>
          {canCreateTransactionalNotification(permissions) ? (
            <ButtonLink
              to="/console/workspace/$workspaceId/transactional-notifications"
              params={{ workspaceId: ws }}
              search={{ new: true }}
            >
              {t`New notification`}
            </ButtonLink>
          ) : null}
          {can('broadcasts', 'write') ? (
            <ButtonLink
              to="/console/workspace/$workspaceId/broadcasts"
              params={{ workspaceId: ws }}
              search={{ compose: 'new' }}
              primary={primary}
            >
              {t`New broadcast`}
            </ButtonLink>
          ) : null}
        </>
      )
    },
    website: {
      title: t`Track your website`,
      description: t`Add the script to your site to count visits and see which emails bring people back.`,
      actions: () =>
        isOwner || can('web_analytics', 'write') ? (
          <Link {...settings('web-analytics')}>{t`Set up web analytics`}</Link>
        ) : null
    },
    team: {
      title: t`Invite your team`,
      description: t`Add teammates, or create an API key for your app.`,
      actions: () => (isOwner ? <Link {...settings('team')}>{t`Open Team & API keys`}</Link> : null)
    }
  }

  const pills = (step: StepState): string[] => {
    const detail = step.detail
    switch (step.id) {
      case 'workspace':
        return [t`Workspace created`]
      case 'provider': {
        const integration = detail?.integration
        const kind = integration?.email_provider?.kind
        const name = emailProviders.find((info) => info.kind === kind)?.name ?? integration?.name ?? ''
        return [t`${name} connected`]
      }
      case 'sender': {
        const marketing = detail?.senders?.marketing ?? ''
        const transactional = detail?.senders?.transactional ?? ''
        if (marketing === transactional) return [t`Sends as ${marketing}`]
        return [t`Marketing sends as ${marketing}`, t`Transactional sends as ${transactional}`]
      }
      case 'contacts': {
        const count = detail?.count ?? 0
        const shown = int(count)
        return [t`${plural(count, { one: `${shown} contact`, other: `${shown} contacts` })}`]
      }
      case 'template': {
        const count = detail?.count ?? 0
        const shown = int(count)
        return [t`${plural(count, { one: `${shown} template`, other: `${shown} templates` })}`]
      }
      case 'send':
        return [t`First email sent`]
      case 'website':
        return [t`Website tracked`]
      case 'team': {
        const result: string[] = []
        const teammates = detail?.teammates ?? 0
        const apiKeys = detail?.apiKeys ?? 0
        if (teammates > 0) result.push(t`${plural(teammates, { one: '# teammate', other: '# teammates' })}`)
        if (apiKeys > 0) result.push(t`${plural(apiKeys, { one: '# API key', other: '# API keys' })}`)
        return result
      }
    }
  }

  const collapsed = shouldCollapse(model) && !showDone
  const steps = model.steps
  const number = (id: StepId) => steps.filter((step) => step.required).findIndex((step) => step.id === id) + 1

  const row = (step: StepState) => {
    const text = copy[step.id]
    const next = model.next === step.id
    const actions = step.done ? null : text.actions(next)
    return (
      <li
        key={step.id}
        data-step={step.id}
        className={`relative flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-line-soft px-3.5 ${
          next ? 'bg-primary-50 py-3' : 'py-2.5'
        }`}
      >
        {next ? <span aria-hidden="true" className="absolute inset-y-0 left-0 w-0.5 bg-primary-500" /> : null}
        <StepMarker step={step} next={next} number={number(step.id)} />
        <div className="min-w-0 flex-1 basis-80">
          <div className={`text-base ${step.done ? 'font-medium text-ink-2' : 'font-semibold text-ink'}`}>
            {text.title}
          </div>
          <div className={`text-sm ${next ? 'text-ink-2' : 'text-ink-3'}`}>{text.description}</div>
          {text.where && !step.done ? <div className="mt-0.5 text-xs text-ink-3">{text.where}</div> : null}
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {step.done ? (
            <Pill tone="success">{t`Done`}</Pill>
          ) : (
            actions
          )}
        </div>
      </li>
    )
  }

  const groupLabel = (label: string, first = false) => (
    <li
      key={label}
      className={`bg-subtle px-3.5 pt-2 pb-1 text-overline font-semibold tracking-wider text-ink-3 uppercase ${
        first ? '' : 'border-t border-line-soft'
      }`}
    >
      {label}
    </li>
  )

  const byId = (id: StepId) => steps.find((step) => step.id === id) as StepState
  const optional = steps.filter((step) => !step.required)
  const doneSteps = steps.filter((step) => step.done)

  const toggle = (
    <Button
      type="link"
      size="small"
      aria-expanded={!collapsed}
      icon={collapsed ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronUp size={14} aria-hidden="true" />}
      iconPlacement="end"
      onClick={() => setShowDone(collapsed)}
    >
      {collapsed ? t`Show done steps` : t`Hide done steps`}
    </Button>
  )

  let body: ReactNode
  if (collapsed) {
    const openOptional = optional.filter((step) => !step.done)
    body = (
      <>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5 bg-subtle px-3.5 py-2">
          {doneSteps.flatMap((step) =>
            pills(step).map((label) => (
              <Pill key={`${step.id}-${label}`} tone="success" icon={<Check aria-hidden="true" />}>
                {label}
              </Pill>
            ))
          )}
          <span className="ml-auto">{toggle}</span>
        </div>
        <ol className="m-0 list-none p-0">{steps.filter((step) => step.required && !step.done).map(row)}</ol>
        {openOptional.length > 0 ? (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 border-t border-line-soft px-3.5 py-2 text-sm text-ink-3">
            <span className="text-overline font-semibold tracking-wider uppercase">{t`Optional`}</span>
            {openOptional.map((step) => (
              <span key={step.id} data-step={step.id} className="inline-flex items-center gap-1.5">
                <span className="text-ink-2">{copy[step.id].title}</span>
                {copy[step.id].actions(false)}
              </span>
            ))}
          </div>
        ) : null}
      </>
    )
  } else {
    body = (
      <>
        {shouldCollapse(model) ? <div className="flex justify-end bg-subtle px-3.5 py-1">{toggle}</div> : null}
        <ol className="m-0 list-none p-0">
          {groupLabel(t`Set up sending`, true)}
          {(['workspace', 'provider', 'sender'] as StepId[]).map((id) => row(byId(id)))}
          {groupLabel(t`Reach your audience`)}
          {(['contacts', 'template', 'send'] as StepId[]).map((id) => row(byId(id)))}
          {groupLabel(t`Optional`)}
          {optional.map(row)}
        </ol>
      </>
    )
  }

  const { done, total } = model
  const progress = Math.round((done / total) * 100)

  return (
    <Panel
      title={title}
      flush
      extra={
        <>
          <span className="text-xs text-ink-2 tabular-nums">
            {t`${plural(done, { one: `# of ${total} done`, other: `# of ${total} done` })}`}
          </span>
          {/* Left out on a phone, where it would squeeze the title; the count says the same. */}
          <span aria-hidden="true" className="hidden h-1.5 w-[120px] overflow-hidden rounded-full bg-fill sm:block">
            <span className="block h-full bg-primary-500" style={{ width: `${progress}%` }} />
          </span>
          <Button type="text" size="small" onClick={hide}>
            {t`Hide`}
          </Button>
        </>
      }
      footer={
        <>
          <span>
            {can('blog') ? (
              <Trans>
                Also available: <Link to="/console/workspace/$workspaceId/blog" params={{ workspaceId: ws }}>a blog</Link>{' '}
                for your site.
              </Trans>
            ) : null}
          </span>
          <span>{t`Steps tick themselves as you go.`}</span>
        </>
      }
    >
      {body}
    </Panel>
  )
}

function StepMarker({ step, next, number }: { step: StepState; next: boolean; number: number }) {
  const { t } = useLingui()
  const base = 'flex size-5.5 shrink-0 items-center justify-center rounded-full'
  if (step.done) {
    return (
      <span role="img" aria-label={t`Done`} className={`${base} bg-success-fg text-surface`}>
        <Check size={13} strokeWidth={3} aria-hidden="true" />
      </span>
    )
  }
  if (!step.required) {
    return (
      <span aria-hidden="true" className={`${base} border border-dashed border-gray-300 text-ink-4`}>
        <Plus size={12} />
      </span>
    )
  }
  return (
    <span
      role="img"
      aria-label={next ? t`Next step` : t`To do`}
      className={`${base} text-2xs font-semibold ${
        next ? 'border-2 border-primary-500 bg-surface text-primary-700' : 'border border-gray-300 text-ink-3'
      }`}
    >
      {number}
    </span>
  )
}
