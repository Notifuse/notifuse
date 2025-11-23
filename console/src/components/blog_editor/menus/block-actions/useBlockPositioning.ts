import { useMemo } from 'react'
import { offset } from '@floating-ui/react'

/**
 * Hook that provides positioning configuration for the block actions grip
 * Returns positioning middleware for floating-ui
 */
export function useBlockPositioning() {
  return useMemo(
    () => ({
      middleware: [
        offset((state) => {
          const { rects } = state
          const blockHeight = rects.reference.height
          const gripHeight = rects.floating.height

          const verticalCenter = blockHeight / 2 - gripHeight / 2

          return {
            mainAxis: 6,
            // For larger blocks, align to top; for smaller ones, center vertically
            crossAxis: blockHeight > 40 ? 0 : verticalCenter
          }
        })
      ]
    }),
    []
  )
}

