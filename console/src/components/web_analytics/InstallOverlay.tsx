import { ReactNode, useCallback, useEffect, useState } from 'react'
import { Button } from 'antd'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { Code2, Settings2 } from 'lucide-react'
import {
  buildInstallSnippet,
  resolveTrackingEndpoint
} from '../../services/api/web_analytics'
import { CodeSnippet } from '../common/CodeSnippet'
import { useWebAnalytics } from './useWebAnalytics'
import { InstallState, useInstallStatus } from './lib/installStatus'

/**
 * Dismissal lasts for the browser tab and no longer: the next visit should say
 * again that nothing is being recorded, because that is still true.
 */
const DISMISS_KEY_PREFIX = 'web_analytics_install_dismissed:'

function readDismissed(workspaceId: string): boolean {
  try {
    return window.sessionStorage.getItem(DISMISS_KEY_PREFIX + workspaceId) === '1'
  } catch {
    // Storage can be unavailable (private browsing, blocked cookies); the
    // overlay simply stays dismissible for the life of the component.
    return false
  }
}

function writeDismissed(workspaceId: string): void {
  try {
    window.sessionStorage.setItem(DISMISS_KEY_PREFIX + workspaceId, '1')
  } catch {
    /* see readDismissed */
  }
}

/** States where the operator is looking at an install problem, not a report. */
const TRAFFIC_STATES: InstallState[] = ['never_received', 'stalled']

/**
 * Takes the blurred report out of the tab order and the accessibility tree.
 * `pointer-events-none` only stops the mouse: without this, Tab walks through
 * a dozen invisible controls — and opens their popups over the card — before
 * reaching it. React 18's typings predate `inert`, and a boolean `true` is
 * dropped with a DOM warning, so it is spread as the empty string.
 */
const INERT = { inert: '' } as unknown as { inert?: string }

interface InstallCardProps {
  state: InstallState
  /** Provided only when there is something behind the card worth revealing. */
  onDismiss?: () => void
}

function InstallCard({ state, onDismiss }: InstallCardProps) {
  const { t } = useLingui()
  const { workspaceId, workspace } = useWebAnalytics()

  const showSnippet = TRAFFIC_STATES.includes(state)
  const snippet =
    showSnippet && workspace
      ? buildInstallSnippet(resolveTrackingEndpoint(workspace), workspace.id)
      : ''

  const title =
    state === 'never_received'
      ? t`Install the tracking snippet`
      : state === 'stalled'
        ? t`No traffic in the last 24 hours`
        : t`Web analytics is not enabled`

  const description =
    state === 'never_received'
      ? t`Web analytics is enabled on this workspace, but no visit has been recorded yet. Paste this snippet before the closing </head> tag of your website.`
      : state === 'stalled'
        ? t`Nothing has been recorded over the last 24 hours. Check that this snippet is still installed on your website.`
        : state === 'disabled'
          ? t`Web analytics is turned off for this workspace. Turn it on in the workspace settings to start collecting traffic.`
          : t`Web analytics is not set up on this workspace yet.`

  return (
    <div className="w-full max-w-2xl rounded-lg border border-gray-200 bg-white p-8 shadow-lg">
      <div className="mb-4 flex items-center gap-3">
        <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gray-100 text-gray-500">
          {showSnippet ? <Code2 size={18} /> : <Settings2 size={18} />}
        </span>
        <div className="text-lg font-medium">{title}</div>
      </div>

      <p className="mb-5 text-gray-500">{description}</p>

      {showSnippet ? (
        <div className="mb-5">
          <CodeSnippet code={snippet} language="markup" />
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Link
          to="/console/workspace/$workspaceId/settings/$section"
          params={{ workspaceId, section: 'web-analytics' }}
        >
          <Button type={showSnippet ? 'default' : 'primary'}>
            {t`Open web analytics settings`}
          </Button>
        </Link>
        {onDismiss ? <Button type="text" onClick={onDismiss}>{t`View anyway`}</Button> : null}
      </div>
    </div>
  )
}

export interface WebAnalyticsGateProps {
  /**
   * `data` also gates on incoming traffic; `config` only on the feature
   * existing, so an attribution rule can still be written before the snippet
   * goes live.
   */
  mode?: 'data' | 'config'
  children: ReactNode
}

/**
 * Fronts a web analytics view with an explanation when it has nothing to show:
 * the feature is off, or no visit has reached it in a day.
 *
 * A traffic problem leaves the view behind the card, blurred and dismissible —
 * a workspace that stopped recording yesterday still owns its history, and
 * locking it away to advertise a snippet would be the wrong trade.
 */
export function WebAnalyticsGate({ mode = 'data', children }: WebAnalyticsGateProps) {
  const { workspaceId } = useWebAnalytics()
  const state = useInstallStatus({ checkTraffic: mode === 'data' })
  const [dismissed, setDismissed] = useState(() => readDismissed(workspaceId))

  useEffect(() => {
    setDismissed(readDismissed(workspaceId))
  }, [workspaceId])

  const dismiss = useCallback(() => {
    writeDismissed(workspaceId)
    setDismissed(true)
  }, [workspaceId])

  const trafficProblem = TRAFFIC_STATES.includes(state)

  if (state === 'ok' || state === 'loading') return <>{children}</>
  if (trafficProblem && dismissed) return <>{children}</>

  // A workspace with the feature off has no report underneath to preview, so
  // the card takes the space rather than blurring an empty grid.
  if (!trafficProblem) {
    return (
      <div className="flex justify-center py-6">
        <InstallCard state={state} />
      </div>
    )
  }

  return (
    <div className="relative">
      {/* Capped: past the card the page would otherwise scroll on into
          content nothing is covering. */}
      {/* aria-hidden and pointer-events-none stay as the floor for browsers
          without inert; inert is what actually removes the tab stops. */}
      <div
        aria-hidden="true"
        {...INERT}
        className="pointer-events-none max-h-[70vh] select-none overflow-hidden blur-[3px]"
      >
        {children}
      </div>
      <div className="absolute inset-0 z-10 flex items-start justify-center bg-white/60 px-4 py-10">
        <InstallCard state={state} onDismiss={dismiss} />
      </div>
    </div>
  )
}
