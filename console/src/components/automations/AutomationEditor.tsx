import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Alert, App, Button, ConfigProvider, Dropdown, Input, Result, Segmented, Skeleton, Tooltip, type MenuProps } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { Ellipsis, Pause, Play, Redo2, Undo2 } from 'lucide-react'
import { FullDrawer, Pill } from '../layout'
import { automationApi, type Automation } from '../../services/api/automation'
import { ApiError } from '../../services/api/errors'
import type { Workspace, Template } from '../../services/api/types'
import type { List } from '../../services/api/list'
import type { Segment } from '../../services/api/segment'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { workspaceTimezone } from '../broadcasts/access'
import { AutomationProvider, useAutomation } from './context'
import { AutomationFlowEditor } from './AutomationFlowEditor'
import { AutomationStatusPill } from './AutomationStatusPill'
import { EditorSettings } from './EditorSettings'
import { NodePalette } from './NodePalette'
import { hasEmailStep } from './journeyRates'
import { useAutomationActions } from './list/useAutomationActions'
import { useNodeStats } from './useNodeStats'
import { NodeStatsContext, type NodeStatsState, type StatsPeriod } from './stats/nodeStatsState'

export interface AutomationEditorProps {
  workspace: Workspace
  /** `new`, or the id of the automation to open. */
  automationParam: string
  /** "With stats" (the URL's `stats=1`). */
  withStats: boolean
  canWrite: boolean
  canReadHistory: boolean
  canReadTemplates: boolean
  lists: List[]
  segments: Segment[]
  templates: Template[]
  /** Journeys since creation (A1), when the list knows them: a draft without any shows no stats. */
  journeyTotal?: number
  onClose: () => void
  /** A new automation was created: the URL moves to its id. */
  onCreated: (id: string) => void
  onToggleStats: (on: boolean) => void
  onViewMessages: (id: string) => void
}

/** The automation editor: a full-screen drawer driven by the URL, over the list. */
export function AutomationEditor(props: AutomationEditorProps) {
  const { t } = useLingui()
  const { workspace, automationParam } = props
  // The session is fixed when the editor opens: `new` turning into an id after the create keeps
  // the same provider, so the edits and their undo history stay.
  const [session] = useState(automationParam)
  const [createdId, setCreatedId] = useState<string | null>(null)
  const [reloadCount, setReloadCount] = useState(0)
  const id = session === 'new' ? createdId : session

  // E1: always read from the server, never from the list's row.
  const e1 = useQuery({
    queryKey: ['automation', workspace.id, id],
    queryFn: () => automationApi.get({ workspace_id: workspace.id, automation_id: id! }),
    enabled: !!id
  })
  const automation = e1.data?.automation
  // The editor opens on a copy read since it opened, never on one cached by an earlier opening:
  // its flow is copied into the editor once, and saving it would overwrite a newer one.
  const [ready, setReady] = useState(false)
  if (!ready && e1.isFetchedAfterMount && e1.status === 'success') setReady(true)

  const shell = (body: ReactNode) => (
    <FullDrawer open onClose={props.onClose} backLabel={t`Automations`} copyLink={false}>
      {body}
    </FullDrawer>
  )

  // The skeleton and "not found" belong to the first load only: a later error keeps the editor.
  if (session !== 'new' && (!ready || !automation)) {
    if (e1.isError && !e1.isFetching) {
      const notFound = e1.error instanceof ApiError && e1.error.status === 404
      return shell(
        <Result
          status={notFound ? '404' : 'error'}
          title={notFound ? t`Automation not found` : t`Couldn't load the automation`}
          extra={
            <div className="flex justify-center gap-2">
              {!notFound ? <Button onClick={() => void e1.refetch()}>{t`Retry`}</Button> : null}
              <Button type="primary" onClick={props.onClose}>
                {t`Back to automations`}
              </Button>
            </div>
          }
        />
      )
    }
    return shell(
      <div data-editor-loading className="flex flex-col gap-3">
        <Skeleton active title paragraph={{ rows: 8 }} />
      </div>
    )
  }

  const readOnly = !props.canWrite || automation?.status === 'live'

  return (
    <AutomationProvider
      key={`${session}:${reloadCount}`}
      workspace={workspace}
      automation={automation}
      lists={props.lists}
      segments={props.segments}
      templates={props.templates}
      canReadTemplates={props.canReadTemplates}
      readOnly={readOnly}
      onSaved={(saved) => {
        if (session === 'new' && !createdId) {
          setCreatedId(saved.id)
          props.onCreated(saved.id)
        }
      }}
    >
      <EditorScreen
        {...props}
        loaded={automation}
        onReload={async () => {
          await e1.refetch()
          setReloadCount((n) => n + 1)
        }}
      />
    </AutomationProvider>
  )
}

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return true
  return target.closest('[contenteditable]:not([contenteditable="false"]), [role="textbox"], [role="combobox"]') !== null
}

