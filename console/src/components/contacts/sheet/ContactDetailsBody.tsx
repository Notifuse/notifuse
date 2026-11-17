import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Dropdown, Skeleton, Tabs } from 'antd'
import type { MenuProps } from 'antd'
import { Copy, MoreHorizontal } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { contactsApi, type Contact } from '../../../services/api/contacts'
import { ApiError } from '../../../services/api/errors'
import type { Segment } from '../../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../../services/api/types'
import { KeyValueList, MetaLine, Mono, useNumberFormat } from '../../layout'
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
import { SHEET_LABEL_WIDTH, SheetSection } from './SheetSection'
import { sheetTimezone } from './sheetDates'
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
}

// The sheet's columns, as whole class strings (Tailwind only generates the classes it finds whole).
// The centre column holds tables: three columns only from 1400px, where it keeps room for them.
const THREE_COLUMNS = {
  columns: 3,
  grid: 'grid grid-cols-1 items-start gap-4 min-[900px]:grid-cols-[280px_minmax(0,1fr)] min-[1400px]:grid-cols-[300px_minmax(0,1fr)_320px]',
  aside:
    'overflow-hidden rounded-lg border border-line bg-surface shadow-card min-[900px]:row-span-2 min-[1400px]:row-span-1',
  main: 'min-w-0 rounded-lg border border-line bg-surface px-4 pb-4 shadow-card min-[900px]:col-start-2 min-[900px]:row-start-2 min-[1400px]:row-start-1'
}
const TWO_COLUMNS = {
  columns: 2,
  grid: 'grid grid-cols-1 items-start gap-4 min-[900px]:grid-cols-[280px_minmax(0,1fr)] min-[1400px]:grid-cols-[300px_minmax(0,1fr)]',
  aside: 'overflow-hidden rounded-lg border border-line bg-surface shadow-card',
  main: 'min-w-0 rounded-lg border border-line bg-surface px-4 pb-4 shadow-card'
}

