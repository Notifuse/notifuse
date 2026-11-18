import { useState, type ReactNode } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { App, Button, Dropdown, Popconfirm, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { Braces, Download, MoreHorizontal, Pencil, Plus, Upload } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { deleteSegment, type Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { Mono, PageHeader, Pill } from '../layout'
import { DeleteListModal } from '../lists/DeleteListModal'
import { segmentDotColor } from '../segment/segmentColor'
import { BulkUpdateDrawer } from './BulkUpdateDrawer'
import { ContactUpsertDrawer } from './ContactUpsertDrawer'
import { useContactsCsvUpload } from './ContactsCsvUploadProvider'
import type { ContactsView } from './contactsView'
import { SegmentActions } from './SegmentDefinedBy'
import { SegmentBuildingIndicator } from './SegmentProgress'

export interface ContactsViewHeaderProps {
  workspace: Workspace
  view: ContactsView
  /** The list of a list view, once loaded. */
  list?: List
  /** The segment of a segment view, once loaded. */
  segment?: Segment
  lists: List[]
  permissions: UserPermissions | null
  /** The member's role: an owner manages custom fields whatever the permission map says. */
  role: 'root' | 'owner' | 'member' | undefined
  /** The view's total, without the row filters nor the status tab: shown next to the title. */
  total?: number
  /** The workspace's contacts, for the share the segment drawer's count shows. */
  workspaceTotal?: number
  onExport: () => void
  onContactCreated: () => void
  /** Opens the settings drawer of the open list. */
  onEditList: () => void
  /** The open list or segment was deleted: the page goes back to All contacts. */
  onViewDeleted: () => void
}

const icon = (Icon: typeof Plus) => <Icon size={14} aria-hidden="true" />

/** The title and the actions of the view: all contacts, a list or a segment. */
export function ContactsViewHeader({
  workspace,
  view,
  list,
  segment,
  lists,
  permissions,
  role,
  total,
  workspaceTotal,
  onExport,
  onContactCreated,
  onEditList,
  onViewDeleted
}: ContactsViewHeaderProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { openDrawer, openDrawerWithSelectedList } = useContactsCsvUpload()
  const [bulkUpdateOpen, setBulkUpdateOpen] = useState(false)
  const [deleteListOpen, setDeleteListOpen] = useState(false)
  const [confirmSegmentDelete, setConfirmSegmentDelete] = useState(false)

  const workspaceId = workspace.id
  const canReadContacts = !!permissions?.contacts?.read
  const canWriteContacts = !!permissions?.contacts?.write
  const canWriteLists = !!permissions?.lists?.write
  const canWriteSegments = !!permissions?.segments?.write
  // As the settings page decides: an owner, or workspace write.
  const canManageCustomFields = role === 'owner' || role === 'root' || !!permissions?.workspace?.write

  const remove = useMutation({
    mutationFn: (id: string) => deleteSegment({ workspace_id: workspaceId, id }),
    onSuccess: () => {
      message.success(t`Segment deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })
      onViewDeleted()
    },
    onError: (error: Error) => message.error(error?.message || t`Failed to delete segment`)
  })

  // The items every view shares, then the view's own Delete last. A divider only between two groups.
  const groups: NonNullable<MenuProps['items']>[] = [
    [
      ...(canReadContacts ? [{ key: 'export', icon: icon(Download), label: t`Export`, onClick: onExport }] : []),
      ...(canWriteContacts
        ? [{ key: 'bulk', icon: icon(Pencil), label: t`Bulk update (CSV)`, onClick: () => setBulkUpdateOpen(true) }]
        : [])
    ],
    [
      ...(canManageCustomFields
        ? [
            {
              key: 'custom-fields',
              icon: icon(Braces),
              label: t`Manage custom fields`,
              onClick: () =>
                navigate({
                  to: '/console/workspace/$workspaceId/settings/$section',
                  params: { workspaceId, section: 'custom-fields' }
                })
            }
          ]
        : [])
    ]
  ]
  if (view.kind === 'list' && canWriteLists) {
    groups.push([{ key: 'delete', label: t`Delete list`, danger: true, onClick: () => setDeleteListOpen(true) }])
  }
  if (view.kind === 'segment' && canWriteSegments && segment) {
    groups.push([
      {
        key: 'delete',
        label: t`Delete segment`,
        danger: true,
        // One object: a popconfirm over « More actions » (a list or many contacts get a dialog).
        onClick: () => setConfirmSegmentDelete(true)
      }
    ])
  }
  const menu: MenuProps['items'] = groups
    .filter((group) => group.length > 0)
    .flatMap((group, index) => (index > 0 ? [{ type: 'divider' as const }, ...group] : group))

  const moreButton =
    menu.length > 0 ? (
      <Dropdown menu={{ items: menu }} trigger={['click']} placement="bottomRight">
        <Button aria-label={t`More actions`} icon={<MoreHorizontal size={15} aria-hidden="true" />} />
      </Dropdown>
    ) : null
  const segmentName = segment?.name ?? ''
  // Always the same tree around the button, so that the segment loading does not remount the
  // menu while it is open.
  const moreMenu = moreButton ? (
    <Popconfirm
      open={confirmSegmentDelete && view.kind === 'segment' && !!segment}
      // Only Delete segment opens it: a click on « More actions » opens the menu.
      onOpenChange={(open) => {
        if (!open) setConfirmSegmentDelete(false)
      }}
      title={t`Delete segment`}
      description={
        <div className="max-w-72">
          {t`"${segmentName}" will be deleted. Its contacts stay in the workspace. This action cannot be undone.`}
        </div>
      }
      okText={t`Delete`}
      cancelText={t`Cancel`}
      okButtonProps={{ danger: true, loading: remove.isPending }}
      placement="bottomRight"
      // A refusal is toasted by the mutation; the confirmation closes either way.
      onConfirm={() =>
        remove
          .mutateAsync(segment?.id ?? '')
          .catch(() => undefined)
          .finally(() => setConfirmSegmentDelete(false))
      }
    >
      {/* The anchor of the confirmation: the menu keeps its own trigger on the button. */}
      <span className="inline-flex">{moreButton}</span>
    </Popconfirm>
  ) : null

  // The number is stale while a segment is computed: none, as in the rail.
  const count = view.kind === 'segment' && segment?.status === 'building' ? undefined : total

  let header: ReactNode
  if (view.kind === 'list') {
    header = (
      <PageHeader
        title={list?.name ?? view.listId}
        count={count}
        // Under the title, one line: the list ID, then the description.
        description={
          list ? (
            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5">
              <span data-list-id className="flex min-w-0 items-center gap-1.5">
                <span>{t`ID`}</span>
                <Mono>{list.id}</Mono>
              </span>
              {list.description ? <span>{list.description}</span> : null}
            </div>
          ) : undefined
        }
        status={
          list ? (
            <Tooltip title={list.is_public ? t`Visible in the notification center` : undefined}>
              <span className="flex">
                <Pill tone={list.is_public ? 'success' : 'neutral'}>{list.is_public ? t`Public` : t`Private`}</Pill>
              </span>
            </Tooltip>
          ) : undefined
        }
        actions={
          <>
            {canWriteContacts && canWriteLists && list ? (
              <Button
                type="primary"
                icon={icon(Upload)}
                onClick={() => openDrawerWithSelectedList(workspaceId, lists, list.id, true)}
              >
                {t`Import to list`}
              </Button>
            ) : null}
            {moreMenu}
            <Tooltip title={canWriteLists ? undefined : t`You don't have write permission for lists`}>
              <Button icon={icon(Pencil)} disabled={!canWriteLists} onClick={onEditList}>
                {t`Edit list`}
              </Button>
            </Tooltip>
          </>
        }
      />
    )
  } else if (view.kind === 'segment') {
    header = (
      <PageHeader
        title={segment?.name ?? view.segmentId}
        count={count}
        leading={
          segment ? (
            <span
              data-segment-dot
              aria-hidden="true"
              className="size-2.5 shrink-0 rounded-full"
              // The segment's own colour, chosen by the user: not a theme token.
              style={{ backgroundColor: segmentDotColor(segment.color) }}
            />
          ) : undefined
        }
        status={
          segment?.status === 'building' ? (
            <SegmentBuildingIndicator workspaceId={workspaceId} segment={segment} />
          ) : undefined
        }
        actions={
          <>
            {moreMenu}
            {canWriteSegments && segment ? (
              <SegmentActions
                // Keyed by segment: Back to another segment must not leave its drawer editing the
                // first one, then save the old form under the new segment's ID.
                key={segment.id}
                workspaceId={workspaceId}
                segment={segment}
                totalContacts={workspaceTotal}
                canReadTemplates={!!permissions?.templates?.read}
              />
            ) : null}
          </>
        }
      />
    )
  } else {
    header = (
      <PageHeader
        title={t`All contacts`}
        count={count}
        actions={
          <>
            {canWriteContacts ? (
              <Button icon={icon(Upload)} onClick={() => openDrawer(workspaceId, lists, true)}>
                {t`Import`}
              </Button>
            ) : null}
            {moreMenu}
            {canWriteContacts ? (
              <ContactUpsertDrawer
                workspace={workspace}
                onSuccess={onContactCreated}
                buttonProps={{ type: 'primary', icon: icon(Plus), buttonContent: t`New contact` }}
              />
            ) : null}
          </>
        }
      />
    )
  }

  return (
    <>
      {header}
      {canWriteContacts ? (
        <BulkUpdateDrawer
          workspaceId={workspaceId}
          lists={lists}
          open={bulkUpdateOpen}
          onClose={() => setBulkUpdateOpen(false)}
        />
      ) : null}
      {view.kind === 'list' && list ? (
        <DeleteListModal
          workspaceId={workspaceId}
          list={list}
          open={deleteListOpen}
          onClose={() => setDeleteListOpen(false)}
          onDeleted={() => {
            setDeleteListOpen(false)
            onViewDeleted()
          }}
        />
      ) : null}
    </>
  )
}
