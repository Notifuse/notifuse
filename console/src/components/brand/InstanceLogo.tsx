import { useState } from 'react'
import { NotifuseLogo, NotifuseMark } from './NotifuseLogo'
import {
  brandImageFailed,
  brandInitial,
  getInstanceBranding,
  markBrandImageFailed,
  type InstanceBranding
} from './instanceBranding'

interface InstanceLogoProps {
  /** full: the main logo or the name; compact: the icon or the initial. */
  variant: 'full' | 'compact'
  /** The frame the mark fits in, in px. The image keeps its proportions inside it. */
  maxWidth: number
  maxHeight: number
  /** The identity to draw; the one the server resolved by default. The settings preview passes its own. */
  branding?: InstanceBranding
  /** Height of Notifuse's own drawing in default mode; it never exceeds maxHeight. */
  notifuseHeight?: number
  className?: string
}

/**
 * The instance's mark: Notifuse's drawings in default mode; in custom mode, the configured image,
 * or the brand name as text when there is no image or it fails to load. A URL that failed is not
 * requested again in this page; a new URL is. Images never send a Referer to the host that serves them.
 */
export function InstanceLogo({
  variant,
  maxWidth,
  maxHeight,
  branding,
  notifuseHeight,
  className
}: InstanceLogoProps) {
  const identity = branding ?? getInstanceBranding()
  // Bumped when an image fails, to re-render with the fallback; the failure itself is remembered
  // per URL for the whole page (markBrandImageFailed).
  const [, setFailures] = useState(0)

  if (!identity.custom) {
    return variant === 'full' ? (
      <NotifuseLogo height={Math.min(notifuseHeight ?? 22, maxHeight)} className={className} />
    ) : (
      <NotifuseMark height={Math.min(notifuseHeight ?? 24, maxHeight)} className={className} />
    )
  }

  const url = variant === 'full' ? identity.logo_url : identity.icon_url
  if (url && !brandImageFailed(url)) {
    return (
      <img
        src={url}
        alt={identity.name}
        referrerPolicy="no-referrer"
        onError={() => {
          markBrandImageFailed(url)
          setFailures((count) => count + 1)
        }}
        className={`block ${className ?? ''}`}
        style={{ maxWidth, maxHeight, width: 'auto', height: 'auto', objectFit: 'contain' }}
      />
    )
  }

  if (variant === 'compact') {
    return (
      <span
        role="img"
        aria-label={identity.name}
        className={`grid place-items-center rounded-md bg-fill text-base font-semibold text-ink ${className ?? ''}`}
        style={{ width: maxWidth, height: maxHeight }}
      >
        {brandInitial(identity.name)}
      </span>
    )
  }

  return (
    <span
      title={identity.name}
      className={`block min-w-0 truncate font-semibold text-ink ${maxHeight >= 40 ? 'text-title' : 'text-subtitle'} ${className ?? ''}`}
      style={{ maxWidth, lineHeight: `${maxHeight}px` }}
    >
      {identity.name}
    </span>
  )
}
