import React from 'react'
import { Alert, App, Button, Drawer, Form, Input, Spin, Switch } from 'antd'
import { Copy } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { listsApi } from '../../services/api/list'
import { ApiError } from '../../services/api/errors'
import type {
  CreateListRequest,
  GetListsResponse,
  List,
  TemplateReference,
  UpdateListRequest
} from '../../services/api/types'
import { TemplateSelectorInput } from '../../components/templates'
import { generateListId, LIST_ID_MAX, LIST_ID_PATTERN, LIST_NAME_MAX } from './listId'
import { useListIdCheck } from './useListIdCheck'
import { useListStats } from './listStats'
import { NotificationCenterListPreview } from './NotificationCenterListPreview'

interface CreateListDrawerProps {
  workspaceId: string
  /** The list to edit; none to create one. Read when the drawer opens. */
  list?: List
  open: boolean
  onClose: () => void
  /** After a create, once the drawer is closed: the Contacts page opens the new list. */
  onCreated?: (list: List) => void
  // Without Templates read the double opt-in template shows the stored ID and is locked.
  canReadTemplates: boolean
}

interface ListFormValues {
  name?: string
  id?: string
  description?: string
  is_public?: boolean
  is_double_optin?: boolean
  double_optin_template_id?: string | null
}

/**
 * The drawer that creates a list or edits one. The body mounts on each opening and unmounts once
 * closed (`destroyOnHidden`), so every opening starts from the list it opened with.
 */
export function CreateListDrawer({
  workspaceId,
  list,
  open,
  onClose,
  onCreated,
  canReadTemplates
}: CreateListDrawerProps) {
  const { t } = useLingui()
  // The list is captured when the drawer opens: a navigation under the open drawer (browser Back)
  // changes the page's list, never the one being edited.
  // Each opening gets a body of its own (`key`), even one that starts while the last closing is
  // still animating, before destroyOnHidden has unmounted it.
  const [shown, setShown] = React.useState<{ list?: List; key: number } | null>(open ? { list, key: 0 } : null)
  const [wasOpen, setWasOpen] = React.useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setShown((prev) => ({ list, key: (prev?.key ?? 0) + 1 }))
  }
  // ×, Escape and a click beside the drawer go through the body's close guard.
  const requestClose = React.useRef<() => void>(onClose)
  const editing = !!shown?.list

  return (
    <Drawer
      title={editing ? t`Edit list` : t`New list`}
      size={480}
      open={open}
      destroyOnHidden
      onClose={() => requestClose.current()}
      styles={{ body: { padding: 0, display: 'flex', flexDirection: 'column' } }}
    >
      {shown ? (
        <ListDrawerBody
          key={shown.key}
          workspaceId={workspaceId}
          list={shown.list}
          canReadTemplates={canReadTemplates}
          onClose={onClose}
          onCreated={onCreated}
          registerClose={(close) => {
            requestClose.current = close
          }}
        />
      ) : null}
    </Drawer>
  )
}

interface ListDrawerBodyProps {
  workspaceId: string
  list?: List
  canReadTemplates: boolean
  onClose: () => void
  onCreated?: (list: List) => void
  registerClose: (close: () => void) => void
}

const GENERIC_SERVER_ERRORS = ['Failed to create list', 'Failed to update list']

// What a form holds, as compared for unsaved changes: an empty description and no template are
// the same whatever antd stores for them, and the template counts only while double opt-in is on
// (it is not saved otherwise).
function comparable(values: ListFormValues, withId: boolean): string {
  const doubleOptin = !!values.is_double_optin
  return JSON.stringify({
    name: values.name ?? '',
    id: withId ? (values.id ?? '') : '',
    description: values.description || '',
    is_public: !!values.is_public,
    is_double_optin: doubleOptin,
    template: doubleOptin ? values.double_optin_template_id || '' : ''
  })
}

