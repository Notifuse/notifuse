import { useEffect, useRef, type RefObject } from 'react'

export interface DrawerNavKeysOptions {
  /** True while the drawer is open. */
  enabled: boolean
  onPrev?: () => void
  onNext?: () => void
  /** An element inside the drawer, from which the hook finds the drawer's overlay. */
  anchorRef: RefObject<HTMLElement | null>
}

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return true
  // jsdom implements no isContentEditable; the attribute is what a browser reads anyway.
  return target.closest('[contenteditable]:not([contenteditable="false"]), [role="textbox"]') !== null
}

/**
 * Whether no open modal or drawer covers this one. antd gives modals and drawers the same z-index,
 * so the one later in the document is the one on top.
 */
function isTopmost(anchor: HTMLElement | null): boolean {
  const own = anchor?.closest<HTMLElement>('.ant-drawer')
  if (!own) return false
  const overlays = document.querySelectorAll<HTMLElement>('.ant-drawer.ant-drawer-open, .ant-modal-wrap')
  return !Array.from(overlays).some(
    (overlay) =>
      overlay !== own &&
      overlay.style.display !== 'none' &&
      (own.compareDocumentPosition(overlay) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
  )
}

/** An open popconfirm, popover or dropdown menu: J and K belong to it, not to the list behind. */
function hasOpenPopup(): boolean {
  return (
    document.querySelector(
      '.ant-popover:not(.ant-popover-hidden), .ant-dropdown:not(.ant-dropdown-hidden)'
    ) !== null
  )
}

/** J goes to the next item and K to the previous one while the drawer is open and on top. */
export function useDrawerNavKeys({ enabled, onPrev, onNext, anchorRef }: DrawerNavKeysOptions) {
  // The latest handlers, so the listener does not resubscribe on every render.
  const handlers = useRef({ onPrev, onNext })
  useEffect(() => {
    handlers.current = { onPrev, onNext }
  })

  useEffect(() => {
    if (!enabled) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat) return
      if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return
      const key = event.key.toLowerCase()
      if (key !== 'j' && key !== 'k') return
      if (isEditable(event.target)) return
      if (!isTopmost(anchorRef.current) || hasOpenPopup()) return
      const action = key === 'j' ? handlers.current.onNext : handlers.current.onPrev
      if (!action) return
      event.preventDefault()
      action()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [enabled, anchorRef])
}
