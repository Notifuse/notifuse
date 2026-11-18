import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ExternalLink } from 'lucide-react'
import { isOpenableUrl } from '../../../../utils/safeHtml'
import { hasLiquidOutput, isLiquidExpression } from '../../../../utils/liquid'

// A Liquid example goes into the message as a variable: braces in a message are placeholders
const LIQUID_VARIABLE = '{{ variable }}'

export interface DraftTextFieldProps {
  value?: string
  /** Writes the committed text; an empty field writes undefined. */
  onChange: (value: string | undefined) => void
  placeholder?: string
  'aria-label': string
  /** 'url': a URL or a Liquid output; or a function returning an error message. */
  validate?: 'url' | ((value: string) => string | null)
  /** Several lines (the preview text): Enter adds a line, the text is written on blur. */
  multiline?: boolean
  rows?: number
  maxLength?: number
  showCount?: boolean
  /** Monospace (URLs, CSS classes). */
  mono?: boolean
  /** Trim the text before writing it (default). */
  trim?: boolean
  /** The address the « open » button follows for a saved value, null for none (default: the value
   *  itself when `validate` is 'url' and the value is an openable web address). */
  linkOf?: (value: string) => string | null
}

// DraftTextField: the text stays in the field while it is typed, and is written to the email once,
// on blur or Enter (Escape reverts), so a word is one undo step and not one per letter. A text the
// check refuses stays in the field, in red, and is not written. When the field goes away without
// a blur (the selection changed from outside), a changed text is still written, for the block it
// was typed for (the editor merges it into that block as it is now).
export function DraftTextField({
  value,
  onChange,
  placeholder,
  'aria-label': ariaLabel,
  validate,
  multiline = false,
  rows = 3,
  maxLength,
  showCount = false,
  mono = false,
  trim = true,
  linkOf
}: DraftTextFieldProps) {
  const { t } = useLingui()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')

  const clean = (text: string): string | undefined => {
    const result = trim ? text.trim() : text
    return result === '' ? undefined : result
  }
  const errorOf = (text: string): string | null => {
    const cleaned = clean(text)
    if (!validate || cleaned === undefined) return null
    if (typeof validate === 'function') return validate(cleaned)
    if (hasLiquidOutput(cleaned)) return null
    try {
      new URL(cleaned)
      return null
    } catch {
      return t`Invalid URL format. Use a valid URL or a Liquid expression like ${LIQUID_VARIABLE}.`
    }
  }

  // The last value written (or received when the field took the focus): a draft equal to it
  // writes nothing, so going through the fields with Tab adds no undo step.
  const committed = useRef<string | undefined>(value)
  const live = useRef({ editing, draft, onChange, clean, errorOf })
  useLayoutEffect(() => {
    live.current = { editing, draft, onChange, clean, errorOf }
  })

  const commit = (text: string) => {
    if (errorOf(text)) return
    const next = clean(text)
    if (next === clean(committed.current ?? '')) return
    committed.current = next
    onChange(next)
  }

  useEffect(
    () => () => {
      const { editing: wasEditing, draft: text, onChange: write, clean: cleanText, errorOf: check } = live.current
      if (!wasEditing || check(text)) return
      const next = cleanText(text)
      if (next === cleanText(committed.current ?? '')) return
      write(next)
    },
    []
  )

  const shown = editing ? draft : (value ?? '')
  const error = editing ? errorOf(draft) : errorOf(value ?? '')

  const handleFocus = () => {
    // A refused text left in the field stays as it was typed
    if (editing) return
    committed.current = value
    setDraft(value ?? '')
    setEditing(true)
  }
  const handleBlur = () => {
    // A refused text stays in the field, in red, until it is corrected or Escape reverts it
    if (errorOf(draft)) return
    commit(draft)
    setEditing(false)
  }
  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    if (event.key === 'Escape') {
      event.stopPropagation()
      setDraft(committed.current ?? '')
      if (errorOf(draft)) setEditing(false)
    } else if (event.key === 'Enter' && !multiline) {
      commit(draft)
    }
  }

  const urlLink = (url: string) => (validate === 'url' && !isLiquidExpression(url) && isOpenableUrl(url) ? url : null)
  const href = !editing && value && !error ? (linkOf ?? urlLink)(value) : null
  const link =
    href ? (
      <a
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={t`Open the link`}
        className="flex text-ink-4 hover:text-ink-2"
      >
        <ExternalLink size={12} aria-hidden="true" />
      </a>
    ) : null
  const liquid = !error && hasLiquidOutput(shown) ? <span className="text-2xs text-primary-700">Liquid</span> : null

  const common = {
    value: shown,
    placeholder,
    'aria-label': ariaLabel,
    'aria-invalid': error ? true : undefined,
    status: error ? ('error' as const) : undefined,
    maxLength,
    onFocus: handleFocus,
    onBlur: handleBlur,
    onKeyDown: handleKeyDown,
    onChange: (event: { target: { value: string } }) => setDraft(event.target.value),
    className: mono ? 'font-mono' : undefined
  }

  return (
    <div className="min-w-0">
      {multiline ? (
        <Input.TextArea {...common} rows={rows} showCount={showCount} />
      ) : (
        <Input {...common} size="small" suffix={link || liquid || <span />} />
      )}
      {error ? <div className="mt-1 text-2xs text-danger-fg">{error}</div> : null}
    </div>
  )
}
