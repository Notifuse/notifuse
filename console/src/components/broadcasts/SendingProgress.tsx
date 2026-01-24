import React from 'react'
import { Progress, Typography, Space } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPaperPlane } from '@fortawesome/free-regular-svg-icons'

const { Text } = Typography

interface SendingProgressProps {
  enqueuedCount: number
  sentCount: number
  failedCount: number
  startedAt?: string
}

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainingMinutes = minutes % 60
  return `${hours}h ${remainingMinutes}m`
}

function calculateEta(startedAt: string | undefined, remaining: number, processed: number): string | null {
  if (!startedAt || remaining === 0 || processed === 0) return null
  const elapsedMs = Date.now() - new Date(startedAt).getTime()
  const ratePerMs = processed / elapsedMs
  if (ratePerMs <= 0) return null
  const remainingMs = remaining / ratePerMs
  return formatDuration(remainingMs)
}

export function SendingProgress({
  enqueuedCount,
  sentCount,
  failedCount,
  startedAt
}: SendingProgressProps) {
  const { t } = useLingui()
  const processed = sentCount + failedCount
  const remaining = Math.max(0, enqueuedCount - processed)
  const percent = enqueuedCount > 0 ? Math.round((processed / enqueuedCount) * 100) : 0

  // Calculate ETA - updates on each render since dependencies change frequently
  const eta = calculateEta(startedAt, remaining, processed)

  if (remaining === 0) return null

  return (
    <div className="mb-4">
      <div className="flex justify-between items-center mb-1">
        <Space>
          <FontAwesomeIcon icon={faPaperPlane} className="text-amber-500" />
          <span className="font-medium text-gray-700">{t`Sending emails...`}</span>
        </Space>
        <Text type="secondary">
          {t`${processed.toLocaleString()} of ${enqueuedCount.toLocaleString()}`}
          {eta && <span className="ml-2">(~{eta})</span>}
        </Text>
      </div>
      <Progress
        percent={percent}
        status="active"
        strokeColor="#f59e0b"
        showInfo={false}
      />
    </div>
  )
}
