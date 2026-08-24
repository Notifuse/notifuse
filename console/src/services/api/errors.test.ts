import { describe, it, expect } from 'vitest'
import {
  ApiError,
  RESOURCE_LABELS,
  permissionDenial,
  permissionDenialFromBody,
  permissionDeniedMessage,
  shouldRetryQuery
} from './errors'
import { ALL_PERMISSION_RESOURCES } from './permissions'

describe('permissionDenial', () => {
  it('reads the missing grant off an ApiError body', () => {
    const err = new ApiError('permission denied', 403, {
      error: 'permission denied: contacts write',
      resource: 'contacts',
      permission: 'write'
    })

    expect(permissionDenial(err)).toEqual({ resource: 'contacts', permission: 'write' })
  })

  // The regression that matters: ErrWorkspaceLimitReached and ErrTeamMemberLimitReached also
  // answer 403, and two call sites string-match their message to raise an upgrade prompt.
  // Detecting a denial by status would relabel a billing problem as a permissions problem.
  it('does not read a quota 403 as a denial', () => {
    const err = new ApiError('team member limit reached', 403, {
      error: 'team member limit reached'
    })

    expect(permissionDenial(err)).toBeNull()
  })

  it('ignores an error that is not an ApiError, and a body that is not an object', () => {
    const plain = new Error('boom')
    ;(plain as unknown as { data: unknown }).data = { resource: 'contacts', permission: 'read' }

    expect(permissionDenial(plain)).toBeNull()
    expect(permissionDenialFromBody(null)).toBeNull()
    expect(permissionDenialFromBody('permission denied')).toBeNull()
  })

  it('ignores a body carrying only one of the two fields', () => {
    expect(permissionDenialFromBody({ error: 'nope', resource: 'contacts' })).toBeNull()
    expect(permissionDenialFromBody({ error: 'nope', permission: 'read' })).toBeNull()
  })

  // A backend newer than this bundle: without a label to render, the server's own sentence is
  // the better answer.
  it('ignores a resource or verb this bundle does not know', () => {
    expect(permissionDenialFromBody({ resource: 'teleporter', permission: 'read' })).toBeNull()
    expect(permissionDenialFromBody({ resource: 'contacts', permission: 'delete' })).toBeNull()
  })

  it('recognises every resource the backend can name', () => {
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(permissionDenialFromBody({ resource, permission: 'read' })).toEqual({
        resource,
        permission: 'read'
      })
    }
  })
})

describe('permissionDeniedMessage', () => {
  it('names the resource and the verb', () => {
    expect(permissionDeniedMessage({ resource: 'contacts', permission: 'read' })).toBe(
      'You do not have read access to Contacts.'
    )
    expect(
      permissionDeniedMessage({ resource: 'webhook_subscriptions', permission: 'write' })
    ).toBe('You do not have write access to Webhook Subscriptions.')
  })

  it('gives every resource a distinct label, never the raw token', () => {
    const messages = ALL_PERMISSION_RESOURCES.map((resource) => {
      expect(RESOURCE_LABELS[resource]).toBeDefined()
      return permissionDeniedMessage({ resource, permission: 'read' })
    })

    expect(new Set(messages).size).toBe(ALL_PERMISSION_RESOURCES.length)
    for (const message of messages) {
      expect(message).not.toContain('_')
    }
  })
})

describe('shouldRetryQuery', () => {
  it('does not retry a denial or an expired session', () => {
    expect(shouldRetryQuery(0, new ApiError('permission denied', 403, {}))).toBe(false)
    expect(shouldRetryQuery(0, new ApiError('Session expired', 401, {}))).toBe(false)
  })

  it('still retries a server error once', () => {
    const error = new ApiError('boom', 500, {})

    expect(shouldRetryQuery(0, error)).toBe(true)
    expect(shouldRetryQuery(1, error)).toBe(false)
  })

  it('still retries an error that never reached the API client', () => {
    expect(shouldRetryQuery(0, new TypeError('Failed to fetch'))).toBe(true)
  })
})
