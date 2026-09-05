import { api } from './client'
import type { LicenseResponse } from '../../types/license'

/**
 * The two licence endpoints, both root-only on the server (they live on the settings handler,
 * which already carries requireRootUser). A non-root caller gets 403 from `get` — that is the
 * expected answer, not a fault, and the licence context treats it as "not my business to know".
 *
 * There is no `licence.features` and no way to read the key back. The feature matrix is a web
 * page the Additional Use Grant pins by URL and by version, and a second copy served from here
 * would drift from the text that is legally binding; the key is a bearer credential, so an
 * endpoint that echoed it would copy it into every browser cache and support screenshot.
 */
export const licenseApi = {
  async get(): Promise<LicenseResponse> {
    return api.get<LicenseResponse>('/api/licence.get')
  },

  // Answers the new state in the same round trip that installed the key, so the console repaints
  // from the swap rather than from a follow-up read that could race it.
  async set(key: string): Promise<LicenseResponse> {
    return api.post<LicenseResponse>('/api/licence.set', { key })
  }
}
