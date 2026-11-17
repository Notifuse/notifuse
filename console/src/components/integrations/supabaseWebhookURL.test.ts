import { describe, it, expect, afterEach } from 'vitest'
import { supabaseWebhookURL } from './supabaseWebhookURL'

afterEach(() => {
  window.API_ENDPOINT = ''
})

describe('supabaseWebhookURL', () => {
  it('points at the API endpoint, with the workspace and the integration', () => {
    window.API_ENDPOINT = 'https://api.acme.com'
    expect(supabaseWebhookURL('auth-email', 'acme', 'int_4c1d')).toBe(
      'https://api.acme.com/webhooks/supabase/auth-email?workspace_id=acme&integration_id=int_4c1d'
    )
    expect(supabaseWebhookURL('before-user-created', 'acme', 'int_4c1d')).toBe(
      'https://api.acme.com/webhooks/supabase/before-user-created?workspace_id=acme&integration_id=int_4c1d'
    )
  })

  it('falls back to the console origin without an API endpoint', () => {
    window.API_ENDPOINT = ''
    expect(supabaseWebhookURL('auth-email', 'acme', 'int_1')).toBe(
      `${window.location.origin}/webhooks/supabase/auth-email?workspace_id=acme&integration_id=int_1`
    )
  })
})
