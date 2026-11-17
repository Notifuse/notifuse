import type { ReactNode } from 'react'
import { ChartColumn } from 'lucide-react'

interface ChartGhostProps {
  message: ReactNode
  /** One default button or link, under the message. */
  action?: ReactNode
}

// Fixed heights (percent of the chart), so the ghost looks the same on every visit and never reads
// as data.
const BARS = [22, 34, 28, 46, 38, 30, 52, 44, 36, 58, 48, 40, 62, 54, 42, 66, 50, 44, 60, 70, 56, 48, 64, 74, 58, 52, 68, 62, 54, 72]

/** The chart before there is anything to draw: grey bars behind what will fill it. */
export function ChartGhost({ message, action }: ChartGhostProps) {
  return (
    <div data-testid="chart-ghost" className="relative h-[250px]">
      <div aria-hidden="true" className="absolute inset-x-0 bottom-0 flex h-full items-end gap-1 border-b border-line">
        {BARS.map((height, index) => (
          <span
            key={index}
            className={`flex-1 rounded-t-xs ${index % 2 === 0 ? 'bg-fill' : 'bg-line-soft'}`}
            style={{ height: `${height}%` }}
          />
        ))}
      </div>
      <div className="absolute inset-0 flex items-center justify-center p-3">
        <div className="flex max-w-[400px] flex-col items-center gap-1.5 rounded-lg bg-surface/90 px-5 py-4 text-center">
          <span aria-hidden="true" className="flex size-8 items-center justify-center rounded-lg bg-primary-100 text-primary-600">
            <ChartColumn size={16} />
          </span>
          <span className="text-sm text-ink-2">{message}</span>
          {action ? <div className="mt-1">{action}</div> : null}
        </div>
      </div>
    </div>
  )
}
