import { createContext } from 'react'
import { Workspace } from '../../services/api/workspace'
import { WebAnalyticsSettings } from '../../services/api/web_analytics'
import { DayjsRange } from './lib/dates'
import {
  ComparisonMode,
  DatePreset,
  Granularity,
  ResolvedRange,
  WebDimensionFilter,
  WebMetricFilter
} from './lib/types'

/** What the web analytics pages share: the workspace, the period, the filters and their setters. */
export interface WebAnalyticsContextValue {
  workspaceId: string
  workspace?: Workspace
  settings?: WebAnalyticsSettings
  customDimensionLabels?: Record<string, string>

  timezone: string
  period: DatePreset
  comparison: ComparisonMode
  showComparison: boolean
  customStart?: string
  customEnd?: string

  range: DayjsRange
  resolved: ResolvedRange
  compareRange: DayjsRange | null
  resolvedCompare: ResolvedRange | null

  granularity: Granularity
  availableGranularities: Granularity[]
  setGranularity: (granularity: Granularity | null) => void

  filters: WebDimensionFilter[]
  metricFilters: WebMetricFilter[]
  minSessions: number
  dimensions: string[]
  tag?: string

  setPeriod: (period: DatePreset) => void
  setCustomRange: (start: string, end: string) => void
  setComparison: (comparison: ComparisonMode) => void
  setTimezone: (timezone: string) => void
  setFilters: (filters: WebDimensionFilter[]) => void
  /** Adds a filter, or removes it when the same one is already applied. */
  toggleFilter: (filter: WebDimensionFilter | WebDimensionFilter[]) => void
  setMetricFilters: (filters: WebMetricFilter[]) => void
  setMinSessions: (minSessions: number) => void
  setDimensions: (dimensions: string[]) => void
  setTag: (tag?: string) => void
}

export const WebAnalyticsContext = createContext<WebAnalyticsContextValue | null>(null)
