import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render } from '@testing-library/react'
import { MetricChart } from './MetricChart'
import type { Annotation } from '../../services/api/annotation'
import type { ChartDataPoint, MetricConfig } from './lib/types'

/** The shapes these tests read back off the captured echarts option. */
interface MarkLineItem {
  xAxis: number
  name: string
  lineStyle: { color: string; type: string; width: number }
  label: { position: string; formatter: string; rich: Record<string, { color?: string }> }
}

interface CapturedSeries {
  name: string
  markLine?: { data: MarkLineItem[] }
}

interface TooltipParam {
  axisValue: string
  value: number
  seriesName: string
  dataIndex: number
}

interface CapturedOption {
  grid: { top: string }
  series: CapturedSeries[]
  tooltip: {
    formatter: (params: TooltipParam[]) => string
    confine?: boolean
    extraCssText?: string
  }
}

// services/api/client imports the router, which imports every page and cycles
// back here; the annotations fetch is never exercised by these tests.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))

// The shared setup mock hands out a NEW `t` on every render, which lands in the
// chart's useMemo deps and would recompute the option every time — hiding the
// very regression "annotations arriving after the first paint" pins.
vi.mock('@lingui/react/macro', () => {
  const lingui = {
    t: (strings: TemplateStringsArray, ...values: unknown[]) =>
      strings.reduce((out, part, index) => out + part + (values[index] ?? ''), '')
  }
  return { useLingui: () => lingui }
})

const { captured } = vi.hoisted(() => ({
  captured: { option: null as CapturedOption | null }
}))

vi.mock('echarts-for-react', () => ({
  default: (props: { option: unknown }) => {
    captured.option = props.option as CapturedOption
    return <div data-testid="chart" />
  }
}))

const option = (): CapturedOption => {
  if (!captured.option) throw new Error('the chart rendered no option')
  return captured.option
}

const markLine = (): MarkLineItem[] | undefined => option().series[0].markLine?.data

const METRIC: MetricConfig = { key: 'sessions', label: 'Sessions', format: 'number', color: '#7763f1' }

/**
 * Buckets as the engine serialises them: wall clocks in the query timezone,
 * stamped with a "Z" they did not earn.
 */
function buckets(start: string, count: number, hourly = false): ChartDataPoint[] {
  return Array.from({ length: count }, (_, index) => {
    const at = new Date(Date.parse(start) + index * (hourly ? 3_600_000 : 86_400_000))
    return { timestamp: `${at.toISOString().slice(0, 19)}Z`, value: index }
  })
}

/**
 * Referentially stable across renders. The chart's memo only recomputes when a
 * dep changes, so a fixture rebuilt per render would make every dep-array test
 * pass whether or not the new props are declared.
 */
const DAILY = buckets('2026-08-13T00:00:00Z', 3)
const HOURLY = buckets('2026-08-14T00:00:00Z', 48, true)
const NO_PREVIOUS: ChartDataPoint[] = []

function annotation(overrides: Partial<Annotation> = {}): Annotation {
  return {
    id: 'ann1',
    annotated_at: '2026-08-14T09:00:00Z',
    timezone: 'UTC',
    title: 'Product launch',
    color: '#ef4444',
    source: 'manual',
    created_at: '2026-08-14T09:00:00Z',
    updated_at: '2026-08-14T09:00:00Z',
    ...overrides
  }
}

function renderChart(props: Partial<React.ComponentProps<typeof MetricChart>> = {}) {
  return render(
    <MetricChart
      metric={METRIC}
      current={DAILY}
      previous={NO_PREVIOUS}
      granularity="day"
      currentLabel="Aug 13-15"
      timezone="UTC"
      {...props}
    />
  )
}

