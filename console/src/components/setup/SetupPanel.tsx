import type { ReactNode } from 'react'
import { Lock } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { HelpTip, Panel, Pill } from '../layout'
import type { SetupStatus } from '../../types/setup'
import { lockedEnvNames, setupPanelDomId, type SetupPanelId } from './setupFields'
import type { SetupPanelState } from './setupHooks'

export interface SetupPanelProps {
  id: SetupPanelId
  number: number
  title: string
  /** The long explanation behind the title's "?". */
  help?: { label: string; content: ReactNode }
  state?: SetupPanelState
  /** Right of the header, after the state: the switch of an optional panel. */
  extra?: ReactNode
  footer?: ReactNode
  children?: ReactNode
}

/** One numbered section of the setup page. */
export function SetupPanel({ id, number, title, help, state, extra, footer, children }: SetupPanelProps) {
  const badgeTone = state?.tone === 'success' ? 'bg-success-bg text-success-fg' : 'bg-primary-100 text-primary-700'
  return (
    <div id={setupPanelDomId(id)} className="scroll-mt-4">
      <Panel
        icon={
          <span
            aria-hidden="true"
            className={`flex size-5 items-center justify-center rounded-full text-2xs font-bold ${badgeTone}`}
          >
            {number}
          </span>
        }
        title={
          help ? (
            <HelpTip title={title} label={help.label}>
              {help.content}
            </HelpTip>
          ) : (
            title
          )
        }
        extra={
          state || extra ? (
            <>
              {state ? (
                <span className="hidden sm:inline-flex">
                  <Pill tone={state.tone}>{state.label}</Pill>
                </span>
              ) : null}
              {extra}
            </>
          ) : undefined
        }
        footer={footer}
      >
        {children}
      </Panel>
    </div>
  )
}

/** The body of a panel whose whole group the environment sets: the variable names, never their values. */
export function EnvLockedBody({ status, panel }: { status: SetupStatus; panel: SetupPanelId }) {
  const { t } = useLingui()
  const names = lockedEnvNames(status, panel)
  return (
    <div className="flex flex-col gap-2">
      <ul className="m-0 flex list-none flex-wrap gap-1.5 p-0" aria-label={t`Environment variables`}>
        {names.map((name) => (
          <li key={name} className="rounded-sm bg-fill px-1.5 py-0.5 font-mono text-2xs text-ink-2">
            {name}
          </li>
        ))}
      </ul>
      <p
        data-unreset
        className="m-0 text-xs text-ink-3"
      >{t`Edit the environment and restart the server to change these values.`}</p>
    </div>
  )
}

/** Shown in place of a field the environment sets. */
export function LockedFieldNote({ envName }: { envName: string }) {
  const { t } = useLingui()
  return (
    <span className="inline-flex items-center gap-1 text-xs text-ink-3">
      <Lock size={12} aria-hidden="true" />
      {t`Set by ${envName}`}
    </span>
  )
}
