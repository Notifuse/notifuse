import type { FileManagerSettings } from './interfaces'

/**
 * The file storage settings a save sends: never the response-only fields, and the secret only
 * when one was typed, since the server keeps the stored secret when the field is absent.
 */
export function savedFileManagerSettings(settings: FileManagerSettings): FileManagerSettings {
  const saved: FileManagerSettings = {
    provider: settings.provider,
    endpoint: settings.endpoint,
    access_key: settings.access_key,
    bucket: settings.bucket,
    region: settings.region,
    cdn_endpoint: settings.cdn_endpoint,
    force_path_style: settings.force_path_style
  }
  if (settings.secret_key) saved.secret_key = settings.secret_key
  return saved
}
