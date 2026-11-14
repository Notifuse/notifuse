import { useLingui } from '@lingui/react/macro'
import type { CanvasDropResult } from '../utils/canvasDrop'

// Why a place refuses the dragged block, in words; null when it does not refuse it
export function useDropRefusalText(): (result: CanvasDropResult | null) => string | null {
  const { t } = useLingui()
  return (result) => {
    if (!result || result.drop || !result.refusal) return null
    if (result.refusal === 'liquid') return t`Not allowed inside a conditional block`
    if (result.reason === 'column') return t`Goes inside a column`
    if (result.reason === 'section') return t`Goes inside a section`
    if (result.reason === 'social') return t`Goes inside a social block`
    return t`Not allowed here`
  }
}
