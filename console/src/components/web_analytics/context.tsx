import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useAuth } from '../../contexts/AuthContext'
import { getBrowserTimezone } from '../../lib/timezoneNormalizer'
import {
  computeComparisonRange,
  computeDateRange,
  determineGranularity,
  determineGranularityForRange,
  getAvailableGranularities,
  resolveRange
} from './lib/dates'
import { WebAnalyticsContext, type WebAnalyticsContextValue } from './webAnalyticsContext'
import {
  ComparisonMode,
  DATE_PRESETS,
  DatePreset,
  Granularity,
  WebDimensionFilter,
  WebMetricFilter
} from './lib/types'

// The tab tuple now lives in ./lib/types, the leaf module every web analytics
// module already imports: the AI tool definitions need the names as a value, and
// importing a value from here would pull the router into them. Re-exported so
// every existing importer of this module keeps working unchanged.
export { WEB_ANALYTICS_TABS, type WebAnalyticsTab } from './lib/types'

/** URL state shared by every tab; see the route's validateSearch. */
export interface WebAnalyticsSearch {
  period?: DatePreset
  timezone?: string
  comparison?: ComparisonMode
  customStart?: string
  customEnd?: string
  /** JSON-encoded WebDimensionFilter[] */
  filters?: string
  /** JSON-encoded WebMetricFilter[] */
  metricFilters?: string
  minSessions?: number
  /** Comma-separated dimension list driving the explore drill-down. */
  dimensions?: string
  /** Attribution-rule tag the filters tab is narrowed to. */
  tag?: string
}

export const DEFAULT_PERIOD: DatePreset = 'previous_7_days'
export const DEFAULT_COMPARISON: ComparisonMode = 'previous_period'
export const DEFAULT_MIN_SESSIONS = 10

const PERIOD_STORAGE_KEY = 'web_analytics_period'
const COMPARISON_STORAGE_KEY = 'web_analytics_comparison'

function parseJsonParam<T>(raw: string | undefined): T[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as T[]) : []
  } catch {
    return []
  }
}

function sameFilter(a: WebDimensionFilter, b: WebDimensionFilter): boolean {
  return (
    a.dimension === b.dimension &&
    a.operator === b.operator &&
    JSON.stringify([...a.values].sort()) === JSON.stringify([...b.values].sort())
  )
}

