import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { App, Button, Dropdown, Form, Modal, Select, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { MoreHorizontal, Plus, Square } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import type { Contact } from '../../../services/api/contacts'
import { contactListApi } from '../../../services/api/contact_list'
import { listsApi } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../../services/api/types'
import { Panel, Pill } from '../../layout'
import { LIST_STATUS_LABEL, LIST_STATUS_TONE } from '../contactDisplay'

type Subscription = Contact['contact_lists'][number]

export interface ContactListsCardProps {
  workspace: Workspace
  contact: Contact
  lists: List[]
  permissions: UserPermissions | null
  /** A subscription changed: the sheet reads the contact, its activity and the table again. */
  onChanged: () => void
}

const STATUSES = ['active', 'pending', 'unsubscribed', 'bounced', 'complained'] as const

/** The contact's list subscriptions, with subscribe, change status and remove. */
export function ContactListsCard({
  workspace,
  contact,
  lists,
  permissions,
  onChanged
}: ContactListsCardProps) {
  const { t, i18n } = useLingui()
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
    mutationFn: (listId: string) =>
      listsApi.subscribe({
        workspace_id: workspace.id,
        contact: { email: contact.email } as Contact,
        list_ids: [listId]
      }),
    onSuccess: () => {
      message.success(t`Contact added to list successfully`)
      setSubscribeOpen(false)
      onChanged()
    },
    onError: (error: Error) => message.error(t`Failed to add to list: ${error.message}`)
  })

  const updateStatus = useMutation({
    mutationFn: (values: { listId: string; status: string }) =>
      contactListApi.updateStatus({
        workspace_id: workspace.id,
        email: contact.email,
        list_id: values.listId,
        status: values.status
      }),
    onSuccess: () => {
      message.success(t`Subscription status updated successfully`)
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

  const menu = (subscription: Subscription): MenuProps['items'] => [
    {
      key: 'status',
      label: t`Change status…`,
      onClick: () => {
        statusForm.setFieldsValue({ status: subscription.status })
        setStatusOf(subscription)
      }
    },
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
      title={t`Lists`}
      extra={
        canWriteLists ? (
          <Button
            type="link"
            size="small"
            icon={<Plus size={13} aria-hidden="true" />}
            disabled={available.length === 0}
            onClick={() => {
              subscribeForm.resetFields()
              setSubscribeOpen(true)
            }}
          >
            {t`Subscribe to list`}
          </Button>
        ) : undefined
      }
    >
      {subscriptions.length === 0 ? (
        <div className="text-sm text-ink-3">{t`This contact is not subscribed to any lists`}</div>
      ) : (
        <ul className="m-0 flex list-none flex-col gap-2 p-0">
          {subscriptions.map((subscription) => (
            <li key={subscription.list_id} data-contact-list className="flex items-center gap-2 text-sm">
              <Square size={12} aria-hidden="true" className="shrink-0 text-ink-4" />
              <span className="min-w-0 flex-1 truncate text-ink">{listName(subscription.list_id)}</span>
              <Pill tone={LIST_STATUS_TONE[subscription.status] ?? 'neutral'}>
                {LIST_STATUS_LABEL[subscription.status]
                  ? i18n._(LIST_STATUS_LABEL[subscription.status])
                  : subscription.status}
              </Pill>
              {subscription.created_at ? (
                <Tooltip title={dayjs(subscription.created_at).tz(timezone).format('LLL')}>
                  <span className="text-xs whitespace-nowrap text-ink-3">
                    {t`since ${dayjs(subscription.created_at).tz(timezone).format('ll')}`}
                  </span>
                </Tooltip>
              ) : null}
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
        <Form form={subscribeForm} layout="vertical" onFinish={(values) => subscribe.mutate(values.list_id)}>
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
            <Select options={STATUSES.map((value) => ({ value, label: i18n._(LIST_STATUS_LABEL[value]) }))} />
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

/** The segments the contact matches, each with when it matched. */
export function ContactSegmentsCard({ workspace, contact, segments }: ContactSegmentsCardProps) {
  const { t } = useLingui()
  const timezone = workspace.settings?.timezone || 'UTC'
  const memberships = contact.contact_segments ?? []
  return (
    <Panel title={t`Segments`}>
      {memberships.length === 0 ? (
        <div className="text-sm text-ink-3">{t`This contact is in no segment`}</div>
      ) : (
        <div className="flex flex-wrap gap-x-3 gap-y-1.5">
          {memberships.map((membership) => {
            const segment = segments.find((s) => s.id === membership.segment_id)
            return (
              <Tooltip
                key={membership.segment_id}
                title={
                  <>
                    <div>
                      {t`Matched on:`}{' '}
                      {membership.matched_at
                        ? dayjs(membership.matched_at).tz(timezone).format('LLL')
                        : t`Unknown date`}
                    </div>
                    <div className="text-xs">
                      {t`Timezone:`} {timezone}
                    </div>
                  </>
                }
              >
                <span className="flex items-center gap-1.5 text-sm text-ink-2">
                  <span
                    aria-hidden="true"
                    className="size-2 rounded-full"
                    // The segment's own colour, chosen by the user: not a theme token.
                    style={{ backgroundColor: segment?.color }}
                  />
                  {segment?.name ?? membership.segment_id}
                </span>
              </Tooltip>
            )
          })}
        </div>
      )}
    </Panel>
  )
}
