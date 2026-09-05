import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// The router graph would come in through services/api/client.ts otherwise; llm.ts itself
// deliberately does not import it, which is the whole reason it calls fetch directly.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))

import { llmApi } from './llm'

/**
 * The assistant streams, so it cannot go through handleResponse — and it therefore had none
 * of what handleResponse does to an error body.
 *
 * `errorData.error` is whatever the server put in the field. For a licence refusal that is
 * literally the wire code `license_required`, and useAIAssistant renders the thrown message
 * into a persistent chat bubble. So the one place a paid capability is refused inside the
 * assistant showed the user a machine string.
 */
describe('llmApi.streamChat error handling', () => {
  const params = {
    workspace_id: 'ws1',
    integration_id: 'int1',
    messages: [{ role: 'user' as const, content: 'hello' }]
  }

  // streamChat never rejects: it hands the failure to onError, because the caller is a chat
  // panel that has to keep its transcript rather than unwind. So the error is collected the
  // way useAIAssistant collects it.
  const errorFrom = async (): Promise<Error> => {
    let captured: Error | undefined
    await llmApi.streamChat(
      params,
      () => {},
      (e) => {
        captured = e
      }
    )
    if (!captured) throw new Error('streamChat reported no error')
    return captured
  }

  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const failWith = (status: number, body: unknown) =>
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status,
        json: async () => body
      })
    )

  it('names the capability instead of throwing the wire code', async () => {
    failWith(402, {
      error: 'license_required',
      feature: 'ses_tenant',
      required_tier: 'Studio',
      message: 'SES tenant isolation requires a Notifuse licence (Studio or above).',
      docs: 'https://notifuse.com/licence-features'
    })

    const error = await errorFrom()

    expect(error.message).toBe('SES tenant isolation requires a Notifuse Studio licence.')
    expect(error.message).not.toContain('license_required')
  })

  it('translates a permission denial too', async () => {
    failWith(403, {
      error: 'user does not have write permission on contacts',
      resource: 'contacts',
      permission: 'write'
    })

    const error = await errorFrom()

    expect(error.message).toBe('You do not have write access to Contacts.')
  })

  // Anything the console has no rewrite for keeps the server's own sentence, which is the
  // same fallback client.ts uses. Inventing a message for something we cannot name is worse
  // than showing English prose.
  it('keeps the server sentence for an ordinary failure', async () => {
    failWith(500, { error: 'the model provider is unreachable' })

    const error = await errorFrom()

    expect(error.message).toBe('the model provider is unreachable')
  })

  it('falls back to the status when the body says nothing', async () => {
    failWith(502, null)

    const error = await errorFrom()

    expect(error.message).toContain('502')
  })
})