export function WebAnalyticsProvider(props: { workspaceId: string; children: ReactNode }) {
  const { workspaceId, children } = props
  const { workspaces } = useAuth()
  const navigate = useNavigate()
  const search = useSearch({ strict: false }) as WebAnalyticsSearch

  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const settings = workspace?.settings?.web_analytics

  // A user-chosen granularity only applies to the period it was chosen for:
  // "hourly" makes no sense once the range becomes a year.
  const [granularityOverride, setGranularityOverride] = useState<{
    scope: string
    value: Granularity
  } | null>(null)

  const patchSearch = useCallback(
    (partial: Partial<WebAnalyticsSearch>) => {
      navigate({
        to: '.',
        search: (previous: Record<string, unknown>) => ({ ...previous, ...partial }),
        replace: true
      })
    },
    [navigate]
  )

  const period = search.period && DATE_PRESETS.includes(search.period) ? search.period : DEFAULT_PERIOD
  const comparison: ComparisonMode = search.comparison ?? DEFAULT_COMPARISON
  const timezone =
    search.timezone || workspace?.settings?.timezone || getBrowserTimezone() || 'UTC'

  // Restore the last period the operator used, but never override a link they
  // followed: an explicit period in the URL always wins.
  useEffect(() => {
    if (search.period || search.comparison) return
    const storedPeriod = localStorage.getItem(PERIOD_STORAGE_KEY) as DatePreset | null
    const storedComparison = localStorage.getItem(COMPARISON_STORAGE_KEY) as ComparisonMode | null
    const patch: Partial<WebAnalyticsSearch> = {}
    if (storedPeriod && DATE_PRESETS.includes(storedPeriod) && storedPeriod !== 'custom') {
      patch.period = storedPeriod
    }
    if (storedComparison) patch.comparison = storedComparison
    if (Object.keys(patch).length > 0) patchSearch(patch)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- restore once on mount
  }, [])

  const range = useMemo(
    () =>
      computeDateRange(
        period,
        timezone,
        search.customStart && search.customEnd
          ? { start: search.customStart, end: search.customEnd }
          : undefined,
        workspace?.created_at
      ),
    [period, timezone, search.customStart, search.customEnd, workspace?.created_at]
  )

  const compareRange = useMemo(
    () => computeComparisonRange(range, comparison),
    [range, comparison]
  )

  const resolved = useMemo(() => resolveRange(range, timezone), [range, timezone])
  const resolvedCompare = useMemo(
    () => (compareRange ? resolveRange(compareRange, timezone) : null),
    [compareRange, timezone]
  )

  const rangeDays = range.end.diff(range.start, 'day')
  const availableGranularities = useMemo(() => getAvailableGranularities(rangeDays), [rangeDays])

  const granularityScope = `${period}-${search.customStart ?? ''}-${search.customEnd ?? ''}`
  const defaultGranularity =
    period === 'custom' ? determineGranularityForRange(range) : determineGranularity(period)
  const granularity =
    granularityOverride &&
    granularityOverride.scope === granularityScope &&
    availableGranularities.includes(granularityOverride.value)
      ? granularityOverride.value
      : availableGranularities.includes(defaultGranularity)
        ? defaultGranularity
        : availableGranularities[0]

  const filters = useMemo(
    () => parseJsonParam<WebDimensionFilter>(search.filters),
    [search.filters]
  )
  const metricFilters = useMemo(
    () => parseJsonParam<WebMetricFilter>(search.metricFilters),
    [search.metricFilters]
  )
  const dimensions = useMemo(
    () => (search.dimensions ? search.dimensions.split(',').filter(Boolean) : []),
    [search.dimensions]
  )

  const setFilters = useCallback(
    (next: WebDimensionFilter[]) => {
      patchSearch({ filters: next.length > 0 ? JSON.stringify(next) : undefined })
    },
    [patchSearch]
  )

  const toggleFilter = useCallback(
    (incoming: WebDimensionFilter | WebDimensionFilter[]) => {
      const additions = Array.isArray(incoming) ? incoming : [incoming]
      let next = [...filters]
      for (const addition of additions) {
        const existing = next.find((candidate) => sameFilter(candidate, addition))
        if (existing) {
          next = next.filter((candidate) => candidate !== existing)
          continue
        }
        // One filter per dimension: clicking another row of the same table
        // replaces the drill-down instead of producing an impossible AND.
        next = next.filter((candidate) => candidate.dimension !== addition.dimension)
        next.push(addition)
      }
      setFilters(next)
    },
    [filters, setFilters]
  )

  const value: WebAnalyticsContextValue = {
    workspaceId,
    workspace,
    settings,
    customDimensionLabels: settings?.custom_dimension_labels,
    timezone,
    period,
    comparison,
    showComparison: comparison !== 'none',
    customStart: search.customStart,
    customEnd: search.customEnd,
    range,
    resolved,
    compareRange,
    resolvedCompare,
    granularity,
    availableGranularities,
    setGranularity: (next) =>
      setGranularityOverride(next ? { scope: granularityScope, value: next } : null),
    filters,
    metricFilters,
    minSessions: search.minSessions ?? DEFAULT_MIN_SESSIONS,
    dimensions,
    tag: search.tag,
    setPeriod: (next) => {
      localStorage.setItem(PERIOD_STORAGE_KEY, next)
      patchSearch(
        next === 'custom'
          ? { period: next }
          : { period: next, customStart: undefined, customEnd: undefined }
      )
    },
    setCustomRange: (start, end) =>
      patchSearch({ period: 'custom', customStart: start, customEnd: end }),
    setComparison: (next) => {
      localStorage.setItem(COMPARISON_STORAGE_KEY, next)
      patchSearch({ comparison: next })
    },
    setTimezone: (next) => patchSearch({ timezone: next }),
    setFilters,
    toggleFilter,
    setMetricFilters: (next) =>
      patchSearch({ metricFilters: next.length > 0 ? JSON.stringify(next) : undefined }),
    setMinSessions: (next) =>
      patchSearch({ minSessions: next > 1 ? next : undefined }),
    setDimensions: (next) =>
      patchSearch({ dimensions: next.length > 0 ? next.join(',') : undefined }),
    setTag: (next) => patchSearch({ tag: next })
  }

  return <WebAnalyticsContext.Provider value={value}>{children}</WebAnalyticsContext.Provider>
}
