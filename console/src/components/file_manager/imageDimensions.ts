import { useCallback, useSyncExternalStore } from 'react'

export interface ImageSize {
  width: number
  height: number
}

// The sizes read on loaded images, by URL, for the rest of the session: going back to a folder
// shows them without loading the images again.
const sizes = new Map<string, ImageSize>()
const listeners = new Set<() => void>()

/** Keeps the natural size of a loaded image; an image without one (not decoded) is ignored. */
export function recordImageSize(url: string, image: HTMLImageElement) {
  const { naturalWidth: width, naturalHeight: height } = image
  if (!width || !height) return
  const known = sizes.get(url)
  if (known && known.width === width && known.height === height) return
  sizes.set(url, { width, height })
  listeners.forEach((listener) => listener())
}

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** The size of an image once one of its renders has loaded, undefined before. */
export function useImageSize(url: string | undefined): ImageSize | undefined {
  const snapshot = useCallback(() => (url ? sizes.get(url) : undefined), [url])
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}

/** For tests: forgets every size. */
export function clearImageSizes() {
  sizes.clear()
  listeners.forEach((listener) => listener())
}
