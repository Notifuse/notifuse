/* eslint-disable react-refresh/only-export-components */
import { ReactNode } from 'react'
import { CircleHelp, Globe } from 'lucide-react'
import { countryName } from './isoCountries'

// Brand marks live in public/icons rather than the bundle: they are decorative,
// and a breakdown table only ever shows a handful of them.

/**
 * Browser brand → asset. Matched as a substring of the ua-parser-js name, so
 * one entry covers every variant of a brand ("Mobile Safari", "Chrome
 * WebView", "Samsung Internet"). Ordered from most to least specific.
 */
const BROWSER_ICONS: [token: string, src: string][] = [
  ['brave', '/icons/browsers/brave.png'],
  ['samsung', '/icons/browsers/samsung.svg'],
  ['edge', '/icons/browsers/edge.svg'],
  ['opera', '/icons/browsers/opera.svg'],
  ['firefox', '/icons/browsers/firefox.svg'],
  ['chrome', '/icons/browsers/chrome.svg'],
  ['safari', '/icons/browsers/safari.svg']
]

/**
 * OS name → asset. The SDK normalizes to a fixed vocabulary (Windows, macOS,
 * iOS, iPadOS, Android, Linux, Chrome OS), but sessions recorded by older
 * snippets can still carry raw ua-parser-js output such as "Mac OS", so the
 * variants are matched too. Longest tokens first: "ipados" and "macos" must
 * not be swallowed by a shorter match.
 */
const OS_ICONS: [token: string, src: string][] = [
  ['chrome os', '/icons/os/chromeos.svg'],
  ['chromeos', '/icons/os/chromeos.svg'],
  ['ipados', '/icons/os/ios.svg'],
  ['macos', '/icons/os/macos.svg'],
  ['mac os', '/icons/os/macos.svg'],
  ['windows', '/icons/os/windows.svg'],
  ['android', '/icons/os/android.svg'],
  ['ubuntu', '/icons/os/ubuntu.svg'],
  ['linux', '/icons/os/linux.svg'],
  ['ios', '/icons/os/ios.svg']
]

const DEVICE_ICONS: Record<string, string> = {
  desktop: '/icons/devices/desktop.svg',
  mobile: '/icons/devices/mobile.svg',
  tablet: '/icons/devices/tablet.svg'
}

const FALLBACK_CLASS = 'w-4 h-4 text-gray-400 flex-shrink-0'

function hideOnError(event: { currentTarget: HTMLImageElement }): void {
  event.currentTarget.style.display = 'none'
}

function ImageIcon(props: { src: string; alt?: string; className?: string }) {
  return (
    <img
      src={props.src}
      alt={props.alt ?? ''}
      className={props.className ?? 'w-4 h-4 flex-shrink-0'}
      onError={hideOnError}
    />
  )
}

function matchToken(value: string, table: [string, string][]): string | undefined {
  return table.find(([token]) => value.includes(token))?.[1]
}

/**
 * Flag for an ISO 3166-1 alpha-2 code. The asset set is comprehensive but not
 * exhaustive (GeoIP also reports codes like "EU"), so a missing file hides the
 * image instead of leaving a broken one in the middle of a table row.
 */
export function CountryFlag(props: { iso2: string; className?: string }): ReactNode {
  const code = props.iso2?.trim().toLowerCase() ?? ''
  if (code.length !== 2) return null
  return (
    <img
      src={`/icons/flags/${code}.svg`}
      alt={countryName(props.iso2)}
      title={countryName(props.iso2)}
      className={props.className ?? 'w-4 h-3 rounded-[2px] object-cover flex-shrink-0'}
      onError={hideOnError}
    />
  )
}

/**
 * Icon for a breakdown row, chosen by the dimension the row belongs to.
 * Accepts either the dimension id (`browser`, `os`, `device`, `country`) or the
 * plural tab key the tables are grouped under.
 */
export function getDeviceIcon(value: string, tabKey: string): ReactNode {
  const lowerValue = value?.toLowerCase().trim() ?? ''

  switch (tabKey) {
    case 'device':
    case 'devices': {
      const src = DEVICE_ICONS[lowerValue]
      return src ? <ImageIcon src={src} /> : <CircleHelp className={FALLBACK_CLASS} />
    }
    case 'browser':
    case 'browsers': {
      const src = matchToken(lowerValue, BROWSER_ICONS)
      return src ? <ImageIcon src={src} /> : <Globe className={FALLBACK_CLASS} />
    }
    case 'os': {
      const src = matchToken(lowerValue, OS_ICONS)
      return src ? <ImageIcon src={src} /> : <CircleHelp className={FALLBACK_CLASS} />
    }
    case 'country':
    case 'countries':
      return <CountryFlag iso2={value} />
    default:
      return null
  }
}
