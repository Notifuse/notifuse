import { useLingui } from '@lingui/react/macro'
import type { ABTestNodeConfig, NodeType } from '../../../services/api/automation'
import { useNumberFormat } from '../../layout'
import { edgeBaseLabel, type EdgeLabelTone } from '../edges/edgeLabel'
import { outputHandles } from '../stats/stepSplits'
import { outputLeft, outputSpacing } from './outputLayout'

/** The text colour of an output's name, per tone, written whole for Tailwind. */
const NAME_TONES: Record<EdgeLabelTone, string> = {
  success: 'text-success-fg',
  warning: 'text-warning-fg',
  neutral: 'text-ink-3',
  info: 'text-info-fg'
}

interface OutputNamesProps {
  nodeType: NodeType
  config: Record<string, unknown> | undefined
}

/**
 * The last row of a multi-output step's card: each output's name, right above its point. The share
 * of each output, with stats on, is on its link.
 */
export function OutputNames({ nodeType, config }: OutputNamesProps) {
  const { i18n } = useLingui()
  const { percent } = useNumberFormat()
  const handles = outputHandles(nodeType, config)
  // An A/B variant's name and weight do not fit one line between close points: two lines.
  const variants = nodeType === 'ab_test' ? ((config as Partial<ABTestNodeConfig> | undefined)?.variants ?? []) : null
  if (handles.length === 0) return null
  // A name is never wider than the space to the next output point.
  const room = outputSpacing(nodeType, handles.length)

  return (
    // Across the whole card (its padding undone), so the % positions match the output points'.
    <div className={`relative -mx-3 mt-2 ${variants ? 'h-8' : 'h-4'}`} data-output-names>
      {handles.map((handle, index) => {
        const base = edgeBaseLabel(i18n, nodeType, handle, config)
        if (!base) return null
        const variant = variants?.find((v) => v.id === handle)
        return (
          <span
            key={handle}
            data-output-name={handle}
            title={base.text}
            className={`absolute top-0 flex -translate-x-1/2 flex-col items-center text-xs font-medium whitespace-nowrap ${NAME_TONES[base.tone]}`}
            style={{ left: `${outputLeft(nodeType, index, handles.length)}%`, maxWidth: `${room - 2}%` }}
          >
            <span className="max-w-full truncate">{variant ? variant.name : base.text}</span>
            {variant ? (
              <span data-output-weight className="font-normal tabular-nums">
                {percent(variant.weight / 100, 0)}
              </span>
            ) : null}
          </span>
        )
      })}
    </div>
  )
}
