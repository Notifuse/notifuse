import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App, Button, Input, Result, Skeleton, Tooltip } from 'antd'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Send } from 'lucide-react'
import { FullDrawer, SaveBar } from '../../layout'
import { ApiError } from '../../../services/api/client'
import { broadcastApi, testEmailListProps, type Broadcast } from '../../../services/api/broadcast'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { templatesApi, type Template } from '../../../services/api/template'
import { canSendTestEmail } from '../../../services/api/permissions'
import type { Workspace } from '../../../services/api/types'
import type { UserPermissions } from '../../../services/api/workspace'
import dayjs from '../../../lib/dayjs'
import SendTemplateModal from '../../templates/SendTemplateModal'
import { BroadcastStatusPill } from '../BroadcastStatusPill'
import { workspaceTimezone } from '../access'
import { useComposeEstimate } from '../useAudienceEstimates'
import { getMarketingSenders } from '../variationSender'
import { AdvancedRow } from './AdvancedRow'
import { AudienceCard } from './AudienceCard'
import { ContentCard } from './ContentCard'
import { PreviewPanel } from './PreviewPanel'
import { SendSummary, errorCount, summaryChecks, useCheckText } from './SendSummary'
import { SendingCard } from './SendingCard'
import { TrackingCard } from './TrackingCard'
import { fromBroadcast, newDraft, withName, type ComposeValues } from './composeValues'
import { useAutosave, type SaveState } from './useAutosave'
import { useIssueText } from './useIssueText'
import { validateDraft } from './validateDraft'
import type { ComposeUpdate } from './types'

/** The statuses the composer edits; any other opens the detail instead. */
export const EDITABLE_STATUSES: Broadcast['status'][] = ['draft', 'scheduled']

export interface BroadcastComposeProps {
  workspace: Workspace
  permissions?: UserPermissions | null
  /** 'new', or the id of the broadcast; fixed for the life of the composer. */
  composeId: string
  /** Removes `compose` from the URL. */
  onClose: () => void
  /** A new broadcast was created: `?compose=new` becomes `?compose={id}`, without remounting. */
  onCreated: (id: string) => void
  /** Replaces `compose` with `broadcast`: after a send, or for a status the composer does not edit. */
  onOpenDetail: (id: string) => void
}

const isNotFound = (error: unknown) => error instanceof ApiError && error.status === 404

/** The one-screen composer of a broadcast, in a full-width drawer. */
export function BroadcastCompose(props: BroadcastComposeProps) {
  const { t } = useLingui()
  const { workspace, permissions, composeId, onClose, onOpenDetail } = props
  const ws = workspace.id
  const isNew = composeId === 'new'
  const canWrite = !!permissions?.broadcasts?.write

  const query = useQuery({
    queryKey: ['broadcast', ws, composeId],
    queryFn: async () => (await broadcastApi.get({ workspace_id: ws, id: composeId, with_templates: true })).broadcast,
    enabled: !isNew,
    retry: (count, error) => !isNotFound(error) && count < 2,
    // The composer owns this broadcast while it is open: its own answers keep the cache current.
    staleTime: Infinity
  })

  // The gate: only a member who may write edits, and only a draft or a scheduled broadcast. The
  // permissions are null until they load: a link to the composer waits for them.
  const known = permissions != null
  const status = query.data?.status
  const gated = known && ((!canWrite && !isNew) || (!!status && !EDITABLE_STATUSES.includes(status)))
  useEffect(() => {
    if (!known) return
    if (!canWrite && isNew) onClose()
    else if (gated) onOpenDetail(composeId)
  }, [known, canWrite, isNew, gated, composeId, onClose, onOpenDetail])

  if (!known || (!canWrite && isNew)) return null
  if (isNew || (query.data && !gated)) {
    return <ComposeEditor {...props} broadcast={query.data} />
  }
  return (
    <FullDrawer open onClose={onClose} backLabel={t`Broadcasts`} copyLink={false}>
      {query.isError ? (
        isNotFound(query.error) ? (
          <Result status="404" title={t`Broadcast not found`} extra={<Button onClick={onClose}>{t`Back to broadcasts`}</Button>} />
        ) : (
          <Result status="error" title={t`Couldn't load this broadcast`} extra={<Button onClick={() => query.refetch()}>{t`Retry`}</Button>} />
        )
      ) : (
        <Skeleton active paragraph={{ rows: 10 }} />
      )}
    </FullDrawer>
  )
}

