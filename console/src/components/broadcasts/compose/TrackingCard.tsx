import { Form, Input, Switch } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Globe } from 'lucide-react'
import { Panel } from '../../layout'
import { TRACKING_DOCS_URL } from '../../../lib/emailTracking'
import type { ComposeValues } from './composeValues'
import type { ComposeUpdate } from './types'

export interface TrackingCardProps {
  values: ComposeValues
  onChange: ComposeUpdate
}

/** UTM parameters and the open and click tracking of the broadcast. */
export function TrackingCard({ values, onChange }: TrackingCardProps) {
  const { t } = useLingui()
  return (
    <Panel title={t`Web tracking`} icon={<Globe size={14} />}>
      <Form layout="vertical" component="div">
        <div className="grid grid-cols-3 gap-3">
          <Form.Item label="utm_source">
            <Input
              aria-label="utm_source"
              placeholder={t`Your website or company name`}
              value={values.utmSource ?? ''}
              onChange={(e) => onChange((v) => ({ ...v, utmSource: e.target.value }))}
            />
          </Form.Item>
          <Form.Item label="utm_medium">
            <Input
              aria-label="utm_medium"
              placeholder="email"
              value={values.utmMedium ?? ''}
              onChange={(e) => onChange((v) => ({ ...v, utmMedium: e.target.value }))}
            />
          </Form.Item>
          <Form.Item label="utm_campaign" extra={values.utmCampaignEdited ? undefined : t`Generated from the name until you edit it`}>
            <Input
              aria-label="utm_campaign"
              value={values.utmCampaign ?? ''}
              onChange={(e) => onChange((v) => ({ ...v, utmCampaign: e.target.value, utmCampaignEdited: true }))}
            />
          </Form.Item>
        </div>
        <Form.Item
          label={t`Open and click tracking`}
          extra={
            <>
              {t`No recipient gets the open pixel or tracked links; UTM parameters are kept. Otherwise each contact's tracking consent applies.`}{' '}
              <a href={TRACKING_DOCS_URL} target="_blank" rel="noopener noreferrer">
                {t`Learn more`}
              </a>
            </>
          }
        >
          <span className="inline-flex items-center gap-2 text-sm text-ink">
            <Switch
              aria-label={t`Don't track opens and clicks for this broadcast`}
              checked={values.noTracking}
              // The server refuses an untracked broadcast with an automatic winner: the winner is
              // chosen on opens or clicks.
              onChange={(noTracking) => onChange((v) => ({ ...v, noTracking, autoSendWinner: noTracking ? false : v.autoSendWinner }))}
            />
            {t`Don't track opens and clicks for this broadcast`}
          </span>
        </Form.Item>
      </Form>
    </Panel>
  )
}
