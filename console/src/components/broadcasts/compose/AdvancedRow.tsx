import { useId, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Panel } from '../../layout'
import type { DataFeedSettings } from '../../../services/api/broadcast'
import { GlobalFeedSettings } from '../GlobalFeedSettings'
import { RecipientFeedSettings } from '../RecipientFeedSettings'
import type { ComposeValues } from './composeValues'
import type { ComposeUpdate } from './types'
import { feedState } from './feedState'

export interface AdvancedRowProps {
  workspaceId: string
  /** Absent until the broadcast exists: the feed tests need it. */
  broadcastId?: string
  values: ComposeValues
  onChange: ComposeUpdate
  /** What the global feed last returned, stored on the broadcast. */
  stored?: DataFeedSettings
}

/** The two data feeds, folded until opened. */
export function AdvancedRow({ workspaceId, broadcastId, values, onChange, stored }: AdvancedRowProps) {
  const { t } = useLingui()
  const [open, setOpen] = useState<'global' | 'recipient' | null>(null)
  const id = useId()
  // The hint describes the button rather than naming it: its name stays the feed's.
  const row = (key: 'global' | 'recipient', label: string, hint: string, state: string) => {
    const expanded = open === key
    const hintId = `${id}-${key}-hint`
    return (
      <button
        type="button"
        aria-expanded={expanded}
        aria-describedby={hintId}
        onClick={() => setOpen(expanded ? null : key)}
        className="flex w-full items-center justify-between gap-3 px-3.5 py-2.5 text-left hover:bg-subtle"
      >
        <span className="flex min-w-0 flex-1 items-start gap-2">
          <span className="mt-0.5 flex text-ink">
            {expanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
          </span>
          <span className="flex min-w-0 flex-col">
            <span className="text-sm font-medium text-ink">{label}</span>
            <span id={hintId} aria-hidden="true" className="text-xs text-ink-3">
              {hint}
            </span>
          </span>
        </span>
        <span className="max-w-1/2 min-w-0 truncate text-sm text-ink-3">{state}</span>
      </button>
    )
  }
  const off = t`Off`
  return (
    <Panel title={t`Advanced`} flush>
      <div className="divide-y divide-line-soft">
        <div>
          {row(
            'global',
            t`Global data feed`,
            t`Data shared by every recipient, in the global_feed variable`,
            feedState(values.globalFeed, off)
          )}
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
          {row(
            'recipient',
            t`Per-recipient data feed`,
            t`One HTTPS call per contact before sending, in the recipient_feed variable`,
            feedState(values.recipientFeed, off)
          )}
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
