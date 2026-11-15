import React from 'react'
import { Tag, Tooltip } from 'antd'
import { i18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import dayjs from '../lib/dayjs'
import { onZoneClock } from '../lib/zoneClock'

/**
 * Smart formatting for all JSON types
 * Handles: booleans (tags), numbers (localized), dates (relative), objects/arrays (tooltips), strings (truncation)
 */
export const formatValue = (value: unknown, timezone?: string): React.ReactNode => {
  if (value === null || value === undefined) {
    return <span style={{ fontStyle: 'italic', color: '#999' }}>null</span>
  }

  // Handle boolean values as tags
  if (typeof value === 'boolean') {
    return <Tag color={value ? 'green' : 'red'}>{value ? 'true' : 'false'}</Tag>
  }

  // Numbers with the separators of the console's language: "12 345" in French, "12,345" in
  // English. A positive decimal keeps up to two digits ("1 234,57"); anything else is rounded
  // half up first, as before, and `|| 0` turns a rounded -0 into 0 rather than "-0" (and NaN,
  // which JSON cannot carry, into 0).
  if (typeof value === 'number') {
    const decimals = String(value).includes('.') && value > 0 ? 2 : 0
    const shown = decimals ? value : Math.round(value) || 0
    return new Intl.NumberFormat(i18n.locale || 'en', { maximumFractionDigits: decimals }).format(shown)
  }

  // Handle date strings (ISO 8601 format)
  if (
    typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/.test(value)
  ) {
    const date = dayjs(value)
    if (date.isValid()) {
      // On the clock of the timezone it names, in the console's language; without one, the
      // browser's clock and no zone name.
      const clock = timezone ? onZoneClock(date, timezone) : undefined
      const when = clock ? clock.when.format('LLLL') : date.format('LLLL')
      // Named `zone` so the message is the "{when} in {zone}" already translated for the timeline.
      const zone = clock?.zone
      const title = zone ? i18n._(msg`${when} in ${zone}`) : when
      return (
        <Tooltip title={title}>
          <span style={{ cursor: 'help', borderBottom: '1px dotted #999' }}>
            {date.fromNow()}
          </span>
        </Tooltip>
      )
    }
  }

  // Handle objects and arrays - show summary with tooltip
  if (typeof value === 'object') {
    if (Array.isArray(value)) {
      const summary = `Array (${value.length} items)`
      const content = JSON.stringify(value, null, 2)
      return (
        <Tooltip title={<pre style={{ margin: 0 }}>{content}</pre>}>
          <span style={{ cursor: 'help', borderBottom: '1px dotted #999' }}>{summary}</span>
        </Tooltip>
      )
    }

    const keys = Object.keys(value)
    const summary = `Object (${keys.length} keys)`
    const content = JSON.stringify(value, null, 2)
    return (
      <Tooltip title={<pre style={{ margin: 0 }}>{content}</pre>}>
        <span style={{ cursor: 'help', borderBottom: '1px dotted #999' }}>{summary}</span>
      </Tooltip>
    )
  }

  // Handle strings with truncation for long values
  if (typeof value === 'string') {
    if (value.length > 100) {
      return (
        <Tooltip title={value}>
          <span style={{ cursor: 'help', borderBottom: '1px dotted #999' }}>
            {value.substring(0, 100)}...
          </span>
        </Tooltip>
      )
    }
    return value
  }

  return String(value)
}
