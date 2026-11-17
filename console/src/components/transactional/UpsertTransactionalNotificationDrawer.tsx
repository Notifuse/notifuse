import { useEffect, useState, type ReactNode } from 'react'
import { templateLanguages } from '../templates/templateLanguages'
import { Alert, App, Button, Form, Input, Select, Space } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { Bell, Copy, Link2, Lock, Mail } from 'lucide-react'
import {
  transactionalNotificationsApi,
  type TransactionalNotification
} from '../../services/api/transactional_notifications'
import { ApiError } from '../../services/api/client'
import type { Workspace } from '../../services/api/types'
import TemplateSelectorInput from '../templates/TemplateSelectorInput'
import TemplatePreviewDrawer from '../templates/TemplatePreviewDrawer'
import { ownVariablePaths } from '../templates/liquidVariablePaths'
import { useLanguageName } from '../templates/useLanguageName'
import { FullDrawer, KeyValueList, Mono, Panel, Pill } from '../layout'
import { SettingsRow, SettingsRows } from '../settings/SettingsRow'
import { getEmailTrackingPolicy } from '../../lib/emailTracking'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { TRANSACTIONAL_ID_MAX_LENGTH, TRANSACTIONAL_NAME_MAX_LENGTH } from '../../constants/limits'
import { ApiCallPanel } from './ApiCallPanel'
import { ApiCommandDrawer } from './ApiCommandDrawer'
import { PreviewPanel } from './PreviewPanel'
import { DeletedTemplatePill, IntegrationTag } from './notificationParts'
import { NotificationChecks, type IdStatus } from './NotificationChecks'
import { resolveSenderInfo } from './senderInfo'
import { useTemplateStates } from './useTransactionalStats'
import {
  API_ID_PATTERN,
  createRequest,
  dataToSend,
  draftOf,
  generateApiId,
  mergeDraft,
  sameDraft,
  trackingEffect,
  updateRequest,
  type NotificationDraft,
  type TrackingEffect
} from './notificationForm'

interface UpsertTransactionalNotificationDrawerProps {
  workspace: Workspace
  notification?: TransactionalNotification
  buttonProps?: Record<string, unknown>
  buttonContent?: ReactNode
  onClose?: () => void
  /**
   * Opens the editor from outside (the notification sheet, a row's menu) instead of its own button,
   * which is then not rendered. `onClose` must set it back to false.
   */
  open?: boolean
  // Without Templates read the template field requests nothing and says why it shows only
  // the template's ID. Required, so a caller cannot forget it.
  canReadTemplates: boolean
  /** The back button of an edit: the notification's name when opened from its sheet. */
  backLabel?: string
}

/**
 * Creates or edits a transactional notification in a full-width drawer, the frame of the sheet it
 * opens from: the notification, its email and its tracking on the left; what it still needs to
 * send, its template's preview and (before it exists) the API call on the right.
 */
export function UpsertTransactionalNotificationDrawer({
  workspace,
  notification,
  canReadTemplates,
  buttonProps = {},
  buttonContent,
  onClose,
  open,
  backLabel
}: UpsertTransactionalNotificationDrawerProps) {
  const { t } = useLingui()
  const controlled = open !== undefined
  const [ownOpen, setOwnOpen] = useState(false)
  const isOpen = controlled ? open : ownOpen

  const close = () => {
    setOwnOpen(false)
    onClose?.()
  }

  return (
    <>
      {controlled ? null : (
        <Button onClick={() => setOwnOpen(true)} {...buttonProps}>
          {buttonContent || (notification ? t`Edit Notification` : t`Create Notification`)}
        </Button>
      )}
      {/* Mounted while open only: each opening starts from the notification, nothing typed kept. */}
      {isOpen ? (
        <NotificationEditor
          key={notification?.id ?? 'new'}
          workspace={workspace}
          notification={notification}
          canReadTemplates={canReadTemplates}
          backLabel={backLabel}
          onClose={close}
        />
      ) : null}
    </>
  )
}

