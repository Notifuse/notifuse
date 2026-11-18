import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCheck, faXmark, faTriangleExclamation } from '@fortawesome/free-solid-svg-icons'
import type { TreeNode } from '../../services/api/segment'
import { Panel } from '../layout'
import { treeHasRelativeDates } from './relative_dates'
import { generateSegmentId } from './segmentId'
import { treeShape, type SaveBlocker, type SegmentIdCheck } from './segmentDrawerState'
import { useSaveBlockerText } from './useSaveBlockerText'

export interface SegmentSummaryPanelProps {
  /** The tree as the form stands, the condition open in its editor included. */
  tree?: TreeNode
  timezone: string
  name: string
  isEdit: boolean
  blockers: SaveBlocker[]
  idCheck: SegmentIdCheck
  /** The last save failed: what went wrong. Not a blocker, so the save can be retried. */
  saveError?: string
}

type Tone = 'ok' | 'blocked' | 'warning'

const TONE_CLASS: Record<Tone, string> = {
  ok: 'text-success-fg',
  blocked: 'text-danger-fg',
  warning: 'text-warning-fg'
}

const TONE_ICON = { ok: faCheck, blocked: faXmark, warning: faTriangleExclamation }

function Line({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <li className="flex items-start gap-2 text-sm leading-5" data-summary-tone={tone}>
      <FontAwesomeIcon icon={TONE_ICON[tone]} className={`mt-0.5 shrink-0 text-xs ${TONE_CLASS[tone]}`} />
      <span className={tone === 'ok' ? 'text-ink-2' : 'text-ink'}>{children}</span>
    </li>
  )
}

/**
 * What the segment will be once saved, and why it cannot be saved yet: the drawer disables its
 * Save button from the same blockers.
 */
export function SegmentSummaryPanel({
  tree,
  timezone,
  name,
  isEdit,
  blockers,
  idCheck,
  saveError
}: SegmentSummaryPanelProps) {
  const { t } = useLingui()
  const { conditions, groups } = treeShape(tree)
  const id = generateSegmentId(name)
  const conditionsLabel = plural(conditions, { one: '# condition', other: '# conditions' })
  const groupsLabel = plural(groups, { one: '# group', other: '# groups' })

  const blockerText = useSaveBlockerText(id)

  return (
    <Panel title={t`Summary`}>
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {saveError ? <Line tone="blocked">{saveError}</Line> : null}
        {blockers.map((blocker) => (
          <Line key={blocker} tone="blocked">
            {blockerText[blocker]}
          </Line>
        ))}
        {!isEdit && idCheck === 'failed' && !blockers.includes('id_empty') ? (
          <Line tone="warning">{t`The ID ${id} could not be checked; saving will tell if it is taken.`}</Line>
        ) : null}
        {conditions > 0 ? (
          <Line tone="ok">
            {groups > 0 ? t`${conditionsLabel}, in ${groupsLabel}` : conditionsLabel}
          </Line>
        ) : null}
        {treeHasRelativeDates(tree) ? (
          <Line tone="ok">{t`Recomputed every day at 5:00 (${timezone}): it counts rolling days.`}</Line>
        ) : null}
        {isEdit ? <Line tone="ok">{t`The segment is recomputed when you save.`}</Line> : null}
      </ul>
    </Panel>
  )
}
