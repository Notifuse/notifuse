import { describe, it, expect } from 'vitest'
import { applyBlockUpdates } from './applyBlockUpdates'
import type { EmailBlock } from '../types'

const block = {
  id: 'b',
  type: 'mj-button',
  attributes: { color: '#ffffff', href: 'https://a.test' },
  content: 'Buy'
} as unknown as EmailBlock

describe('applyBlockUpdates', () => {
  it('sets attributes and leaves the others', () => {
    const next = applyBlockUpdates(block, { color: '#000000' })
    expect(next.attributes).toEqual({ color: '#000000', href: 'https://a.test' })
  })

  it('removes an attribute set to undefined', () => {
    const next = applyBlockUpdates(block, { href: undefined })
    expect(next.attributes).toEqual({ color: '#ffffff' })
    expect('href' in (next.attributes as Record<string, unknown>)).toBe(false)
  })

  it('writes content on the block, not in its attributes', () => {
    const next = applyBlockUpdates(block, { content: 'Shop' })
    expect((next as { content?: string }).content).toBe('Shop')
    expect('content' in (next.attributes as Record<string, unknown>)).toBe(false)
  })

  it('creates the attributes of a block that has none', () => {
    const bare = { id: 'b', type: 'mj-spacer' } as unknown as EmailBlock
    expect(applyBlockUpdates(bare, { height: '20px' }).attributes).toEqual({ height: '20px' })
  })

  it('does not change the block it was given', () => {
    applyBlockUpdates(block, { color: '#000000', href: undefined })
    expect(block.attributes).toEqual({ color: '#ffffff', href: 'https://a.test' })
  })
})
