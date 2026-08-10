import { useState } from 'react'
import { Button, Card, Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import { BreakdownModal } from './BreakdownModal'
import { getDimensionLabel } from '../lib/dimensions'
import { WebDimensionFilter } from '../lib/types'

interface ExploreTemplate {
  key: string
  title: string
  description: string
  dimensions: string[]
  filters?: WebDimensionFilter[]
}

interface ExploreTemplatesProps {
  onSelect: (dimensions: string[], filters?: WebDimensionFilter[]) => void
}

/**
 * The empty state of the explore tab.
 *
 * A report is a specific ordering of dimensions, which is a lot to ask of
 * someone opening the tab for the first time; the templates are the questions
 * people actually arrive with, already expressed as one.
 */
export function ExploreTemplates(props: ExploreTemplatesProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [customOpen, setCustomOpen] = useState(false)

  const templates: ExploreTemplate[] = [
    {
      key: 'custom',
      title: t`Custom report`,
      description: t`Build a report from your own dimensions`,
      dimensions: []
    },
    {
      key: 'not_mapped',
      title: t`Not-mapped channels`,
      description: t`Traffic no filter rule has classified`,
      dimensions: ['referrer_domain', 'utm_source', 'utm_medium', 'utm_campaign'],
      filters: [{ dimension: 'channel', operator: 'equals', values: ['not-mapped'] }]
    },
    {
      key: 'campaigns',
      title: t`UTM campaigns`,
      description: t`Compare TimeScore across campaign sources`,
      dimensions: ['utm_source', 'utm_medium', 'utm_campaign', 'device']
    },
    {
      key: 'channels',
      title: t`Channels`,
      description: t`High-level channel performance`,
      dimensions: ['channel_group', 'channel', 'utm_campaign', 'device']
    },
    {
      key: 'landing',
      title: t`Landing pages`,
      description: t`Content quality by traffic source`,
      dimensions: ['landing_path', 'utm_source', 'device']
    },
    {
      key: 'referrals',
      title: t`Referral traffic`,
      description: t`Understand the quality of referrals`,
      dimensions: ['referrer_domain', 'referrer_path', 'landing_path']
    },
    {
      key: 'devices',
      title: t`Devices and tech`,
      description: t`Technical performance insights`,
      dimensions: ['device', 'browser', 'os', 'connection_type']
    },
    {
      key: 'time',
      title: t`Time patterns`,
      description: t`When engagement is at its best`,
      dimensions: ['day_of_week', 'hour_of_day', 'is_weekend']
    },
    {
      key: 'geography',
      title: t`Geography`,
      description: t`Engagement by country and region`,
      dimensions: ['country', 'region', 'city', 'timezone']
    }
  ]

  const open = (template: ExploreTemplate) => {
    if (template.key === 'custom') {
      setCustomOpen(true)
      return
    }
    props.onSelect(template.dimensions, template.filters)
  }

  return (
    <>
      <p className="mb-6 text-gray-500">
        {t`Pick a report to start from, or build your own.`}
      </p>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        {templates.map((template) => (
          <Card
            key={template.key}
            className="cursor-pointer transition-shadow hover:shadow-md"
            onClick={() => open(template)}
            styles={{
              body: { padding: 16, display: 'flex', flexDirection: 'column', height: '100%' }
            }}
          >
            <div className="flex-1">
              <h3 className="m-0 mb-2 text-sm font-semibold">{template.title}</h3>
              <p className="mb-3 text-sm text-gray-500">{template.description}</p>
              {template.dimensions.length > 0 ? (
                <div className="mb-3 flex flex-col items-start gap-1">
                  {template.dimensions.map((dimension) => (
                    <Tag key={dimension} color="blue" variant="filled" className="!mr-0">
                      {getDimensionLabel(dimension, context.customDimensionLabels)}
                    </Tag>
                  ))}
                </div>
              ) : null}
              {template.filters?.length ? (
                <div className="mb-3">
                  <div className="mb-1 text-xs text-gray-400">{t`With:`}</div>
                  {template.filters.map((filter, index) => (
                    <Tag
                      key={`${filter.dimension}-${index}`}
                      color="orange"
                      variant="filled"
                      className="!mr-0"
                    >
                      {getDimensionLabel(filter.dimension, context.customDimensionLabels)}{' '}
                      {filter.values.join(', ')}
                    </Tag>
                  ))}
                </div>
              ) : null}
            </div>
            <Button
              type="primary"
              ghost
              block
              onClick={(event) => {
                event.stopPropagation()
                open(template)
              }}
            >
              {template.key === 'custom' ? t`Create my own` : t`Select`}
            </Button>
          </Card>
        ))}
      </div>

      <BreakdownModal
        open={customOpen}
        title={t`Create a custom report`}
        submitText={t`Generate report`}
        onCancel={() => setCustomOpen(false)}
        onSubmit={(dimensions) => {
          setCustomOpen(false)
          props.onSelect(dimensions)
        }}
      />
    </>
  )
}
