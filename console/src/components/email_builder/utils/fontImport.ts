import { withHttpsScheme } from '../../../utils/safeHtml'
import { hasLiquidOutput } from '../../../utils/liquid'

// Popular Google Fonts: picking one writes its name and stylesheet together
export const POPULAR_FONTS = [
  { name: 'Open Sans', url: 'https://fonts.googleapis.com/css2?family=Open+Sans:wght@300;400;600;700&display=swap' },
  { name: 'Roboto', url: 'https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap' },
  { name: 'Lato', url: 'https://fonts.googleapis.com/css2?family=Lato:wght@300;400;700&display=swap' },
  { name: 'Montserrat', url: 'https://fonts.googleapis.com/css2?family=Montserrat:wght@300;400;500;600;700&display=swap' },
  {
    name: 'Source Sans Pro',
    url: 'https://fonts.googleapis.com/css2?family=Source+Sans+Pro:wght@300;400;600;700&display=swap'
  }
] as const

// A stylesheet address: an http(s) address (protocol-relative included, as the email reads it) or
// a Liquid output, which has no address until the email is sent
export const isFontStylesheetAddress = (href: string): boolean => {
  if (hasLiquidOutput(href)) return true
  try {
    return ['http:', 'https:'].includes(new URL(withHttpsScheme(href)).protocol)
  } catch {
    return false
  }
}
