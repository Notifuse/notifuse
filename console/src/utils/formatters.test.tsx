import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import dayjs from '../lib/dayjs'
import { formatValue } from './formatters'

describe('formatValue dates', () => {
  // A browser clock other than Tokyo's, so a label read on it would show another hour
  const browserZone = process.env.TZ
  beforeEach(() => {
    process.env.TZ = 'America/Los_Angeles'
  })
  afterEach(() => {
    // An unset TZ must stay unset: assigning undefined stores the string "undefined"
    if (browserZone === undefined) delete process.env.TZ
    else process.env.TZ = browserZone
  })

  it('reads the date in UTC, and says so, for a timezone the browser does not know', async () => {
    render(<>{formatValue('2026-08-12T10:00:00Z', 'Mars/Olympus_Mons')}</>)

    fireEvent.mouseEnter(screen.getByText(dayjs('2026-08-12T10:00:00Z').fromNow()))

    expect(await screen.findByText('Wednesday, August 12, 2026 10:00 AM in UTC')).toBeInTheDocument()
  })

  it('gives the full date on the clock of the timezone it names', async () => {
    render(<>{formatValue('2026-08-12T10:00:00Z', 'Asia/Tokyo')}</>)

    fireEvent.mouseEnter(screen.getByText(dayjs('2026-08-12T10:00:00Z').fromNow()))

    // 10:00 UTC is 19:00 in Tokyo; the browser's clock, which the old label used, says 3:00 AM.
    expect(await screen.findByText('Wednesday, August 12, 2026 7:00 PM in Asia/Tokyo')).toBeInTheDocument()
  })

  it('names no timezone when it has none', async () => {
    render(<>{formatValue('2026-08-12T10:00:00Z')}</>)

    fireEvent.mouseEnter(screen.getByText(dayjs('2026-08-12T10:00:00Z').fromNow()))

    expect(await screen.findByText(dayjs('2026-08-12T10:00:00Z').format('LLLL'))).toBeInTheDocument()
  })
})

describe('formatValue numbers', () => {
  afterEach(() => {
    i18n.activate('en')
  })

  it('writes numbers with the separators of the console language', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    // French groups thousands with a narrow no-break space, which \s matches.
    render(
      <>
        <span>{formatValue(12345)}</span>
        <span>{formatValue(1234.567)}</span>
      </>
    )

    expect(screen.getByText(/^12\s345$/)).toBeInTheDocument()
    expect(screen.getByText(/^1\s234,57$/)).toBeInTheDocument()
  })

  it('keeps the English separators in English', () => {
    render(
      <>
        <span>{formatValue(12345)}</span>
        <span>{formatValue(1234.5)}</span>
      </>
    )

    expect(screen.getByText('12,345')).toBeInTheDocument()
    expect(screen.getByText('1,234.5')).toBeInTheDocument()
  })

  it('rounds a negative decimal half up, never to "-0"', () => {
    render(
      <>
        <span>{formatValue(-0.4)}</span>
        <span>{formatValue(-1234.5)}</span>
      </>
    )

    expect(screen.getByText('0')).toBeInTheDocument()
    expect(screen.getByText('-1,234')).toBeInTheDocument()
  })
})

