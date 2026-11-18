import { Alert, Button } from 'antd'
import { Check } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { Panel, Pill } from '../layout'
import type { PanelSummary } from './setupHooks'

export interface SetupRailProps {
  summaries: PanelSummary[]
  /** The server's answer to a refused submit, kept until the next one. */
  error?: string
  submitting: boolean
  onGoTo: (summary: PanelSummary) => void
}

/** Beside the panels: what is left to fill, and the button that completes the setup. */
export function SetupRail({ summaries, error, submitting, onGoTo }: SetupRailProps) {
  const { t } = useLingui()
  return (
    <div className="flex flex-col gap-3">
      {error ? <Alert type="error" showIcon title={t`Setup was not saved`} description={error} /> : null}
      <Panel
        title={t`Before you finish`}
        footer={
          <div className="flex w-full flex-col gap-1.5 py-1">
            <Button
              type="primary"
              htmlType="submit"
              block
              loading={submitting}
              icon={<Check size={15} aria-hidden="true" />}
            >
              {submitting ? t`Setting up…` : t`Complete setup`}
            </Button>
            <span className="text-center">{t`The server restarts to apply the configuration.`}</span>
          </div>
        }
      >
        <ul className="m-0 flex list-none flex-col p-0">
          {summaries.map((summary) => (
            <li key={summary.id}>
              <button
                type="button"
                onClick={() => onGoTo(summary)}
                className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md border-0 bg-transparent px-1.5 py-1.5 text-left text-sm text-ink-2 hover:bg-fill"
              >
                <span className="truncate">{summary.title}</span>
                <Pill tone={summary.tone}>{summary.label}</Pill>
              </button>
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  )
}
