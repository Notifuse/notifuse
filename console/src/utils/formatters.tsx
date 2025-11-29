import React from 'react'
import { Tag, Tooltip } from 'antd'
import numbro from 'numbro'
import dayjs from '../lib/dayjs'

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

  // Format number values with numbro
  if (typeof value === 'number') {
    // For currency-like fields (decimals)
    if (String(value).includes('.') && value > 0) {
      return numbro(value).format({
        thousandSeparated: true,
        mantissa: 2,
        trimMantissa: true
      })
    }
    // For integer values
    return numbro(value).format({
      thousandSeparated: true,
      mantissa: 0
    })
  }

  // Handle date strings (ISO 8601 format)
  if (
    typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/.test(value)
  ) {
    const date = dayjs(value)
    if (date.isValid()) {
      const timezoneSuffix = timezone ? ` in ${timezone}` : ''
      return (
        <Tooltip title={`${date.format('LLLL')}${timezoneSuffix}`}>
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

/**
 * Convert semantic event names to human-readable format
 * Examples:
 *   "orders/fulfilled" → "Orders Fulfilled"
 *   "payment.succeeded" → "Payment Succeeded"
 *   "user_login" → "User Login"
 */
export const formatEventName = (eventName: string): string => {
  if (!eventName) return ''

  // Split on /, ., or _
  const parts = eventName.split(/[/._]/)

  // Capitalize each word
  const formatted = parts
    .map((part) => {
      if (!part) return ''
      return part.charAt(0).toUpperCase() + part.slice(1).toLowerCase()
    })
    .join(' ')
    .trim()

  return formatted
}

/**
 * Returns colored Tag component for custom event sources
 * - API: blue
 * - Integration: green
 * - Import: orange
 */
export const getSourceBadge = (source: string): React.ReactElement => {
  const sourceMap: Record<string, { color: string; label: string }> = {
    api: { color: 'blue', label: 'API' },
    integration: { color: 'green', label: 'Integration' },
    import: { color: 'orange', label: 'Import' }
  }

  const sourceInfo = sourceMap[source.toLowerCase()] || {
    color: 'default',
    label: source
  }

  return <Tag color={sourceInfo.color}>{sourceInfo.label}</Tag>
}