describe('MetricChart annotations', () => {
  beforeEach(() => {
    captured.option = null
  })

  it('draws no markLine and keeps the tight grid without annotations', () => {
    renderChart()

    expect(markLine()).toBeUndefined()
    expect(option().grid.top).toBe('5%')
  })

  it('draws one dotted vertical per annotated bucket and makes room for the titles', () => {
    renderChart({ annotations: [annotation()] })

    const lines = markLine()
    expect(lines).toHaveLength(1)
    expect(lines?.[0].xAxis).toBe(1)
    expect(lines?.[0].name).toBe('ann1')
    expect(lines?.[0].lineStyle).toMatchObject({ color: '#ef4444', type: 'dotted', width: 1 })
    expect(option().grid.top).toBe('18%')
  })

  it('collapses annotations sharing a bucket onto one vertical counting the rest', () => {
    // Both land in the Aug 14 bucket. Drawn one per annotation, a busy range
    // would stack hundreds of dotted lines and titles on a dozen buckets.
    renderChart({
      annotations: [
        annotation({ id: 'a', title: 'Launch day', annotated_at: '2026-08-14T09:00:00Z' }),
        annotation({ id: 'b', title: 'Outage', annotated_at: '2026-08-14T18:00:00Z' })
      ]
    })

    const lines = markLine()
    expect(lines).toHaveLength(1)
    expect(lines?.[0].xAxis).toBe(1)
    expect(lines?.[0].label.formatter).toBe('{dot|●}{title|Launch day +1}')

    // The collapsed ones are not lost: the tooltip still lists every title.
    const html = option().tooltip.formatter([
      { axisValue: 'Aug 14', value: 5, seriesName: 'Aug 13-15', dataIndex: 1 }
    ])
    expect(html).toContain('Launch day')
    expect(html).toContain('Outage')
  })

  it('bounds the tooltip so a long description wraps instead of running off the chart', () => {
    // A description is free text up to 500 characters and the tooltip box sizes
    // to max-content, so without a width cap it renders as one unbroken line
    // wider than the viewport, spilling off both edges.
    renderChart({
      annotations: [
        annotation({
          id: 'a',
          title: 'Launch day',
          description: 'x'.repeat(500),
          annotated_at: '2026-08-14T09:00:00Z'
        })
      ]
    })

    const { confine, extraCssText } = option().tooltip
    expect(confine).toBe(true)
    expect(extraCssText).toMatch(/max-width:\s*\d+px/)
    expect(extraCssText).toMatch(/white-space:\s*normal/)

    // The full description still reaches the tooltip — it is bounded, not truncated.
    const html = option().tooltip.formatter([
      { axisValue: 'Aug 14', value: 5, seriesName: 'Aug 13-15', dataIndex: 1 }
    ])
    expect(html).toContain('x'.repeat(500))
  })

  it('keeps one vertical per bucket, in x-order, when the buckets differ', () => {
    // Passed newest-first, as the endpoint returns them, so this also pins the
    // left-to-right ordering the alternating label side depends on.
    renderChart({
      annotations: [
        annotation({ id: 'b', title: 'Outage', annotated_at: '2026-08-15T18:00:00Z' }),
        annotation({ id: 'a', title: 'Launch day', annotated_at: '2026-08-14T09:00:00Z' })
      ]
    })

    const lines = markLine()
    expect(lines).toHaveLength(2)
    expect(lines?.map((line) => line.xAxis)).toEqual([1, 2])
    expect(lines?.map((line) => line.label.formatter)).toEqual([
      '{dot|●}{title|Launch day}',
      '{dot|●}{title|Outage}'
    ])
  })

  it('places annotations by bucket index, so repeated hour labels stay apart', () => {
    // Both sit at 03:00, which formatXAxisLabel renders as "3am" for either.
    // Keying by that label would collapse them onto the same, last, bucket.
    renderChart({
      current: HOURLY,
      granularity: 'hour',
      annotations: [
        annotation({ id: 'a', annotated_at: '2026-08-14T03:00:00Z' }),
        annotation({ id: 'b', annotated_at: '2026-08-15T03:00:00Z' })
      ]
    })

    expect(markLine()?.map((line) => line.xAxis)).toEqual([3, 27])
  })

  it('drops an annotation that falls outside the series instead of clamping it', () => {
    renderChart({
      annotations: [annotation({ annotated_at: '2026-07-01T09:00:00Z' })]
    })

    expect(markLine()).toBeUndefined()
    expect(option().grid.top).toBe('5%')
  })

  it('leaves the comparison series unmarked', () => {
    // It covers a different period, so a date line on it would mark a moment
    // that series does not contain.
    renderChart({
      previous: DAILY,
      previousLabel: 'Aug 10-12',
      annotations: [annotation()]
    })

    const series = option().series
    expect(series).toHaveLength(2)
    expect(series[0].markLine).toBeDefined()
    expect(series[1].markLine).toBeUndefined()
  })

  it('alternates the label side so neighbouring titles do not overprint', () => {
    renderChart({
      annotations: [
        annotation({ id: 'a', annotated_at: '2026-08-13T09:00:00Z' }),
        annotation({ id: 'b', annotated_at: '2026-08-14T09:00:00Z' }),
        annotation({ id: 'c', annotated_at: '2026-08-15T09:00:00Z' })
      ]
    })

    expect(markLine()?.map((line) => line.label.position)).toEqual([
      'insideEndTop',
      'insideEndBottom',
      'insideEndTop'
    ])
  })

  it('neutralises rich-text markup in the label and truncates the title', () => {
    renderChart({
      annotations: [annotation({ title: 'Summer {promo|v2} clearance kickoff' })]
    })

    const label = markLine()?.[0].label
    // 20 characters of label budget, the ellipsis included.
    expect(label?.formatter).toBe('{dot|●}{title|Summer promo v2 cle…}')
    expect(label?.rich.dot.color).toBe('#ef4444')
  })

  it('escapes annotation text on its way into the tooltip HTML', () => {
    renderChart({
      annotations: [
        annotation({
          title: '<img src=x onerror=alert(1)>',
          description: '<script>alert(2)</script>'
        })
      ]
    })

    const html = option().tooltip.formatter([
      { axisValue: 'Aug 14', value: 5, seriesName: 'Aug 13-15', dataIndex: 1 }
    ])

    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;')
    expect(html).toContain('&lt;script&gt;alert(2)&lt;/script&gt;')
    expect(html).not.toContain('<img')
    expect(html).not.toContain('<script')
  })

  it('lists only the annotations of the hovered bucket', () => {
    renderChart({
      annotations: [
        annotation({ id: 'a', title: 'Launch day', annotated_at: '2026-08-14T09:00:00Z' }),
        annotation({ id: 'b', title: 'Outage', annotated_at: '2026-08-15T09:00:00Z' })
      ]
    })

    const formatter = option().tooltip.formatter
    const params = (dataIndex: number): TooltipParam[] => [
      { axisValue: 'Aug', value: 5, seriesName: 'Aug 13-15', dataIndex }
    ]

    expect(formatter(params(1))).toContain('Launch day')
    expect(formatter(params(1))).not.toContain('Outage')
    expect(formatter(params(2))).toContain('Outage')
    expect(formatter(params(0))).not.toContain('Launch day')
  })

  it('renders annotations that arrive after the first paint', () => {
    // Every other prop is referentially identical across the two renders, so
    // this fails unless `annotations` is declared in the option memo's deps.
    const { rerender } = renderChart()
    expect(markLine()).toBeUndefined()

    rerender(
      <MetricChart
        metric={METRIC}
        current={DAILY}
        previous={NO_PREVIOUS}
        granularity="day"
        currentLabel="Aug 13-15"
        timezone="UTC"
        annotations={[annotation()]}
      />
    )

    expect(markLine()).toHaveLength(1)
  })

  it('re-places annotations when the query timezone changes', () => {
    // Buckets are wall clocks: midnight on Aug 15 in New York is 04:00Z, so an
    // annotation at 02:00Z on the 15th belongs to the Aug 14 bucket there.
    // Same failure mode as above — only `timezone` differs between renders.
    const annotations = [annotation({ annotated_at: '2026-08-15T02:00:00Z' })]

    const { rerender } = renderChart({ annotations })
    expect(markLine()?.[0].xAxis).toBe(2)

    rerender(
      <MetricChart
        metric={METRIC}
        current={DAILY}
        previous={NO_PREVIOUS}
        granularity="day"
        currentLabel="Aug 13-15"
        timezone="America/New_York"
        annotations={annotations}
      />
    )

    expect(markLine()?.[0].xAxis).toBe(1)
  })
})