/** The sheet of one contact. Mounted per email, so its pages and cursors never mix two contacts. */
export function ContactDetailsBody({
  workspace,
  email,
  lists,
  segments,
  permissions,
  onClose
}: ContactDetailsBodyProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const workspaceId = workspace.id

  const canWriteContacts = !!permissions?.contacts?.write
  const canReadLists = !!permissions?.lists?.read
  const canReadSegments = !!permissions?.segments?.read
  const hasMemberships = canReadLists || canReadSegments
  const layout = hasMemberships ? THREE_COLUMNS : TWO_COLUMNS
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
      // A refetch already running would show the old values again once it lands
      await Promise.all([
        queryClient.cancelQueries({ queryKey: contactKey(workspaceId, email) }),
        queryClient.cancelQueries({ queryKey: ['contacts', workspaceId] })
      ])
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
  const workspaceTimezone = workspace.settings?.timezone || 'UTC'
  const timezone = sheetTimezone(contact.timezone, workspaceTimezone)
  const place = [contact.country, contact.state].filter(Boolean).join(', ')
  const messagesCount = stats.data?.messages
  const visitsCount = visits.data?.pages[0]?.summary?.sessions

  const tabLabel = (label: string, count?: number) => (
    <span className="flex items-center gap-1.5">
      {label}
      {count !== undefined ? <span className="text-xs text-ink-3 tabular-nums">{int(count)}</span> : null}
    </span>
  )

  const externalId = contact.external_id
  // Refresh and Delete sit in the drawer bar (ContactSheetTopActions): « … » keeps the copy of the
  // external ID, and is not shown without one.
  const actions: MenuProps['items'] = externalId
    ? [
        {
          key: 'external-id',
          icon: <Copy size={14} aria-hidden="true" />,
          label: t`Copy external ID`,
          onClick: async () => {
            try {
              await navigator.clipboard.writeText(externalId)
              message.success(t`External ID copied`)
            } catch {
              message.error(t`Could not copy the external ID`)
            }
          }
        }
      ]
    : []

  return (
    <>
      {/* One card, one row: the contact, its KPIs, the menu. Under 900px the KPIs wrap below,
          the menu staying on the name's row. */}
      <div
        data-contact-header
        className="flex flex-wrap overflow-hidden rounded-lg border border-line bg-surface min-[900px]:flex-nowrap"
      >
        <div className="order-1 flex min-w-0 flex-1 items-center gap-3 px-4 py-3 min-[900px]:max-w-[40%] min-[900px]:flex-none">
          <ContactAvatar contact={contact} />
          <div className="flex min-w-0 flex-col gap-0.5">
            <h1 data-unreset className="m-0 truncate text-title font-semibold text-ink">
              {name || contact.email}
            </h1>
            <MetaLine
              items={[
                <Mono key="email" copy>
                  {contact.email}
                </Mono>,
                contact.job_title || null
              ]}
            />
            {/* The place and the date on a line of their own. */}
            <MetaLine
              items={[place || null, t`Added on ${dayjs(contact.created_at).tz(timezone).format('ll')}`]}
            />
          </div>
        </div>
        <div
          data-contact-header-kpis
          className="order-3 min-w-0 basis-full border-t border-line-soft min-[900px]:order-2 min-[900px]:flex-1 min-[900px]:basis-auto min-[900px]:border-t-0 min-[900px]:border-l"
        >
          <ContactKpis
            workspaceId={workspaceId}
            email={email}
            canReadMessages={canReadMessages}
            canReadVisits={canReadVisits}
            embedded
          />
        </div>
        {actions.length > 0 ? (
          <div className="order-2 flex shrink-0 items-center px-3 min-[900px]:order-3">
            <Dropdown trigger={['click']} placement="bottomRight" menu={{ items: actions }}>
              <Button aria-label={t`More actions`} icon={<MoreHorizontal size={15} aria-hidden="true" />} />
            </Dropdown>
          </div>
        ) : null}
      </div>

      {/* Three columns from 1400px: attributes, activity, memberships. Below, the memberships go
          above the activity; under 900px everything stacks. Without lists or segments to read, two
          columns. */}
      <div data-columns={layout.columns} className={layout.grid}>
        <aside data-contact-aside className={layout.aside}>
          <ContactProfileFields
            workspace={workspace}
            contact={contact}
            canWrite={canWriteContacts}
            onSave={(key, value) => saveFields({ [key]: value })}
          />
          <SheetSection title={t`Tracking consent`}>
            <TrackingConsentEditor
              opens={contact.tracking_opens_consent}
              clicks={contact.tracking_clicks_consent}
              updatedAt={contact.tracking_consent_updated_at}
              source={contact.tracking_consent_source}
              disabled={!canWriteContacts}
              onSave={saveFields}
            />
          </SheetSection>
          <SheetSection title={t`System`}>
            <KeyValueList
              labelWidth={SHEET_LABEL_WIDTH}
              items={[
                // In the zone the activity is dated in (« Times in … »), named after the time.
                {
                  label: t`Created At`,
                  value: <ZonedTime at={contact.created_at} timezone={timezone} />
                },
                {
                  label: t`Updated At`,
                  value: <ZonedTime at={contact.updated_at} timezone={timezone} />
                }
              ]}
            />
          </SheetSection>
        </aside>

        <div data-contact-main className={layout.main}>
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
                      children: (
                        <ContactMessagesTab
                          workspace={workspace}
                          email={email}
                          permissions={permissions}
                          timezone={timezone}
                          total={messagesCount}
                        />
                      )
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

        {hasMemberships ? (
          <div
            data-contact-memberships
            className="flex min-w-0 flex-col gap-3 min-[900px]:col-start-2 min-[900px]:row-start-1 min-[1400px]:col-start-3"
          >
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
          </div>
        ) : null}
      </div>
    </>
  )
}

/** A date and time in the given zone, the zone named after it. */
function ZonedTime({ at, timezone }: { at: string; timezone: string }) {
  return (
    <span>
      {dayjs(at).tz(timezone).format('lll')} <span className="text-ink-3">{timezone}</span>
    </span>
  )
}
