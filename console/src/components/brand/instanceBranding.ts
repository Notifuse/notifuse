// The identity the instance shows, as the server resolved it into window.INSTANCE_BRANDING
// (/config.js). The console never reads a stored brand itself: the server already applied the
// licence, so every screen and the inline bootstrap of index.html agree.

export type InstanceBranding = InstanceBrandingGlobal

export const DEFAULT_BRAND_NAME = 'Notifuse'

/** The Notifuse identity, used when config.js predates branding or did not load. */
export const DEFAULT_INSTANCE_BRANDING: InstanceBranding = {
  custom: false,
  name: DEFAULT_BRAND_NAME,
  logo_url: '',
  icon_url: '',
  hide_help_menu: false,
  splash_logo_src: '/console/logo.png',
  favicon_src: '/console/favicon-32x32.png',
  revision: ''
}

/** The identity to show now. */
export function getInstanceBranding(): InstanceBranding {
  const served = typeof window === 'undefined' ? undefined : window.INSTANCE_BRANDING
  if (!served || typeof served !== 'object') {
    return DEFAULT_INSTANCE_BRANDING
  }
  return { ...DEFAULT_INSTANCE_BRANDING, ...served }
}

/**
 * The name to print where Notifuse's would be. A plain function for modules that are not React
 * components; components use useBrandName.
 */
export function getBrandName(): string {
  return getInstanceBranding().name || DEFAULT_BRAND_NAME
}

type GraphemeSegmenterCtor = new (
  locales?: string | string[],
  options?: { granularity: 'grapheme' }
) => { segment(input: string): Iterable<{ segment: string }> }

/**
 * `Intl.Segmenter` is ES2022 while the app compiles against ES2021.Intl, so it is read off the
 * global behind a feature check rather than through its type (as in web_analytics annotations).
 */
const segmenterCtor = (Intl as unknown as { Segmenter?: GraphemeSegmenterCtor }).Segmenter

/**
 * The first user-perceived character of a name, upper-cased, for a compact mark; "?" when the
 * name has none. Without Segmenter, the first code point is kept whole, which splits composed
 * emoji but never a surrogate pair. The segmenter is a parameter (null: none) so both paths can
 * be tested.
 */
export function brandInitial(
  name: string,
  Segmenter: GraphemeSegmenterCtor | null = segmenterCtor ?? null
): string {
  const trimmed = name.trim()
  if (!trimmed) return '?'
  let first: string | undefined
  if (Segmenter) {
    for (const part of new Segmenter(undefined, { granularity: 'grapheme' }).segment(trimmed)) {
      first = part.segment
      break
    }
  } else {
    first = Array.from(trimmed)[0]
  }
  return first ? first.toLocaleUpperCase() : '?'
}

// Brand image URLs that failed to load in this page. Kept outside the components so that the
// sidebar, which swaps its full and compact marks on every fold, does not request a broken image
// again each time. A new URL is tried: only that exact URL is remembered.
const failedImageUrls = new Set<string>()

export function markBrandImageFailed(url: string): void {
  failedImageUrls.add(url)
}

export function brandImageFailed(url: string): boolean {
  return failedImageUrls.has(url)
}

/** Forgets every failure; for tests, which share this module. */
export function resetBrandImageFailures(): void {
  failedImageUrls.clear()
}
