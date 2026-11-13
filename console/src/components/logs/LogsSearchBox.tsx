import { useState } from 'react'
import { Input, Select, Space, Tooltip } from 'antd'
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
  const [seenUrlKey, setSeenUrlKey] = useState(urlKey)
  if (urlKey !== seenUrlKey) {
    setSeenUrlKey(urlKey)
    if (urlKey) setChosen(urlKey)
  }

  const writeFor = (key: K, value: string) => {
    const clean = key === emailKey ? value.toLowerCase() : value
    if (clean && key === emailKey && !isFullEmail(clean)) return
    const patch: Partial<Record<K, string | undefined>> = {}
    for (const candidate of keys) patch[candidate.key] = undefined
    if (clean) patch[key] = clean
    onWrite(patch)
  }

  const [text, setText, flush] = useDebouncedUrlValue(
    urlKey ? (values[urlKey] ?? '') : '',
    (value) => writeFor(chosen, value)
  )

  const onChange = (value: string) => {
    if (emailKey && chosen !== emailKey && value.includes('@')) setChosen(emailKey)
    setText(value)
  }

  const onChoose = (key: K) => {
    setChosen(key)
    // The text typed moves to the parameter chosen.
    if (text.trim()) writeFor(key, text.trim())
  }

  const incomplete =
    chosen === emailKey && text.trim() !== '' && !isFullEmail(text.trim().toLowerCase())

  return (
    <div className="flex flex-col gap-1">
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
          <Input
            allowClear
            aria-label={ariaLabel}
            placeholder={placeholder}
            value={text}
            onChange={(event) => onChange(event.target.value)}
            onPressEnter={flush}
            status={incomplete ? 'warning' : undefined}
            style={{ width: 300 }}
          />
        </Tooltip>
      </Space.Compact>
      {incomplete ? (
        <span className="text-xs text-ink-3">{t`Enter a full email address`}</span>
      ) : null}
    </div>
  )
}