interface ComposeEditorProps extends BroadcastComposeProps {
  broadcast?: Broadcast
}

function ComposeEditor({ workspace, permissions, broadcast, onClose, onCreated, onOpenDetail }: ComposeEditorProps) {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const ws = workspace.id
  const issueText = useIssueText()

  const initial = useMemo<ComposeValues>(() => {
    if (broadcast) return fromBroadcast(broadcast, workspace)
    const today = new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'short' }).format(new Date())
    return newDraft(t`Broadcast ${today}`, workspace)
    // Fixed at mount: the composer edits from here on.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const [values, setValues] = useState<ComposeValues>(initial)
  const [savedValues, setSavedValues] = useState<ComposeValues>(initial)
  const update: ComposeUpdate = (fn) => setValues((current) => fn(current))
  const [previewIndex, setPreviewIndex] = useState(0)
  const [testTemplate, setTestTemplate] = useState<Template | null>(null)
  const [sending, setSending] = useState(false)
  const [leftEditable, setLeftEditable] = useState(false)

  const scheduled = broadcast?.status === 'scheduled'
  const senderIds = getMarketingSenders(workspace).map((s) => s.id)
  const context = { senderIds }
  const canReadLists = !!permissions?.lists?.read
  const canReadSegments = !!permissions?.segments?.read
  // Without Templates read, templates.get answers 403: the template fields show the stored IDs.
  const canReadTemplates = !!permissions?.templates?.read

  const autosave = useAutosave({
    workspaceId: ws,
    values,
    context,
    broadcast,
    auto: !scheduled,
    onCreated: (created) => onCreated(created.id),
    onError: async () => {
      // A refusal that means the broadcast left draft or scheduled (sent from another tab).
      const id = autosave.id ?? broadcast?.id
      if (!id) return
      try {
        const { broadcast: fresh } = await broadcastApi.get({ workspace_id: ws, id, with_templates: true })
        if (!EDITABLE_STATUSES.includes(fresh.status)) {
          autosave.freeze()
          setLeftEditable(true)
        }
      } catch {
        // The refusal itself stays on screen.
      }
    }
  })
  const id = autosave.id
  useEffect(() => {
    if (autosave.state.kind === 'saved') setSavedValues(values)
    // Only a save moves what the server holds.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autosave.state])

  // C3: the latest version of each chosen template, read again when the tab regains focus, since
  // its subject is changed in the template editor of another tab.
  const templateIds = [...new Set(values.variants.map((v) => v.template_id).filter(Boolean))]
  const templateQueries = useQueries({
    queries: templateIds.map((templateId) => ({
      queryKey: ['template', ws, templateId],
      queryFn: () => templatesApi.get({ workspace_id: ws, id: templateId }),
      enabled: canReadTemplates,
      refetchOnWindowFocus: true
    }))
  })
  const templates: Record<string, Template | undefined> = {}
  templateIds.forEach((templateId, index) => {
    templates[templateId] = templateQueries[index]?.data?.template
  })

  const listsQuery = useQuery({
    queryKey: ['lists', ws, 'with_counts'],
    queryFn: () => listsApi.list({ workspace_id: ws, with_counts: true }),
    enabled: canReadLists
  })
  const segmentsQuery = useQuery({
    queryKey: ['segments', ws],
    queryFn: () => listSegments({ workspace_id: ws, with_count: true }),
    enabled: canReadSegments
  })
  const lists = listsQuery.data?.lists ?? []
  const segments = segmentsQuery.data?.segments ?? []

  const estimate = useComposeEstimate(
    ws,
    { list: values.list, segments: values.segments, exclude_unsubscribed: values.excludeUnsubscribed },
    permissions
  )

  const issue = validateDraft(values, context)
  const hasProvider = !!workspace.settings?.marketing_email_provider_id
  const checks = summaryChecks({ values, estimate, templates, hasMarketingProvider: hasProvider, senderIds, issue, booked: scheduled })
  const errors = errorCount(checks)
  const listName = lists.find((l) => l.id === values.list)?.name
  const segmentNames = values.segments.map((sid) => segments.find((s) => s.id === sid)?.name ?? sid)
  const checkText = useCheckText({ values, listName, segmentNames, workspaceId: ws })

  const changed = values !== savedValues
  // Leaving loses something: a refusal, an incomplete change, or a scheduled broadcast's edits.
  const losesChanges =
    autosave.state.kind === 'error' || (changed && issue !== null) || (scheduled && autosave.dirty)

  const closeReason = () => {
    if (autosave.state.kind === 'error') return autosave.state.message
    if (issue) return issueText(issue)
    return t`You have unsaved changes.`
  }

  const finishClose = () => {
    queryClient.invalidateQueries({ queryKey: ['broadcasts', ws] })
    onClose()
  }

  const handleClose = async () => {
    if (losesChanges) {
      const reason = closeReason()
      modal.confirm({
        title: t`Close without saving?`,
        content: t`The last changes are not saved: ${reason}`,
        okText: t`Close`,
        cancelText: t`Keep editing`,
        okButtonProps: { danger: true },
        onOk: finishClose
      })
      return
    }
    if (!scheduled && changed) {
      const { ok } = await autosave.flush()
      if (!ok) return
    }
    finishClose()
  }

  // Leaving by the browser or the sidebar: a pending save is sent first; what cannot be saved asks.
  const blocker = useBlocker({
    shouldBlockFn: async ({ current, next }) => {
      const nextSearch = next.search as { compose?: string }
      const leaving = next.pathname !== current.pathname || !nextSearch.compose
      if (!leaving) return false
      if (losesChanges) return true
      if (!scheduled && changed) {
        const { ok } = await autosave.flush()
        return !ok
      }
      return false
    },
    enableBeforeUnload: () => losesChanges || (changed && !scheduled),
    withResolver: true
  })
  const blockedOnce = useRef(false)
  useEffect(() => {
    if (blocker.status !== 'blocked' || blockedOnce.current) return
    blockedOnce.current = true
    const reason = closeReason()
    modal.confirm({
      title: t`Leave without saving?`,
      content: t`The last changes are not saved: ${reason}`,
      okText: t`Leave`,
      cancelText: t`Keep editing`,
      okButtonProps: { danger: true },
      onOk: () => {
        blockedOnce.current = false
        blocker.proceed?.()
      },
      onCancel: () => {
        blockedOnce.current = false
        blocker.reset?.()
      }
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [blocker.status])

  const previewVariant = (values.abEnabled ? values.variants : values.variants.slice(0, 1))[previewIndex]
  const previewTemplate = previewVariant?.template_id ? templates[previewVariant.template_id] : undefined
  const testAllowed = canSendTestEmail(permissions)
  const sendTest = async () => {
    if (!previewTemplate) return
    await autosave.flush()
    setTestTemplate(previewTemplate)
  }

  const send = async (now: boolean) => {
    if (!id) return
    setSending(true)
    const { ok } = await autosave.flush()
    if (!ok) {
      setSending(false)
      message.error(t`The broadcast could not be saved`)
      return
    }
    autosave.freeze()
    try {
      await broadcastApi.schedule(
        now
          ? { workspace_id: ws, id, send_now: true }
          : {
              workspace_id: ws,
              id,
              send_now: false,
              scheduled_date: values.scheduledDate,
              scheduled_time: values.scheduledTime,
              timezone: values.timezone,
              use_recipient_timezone: false
            }
      )
      const name = values.name.trim()
      if (now) message.success(t`Broadcast "${name}" sending started`)
      else {
        const date = values.scheduledDate ?? ''
        message.success(t`Scheduled for ${date}`)
      }
      queryClient.invalidateQueries({ queryKey: ['broadcast', ws, id] })
      queryClient.invalidateQueries({ queryKey: ['broadcasts', ws] })
      onOpenDetail(id)
    } catch (error) {
      autosave.unfreeze()
      // The server says why, demo mode included.
      message.error(error instanceof Error && error.message ? error.message : now ? t`Failed to send broadcast` : t`Failed to schedule broadcast`)
    } finally {
      setSending(false)
    }
  }

  const confirmSendNow = () => {
    const count = estimate.count
    modal.confirm({
      title:
        count === undefined
          ? t`Send this broadcast now?`
          : t`Send to ≈ ${plural(count, { one: '# recipient', other: '# recipients' })} now?`,
      content: (
        <ul data-unreset className="m-0 flex list-none flex-col gap-1 p-0 text-sm">
          {checks
            .filter((check) => check.level === 'ok')
            .map((check) => (
              <li key={check.key}>{checkText(check)}</li>
            ))}
        </ul>
      ),
      okText: t`Send now`,
      cancelText: t`Cancel`,
      onOk: () => send(true)
    })
  }

  const mainDisabledReason = !permissions?.broadcasts?.write
    ? t`You don't have write permission for broadcasts`
    : !hasProvider
    ? t`No marketing email provider`
    : errors > 0
      ? t`Fix ${plural(errors, { one: '# issue', other: '# issues' })} first`
      : !id
        ? t`Choose a list first`
        : undefined
  const mainButton = scheduled ? null : (
    <Tooltip title={mainDisabledReason}>
      <span className="inline-flex">
        <Button
          type="primary"
          icon={<Send size={14} />}
          loading={sending}
          disabled={!!mainDisabledReason || leftEditable}
          onClick={() => (values.sendChoice === 'now' ? confirmSendNow() : send(false))}
        >
          {values.sendChoice === 'now' ? t`Send now` : t`Schedule`}
        </Button>
      </span>
    </Tooltip>
  )

  const testReason = !id
    ? t`Choose a list first`
    : !testAllowed
      ? t`You need Templates read and Transactional write permissions to send test emails`
      : !previewTemplate
        ? t`Choose a template first`
        : undefined
  const testButton = (
    <Tooltip title={testReason}>
      <span className="inline-flex">
        <Button disabled={!!testReason} onClick={sendTest}>
          {t`Send test`}
        </Button>
      </span>
    </Tooltip>
  )

  const barTitle = (
    <>
      <div className="w-72">
        <Input
          aria-label={t`Broadcast name`}
          variant="borderless"
          size="small"
          value={values.name}
          onChange={(e) => update((v) => withName(v, e.target.value))}
        />
      </div>
      <BroadcastStatusPill broadcast={broadcast ?? ({ status: 'draft' } as Broadcast)} />
      {!scheduled ? <SaveStatus state={autosave.state} issueText={issueText} onRetry={() => void autosave.flush()} /> : null}
    </>
  )

  return (
    <FullDrawer
      open
      onClose={handleClose}
      backLabel={t`Broadcasts`}
      copyLink={false}
      barTitle={barTitle}
      topActions={
        <>
          {testButton}
          {mainButton}
        </>
      }
    >
      {leftEditable && id ? (
        <Alert
          type="warning"
          showIcon
          title={t`This broadcast is no longer a draft, so it can't be edited here.`}
          action={<Button size="small" onClick={() => onOpenDetail(id)}>{t`Open its page`}</Button>}
        />
      ) : null}
      <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
        <div className="flex min-w-0 flex-1 flex-col gap-3">
          <AudienceCard
            values={values}
            onChange={update}
            lists={lists}
            segments={segments}
            canReadLists={canReadLists}
            canReadSegments={canReadSegments}
            estimate={estimate}
          />
          <ContentCard
            workspace={workspace}
            values={values}
            onChange={update}
            templates={templates}
            previewIndex={previewIndex}
            onPreview={setPreviewIndex}
            estimateCount={estimate.count}
            canReadTemplates={canReadTemplates}
          />
          <SendingCard
            values={values}
            onChange={update}
            booked={scheduled ? broadcast?.schedule : undefined}
            workspaceTimezone={workspaceTimezone(workspace)}
          />
          <TrackingCard values={values} onChange={update} />
          <AdvancedRow workspaceId={ws} broadcastId={id} values={values} onChange={update} stored={broadcast?.data_feed} />
          {scheduled ? (
            <SaveBar
              dirty={autosave.dirty}
              saving={autosave.state.kind === 'saving'}
              onSave={async () => {
                const { ok } = await autosave.flush()
                if (ok) message.success(t`Broadcast updated successfully`)
                else if (autosave.state.kind === 'error') message.error(autosave.state.message)
              }}
              onDiscard={() => setValues(savedValues)}
              leaveWarning={t`Your changes to this scheduled broadcast will be lost.`}
              saveLabel={t`Save changes`}
              saveDisabled={issue !== null}
              saveDisabledReason={issue ? issueText(issue) : undefined}
              blockNavigation={false}
            />
          ) : null}
        </div>
        <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:sticky min-[1200px]:top-14 min-[1200px]:w-aside">
          <PreviewPanel
            workspace={workspace}
            variants={values.variants}
            abEnabled={values.abEnabled}
            templates={templates}
            previewIndex={previewIndex}
            onPreview={setPreviewIndex}
            canReadTemplates={canReadTemplates}
          />
          <SendSummary checks={checks} values={values} listName={listName} segmentNames={segmentNames} workspaceId={ws} />
        </aside>
      </div>
      <SendTemplateModal
        isOpen={!!testTemplate}
        onClose={() => setTestTemplate(null)}
        template={testTemplate}
        workspace={workspace}
        canReadLists={canReadLists}
        canReadContacts={!!permissions?.contacts?.read}
        {...testEmailListProps({ audience: { list: values.list, exclude_unsubscribed: values.excludeUnsubscribed } } as Broadcast)}
      />
    </FullDrawer>
  )
}

/** "Saving…", "Saved 2 minutes ago", "Incomplete: …" or "Not saved" with Retry. */
function SaveStatus({
  state,
  issueText,
  onRetry
}: {
  state: SaveState
  issueText: ReturnType<typeof useIssueText>
  onRetry: () => void
}) {
  const { t } = useLingui()
  switch (state.kind) {
    case 'saving':
      return <span className="text-xs text-ink-3">{t`Saving…`}</span>
    case 'saved': {
      const ago = dayjs(state.at).fromNow()
      return <span className="text-xs text-ink-3">{t`Saved ${ago}`}</span>
    }
    case 'incomplete': {
      const reason = issueText(state.issue)
      return <span className="truncate text-xs text-warning-fg">{t`Incomplete: ${reason}`}</span>
    }
    case 'error':
      return (
        <span className="flex items-center gap-1.5 text-xs">
          <Tooltip title={state.message}>
            <span className="cursor-help text-danger-fg">{t`Not saved`}</span>
          </Tooltip>
          <button type="button" onClick={onRetry} className="text-primary-600 hover:underline">
            {t`Retry`}
          </button>
        </span>
      )
    default:
      return null
  }
}
