import { Tag } from 'antd'
import { Filter, X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../useWebAnalytics'
import { describeDimensionFilter, useOperatorLabels } from '../lib/filterLabels'
import { filtersForSchema } from './queries'

/**
 * The page filters the goal dashboard applies, each removable. They are the whole section's
 * filters (a row or a country clicked in the drawer adds one), which the line says.
 */
export function GoalFiltersLine() {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const operatorLabels = useOperatorLabels()
  // The goal viewed replaces a filter on the goal name: it is not one the drawer applies.
  const applied = filtersForSchema(context.filters, 'web_goals').filter((filter) => filter.dimension !== 'goal_name')

  return (
    <div data-goal-filters className="flex flex-wrap items-center gap-x-2 gap-y-1.5 text-sm text-ink-3">
      {applied.length === 0 ? (
        <span className="inline-flex items-start gap-2">
          <Filter size={13} aria-hidden="true" className="mt-0.5 shrink-0" />
          <span>{t`Click a row or a country to filter the whole Web Analytics section.`}</span>
        </span>
      ) : (
        <>
          <span className="inline-flex items-center gap-2">
            <Filter size={13} aria-hidden="true" />
            {t`Filtered by`}
          </span>
          {applied.map((filter) => {
            const label = describeDimensionFilter(filter, operatorLabels, context.customDimensionLabels)
            return (
              <Tag
                key={label}
                color="processing"
                closable
                closeIcon={<X size={12} aria-label={t`Remove ${label}`} />}
                onClose={(event) => {
                  event.preventDefault()
                  context.setFilters(context.filters.filter((candidate) => candidate !== filter))
                }}
              >
                {label}
              </Tag>
            )
          })}
          <span>{t`· applies to the whole Web Analytics section. Click a row or a country to add one.`}</span>
        </>
      )}
    </div>
  )
}
