import type { AutoUpdateOptions, UseDismissProps, UseFloatingOptions } from '@floating-ui/react'
import {
  autoUpdate,
  useDismiss,
  useFloating,
  useInteractions,
  useTransitionStyles
} from '@floating-ui/react'
import { useEffect, useMemo } from 'react'

export interface FloatingMenuReturn {
  /**
   * Whether the floating menu is currently mounted in the DOM.
   */
  isMounted: boolean
  /**
   * Ref function to attach to the floating menu DOM node.
   */
  ref: (node: HTMLElement | null) => void
  /**
   * Combined styles for positioning, transitions, and z-index.
   */
  style: React.CSSProperties
  /**
   * Returns props that should be spread onto the floating menu.
   */
  getFloatingProps: (userProps?: React.HTMLProps<HTMLElement>) => Record<string, unknown>
  /**
   * Returns props that should be spread onto the reference element.
   */
  getReferenceProps: (userProps?: React.HTMLProps<Element>) => Record<string, unknown>
}

/**
 * Custom hook for creating and managing floating menus relative to an anchor position
 *
 * @param isOpen - Boolean controlling visibility of the floating menu
 * @param anchorElement - DOMRect, function returning DOMRect, or HTMLElement representing the position to anchor the floating menu to
 * @param zIndex - Z-index value for the floating menu
 * @param options - Additional options to pass to the underlying useFloating hook
 * @param autoUpdateOptions - Options for auto-updating the floating menu position
 * @returns Object containing properties and methods to control the floating menu
 */
export function useFloatingMenu(
  isOpen: boolean,
  anchorElement: HTMLElement | DOMRect | (() => DOMRect | null) | null,
  zIndex: number,
  options?: Partial<UseFloatingOptions & { dismissOptions?: UseDismissProps }>,
  autoUpdateOptions?: AutoUpdateOptions
): FloatingMenuReturn {
  const { dismissOptions, ...floatingOptions } = options || {}

  const { refs, context, floatingStyles } = useFloating({
    open: isOpen,
    whileElementsMounted(referenceEl, floatingEl, update) {
      const cleanup = autoUpdate(referenceEl, floatingEl, update, autoUpdateOptions)
      return cleanup
    },
    ...floatingOptions
  })

  const { isMounted, styles } = useTransitionStyles(context)

  const dismiss = useDismiss(context, dismissOptions)

  const { getReferenceProps, getFloatingProps } = useInteractions([dismiss])

  useEffect(() => {
    if (anchorElement === null) {
      refs.setReference(null)
      return
    }

    // If anchorElement is an actual DOM element, use it directly
    // autoUpdate will automatically observe it for scroll/resize
    if (anchorElement instanceof HTMLElement) {
      refs.setReference(anchorElement)
      return
    }

    const getBoundingClientRect = () => {
      const rect = typeof anchorElement === 'function' ? anchorElement() : anchorElement
      return rect || new DOMRect()
    }

    refs.setReference({
      getBoundingClientRect
    })
  }, [anchorElement, refs])

  return useMemo(
    () => ({
      isMounted,
      ref: refs.setFloating,
      style: {
        ...styles,
        ...floatingStyles,
        zIndex
      },
      getFloatingProps,
      getReferenceProps
    }),
    [
      floatingStyles,
      isMounted,
      refs.setFloating,
      styles,
      zIndex,
      getFloatingProps,
      getReferenceProps
    ]
  )
}
