import { useState } from 'react'
import { Button, Segmented, Table, Tooltip } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { CircleHelp, Users } from 'lucide-react'
import { Mono, Panel, RateBar, useNumberFormat } from '../../layout'
import { getBroadcastLinkStats, type LinkClickStats } from '../../../services/api/messages_history'

/** The server returns the top links only. */
export const LINK_STATS_LIMIT = 200
const ROWS_SHOWN = 10

export interface LinkScope {
  /** "A", "B"… */
  letter: string
  templateId: string
  /** Emails of the variant whose clicks were tracked. */
  clicksTracked: number
}

export interface LinkStatsPanelProps {
  workspaceId: string
  broadcastId: string
  /** Emails of the whole broadcast whose clicks were tracked. */
  clicksTracked: number
  /** The variants of an A/B test; empty without one. */
  variants: LinkScope[]
  untracked: boolean
}

/** F5: the links clicked, by unique clicks, for the whole broadcast or one variant. */
export function LinkStatsPanel({ workspaceId, broadcastId, clicksTracked, variants, untracked }: LinkStatsPanelProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const navigate = useNavigate()
  const [scope, setScope] = useState<string>('all')
  const [showAll, setShowAll] = useState(false)
  const variant = variants.find((v) => v.templateId === scope)
  const templateId = variant?.templateId

  const query = useQuery({
    queryKey: ['broadcast-link-stats', workspaceId, broadcastId, templateId ?? ''],
    queryFn: () => getBroadcastLinkStats(workspaceId, broadcastId, templateId),
    enabled: !untracked,
    staleTime: 60_000,
    retry: false
  })

  if (untracked) {
    return (
      <Panel title={t`Clicked links`}>
        <p data-unreset className="m-0 text-sm text-ink-3">{t`Sent without click tracking`}</p>
      </Panel>
    )
  }

  const links = query.data?.link_stats ?? []
  // Nothing was clicked at all: the panel would be an empty box.
  if (scope === 'all' && query.isSuccess && links.length === 0) return null

  const denominator = variant ? variant.clicksTracked : clicksTracked
  const rateOf = (row: LinkClickStats) => (denominator > 0 ? row.unique_clicks / denominator : null)
  const maxRate = links.reduce((max, row) => Math.max(max, rateOf(row) ?? 0), 0)
  const shown = showAll ? links : links.slice(0, ROWS_SHOWN)

  const showClickers = (url: string) => {
    const params = new URLSearchParams()
    params.set('broadcast_id', broadcastId)
    if (templateId) params.set('template_id', templateId)
    params.set('clicked_url', url)
    navigate({ to: `/console/workspace/${workspaceId}/logs?${params.toString()}` as string & {} })
  }

  const caveats = (
    <div className="flex flex-col gap-2">
      <div>{t`Unique clicks counts each recipient once per link — someone who clicks several links appears in every row, so these won't add up to this variation's total number of clickers.`}</div>
      <div>{t`Only clicks recorded since per-link tracking was enabled appear here; earlier clicks still count toward the variation's Clicks total.`}</div>
      <div>{t`Counts may include automated security-scanner and Apple Mail prefetch clicks; Unique clicks is the most reliable human signal.`}</div>
    </div>
  )

  return (
    <Panel
      flush
      title={
        <span className="inline-flex items-center gap-1.5">
          {t`Clicked links`}
          <Tooltip title={caveats} styles={{ root: { maxWidth: 360 } }}>
            <CircleHelp size={13} aria-label={t`About these numbers`} className="text-ink-4" />
          </Tooltip>
        </span>
      }
      extra={
        variants.length > 0 ? (
          <Segmented<string>
            size="small"
            value={scope}
            onChange={(value) => {
              setScope(value)
              setShowAll(false)
            }}
            options={[{ value: 'all', label: t`All` }, ...variants.map((v) => ({ value: v.templateId, label: v.letter }))]}
          />
        ) : undefined
      }
      footer={
        links.length > ROWS_SHOWN || links.length >= LINK_STATS_LIMIT ? (
          <>
            <span>{links.length >= LINK_STATS_LIMIT ? t`Showing the top 200 links by unique clicks.` : ''}</span>
            {links.length > ROWS_SHOWN ? (
              <Button type="link" size="small" onClick={() => setShowAll((v) => !v)}>
                {showAll
                  ? t`Show less`
                  : t`${plural(links.length, { one: 'Show all #', other: 'Show all #' })}`}
              </Button>
            ) : null}
          </>
        ) : undefined
      }
    >
      {query.isError ? (
        <p data-unreset className="m-0 px-3.5 py-3 text-sm text-ink-3">{t`Per-link click data is not available.`}</p>
      ) : (
        <Table<LinkClickStats>
          rowKey="url"
          size="small"
          loading={query.isLoading}
          dataSource={shown}
          pagination={false}
          tableLayout="fixed"
          locale={{ emptyText: t`No link clicks recorded for this variation yet` }}
          columns={[
            {
              key: 'url',
              title: t`Link`,
              // Plain text on purpose: recorded URLs can be forged by any recipient (tracking
              // tokens are mintable), so a clickable link would be a phishing vector.
              render: (_, row) => <Mono copy>{row.url}</Mono>
            },
            {
              key: 'unique',
              title: t`Unique`,
              width: 90,
              align: 'right',
              render: (_, row) => <span className="tabular-nums">{int(row.unique_clicks)}</span>
            },
            {
              key: 'rate',
              title: (
                <Tooltip title={t`Unique clicks ÷ recipients of this variation whose clicks were tracked.`}>
                  <span>{t`Click rate`}</span>
                </Tooltip>
              ),
              width: 130,
              render: (_, row) => <RateBar rate={rateOf(row)} scale={maxRate || 1} />
            },
            {
              key: 'total',
              title: (
                <Tooltip title={t`All click events, including repeat clicks. May be inflated by bots and prefetching.`}>
                  <span>{t`Total`}</span>
                </Tooltip>
              ),
              width: 80,
              align: 'right',
              render: (_, row) => <span className="tabular-nums text-ink-3">{int(row.total_clicks)}</span>
            },
            {
              key: 'who',
              width: 48,
              align: 'right',
              render: (_, row) => (
                <Tooltip title={t`See who clicked`}>
                  <Button
                    type="text"
                    size="small"
                    aria-label={t`See who clicked`}
                    icon={<Users size={14} aria-hidden="true" />}
                    onClick={() => showClickers(row.url)}
                  />
                </Tooltip>
              )
            }
          ]}
        />
      )}
    </Panel>
  )
}
