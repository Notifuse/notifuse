import { describe, expect, it } from 'vitest'
import { ApiError } from '../../services/api/errors'
import { templateVersionConflict } from './templateVersionConflict'

describe('templateVersionConflict', () => {
  it('reads a 409 with latest_version as a version conflict', () => {
    const error = new ApiError('template version conflict', 409, { latest_version: 7 })
    expect(templateVersionConflict(error)).toEqual({ latestVersion: 7 })
  })

  it('reads a 409 without a body as a version conflict with no known version', () => {
    expect(templateVersionConflict(new ApiError('conflict', 409))).toEqual({ latestVersion: undefined })
  })

  it('does not read a taken slug as a version conflict', () => {
    const error = new ApiError('blog_post my-post already exists', 409, {
      error: 'blog_post my-post already exists',
      already_exists: { entity: 'blog_post', id: 'my-post', deleted: false }
    })
    expect(templateVersionConflict(error)).toBeNull()
  })

  it('ignores other statuses and other errors', () => {
    expect(templateVersionConflict(new ApiError('bad request', 400, { latest_version: 7 }))).toBeNull()
    expect(templateVersionConflict(new Error('network'))).toBeNull()
  })
})
