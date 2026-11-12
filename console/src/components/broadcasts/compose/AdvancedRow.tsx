import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Panel } from '../../layout'
import type { DataFeedSettings, GlobalFeedSettings as GlobalFeed, RecipientFeedSettings as RecipientFeed } from '../../../services/api/broadcast'
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

/** "Off", or the host of the feed's URL. */
export function feedState(feed: GlobalFeed | RecipientFeed, off: string): string {
  if (!feed.enabled) return off
  try {
    return new URL(feed.url ?? '').host || feed.url || off
  } catch {
    return feed.url || off
  }
}

/** The two data feeds, folded until opened. */
export function AdvancedRow({ workspaceId, broadcastId, values, onChange, stored }: AdvancedRowProps) {
  const { t } = useLingui()
  const [open, setOpen] = useState<'global' | 'recipient' | null>(null)
  const row = (key: 'global' | 'recipient', label: string, state: string) => {
    const expanded = open === key
    return (
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setOpen(expanded ? null : key)}
        className="flex w-full items-center justify-between gap-3 px-3.5 py-2.5 text-left hover:bg-subtle"
      >
        <span className="flex items-center gap-2 text-sm font-medium text-ink">
          {expanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
          {label}
        </span>
        <span className="truncate text-sm text-ink-3">{state}</span>
      </button>
    )
  }
  const off = t`Off`
  return (
    <Panel title={t`Advanced`} flush>
      <div className="divide-y divide-line-soft">
        <div>
          {row('global', t`Global data feed`, feedState(values.globalFeed, off))}
          {open === 'global' ? (
            <div className="px-3.5 pb-3">
              <GlobalFeedSettings
                workspaceId={workspaceId}
                broadcastId={broadcastId}
                value={values.globalFeed}
                onChange={(globalFeed) => onChange((v) => ({ ...v, globalFeed }))}
                globalFeedData={stored?.global_feed_data}
                globalFeedFetchedAt={stored?.global_feed_fetched_at}
              />
            </div>
          ) : null}
        </div>
        <div>
          {row('recipient', t`Per-recipient data feed`, feedState(values.recipientFeed, off))}
          {open === 'recipient' ? (
            <div className="px-3.5 pb-3">
              <RecipientFeedSettings
                workspaceId={workspaceId}
                broadcastId={broadcastId}
                value={values.recipientFeed}
                onChange={(recipientFeed) => onChange((v) => ({ ...v, recipientFeed }))}
              />
            </div>
          ) : null}
        </div>
      </div>
    </Panel>
  )
}
