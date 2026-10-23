import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { NotifuseEditor, type NotifuseEditorRef } from './NotifuseEditor'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('NotifuseEditor getHTML', () => {
  it('highlights code blocks away from the page, keeping their text as text', async () => {
    const ref = React.createRef<NotifuseEditorRef>()
    render(
      <NotifuseEditor
        ref={ref}
        initialContent={
          '<pre><code class="language-javascript">' +
          'const a = "&lt;img src=x onerror=window.__probe=1&gt;"</code></pre>'
        }
      />
    )
    await waitFor(() => expect(ref.current?.editor).toBeTruthy())
    const editor = ref.current?.editor
    const createElement = vi.spyOn(document, 'createElement')

    // The editor's own serialiser builds its nodes in the page...
    editor?.getHTML()
    const serialiserNodes = createElement.mock.calls.map(([tag]) => tag)
    createElement.mockClear()

    // ...and highlighting adds none of its own
    const html = ref.current?.getHTML() ?? ''
    expect(createElement.mock.calls.map(([tag]) => tag)).toEqual(serialiserNodes)
    expect(html).toContain('<span class="hljs-keyword">const</span>')
    expect(html).toContain('&lt;img src=x onerror=window.__probe=1&gt;')
    expect(html).not.toContain('<img')
  })
})
