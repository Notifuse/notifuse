import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { App, Button, Dropdown, Form, Modal, Select, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { List as ListIcon, MoreHorizontal, Plus } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import type { Contact } from '../../../services/api/contacts'
import { contactListApi } from '../../../services/api/contact_list'
import { listsApi } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../../services/api/types'
import { Panel, Pill, useNumberFormat } from '../../layout'
import { segmentDotColor } from '../../segment/segmentColor'
import { LIST_STATUS_LABEL, LIST_STATUS_TONE, canResubscribe, contactListStatusOptions } from '../contactDisplay'
import { CountBadge } from './SheetSection'

type Subscription = Contact['contact_lists'][number]

export interface ContactListsCardProps {
  workspace: Workspace
  contact: Contact
  lists: List[]
  permissions: UserPermissions | null
  /** A subscription changed: the sheet reads the contact, its activity and the table again. */
  onChanged: () => void
}

/** The contact's list subscriptions, with subscribe, change status and remove. */
export function ContactListsCard({
  workspace,
  contact,
  lists,
  permissions,
  onChanged
}: ContactListsCardProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const { message, modal } = App.useApp()
  const [subscribeOpen, setSubscribeOpen] = useState(false)
  const [statusOf, setStatusOf] = useState<Subscription | null>(null)
  const [subscribeForm] = Form.useForm<{ list_id: string }>()
  const [statusForm] = Form.useForm<{ status: string }>()

  const canWriteLists = !!permissions?.lists?.write
  // contactLists.updateStatus and removeContact need both rights.
  const canEditSubscriptions = canWriteLists && !!permissions?.contacts?.write
  const subscriptions = contact.contact_lists ?? []
  const listName = (id: string) => lists.find((list) => list.id === id)?.name ?? id
  const available = lists.filter((list) => !subscriptions.some((s) => s.list_id === list.id))
  const timezone = workspace.settings?.timezone || 'UTC'

  const subscribe = useMutation({
    mutationFn: (values: { listId: string; resubscribe?: boolean }) =>
      listsApi.subscribe({
        workspace_id: workspace.id,
        contact: { email: contact.email } as Contact,
        list_ids: [values.listId],
        ...(values.resubscribe ? { resubscribe: true } : {})
      }),
    onSuccess: (response, values) => {
      setSubscribeOpen(false)
      onChanged()
      const status = response.contact_lists?.find((cl) => cl.list_id === values.listId)?.status
      const name = listName(values.listId)
      // The server keeps an earlier opt-out: say so rather than report a subscription.
      if (status === 'unsubscribed') {
        modal.confirm({
          title: t`${contact.email} unsubscribed from ${name}`,
          content: t`They stay unsubscribed. Only re-subscribe them if they asked you to. They get no confirmation email.`,
          okText: t`Re-subscribe`,
          cancelText: t`Keep unsubscribed`,
          onOk: () => subscribe.mutateAsync({ listId: values.listId, resubscribe: true })
        })
        return
      }
      if (status === 'bounced' || status === 'complained') {
        message.warning(
          status === 'bounced'
            ? t`Not subscribed: this address bounced on ${name}`
            : t`Not subscribed: this contact reported emails from ${name} as spam`
        )
        return
      }
      message.success(t`Contact added to list successfully`)
    },
    onError: (error: Error) => message.error(t`Failed to add to list: ${error.message}`)
  })

  const updateStatus = useMutation({
    mutationFn: (values: { listId: string; status: string; resubscribe?: boolean }) =>
      contactListApi.updateStatus({
        workspace_id: workspace.id,
        email: contact.email,
        list_id: values.listId,
        status: values.status,
        ...(values.resubscribe ? { resubscribe: true } : {})
      }),
    onSuccess: (_, values) => {
      message.success(values.resubscribe ? t`Contact re-subscribed` : t`Subscription status updated successfully`)
      setStatusOf(null)
      onChanged()
    },
    onError: (error: Error) => message.error(t`Failed to update status: ${error.message}`)
  })

  const remove = useMutation({
    mutationFn: (listId: string) =>
      contactListApi.removeContact({ workspace_id: workspace.id, email: contact.email, list_id: listId }),
    onSuccess: () => {
      message.success(t`Contact removed from the list`)
      onChanged()
    },
    onError: (error: Error) => message.error(error.message)
  })

  const confirmResubscribe = (subscription: Subscription) =>
    modal.confirm({
      title: t`Re-subscribe ${contact.email} to ${listName(subscription.list_id)}?`,
      content:
        subscription.status === 'bounced'
          ? t`Their address bounced on this list. Only re-subscribe them if it works again. They get no confirmation email.`
          : t`They unsubscribed from this list. Only re-subscribe them if they asked you to. They get no confirmation email.`,
      okText: t`Re-subscribe`,
      cancelText: t`Cancel`,
      onOk: () =>
        updateStatus.mutateAsync({ listId: subscription.list_id, status: 'active', resubscribe: true })
    })

  const menu = (subscription: Subscription): MenuProps['items'] => [
    ...(canResubscribe(subscription.status)
      ? [{ key: 'resubscribe', label: t`Re-subscribe…`, onClick: () => confirmResubscribe(subscription) }]
      : []),
    // A complaint is final: there is no status to move it to.
    ...(contactListStatusOptions(subscription.status).length > 1
      ? [
          {
            key: 'status',
            label: t`Change status…`,
            onClick: () => {
              statusForm.setFieldsValue({ status: subscription.status })
              setStatusOf(subscription)
            }
          }
        ]
      : []),
    {
      key: 'remove',
      label: t`Remove from list`,
      danger: true,
      onClick: () =>
        modal.confirm({
          title: t`Remove from list`,
          content: t`Remove ${contact.email} from ${listName(subscription.list_id)}?`,
          okText: t`Remove`,
          cancelText: t`Cancel`,
          okButtonProps: { danger: true },
          onOk: () => remove.mutateAsync(subscription.list_id)
        })
    }
  ]

  return (
    <Panel
      flush
      title={
        <span className="flex items-center gap-1.5">
          {t`Lists`}
          <CountBadge count={int(subscriptions.length)} />
        </span>
      }
      extra={
        canWriteLists ? (
          <Tooltip title={t`Subscribe to list`}>
            <Button
              type="text"
              size="small"
              aria-label={t`Subscribe to list`}
              icon={<Plus size={14} aria-hidden="true" />}
              disabled={available.length === 0}
              onClick={() => {
                subscribeForm.resetFields()
                setSubscribeOpen(true)
              }}
            />
          </Tooltip>
        ) : undefined
      }
    >
      {subscriptions.length === 0 ? (
        <div className="px-3.5 py-3 text-sm text-ink-3">{t`This contact is not subscribed to any lists`}</div>
      ) : (
        <ul className="m-0 list-none p-0">
          {subscriptions.map((subscription) => (
            <li
              key={subscription.list_id}
              data-contact-list
              className="flex flex-col gap-0.5 border-b border-line-soft py-2 pr-2 pl-3.5 last:border-b-0"
            >
              <div className="flex min-h-6 items-center gap-2 text-sm">
                <ListIcon size={12} aria-hidden="true" className="shrink-0 text-ink-4" />
                <span className="min-w-0 flex-1 truncate font-medium text-ink">
                  {listName(subscription.list_id)}
                </span>
                <Pill tone={LIST_STATUS_TONE[subscription.status] ?? 'neutral'}>
                  {LIST_STATUS_LABEL[subscription.status]
                    ? i18n._(LIST_STATUS_LABEL[subscription.status])
                    : subscription.status}
                </Pill>
                {canEditSubscriptions ? (
                  <Dropdown menu={{ items: menu(subscription) }} trigger={['click']} placement="bottomRight">
                    <Button
                      type="text"
                      size="small"
                      aria-label={t`Actions for ${listName(subscription.list_id)}`}
                      icon={<MoreHorizontal size={14} aria-hidden="true" />}
                    />
                  </Dropdown>
                ) : null}
              </div>
              {subscription.created_at ? (
                <Tooltip title={dayjs(subscription.created_at).tz(timezone).format('LLL')} placement="bottomLeft">
                  <span className="self-start pl-5 text-xs text-ink-3">
                    {t`Since ${dayjs(subscription.created_at).tz(timezone).format('ll')}`}
                  </span>
                </Tooltip>
              ) : null}
            </li>
          ))}
        </ul>
      )}

      <Modal
        title={t`Subscribe to List`}
        open={subscribeOpen}
        onCancel={() => setSubscribeOpen(false)}
        onOk={() => subscribeForm.submit()}
        okText={t`Subscribe`}
        confirmLoading={subscribe.isPending}
      >
        <Form
          form={subscribeForm}
          layout="vertical"
          onFinish={(values) => subscribe.mutate({ listId: values.list_id })}
        >
          <Form.Item
            name="list_id"
            label={t`Select List`}
            rules={[{ required: true, message: t`Please select a list` }]}
          >
            <Select
              options={available.map((list) => ({ label: list.name, value: list.id }))}
              placeholder={t`Select a list`}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={statusOf ? t`Change Status for ${listName(statusOf.list_id)}` : undefined}
        open={!!statusOf}
        onCancel={() => setStatusOf(null)}
        onOk={() => statusForm.submit()}
        okText={t`Update Status`}
        confirmLoading={updateStatus.isPending}
      >
        <Form
          form={statusForm}
          layout="vertical"
          onFinish={(values) =>
            statusOf && updateStatus.mutate({ listId: statusOf.list_id, status: values.status })
          }
        >
          <Form.Item
            name="status"
            label={t`Subscription Status`}
            rules={[{ required: true, message: t`Please select a status` }]}
          >
            <Select
              options={contactListStatusOptions(statusOf?.status ?? 'active').map((value) => ({
                value,
                label: i18n._(LIST_STATUS_LABEL[value])
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </Panel>
  )
}

export interface ContactSegmentsCardProps {
  workspace: Workspace
  contact: Contact
  segments: Segment[]
}

/** The segments the contact matches, one per line with when it matched. */
export function ContactSegmentsCard({ workspace, contact, segments }: ContactSegmentsCardProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const timezone = workspace.settings?.timezone || 'UTC'
  const memberships = contact.contact_segments ?? []
  return (
    <Panel
      flush
      title={
        <span className="flex items-center gap-1.5">
          {t`Segments`}
          <CountBadge count={int(memberships.length)} />
        </span>
      }
    >
      {memberships.length === 0 ? (
        <div className="px-3.5 py-3 text-sm text-ink-3">{t`This contact is in no segment`}</div>
      ) : (
        <ul className="m-0 list-none py-1 pl-0">
          {memberships.map((membership) => {
            const segment = segments.find((s) => s.id === membership.segment_id)
            const matched = membership.matched_at ? dayjs(membership.matched_at).tz(timezone) : null
            return (
              <li
                key={membership.segment_id}
                data-contact-segment
                className="flex items-center justify-between gap-2 px-3.5 py-1"
              >
                <span className="inline-flex h-5.5 min-w-0 items-center gap-1.5 rounded-full border border-line bg-surface px-2 text-xs text-ink-2">
                  <span
                    aria-hidden="true"
                    className="size-1.5 shrink-0 rounded-full"
                    // The segment's own colour, chosen by the user: not a theme token.
                    style={{ backgroundColor: segmentDotColor(segment?.color) }}
                  />
                  <span className="truncate">{segment?.name ?? membership.segment_id}</span>
                </span>
                {matched ? (
                  <Tooltip title={`${matched.format('LLL')} (${timezone})`}>
                    <span className="shrink-0 text-xs text-ink-3">{t`since ${matched.format('ll')}`}</span>
                  </Tooltip>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}
    </Panel>
  )
}
