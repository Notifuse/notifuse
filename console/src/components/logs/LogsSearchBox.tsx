import { forwardRef, useId, useState } from 'react'
import { Input, Select, Space, Tooltip, type InputProps, type InputRef } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { isFullEmail } from '../../lib/email'
import { useDebouncedUrlValue } from '../../lib/useDebouncedUrlValue'

export interface LogsSearchKey<K extends string> {
  key: K
  label: string
  /** An address: written lower-cased, and only once the server would accept it. */
  email?: boolean
}

export interface LogsSearchBoxProps<K extends string> {
  /** The parameters the box can fill, one at a time; the email one first. */
  keys: readonly LogsSearchKey<K>[]
  /** Their values in the URL. */
  values: Partial<Record<K, string>>
  /** Sets one parameter and removes the others (undefined); written in `replace`. */
  onWrite: (patch: Partial<Record<K, string | undefined>>) => void
  ariaLabel: string
  placeholder: string
}

/**
 * The search of a logs tab: a choice of the parameter searched, then the text, written to the URL
 * after a pause or on Enter. An "@" switches to the email parameter, and an address the server
 * would refuse (400) is not written: a hint says so and the list stays as it is.
 */
export function LogsSearchBox<K extends string>({
  keys,
  values,
  onWrite,
  ariaLabel,
  placeholder
}: LogsSearchBoxProps<K>) {
  const { t } = useLingui()
  const emailKey = keys.find((key) => key.email)?.key
  const urlKey = keys.find((key) => values[key.key] !== undefined)?.key

  // The parameter searched follows the URL (a link, Back); otherwise the member's choice.
  const [chosen, setChosen] = useState<K>(urlKey ?? keys[0].key)
  // A key the member picked in the menu stays until the box is emptied, by the member or by the URL
  // (Clear filters, Back): an ID may hold an "@" (a reply's Message-ID, an external ID). Otherwise a
  // newly typed "@" switches to Email. A key the URL brings is not a pick.
  const [picked, setPicked] = useState(false)
  const [seenUrlKey, setSeenUrlKey] = useState(urlKey)
  // The box's last write removed every key: a URL without a key is then its own write, not a clear
  const [clearedItself, setClearedItself] = useState(false)

  const writeFor = (key: K, value: string) => {
    const clean = key === emailKey ? value.toLowerCase() : value
    if (clean && key === emailKey && !isFullEmail(clean)) return
    const patch: Partial<Record<K, string | undefined>> = {}
    for (const candidate of keys) patch[candidate.key] = undefined
    if (clean) patch[key] = clean
    setClearedItself(!clean)
    onWrite(patch)
  }

  const [text, setText, flush] = useDebouncedUrlValue(
    urlKey ? (values[urlKey] ?? '') : '',
    (value) => writeFor(chosen, value)
  )

  if (urlKey !== seenUrlKey) {
    setSeenUrlKey(urlKey)
    if (urlKey) setChosen(urlKey)
    // The box writes its text to the key chosen, and an empty or blank text by removing every key;
    // another key, or every key removed by anything but the box, comes from outside, even while the
    // box holds only spaces (Clear filters during the pause). The box follows the URL after this render.
    if (urlKey ? urlKey !== chosen : !clearedItself) setPicked(false)
    setClearedItself(false)
  }

  const onChange = (value: string) => {
    if (value === '') setPicked(false)
    else if (
      !picked &&
      emailKey &&
      chosen !== emailKey &&
      value.includes('@') &&
      !text.includes('@')
    )
      setChosen(emailKey)
    setText(value)
  }

  const onChoose = (key: K) => {
    setChosen(key)
    setPicked(true)
    // The text typed moves to the parameter chosen.
    if (text.trim()) writeFor(key, text.trim())
  }

  const hintId = useId()
  const incomplete =
    chosen === emailKey && text.trim() !== '' && !isFullEmail(text.trim().toLowerCase())

  return (
    <div className="flex flex-col">
      <Space.Compact>
        <Select<K>
          aria-label={t`Search by`}
          value={chosen}
          onChange={onChoose}
          options={keys.map((key) => ({ value: key.key, label: key.label }))}
          popupMatchSelectWidth={false}
          style={{ width: 130 }}
        />
        <Tooltip title={t`Exact match`}>
          <DescribedInput
            hintId={incomplete ? hintId : undefined}
            allowClear
            aria-label={ariaLabel}
            placeholder={placeholder}
            value={text}
            onChange={(event) => onChange(event.target.value)}
            onPressEnter={flush}
            status={incomplete ? 'warning' : undefined}
            // 240: at 1440px the toolbar keeps four chips and "Clear all" on one line.
            style={{ width: 240 }}
          />
        </Tooltip>
      </Space.Compact>
      {/* Always there, so a screen reader reads the hint when it appears while typing */}
      <div aria-live="polite">
        {incomplete ? (
          <span id={hintId} className="mt-1 block text-xs text-ink-3">
            {t`Enter a full email address`}
          </span>
        ) : null}
      </div>
    </div>
  )
}

/**
 * The search input, described by its tooltip and by the hint below it. The tooltip clones its child
 * with its own aria-describedby (its id while open), which would replace the hint's: both are joined
 * here, and every other prop and the ref pass through.
 */
const DescribedInput = forwardRef<InputRef, InputProps & { hintId?: string }>(
  function DescribedInput({ hintId, 'aria-describedby': tooltipId, ...props }, ref) {
    const describedBy = [tooltipId, hintId].filter(Boolean).join(' ')
    return <Input ref={ref} {...props} aria-describedby={describedBy || undefined} />
  }
)
