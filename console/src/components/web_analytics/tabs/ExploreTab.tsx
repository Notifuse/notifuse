import { Key, useCallback, useEffect, useMemo, useState } from 'react'
import { Alert } from 'antd'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import { BreakdownDrawer, buildBreakdownQuery } from '../explore/BreakdownDrawer'
import { BreakdownModal } from '../explore/BreakdownModal'
import { ExploreSummary } from '../explore/ExploreSummary'
import { ExploreTable } from '../explore/ExploreTable'
import { ExploreTemplates } from '../explore/ExploreTemplates'
import {
  buildExploreRows,
  calculateChildrenDimensionsAndFilters,
  ExploreRow,
  ExploreTotals,
  findMaxMeasure,
  insertChildrenIntoTree,
  mergeComparisonData,
  rowCoversDimensions,
  setRowLoading
} from '../lib/exploreRows'
import { changePercent } from '../lib/format'
import {
  buildBestCombinationQuery,
  buildWebQuery,
  readMeasure,
  useWebComparisonQuery,
  useWebQuery,
  webAnalyticsClient
} from '../lib/query'
import { ResolvedRange, SESSION_METRIC_KEYS, WebDimensionFilter } from '../lib/types'

/** Rows fetched per level. Going deeper is how the report narrows, not paging. */
const LEVEL_LIMIT = 100

interface BreakdownState {
  row: ExploreRow
  dimensions: string[]
  parentFilters: WebDimensionFilter[]
  stage: 'modal' | 'drawer'
}

/**
 * The explore report: an ordered list of dimensions rendered as a drill-down.
 *
 * Only the top level is queried up front. Expanding a row queries that branch
 * alone, so a four-dimension report costs one query plus one per branch the
 * operator actually opens, instead of the full cross-product.
 */
