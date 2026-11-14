import { useCallback, useEffect, useRef } from 'react'

/**
 * Gives the focus back to a menu's button when the menu closes from the keyboard and leaves it
 * nowhere: antd 6.6 drops it on the body after Escape. A close that moved the focus elsewhere, a
 * click on another control, keeps it there, and so does a close made with the pointer (an entry
 * clicked): a focus put back then would hold the unpinned sidebar open.
 */
export function useFocusBack<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const lastInput = useRef<'keyboard' | 'pointer'>('keyboard')
  useEffect(() => {
    const onKey = () => {
      lastInput.current = 'keyboard'
    }
    const onPointer = () => {
      lastInput.current = 'pointer'
    }
    document.addEventListener('keydown', onKey, true)
    document.addEventListener('pointerdown', onPointer, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      document.removeEventListener('pointerdown', onPointer, true)
    }
  }, [])
  const focusBack = useCallback(() => {
    if (lastInput.current === 'pointer') return
    // After antd has handled the key, so the focus it leaves is the one read
    setTimeout(() => {
      const active = document.activeElement
      if (!active || active === document.body || active.closest('.ant-dropdown')) ref.current?.focus()
    })
  }, [])
  return { ref, focusBack }
}
