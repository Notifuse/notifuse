/**
 * How the licence banner and the fixed chrome underneath it stay out of each other's way.
 *
 * The banner is mounted above workspace selection, in the authenticated shell, because read-only
 * mode is a property of the deployment rather than of a workspace. Everything it has to displace
 * — WorkspaceLayout's fixed header and sider, the workspace picker's fixed panel — is mounted by
 * routes rendered underneath it, so a prop cannot reach them.
 *
 * A CSS variable on the document element can. The banner publishes its measured height there;
 * the layouts read it through calc(). The fallback is 0px, so a console with nothing to say about
 * its licence — every licensed one, which is nearly all of them — lays out byte for byte as it
 * did before.
 */
export const LICENSE_BANNER_HEIGHT_VAR = '--license-banner-height'

/** `calc(base + banner)`, for a fixed offset that has to follow the banner. */
export function withBannerOffset(base: string): string {
  return `calc(${base} + var(${LICENSE_BANNER_HEIGHT_VAR}, 0px))`
}

/** `calc(base - banner)`, for a full-height element that has to shrink by the banner. */
export function minusBannerOffset(base: string): string {
  return `calc(${base} - var(${LICENSE_BANNER_HEIGHT_VAR}, 0px))`
}
