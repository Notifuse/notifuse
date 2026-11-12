import { describe, it, expect } from 'vitest'
import { postViewsQuery, viewsWindow, viewsByPath } from './postViews'

describe('viewsWindow', () => {
  it('runs from 30 days ago, down to the hour, to the next hour', () => {
    expect(viewsWindow(new Date('2026-11-12T10:25:30Z'))).toEqual([
      '2026-10-13T10:00:00.000Z',
      '2026-11-12T11:00:00.000Z'
    ])
  })

  it('keeps an exact hour as the end', () => {
    expect(viewsWindow(new Date('2026-11-12T10:00:00Z'))[1]).toBe('2026-11-12T10:00:00.000Z')
  })
})

describe('postViewsQuery', () => {
  it('counts the page views of the paths in the window, a date range the partitions can use', () => {
    const query = postViewsQuery(['/news/a', '/news/b'], ['from', 'to'])
    expect(query).toEqual({
      schema: 'web_pages',
      measures: ['page_count'],
      dimensions: ['page_path'],
      filters: [
        { member: 'page_path', operator: 'in', values: ['/news/a', '/news/b'] },
        { member: 'entered_at', operator: 'inDateRange', values: ['from', 'to'] }
      ],
      limit: 2
    })
  })
})

describe('viewsByPath', () => {
  it('maps each path to its count, numbers sent as text included', () => {
    const views = viewsByPath([
      { page_path: '/news/a', page_count: 12 },
      { page_path: '/news/b', page_count: '3' }
    ])
    expect(views.get('/news/a')).toBe(12)
    expect(views.get('/news/b')).toBe(3)
    expect(views.get('/news/c')).toBeUndefined()
  })
})
