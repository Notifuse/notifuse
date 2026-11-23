import { ApiError } from '../../services/api/errors'

/**
 * Reads a failed post update as a template version conflict: the template moved past the
 * version the editor started from, which the user resolves by overwriting or reloading.
 *
 * A post update also answers 409 when the slug is already taken. That body carries
 * `already_exists` and has no version to reload, so it is not a version conflict.
 */
export function templateVersionConflict(error: unknown): { latestVersion?: number } | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null
  const data = error.data as { latest_version?: number; already_exists?: unknown } | undefined
  if (data?.already_exists) return null
  return { latestVersion: data?.latest_version }
}
