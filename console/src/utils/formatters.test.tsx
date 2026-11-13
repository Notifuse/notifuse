import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import dayjs from '../lib/dayjs'
import { formatValue } from './formatters'

describe('formatValue dates', () => {
  it('gives the full date on the clock of the timezone it names', async () => {
    render(<>{formatValue('2026-08-12T10:00:00Z', 'Asia/Tokyo')}</>)

    fireEvent.mouseEnter(screen.getByText(dayjs('2026-08-12T10:00:00Z').fromNow()))

    // 10:00 UTC is 19:00 in Tokyo. The browser's clock, which the old label used, shows another
    // hour on every runner not on UTC+9.
    expect(await screen.findByText('Wednesday, August 12, 2026 7:00 PM in Asia/Tokyo')).toBeInTheDocument()
  })

  it('names no timezone when it has none', async () => {
    render(<>{formatValue('2026-08-12T10:00:00Z')}</>)

    fireEvent.mouseEnter(screen.getByText(dayjs('2026-08-12T10:00:00Z').fromNow()))

    expect(await screen.findByText(dayjs('2026-08-12T10:00:00Z').format('LLLL'))).toBeInTheDocument()
  })
})
