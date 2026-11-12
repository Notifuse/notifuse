import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'

/** A duration in the console language: « 48 s », « 2 min 48 s », « 1 h 05 min ». */
export function formatDuration(ms: number, i18n: I18n): string {
  const total = Math.max(0, Math.round(ms / 1000))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  if (hours > 0) {
    const mm = String(minutes).padStart(2, '0')
    return i18n._(msg`${hours} h ${mm} min`)
  }
  if (minutes > 0) return i18n._(msg`${minutes} min ${seconds} s`)
  return i18n._(msg`${seconds} s`)
}
