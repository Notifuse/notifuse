import { useQuery } from '@tanstack/react-query'
import { App, Button, Table, Tooltip, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCopy, faCircleQuestion } from '@fortawesome/free-regular-svg-icons'
import { getBroadcastLinkStats, LinkClickStats } from '../../services/api/messages_history'

const { Text } = Typography

// Single sequential hue for the magnitude bars — the brand primary at reduced opacity
// (a solid primary reads too bright as a full bar), over a lighter tint of it as the track.
const BAR_FILL = 'rgba(119, 99, 241, 0.5)' // --color-primary @ 50%
const BAR_TRACK = 'rgba(119, 99, 241, 0.12)'

interface BroadcastLinkStatsProps {
  workspaceId: string
  broadcastId: string
  // Scopes the breakdown to a single A/B variation (the template its messages were sent
  // with). Omit for the whole broadcast.
  templateId?: string
  // Denominator for the per-link click rate — the number of emails sent for this
  // variation (the same figure the variation's Clicks % is computed from).
  recipients?: number
  enabled?: boolean
}

// Percentage formatter mirroring BroadcastStats.getRate so per-link and aggregate rates
// round the same way.
const formatRate = (numerator: number, denominator: number) => {
  if (!denominator) return '-'
  const pct = (numerator / denominator) * 100
  return pct === 0 || pct >= 10 ? `${Math.round(pct)}%` : `${pct.toFixed(1)}%`
}

export function BroadcastLinkStats({
  workspaceId,
  broadcastId,
  templateId,
  recipients = 0,
  enabled = true
}: BroadcastLinkStatsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()

  // Per-link breakdown for this variation. Mounted only when the variation row is
  // expanded (antd renders expandedRowRender lazily), so the JSONB aggregation runs on
  // expand rather than for every broadcast on the list. No polling; kept fresh for a
  // minute so re-renders don't refire it.
  const { data, isLoading, isError } = useQuery({
    queryKey: ['broadcast-link-stats', workspaceId, broadcastId, templateId ?? ''],
    queryFn: () => getBroadcastLinkStats(workspaceId, broadcastId, templateId),
    enabled,
    staleTime: 60_000,
    retry: false
  })

  const linkStats = data?.link_stats ?? []
  const maxUnique = linkStats.reduce((max, row) => Math.max(max, row.unique_clicks), 0)

  const copyUrl = (url: string) => {
    navigator.clipboard.writeText(url)
    message.success(t`URL copied to clipboard.`)
  }

  // Older servers answer this route with an empty 200 body, which makes the API client
  // throw while parsing. Degrade to a quiet notice instead of a broken expanded row.
  if (isError) {
    return (
      <div className="py-2">
        <Text type="secondary">{t`Per-link click data is not available.`}</Text>
      </div>
    )
  }

  return (
    <div className="py-2">
      <Table
        dataSource={linkStats.map((row) => ({ ...row, key: row.url }))}
        columns={[
          {
            title: (
              <span className="flex items-center gap-2">
                {t`Link`}
                <Tooltip
                  styles={{ root: { maxWidth: 360 } }}
                  title={
                    <div className="space-y-2">
                      <div>
                        {t`Unique clicks counts each recipient once per link — someone who clicks several links appears in every row, so these won't add up to this variation's total number of clickers.`}
                      </div>
                      <div>
                        {t`Only clicks recorded since per-link tracking was enabled appear here; earlier clicks still count toward the variation's Clicks total.`}
                      </div>
                      <div>
                        {t`Counts may include automated security-scanner and Apple Mail prefetch clicks; Unique clicks is the most reliable human signal.`}
                      </div>
                    </div>
                  }
                >
                  <FontAwesomeIcon
                    icon={faCircleQuestion}
                    className="text-gray-400 cursor-help"
                    style={{ opacity: 0.7 }}
                  />
                </Tooltip>
              </span>
            ),
            dataIndex: 'url',
            key: 'url',
            render: (url: string) => (
              <div className="flex items-center gap-1">
                {/* Plain text on purpose: recorded URLs can be forged by any email
                    recipient (tracking tokens are mintable), so rendering them as
                    clickable links would hand admins a phishing vector. */}
                <Tooltip title={url}>
                  <span className="truncate inline-block align-bottom" style={{ maxWidth: 480 }}>
                    {url}
                  </span>
                </Tooltip>
                <Tooltip title={t`Copy URL`}>
                  <Button
                    type="text"
                    size="small"
                    icon={<FontAwesomeIcon icon={faCopy} style={{ opacity: 0.7 }} />}
                    onClick={() => copyUrl(url)}
                  />
                </Tooltip>
              </div>
            )
          },
          {
            title: t`Unique clicks`,
            dataIndex: 'unique_clicks',
            key: 'unique_clicks',
            width: 220,
            defaultSortOrder: 'descend' as const,
            sorter: (a: LinkClickStats, b: LinkClickStats) => a.unique_clicks - b.unique_clicks,
            render: (value: number) => (
              <div className="flex items-center gap-2">
                {/* Inline magnitude bar: width relative to the top link so "which link
                    won" is scannable at a glance. Left-baselined, 4px rounded right end. */}
                <div
                  className="h-2 flex-1 rounded-sm overflow-hidden"
                  style={{ backgroundColor: BAR_TRACK, maxWidth: 140 }}
                >
                  <div
                    data-testid="unique-bar-fill"
                    className="h-2 rounded-sm"
                    style={{
                      backgroundColor: BAR_FILL,
                      width: `${maxUnique > 0 ? (value / maxUnique) * 100 : 0}%`
                    }}
                  />
                </div>
                <span className="tabular-nums text-right" style={{ minWidth: 40 }}>
                  {value.toLocaleString()}
                </span>
              </div>
            )
          },
          {
            title: (
              <Tooltip title={t`Unique clicks ÷ recipients of this variation.`}>
                <span className="cursor-help">{t`Click rate`}</span>
              </Tooltip>
            ),
            key: 'click_rate',
            width: 110,
            align: 'right' as const,
            render: (_: unknown, record: LinkClickStats) => (
              <span className="tabular-nums">{formatRate(record.unique_clicks, recipients)}</span>
            )
          },
          {
            title: (
              <Tooltip
                title={t`All click events, including repeat clicks. May be inflated by bots and prefetching.`}
              >
                <span className="cursor-help">{t`Total clicks`}</span>
              </Tooltip>
            ),
            dataIndex: 'total_clicks',
            key: 'total_clicks',
            width: 120,
            align: 'right' as const,
            sorter: (a: LinkClickStats, b: LinkClickStats) => a.total_clicks - b.total_clicks,
            render: (value: number) => (
              <span className="tabular-nums text-gray-500">{value.toLocaleString()}</span>
            )
          }
        ]}
        size="small"
        loading={isLoading}
        pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: t`No link clicks recorded for this variation yet` }}
      />
      {linkStats.length >= 200 && (
        <div className="text-xs text-gray-400 mt-1">
          {t`Showing the top 200 links by unique clicks.`}
        </div>
      )}
    </div>
  )
}
