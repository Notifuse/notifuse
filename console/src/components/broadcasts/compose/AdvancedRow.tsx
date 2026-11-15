import { useId, type ReactNode } from 'react'
import { Switch } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../../layout'
import type { DataFeedSettings } from '../../../services/api/broadcast'
import { GlobalFeedSettings } from '../GlobalFeedSettings'
import { RecipientFeedSettings } from '../RecipientFeedSettings'
import type { ComposeValues } from './composeValues'
import type { ComposeUpdate } from './types'

export interface AdvancedRowProps {
  workspaceId: string
  /** Absent until the broadcast exists: the feed tests need it. */
  broadcastId?: string
  values: ComposeValues
  onChange: ComposeUpdate
  /** What the global feed last returned, stored on the broadcast. */
  stored?: DataFeedSettings
}

/**
 * The two data feeds side by side, each turned on by the switch on its right; its settings show
 * under it, in its column, once it is on.
 */
export function AdvancedRow({ workspaceId, broadcastId, values, onChange, stored }: AdvancedRowProps) {
  const { t } = useLingui()
  const id = useId()
  // The switch is named by the feed and described by the hint under it.
  const row = (
    key: 'global' | 'recipient',
    label: string,
    hint: string,
    enabled: boolean,
    onToggle: (enabled: boolean) => void,
    settings: ReactNode
  ) => {
    const hintId = `${id}-${key}-hint`
    return (
      <div className="min-w-0">
        <div className="flex items-center justify-between gap-3 px-3.5 py-2.5">
          <span className="flex min-w-0 flex-col">
            <span className="text-sm font-medium text-ink">{label}</span>
            <span id={hintId} className="text-xs text-ink-3">
              {hint}
            </span>
          </span>
          <Switch aria-label={label} aria-describedby={hintId} checked={enabled} onChange={onToggle} />
        </div>
        {enabled ? <div className="px-3.5 pb-3">{settings}</div> : null}
      </div>
    )
  }
  return (
    <Panel title={t`Data feeds`} flush>
      <div className="grid grid-cols-2 divide-x divide-line-soft">
        {row(
          'global',
          t({ message: 'Global', context: 'data feed' }),
          t`Data shared by every recipient, in the global_feed variable`,
          values.globalFeed.enabled,
          (enabled) => onChange((v) => ({ ...v, globalFeed: { ...v.globalFeed, enabled } })),
          <GlobalFeedSettings
            workspaceId={workspaceId}
            broadcastId={broadcastId}
            value={values.globalFeed}
            onChange={(globalFeed) => onChange((v) => ({ ...v, globalFeed }))}
            globalFeedData={stored?.global_feed_data}
            globalFeedFetchedAt={stored?.global_feed_fetched_at}
          />
        )}
        {row(
          'recipient',
          t({ message: 'Per-recipient', context: 'data feed' }),
          t`One HTTPS call per contact before sending, in the recipient_feed variable`,
          values.recipientFeed.enabled,
          (enabled) => onChange((v) => ({ ...v, recipientFeed: { ...v.recipientFeed, enabled } })),
          <RecipientFeedSettings
            workspaceId={workspaceId}
            broadcastId={broadcastId}
            value={values.recipientFeed}
            onChange={(recipientFeed) => onChange((v) => ({ ...v, recipientFeed }))}
          />
        )}
      </div>
    </Panel>
  )
}
