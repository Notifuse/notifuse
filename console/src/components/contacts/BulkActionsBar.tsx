import { useState } from 'react'
import { Button, Select, Space, Typography, Modal, Input, Alert, Tooltip } from 'antd'
import {
  DeleteOutlined,
  UserAddOutlined,
  UserDeleteOutlined,
  StopOutlined,
  CloseOutlined
} from '@ant-design/icons'
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

  const contactsWriteTooltip = !canWriteContacts
    ? t`You don't have write permission for contacts`
    : undefined
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

  const addDisabled = disabled || !canWriteContacts || !canWriteLists
  const writeDisabled = disabled || !canWriteContacts
  const runningTooltip = disabled ? t`A bulk action is already running` : undefined

  return (
    <>
      <div className="mb-4 rounded-md bg-gradient-to-r from-pink-500 via-fuchsia-500 to-violet-500 p-[1px]">
        <div className="flex items-center gap-3 rounded-md bg-white px-3 py-2">
        <Text strong>
          <Plural
            value={selectedCount}
            one="# contact selected"
            other="# contacts selected"
          />
        </Text>
        <Space wrap>
          <Tooltip title={runningTooltip || listsWriteTooltip || contactsWriteTooltip}>
            <span>
              <Button
                type="primary"
                ghost
                size="small"
                icon={<UserAddOutlined />}
                disabled={addDisabled}
                onClick={() => openListPicker('add')}
              >
                {t`Add to list`}
              </Button>
            </span>
          </Tooltip>
          <Tooltip title={runningTooltip || contactsWriteTooltip}>
            <span>
              <Button
                type="primary"
                ghost
                size="small"
                icon={<UserDeleteOutlined />}
                disabled={writeDisabled}
                onClick={() => openListPicker('remove')}
              >
                {t`Remove from list`}
              </Button>
            </span>
          </Tooltip>
          <Tooltip title={runningTooltip || contactsWriteTooltip}>
            <span>
              <Button
                type="primary"
                ghost
                size="small"
                icon={<StopOutlined />}
                disabled={writeDisabled}
                onClick={() => openListPicker('unsubscribe')}
              >
                {t`Unsubscribe from list`}
              </Button>
            </span>
          </Tooltip>
          <Tooltip title={runningTooltip || contactsWriteTooltip}>
            <span>
              <Button
                type="primary"
                ghost
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={writeDisabled}
                onClick={openDelete}
              >
                {t`Delete`}
              </Button>
            </span>
          </Tooltip>
        </Space>
          <Button type="text" icon={<CloseOutlined />} onClick={onClear} className="ml-auto">
            {t`Clear`}
          </Button>
        </div>
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
          <Alert type="info" message={listPickerDescription} />
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
            message={
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
