import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { emailColors, normaliseHex } from './emailColors'

let nextId = 0
const block = (type: string, attributes: Record<string, unknown>, children: unknown[] = [], content?: string) =>
  ({ id: `b${++nextId}`, type, attributes, children, content }) as unknown as EmailBlock

const tree = (...children: EmailBlock[]) => block('mjml', {}, [block('mj-body', {}, children)])

describe('emailColors', () => {
  it('orders by frequency, then by first appearance', () => {
    const colors = emailColors(
      tree(
        block('mj-section', { backgroundColor: '#111111' }),
        block('mj-text', { color: '#222222' }),
        block('mj-text', { color: '#333333' }),
        block('mj-button', { backgroundColor: '#333333', color: '#222222' }),
        block('mj-text', { color: '#333333' })
      )
    )
    expect(colors).toEqual(['#333333', '#222222', '#111111'])
  })

  it('counts #FFF and #ffffff as one colour, written in lowercase 6 digits', () => {
    const colors = emailColors(
      tree(block('mj-section', { backgroundColor: '#FFF' }), block('mj-text', { color: '#ffffff' }))
    )
    expect(colors).toEqual(['#ffffff'])
  })

  it('reads every colour key and the colour inside the border shorthands', () => {
    const colors = emailColors(
      tree(
        block('mj-column', {
          containerBackgroundColor: '#000001',
          innerBackgroundColor: '#000002',
          borderTop: '1px solid #000003',
          innerBorderLeft: '2px dashed #000004'
        }),
        block('mj-divider', { borderColor: '#000005' }),
        block('mj-social-element', { border: '1px solid #000006' })
      )
    )
    expect(colors).toEqual(['#000001', '#000002', '#000003', '#000004', '#000005', '#000006'])
  })

  it('cuts an opaque alpha and keeps any other one', () => {
    const colors = emailColors(
      tree(block('mj-text', { color: '#112233FF' }), block('mj-section', { backgroundColor: '#11223380' }))
    )
    expect(colors).toEqual(['#112233', '#11223380'])
  })

  it('skips transparent and values that are not hex', () => {
    const colors = emailColors(
      tree(
        block('mj-section', { backgroundColor: 'transparent', borderTop: 'none' }),
        block('mj-text', { color: 'red' }),
        block('mj-button', { backgroundColor: 'rgb(0, 0, 0)', borderBottom: '1px solid transparent' }),
        block('mj-text', { color: '{{ brand.color }}' }),
        block('mj-text', { color: '#12' })
      )
    )
    expect(colors).toEqual([])
  })

  it('does not read colours written inline in a text', () => {
    const colors = emailColors(tree(block('mj-text', {}, [], '<p style="color:#ff0000">Hi</p>')))
    expect(colors).toEqual([])
  })

  it('keeps the ten most used colours', () => {
    const texts = Array.from({ length: 12 }, (_, i) =>
      block('mj-text', { color: `#0000${i.toString(16).padStart(2, '0')}` })
    )
    // The last colour is used twice, so it leads.
    texts.push(block('mj-text', { color: '#00000b' }))
    const colors = emailColors(tree(...texts))
    expect(colors).toHaveLength(10)
    expect(colors[0]).toBe('#00000b')
    expect(colors.slice(1)).toEqual(['#000000', '#000001', '#000002', '#000003', '#000004', '#000005', '#000006', '#000007', '#000008'])
  })
})

describe('normaliseHex', () => {
  it('expands 3 digits and lowercases', () => {
    expect(normaliseHex('#AbC')).toBe('#aabbcc')
  })

  it('returns undefined for anything but a hex colour', () => {
    expect(normaliseHex('transparent')).toBeUndefined()
    expect(normaliseHex(undefined)).toBeUndefined()
    expect(normaliseHex('#abcd')).toBeUndefined()
  })
})
