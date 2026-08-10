import { ReactNode, useCallback, useMemo, useState } from 'react'
import ReactECharts from 'echarts-for-react'
import { Empty, Spin } from 'antd'
import { i18n } from '@lingui/core'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from './context'
import { WidgetTabs } from './WidgetTabs'
import { HOUR_LABELS, weekdayLabel } from './lib/dictionaries'
import { formatValue, toNumber } from './lib/format'
import { mergeWidgetFilters } from './lib/dimensions'
import { getTimescoreCellColor } from './lib/heatmap'
import { buildWebQuery, readMeasure, useWebQuery } from './lib/query'
import { PRIMARY_COLOR, WebDimensionFilter } from './lib/types'

export interface HeatmapTab {
  key: string
  label: string
  /** Measure of the widget's schema this tab shades cells by. */
  measure: string
  format: 'number' | 'duration'
}



interface TrafficHeatmapWidgetProps {
  title: string
  schema?: 'web_sessions' | 'web_goals'
  tabs?: HeatmapTab[]
  /** Applied on top of the global filters, for a widget scoped to one goal. */
  extraFilters?: WebDimensionFilter[]
  emptyText?: string
}

interface TooltipParams {
  data?: [number, number, number] | { value: [number, number, number] }
}

function coordinates(
  params: TooltipParams
): [hour: number, day: number, value: number] | undefined {
  if (!params.data) return undefined
  return Array.isArray(params.data) ? params.data : params.data.value
}

