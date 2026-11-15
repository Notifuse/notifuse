import React from 'react'
import { Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useQuery } from '@tanstack/react-query'
import { broadcastApi } from '../../services/api/broadcast'
import { useWorkspacePermissions } from '../../contexts/AuthContext'
import type { Sender } from '../../services/api/workspace'

const { Text } = Typography

interface SenderLineProps {
  // The senders of the integration the email goes out through
  senders: Sender[]
  // The sender the email is sent with, when it names one of those
  senderId?: string
  // The From name the message was sent with, in place of the sender's
  fromName?: string
}

// The From of a previewed email: its sender, else the integration's default
export const SenderLine: React.FC<SenderLineProps> = ({ senders, senderId, fromName }) => {
  const { t } = useLingui()
  const defaultSender = senders.find((s) => s.is_default)
  const sender = senders.find((s) => s.id === senderId)
  if (fromName) {
    const original = sender?.name || defaultSender?.name
    return (
      <>
        <Text>
          {fromName} &lt;{sender?.email || defaultSender?.email || t`no email`}&gt;
        </Text>
        {original && <Text type="secondary" className="text-xs pl-2">{t`(original: ${original})`}</Text>}
      </>
    )
  }
  const shown = sender ?? defaultSender
  if (!shown) return <Text>{t`No default sender configured`}</Text>
  return (
    <Text>
      {shown.name}
      <Text> &lt;{shown.email}&gt;</Text>
    </Text>
  )
}

interface BroadcastSenderLineProps extends Omit<SenderLineProps, 'senderId'> {
  workspaceId: string
  broadcastId: string
  templateId: string
  templateSenderId?: string
}

// The From of a broadcast email from the message history: the sender of the broadcast's variation for this template
// when it has one (read with Broadcasts read), else the template's. Mounted with the drawer's content, so the
// broadcast is read only once the drawer opens.
export const BroadcastSenderLine: React.FC<BroadcastSenderLineProps> = ({
  workspaceId,
  broadcastId,
  templateId,
  templateSenderId,
  senders,
  fromName
}) => {
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { data: broadcast } = useQuery({
    queryKey: ['broadcast-variation-senders', workspaceId, broadcastId],
    queryFn: async () => (await broadcastApi.get({ workspace_id: workspaceId, id: broadcastId })).broadcast,
    enabled: !!permissions?.broadcasts?.read,
    staleTime: 60 * 1000
  })
  const variationSenderId = broadcast?.test_settings?.variations?.find(
    (variation) => variation.template_id === templateId
  )?.sender_id
  const senderId =
    variationSenderId && senders.some((s) => s.id === variationSenderId) ? variationSenderId : templateSenderId
  return <SenderLine senders={senders} senderId={senderId} fromName={fromName} />
}
