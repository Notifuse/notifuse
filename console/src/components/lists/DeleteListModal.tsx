import { useState } from 'react'
import { Button, Input, Modal, Typography, message } from 'antd'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { listsApi } from '../../services/api/list'
import type { AutomationRef, ListInUseError } from '../../services/api/list'
import { ApiError } from '../../services/api/errors'
import type { List } from '../../services/api/types'

const { Text } = Typography

// Reads the 409 lists.delete answers while live automations use the list. Null for a body
// without the automations — from a server that does not send them — whose own sentence is
// then shown instead.
const listInUseError = (data: unknown): ListInUseError | null => {
  if (!data || typeof data !== 'object') return null
  const { error, automations, automations_count } = data as Record<string, unknown>
  if (typeof automations_count !== 'number' || !Array.isArray(automations)) return null
  const named = automations.every(
    (automation: unknown) =>
      !!automation &&
      typeof automation === 'object' &&
      typeof (automation as Partial<AutomationRef>).name === 'string'
  )
  if (!named) return null
  return {
    error: typeof error === 'string' ? error : '',
    automations: automations as AutomationRef[],
    automations_count
  }
}

export interface DeleteListModalProps {
  workspaceId: string
  list: List | null
  open: boolean
  onClose: () => void
  /** After the list is deleted; the lists and their counts are already refreshed. */
  onDeleted: (list: List) => void
}

/** Deletes a list once its ID is typed, and says which live automations hold it on a refusal. */
export function DeleteListModal({ workspaceId, list, open, onClose, onDeleted }: DeleteListModalProps) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const [confirmationInput, setConfirmationInput] = useState('')
  const [isDeleting, setIsDeleting] = useState(false)

  // Why the list cannot be deleted: the live automations to pause, the first three by name and
  // any others counted, or only how many for a member who may not read automations. Each case
  // is one whole sentence in the catalogue, quotes and separators included, for a translation
  // to word all of it. It is a string because the static message API renders outside the
  // I18nProvider, where a <Plural> element would throw.
  const listInUseMessage = (listName: string, refusal: ListInUseError): string => {
    const names = refusal.automations.map(({ name }) => name)
    if (names.length === 0) {
      const count = refusal.automations_count
      return t`${plural(count, {
        one: `The list "${listName}" can't be deleted while a live automation uses it. Ask someone with access to automations to pause it.`,
        other: `The list "${listName}" can't be deleted while # live automations use it. Ask someone with access to automations to pause them.`
      })}`
    }
    if (names.length === 1) {
      const [automationName] = names
      return t`The list "${listName}" can't be deleted while the live automation "${automationName}" uses it. Pause it first, or change it to stop using the list.`
    }
    const [first, second, third] = names
    if (names.length === 2) {
      return t`The list "${listName}" can't be deleted while the live automations "${first}" and "${second}" use it. Pause them first, or change them to stop using the list.`
    }
    if (names.length === 3) {
      return t`The list "${listName}" can't be deleted while the live automations "${first}", "${second}" and "${third}" use it. Pause them first, or change them to stop using the list.`
    }
    const more = names.length - 3
    return t`${plural(more, {
      one: `The list "${listName}" can't be deleted while the live automations "${first}", "${second}", "${third}" and # more use it. Pause them first, or change them to stop using the list.`,
      other: `The list "${listName}" can't be deleted while the live automations "${first}", "${second}", "${third}" and # more use it. Pause them first, or change them to stop using the list.`
    })}`
  }

  const close = () => {
    setConfirmationInput('')
    onClose()
  }

  const handleDelete = async () => {
    if (!list) return

    setIsDeleting(true)
    try {
      await listsApi.delete({ workspace_id: workspaceId, id: list.id })

      message.success(t`List "${list.name}" deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      queryClient.invalidateQueries({ queryKey: ['lists-with-counts', workspaceId] })
      setConfirmationInput('')
      onDeleted(list)
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        // Live automations use the list. This is the only thing that tells the user what to
        // pause, so it stays on screen long enough to read, and a body the console cannot read
        // keeps the server's own sentence rather than the generic one.
        const refusal = listInUseError(error.data)
        message.error({
          content: refusal ? listInUseMessage(list.name, refusal) : error.message || t`Failed to delete list`,
          duration: 10
        })
      } else {
        message.error(t`Failed to delete list`)
      }
      console.error(error)
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <Modal
      title={t`Delete List`}
      open={open}
      onCancel={close}
      footer={[
        <Button key="cancel" onClick={close}>
          {t`Cancel`}
        </Button>,
        <Button
          key="delete"
          type="primary"
          danger
          loading={isDeleting}
          disabled={confirmationInput !== (list?.id || '')}
          onClick={handleDelete}
        >
          {t`Delete`}
        </Button>
      ]}
    >
      {list && (
        <>
          <p>{t`Are you sure you want to delete the list "${list.name}"?`}</p>
          <p>
            {t`This action cannot be undone. To confirm, please enter the list ID:`}{' '}
            <Text code>{list.id}</Text>
          </p>
          <Input
            placeholder={t`Enter list ID to confirm`}
            value={confirmationInput}
            onChange={(e) => setConfirmationInput(e.target.value)}
            status={confirmationInput && confirmationInput !== list.id ? 'error' : ''}
          />
          {confirmationInput && confirmationInput !== list.id && (
            <p className="mt-2 text-danger-fg">{t`ID doesn't match`}</p>
          )}
        </>
      )}
    </Modal>
  )
}
