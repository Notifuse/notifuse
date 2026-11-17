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
// notification password_reset of workspace ws_123 against https://api.example.test. It has since gained the
// email_options.sender_id option.
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
  it.each(Object.entries(GENERATORS))('writes the %s sample as the fixture holds it', (key, generate) => {
    expect(generate({ id: 'password_reset' }, 'ws_123')).toBe(captured[key as keyof typeof captured])
  })

  it.each(Object.entries(GENERATORS).filter(([key]) => key !== 'smtp'))(
    'shows the email_options sender_id option in the %s sample',
    (_, generate) => {
      expect(generate({ id: 'order_shipped' }, 'ws_other')).toMatch(/sender_id/)
    }
  )

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

describe('transactionalApiSnippets with the template data', () => {
  const data = {
    order: { number: 'A-1042', total: 99.5, gift: false, note: null, items: ['book', 'pen'] },
    'first-name': "Jane O'Neil"
  }
  const notification = { id: 'order_shipped', data }

  it('sends the data as JSON in the SMTP payload', () => {
    const payload = JSON.parse(smtpPayloadSnippet(notification, 'ws'))
    expect(payload.notification.data).toEqual(data)
  })

  it('sends the data as JSON in the cURL body, with its quote escaped for the shell', () => {
    const code = curlSnippet(notification, 'ws')
    // The body runs from -d ' to the last quote; the shell joins '\'' back into one quote.
    const body = code.slice(code.indexOf("-d '") + 4, code.lastIndexOf("'")).replace(/'\\''/g, "'")
    expect(JSON.parse(body).notification.data).toEqual(data)
    expect(code).not.toContain('your_template_variable')
  })

  it('writes the data as a TypeScript object, quoting only the keys that need it', () => {
    const code = typeScriptSnippet(notification, 'ws')
    expect(code).toContain('      data: {\n        order: {\n          number: "A-1042",')
    expect(code).toContain('"first-name": "Jane O\'Neil"')
    expect(code).toContain('note: null')
    expect(code).not.toContain('your_template_variable')
  })

  it('writes the data as a Python dict', () => {
    const code = pythonSnippet(notification, 'ws')
    expect(code).toContain('            "data": {\n                "order": {\n                    "number": "A-1042",')
    expect(code).toContain('"gift": False')
    expect(code).toContain('"note": None')
    expect(code).not.toContain('your_template_variable')
  })

  it('writes a number JSON cannot hold as None in Python and null elsewhere', () => {
    const odd = { id: 'x', data: { ratio: Number.NaN, limit: Number.POSITIVE_INFINITY } }
    const python = pythonSnippet(odd, 'ws')
    expect(python).toContain('"ratio": None')
    expect(python).toContain('"limit": None')
    expect(python).not.toMatch(/: null\b/)
    expect(typeScriptSnippet(odd, 'ws')).toContain('ratio: null')
    expect(JSON.parse(smtpPayloadSnippet(odd, 'ws')).notification.data).toEqual({ ratio: null, limit: null })
  })

  it('writes empty data as an empty object', () => {
    expect(JSON.parse(smtpPayloadSnippet({ id: 'x', data: {} }, 'ws')).notification.data).toEqual({})
  })

  it.each([
    ['golang', goSnippet],
    ['java', javaSnippet]
  ])('keeps the example data in the %s sample', (_, generate) => {
    expect(generate(notification, 'ws')).toContain('your_template_variable')
  })
})
