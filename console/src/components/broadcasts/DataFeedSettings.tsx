import { Divider } from 'antd'
import { GlobalFeedSettings } from './GlobalFeedSettings'
import { RecipientFeedSettings } from './RecipientFeedSettings'
import type {
  GlobalFeedSettings as GlobalFeedSettingsType,
  RecipientFeedSettings as RecipientFeedSettingsType
} from '../../services/api/broadcast'

interface DataFeedSettingsProps {
  workspaceId: string
  broadcastId?: string
  globalFeed?: GlobalFeedSettingsType
  onGlobalFeedChange?: (settings: GlobalFeedSettingsType) => void
  globalFeedData?: Record<string, unknown>
  globalFeedFetchedAt?: string
  recipientFeed?: RecipientFeedSettingsType
  onRecipientFeedChange?: (settings: RecipientFeedSettingsType) => void
  disabled?: boolean
}

export function DataFeedSettings({
  workspaceId,
  broadcastId,
  globalFeed,
  onGlobalFeedChange,
  globalFeedData,
  globalFeedFetchedAt,
  recipientFeed,
  onRecipientFeedChange,
  disabled = false
}: DataFeedSettingsProps) {
  return (
    <>
      <GlobalFeedSettings
        workspaceId={workspaceId}
        broadcastId={broadcastId}
        value={globalFeed}
        onChange={onGlobalFeedChange}
        globalFeedData={globalFeedData}
        globalFeedFetchedAt={globalFeedFetchedAt}
        disabled={disabled}
      />
      <Divider className="!mt-8" />
      <RecipientFeedSettings
        workspaceId={workspaceId}
        broadcastId={broadcastId}
        value={recipientFeed}
        onChange={onRecipientFeedChange}
        disabled={disabled}
      />
    </>
  )
}