function ListDrawerBody({ workspaceId, list, canReadTemplates, onClose, onCreated, registerClose }: ListDrawerBodyProps) {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<ListFormValues>()
  const isEdit = !!list
  const ids = {
    publicLabel: React.useId(),
    publicHelp: React.useId(),
    optinLabel: React.useId(),
    optinHelp: React.useId()
  }

  const [initialValues] = React.useState<ListFormValues>(() =>
    list
      ? {
          name: list.name,
          description: list.description ?? '',
          is_public: list.is_public,
          is_double_optin: list.is_double_optin,
          double_optin_template_id: list.double_optin_template?.id
        }
      : { name: '', id: '', description: '', is_public: false, is_double_optin: false }
  )
  const [snapshot] = React.useState(() => comparable(initialValues, !isEdit))
  const isDirty = () => comparable(form.getFieldsValue(true), !isEdit) !== snapshot

  const name = Form.useWatch('name', form) ?? ''
  const id = Form.useWatch('id', form) ?? ''
  const description = Form.useWatch('description', form) ?? ''
  const isPublic = !!Form.useWatch('is_public', form)
  const doubleOptin = !!Form.useWatch('is_double_optin', form)

  // The ID follows the name until it is opened by hand; a name that gives no ID opens it.
  const [idOpen, setIdOpen] = React.useState(false)
  const [idTouched, setIdTouched] = React.useState(false)
  const nameGivesNoId = !isEdit && name.trim() !== '' && generateListId(name) === ''
  const idStatus = useListIdCheck(workspaceId, id, !isEdit)
  // The IDs lists.create refused, with why: the check while typing cannot see a deleted list's
  // ID, so a refusal stays on that ID instead of letting Create send it again.
  const [refused, setRefused] = React.useState<Record<string, string>>({})
  const [saveError, setSaveError] = React.useState<string | null>(null)
  const idInput = React.useRef<React.ComponentRef<typeof Input>>(null)

  const takenMessage = (listId: string) => t`A list already has the ID “${listId}”. Change it.`
  // Own properties only: an ID such as « constructor » must not read Object.prototype.
  const refusal = Object.prototype.hasOwnProperty.call(refused, id) ? refused[id] : null
  const idError = refusal ?? (idStatus === 'taken' ? takenMessage(id) : null)
  const blocked = !isEdit && (idStatus === 'checking' || idStatus === 'taken' || refusal !== null)
  // A taken or refused ID opens the field, with the reason on it, and the field stays open: it
  // must not close under someone correcting the ID.
  if (!isEdit && idError !== null && !idOpen) setIdOpen(true)
  const idShown = !isEdit && (idOpen || nameGivesNoId)

  // The page's own query: no request when the page already holds the list's stats.
  const stats = useListStats(workspaceId, list?.id, !!list?.is_double_optin)
  const pending = stats.data?.stats.total_pending

  // Set once the drawer closes after a save: the navigation to the new list is let through.
  const leaving = React.useRef(false)
  const confirming = React.useRef(false)

  const createMutation = useMutation({
    mutationFn: (data: CreateListRequest) => listsApi.create(data)
  })
  const updateMutation = useMutation({
    mutationFn: (data: UpdateListRequest) => listsApi.update(data)
  })
  const saving = createMutation.isPending || updateMutation.isPending
  const savingRef = React.useRef(saving)
  savingRef.current = saving

  // The lists and the counts of the Contacts rail, which has its own key so that the frequent
  // invalidations of 'lists' do not recount every list.
  function invalidateLists() {
    queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
    queryClient.invalidateQueries({ queryKey: ['lists-with-counts', workspaceId] })
  }

  // Asks before unsaved changes are dropped. Resolves true to discard. One window at a time: a
  // second request while it is open keeps editing.
  const confirmDiscard = () =>
    new Promise<boolean>((resolve) => {
      if (confirming.current) {
        resolve(false)
        return
      }
      confirming.current = true
      modal.confirm({
        title: t`Discard your changes?`,
        content: isEdit ? t`Your changes to this list will be lost.` : t`This list is not created yet.`,
        okText: t`Discard`,
        cancelText: t`Keep editing`,
        okButtonProps: { danger: true },
        onOk: () => {
          confirming.current = false
          resolve(true)
        },
        onCancel: () => {
          confirming.current = false
          resolve(false)
        }
      })
    })

  const close = () => {
    leaving.current = true
    onClose()
  }

  const requestClose = () => {
    if (savingRef.current) return
    if (!isDirty()) {
      close()
      return
    }
    void confirmDiscard().then((discard) => {
      if (discard) close()
    })
  }
  registerClose(requestClose)

  // Browser Back while the drawer is open: unsaved changes ask first; otherwise the drawer closes
  // and the navigation goes on. Read through refs, since the blocker keeps the closure of the
  // render before a close (a create navigates in the same tick).
  useBlocker({
    shouldBlockFn: async ({ action }) => {
      // The page's own replaces (the search written to the URL after a pause, its effects) are not
      // someone leaving.
      if (action === 'REPLACE') return false
      if (leaving.current) return false
      if (savingRef.current) return true
      if (!isDirty()) {
        close()
        return false
      }
      const discard = await confirmDiscard()
      if (discard) close()
      return !discard
    },
    enableBeforeUnload: () => !leaving.current && isDirty()
  })

  const showIdError = (refusedId: string, text: string) => {
    setRefused((prev) => ({ ...prev, [refusedId]: text }))
    setIdOpen(true)
    window.setTimeout(() => idInput.current?.focus(), 0)
  }

  const failed = (error: unknown, fallback: string, sentId: string) => {
    const code = error instanceof ApiError ? (error.data as { code?: string } | undefined)?.code : undefined
    if (code === 'list_id_exists') {
      showIdError(sentId, takenMessage(sentId))
      return
    }
    if (code === 'list_id_deleted') {
      const listId = sentId
      showIdError(sentId, t`The ID “${listId}” belonged to a deleted list and cannot be reused. Choose another.`)
      return
    }
    const detail = error instanceof Error ? error.message : ''
    setSaveError(detail && !GENERIC_SERVER_ERRORS.includes(detail) ? `${fallback} ${detail}` : fallback)
  }

  const onFinish = (values: ListFormValues) => {
    // Enter in a field submits without the button: the same gate. An ID the watch has not handed
    // over yet has not been checked either.
    if (blocked || saving || (!isEdit && values.id !== id)) {
      if (idStatus === 'taken' || refusal !== null) setIdOpen(true)
      return
    }
    setSaveError(null)
    let template: TemplateReference | undefined
    if (values.is_double_optin && values.double_optin_template_id) {
      // The confirmation send reads only the template's ID (list_service.go).
      template = { id: String(values.double_optin_template_id), version: 1 }
    }
    const fields = {
      workspace_id: workspaceId,
      name: String(values.name),
      is_double_optin: !!values.is_double_optin,
      is_public: !!values.is_public,
      description: values.description ? String(values.description) : undefined,
      double_optin_template: template
    }

    if (list) {
      updateMutation.mutate(
        { ...fields, id: list.id },
        {
          onSuccess: () => {
            message.success(t`List updated`)
            invalidateLists()
            close()
          },
          onError: (error) => failed(error, t`The list could not be saved.`, list.id)
        }
      )
      return
    }

    const newId = String(values.id)
    createMutation.mutate(
      { ...fields, id: newId },
      {
        onSuccess: (response) => {
          const created: List = response?.list ?? {
            ...fields,
            id: newId,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString()
          }
          // In the page's lists before it opens the new one, which it would otherwise read as gone
          // until the refetch lands.
          queryClient.setQueryData<GetListsResponse>(['lists', workspaceId], (old) =>
            old && !old.lists.some((l) => l.id === created.id) ? { ...old, lists: [...old.lists, created] } : old
          )
          invalidateLists()
          message.success(t`List created`)
          close()
          onCreated?.(created)
        },
        onError: (error) => failed(error, t`The list could not be created.`, newId)
      }
    )
  }

  const onValuesChange = (changed: ListFormValues) => {
    setSaveError(null)
    if ('name' in changed && !isEdit && !idTouched) {
      form.setFieldsValue({ id: generateListId(changed.name ?? '') })
    }
  }

  const copyId = async () => {
    if (!list) return
    try {
      await navigator.clipboard.writeText(list.id)
      message.success(t`ID copied`)
    } catch {
      message.error(t`Could not copy the ID`)
    }
  }

  const statusLine = (() => {
    // A refusal is the answer that counts: lists.get still finds no deleted list.
    if (refusal !== null) return null
    if (idStatus === 'checking') {
      return (
        <span className="inline-flex items-center gap-1">
          <Spin size="small" /> {t`Checking…`}
        </span>
      )
    }
    if (idStatus === 'taken') return <span className="text-danger-fg">{t`already used`}</span>
    if (idStatus === 'available') return <span className="text-success-fg">{t`available`}</span>
    return null
  })()

  const createdOn = list ? i18n.date(new Date(list.created_at), { dateStyle: 'long' }) : null

  return (
    <>
      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
        {saveError ? (
          <Alert type="error" showIcon title={saveError} role="alert" style={{ marginBottom: 16 }} />
        ) : null}
        <Form
          form={form}
          layout="vertical"
          initialValues={initialValues}
          onFinish={onFinish}
          onFinishFailed={({ errorFields }) => {
            // Only an ID worth correcting: an empty one follows the name, which has its own error.
            if (errorFields.some((field) => field.name[0] === 'id') && form.getFieldValue('id')) setIdOpen(true)
          }}
          onValuesChange={onValuesChange}
          requiredMark={false}
        >
          <Form.Item
            name="name"
            label={t`Name`}
            style={{ marginBottom: isEdit || idShown ? undefined : 4 }}
            rules={[
              { required: true, message: t`Please enter a list name` },
              { max: LIST_NAME_MAX, message: t`Name must be less than 255 characters` }
            ]}
          >
            <Input placeholder={t`e.g. Newsletter, Customers, Beta`} autoFocus={!isEdit} />
          </Form.Item>

          {!isEdit && !idShown ? (
            <div className="mb-5 flex flex-wrap items-center gap-x-1.5 text-xs text-ink-3">
              {id ? (
                <>
                  <span>{t`ID`}</span>
                  <code className="font-mono text-ink-2">{id}</code>
                  <span aria-live="polite">{statusLine}</span>
                  <span aria-hidden="true">·</span>
                  <Button
                    type="link"
                    size="small"
                    htmlType="button"
                    style={{ padding: 0, height: 'auto' }}
                    onClick={() => {
                      setIdOpen(true)
                      setIdTouched(true)
                      window.setTimeout(() => idInput.current?.focus(), 0)
                    }}
                  >
                    {t`Modify`}
                  </Button>
                </>
              ) : (
                <span>{t`ID derived from the name`}</span>
              )}
            </div>
          ) : null}

          {/* Always mounted, so its rules run and the form submits it while it is collapsed. */}
          {!isEdit ? (
            <Form.Item
              name="id"
              label={t`ID`}
              hidden={!idShown}
              validateStatus={idError ? 'error' : undefined}
              help={idError ?? undefined}
              extra={
                <span>
                  {nameGivesNoId ? t`The name gives no ID: enter one.` : <span aria-live="polite">{idShown ? statusLine : null}</span>}
                  {nameGivesNoId ? null : (
                    <span className="block">
                      {t`Letters and digits, 32 at most. Used by the API and forms; cannot change after creation.`}
                    </span>
                  )}
                </span>
              }
              rules={[
                { required: true, message: t`Please enter a list ID` },
                { pattern: LIST_ID_PATTERN, message: t`ID must be alphanumeric` },
                { max: LIST_ID_MAX, message: t`ID must be less than 32 characters` }
              ]}
            >
              <Input
                ref={idInput}
                placeholder={t`e.g. members`}
                onChange={() => {
                  // Typed by hand: the field stays open and no longer follows the name.
                  setIdTouched(true)
                  setIdOpen(true)
                }}
                style={{ fontFamily: 'var(--font-mono)' }}
              />
            </Form.Item>
          ) : (
            <div className="mb-5">
              <div className="mb-2 text-ink">{t`ID`}</div>
              <div className="flex items-center gap-2">
                <code className="rounded-sm border border-line bg-subtle px-2 py-1 font-mono text-sm text-ink-2">
                  {list.id}
                </code>
                <Button size="small" icon={<Copy size={14} aria-hidden="true" />} onClick={copyId}>
                  {t`Copy`}
                </Button>
              </div>
              <div className="mt-1 text-xs text-ink-3">{t`Cannot be changed, used by the API and forms.`}</div>
            </div>
          )}

          <Form.Item
            name="description"
            label={t`Description`}
            extra={t`Shown to contacts in the notification center when the list is public.`}
          >
            <Input.TextArea rows={3} />
          </Form.Item>

          <div className="mb-1 mt-6 text-overline font-semibold uppercase text-ink-3">{t`Subscription`}</div>

          <div className="flex items-start justify-between gap-4 border-b border-line-soft py-3">
            <div>
              <div id={ids.publicLabel} className="font-medium text-ink">
                {t`Public list`}
              </div>
              <div id={ids.publicHelp} className="text-sm text-ink-3">
                {t`Contacts can subscribe or unsubscribe from the notification center.`}
              </div>
            </div>
            <Form.Item name="is_public" valuePropName="checked" noStyle>
              <Switch aria-labelledby={ids.publicLabel} aria-describedby={ids.publicHelp} />
            </Form.Item>
          </div>

          <div className="flex items-start justify-between gap-4 py-3">
            <div>
              <div id={ids.optinLabel} className="font-medium text-ink">
                {t`Double opt-in`}
              </div>
              <div id={ids.optinHelp} className="text-sm text-ink-3">
                {t`A confirmation email is sent before the subscription; the contact stays “Pending” until they click.`}
              </div>
            </div>
            <Form.Item name="is_double_optin" valuePropName="checked" noStyle>
              <Switch aria-labelledby={ids.optinLabel} aria-describedby={ids.optinHelp} />
            </Form.Item>
          </div>

          {doubleOptin ? (
            <Form.Item
              name="double_optin_template_id"
              label={t`Confirmation template`}
              rules={[{ required: true, message: t`Choose a confirmation template` }]}
            >
              <TemplateSelectorInput
                variant="card"
                workspaceId={workspaceId}
                category="opt_in"
                // The confirmation goes out through the marketing provider (list_service.go).
                sendsWith="marketing"
                placeholder={t`Choose a confirmation template`}
                subtitle={t`The confirmation email each new subscriber receives`}
                clearable={false}
                canReadTemplates={canReadTemplates}
              />
            </Form.Item>
          ) : null}

          {/* Turning double opt-in off changes no status: list_service.go UpdateList. */}
          {list?.is_double_optin && !doubleOptin && pending !== 0 ? (
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              title={
                pending === undefined
                  ? t`Turning double opt-in off does not change the “Pending” contacts: they stay pending until they confirm.`
                  : plural(pending, {
                      one: 'Turning double opt-in off does not change the # “Pending” contact: they stay pending until they confirm.',
                      other:
                        'Turning double opt-in off does not change the # “Pending” contacts: they stay pending until they confirm.'
                    })
              }
            />
          ) : null}

          {isPublic ? (
            <div className="mt-4">
              <NotificationCenterListPreview name={name} description={description} />
            </div>
          ) : null}
        </Form>
      </div>

      <div className="flex items-center gap-2 border-t border-line px-6 py-3">
        <span className="flex-1 text-xs text-ink-3">{createdOn ? t`Created on ${createdOn}` : null}</span>
        <Button onClick={requestClose} disabled={saving}>
          {t`Cancel`}
        </Button>
        <Button type="primary" onClick={() => form.submit()} loading={saving} disabled={blocked}>
          {isEdit ? t`Save` : t`Create list`}
        </Button>
      </div>
    </>
  )
}
