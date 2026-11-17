import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { WebAnalyticsBackfillStatus } from '../../../services/api/web_analytics'
import type { PillTone } from '../../layout'
import { formatPercent } from './format'

/**
 * - `never`: no backfill has run on the workspace.
 * - `active`: a run is pending, running or parked between two runtime slices (paused).
 * - `cancelled`: stopped from the console. The server records it as failed, but nothing broke.
 */
export type BackfillState = 'never' | 'active' | 'completed' | 'failed' | 'cancelled'

export interface BackfillDescription {
  state: BackfillState
  tone: PillTone
  /** The state in a word or two, translated by the caller (`i18n._(label)`). */
  label: MessageDescriptor
  /** The progress of a run under way, a whole percent; null otherwise. */
  progress: number | null
}

/** The workspace's last backfill, as the rules tab and the rule editor show it. */
export function describeBackfill(status: WebAnalyticsBackfillStatus | null | undefined): BackfillDescription {
  if (!status) return { state: 'never', tone: 'neutral', label: msg`Never run`, progress: null }
  switch (status.status) {
    case 'completed':
      return { state: 'completed', tone: 'success', label: msg`Completed`, progress: null }
    case 'failed':
      return status.cancelled === true
        ? { state: 'cancelled', tone: 'neutral', label: msg`Cancelled`, progress: null }
        : { state: 'failed', tone: 'danger', label: msg`Failed`, progress: null }
    default: {
      const percent = Math.round(status.progress ?? 0)
      // Named as the message's placeholder: the catalogues read "Running · {progress}".
      const progress = formatPercent(percent, 0)
      return { state: 'active', tone: 'info', label: msg`Running · ${progress}`, progress: percent }
    }
  }
}
