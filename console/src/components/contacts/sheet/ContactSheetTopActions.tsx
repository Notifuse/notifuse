import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { Button, Tooltip } from 'antd'
import { RefreshCw, Trash2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { contactKey } from './contactKey'
import { PopconfirmWithTooltip } from '../../layout/PopconfirmWithTooltip'

export interface ContactSheetTopActionsProps {
  workspaceId: string
  email: string
  canWriteContacts: boolean
  /** Deletes the contact: the popover here is the only confirmation. */
  onDelete: (email: string) => void
}

/** Refresh and Delete of the contact sheet, as icons in the drawer bar. */
export function ContactSheetTopActions({
  workspaceId,
  email,
  canWriteContacts,
  onDelete
}: ContactSheetTopActionsProps) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: contactKey(workspaceId, email) }) > 0

  // The contact and everything the sheet reads about it.
  const refresh = () => {
    for (const queryKey of [
      contactKey(workspaceId, email),
      ['contact_timeline', workspaceId, email],
      ['message_history', workspaceId, email],
      ['contact-message-stats', workspaceId, email],
      ['contact-visits', workspaceId, email]
    ]) {
      queryClient.invalidateQueries({ queryKey })
    }
  }

  return (
    <>
      <Tooltip title={t`Refresh`}>
        <Button
          type="text"
          aria-label={t`Refresh`}
          icon={<RefreshCw size={15} aria-hidden="true" />}
          disabled={fetching}
          onClick={refresh}
        />
      </Tooltip>
      {canWriteContacts ? (
        <PopconfirmWithTooltip
          tooltip={t`Delete contact`}
          title={t`Delete contact`}
          description={t`This will permanently remove the contact and their subscriptions.`}
          okText={t`Delete`}
          cancelText={t`Cancel`}
          okButtonProps={{ danger: true }}
          placement="bottomRight"
          onConfirm={() => onDelete(email)}
        >
          <Button
            type="text"
            aria-label={t`Delete contact`}
            icon={<Trash2 size={15} aria-hidden="true" />}
          />
        </PopconfirmWithTooltip>
      ) : null}
    </>
  )
}
