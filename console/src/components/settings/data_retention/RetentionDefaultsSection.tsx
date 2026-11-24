import { useMemo, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Skeleton, Typography } from 'antd'
import {
  dataRetentionApi,
  type RetentionCategory,
  type RetentionDays,
  type RetentionDefaultsView
} from '../../../services/api/dataRetention'
import { CATEGORY_LABELS } from './categoryText'
import { fromDays } from './periods'
import { PeriodInput } from './PeriodInput'
import { useLicense } from '../../../hooks/useLicense'
import { LicenceGateNotice } from '../../license/LicenceGateNotice'

const { Title, Text } = Typography

// Server values, in the order the run processes them (the API lists them in that order).
const CATEGORIES: RetentionCategory[] = [
  'inactive_contacts',
  'message_history',
  'contact_timeline',
  'custom_events',
  'automation_history',
  'web_analytics',
  'inbound_webhook_events',
  'webhook_deliveries',
  'failed_emails'
]

const DEFAULTS_KEY = ['data-retention-defaults'] as const

/**
 * The deployment's default periods (root), in the system settings drawer. Saved on its own
 * route, without the restart the rest of the drawer needs.
 */
export function RetentionDefaultsSection() {
  const { t } = useLingui()
  const { data, isLoading, error } = useQuery({ queryKey: DEFAULTS_KEY, queryFn: () => dataRetentionApi.getDefaults() })
  return (
    <section aria-labelledby="retention-defaults-title" className="flex flex-col gap-3">
      <div id="retention-defaults-title">
        <Title level={5} style={{ margin: 0 }}>
          {t`Data retention`}
        </Title>
      </div>
      <Text type="secondary" className="text-xs">
        {t`Default periods for every workspace. A workspace that sets its own period keeps it. A default that deletes more waits 7 days, and the owners of the workspaces it reaches get an email.`}
      </Text>
      {isLoading ? (
        <Skeleton active />
      ) : error || !data ? (
        <Alert type="error" showIcon message={t`Could not load the data retention defaults`} />
      ) : (
        <DefaultsForm key={JSON.stringify(data.stored)} view={data} />
      )}
    </section>
  )
}

function DefaultsForm({ view }: { view: RetentionDefaultsView }) {
  const { t, i18n } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  // What root last asked for: the waiting change if any, else the defaults in force.
  const initial = useMemo<Record<RetentionCategory, number>>(() => {
    const out = { ...view.defaults }
    for (const [category, days] of Object.entries(view.stored?.pending?.categories ?? {}) as [RetentionCategory, number][]) {
      out[category] = days
    }
    return out
  }, [view])
  const [draft, setDraft] = useState(initial)
  const dirty = CATEGORIES.some((c) => draft[c] !== initial[c])
  const { has } = useLicense()
  const locked = !has('data_retention')

  const save = useMutation({
    mutationFn: (categories: RetentionDays) => dataRetentionApi.setDefaults(categories),
    onSuccess: async (response) => {
      message.success(
        response.stored.pending
          ? t`Saved. The defaults that delete more take effect in 7 days.`
          : t`Data retention defaults saved`
      )
      await queryClient.invalidateQueries({ queryKey: DEFAULTS_KEY })
    },
    onError: (err: Error) => message.error(err.message)
  })

  const formatPeriod = (days: number) => {
    if (days === 0) return t`Off`
    const { value, unit } = fromDays(days)
    if (unit === 'years') return plural(value, { one: '# year', other: '# years' })
    if (unit === 'months') return plural(value, { one: '# month', other: '# months' })
    return plural(value, { one: '# day', other: '# days' })
  }
  const specs = view.categories ?? []
  const minimum = (category: RetentionCategory) => specs.find((spec) => spec.category === category)?.min_days ?? 0
  const suggested = (category: RetentionCategory) => specs.find((spec) => spec.category === category)?.suggested_days ?? 365
  const tooShort = CATEGORIES.some((c) => draft[c] > 0 && draft[c] < minimum(c))
  const pending = view.stored?.pending
  const waiting = pending
    ? CATEGORIES.filter((c) => (pending.categories[c] ?? view.defaults[c]) !== view.defaults[c]).map(
        (c) => `${i18n._(CATEGORY_LABELS[c])}: ${formatPeriod(view.defaults[c])} → ${formatPeriod(pending.categories[c] ?? 0)}`
      )
    : []

  return (
    <div className="flex flex-col gap-3">
      <LicenceGateNotice feature="data_retention" className="" />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {CATEGORIES.map((category) => {
          const days = draft[category] ?? 0
          const label = i18n._(CATEGORY_LABELS[category])
          return (
            <div key={category} className="flex flex-col gap-1">
              <Text>{label}</Text>
              <PeriodInput
                days={days}
                onDays={suggested(category)}
                label={label}
                offOnly={locked && (specs.find((spec) => spec.category === category)?.default_days ?? 0) === 0}
                onChange={(next) => setDraft((d) => ({ ...d, [category]: next }))}
              />
              {days > 0 && days < minimum(category) ? (
                <Text type="danger" className="text-xs">
                  {t`At least ${formatPeriod(minimum(category))}`}
                </Text>
              ) : null}
            </div>
          )
        })}
      </div>
      {pending && waiting.length > 0 ? (
        <Alert
          type="warning"
          showIcon
          message={t`Waiting until ${new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(pending.effective_at))}`}
          description={waiting.join(' · ')}
          action={
            <Button size="small" danger loading={save.isPending} onClick={() => save.mutate(view.defaults)}>
              {t`Cancel this change`}
            </Button>
          }
        />
      ) : null}
      <div className="flex items-center justify-between gap-3">
        <Text type="secondary" className="text-xs">
          {t`Saved on its own: the server does not restart.`}
        </Text>
        <div className="flex gap-2">
          <Button size="small" disabled={!dirty} onClick={() => setDraft(initial)}>
            {t`Discard`}
          </Button>
          <Button size="small" type="primary" disabled={!dirty || tooShort} loading={save.isPending} onClick={() => save.mutate(draft)}>
            {t`Save the defaults`}
          </Button>
        </div>
      </div>
    </div>
  )
}
