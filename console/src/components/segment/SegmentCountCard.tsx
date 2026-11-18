import { Button, Popover, Spin, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faTriangleExclamation } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import type { PreviewSegmentResponse, SegmentValueErrorDetails } from '../../services/api/segment'
import { segmentValueErrorText } from './segmentValueError'
import { FillBar } from '../layout/FillBar'

export interface SegmentCountCardProps {
  /** The last count received, if any. */
  response?: PreviewSegmentResponse
  /** The workspace's contacts, to say what share of them the segment holds. */
  totalContacts?: number
  /** A count is on its way (requested, or waiting out the debounce). */
  refreshing: boolean
  /** The count on screen no longer answers what the form says. */
  stale: boolean
  /** Stale with nothing on its way: the condition being written cannot be counted yet. */
  blocked: boolean
  error?: string
  /** The count failed on a contact value the conditions cannot convert: no retry can help. */
  valueError?: SegmentValueErrorDetails
  /** The workspace's custom field labels, to name the field of a value error. */
  customFieldLabels?: Record<string, string>
  /** Whether a count can be asked for now (the tree compiles). */
  canCount: boolean
  onCount: () => void
}

/**
 * How many contacts the segment holds, as the form stands: the number, its share of the
 * workspace, and the query behind it. Its height does not move from one state to the next, so the
 * conditions below it stay put while a condition is written.
 */
export function SegmentCountCard({
  response,
  totalContacts,
  refreshing,
  stale,
  blocked,
  error,
  valueError,
  customFieldLabels,
  canCount,
  onCount
}: SegmentCountCardProps) {
  const { t, i18n } = useLingui()
  const count = response?.total_count
  // The workspace total is cached, the count is fresh: a segment can briefly hold more than it
  const ratio = count !== undefined && totalContacts ? Math.min(1, count / totalContacts) : undefined
  const format = new Intl.NumberFormat(i18n.locale, {
    maximumFractionDigits: 1
  })
  const percent =
    ratio === undefined
      ? undefined
      : ratio > 0 && ratio < 0.001
        ? `< ${format.format(0.1)}`
        : format.format(ratio * 100)
  const total = totalContacts !== undefined ? format.format(totalContacts) : ''

  const query = response ? (
    <div className="flex max-h-[600px] w-[600px] flex-col gap-2 overflow-auto">
      <div>
        <strong>{t`Matching contacts:`}</strong> {response.total_count}
      </div>
      {response.generated_sql && (
        <>
          <strong>{t`Generated SQL:`}</strong>
          <pre className="m-0 max-h-[200px] overflow-auto rounded-md border border-line-soft bg-subtle p-2 font-mono text-2xs whitespace-pre-wrap">
            {response.generated_sql}
          </pre>
        </>
      )}
      {!!response.sql_args?.length && (
        <>
          <strong>{t`SQL Arguments:`}</strong>
          <pre className="m-0 max-h-[100px] overflow-auto rounded-md border border-line-soft bg-subtle p-2 font-mono text-2xs">
            {JSON.stringify(response.sql_args, null, 2)}
          </pre>
        </>
      )}
    </div>
  ) : null

  return (
    <div className="rounded-lg border border-line bg-subtle px-3.5 py-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-overline font-semibold tracking-wide text-ink-3 uppercase">{t`Matching contacts`}</span>
        {query && (
          <Popover title={t`Preview Results`} placement="left" content={query} trigger="click">
            <button type="button" className="cursor-pointer text-xs text-ink-3 hover:text-primary-700">
              {t`See the query`}
            </button>
          </Popover>
        )}
      </div>

      <Spin spinning={refreshing} size="small">
        <div className={`transition-opacity ${stale && !refreshing ? 'opacity-40' : ''}`}>
          <div className="mt-1 text-title font-bold text-ink">
            {count !== undefined ? plural(count, { one: '# contact', other: '# contacts' }) : '—'}
          </div>
          <FillBar ratio={ratio ?? 0} className="my-2 flex w-full" />
        </div>
      </Spin>

      {/* One line, whatever the state, so the card keeps its height */}
      <div className="min-h-5 text-xs text-ink-3">
        {valueError && canCount ? (
          <Tooltip title={segmentValueErrorText(valueError, i18n, customFieldLabels)}>
            <span className="inline-flex items-center gap-1.5 text-warning-fg" data-value-error>
              <FontAwesomeIcon icon={faTriangleExclamation} />
              {t`A contact's value can't be read.`}
            </span>
          </Tooltip>
        ) : error && canCount ? (
          <Tooltip title={error}>
            <span className="inline-flex items-center gap-1.5 text-warning-fg">
              <FontAwesomeIcon icon={faTriangleExclamation} />
              {t`The count failed.`}
              <button type="button" className="cursor-pointer underline" onClick={onCount}>
                {t`Retry`}
              </button>
            </span>
          </Tooltip>
        ) : blocked || error ? (
          t`Complete the condition to refresh the count`
        ) : count === undefined ? (
          <Button size="small" onClick={onCount} disabled={!canCount || refreshing}>
            {t`Preview`}
          </Button>
        ) : percent !== undefined ? (
          plural(totalContacts ?? 0, {
            one: `${percent} % of ${total} contact`,
            other: `${percent} % of ${total} contacts`
          })
        ) : null}
      </div>
    </div>
  )
}
