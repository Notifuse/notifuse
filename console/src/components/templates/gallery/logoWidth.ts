import { useEffect, useState } from 'react'

/** The height a logo has in the gallery's headers: the placeholder's (120×40). */
export const LOGO_HEIGHT = 40
/** A very wide logo stops here, a very narrow one there. */
const MAX_WIDTH = 180
const MIN_WIDTH = 24
/** A workspace logo whose size is not known yet (or that failed to load): a square's width. */
export const UNKNOWN_LOGO_WIDTH = LOGO_HEIGHT

/**
 * The width that gives a logo the headers' height, from its natural size. MJML sizes an image by
 * its width, so a square logo given the placeholder's 120px would stand 120px tall.
 */
export function logoWidthFor(naturalWidth: number, naturalHeight: number): number {
  if (!naturalWidth || !naturalHeight) return UNKNOWN_LOGO_WIDTH
  const width = Math.round((naturalWidth * LOGO_HEIGHT) / naturalHeight)
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, width))
}

// One load per address for the console's lifetime: the modal, its previews and the drawer share it.
const loads = new Map<string, Promise<number>>()

/** Loads a logo once and gives the width that sets it at the headers' height. */
export function loadLogoWidth(url: string): Promise<number> {
  const known = loads.get(url)
  if (known) return known
  const load = new Promise<number>((resolve) => {
    if (typeof Image === 'undefined') {
      resolve(UNKNOWN_LOGO_WIDTH)
      return
    }
    const image = new Image()
    image.onload = () => resolve(logoWidthFor(image.naturalWidth, image.naturalHeight))
    image.onerror = () => resolve(UNKNOWN_LOGO_WIDTH)
    image.src = url
  })
  loads.set(url, load)
  return load
}

/** Loads a logo to read its natural size; undefined until then, or without a logo. */
export function useLogoWidth(url: string | undefined): number | undefined {
  const [width, setWidth] = useState<{ url: string; width: number } | undefined>()
  useEffect(() => {
    if (!url) return
    let current = true
    void loadLogoWidth(url).then((logoWidth) => current && setWidth({ url, width: logoWidth }))
    return () => {
      current = false
    }
  }, [url])
  return width && width.url === url ? width.width : undefined
}
