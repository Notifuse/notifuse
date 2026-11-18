import type { ReactNode } from 'react'
import { Alert, Button, Spin } from 'antd'
import { ArrowRight, Check, Minus } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../layout'

export type RestartPhase = 'waiting' | 'ready' | 'timeout'

export interface SetupRestartViewProps {
  phase: RestartPhase
  /** Seconds spent waiting for the server, shown while it restarts. */
  elapsed: number
  /** The first root email, to say who signs in. */
  rootEmail?: string
  onSignIn: () => void
  onReload: () => void
}

function Step({ state, children }: { state: 'done' | 'active' | 'todo'; children: ReactNode }) {
  const icon =
    state === 'done' ? (
      <span className="flex size-4 items-center justify-center rounded-full bg-success-bg text-success-fg">
        <Check size={10} aria-hidden="true" />
      </span>
    ) : state === 'active' ? (
      <Spin size="small" />
    ) : (
      <span className="flex size-4 items-center justify-center rounded-full bg-fill text-ink-4">
        <Minus size={10} aria-hidden="true" />
      </span>
    )
  return (
    <li className="flex items-center gap-2 text-sm text-ink-2">
      {icon}
      {children}
    </li>
  )
}

/** After « Complete setup »: the server restarts, then sign-in. Replaces the toasts of the old page. */
export function SetupRestartView({ phase, elapsed, rootEmail, onSignIn, onReload }: SetupRestartViewProps) {
  const { t } = useLingui()
  return (
    <div className="mx-auto w-full max-w-[480px]">
      <Panel>
        <div className="flex flex-col items-center gap-1 px-2 py-4 text-center" aria-live="polite">
          {phase === 'ready' ? (
            <>
              <span className="mb-2 flex size-11 items-center justify-center rounded-full bg-success-bg text-success-fg">
                <Check size={22} aria-hidden="true" />
              </span>
              <h2 data-unreset className="m-0 text-subtitle font-semibold text-ink">{t`Notifuse is ready`}</h2>
              <p data-unreset className="m-0 text-ink-3">
                {rootEmail ? t`Sign in with ${rootEmail}.` : t`You can now sign in.`}
              </p>
            </>
          ) : (
            <>
              {phase === 'waiting' ? (
                <span className="mb-2">
                  <Spin />
                </span>
              ) : null}
              <h2
                data-unreset
                className="m-0 text-subtitle font-semibold text-ink"
              >{t`Applying your configuration`}</h2>
              <p
                data-unreset
                className="m-0 text-ink-3"
              >{t`The server restarts with the new settings. This usually takes a few seconds.`}</p>
              <ul className="m-0 mt-4 flex w-full list-none flex-col gap-1.5 p-0 text-left">
                <Step state="done">{t`Configuration saved`}</Step>
                <Step state={phase === 'waiting' ? 'active' : 'todo'}>
                  <span className="flex-1">{t`Waiting for the server to come back`}</span>
                  <span className="text-xs text-ink-4">{t`${elapsed} s`}</span>
                </Step>
                <Step state="todo">{t`Ready to sign in`}</Step>
              </ul>
            </>
          )}
          {phase === 'timeout' ? (
            <div className="mt-4 w-full text-left">
              <Alert
                type="warning"
                showIcon
                title={t`The server has not come back after 60 seconds.`}
                description={t`Check the server logs, then reload this page. Your configuration is already saved.`}
              />
            </div>
          ) : null}
          <div className="mt-4 w-full">
            {phase === 'timeout' ? (
              <Button block onClick={onReload}>
                {t`Reload`}
              </Button>
            ) : (
              <Button
                type="primary"
                block
                disabled={phase !== 'ready'}
                onClick={onSignIn}
                icon={<ArrowRight size={15} aria-hidden="true" />}
                iconPlacement="end"
              >
                {t`Go to sign in`}
              </Button>
            )}
          </div>
        </div>
      </Panel>
    </div>
  )
}