export function TrafficHeatmapWidget(props: TrafficHeatmapWidgetProps): ReactNode {
  const { title, schema = 'web_sessions', extraFilters, emptyText } = props
  const { t } = useLingui()
  const { workspaceId, resolved, timezone, filters, toggleFilter } = useWebAnalytics()
  const [activeKey, setActiveKey] = useState<string | null>(null)

  const tabs = useMemo<HeatmapTab[]>(
    () =>
      props.tabs?.length
        ? props.tabs
        : [
            { key: 'sessions', label: t`Sessions`, measure: 'sessions', format: 'number' },
            { key: 'timescore', label: t`TimeScore`, measure: 'median_duration', format: 'duration' }
          ],
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `t` is a new function on every render
    [props.tabs]
  )

  const activeTab = tabs.find((tab) => tab.key === activeKey) ?? tabs[0]

  // Every tab's measure rides along in one query, so switching tabs re-colours
  // the grid the operator is already looking at instead of refetching it.
  const measures = useMemo(() => [...new Set(tabs.map((tab) => tab.measure))], [tabs])

  const query = useMemo(
    () =>
      buildWebQuery({
        schema,
        measures,
        dimensions: ['day_of_week', 'hour_of_day'],
        range: resolved,
        filters: mergeWidgetFilters(filters, extraFilters, schema),
        timezone,
        limit: 7 * 24
      }),
    [schema, measures, resolved, filters, extraFilters, timezone]
  )

  const { data: response, isLoading } = useWebQuery(workspaceId, query)
  const rows = response?.data

  // One 7 × 24 grid per measure, zero-filled: the engine only returns the
  // buckets that saw traffic, and an absent cell means zero, not a hole.
  const grids = useMemo(() => {
    const byMeasure: Record<string, number[][]> = {}
    for (const measure of measures) {
      byMeasure[measure] = Array.from({ length: 7 }, () => new Array<number>(24).fill(0))
    }
    for (const row of rows ?? []) {
      const day = toNumber(row.day_of_week) - 1
      const hour = toNumber(row.hour_of_day)
      if (day < 0 || day > 6 || hour < 0 || hour > 23) continue
      for (const measure of measures) byMeasure[measure][day][hour] = readMeasure(row, measure)
    }
    return byMeasure
  }, [rows, measures])

  const cells = useMemo(() => {
    const grid = grids[activeTab.measure]
    const result: [number, number, number][] = []
    if (!grid) return result
    for (let day = 0; day < 7; day++) {
      for (let hour = 0; hour < 24; hour++) result.push([hour, day, grid[day][hour]])
    }
    return result
  }, [grids, activeTab.measure])

  const maxValue = useMemo(() => cells.reduce((max, cell) => Math.max(max, cell[2]), 0), [cells])

  // Built here rather than at module scope: the labels follow the console's
  // locale, which is not known when the module is first evaluated. Memoized on
  // the locale so the option below keeps its own memo.
  const dayLabels = useMemo(
    () => [1, 2, 3, 4, 5, 6, 7].map((day) => weekdayLabel(day, i18n.locale, 'short') ?? String(day)),
    [i18n.locale]
  )

  const option = useMemo(() => {
    // A duration is judged against the TimeScore reference rather than against
    // the busiest cell, so its colours are set per cell instead of by a scale.
    const perCellColor = activeTab.format === 'duration'

    return {
      animation: false,
      tooltip: {
        trigger: 'item',
        backgroundColor: 'rgba(255, 255, 255, 0.95)',
        borderColor: '#e5e7eb',
        borderWidth: 1,
        textStyle: { color: '#374151', fontSize: 12 },
        formatter: (params: TooltipParams) => {
          const cell = coordinates(params)
          if (!cell) return ''
          const [hour, day, value] = cell
          return `<div style="font-weight:500">${dayLabels[day]} ${HOUR_LABELS[hour]}</div><div>${
            activeTab.label
          }: ${formatValue(value, activeTab.format)}</div>`
        }
      },
      grid: { left: 50, right: 20, top: 20, bottom: 40 },
      xAxis: {
        type: 'category',
        data: HOUR_LABELS,
        splitArea: { show: true },
        axisLine: { lineStyle: { color: '#e5e7eb' } },
        axisLabel: { color: '#6b7280', fontSize: 10, interval: 2 }
      },
      yAxis: {
        type: 'category',
        data: dayLabels,
        // Categories run bottom-up by default, which would print the week
        // upside down.
        inverse: true,
        splitArea: { show: true },
        axisLine: { lineStyle: { color: '#e5e7eb' } },
        axisLabel: { color: '#6b7280', fontSize: 10 }
      },
      visualMap: perCellColor
        ? { show: false }
        : {
            min: 0,
            max: maxValue || 1,
            show: false,
            inRange: {
              color: [
                '#f5f5f5',
                'rgba(119, 99, 241, 0.2)',
                'rgba(119, 99, 241, 0.4)',
                'rgba(119, 99, 241, 0.6)',
                'rgba(119, 99, 241, 0.8)',
                'rgba(119, 99, 241, 1)'
              ]
            }
          },
      series: [
        {
          type: 'heatmap',
          data: perCellColor
            ? cells.map(([hour, day, value]) => ({
                value: [hour, day, value],
                itemStyle: { color: getTimescoreCellColor(value, maxValue) }
              }))
            : cells,
          emphasis: { itemStyle: { borderColor: PRIMARY_COLOR, borderWidth: 2 } },
          label: { show: false }
        }
      ]
    }
  }, [cells, maxValue, activeTab, dayLabels])

  const onEvents = useMemo(
    () => ({
      click: (params: TooltipParams) => {
        const cell = coordinates(params)
        if (!cell) return
        const [hour, day] = cell
        // Both halves of the cell go in one call so the URL is rewritten once.
        toggleFilter([
          { dimension: 'day_of_week', operator: 'equals', values: [day + 1] },
          { dimension: 'hour_of_day', operator: 'equals', values: [hour] }
        ])
      }
    }),
    [toggleFilter]
  )

  const selectTab = useCallback((key: string) => setActiveKey(key), [])

  const hasData = (rows?.length ?? 0) > 0

  return (
    <div className="rounded-lg border border-gray-200 overflow-hidden">
      <div className="px-4 pt-4 pb-4">
        <h3 className="text-base font-semibold text-gray-900">{title}</h3>
      </div>

      <WidgetTabs tabs={tabs} activeKey={activeTab.key} onChange={selectTab} />

      {isLoading && !hasData ? (
        <div className="flex items-center justify-center py-12">
          <Spin />
        </div>
      ) : !hasData ? (
        <Empty
          description={emptyText ?? t`No data available`}
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          className="py-8"
        />
      ) : (
        <ReactECharts
          option={option}
          style={{ height: 300, cursor: 'pointer' }}
          opts={{ renderer: 'svg' }}
          onEvents={onEvents}
          notMerge
        />
      )}
    </div>
  )
}
