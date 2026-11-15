import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { FillBar } from './FillBar'

const fill = (ratio: number) => {
  const { container, unmount } = render(<FillBar ratio={ratio} />)
  const width = (container.querySelector('[data-rate-fill]') as HTMLElement).style.width
  unmount()
  return width
}

describe('FillBar', () => {
  it('fills to the ratio, rounded to a tenth of a percent', () => {
    expect(fill(0.5)).toBe('50%')
    expect(fill(0.12345)).toBe('12.3%')
  })

  it('clamps outside 0-1 and draws NaN empty', () => {
    expect(fill(1.4)).toBe('100%')
    expect(fill(-0.2)).toBe('0%')
    expect(fill(Number.NaN)).toBe('0%')
  })

  it('takes the width it is given', () => {
    const { container } = render(<FillBar ratio={0.5} className="flex-1" />)
    expect(container.firstElementChild!.className).toMatch(/\bflex-1\b/)
    expect(container.firstElementChild!.className).not.toMatch(/\bw-6\.5\b/)
  })
})
