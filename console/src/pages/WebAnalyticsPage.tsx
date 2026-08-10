import { ReactNode, Suspense, lazy, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { Button, Skeleton, Tooltip } from 'antd'
import { Download } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import {
  WEB_ANALYTICS_TABS,
  WebAnalyticsProvider,
  WebAnalyticsTab,
  useWebAnalytics
} from '../components/web_analytics/context'
import { ComparisonPicker, DateRangePicker } from '../components/web_analytics/toolbar'
import { CsvExportModal } from '../components/web_analytics/explore/CsvExportModal'
import { DimensionSelector } from '../components/web_analytics/explore/DimensionSelector'
import { MinSessionsInput } from '../components/web_analytics/explore/MinSessionsInput'
import { FilterBuilder } from '../components/web_analytics/FilterBuilder'
import { DashboardTab } from '../components/web_analytics/tabs/DashboardTab'
import { FiltersTab } from '../components/web_analytics/tabs/FiltersTab'
import { LiveButton } from '../components/web_analytics/LiveButton'

// Explore and Goals pull in the drill-down table and the goal drawer, neither
// of which the landing section needs.
const ExploreTab = lazy(() =>
  import('../components/web_analytics/tabs/ExploreTab').then((module) => ({
    default: module.ExploreTab
  }))
)
const GoalsTab = lazy(() =>
  import('../components/web_analytics/tabs/GoalsTab').then((module) => ({
    default: module.GoalsTab
  }))
)

/** Sections that read analytics data, and so share the period toolbar. */
const DATA_SECTIONS: WebAnalyticsTab[] = ['dashboard', 'explore', 'goals']

export function WebAnalyticsPage() {
  const { workspaceId, tab } = useParams({
    from: '/console/workspace/$workspaceId/web-analytics/$tab'
  })

  return (
    <WebAnalyticsProvider workspaceId={workspaceId}>
      <WebAnalyticsSection workspaceId={workspaceId} tab={tab as WebAnalyticsTab} />
    </WebAnalyticsProvider>
  )
}

// Sections are reached from the workspace sidebar; the route param alone says
// which one to render.
function WebAnalyticsSection(props: { workspaceId: string; tab: WebAnalyticsTab }) {
  const { t } = useLingui()
  const { settings, dimensions, setDimensions } = useWebAnalytics()

  const activeTab = WEB_ANALYTICS_TABS.includes(props.tab) ? props.tab : 'dashboard'

  // Explore opens on the report picker, which answers "which report" rather
  // than reading data. A range and a segment narrow a report you already have,
  // so they only earn their row once one is chosen.
  const pickingReport = activeTab === 'explore' && dimensions.length === 0
  const showToolbar = DATA_SECTIONS.includes(activeTab) && !pickingReport

  // Explore builds its own report, so it owns a row the other tabs do not: the
  // dimensions that define it. That row displaces its period controls up
  // beside the title, and the export button rides along with them.
  const isExplore = activeTab === 'explore'
  const [csvOpen, setCsvOpen] = useState(false)

  // The dashboard is the feature's landing page and keeps its name; the other
  // sections title themselves, since the sidebar entry is all that says where
  // you are now that the tab strip is gone.
  const titles: Record<WebAnalyticsTab, string> = {
    dashboard: t`Web Analytics`,
    explore: t`Explore`,
    goals: t`Goals`,
    filters: t`Filters`
  }

  const panes: Record<WebAnalyticsTab, ReactNode> = {
    dashboard: <DashboardTab />,
    explore: (
      <Suspense fallback={<Skeleton active />}>
        <ExploreTab />
      </Suspense>
    ),
    goals: (
      <Suspense fallback={<Skeleton active />}>
        <GoalsTab />
      </Suspense>
    ),
    filters: <FiltersTab />
  }

  const periodControls = (
    <div className="flex flex-wrap items-center gap-2">
      {isExplore ? (
        <Tooltip title={t`Export to CSV`}>
          <Button
            type="text"
            size="small"
            icon={<Download size={16} />}
            onClick={() => setCsvOpen(true)}
          />
        </Tooltip>
      ) : null}
      <DateRangePicker />
      <ComparisonPicker />
    </div>
  )

  return (
    <div className="p-4 md:p-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="text-2xl font-medium">{titles[activeTab]}</div>
          {/* The dashboard is the section's landing page, so the live count
              belongs there; on the other tabs it is a second, unrelated period
              sitting next to a title about the one you chose. */}
          {activeTab === 'dashboard' && settings?.enabled ? <LiveButton /> : null}
        </div>
        {showToolbar && settings && isExplore ? periodControls : null}
      </div>

      {/* Dimensions above filters: the dimensions are the report, the filters
          only narrow it, so they read in the order they are applied. */}
      {showToolbar && settings && isExplore ? (
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <DimensionSelector value={dimensions} onChange={setDimensions} />
          <MinSessionsInput />
        </div>
      ) : null}

      {showToolbar && settings ? (
        // On the other tabs the period controls sit with the filter bar rather
        // than in the page header: both narrow the same question, and a reader
        // scanning a chart looks for "which range" beside "which segment".
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <FilterBuilder
            schema={activeTab === 'goals' ? 'web_goals' : 'web_sessions'}
            allowMetricFilters={activeTab === 'explore'}
          />
          {isExplore ? null : periodControls}
        </div>
      ) : null}

      {settings ? panes[activeTab] : <NotConfigured workspaceId={props.workspaceId} />}

      {/* Lives here rather than in the tab because its trigger moved up to the
          page header; the modal reads the report it exports from context. */}
      <CsvExportModal open={csvOpen} onCancel={() => setCsvOpen(false)} />
    </div>
  )
}

/**
 * Web analytics only appears in the workspace once its settings exist. Until
 * then every data tab would query tables the schema resolver does not expose,
 * so they point at the workspace settings instead of failing one widget at a
 * time.
 */
function NotConfigured(props: { workspaceId: string }) {
  const { t } = useLingui()
  return (
    <div className="rounded-md bg-white p-8 text-center text-gray-500">
      <p className="mb-4">{t`Web analytics is not set up on this workspace yet.`}</p>
      <Link
        to="/console/workspace/$workspaceId/settings/$section"
        params={{ workspaceId: props.workspaceId, section: 'web-analytics' }}
        className="text-[var(--primary)]"
      >
        {t`Open the workspace settings to install the tracking snippet`}
      </Link>
    </div>
  )
}