interface NotificationEditorProps {
  workspace: Workspace
  notification?: TransactionalNotification
  canReadTemplates: boolean
  backLabel?: string
  onClose: () => void
}

/** How long typing must pause before the identifier is looked up. */
const ID_CHECK_DELAY = 400

/** The identifier once typing pauses, for the « is free » lookup. */
function useSettledValue(value: string, delay: number): string {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(value), delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
  return settled
}

function NotificationEditor({ workspace, notification, canReadTemplates, backLabel, onClose }: NotificationEditorProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const languageName = useLanguageName()
  const [form] = Form.useForm<NotificationDraft>()
  const editing = Boolean(notification)
  const managed = Boolean(notification?.integration_id)
  const integration = managed
    ? workspace.integrations?.find((item) => item.id === notification?.integration_id)
    : undefined

  const [baseline] = useState<NotificationDraft>(() => draftOf(notification, workspace))
  // A new notification's identifier follows its name, and utm_content its identifier, until typed in.
  const [idEdited, setIdEdited] = useState(false)
  const [utmContentEdited, setUtmContentEdited] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [apiOpen, setApiOpen] = useState(false)

  const watched = Form.useWatch([], { form, preserve: true }) as Partial<NotificationDraft> | undefined
  const draft = mergeDraft(baseline, watched)
  const dirty = !sameDraft(draft, baseline)
  const mode = draft.tracking_settings.tracking_mode

  // The template, read under the sheet's key: summary, checks and preview share it.
  const templateId = draft.channels.email.template_id
  const templates = useTemplateStates({
    workspaceId: workspace.id,
    templateIds: templateId ? [templateId] : [],
    ready: true,
    canRead: canReadTemplates
  })
  const templateState = templateId ? templates.get(templateId) : undefined
  const template = templateState?.status === 'ok' ? templateState.template : undefined

  // Whether the identifier typed is free: before Save, rather than a failed create.
  const typedId = draft.id.trim()
  const settledId = useSettledValue(typedId, ID_CHECK_DELAY)
  const lookupId = !editing && settledId !== '' && API_ID_PATTERN.test(settledId) ? settledId : ''
  const idLookup = useQuery({
    queryKey: ['transactional-id-check', workspace.id, lookupId],
    queryFn: () => transactionalNotificationsApi.get({ workspace_id: workspace.id, id: lookupId }),
    enabled: lookupId !== '',
    retry: false,
    staleTime: 0
  })
  let idStatus: IdStatus = 'unknown'
  if (lookupId !== '' && lookupId === typedId) {
    if (idLookup.isFetching) idStatus = 'checking'
    else if (idLookup.data?.notification) idStatus = 'taken'
    else if (idLookup.error instanceof ApiError && idLookup.error.status === 404) idStatus = 'free'
  }

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['transactional-notifications', workspace.id] })
    if (notification) {
      // The sheet loads the notification on its own key.
      void queryClient.invalidateQueries({
        queryKey: ['transactional-notification', workspace.id, notification.id]
      })
    }
  }

  const save = useMutation({
    mutationFn: (values: NotificationDraft) =>
      notification
        ? transactionalNotificationsApi.update(updateRequest(values, notification, workspace.id))
        : transactionalNotificationsApi.create(createRequest(values, workspace.id)),
    onSuccess: () => {
      message.success(notification ? t`Notification updated successfully` : t`Notification created successfully`)
      onClose()
      invalidate()
    },
    onError: (error: Error) => {
      message.error(
        notification
          ? t`Failed to update notification: ${error.message}`
          : t`Failed to create notification: ${error.message}`
      )
    }
  })

  const onFinish = () => {
    // The whole store, not only the fields antd validated: every value the draft holds is sent.
    save.mutate(mergeDraft(baseline, form.getFieldsValue(true) as Partial<NotificationDraft>))
  }

  const requestClose = () => {
    if (!dirty || save.isPending) {
      onClose()
      return
    }
    // The template editor's question, with buttons that say what they do.
    modal.confirm({
      title: t`Discard your changes?`,
      content: t`The changes you made since opening this notification will be lost.`,
      okText: t`Discard`,
      okButtonProps: { danger: true },
      cancelText: t`Keep editing`,
      onOk: onClose
    })
  }

  const showField = (path: string[]) => {
    form.scrollToField(path, { block: 'center', behavior: 'smooth' })
    form.focusField(path)
  }

  const copyId = async () => {
    if (!notification) return
    if (await copyToClipboard(notification.id)) message.success(t`ID copied`)
    else message.error(t`Failed to copy to clipboard`)
  }

  // Bar
  const barTitle = (
    <>
      <span className="truncate text-sm font-semibold text-ink">
        {editing ? t`Edit notification` : draft.name.trim() || t`New notification`}
      </span>
      {!editing && typedId ? <Mono>{typedId}</Mono> : null}
      {!editing ? <Pill tone="primary">{t`Not created`}</Pill> : null}
      {notification && managed ? <IntegrationTag notification={notification} workspace={workspace} /> : null}
      {editing && dirty ? <Pill tone="warning">{t`Unsaved changes`}</Pill> : null}
    </>
  )
  const topActions = editing ? (
    <>
      {/* The baseline names every field, so discarding also clears what was typed in an empty one. */}
      {dirty ? <Button onClick={() => form.setFieldsValue(baseline)}>{t`Discard`}</Button> : null}
      <Button type="primary" disabled={!dirty} loading={save.isPending} onClick={() => form.submit()}>
        {t`Save changes`}
      </Button>
    </>
  ) : (
    <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
      {t`Create notification`}
    </Button>
  )

  // Locks say their reason under the field.
  const lockedLabel = (label: string) => (
    <span className="inline-flex items-center gap-1">
      {label}
      <Lock size={12} aria-hidden="true" className="text-ink-3" />
    </span>
  )
  const integrationName = integration?.name
  const setByIntegration = integrationName ? t`Set by ${integrationName}` : t`Set by the integration`
  const templateSetBy = integrationName
    ? t`Set by ${integrationName}. Edit the template itself to change its content.`
    : t`Set by the integration. Edit the template itself to change its content.`
  const managedLabel = integration
    ? t`Managed by ${integration.name} (${integration.type} integration)`
    : t`Managed by an integration`

  // An identifier refused by its pattern gets the error alone: the hint would say it a second time.
  const idRefused = typedId !== '' && !API_ID_PATTERN.test(typedId)
  const idExtra = idRefused
    ? undefined
    : idEdited
      ? t`Lowercase letters, numbers and underscores`
      : t`Follows the name until you edit it. Lowercase letters, numbers and underscores`
  // The checks' message, with the same variable, so both read one translation.
  const id = typedId
  const idTaken = idStatus === 'taken' ? t`${id} is already used by another notification` : undefined

  // The template's summary under its field.
  const sender = template ? resolveSenderInfo(workspace, template).sender : null
  const defaultLanguage = workspace.settings?.default_language
  const languages = template
    ? [
        ...(defaultLanguage ? [`${languageName(defaultLanguage)} ${t`(default)`}`] : []),
        // Only the translations with content, the rule the template selector shows too.
        ...templateLanguages(template, defaultLanguage ?? '')
          .slice(1)
          .map((code) => languageName(code))
      ]
    : []
  const variables = template ? ownVariablePaths(template.test_data) : []
  const summary = template ? (
    <div data-testid="template-summary" className="rounded-md border border-line-soft bg-subtle px-3 py-2.5">
      <KeyValueList
        labelWidth={96}
        items={[
          { label: t`Subject`, value: template.email?.subject || '—' },
          {
            label: t`Sender`,
            value: sender ? (
              sender.name ? `${sender.name} · ${sender.email}` : sender.email
            ) : (
              <span className="text-warning-fg">{t`No sender`}</span>
            )
          },
          { label: t`Languages`, value: languages.length ? languages.join(', ') : '—' },
          {
            label: t`Data to send`,
            value: variables.length ? (
              <span className="flex flex-wrap gap-1">
                {variables.map((path) => (
                  <Mono key={path}>{path}</Mono>
                ))}
              </span>
            ) : (
              t`None`
            )
          }
        ]}
      />
    </div>
  ) : null

  const templateExtra = !canReadTemplates
    ? t`You need Templates read permission to see this template's name and preview`
    : managed
      ? templateSetBy
      : editing || draft.channels.email.template_id
        ? undefined
        : t`Transactional templates. Its subject, sender and languages show here once chosen.`

  // The API call of a notification not created yet, with the identifier typed and the template's data.
  const apiNotification = {
    id: typedId || 'notification_id',
    name: draft.name.trim() || t`New notification`,
    data: template ? dataToSend(template.test_data) : undefined
  }

  const utmDisabled = mode === 'disabled'
  const utmLabel = (field: string) => <span className="font-mono text-xs text-ink-2">{field}</span>

  return (
    <FullDrawer
      open
      onClose={requestClose}
      backLabel={editing ? (backLabel ?? t`Transactional`) : t`Transactional`}
      copyLink={false}
      // Escape stays in the fields; closing by accident would lose what was typed.
      keyboard={false}
      // The template selector and the preview open windows of their own: the editor must not slide aside.
      push={false}
      ariaLabel={editing ? t`Edit notification` : t`New notification`}
      barTitle={barTitle}
      topActions={topActions}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={baseline}
        onFinish={onFinish}
        onFinishFailed={() => message.error(t`Please check the form for errors.`)}
      >
        <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
          <div className="flex min-w-0 flex-1 flex-col gap-3">
            {managed ? (
              <Alert
                type="info"
                showIcon
                title={managedLabel}
                description={t`Its name and its template are set by the integration. You can change the description and the tracking.`}
              />
            ) : null}

            <Panel title={t`Notification`} icon={<Bell size={14} aria-hidden="true" />}>
              <div className="grid grid-cols-2 gap-x-3">
                <Form.Item
                  name="name"
                  label={managed ? lockedLabel(t`Name`) : t`Name`}
                  extra={managed ? setByIntegration : undefined}
                  rules={[
                    { required: true, message: t`Please enter a notification name` },
                    {
                      max: TRANSACTIONAL_NAME_MAX_LENGTH,
                      message: t`Notification name must be at most ${TRANSACTIONAL_NAME_MAX_LENGTH} characters long`
                    }
                  ]}
                >
                  <Input
                    autoFocus={!editing}
                    placeholder={t`E.g. Password Reset Email`}
                    disabled={managed}
                    maxLength={TRANSACTIONAL_NAME_MAX_LENGTH}
                    showCount
                    onChange={(event) => {
                      if (editing) return
                      const generated = generateApiId(event.target.value)
                      if (!idEdited) form.setFieldValue('id', generated)
                      if (!utmContentEdited) {
                        form.setFieldValue(
                          ['tracking_settings', 'utm_content'],
                          idEdited ? form.getFieldValue('id') : generated
                        )
                      }
                    }}
                  />
                </Form.Item>

                {notification ? (
                  <Form.Item label={lockedLabel(t`API identifier`)} extra={t`Fixed once created: API calls send it`}>
                    <Space.Compact block>
                      <Input value={notification.id} disabled aria-label={t`API identifier`} />
                      <Button
                        icon={<Copy size={14} aria-hidden="true" />}
                        aria-label={t`Copy ID`}
                        onClick={() => void copyId()}
                      />
                    </Space.Compact>
                  </Form.Item>
                ) : (
                  <Form.Item
                    name="id"
                    label={t`API identifier`}
                    extra={idExtra}
                    validateStatus={idTaken ? 'error' : undefined}
                    help={idTaken}
                    rules={[
                      { required: true, message: t`Please enter an API identifier` },
                      {
                        // Only an identifier known to be taken refuses: not one still being checked,
                        // nor one whose lookup failed otherwise than not found. The value is compared
                        // because a change validates with the rules of the render before it.
                        validator: (_, value: string | undefined) =>
                          idTaken && (value ?? '').trim() === typedId ? Promise.reject(new Error(idTaken)) : Promise.resolve()
                      },
                      {
                        pattern: API_ID_PATTERN,
                        message: t`ID can only contain lowercase letters, numbers, and underscores`
                      },
                      {
                        max: TRANSACTIONAL_ID_MAX_LENGTH,
                        message: t`API identifier must be at most ${TRANSACTIONAL_ID_MAX_LENGTH} characters long`
                      }
                    ]}
                  >
                    <Input
                      placeholder={t`E.g. password_reset`}
                      maxLength={TRANSACTIONAL_ID_MAX_LENGTH}
                      showCount
                      onChange={(event) => {
                        setIdEdited(true)
                        if (!utmContentEdited) form.setFieldValue(['tracking_settings', 'utm_content'], event.target.value)
                      }}
                    />
                  </Form.Item>
                )}

                <div className="col-span-2">
                  <Form.Item name="description" label={t`Description`}>
                    <Input.TextArea rows={3} placeholder={t`A brief description of this notification's purpose`} />
                  </Form.Item>
                </div>
              </div>
            </Panel>

            <Panel
              title={t`Email`}
              icon={<Mail size={14} aria-hidden="true" />}
              extra={<span className="text-xs text-ink-3">{t`The only channel today`}</span>}
            >
              {/* The field's own bottom margin sets the summary apart. */}
              <div className="flex flex-col">
                <Form.Item
                  name={['channels', 'email', 'template_id']}
                  label={managed ? lockedLabel(t`Email template`) : t`Email template`}
                  rules={[{ required: true, message: t`Please select an email template` }]}
                  extra={
                    template?.deleted_at ? (
                      <span className="flex flex-col gap-1">
                        {templateExtra ? <span>{templateExtra}</span> : null}
                        <span data-testid="deleted-template-warning" className="flex flex-wrap items-center gap-2">
                          <DeletedTemplatePill />
                          <span className="text-warning-fg">
                            {managed
                              ? t`This notification still sends with it`
                              : t`This notification still sends with it. Choose another template to stop depending on a deleted one.`}
                          </span>
                        </span>
                      </span>
                    ) : (
                      templateExtra
                    )
                  }
                >
                  <TemplateSelectorInput
                    workspaceId={workspace.id}
                    placeholder={t`Select email template`}
                    category="transactional"
                    sendsWith="transactional"
                    disabled={managed}
                    canReadTemplates={canReadTemplates}
                  />
                </Form.Item>
                {summary}
              </div>
            </Panel>

            <Panel
              title={t`Tracking`}
              icon={<Link2 size={14} aria-hidden="true" />}
              extra={<span className="text-xs text-ink-3">{t`Links in this notification's emails`}</span>}
            >
              <SettingsRows>
                <SettingsRow
                  label={t`Tracking`}
                  help={
                    mode === 'disabled'
                      ? t`Links are left as written: no tracking and no UTM parameters.`
                      : mode === 'no_tracking'
                        ? t`Opens and clicks are not tracked; links keep their UTM parameters.`
                        : t`Following the workspace setting also respects each contact's tracking consent.`
                  }
                >
                  <div className="flex flex-col gap-1.5">
                    <Form.Item name={['tracking_settings', 'tracking_mode']}>
                      <Select
                        aria-label={t`Tracking`}
                        options={[
                          { value: 'inherit', label: t`Follow workspace setting` },
                          { value: 'no_tracking', label: t`No open or click tracking, keep UTM parameters` },
                          { value: 'disabled', label: t`Disabled for this notification` }
                        ]}
                      />
                    </Form.Item>
                    <TrackingPills effect={trackingEffect(mode, getEmailTrackingPolicy(workspace.settings))} />
                  </div>
                </SettingsRow>
                <SettingsRow
                  label={t`UTM parameters`}
                  help={
                    utmDisabled
                      ? t`Not added while tracking is disabled: links are never rewritten. Kept for when you turn it back on.`
                      : t`Added to every link, with or without open and click tracking.`
                  }
                >
                  <div className="grid grid-cols-2 gap-x-2.5 gap-y-2">
                    <Form.Item
                      name={['tracking_settings', 'utm_source']}
                      label={utmLabel('utm_source')}
                      tooltip={t`Identifies which site sent the traffic (e.g. google, newsletter)`}
                    >
                      <Input placeholder={t`e.g. notifuse`} disabled={utmDisabled} />
                    </Form.Item>
                    <Form.Item
                      name={['tracking_settings', 'utm_medium']}
                      label={utmLabel('utm_medium')}
                      tooltip={t`Identifies what type of link was used (e.g. email, cpc, banner)`}
                    >
                      <Input placeholder={t`e.g. email`} disabled={utmDisabled} />
                    </Form.Item>
                    <Form.Item
                      name={['tracking_settings', 'utm_campaign']}
                      label={utmLabel('utm_campaign')}
                      tooltip={t`Identifies a specific product promotion or strategic campaign`}
                    >
                      <Input placeholder={t`e.g. welcome_series`} disabled={utmDisabled} />
                    </Form.Item>
                    <Form.Item
                      name={['tracking_settings', 'utm_content']}
                      label={utmLabel('utm_content')}
                      tooltip={t`Identifies what specifically was clicked (e.g. header_link, body_link)`}
                    >
                      {/* Left empty, a send uses the template's ID (email_service.go). */}
                      <Input
                        placeholder={t`Defaults to the template ID`}
                        disabled={utmDisabled}
                        onChange={() => setUtmContentEdited(true)}
                      />
                    </Form.Item>
                    <Form.Item
                      name={['tracking_settings', 'utm_term']}
                      label={utmLabel('utm_term')}
                      tooltip={t`Identifies the keyword of a paid search (e.g. running_shoes)`}
                    >
                      <Input placeholder={t`Optional`} disabled={utmDisabled} />
                    </Form.Item>
                  </div>
                </SettingsRow>
              </SettingsRows>
            </Panel>
          </div>

          <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:sticky min-[1200px]:top-16 min-[1200px]:w-aside">
            <NotificationChecks
              workspace={workspace}
              editing={editing}
              draft={draft}
              baseline={baseline}
              idStatus={idStatus}
              template={templateState}
              templateLocked={managed}
              onShowField={showField}
            />
            {templateState ? (
              <PreviewPanel workspace={workspace} template={templateState} onOpen={() => setPreviewOpen(true)} />
            ) : null}
            {editing ? null : (
              <ApiCallPanel
                notification={apiNotification}
                workspaceId={workspace.id}
                onOpenAll={() => setApiOpen(true)}
                note={t`Works once created`}
              />
            )}
          </aside>
        </div>
      </Form>

      {previewOpen && template ? (
        <TemplatePreviewDrawer open record={template} workspace={workspace} onClose={() => setPreviewOpen(false)} />
      ) : null}
      {apiOpen ? (
        <ApiCommandDrawer
          open
          onClose={() => setApiOpen(false)}
          notification={apiNotification}
          workspaceId={workspace.id}
        />
      ) : null}
    </FullDrawer>
  )
}

/** What the tracking mode does to this notification's emails, with the workspace's setting. */
function TrackingPills({ effect }: { effect: TrackingEffect }) {
  const { t } = useLingui()
  const opens = { on: t`Opens on`, off: t`Opens off`, workspaceOff: t`Opens off (workspace)` }[effect.opens]
  const clicks = { on: t`Clicks on`, off: t`Clicks off`, workspaceOff: t`Clicks off (workspace)` }[effect.clicks]
  return (
    <div data-testid="tracking-effect" className="flex flex-wrap gap-1.5">
      <Pill tone={effect.opens === 'on' ? 'success' : 'neutral'}>{opens}</Pill>
      <Pill tone={effect.clicks === 'on' ? 'success' : 'neutral'}>{clicks}</Pill>
      {/* Green whenever they are added, as the opens and clicks pills are when on. */}
      <Pill tone={effect.utm ? 'success' : 'neutral'}>
        {effect.utm ? t`UTM added` : t`No UTM`}
      </Pill>
    </div>
  )
}

export default UpsertTransactionalNotificationDrawer
