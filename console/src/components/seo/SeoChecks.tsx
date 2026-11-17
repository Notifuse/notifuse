import type { ReactNode } from 'react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { Button } from 'antd'
import { CircleAlert, CircleCheck, Info, ListChecks } from 'lucide-react'
import { Panel } from '../layout'

export interface SeoCheck {
  key: string
  ok: boolean
  label: string
  /** What the page uses instead, or what happens without it. */
  why?: string
  /** The form field that settles it, brought into view by « Add ». */
  field?: string[]
  /** `info`: a fact worth knowing rather than something to fix; not counted as to check. */
  tone?: 'info'
}

export interface SeoChecksProps {
  title: ReactNode
  checks: SeoCheck[]
  onShowField: (path: string[]) => void
}

/** A page's checks before it goes out: each one met, or what is missing and what is used instead. */
export function SeoChecks({ title, checks, onShowField }: SeoChecksProps) {
  const { t } = useLingui()
  const toCheck = checks.filter((check) => !check.ok && check.tone !== 'info').length

  return (
    <Panel
      title={title}
      icon={<ListChecks size={14} aria-hidden="true" />}
      extra={
        <span className="text-xs text-ink-3">
          {toCheck === 0 ? t`All set` : plural(toCheck, { one: '# to check', other: '# to check' })}
        </span>
      }
    >
      <ul className="m-0 flex list-none flex-col p-0">
        {checks.map((check) => (
          <li
            key={check.key}
            data-check={check.key}
            data-ok={check.ok}
            data-tone={check.tone}
            className="flex items-start gap-2 border-t border-line-soft py-1.5 first:border-t-0 first:pt-0 last:pb-0"
          >
            <span
              className={`mt-0.5 flex shrink-0 ${
                check.ok ? 'text-success-fg' : check.tone === 'info' ? 'text-info-fg' : 'text-warning-fg'
              }`}
            >
              {check.ok ? (
                <CircleCheck size={14} aria-hidden="true" />
              ) : check.tone === 'info' ? (
                <Info size={14} aria-hidden="true" />
              ) : (
                <CircleAlert size={14} aria-hidden="true" />
              )}
            </span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="text-sm text-ink">{check.label}</span>
              {check.why ? (
                <span className="truncate text-xs text-ink-3" title={check.why}>
                  {check.why}
                </span>
              ) : null}
            </span>
            {!check.ok && check.field ? (
              <Button type="link" size="small" onClick={() => onShowField(check.field!)}>
                {t`Add`}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </Panel>
  )
}

