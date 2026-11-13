import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { App, Button, Dropdown, Popconfirm } from 'antd'
import type { MenuProps } from 'antd'
import { Download, MoreHorizontal, Pencil, Plus, RefreshCw, Upload } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { deleteSegment, rebuildSegment, type Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { PageHeader } from '../layout'
import { DeleteListModal } from '../lists/DeleteListModal'
import { DrawerSegment } from '../segment/button_upsert'
import { BulkUpdateDrawer } from './BulkUpdateDrawer'
import { ContactUpsertDrawer } from './ContactUpsertDrawer'
import { useContactsCsvUpload } from './ContactsCsvUploadProvider'
import type { ContactsView } from './contactsView'
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
  onExport: () => void
  onContactCreated: () => void
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
  onExport,
  onContactCreated,
  onViewDeleted
}: ContactsViewHeaderProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const { openDrawer, openDrawerWithSelectedList } = useContactsCsvUpload()
  const [bulkUpdateOpen, setBulkUpdateOpen] = useState(false)
  const [deleteListOpen, setDeleteListOpen] = useState(false)
  const [editSegmentOpen, setEditSegmentOpen] = useState(false)

  const workspaceId = workspace.id
  const canReadContacts = !!permissions?.contacts?.read
  const canWriteContacts = !!permissions?.contacts?.write
  const canWriteLists = !!permissions?.lists?.write
  const canWriteSegments = !!permissions?.segments?.write

  const exportButton = canReadContacts ? (
    <Button icon={icon(Download)} onClick={onExport}>
      {t`Export`}
    </Button>
  ) : null

  const invalidateSegments = () => queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })

  const rebuild = useMutation({
    mutationFn: (id: string) => rebuildSegment({ workspace_id: workspaceId, segment_id: id }),
    onSuccess: (data) => {
      message.success(data.message || t`Segment rebuild started successfully`)
      invalidateSegments()
    },
    onError: (error: Error) => message.error(error?.message || t`Failed to rebuild segment`)
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteSegment({ workspace_id: workspaceId, id }),
    onSuccess: () => {
      message.success(t`Segment deleted successfully`)
      invalidateSegments()
      onViewDeleted()
    },
    onError: (error: Error) => message.error(error?.message || t`Failed to delete segment`)
  })

  if (view.kind === 'list') {
    const menu: MenuProps['items'] = canWriteLists
      ? [{ key: 'delete', label: t`Delete list`, danger: true, onClick: () => setDeleteListOpen(true) }]
      : []
    return (
      <>
        <PageHeader
          title={list?.name ?? view.listId}
          description={list?.description || undefined}
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
              {exportButton}
              {menu.length > 0 ? <MoreMenu items={menu} label={t`More actions`} /> : null}
            </>
          }
        />
        {list ? (
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

  if (view.kind === 'segment') {
    const menu: MenuProps['items'] =
      canWriteSegments && segment
        ? [
            {
              key: 'delete',
              label: t`Delete segment`,
              danger: true,
              onClick: () =>
                modal.confirm({
                  title: t`Delete segment`,
                  content: t`Are you sure you want to delete "${segment.name}"?`,
                  okText: t`Yes`,
                  cancelText: t`No`,
                  okButtonProps: { danger: true },
                  onOk: () => remove.mutateAsync(segment.id)
                })
            }
          ]
        : []
    return (
      <>
        <PageHeader
          title={segment?.name ?? view.segmentId}
          status={
            segment?.status === 'building' ? (
              <SegmentBuildingIndicator workspaceId={workspaceId} segment={segment} />
            ) : undefined
          }
          actions={
            <>
              {canWriteSegments && segment ? (
                <>
                  <Popconfirm
                    title={t`Rebuild segment`}
                    description={t`Are you sure you want to rebuild "${segment.name}"? This will recalculate segment membership.`}
                    okText={t`Yes`}
                    cancelText={t`No`}
                    onConfirm={() => rebuild.mutate(segment.id)}
                  >
                    <Button icon={icon(RefreshCw)} loading={rebuild.isPending}>
                      {t`Recompute`}
                    </Button>
                  </Popconfirm>
                  <Button icon={icon(Pencil)} onClick={() => setEditSegmentOpen(true)}>
                    {t`Edit segment`}
                  </Button>
                </>
              ) : null}
              {exportButton}
              {menu.length > 0 ? <MoreMenu items={menu} label={t`More actions`} /> : null}
            </>
          }
        />
        {editSegmentOpen && segment ? (
          <DrawerSegment
            segment={segment}
            canReadTemplates={!!permissions?.templates?.read}
            setDrawserVisible={(visible) => setEditSegmentOpen(visible)}
            onSuccess={invalidateSegments}
          />
        ) : null}
      </>
    )
  }

  const menu: MenuProps['items'] = canWriteContacts
    ? [{ key: 'bulk', label: t`Bulk update (CSV)`, onClick: () => setBulkUpdateOpen(true) }]
    : []
  return (
    <>
      <PageHeader
        title={t`Contacts`}
        actions={
          <>
            {canWriteContacts ? (
              <Button icon={icon(Upload)} onClick={() => openDrawer(workspaceId, lists, true)}>
                {t`Import`}
              </Button>
            ) : null}
            {exportButton}
            {canWriteContacts ? (
              <ContactUpsertDrawer
                workspace={workspace}
                onSuccess={onContactCreated}
                buttonProps={{ type: 'primary', icon: icon(Plus), buttonContent: t`New contact` }}
              />
            ) : null}
            {menu.length > 0 ? <MoreMenu items={menu} label={t`More actions`} /> : null}
          </>
        }
      />
      {canWriteContacts ? (
        <BulkUpdateDrawer
          workspaceId={workspaceId}
          lists={lists}
          open={bulkUpdateOpen}
          onClose={() => setBulkUpdateOpen(false)}
        />
      ) : null}
    </>
  )
}

function MoreMenu({ items, label }: { items: MenuProps['items']; label: string }) {
  return (
    <Dropdown menu={{ items }} trigger={['click']} placement="bottomRight">
      <Button aria-label={label} icon={<MoreHorizontal size={15} aria-hidden="true" />} />
    </Dropdown>
  )
}
