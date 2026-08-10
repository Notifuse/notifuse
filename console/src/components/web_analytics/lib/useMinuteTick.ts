import { Dayjs } from 'dayjs'
import { useEffect, useState } from 'react'
import dayjs from '../../../lib/dayjs'

/**
 * A clock that only advances once a minute.
 *
 * Live views bound their range to "now", but reading the real clock during
 * render gives every render a different range — and therefore a different
 * query key — which turns a cache lookup into an endless refetch. Ticking on
 * an interval keeps the key stable between renders while the window still
 * follows the clock.
 */
export function useMinuteTick(): Dayjs {
  const [anchor, setAnchor] = useState(() => dayjs().startOf('minute'))

  useEffect(() => {
    const timer = setInterval(() => setAnchor(dayjs().startOf('minute')), 60_000)
    return () => clearInterval(timer)
  }, [])

  return anchor
}
