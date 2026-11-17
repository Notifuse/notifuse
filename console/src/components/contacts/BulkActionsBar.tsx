import { useState, type ReactNode } from 'react'
import { Button, Select, Typography, Modal, Input, Alert, Tooltip } from 'antd'
import { ListMinus, ListPlus, MailX, Trash2, X } from 'lucide-react'
import { useLingui, Plural } from '@lingui/react/macro'
import type { List } from '../../services/api/list'

const { Text } = Typography

export interface BulkActionsBarProps {
  selectedCount: number
  lists: List[]
  canWriteContacts: boolean
  canWriteLists: boolean
  /** When true, all action buttons are disabled (e.g. a bulk op is already running). */
  disabled?: boolean
  onAddToList: (listId: string) => void
  onRemoveFromList: (listId: string) => void
  onUnsubscribeFromList: (listId: string) => void
  onDelete: () => void
  onClear: () => void
}

type ListPickerAction = 'add' | 'remove' | 'unsubscribe'

export function BulkActionsBar({
  selectedCount,
  lists,
  canWriteContacts,
  canWriteLists,
  disabled = false,
  onAddToList,
  onRemoveFromList,
  onUnsubscribeFromList,
  onDelete,
  onClear
}: BulkActionsBarProps) {
  const { t } = useLingui()
  const [listPicker, setListPicker] = useState<ListPickerAction | null>(null)
  const [pickedListId, setPickedListId] = useState<string>('')
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteInput, setDeleteInput] = useState('')

  if (selectedCount === 0) return null

  const contactsWriteTooltip = !canWriteContacts ? t`You don't have write permission for contacts` : undefined
  const listsWriteTooltip = !canWriteLists ? t`You don't have write permission for lists` : undefined

  const openListPicker = (action: ListPickerAction) => {
    setPickedListId('')
    setListPicker(action)
  }

  const closeListPicker = () => {
    setListPicker(null)
    setPickedListId('')
  }

  const confirmListPicker = () => {
    if (!pickedListId || !listPicker) return
    if (listPicker === 'add') onAddToList(pickedListId)
    else if (listPicker === 'remove') onRemoveFromList(pickedListId)
    else if (listPicker === 'unsubscribe') onUnsubscribeFromList(pickedListId)
    closeListPicker()
  }

  const openDelete = () => {
    setDeleteInput('')
    setDeleteOpen(true)
  }

  const closeDelete = () => {
    setDeleteOpen(false)
    setDeleteInput('')
  }

  const confirmDelete = () => {
    if (deleteInput.trim() !== String(selectedCount)) return
    onDelete()
    closeDelete()
  }

  const listPickerTitle =
    listPicker === 'add'
      ? t`Add to list`
      : listPicker === 'remove'
        ? t`Remove from list`
        : listPicker === 'unsubscribe'
          ? t`Unsubscribe from list`
          : ''

  const listPickerDescription =
    listPicker === 'add'
      ? t`These contacts will be subscribed to the selected list (status: active).`
      : listPicker === 'remove'
        ? t`These contacts will be removed from the selected list. This is a true removal, not an unsubscribe.`
        : listPicker === 'unsubscribe'
          ? t`These contacts will be marked as unsubscribed from the selected list. They stay in the list with status "unsubscribed".`
          : ''

  // A bulk add goes in as active on any list: on a double opt-in list no confirmation email
  // leaves, the member's add standing as the contacts' consent. The dialog says so.
  const pickedList = lists.find((l) => l.id === pickedListId)
  const pickedListName = pickedList?.name ?? ''
  const skipsOptIn = listPicker === 'add' && !!pickedList?.is_double_optin

  // Adding, removing and unsubscribing change list memberships: the server asks for lists write
  // as well as contacts write. Deleting only needs contacts write.
  const listDisabled = disabled || !canWriteContacts || !canWriteLists
  const writeDisabled = disabled || !canWriteContacts
  const runningTooltip = disabled ? t`A bulk action is already running` : undefined
  const listTooltip = runningTooltip || contactsWriteTooltip || listsWriteTooltip

  const action = (
    label: string,
    icon: ReactNode,
    onClick: () => void,
    off: boolean,
    tooltip?: string,
    danger = false
  ) => (
    <Tooltip title={tooltip}>
      <span>
        <Button type="text" size="small" danger={danger} icon={icon} disabled={off} onClick={onClick}>
          {label}
        </Button>
      </span>
    </Tooltip>
  )

  return (
    <>
      <div
        data-bulk-bar
        className="flex flex-wrap items-center gap-1 border-b border-line-soft bg-primary-50 px-3 py-2"
      >
        <span className="mr-2">
          <Text strong>
            <Plural value={selectedCount} one="# selected" other="# selected" />
          </Text>
        </span>
        {action(
          t`Add to list`,
          <ListPlus size={14} aria-hidden="true" />,
          () => openListPicker('add'),
          listDisabled,
          listTooltip
        )}
        {action(
          t`Remove from list`,
          <ListMinus size={14} aria-hidden="true" />,
          () => openListPicker('remove'),
          listDisabled,
          listTooltip
        )}
        {action(
          t`Unsubscribe from list`,
          <MailX size={14} aria-hidden="true" />,
          () => openListPicker('unsubscribe'),
          listDisabled,
          listTooltip
        )}
        {action(
          t`Delete`,
          <Trash2 size={14} aria-hidden="true" />,
          openDelete,
          writeDisabled,
          runningTooltip || contactsWriteTooltip,
          true
        )}
        <span className="flex-1" />
        <Tooltip title={t`Clear selection`}>
          <Button
            type="text"
            size="small"
            aria-label={t`Clear selection`}
            icon={<X size={14} aria-hidden="true" />}
            onClick={onClear}
          />
        </Tooltip>
      </div>

      <Modal
        title={listPickerTitle}
        open={listPicker !== null}
        onCancel={closeListPicker}
        onOk={confirmListPicker}
        okText={t`Confirm`}
        okButtonProps={{ disabled: !pickedListId }}
        destroyOnHidden
      >
        <div className="flex flex-col gap-4 py-4">
          <Alert type="info" title={listPickerDescription} />
          <div className="flex flex-col gap-2">
            <Text>{t`Target list:`}</Text>
            <Select
              className="w-full"
              placeholder={t`Select a list`}
              value={pickedListId || undefined}
              onChange={setPickedListId}
              options={lists.map((l) => ({ label: l.name, value: l.id }))}
              showSearch
              optionFilterProp="label"
            />
          </div>
          {skipsOptIn ? (
            <Alert
              type="warning"
              title={t`${pickedListName} uses double opt-in, but no confirmation email is sent: these contacts are added as active, your choice to add them standing as their consent.`}
            />
          ) : null}
        </div>
      </Modal>

      <Modal
        title={t`Delete contacts`}
        open={deleteOpen}
        onCancel={closeDelete}
        onOk={confirmDelete}
        okText={t`Delete`}
        okButtonProps={{
          danger: true,
          disabled: deleteInput.trim() !== String(selectedCount)
        }}
        destroyOnHidden
      >
        <div className="flex flex-col gap-4 py-4">
          <Alert
            type="warning"
            title={
              <Plural
                value={selectedCount}
                one="This will permanently delete # contact. This action cannot be undone."
                other="This will permanently delete # contacts. This action cannot be undone."
              />
            }
          />
          <div className="flex flex-col gap-2">
            <Text>{t`Type ${selectedCount} to confirm:`}</Text>
            <Input
              value={deleteInput}
              onChange={(e) => setDeleteInput(e.target.value)}
              placeholder={String(selectedCount)}
              onPressEnter={confirmDelete}
              autoFocus
            />
          </div>
        </div>
      </Modal>
    </>
  )
}