/** No modal or other drawer covers the editor (same test as the drawer navigation keys). */
function isTopmost(anchor: HTMLElement | null): boolean {
  const own = anchor?.closest<HTMLElement>('.ant-drawer')
  if (!own) return false
  const overlays = document.querySelectorAll<HTMLElement>('.ant-drawer.ant-drawer-open, .ant-modal-wrap')
  return !Array.from(overlays).some(
    (overlay) =>
      overlay !== own &&
      overlay.style.display !== 'none' &&
      (own.compareDocumentPosition(overlay) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
  )
}

interface EditorScreenProps extends AutomationEditorProps {
  /** The automation as last read from the server (E1). */
  loaded: Automation | undefined
  onReload: () => Promise<void>
}

function EditorScreen({
  workspace,
  withStats,
  canWrite,
  canReadHistory,
  journeyTotal,
  loaded,
  onClose,
  onToggleStats,
  onViewMessages,
  onReload
}: EditorScreenProps) {
  const { t } = useLingui()
  const { modal, message } = App.useApp()
  const ctx = useAutomation()
  const {
    automation: saved,
    automationId,
    isEditing,
    readOnly,
    name,
    setName,
    listId,
    hasUnsavedChanges: dirty,
    isSaving,
    save,
    validate,
    canUndo,
    canRedo,
    undo,
    redo,
    conflict,
    selectedNodeId
  } = ctx
  const actions = useAutomationActions(workspace.id)
  const anchorRef = useRef<HTMLDivElement>(null)
  const [period, setPeriod] = useState<StatsPeriod>('last30')
  const [savedThisSession, setSavedThisSession] = useState(false)
  const [busy, setBusy] = useState<'activate' | 'pause' | null>(null)

  const status = loaded?.status ?? saved?.status ?? 'draft'
  const live = status === 'live'
  // A draft no contact entered has nothing to show: the toggle is off with its reason.
  const neverRan = !isEditing || (status === 'draft' && (journeyTotal ?? 0) === 0)
  const statsOn = withStats && isEditing && !neverRan

  // E3, while the results are on screen.
  const e3 = useNodeStats({
    workspaceId: workspace.id,
    automationId,
    timezone: workspaceTimezone(workspace),
    period,
    available: isEditing && !neverRan,
    watched: statsOn || !!selectedNodeId,
    live
  })
  const savedNodeIds = useMemo(() => new Set((saved?.nodes ?? []).map((n) => n.id)), [saved])
  const statsState: NodeStatsState = {
    enabled: statsOn,
    period,
    loading: e3.isLoading,
    error: e3.isError,
    nodes: e3.data?.nodes,
    savedNodeIds,
    canReadHistory,
    neverRan
  }

  const leaveWarning = t`You have unsaved changes. Are you sure you want to close?`
  const confirmLeave = (onOk: () => void) =>
    modal.confirm({
      title: t`Unsaved Changes`,
      content: leaveWarning,
      okText: t`Close without saving`,
      cancelText: t`Cancel`,
      onOk
    })

  // Leaving the editor with changes asks; toggling the stats or the URL's own writes never do.
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => {
      if (!dirty) return false
      const nextSearch = next.search as { automation?: string }
      const currentSearch = current.search as { automation?: string }
      return next.pathname !== current.pathname || nextSearch.automation !== currentSearch.automation
    },
    enableBeforeUnload: () => dirty,
    withResolver: true
  })
  const blockedOnce = useRef(false)
  useEffect(() => {
    if (blocker.status !== 'blocked' || blockedOnce.current) return
    blockedOnce.current = true
    modal.confirm({
      title: t`Unsaved Changes`,
      content: leaveWarning,
      okText: t`Close without saving`,
      cancelText: t`Cancel`,
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

  const close = () => (dirty ? confirmLeave(onClose) : onClose())

  const handleSave = useCallback(async () => {
    if (readOnly) return
    if (!name.trim()) {
      modal.error({ title: t`Validation Error`, content: t`Please enter an automation name` })
      return
    }
    const warnings = validate().filter((e) => e.message.startsWith('Warning:'))
    const run = async () => {
      if (await save()) setSavedThisSession(true)
    }
    if (warnings.length > 0) {
      modal.confirm({
        title: t`Warning`,
        content: warnings.map((w) => w.message).join('\n'),
        okText: t`Save Anyway`,
        cancelText: t`Cancel`,
        onOk: run
      })
      return
    }
    await run()
  }, [readOnly, name, validate, save, modal, t])

  // ⌘S / Ctrl+S saves; ⌘Z, ⇧⌘Z and Ctrl+Y undo and redo, outside fields and when on top.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const modifier = navigator.platform.toUpperCase().includes('MAC') ? e.metaKey : e.ctrlKey
      if (!modifier || readOnly) return
      if (!isTopmost(anchorRef.current)) return
      const key = e.key.toLowerCase()
      if (key === 's') {
        e.preventDefault()
        if (!e.repeat) void handleSave()
        return
      }
      if (isEditable(e.target)) return
      if (key === 'z' && !e.shiftKey) {
        e.preventDefault()
        if (canUndo) undo()
      } else if ((key === 'z' && e.shiftKey) || key === 'y') {
        e.preventDefault()
        if (canRedo) redo()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [readOnly, canUndo, canRedo, undo, redo, handleSave])

  const runStatus = async (kind: 'activate' | 'pause') => {
    setBusy(kind)
    try {
      await (kind === 'activate' ? actions.activate(automationId) : actions.pause(automationId))
    } finally {
      setBusy(null)
    }
  }
  const confirmStatus = (kind: 'activate' | 'pause') =>
    modal.confirm(
      kind === 'activate'
        ? {
            title: t`Activate automation?`,
            content: t`The automation will start processing contacts that match the trigger.`,
            okText: t`Yes, activate`,
            cancelText: t`Cancel`,
            onOk: () => runStatus('activate')
          }
        : {
            title: t`Pause automation?`,
            content: t`The automation will stop processing new contacts.`,
            okText: t`Yes, pause`,
            cancelText: t`Cancel`,
            onOk: () => runStatus('pause')
          }
    )

  const confirmDelete = () =>
    modal.confirm({
      title: t`Delete automation?`,
      content: t`This action cannot be undone.`,
      okText: t`Yes, delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      onOk: async () => {
        if (await actions.remove(automationId)) onClose()
      }
    })

  const reload = () => {
    const run = () => void onReload()
    if (dirty) {
      modal.confirm({
        title: t`Reload the automation?`,
        content: t`Your changes will be lost.`,
        okText: t`Reload`,
        cancelText: t`Cancel`,
        okButtonProps: { danger: true },
        onOk: run
      })
    } else run()
  }

  const noWrite = t`You don't have write permission for automations`
  const menuItems: MenuProps['items'] = [
    {
      key: 'copy',
      label: t`Copy ID`,
      disabled: !isEditing,
      onClick: async () => {
        if (await copyToClipboard(automationId)) message.success(t`ID copied`)
        else message.error(t`Failed to copy to clipboard`)
      }
    }
  ]
  if (isEditing && canReadHistory && saved && hasEmailStep(saved)) {
    menuItems.push({ key: 'messages', label: t`View messages`, onClick: () => onViewMessages(automationId) })
  }
  if (isEditing && canWrite && !live) {
    menuItems.push({ type: 'divider' })
    menuItems.push({ key: 'delete', danger: true, label: t`Delete Automation`, onClick: confirmDelete })
  }

  const statsDisabledReason = !isEditing ? t`Save the automation first` : neverRan ? t`No journeys yet` : undefined

  const barTitle = (
    <>
      {readOnly ? (
        <span data-unreset className="truncate text-sm font-medium text-ink">
          {name || t`Untitled automation`}
        </span>
      ) : (
        <Input
          variant="borderless"
          size="small"
          aria-label={t`Automation name`}
          placeholder={t`Automation name`}
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-64"
        />
      )}
      <AutomationStatusPill status={status} />
      {dirty ? (
        <Pill tone="warning" dot={false}>
          {t`Unsaved changes`}
        </Pill>
      ) : savedThisSession ? (
        <span className="text-xs text-ink-3">{t`Saved`}</span>
      ) : null}
    </>
  )

  const topActions = (
    <>
      {!readOnly ? (
        <span className="flex items-center">
          <Tooltip title={t`Undo (Ctrl+Z)`}>
            <Button type="text" size="small" aria-label={t`Undo`} icon={<Undo2 size={14} aria-hidden="true" />} disabled={!canUndo} onClick={undo} />
          </Tooltip>
          <Tooltip title={t`Redo (Ctrl+Shift+Z)`}>
            <Button type="text" size="small" aria-label={t`Redo`} icon={<Redo2 size={14} aria-hidden="true" />} disabled={!canRedo} onClick={redo} />
          </Tooltip>
        </span>
      ) : null}
      <Tooltip title={statsDisabledReason}>
        <span>
          <Segmented<'edit' | 'stats'>
            size="small"
            aria-label={t`View`}
            disabled={!!statsDisabledReason}
            value={statsOn ? 'stats' : 'edit'}
            onChange={(value) => onToggleStats(value === 'stats')}
            options={[
              { value: 'edit', label: t`Edit` },
              { value: 'stats', label: t`With stats` }
            ]}
          />
        </span>
      </Tooltip>
      {statsOn ? (
        <Segmented<StatsPeriod>
          size="small"
          aria-label={t`Period`}
          value={period}
          onChange={setPeriod}
          options={[
            { value: 'last30', label: t`Last 30 days` },
            { value: 'all', label: t`All time` }
          ]}
        />
      ) : null}
      {isEditing ? (
        live ? (
          <Tooltip title={canWrite ? undefined : noWrite}>
            <Button
              size="small"
              icon={<Pause size={14} aria-hidden="true" />}
              disabled={!canWrite}
              loading={busy === 'pause'}
              onClick={() => confirmStatus('pause')}
            >
              {t`Pause`}
            </Button>
          </Tooltip>
        ) : (
          <Tooltip title={!canWrite ? noWrite : dirty ? t`Save your changes first` : undefined}>
            <Button
              size="small"
              icon={<Play size={14} aria-hidden="true" />}
              disabled={!canWrite || dirty}
              loading={busy === 'activate'}
              onClick={() => confirmStatus('activate')}
            >
              {t`Activate`}
            </Button>
          </Tooltip>
        )
      ) : null}
      {!readOnly ? (
        <Button type="primary" size="small" loading={isSaving} onClick={() => void handleSave()}>
          {isEditing ? t`Save` : t`Create`}
        </Button>
      ) : null}
      <Dropdown menu={{ items: menuItems }} trigger={['click']} placement="bottomRight">
        <Button type="text" size="small" aria-label={t`More actions`} icon={<Ellipsis size={14} aria-hidden="true" />} />
      </Dropdown>
    </>
  )

  const overlay =
    live || conflict || (statsOn && e3.isError) ? (
      <div className="flex flex-col gap-2">
        {live ? (
          <Alert
            type="info"
            showIcon
            title={t`This automation is live. Pause it to edit.`}
            action={
              canWrite ? (
                <Button size="small" loading={busy === 'pause'} onClick={() => confirmStatus('pause')}>
                  {t`Pause`}
                </Button>
              ) : undefined
            }
          />
        ) : null}
        {conflict ? (
          <Alert
            type="warning"
            showIcon
            title={t`This automation was changed elsewhere`}
            action={
              <Button size="small" onClick={reload}>
                {t`Reload`}
              </Button>
            }
          />
        ) : null}
        {statsOn && e3.isError ? (
          <Alert
            type="error"
            showIcon
            title={t`Couldn't load stats`}
            action={
              <Button size="small" onClick={() => void e3.refetch()}>
                {t`Retry`}
              </Button>
            }
          />
        ) : null}
      </div>
    ) : null

  return (
    <FullDrawer
      open
      onClose={close}
      backLabel={t`Automations`}
      copyLink={isEditing}
      barTitle={barTitle}
      topActions={topActions}
      fill
      // The condition, template and preview drawers open inside: the editor must not slide aside.
      push={false}
    >
      <NodeStatsContext.Provider value={statsState}>
        <div ref={anchorRef} className="flex min-h-0 min-w-0 flex-1" data-automation-editor>
          <div className="flex w-rail shrink-0 flex-col gap-4 overflow-y-auto border-r border-line bg-surface px-3.5 py-3">
            <ConfigProvider componentDisabled={readOnly}>
              <EditorSettings />
            </ConfigProvider>
            {!readOnly ? <NodePalette hasListSelected={!!listId} /> : null}
          </div>
          <AutomationFlowEditor overlay={overlay} />
        </div>
      </NodeStatsContext.Provider>
    </FullDrawer>
  )
}
