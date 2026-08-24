import { useState } from 'react'
import type { ReactNode } from 'react'
import { Switch, Table, Tag, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronRight } from '@fortawesome/free-solid-svg-icons'
import { Trans, useLingui } from '@lingui/react/macro'
import {
  ALL_PERMISSION_RESOURCES,
  createEmptyPermissions,
  grantUnenforcedPermissions,
  isPermissionEnforced,
  PERMISSION_DESCRIPTORS
} from '../../services/api/permissions'
import type {
  PermissionDescriptor,
  PermissionResource,
  PermissionType,
  PermissionVerbDetail,
  StoredPermissions,
  UserPermissions
} from '../../services/api/permissions'

interface PermissionsMatrixProps {
  // A received map: it may be partial, empty or null. Every resource it leaves out is denied.
  value: StoredPermissions | null | undefined
  // Omit to render the grant read-only. The handler always receives a complete map.
  onChange?: (permissions: UserPermissions) => void
  className?: string
}

interface MatrixRow {
  key: PermissionResource
  resource: string
  read: boolean
  write: boolean
}

// One verb's worth of the expanded row: the endpoints it gates, or the reason it gates none.
function PermissionVerbSection({
  heading,
  detail
}: {
  heading: ReactNode
  detail: PermissionVerbDetail
}) {
  const { i18n } = useLingui()

  return (
    <section className="mt-3">
      <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500 m-0">
        {heading}
      </h4>
      {detail.endpoints.length > 0 ? (
        <ul className="mt-1 mb-0 pl-0 list-none space-y-1">
          {detail.endpoints.map((entry) => (
            <li key={entry.endpoint} className="text-xs leading-snug text-gray-700">
              {/* The endpoint is an identifier, never translated — and it is plain selectable
                  text, so it can be copied straight into whatever is being scoped. */}
              <code className="font-mono whitespace-nowrap text-gray-900 bg-gray-100 rounded px-1 py-px select-text">
                {entry.endpoint}
              </code>
              <span className="mx-1 text-gray-400">—</span>
              {i18n._(entry.action)}
            </li>
          ))}
        </ul>
      ) : (
        <p className="mt-1 mb-0 text-xs leading-snug text-gray-600 italic">
          {detail.note ? (
            i18n._(detail.note)
          ) : (
            <Trans>This permission gates no endpoint today.</Trans>
          )}
        </p>
      )}
    </section>
  )
}

// Deliberately unsized: this renders inside an expanded table row, in normal document flow, so it
// is the host drawer's body that scrolls. A height cap and a fixed width here would only
// reintroduce a second scroll region inside the first.
function PermissionDetails({ descriptor }: { descriptor: PermissionDescriptor }) {
  const { i18n } = useLingui()

  return (
    <div>
      <p className="m-0 text-xs leading-snug text-gray-700">{i18n._(descriptor.scope)}</p>

      <PermissionVerbSection heading={<Trans>Read</Trans>} detail={descriptor.read} />
      <PermissionVerbSection heading={<Trans>Write</Trans>} detail={descriptor.write} />

      {descriptor.caveat && (
        <div className="mt-3 border-l-2 border-amber-400 bg-amber-50 rounded-r px-2 py-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-amber-700 m-0">
            <Trans>Watch out</Trans>
          </h4>
          <p className="mt-1 mb-0 text-xs leading-snug text-amber-900">
            {i18n._(descriptor.caveat)}
          </p>
        </div>
      )}
    </div>
  )
}

