import type { ReactNode } from 'react'
import { Check } from 'lucide-react'

export interface StepTrailStep {
  key: string
  label: ReactNode
}

export interface StepTrailProps {
  steps: StepTrailStep[]
  /** Index of the step in progress; the ones before it are done. */
  current: number
  /** Every step done, the last one included (a finished import). */
  complete?: boolean
  /** The list's accessible name ("Import steps"). */
  ariaLabel: string
}

/**
 * The numbered steps of a flow, in a full drawer's bar: done ones carry a check, the current one
 * is filled. A trail to read, not tabs: the flow moves the user from one step to the next.
 */
export function StepTrail({ steps, current, complete, ariaLabel }: StepTrailProps) {
  return (
    <ol data-unreset aria-label={ariaLabel} className="m-0 flex list-none items-center gap-2 p-0">
      {steps.map((step, index) => {
        const done = complete || index < current
        const isCurrent = !complete && index === current
        return (
          <li key={step.key} className="flex items-center gap-2" aria-current={isCurrent ? 'step' : undefined}>
            {index > 0 ? <span aria-hidden="true" className="h-px w-7 bg-line" /> : null}
            <span
              data-step-state={done ? 'done' : isCurrent ? 'current' : 'todo'}
              className={`flex items-center gap-1.5 text-sm whitespace-nowrap ${
                isCurrent ? 'font-medium text-ink' : done ? 'text-ink-2' : 'text-ink-3'
              }`}
            >
              <span
                aria-hidden="true"
                className={`flex size-4.5 items-center justify-center rounded-full text-2xs font-semibold ${
                  isCurrent
                    ? 'bg-primary-500 text-surface'
                    : done
                      ? 'bg-primary-100 text-primary-700'
                      : 'bg-fill text-ink-2'
                }`}
              >
                {done ? <Check size={11} strokeWidth={3} /> : index + 1}
              </span>
              {step.label}
            </span>
          </li>
        )
      })}
    </ol>
  )
}
