import { useCallback, useRef } from 'react'

let focusSessions = 0

// useCoalesceKey gives a typed number field the key that makes its writes one undo step: the same
// key from focus to blur, a new one at the next focus.
export function useCoalesceKey(base: string): { coalesceKey: () => string; onFocus: () => void } {
  const session = useRef<number>(++focusSessions)
  const onFocus = useCallback(() => {
    session.current = ++focusSessions
  }, [])
  const coalesceKey = useCallback(() => `${base}:${session.current}`, [base])
  return { coalesceKey, onFocus }
}
