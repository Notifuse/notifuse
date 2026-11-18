import { afterEach, describe, it, expect, vi } from 'vitest'
import { buildCsv, downloadCsv, escapeCsvValue } from './csv'
import { downloadBlob } from './download'

vi.mock('./download', () => ({ downloadBlob: vi.fn() }))

afterEach(() => vi.clearAllMocks())

describe('escapeCsvValue', () => {
  it('leaves a plain value as it is and empties null and undefined', () => {
    expect(escapeCsvValue('anna@acme.io')).toBe('anna@acme.io')
    expect(escapeCsvValue(42)).toBe('42')
    expect(escapeCsvValue(null)).toBe('')
    expect(escapeCsvValue(undefined)).toBe('')
  })

  it('quotes a value holding a comma, a quote or a line break, doubling the quotes', () => {
    expect(escapeCsvValue('Ortiz, Javier')).toBe('"Ortiz, Javier"')
    expect(escapeCsvValue('say "hi"')).toBe('"say ""hi"""')
    expect(escapeCsvValue('two\nlines')).toBe('"two\nlines"')
    expect(escapeCsvValue('cr\rhere')).toBe('"cr\rhere"')
  })
})

describe('buildCsv', () => {
  it('writes the header row then one line per row', () => {
    expect(buildCsv(['email', 'error'], [['a@b.co', 'invalid, really']])).toBe(
      'email,error\na@b.co,"invalid, really"'
    )
  })
})

describe('downloadCsv', () => {
  it('hands the browser a UTF-8 CSV that starts with a byte order mark', async () => {
    downloadCsv('email\nà@b.co', 'failed.csv')
    expect(downloadBlob).toHaveBeenCalledTimes(1)
    const [blob, name] = vi.mocked(downloadBlob).mock.calls[0]
    expect(name).toBe('failed.csv')
    expect(blob.type).toBe('text/csv;charset=utf-8;')
    // jsdom's Blob has no arrayBuffer(): read it the way a browser page would.
    const buffer = await new Promise<ArrayBuffer>((resolve) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as ArrayBuffer)
      reader.readAsArrayBuffer(blob)
    })
    const bytes = new Uint8Array(buffer)
    expect(Array.from(bytes.slice(0, 3))).toEqual([0xef, 0xbb, 0xbf])
    expect(new TextDecoder().decode(bytes.slice(3))).toBe('email\nà@b.co')
  })
})