export function ExploreTab() {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const queryClient = useQueryClient()

  const [reportData, setReportData] = useState<ExploreRow[]>([])
  const [expandedRowKeys, setExpandedRowKeys] = useState<Key[]>([])
  const [loadingRows, setLoadingRows] = useState<Set<string>>(new Set())
  const [bestValue, setBestValue] = useState(0)
  const [breakdown, setBreakdown] = useState<BreakdownState | null>(null)
  const [pendingFilters, setPendingFilters] = useState<WebDimensionFilter[] | null>(null)

  const { dimensions, filters, metricFilters, minSessions, showComparison } = context
  const hasDimensions = dimensions.length > 0

  const levelBase = useMemo(
    () => ({
      schema: 'web_sessions' as const,
      measures: SESSION_METRIC_KEYS,
      filters,
      metricFilters,
      minSessions,
      order: { sessions: 'desc' as const },
      limit: LEVEL_LIMIT,
      timezone: context.timezone
    }),
    [filters, metricFilters, minSessions, context.timezone]
  )

  const rootResult = useWebComparisonQuery(
    context.workspaceId,
    hasDimensions
      ? buildWebQuery({ ...levelBase, dimensions: [dimensions[0]], range: context.resolved })
      : null,
    hasDimensions && showComparison && context.resolvedCompare
      ? buildWebQuery({
          ...levelBase,
          dimensions: [dimensions[0]],
          range: context.resolvedCompare
        })
      : null
  )

  const rootCurrent = rootResult.current
  const rootPrevious = rootResult.previous

  // Totals deliberately ignore the metric filters and the session threshold:
  // those select rows of a breakdown, and the engine has no way to re-aggregate
  // the surviving rows into a period median. The shares in the table are
  // therefore shares of the period, which is what they claim to be.
  const totalsBase = useMemo(
    () => ({
      schema: 'web_sessions' as const,
      measures: SESSION_METRIC_KEYS,
      dimensions: [],
      filters,
      timezone: context.timezone
    }),
    [filters, context.timezone]
  )

  const totalsResult = useWebComparisonQuery(
    context.workspaceId,
    hasDimensions ? buildWebQuery({ ...totalsBase, range: context.resolved }) : null,
    hasDimensions && showComparison && context.resolvedCompare
      ? buildWebQuery({ ...totalsBase, range: context.resolvedCompare })
      : null
  )

  // The best combination is a property of the whole report, not of a level, so
  // this is the one query that groups by every dimension at once. Ordering by
  // TimeScore and taking a single row returns the winner and the values that
  // achieved it together, which is what the summary tooltip names.
  const bestResult = useWebQuery(
    context.workspaceId,
    hasDimensions ? buildBestCombinationQuery(levelBase, dimensions, context.resolved) : null
  )

  // `keepPreviousData` serves the previous report's winner while a new
  // dimension list is in flight, and that row has no column for a dimension
  // just added.
  const bestRow = bestResult.data?.data?.[0]
  const bestIsCurrent = rowCoversDimensions(bestRow, dimensions)
  const bestTimeScore = bestIsCurrent ? readMeasure(bestRow, 'median_duration') : undefined

  // The server value is the true maximum across every combination, so it is
  // what the heat scale should be anchored to. The running client max stays as
  // a floor: `minSessions` can drop a combination from the query above while
  // the tree still shows a row that beats what survived.
  const heatCeiling = Math.max(bestValue, bestTimeScore ?? 0)

  const totalsCurrent = totalsResult.current
  const totalsPrevious = totalsResult.previous

  const totals: ExploreTotals | undefined = useMemo(() => {
    const row = totalsCurrent?.data?.[0]
    if (!row) return undefined
    const previousRow = totalsPrevious?.data?.[0]
    const value = (measure: string) => readMeasure(row, measure)
    const change = (measure: string) =>
      previousRow ? changePercent(value(measure), readMeasure(previousRow, measure)) : undefined

    return {
      sessions: value('sessions'),
      median_duration: value('median_duration'),
      bounce_rate: value('bounce_rate'),
      median_scroll: value('median_scroll'),
      sessions_change: change('sessions'),
      median_duration_change: change('median_duration'),
      bounce_rate_change: change('bounce_rate'),
      median_scroll_change: change('median_scroll')
    }
  }, [totalsCurrent, totalsPrevious])

  // A new root level invalidates every loaded branch, so the tree is rebuilt
  // from scratch and everything collapses.
  useEffect(() => {
    if (!hasDimensions || !rootCurrent) {
      setReportData([])
      setBestValue(0)
      setExpandedRowKeys([])
      return
    }

    // `data` is typed as an array but the API can still answer null for a
    // breakdown that matched nothing, so the type does not protect this call.
    const rows = showComparison
      ? mergeComparisonData(rootCurrent.data ?? [], rootPrevious?.data, dimensions[0])
      : (rootCurrent.data ?? [])

    const exploreRows = buildExploreRows(rows, dimensions, 0, null, showComparison)
    setReportData(exploreRows)
    setBestValue(findMaxMeasure(exploreRows, 'median_duration'))
    setExpandedRowKeys([])
  }, [rootCurrent, rootPrevious, dimensions, showComparison, hasDimensions])

  const fetchChildren = useCallback(
    async (record: ExploreRow) => {
      if (record.childrenLoaded || record.isLoading) return

      setLoadingRows((current) => new Set(current).add(record.key))
      setReportData((current) => setRowLoading(current, record.key, true))

      try {
        const { childDimensionIndex, dimensionsToFetch, filters: childFilters } =
          calculateChildrenDimensionsAndFilters(record, dimensions, filters)

        const run = (range: ResolvedRange) => {
          const query = buildWebQuery({
            ...levelBase,
            dimensions: dimensionsToFetch,
            filters: childFilters,
            range
          })
          return queryClient.fetchQuery({
            queryKey: ['web-analytics', context.workspaceId, query],
            queryFn: () => webAnalyticsClient.query(query, context.workspaceId),
            staleTime: 30_000
          })
        }

        const [current, previous] = await Promise.all([
          run(context.resolved),
          showComparison && context.resolvedCompare ? run(context.resolvedCompare) : undefined
        ])

        const childDimension = dimensions[childDimensionIndex]
        const rows = showComparison
          ? mergeComparisonData(current.data ?? [], previous?.data, childDimension)
          : (current.data ?? [])

        const childRows = buildExploreRows(
          rows,
          dimensions,
          childDimensionIndex,
          record.key,
          showComparison
        )

        // An empty branch under a threshold is the threshold talking, not an
        // absence of traffic; the table says so instead of showing nothing.
        const filteredOut = childRows.length === 0 && minSessions > 1
        setReportData((tree) =>
          insertChildrenIntoTree(tree, record.key, childRows, filteredOut)
        )
        setBestValue((max) => Math.max(max, findMaxMeasure(childRows, 'median_duration')))
      } catch {
        setReportData((tree) => setRowLoading(tree, record.key, false))
      } finally {
        setLoadingRows((current) => {
          const next = new Set(current)
          next.delete(record.key)
          return next
        })
      }
    },
    [
      dimensions,
      filters,
      levelBase,
      minSessions,
      queryClient,
      showComparison,
      context.resolved,
      context.resolvedCompare,
      context.workspaceId
    ]
  )

  const handleExpand = useCallback(
    (expanded: boolean, record: ExploreRow) => {
      if (expanded && !record.childrenLoaded) fetchChildren(record)
    },
    [fetchChildren]
  )

  /** The row's path as filters, plus the report dimensions still free below it. */
  const breakdownContextFor = useCallback(
    (row: ExploreRow) => {
      const { filters: parentFilters } = calculateChildrenDimensionsAndFilters(
        row,
        dimensions,
        filters
      )
      const used = new Set(dimensions.slice(0, row.dimensionIndex + 1))
      const available = dimensions.filter((dimension) => !used.has(dimension))
      return { parentFilters, available }
    },
    [dimensions, filters]
  )

  const openBreakdown = useCallback(
    (row: ExploreRow) => {
      const { parentFilters, available } = breakdownContextFor(row)
      setBreakdown({
        row,
        parentFilters,
        dimensions: available.slice(0, 2),
        stage: 'modal'
      })
    },
    [breakdownContextFor]
  )

  const prefetchBreakdown = useCallback(
    (row: ExploreRow) => {
      const { parentFilters, available } = breakdownContextFor(row)
      for (const dimension of available.slice(0, 2)) {
        const query = buildBreakdownQuery({
          dimension,
          filters: parentFilters,
          metricFilters,
          minSessions,
          range: context.resolved,
          timezone: context.timezone
        })
        queryClient.prefetchQuery({
          queryKey: ['web-analytics', context.workspaceId, query],
          queryFn: () => webAnalyticsClient.query(query, context.workspaceId),
          staleTime: 60_000
        })
      }
    },
    [
      breakdownContextFor,
      metricFilters,
      minSessions,
      queryClient,
      context.resolved,
      context.timezone,
      context.workspaceId
    ]
  )

  const applyTemplate = useCallback(
    (nextDimensions: string[], templateFilters?: WebDimensionFilter[]) => {
      // Both live in the URL and each setter patches it once, so a second
      // synchronous patch would read the search params from before the first.
      // The filters are held back until the dimensions have landed.
      if (templateFilters?.length) setPendingFilters(templateFilters)
      context.setDimensions(nextDimensions)
    },
    [context]
  )

  useEffect(() => {
    if (!pendingFilters || !hasDimensions) return
    context.setFilters(pendingFilters)
    setPendingFilters(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- runs once the dimension patch lands
  }, [pendingFilters, hasDimensions])

  if (!hasDimensions) {
    return <ExploreTemplates onSelect={applyTemplate} />
  }

  return (
    <div>
      {rootResult.error ? (
        <Alert
          type="error"
          showIcon
          className="mb-4"
          title={t`Could not load this report`}
          description={
            rootResult.error instanceof Error ? rootResult.error.message : undefined
          }
        />
      ) : null}

      <ExploreSummary
        totals={totals}
        showComparison={showComparison}
        loading={totalsResult.isLoading}
        bestValue={heatCeiling}
        bestTimeScore={bestTimeScore}
        bestCombination={bestIsCurrent ? bestRow : undefined}
        bestError={Boolean(bestResult.error)}
        dimensions={dimensions}
      />

      <ExploreTable
        data={reportData}
        dimensions={dimensions}
        expandedRowKeys={expandedRowKeys}
        onExpand={handleExpand}
        onExpandedRowsChange={setExpandedRowKeys}
        loadingRows={loadingRows}
        bestValue={heatCeiling}
        bestCombination={bestIsCurrent ? bestRow : undefined}
        loading={rootResult.isLoading && reportData.length === 0}
        totals={totals}
        onBreakdownClick={openBreakdown}
        onBreakdownHover={prefetchBreakdown}
      />

      <BreakdownModal
        open={breakdown?.stage === 'modal'}
        initialDimensions={breakdown?.dimensions ?? []}
        excludeDimensions={
          breakdown ? dimensions.slice(0, breakdown.row.dimensionIndex + 1) : []
        }
        onCancel={() => setBreakdown(null)}
        onSubmit={(selected) =>
          setBreakdown((current) =>
            current ? { ...current, dimensions: selected, stage: 'drawer' } : current
          )
        }
      />

      {breakdown?.stage === 'drawer' ? (
        <BreakdownDrawer
          open
          onClose={() => setBreakdown(null)}
          row={breakdown.row}
          breakdownDimensions={breakdown.dimensions}
          parentFilters={breakdown.parentFilters}
          dimensions={dimensions}
        />
      ) : null}

    </div>
  )
}
