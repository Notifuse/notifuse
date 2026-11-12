import { Button, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { Eye, FlaskConical, Trophy } from 'lucide-react'
import { Panel, Pill, RateBar, useNumberFormat } from '../../layout'
import type { Broadcast, BroadcastVariation, TestResultsResponse } from '../../../services/api/broadcast'
import type { MessageHistoryStatusSum } from '../../../services/api/messages_history'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import type { UserPermissions } from '../../../services/api/workspace'
import { canSelectWinner } from '../../../lib/abTestWinner'
import SendTestEmailButton from '../../templates/SendTestEmailButton'
import TemplatePreviewDrawer from '../../templates/TemplatePreviewDrawer'
import { BroadcastAnalyticsLinks } from '../BroadcastAnalyticsLinks'
import { getRemainingTestTime } from '../broadcastStatus'
import { describeVariationRate, toVariationMetrics, type VariationRate, type VariationRowMetrics } from '../variationMetrics'
import { workspaceTimezone } from '../access'
import { useBroadcastDates } from '../useBroadcastDates'
import { VARIANT_LETTERS } from '../compose/composeValues'
import { VariationSender } from './VariationSender'

/** The template a broadcast joins to a variant, as the template screens type it. */
export const asTemplate = (variation: BroadcastVariation) => variation.template as unknown as Template | undefined

export interface AbTestPanelProps {
  workspace: Workspace
  broadcast: Broadcast
  testResults?: TestResultsResponse
  /** F4, by template id. */
  variationStats: Record<string, MessageHistoryStatusSum>
  permissions?: UserPermissions | null
  onPickWinner: (templateId: string, templateName: string) => void
  onSendTest: (template: Template) => void
}

interface Row {
  key: string
  letter: string
  variation: BroadcastVariation
  templateName: string
  isWinner: boolean
  isRecommended: boolean
  metrics?: VariationRowMetrics
}

/** The variants of an A/B test, with their results. Shown only when the test is enabled. */
export function AbTestPanel({
  workspace,
  broadcast,
  testResults,
  variationStats,
  permissions,
  onPickWinner,
  onSendTest
}: AbTestPanelProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const settings = broadcast.test_settings
  if (!settings?.enabled) return null

  const winner = testResults?.winning_template || broadcast.winning_template
  const recommended = testResults?.recommended_winner
  const sent = broadcast.status !== 'draft' && broadcast.status !== 'scheduled'
  const canWrite = !!permissions?.broadcasts?.write
  // Without Templates read the broadcast comes without its templates: the ID says which.
  const canReadTemplates = !!permissions?.templates?.read
  const winnerSelectable = canSelectWinner(broadcast)

  const rows: Row[] = (settings.variations ?? []).map((variation, index) => ({
    key: variation.template_id || String(index),
    letter: VARIANT_LETTERS[index] ?? String(index + 1),
    variation,
    templateName: variation.template?.name || (canReadTemplates ? t`Untitled` : variation.template_id),
    isWinner: !!winner && winner === variation.template_id,
    isRecommended: !winner && !!recommended && recommended === variation.template_id,
    metrics:
      testResults?.variation_results?.[variation.template_id] ||
      toVariationMetrics(variationStats[variation.template_id]) ||
      variation.metrics
  }))

  const rateCell = (kind: 'opens' | 'clicks', metrics?: VariationRowMetrics) => {
    if (!metrics) return <RateBar rate={null} />
    const recipients = metrics.recipients || 0
    const count = kind === 'opens' ? metrics.opens || 0 : metrics.clicks || 0
    const rate =
      kind === 'opens'
        ? describeVariationRate(metrics.open_rate || 0, recipients, metrics.opens_tracked)
        : describeVariationRate(metrics.click_rate || 0, recipients, metrics.clicks_tracked)
    const tracked = rate.tracked
    const title =
      kind === 'opens'
        ? rate.trackedShare === undefined
          ? t`${count} opens out of ${recipients} recipients`
          : t`${count} opens out of ${tracked} tracked recipients (${recipients} sent)`
        : rate.trackedShare === undefined
          ? t`${count} clicks out of ${recipients} recipients`
          : t`${count} clicks out of ${tracked} tracked recipients (${recipients} sent)`
    const value = kind === 'opens' ? metrics.open_rate || 0 : metrics.click_rate || 0
    return (
      <Tooltip title={title}>
        <span className="inline-flex cursor-help flex-col">
          <RateBar rate={tracked > 0 ? value : null} />
          <TrackedShareNote rate={rate} />
        </span>
      </Tooltip>
    )
  }

  const columns: ColumnsType<Row> = [
    {
      title: t`Variant`,
      key: 'variant',
      width: 120,
      render: (_, row) => (
        <div className="flex items-center gap-2">
          <span className="flex size-5 shrink-0 items-center justify-center rounded-sm bg-fill text-xs font-semibold text-ink-2">
            {row.letter}
          </span>
          <span className="truncate text-sm text-ink-3">{row.variation.variation_name}</span>
        </div>
      )
    },
    {
      title: t`Template`,
      key: 'template',
      render: (_, row) => <span className="text-sm text-ink">{row.templateName}</span>
    },
    {
      title: t`Subject`,
      key: 'subject',
      render: (_, row) => {
        const email = row.variation.template?.email
        return (
          <div className="flex min-w-0 flex-col gap-0.5">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm text-ink">{email?.subject || t`N/A`}</span>
              {row.isWinner ? (
                <Pill tone="success" icon={<Trophy />}>
                  {t`Winner`}
                </Pill>
              ) : row.isRecommended ? (
                <Pill tone="info">{t`Recommended`}</Pill>
              ) : null}
            </div>
            {email?.subject_preview ? (
              <span className="truncate text-xs text-ink-3">{email.subject_preview}</span>
            ) : null}
          </div>
        )
      }
    },
    {
      title: t`From`,
      key: 'from',
      render: (_, row) => <VariationSender workspace={workspace} variation={row.variation} />
    },
    ...(sent
      ? ([
          {
            title: t`Sent`,
            key: 'sent',
            align: 'right',
            width: 90,
            render: (_, row) => (
              <span className="text-sm tabular-nums text-ink">{row.metrics ? int(row.metrics.recipients) : '—'}</span>
            )
          },
          { title: t`Opens`, key: 'opens', width: 110, render: (_, row) => rateCell('opens', row.metrics) },
          { title: t`Clicks`, key: 'clicks', width: 110, render: (_, row) => rateCell('clicks', row.metrics) }
        ] as ColumnsType<Row>)
      : []),
    {
      key: 'actions',
      align: 'right',
      render: (_, row) => {
        const template = asTemplate(row.variation)
        return (
          <div className="flex items-center justify-end gap-1">
            {template ? (
              <TemplatePreviewDrawer record={template} workspace={workspace}>
                <Tooltip title={t`Preview template`}>
                  <Button size="small" type="text" aria-label={t`Preview template`} icon={<Eye size={14} />} />
                </Tooltip>
              </TemplatePreviewDrawer>
            ) : null}
            {template ? (
              <SendTestEmailButton size="small" permissions={permissions} onClick={() => onSendTest(template)} />
            ) : null}
            <BroadcastAnalyticsLinks
              workspaceId={workspace.id}
              workspace={workspace}
              broadcast={broadcast}
              permissions={permissions}
              variationTemplateId={row.variation.template_id ?? ''}
            />
            {winnerSelectable && row.variation.template_id ? (
              <Tooltip title={canWrite ? undefined : t`You don't have write permission for broadcasts`}>
                <Button
                  size="small"
                  type="primary"
                  disabled={!canWrite}
                  onClick={() => onPickWinner(row.variation.template_id, row.templateName)}
                >
                  {t`Pick as winner`}
                </Button>
              </Tooltip>
            ) : null}
          </div>
        )
      }
    }
  ]

  return (
    <Panel
      title={t`A/B test`}
      icon={<FlaskConical size={14} />}
      extra={<AbTestSummary broadcast={broadcast} testResults={testResults} timezone={workspaceTimezone(workspace)} />}
      flush
    >
      <Table<Row>
        rowKey="key"
        size="small"
        columns={columns}
        dataSource={rows}
        pagination={false}
        scroll={{ x: 'max-content' }}
      />
    </Panel>
  )
}

/** The flag under a rate computed over fewer than half the recipients. */
function TrackedShareNote({ rate }: { rate: VariationRate }) {
  const { t } = useLingui()
  if (rate.trackedShare === undefined || !rate.lowTrackedShare) return null
  const share = Math.round(rate.trackedShare * 100)
  return <span className="text-xs text-warning-fg">{t`${share}% tracked`}</span>
}

interface AbTestSummaryProps {
  broadcast: Broadcast
  testResults?: TestResultsResponse
  timezone: string
}

/** "Sample 20 % · winner on open rate after 4 h · Test ends in 3 hours". */
export function AbTestSummary({ broadcast, testResults, timezone }: AbTestSummaryProps) {
  const { t } = useLingui()
  const dates = useBroadcastDates()
  const { int } = useNumberFormat()
  const settings = broadcast.test_settings
  const sample = settings.sample_percentage
  const hours = settings.test_duration_hours ?? 0
  const metric = settings.auto_send_winner_metric === 'click_rate' ? t`click rate` : t`open rate`
  const parts = [
    settings.auto_send_winner
      ? t`Sample ${sample} % · winner on ${metric} after ${hours} h`
      : t`Sample ${sample} % · winner picked manually`
  ]
  if (broadcast.winner_sent_at) {
    const remaining = int(broadcast.winner_phase_recipient_count ?? 0)
    const time = dates.time(broadcast.winner_sent_at, timezone)
    parts.push(t`sent to the remaining ${remaining} at ${time}`)
  } else {
    const left = getRemainingTestTime(broadcast, testResults)
    if (left) parts.push(t`Test ends in ${left}`)
  }
  return <span className="text-xs text-ink-3">{parts.join(' · ')}</span>
}
