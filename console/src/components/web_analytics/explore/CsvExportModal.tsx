import { useState } from 'react'
import { Alert, App, Button, Descriptions, Modal } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import { Plural, useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { useWebAnalytics } from '../context'
import { getDimensionLabel } from '../lib/dimensions'
import {
  buildCsv,
  downloadCsv,
  exploreRowToCsvValues,
  mergeComparisonRowsForCsv
} from '../lib/csv'
import { toNumber } from '../lib/format'
import { buildWebQuery, webAnalyticsClient } from '../lib/query'
import { SESSION_METRIC_KEYS } from '../lib/types'

interface CsvExportModalProps {
  open: boolean
  onCancel: () => void
}

const MAX_ROWS = 1000

/**
 * Exports the report as one flat table.
 *
 * The screen loads a level at a time and only where the operator expanded, so
 * the export runs its own query grouped by every dimension at once: a
 * spreadsheet wants the full cross-product, not whichever branches happened to
 * be open.
 */
export function CsvExportModal(props: CsvExportModalProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const context = useWebAnalytics()
  const [exporting, setExporting] = useState(false)

  const filename = `explore-report-${dayjs().format('YYYY-MM-DD')}.csv`

  const headers = [
    ...context.dimensions.map((dimension) =>
      getDimensionLabel(dimension, context.customDimensionLabels)
    ),
    t`Sessions`,
    t`Sessions (%)`,
    t`TimeScore`,
    t`TimeScore (seconds)`,
    t`Bounce Rate (%)`,
    t`Median Scroll Depth (%)`
  ]

  if (context.showComparison) {
    headers.push(
      t`Sessions (previous)`,
      t`Sessions (change %)`,
      t`TimeScore (previous)`,
      t`TimeScore (change %)`,
      t`Bounce Rate (previous)`,
      t`Bounce Rate (change %)`,
      t`Median Scroll Depth (previous)`,
      t`Median Scroll Depth (change %)`
    )
  }

  const runExport = async () => {
    setExporting(true)
    try {
      const base = {
        schema: 'web_sessions' as const,
        measures: SESSION_METRIC_KEYS,
        dimensions: context.dimensions,
        filters: context.filters,
        metricFilters: context.metricFilters,
        minSessions: context.minSessions,
        order: { sessions: 'desc' as const },
        limit: MAX_ROWS,
        timezone: context.timezone
      }

      const current = await webAnalyticsClient.query(
        buildWebQuery({ ...base, range: context.resolved }),
        context.workspaceId
      )
      const previous =
        context.showComparison && context.resolvedCompare
          ? await webAnalyticsClient.query(
              buildWebQuery({ ...base, range: context.resolvedCompare }),
              context.workspaceId
            )
          : undefined

      const rows = context.showComparison
        ? mergeComparisonRowsForCsv(current.data, previous?.data, context.dimensions)
        : current.data

      const totalSessions = rows.reduce((sum, row) => sum + toNumber(row.sessions), 0)
      const emptyLabel = t`(empty)`

      downloadCsv(
        buildCsv(
          headers,
          rows.map((row) =>
            exploreRowToCsvValues(
              row,
              context.dimensions,
              totalSessions,
              context.showComparison,
              emptyLabel
            )
          )
        ),
        filename
      )

      message.success(<Plural value={rows.length} one="Exported # row" other="Exported # rows" />)
      props.onCancel()
    } catch {
      message.error(t`The export failed. Please try again.`)
    } finally {
      setExporting(false)
    }
  }

  return (
    <Modal
      open={props.open}
      title={t`Export to CSV`}
      width={500}
      centered
      onCancel={props.onCancel}
      footer={[
        <Button key="cancel" onClick={props.onCancel}>
          {t`Cancel`}
        </Button>,
        <Button
          key="export"
          type="primary"
          icon={<DownloadOutlined />}
          loading={exporting}
          onClick={runExport}
        >
          {t`Download`}
        </Button>
      ]}
    >
      <Alert
        type="info"
        showIcon
        className="!mt-4"
        title={t`What gets exported`}
        description={
          <ul className="mt-2 list-disc space-y-1 pl-5">
            <li>
              {t`Dimensions:`}{' '}
              {context.dimensions
                .map((dimension) => getDimensionLabel(dimension, context.customDimensionLabels))
                .join(' → ')}
            </li>
            <li>{t`The top ${MAX_ROWS} rows by session count`}</li>
            <li>{t`The current date range, filters and session threshold`}</li>
            {context.showComparison ? <li>{t`The comparison period as extra columns`}</li> : null}
          </ul>
        }
      />
      <Descriptions column={1} size="small" bordered className="!mt-4">
        <Descriptions.Item label={t`File name`}>{filename}</Descriptions.Item>
      </Descriptions>
    </Modal>
  )
}