export function PermissionsMatrix({ value, onChange, className }: PermissionsMatrixProps) {
  const { t } = useLingui()
  const editable = Boolean(onChange)

  // One row at a time. Several resources gate two dozen endpoints — Contacts alone gates 23 — so
  // letting them stack would bury the switches being compared under a wall of text and leave the
  // host drawer scrolling for pages. Opening a row therefore closes the previous one.
  const [expandedResource, setExpandedResource] = useState<PermissionResource | null>(null)

  const resourceLabels: Record<PermissionResource, string> = {
    contacts: t`Contacts`,
    lists: t`Lists`,
    templates: t`Templates`,
    broadcasts: t`Broadcasts`,
    transactional: t`Transactional`,
    workspace: t`Workspace`,
    message_history: t`Message History`,
    blog: t`Blog`,
    automations: t`Automations`,
    llm: t`LLM`,
    web_analytics: t`Web Analytics`,
    segments: t`Segments`,
    webhook_subscriptions: t`Webhook Subscriptions`,
    webhook_events: t`Webhook Events`
  }

  // The canonical resource list drives the rows, never the keys of `value`: a resource the stored
  // map omits still gets a row, which is what makes it grantable instead of frozen at denied.
  const rows: MatrixRow[] = ALL_PERMISSION_RESOURCES.map((resource) => {
    const granted = value?.[resource] ?? { read: false, write: false }
    return {
      key: resource,
      resource: resourceLabels[resource],
      read: granted.read || (editable && !isPermissionEnforced(resource, 'read')),
      write: granted.write || (editable && !isPermissionEnforced(resource, 'write'))
    }
  })

  const setPermission = (resource: PermissionResource, type: PermissionType, checked: boolean) => {
    if (!onChange) return

    // Built from the rows rather than from `value`, so a resource the stored map omitted is sent
    // explicitly denied instead of silently dropped.
    const next = createEmptyPermissions()
    for (const row of rows) {
      next[row.key] = { read: row.read, write: row.write }
    }
    next[resource] = { ...next[resource], [type]: checked }

    onChange(grantUnenforcedPermissions(next))
  }

  const renderSwitch =
    (type: PermissionType) => (checked: boolean, record: MatrixRow) => {
      const control = (
        <Switch
          checked={checked}
          disabled={!isPermissionEnforced(record.key, type)}
          onChange={(next) => setPermission(record.key, type, next)}
          size="small"
        />
      )

      if (isPermissionEnforced(record.key, type)) {
        return control
      }

      return (
        <Tooltip
          title={t`This permission is not enforced yet, so it is always granted. It cannot be turned off.`}
        >
          <span className="inline-block">{control}</span>
        </Tooltip>
      )
    }

  const renderTag = (type: PermissionType) => (granted: boolean) => (
    <Tag color={granted ? 'green' : 'red'}>
      {granted ? (type === 'read' ? t`Read` : t`Write`) : t`No`}
    </Tag>
  )

  // The endpoint list opens as an expanded row rather than an overlay: it is far too long to be
  // popover-shaped, and in normal flow it can never land off-screen the way an overlay anchored to
  // a row halfway down a drawer did.
  //
  // Chevron and label are one button, not two controls that happen to do the same thing: the
  // resource name is the obvious thing to click, and a screen reader should meet a single
  // expander per row carrying the row's own accessible name. The switches sit in their own
  // cells, outside this button, so operating one never toggles the row.
  //
  // Only on the editable matrix. The read-only one is itself the body of a hover Popover on the
  // Team page (the permission-count tag in WorkspaceMembers), and a click-to-expand inside a
  // hover overlay is unusable — the overlay closes the moment the pointer leaves the tag.
  const renderResource = (label: string, record: MatrixRow) => {
    const expanded = expandedResource === record.key

    return (
      <button
        type="button"
        aria-expanded={expanded}
        aria-label={t`Endpoints gated by ${label}`}
        onClick={() => setExpandedResource(expanded ? null : record.key)}
        // The negative margin buys a target taller and wider than the text without moving the
        // text itself, so the column stays aligned with the header and with the switches.
        className="group inline-flex items-center gap-2 text-left cursor-pointer border-0 bg-transparent -m-1 p-1"
      >
        <FontAwesomeIcon
          icon={faChevronRight}
          className={`text-xs text-gray-400 transition-transform group-hover:text-gray-600 ${
            expanded ? 'rotate-90' : ''
          }`}
        />
        <span>{label}</span>
      </button>
    )
  }

  const columns: ColumnsType<MatrixRow> = editable
    ? [
        {
          title: t`Resource`,
          dataIndex: 'resource',
          key: 'resource',
          width: '40%',
          render: renderResource
        },
        {
          title: t`Read`,
          dataIndex: 'read',
          key: 'read',
          width: '30%',
          render: renderSwitch('read')
        },
        {
          title: t`Write`,
          dataIndex: 'write',
          key: 'write',
          width: '30%',
          render: renderSwitch('write')
        }
      ]
    : [
        {
          dataIndex: 'resource',
          key: 'resource',
          width: 120
        },
        {
          dataIndex: 'read',
          key: 'read',
          width: 60,
          render: renderTag('read')
        },
        {
          dataIndex: 'write',
          key: 'write',
          width: 60,
          render: renderTag('write')
        }
      ]

  return (
    <Table
      dataSource={rows}
      columns={columns}
      pagination={false}
      showHeader={editable}
      size="small"
      // No expand column of its own: the chevron lives in the resource cell, next to the label it
      // belongs to, instead of adding a near-empty first column to a three-column table.
      expandable={
        editable
          ? {
              expandedRowKeys: expandedResource ? [expandedResource] : [],
              showExpandColumn: false,
              expandedRowRender: (record: MatrixRow) => (
                <PermissionDetails descriptor={PERMISSION_DESCRIPTORS[record.key]} />
              )
            }
          : undefined
      }
      className={className ?? (editable ? 'border border-gray-200 rounded-md' : 'min-w-64')}
    />
  )
}
