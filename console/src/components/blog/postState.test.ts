import { describe, it, expect } from 'vitest'
import { postState, postPath } from './postState'
import type { BlogPost } from '../../services/api/blog'

const now = new Date('2026-11-12T12:00:00Z')
const post = (published_at: string | null) => ({ id: 'p', slug: 'hello', published_at }) as BlogPost

describe('postState', () => {
  it('is a draft without a publication date', () => {
    expect(postState(post(null), now)).toBe('draft')
  })

  it('is scheduled while its date is still to come', () => {
    expect(postState(post('2026-11-13T09:00:00Z'), now)).toBe('scheduled')
  })

  it('is published from its date on', () => {
    expect(postState(post('2026-11-12T12:00:00Z'), now)).toBe('published')
    expect(postState(post('2026-10-01T09:00:00Z'), now)).toBe('published')
  })
})

describe('postPath', () => {
  it('is the category slug then the post slug, as the public blog serves it', () => {
    expect(postPath('news', 'hello')).toBe('/news/hello')
  })
})
