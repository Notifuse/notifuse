import type { ReactNode } from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import { faCalendar } from '@fortawesome/free-regular-svg-icons'

/**
 * The small pieces a segment condition is written with: the tokens of its sentence, the type badge
 * of a field and the icon square of a condition type. Whole class names, so Tailwind finds them.
 */

export type TokenTone = 'field' | 'value' | 'plain' | 'negative' | 'path' | 'index'

const TOKEN_TONE: Record<TokenTone, string> = {
  field: 'font-semibold text-ink',
  value: 'rounded-sm bg-primary-100 px-1.5 text-primary-700 font-medium',
  plain: 'rounded-sm bg-fill px-1.5 text-ink font-medium',
  negative: 'rounded-sm bg-danger-bg px-1.5 text-danger-fg font-medium',
  path: 'rounded-sm bg-info-bg px-1.5 font-mono text-2xs text-info-fg',
  index: 'rounded-sm bg-primary-50 px-1.5 font-mono text-2xs text-primary-700'
}

/** One piece of a condition sentence: a field, a value, an operator… */
export function Token({ tone, children }: { tone: TokenTone; children: ReactNode }) {
  return (
    <span className={`inline-flex h-5.5 items-center whitespace-nowrap ${TOKEN_TONE[tone]}`}>{children}</span>
  )
}

/** The linking words of a sentence (« contains », « with the status »). */
export function Word({ children }: { children: ReactNode }) {
  return <span className="text-ink-2">{children}</span>
}

/** A line of a sentence: its pieces wrap as words do. */
export function SentenceLine({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1 leading-6">{children}</div>
}

/** A JSON path as tokens: keys in blue, array indexes in violet. */
export function JsonPathTokens({ path }: { path?: string[] }) {
  if (!path?.length) return null
  return (
    <>
      {path.map((segment, index) =>
        /^\d+$/.test(segment) ? (
          <Token key={index} tone="index">
            [{segment}]
          </Token>
        ) : (
          <Token key={index} tone="path">
            {segment}
          </Token>
        )
      )}
    </>
  )
}

/** The type of a field, as the field picker shows it before the name. */
export function TypeBadge({ type }: { type: string }) {
  let content: ReactNode = 'Abc'
  if (type === 'number' || type === 'boolean') content = '123'
  if (type === 'time') content = <FontAwesomeIcon icon={faCalendar} className="text-overline" />
  if (type === 'json') content = '{ }'
  return (
    <span
      aria-hidden="true"
      className="mr-2 inline-flex h-4 min-w-6.5 items-center justify-center rounded-xs bg-fill px-1 font-mono text-overline font-semibold text-ink-3"
    >
      {content}
    </span>
  )
}

export type ConditionTone = 'contacts' | 'contact_lists' | 'contact_timeline' | 'custom_events_goals' | 'group'

const ICON_TONE: Record<ConditionTone, string> = {
  contacts: 'bg-primary-100 text-primary-700',
  contact_lists: 'bg-success-bg text-success-fg',
  contact_timeline: 'bg-info-bg text-info-fg',
  custom_events_goals: 'bg-warning-bg text-warning-fg',
  group: 'bg-fill text-ink-2'
}

/** The coloured square that tells a condition's type at a glance. */
export function ConditionIcon({ tone, icon }: { tone: ConditionTone; icon?: IconDefinition }) {
  return (
    <span
      aria-hidden="true"
      className={`flex size-7 shrink-0 items-center justify-center rounded-md ${ICON_TONE[tone] ?? ICON_TONE.group}`}
    >
      {icon && <FontAwesomeIcon icon={icon} className="text-xs" />}
    </span>
  )
}

/** A labelled line of a condition editor (« What », « How many »…). */
export function EditorLine({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="mb-2 flex flex-wrap items-start gap-2 last:mb-0">
      <span className="w-24 shrink-0 text-sm leading-7 text-ink-3">{label}</span>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">{children}</div>
    </div>
  )
}
