import { getBrandName } from '../components/brand/instanceBranding'

/**
 * The name to print where Notifuse's would be: the instance brand while a licence covers it,
 * "Notifuse" otherwise. The value is fixed for the life of the page: a new brand or a licence
 * change shows at the next page load, when config.js is read again.
 */
export function useBrandName(): string {
  return getBrandName()
}
