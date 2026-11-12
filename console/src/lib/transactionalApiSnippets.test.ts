import { describe, expect, it, beforeEach } from 'vitest'
import captured from './__fixtures__/transactionalApiSnippets.json'
import {
  curlSnippet,
  goSnippet,
  javaSnippet,
  pythonSnippet,
  smtpPayloadSnippet,
  typeScriptSnippet
} from './transactionalApiSnippets'

// The fixture was copied from the API command modal before its generators moved here, for the
// notification password_reset of workspace ws_123 against https://api.example.test.
const GENERATORS = {
  curl: curlSnippet,
  typescript: typeScriptSnippet,
  python: pythonSnippet,
  golang: goSnippet,
  java: javaSnippet,
  smtp: smtpPayloadSnippet
}

beforeEach(() => {
  window.API_ENDPOINT = 'https://api.example.test'
})

describe('transactionalApiSnippets', () => {
  it.each(Object.entries(GENERATORS))('writes the %s sample exactly as the modal did', (key, generate) => {
    expect(generate({ id: 'password_reset' }, 'ws_123')).toBe(captured[key as keyof typeof captured])
  })

  it.each(Object.entries(GENERATORS))('puts the notification and the workspace in the %s sample', (_, generate) => {
    const code = generate({ id: 'order_shipped' }, 'ws_other')
    expect(code).toContain('order_shipped')
    expect(code).toContain('ws_other')
  })

  it.each(Object.entries(GENERATORS).filter(([key]) => key !== 'smtp'))(
    'calls the API endpoint of the console in the %s sample',
    (_, generate) => {
      window.API_ENDPOINT = 'https://notifuse.acme.test'
      expect(generate({ id: 'x' }, 'ws')).toContain('https://notifuse.acme.test/api/transactional.send')
    }
  )
})
