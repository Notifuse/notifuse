import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import jaJP from 'antd/locale/ja_JP'
import { i18n } from '@lingui/core'
import dayjs from '../../lib/dayjs'
import { activateDayjsLocale } from '../../i18n/dayjsLocales'
import { DateRangePicker } from './toolbar'

vi.mock('./context', () => ({
  useWebAnalytics: () => ({
    period: 'custom',
    range: { start: dayjs('2026-02-03'), end: dayjs('2026-02-09').endOf('day') },
    customStart: '2026-02-03',
    customEnd: '2026-02-09',
    setPeriod: vi.fn(),
    setCustomRange: vi.fn(),
    timezone: 'UTC',
    setTimezone: vi.fn()
  })
}))

afterEach(() => {
  activateDayjsLocale('en')
  i18n.activate('en')
})

describe('DateRangePicker in the console language', () => {
  it('labels the custom range and fills the picker in Japanese', async () => {
    i18n.load('ja', {})
    i18n.activate('ja')
    activateDayjsLocale('ja')
    render(
      <ConfigProvider locale={jaJP}>
        <DateRangePicker />
      </ConfigProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: /02\/03～02\/09/ }))
    fireEvent.click(await screen.findByText('Custom range'))

    expect(await screen.findByDisplayValue('2026年2月3日')).toBeInTheDocument()
    expect(screen.getByDisplayValue('2026年2月9日')).toBeInTheDocument()
  })
})
