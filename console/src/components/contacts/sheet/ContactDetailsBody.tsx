import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dropdown, Skeleton, Tabs } from 'antd'
import { MoreHorizontal, RefreshCw } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { contactsApi, type Contact } from '../../../services/api/contacts'
import { ApiError } from '../../../services/api/errors'
import type { Segment } from '../../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../../services/api/types'
import { KeyValueList, MetaLine, Mono, Panel, useNumberFormat } from '../../layout'
import { BlockError } from '../BlockError'
import { TrackingConsentEditor } from '../TrackingConsent'
import { contactDisplayName, contactInitials } from '../contactDisplay'
import type { ContactsTableData } from '../useContactsTableQuery'
import { ContactWebVisits } from '../visits/ContactWebVisits'
import { ContactActivityTab } from './ContactActivityTab'
import { ContactKpis } from './ContactKpis'
import { ContactListsCard, ContactSegmentsCard } from './ContactListsCard'
import { ContactMessagesTab } from './ContactMessagesTab'
import { ContactProfileFields } from './ContactProfileFields'
import { useContactMessageStats, useContactVisits } from './useContactSheetQueries'
import { contactKey } from './contactKey'

type FieldValues = Record<string, string | number | object | null>

/** Gravatar's SHA-256 address of an email, with its « mystery person » default (d=mp). */
async function gravatarUrl(email: string, size: number): Promise<string | undefined> {
  try {
    const data = new TextEncoder().encode(email.trim().toLowerCase())
    const hash = await crypto.subtle.digest('SHA-256', data)
    const hex = Array.from(new Uint8Array(hash), (b) => b.toString(16).padStart(2, '0')).join('')
    return `https://www.gravatar.com/avatar/${hex}?s=${size}&d=mp`
  } catch {
    return undefined
  }
}

/** The contact's photo, with its initials until the photo loads or when it fails. */
function ContactAvatar({ contact }: { contact: Contact }) {
  const [src, setSrc] = useState<string>()
  const [loaded, setLoaded] = useState(false)
  useEffect(() => {
    let live = true
    gravatarUrl(contact.email, 80).then((url) => live && setSrc(url))
    return () => {
      live = false
    }
  }, [contact.email])
  return (
    <span className="relative flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary-50 text-sm font-semibold text-primary-700">
      {loaded ? null : <span aria-hidden="true">{contactInitials(contact)}</span>}
      {src ? (
        <img
          src={src}
          alt=""
          className={loaded ? 'size-10' : 'absolute size-0'}
          onLoad={() => setLoaded(true)}
          onError={() => setLoaded(false)}
        />
      ) : null}
    </span>
  )
}

export interface ContactDetailsBodyProps {
  workspace: Workspace
  email: string
  lists: List[]
  segments: Segment[]
  permissions: UserPermissions | null
  onClose: () => void
  onDelete: (email: string) => void
}

