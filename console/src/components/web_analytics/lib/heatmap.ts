import { CSSProperties } from 'react'
import { TIMESCORE_REFERENCE_SECONDS } from './types'

/**
 * Colour for a TimeScore value.
 *
 * The scale has two halves so that "good" and "exceptional" stay
 * distinguishable at a glance: below the reference it runs white → green, and
 * above it green → cyan. Without the split, one outlier would wash every other
 * row out to near-white.
 */
export function getHeatMapColor(
  value: number,
  bestValue: number,
  referenceValue: number = TIMESCORE_REFERENCE_SECONDS
): string {
  if (!bestValue || value <= 0) return 'transparent'
  const reference = referenceValue || bestValue
  const effectiveMax = Math.max(bestValue, reference)

  if (value <= reference) {
    const lightness = 100 - (value / reference) * 40
    return `hsl(142, 70%, ${lightness}%)`
  }

  const headroom = effectiveMax - reference
  if (headroom <= 0) return 'hsl(180, 70%, 50%)'
  const ratio = Math.min((value - reference) / headroom, 1)
  return `hsl(${142 + ratio * 38}, 70%, ${60 - ratio * 10}%)`
}

export function getHeatMapStyle(
  value: number,
  bestValue: number,
  referenceValue?: number,
  size = 7
): CSSProperties {
  return {
    backgroundColor: getHeatMapColor(value, bestValue, referenceValue),
    width: `${size}px`,
    height: `${size}px`,
    borderRadius: '50%',
    display: 'inline-block',
    flexShrink: 0
  }
}

/** Same ramp, muted, for the day × hour grid where cells sit edge to edge. */
export function getTimescoreCellColor(
  value: number,
  bestValue: number,
  referenceValue: number = TIMESCORE_REFERENCE_SECONDS
): string {
  if (value <= 0) return '#f5f5f5'
  const reference = referenceValue || bestValue
  const effectiveMax = Math.max(bestValue, reference)
  if (value <= reference) {
    return `hsl(142, 50%, ${95 - (value / reference) * 30}%)`
  }
  const headroom = effectiveMax - reference
  if (headroom <= 0) return 'hsl(180, 50%, 55%)'
  const ratio = Math.min((value - reference) / headroom, 1)
  return `hsl(${142 + ratio * 38}, 50%, ${65 - ratio * 10}%)`
}
