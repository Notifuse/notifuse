import { describe, it, expect } from 'vitest'
import { ApiError } from './errors'
import {
  LICENSE_FEATURE_LABELS,
  licenseRefusal,
  licenseRefusalFromBody,
  licenseRefusedMessage
} from './licenseErrors'

describe('licenseRefusalFromBody', () => {
  it('reads the refused gate off a 402 body', () => {
    expect(
      licenseRefusalFromBody({
        error: 'license_required',
        feature: 'ses_tenant',
        required_tier: 'Studio',
        message: 'SES tenant isolation requires a Notifuse licence (Studio or above).',
        docs: 'https://notifuse.com/licence-features'
      })
    ).toEqual({
      feature: 'ses_tenant',
      requiredTier: 'Studio',
      message: 'SES tenant isolation requires a Notifuse licence (Studio or above).',
      docs: 'https://notifuse.com/licence-features'
    })
  })

  // The workspace ceiling deliberately carries no required_tier: which plan lifts it depends on
  // how many workspaces the deployment already holds.
  it('reports no tier when the body names none', () => {
    const refusal = licenseRefusalFromBody({
      error: 'license_required',
      feature: 'workspaces',
      message: 'workspace quota reached: 8 workspaces exist (limit: 3)',
      docs: 'https://notifuse.com/licence-features'
    })

    expect(refusal?.requiredTier).toBeNull()
  })

  // Detection is on the agreed body code, never on the status: a 402 with some other body is not
  // ours to relabel, and a permission denial must keep its own path.
  it('ignores a body that does not carry the licence code', () => {
    expect(licenseRefusalFromBody({ error: 'permission denied' })).toBeNull()
    expect(licenseRefusalFromBody(null)).toBeNull()
    expect(licenseRefusalFromBody('license_required')).toBeNull()
  })

  it('reads the refusal off a thrown ApiError, and ignores anything else', () => {
    const err = new ApiError('licence required', 402, {
      error: 'license_required',
      feature: 'sso',
      message: 'read-only'
    })

    expect(licenseRefusal(err)?.feature).toBe('sso')
    expect(licenseRefusal(new Error('boom'))).toBeNull()
  })
})

describe('licenseRefusedMessage', () => {
  // sso has no special case any more. The SSO gate refuses at the OIDC service, which never
  // produces a 402 — the sign-in page is unauthenticated, and an error there naming a licence
  // would publish this deployment's commercial status to anyone who loaded it. The label is
  // kept so a future gate that does refuse sso with a 402 has a name for it.
  it('names single sign-on like any other capability', () => {
    const message = licenseRefusedMessage({
      feature: 'sso',
      requiredTier: 'Enterprise',
      message: 'server prose',
      docs: 'https://notifuse.com/licence-features'
    })

    expect(message).toContain('Single sign-on')
    expect(message).toContain('Enterprise')
    expect(message).not.toContain('read-only')
  })

  it('says which plan a capability needs', () => {
    expect(
      licenseRefusedMessage({
        feature: 'rbac',
        requiredTier: 'Studio',
        message: 'server prose',
        docs: ''
      })
    ).toBe('Custom permissions requires a Notifuse Studio licence.')
  })

  it('says existing workspaces are unaffected when the quota is reached', () => {
    const message = licenseRefusedMessage({
      feature: 'workspaces',
      requiredTier: null,
      message: 'server prose',
      docs: ''
    })

    expect(message).toContain('unaffected')
  })

  // A backend newer than this bundle. Inventing a name for a gate we cannot name is worse than
  // showing the English sentence the server already wrote.
  it('falls back to the server sentence for a gate it has no label for', () => {
    expect(
      licenseRefusedMessage({
        feature: 'teleportation',
        requiredTier: 'Enterprise',
        message: 'Teleportation requires a Notifuse licence.',
        docs: ''
      })
    ).toBe('Teleportation requires a Notifuse licence.')
  })

  it('labels every gate the backend can name, never the raw token', () => {
    for (const [feature, descriptor] of Object.entries(LICENSE_FEATURE_LABELS)) {
      expect(descriptor).toBeDefined()
      expect(
        licenseRefusedMessage({ feature, requiredTier: null, message: '', docs: '' })
      ).not.toBe('')
    }
  })
})