/** The sheet of one contact. Mounted per email, so its pages and cursors never mix two contacts. */
export function ContactDetailsBody({
  workspace,
  email,
  lists,
  segments,
  permissions,
  onClose,
  onDelete
}: ContactDetailsBodyProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const queryClient = useQueryClient()
  const workspaceId = workspace.id

  const canWriteContacts = !!permissions?.contacts?.write
  const canReadLists = !!permissions?.lists?.read
  const canReadSegments = !!permissions?.segments?.read
  const canReadMessages = !!permissions?.message_history?.read
  const canReadVisits = !!workspace.settings?.web_analytics?.enabled && !!permissions?.web_analytics?.read

  // contacts.getByEmail is an exact match: the list filter on email is an ILIKE, which opened
  // joann@x.com for ann@x.com.
  const contactQuery = useQuery({
    queryKey: contactKey(workspaceId, email),
    queryFn: async () => (await contactsApi.getByEmail({ workspace_id: workspaceId, email })).contact,
    retry: (count, error) => !(error instanceof ApiError && error.status === 404) && count < 2
  })
  const stats = useContactMessageStats(workspaceId, email, canReadMessages)
  const visits = useContactVisits(workspaceId, email, 'all', canReadVisits)

  const refreshActivity = () =>
    queryClient.invalidateQueries({ queryKey: ['contact_timeline', workspaceId, email] })

  const refreshAll = () => {
    contactQuery.refetch()
    refreshActivity()
    queryClient.invalidateQueries({ queryKey: ['message_history', workspaceId, email] })
    queryClient.invalidateQueries({ queryKey: ['contact-message-stats', workspaceId, email] })
    queryClient.invalidateQueries({ queryKey: ['contact-visits', workspaceId, email] })
  }

  const afterListChange = () => {
    queryClient.invalidateQueries({ queryKey: contactKey(workspaceId, email) })
    queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
    queryClient.invalidateQueries({ queryKey: ['list-stats', workspaceId] })
    refreshActivity()
  }

  // Writes several fields in one upsert, so they make one change in the activity.
  const saveFields = async (fields: FieldValues) => {
    const response = await contactsApi.upsert({ workspace_id: workspaceId, contact: { email, ...fields } })
    if (response.action === 'error') throw new Error(response.error || t`Failed to update field`)
    const stored = response.contact
    if (stored) {
      // The read-back omits a field stored as NULL (omitempty), so a cleared field is cleared here
      // from what was sent. Lists and segments keep the cached ones, which their own card refreshes.
      const cleared = Object.fromEntries(Object.entries(fields).filter(([, value]) => value === null))
      const merge = (old: Contact): Contact => ({
        ...old,
        ...stored,
        ...cleared,
        contact_lists: old.contact_lists,
        contact_segments: old.contact_segments
      })
      queryClient.setQueryData<Contact>(contactKey(workspaceId, email), (old) => (old ? merge(old) : old))
      queryClient.setQueriesData<ContactsTableData>({ queryKey: ['contacts', workspaceId] }, (old) =>
        old ? { ...old, contacts: old.contacts.map((row) => (row.email === email ? merge(row) : row)) } : old
      )
    } else {
      queryClient.invalidateQueries({ queryKey: contactKey(workspaceId, email) })
      queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
    }
    refreshActivity()
  }

  if (contactQuery.isError && contactQuery.error instanceof ApiError && contactQuery.error.status === 404) {
    return (
      <div className="flex flex-col items-center gap-3 py-16 text-sm">
        <span className="text-ink-2">{t`Contact not found`}</span>
        <Button onClick={onClose}>{t`Close`}</Button>
      </div>
    )
  }
  if (contactQuery.isError) return <BlockError onRetry={() => contactQuery.refetch()} />

  const contact = contactQuery.data
  if (!contact) return <Skeleton active avatar paragraph={{ rows: 8 }} />

  const name = contactDisplayName(contact)
  const timezone = contact.timezone || workspace.settings?.timezone || 'UTC'
  const workspaceTimezone = workspace.settings?.timezone || 'UTC'
  const place = [contact.country, contact.state].filter(Boolean).join(', ')
  const messagesCount = stats.data?.messages
  const visitsCount = visits.data?.pages[0]?.summary?.sessions

  const tabLabel = (label: string, count?: number) => (
    <span className="flex items-center gap-1.5">
      {label}
      {count !== undefined ? <span className="text-xs text-ink-3 tabular-nums">{int(count)}</span> : null}
    </span>
  )

  return (
    <>
      <div className="flex items-start justify-between gap-4 rounded-lg border border-line bg-surface px-4 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <ContactAvatar contact={contact} />
          <div className="flex min-w-0 flex-col gap-0.5">
            <h1 data-unreset className="m-0 truncate text-title font-semibold text-ink">
              {name || contact.email}
            </h1>
            <Mono copy>{contact.email}</Mono>
            <MetaLine
              items={[
                contact.job_title || null,
                place || null,
                t`Added on ${dayjs(contact.created_at).tz(workspaceTimezone).format('ll')}`
              ]}
            />
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            icon={<RefreshCw size={14} aria-hidden="true" />}
            loading={contactQuery.isFetching}
            onClick={refreshAll}
          >
            {t`Refresh`}
          </Button>
          {canWriteContacts ? (
            <Dropdown
              trigger={['click']}
              placement="bottomRight"
              menu={{
                items: [
                  { key: 'delete', label: t`Delete contact`, danger: true, onClick: () => onDelete(email) }
                ]
              }}
            >
              <Button aria-label={t`More actions`} icon={<MoreHorizontal size={15} aria-hidden="true" />} />
            </Dropdown>
          ) : null}
        </div>
      </div>

      <ContactKpis
        workspaceId={workspaceId}
        email={email}
        canReadMessages={canReadMessages}
        canReadVisits={canReadVisits}
      />

      <div className="flex flex-col gap-4 min-[1200px]:flex-row min-[1200px]:items-start">
        <div className="min-w-0 flex-1 rounded-lg border border-line bg-surface px-4 pb-4">
          <Tabs
            items={[
              {
                key: 'activity',
                label: t`Activity`,
                children: (
                  <ContactActivityTab
                    workspace={workspace}
                    email={email}
                    timezone={timezone}
                    segments={segments}
                    canReadMessages={canReadMessages}
                    canReadVisits={canReadVisits}
                  />
                )
              },
              ...(canReadMessages
                ? [
                    {
                      key: 'messages',
                      label: tabLabel(t`Messages`, messagesCount),
                      children: <ContactMessagesTab workspace={workspace} email={email} />
                    }
                  ]
                : []),
              ...(canReadVisits
                ? [
                    {
                      key: 'visits',
                      label: tabLabel(t`Web visits`, visitsCount),
                      children: <ContactWebVisits workspace={workspace} email={email} />
                    }
                  ]
                : [])
            ]}
          />
        </div>

        <aside
          data-contact-aside
          className="flex flex-col gap-3 rounded-lg bg-subtle p-3 [&>*]:shrink-0 min-[1200px]:sticky min-[1200px]:top-14 min-[1200px]:max-h-[calc(100vh-72px)] min-[1200px]:w-aside min-[1200px]:shrink-0 min-[1200px]:overflow-y-auto"
        >
          <ContactProfileFields
            workspace={workspace}
            contact={contact}
            canWrite={canWriteContacts}
            onSave={(key, value) => saveFields({ [key]: value })}
          />
          <Panel title={t`Tracking consent`}>
            <TrackingConsentEditor
              opens={contact.tracking_opens_consent}
              clicks={contact.tracking_clicks_consent}
              updatedAt={contact.tracking_consent_updated_at}
              source={contact.tracking_consent_source}
              disabled={!canWriteContacts}
              onSave={saveFields}
            />
          </Panel>
          <Panel title={t`System`}>
            <KeyValueList
              items={[
                {
                  label: t`Created At`,
                  value: dayjs(contact.created_at).tz(workspaceTimezone).format('lll')
                },
                {
                  label: t`Updated At`,
                  value: dayjs(contact.updated_at).tz(workspaceTimezone).format('lll')
                }
              ]}
            />
          </Panel>
          {canReadLists ? (
            <ContactListsCard
              workspace={workspace}
              contact={contact}
              lists={lists}
              permissions={permissions}
              onChanged={afterListChange}
            />
          ) : null}
          {canReadSegments ? (
            <ContactSegmentsCard workspace={workspace} contact={contact} segments={segments} />
          ) : null}
        </aside>
      </div>
    </>
  )
}
