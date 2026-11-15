import { describe, it, expect, afterEach } from 'vitest'
import { createBrowserHistory } from '@tanstack/history'

// ContactsPage writes a pending search (replace) and opens the sheet (push) in one click. This pins
// the browser history behaviour its flush between the two relies on.
const popped = () => new Promise((resolve) => window.addEventListener('popstate', resolve, { once: true }))
const microtask = () => new Promise<void>((resolve) => queueMicrotask(resolve))

describe('the browser history of the router', () => {
  afterEach(() => window.history.replaceState(null, '', '/'))

  it('merges a replace and a push of the same tick into one push', async () => {
    window.history.replaceState(null, '', '/contacts')
    const history = createBrowserHistory()
    history.replace('/contacts?search=bob')
    history.push('/contacts?search=bob&contact=bob')
    await microtask()
    const back = popped()
    window.history.back()
    await back
    // The entry under the sheet lost the search
    expect(window.location.search).toBe('')
    history.destroy()
  })

  it('keeps the replace when it is flushed before the push', async () => {
    window.history.replaceState(null, '', '/contacts')
    const history = createBrowserHistory()
    history.replace('/contacts?search=bob')
    history.flush()
    history.push('/contacts?search=bob&contact=bob')
    await microtask()
    const back = popped()
    window.history.back()
    await back
    expect(window.location.search).toBe('?search=bob')
    history.destroy()
  })
})
