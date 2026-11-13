import { useState } from 'react'
import { App, Button, Input, Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Mono } from '../layout'
import type { Broadcast } from '../../services/api/broadcast'

export interface DeleteBroadcastModalProps {
  broadcast: Broadcast | null
  onClose: () => void
  /** Deletes the broadcast; rejects on failure. */
  onDelete: (broadcast: Broadcast) => Promise<void>
}

/** Deleting a draft is confirmed by typing its identifier. */
export function DeleteBroadcastModal({ broadcast, onClose, onDelete }: DeleteBroadcastModalProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [input, setInput] = useState('')
  const [deleting, setDeleting] = useState(false)

  const close = () => {
    setInput('')
    onClose()
  }

  const handleDelete = async () => {
    if (!broadcast) return
    setDeleting(true)
    try {
      await onDelete(broadcast)
      close()
    } catch {
      message.error(t`Failed to delete broadcast`)
    } finally {
      setDeleting(false)
    }
  }

  const mismatch = Boolean(broadcast && input && input !== broadcast.id)

  return (
    <Modal
      title={t`Delete Broadcast`}
      open={broadcast !== null}
      onCancel={close}
      footer={[
        <Button key="cancel" onClick={close}>
          {t`Cancel`}
        </Button>,
        <Button
          key="delete"
          type="primary"
          danger
          loading={deleting}
          disabled={!broadcast || input !== broadcast.id}
          onClick={handleDelete}
        >
          {t`Delete`}
        </Button>
      ]}
    >
      {broadcast ? (
        <div className="flex flex-col gap-2">
          <p data-unreset className="m-0">{t`Are you sure you want to delete the broadcast "${broadcast.name}"?`}</p>
          <p data-unreset className="m-0">
            {t`This action cannot be undone. To confirm, please enter the broadcast ID:`}{' '}
            <Mono copy>{broadcast.id}</Mono>
          </p>
          <Input
            placeholder={t`Enter broadcast ID to confirm`}
            aria-label={t`Broadcast ID`}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            status={mismatch ? 'error' : ''}
          />
          {mismatch ? <p data-unreset className="m-0 text-sm text-danger-fg">{t`ID doesn't match`}</p> : null}
        </div>
      ) : null}
    </Modal>
  )
}
