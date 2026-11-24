import { DEFAULT_INSTANCE_BRANDING, type InstanceBranding } from './instanceBranding'
import { validateBrandingUrl } from './brandingValidation'

/** The identity the preview draws from the values being edited, before anything is saved. */
export function previewBranding(values: {
  name: string
  logoUrl: string
  iconUrl: string
}): InstanceBranding {
  const name = values.name.trim()
  const logo = validateBrandingUrl(values.logoUrl.trim()) ? '' : values.logoUrl.trim()
  const icon = validateBrandingUrl(values.iconUrl.trim()) ? '' : values.iconUrl.trim()
  if (!name && !logo && !icon) return DEFAULT_INSTANCE_BRANDING
  return {
    ...DEFAULT_INSTANCE_BRANDING,
    custom: true,
    name: name || DEFAULT_INSTANCE_BRANDING.name,
    logo_url: logo,
    icon_url: icon,
    splash_logo_src: logo,
    favicon_src: '/console/neutral-icon.png'
  }
}
