import { useState } from 'react'
import { Button, InputNumber, Popover, Tag } from 'antd'
import { Trans, useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'

/**
 * Session floor for the report.
 *
 * A drill-down grows a long tail of one-session rows that say nothing and bury
 * the rows that do, so every level of the table drops anything below this
 * threshold. The value is applied on confirm rather than on every keystroke:
 * it lives in the URL and each change refetches every open level.
 */
export function MinSessionsInput() {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(context.minSessions)

  const apply = () => {
    context.setMinSessions(draft)
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      trigger="click"
      placement="bottom"
      title={t`Minimum sessions`}
      onOpenChange={(next) => {
        if (next) setDraft(context.minSessions)
        setOpen(next)
      }}
      content={
        <div className="w-48">
          <InputNumber
            autoFocus
            min={1}
            max={100000}
            className="mb-2 w-full"
            value={draft}
            onChange={(value) => setDraft(value ?? 1)}
            onPressEnter={apply}
          />
          <Button type="primary" block onClick={apply}>
            {t`Apply`}
          </Button>
        </div>
      }
    >
      <Button type="text" size="small">
        <span className="text-gray-500">
          <Trans>
            Having at least{' '}
            <Tag color="purple" className="!mx-1">
              {context.minSessions}
            </Tag>{' '}
            sessions
          </Trans>
        </span>
      </Button>
    </Popover>
  )
}
