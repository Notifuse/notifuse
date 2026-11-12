import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { EmailBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'

// The width the blocks are laid out at before being scaled down: the width of a standard email
export const THUMB_LAYOUT_WIDTH = 600

interface BlockThumbProps {
  block: EmailBlock
  // The email the block would go into: its head gives the fonts, styles and attribute defaults
  emailTree: EmailBlock
  // Changes when the email's head changes. The thumbnail renders again only then or when the block changes, not on
  // every edit of the email
  headKey: string
  // Width shown, in pixels
  width: number
  // Height shown at most; taller content is cut
  maxHeight?: number
  // Height of the placeholder shown until the thumbnail is near the viewport
  placeholderHeight?: number
}

const noop = () => {}
const noopSave = () => Promise.resolve('')

// When the element comes near the viewport; at once where IntersectionObserver does not exist
const useNearViewport = (ref: React.RefObject<HTMLElement | null>) => {
  const [near, setNear] = useState(() => typeof IntersectionObserver === 'undefined')
  useEffect(() => {
    if (near || !ref.current) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) setNear(true)
      },
      { rootMargin: '200px' }
    )
    observer.observe(ref.current)
    return () => observer.disconnect()
  }, [near, ref])
  return near
}

const BlockThumbContent: React.FC<BlockThumbProps> = ({
  block,
  emailTree,
  headKey,
  width,
  maxHeight,
  placeholderHeight = 80
}) => {
  const outerRef = useRef<HTMLDivElement>(null)
  const innerRef = useRef<HTMLDivElement>(null)
  const near = useNearViewport(outerRef)
  const [contentHeight, setContentHeight] = useState<number | null>(null)
  const scale = width / THUMB_LAYOUT_WIDTH

  // Its own ids: a saved block keeps the ids of the block it was saved from, which may still be on the canvas
  const copy = useMemo(
    () => EmailBlockClass.regenerateIds(JSON.parse(JSON.stringify(block)) as EmailBlock),
    [block]
  )

  const rendered = useMemo(
    () =>
      near
        ? EmailBlockClass.renderEmailBlock(
            copy,
            EmailBlockClass.extractAttributeDefaults(emailTree),
            null,
            noop,
            emailTree,
            noop,
            noop,
            noop,
            noopSave
          )
        : null,
    // The email is read for its head only, which headKey stands for
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [near, copy, headKey]
  )

  // A transform does not shrink the layout box: the scaled height is set on the outer box
  useLayoutEffect(() => {
    const inner = innerRef.current
    if (!inner) return
    const measure = () => setContentHeight(inner.offsetHeight)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    observer.observe(inner)
    return () => observer.disconnect()
  }, [rendered])

  const scaledHeight = contentHeight ? Math.ceil(contentHeight * scale) : placeholderHeight
  const height = maxHeight ? Math.min(scaledHeight, maxHeight) : scaledHeight

  return (
    <div
      ref={outerRef}
      data-testid="block-thumb"
      aria-hidden="true"
      // React 18's typings predate `inert`, and a boolean `true` is dropped, so it is spread as the empty string
      {...({ inert: '' } as unknown as { inert?: string })}
      className="relative overflow-hidden bg-white"
      style={{ width, height, pointerEvents: 'none' }}
    >
      {rendered ? (
        <div
          ref={innerRef}
          className="email-canvas"
          style={{
            width: THUMB_LAYOUT_WIDTH,
            transform: `scale(${scale})`,
            transformOrigin: 'top left',
            position: 'absolute',
            top: 0,
            left: 0
          }}
        >
          {rendered}
        </div>
      ) : (
        <div data-testid="block-thumb-placeholder" className="h-full w-full bg-gray-100" />
      )}
    </div>
  )
}

// A small, inert rendering of a block, for the library's cards and the save dialog
export const BlockThumb = React.memo(
  BlockThumbContent,
  (prev, next) =>
    prev.block === next.block &&
    prev.headKey === next.headKey &&
    prev.width === next.width &&
    prev.maxHeight === next.maxHeight &&
    prev.placeholderHeight === next.placeholderHeight
)
