import { describe, it, expect, vi } from 'vitest'
import { act, render, waitFor } from '@testing-library/react'
import { TiptapRichEditor } from './TiptapRichEditor'
import { TiptapInlineEditor } from './TiptapInlineEditor'

const handlerAttributes = (root: HTMLElement) =>
  Array.from(root.querySelectorAll('*')).flatMap((element) =>
    element.getAttributeNames().filter((name) => name.toLowerCase().startsWith('on'))
  )

const hrefs = (root: HTMLElement) =>
  Array.from(root.querySelectorAll('a')).map((a) => a.getAttribute('href'))

const markupLikeTargets = [
  'https://x.example/?q=a]>b',
  'https://x.example/{{ campaign | append: "-->" }}'
]
const linksWithMarkupLikeTargets =
  `<a href="${markupLikeTargets[0]}">a</a> <a href='${markupLikeTargets[1]}'>b</a>`

const editorRoot = (container: HTMLElement) => {
  const root = container.querySelector<HTMLElement>('.ProseMirror')
  expect(root).not.toBeNull()
  return root as HTMLElement
}

describe('TiptapRichEditor', () => {
  it('loads block HTML without handlers or script links', async () => {
    const onChange = vi.fn()
    const content =
      '<p style="text-align: center" onmouseover="window.__probe = 1">' +
      'Hi {{ contact.first_name }} ' +
      '<img src="x" onerror="window.__probe = 1">' +
      '<a href="javascript:window.__probe = 1">bad</a> ' +
      '<a href="{{ cta_url }}">good</a> ' +
      '<a href="webcal://example.com/cal.ics">calendar</a></p>'
    const { container } = render(<TiptapRichEditor content={content} onChange={onChange} />)

    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(root.textContent).toContain('Hi {{ contact.first_name }}'))
    expect(handlerAttributes(root)).toEqual([])
    expect(hrefs(root)).toEqual(['{{ cta_url }}', 'webcal://example.com/cal.ics'])
    expect(onChange).not.toHaveBeenCalled()
  })

  it('loads content it is given later the same way', async () => {
    const onChange = vi.fn()
    const { container, rerender } = render(
      <TiptapRichEditor content="<p>First</p>" onChange={onChange} />
    )
    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(root.textContent).toBe('First'))

    rerender(
      <TiptapRichEditor
        content={
          '<p onmouseover="window.__probe = 1">Second ' +
          '<a href="javascript:window.__probe = 1">bad</a></p>'
        }
        onChange={onChange}
      />
    )
    await waitFor(() => expect(root.textContent).toContain('Second'))
    expect(handlerAttributes(root)).toEqual([])
    expect(hrefs(root)).toEqual([])
    expect(onChange).not.toHaveBeenCalledWith(expect.stringContaining('Second'))
  })

  it('keeps link targets that contain ]> or -->, as loaded and as given later', async () => {
    const content = `<p>${linksWithMarkupLikeTargets}</p>`
    const onChange = vi.fn()
    const { container, rerender } = render(
      <TiptapRichEditor content={content} onChange={onChange} />
    )
    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(hrefs(root)).toEqual(markupLikeTargets))

    rerender(<TiptapRichEditor content="<p>Other</p>" onChange={onChange} />)
    await waitFor(() => expect(root.textContent).toBe('Other'))
    rerender(<TiptapRichEditor content={content} onChange={onChange} />)
    await waitFor(() => expect(hrefs(root)).toEqual(markupLikeTargets))
  })
})

describe('TiptapInlineEditor', () => {
  it.each([
    [
      'wrapped',
      '<span data-inline-doc="" onmouseover="window.__probe = 1" onfocus="window.__probe = 1">' +
        'Buy <span style="color: red" onclick="window.__probe = 1">now</span> ' +
        '<a href="javascript:window.__probe = 1">x</a></span>'
    ],
    [
      'block',
      '<p onmouseover="window.__probe = 1">Buy <span style="color: red">now</span> ' +
        '<span><img src="x" onerror="window.__probe = 1"></span>' +
        '<a href=" JAVASCRIPT:window.__probe = 1">x</a></p>'
    ]
  ])('loads %s button HTML without handlers or script links', async (_shape, content) => {
    const onChange = vi.fn()
    const { container } = render(<TiptapInlineEditor content={content} onChange={onChange} />)

    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(root.textContent).toContain('Buy now'))
    expect(handlerAttributes(root)).toEqual([])
    expect(hrefs(root).filter((href) => href?.toLowerCase().includes('javascript'))).toEqual([])
    expect(root.querySelector('span[style*="color: red"]')).not.toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('loads content it is given later the same way', async () => {
    const onChange = vi.fn()
    const { container, rerender } = render(
      <TiptapInlineEditor content="First" onChange={onChange} />
    )
    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(root.textContent).toBe('First'))

    rerender(
      <TiptapInlineEditor
        content={
          '<span style="color: red" onclick="window.__probe = 1">Second</span> ' +
          '<img src="x" onerror="window.__probe = 1">'
        }
        onChange={onChange}
      />
    )
    await waitFor(() => expect(root.textContent).toContain('Second'))
    expect(handlerAttributes(root)).toEqual([])
    expect(root.querySelector('span[style*="color: red"]')).not.toBeNull()
    expect(onChange).not.toHaveBeenCalledWith(expect.stringContaining('Second'))
  })

  it('keeps link targets that contain ]> or -->, as loaded and as given later', async () => {
    const content = `<p>Buy ${linksWithMarkupLikeTargets}</p>`
    const onChange = vi.fn()
    const { container, rerender } = render(
      <TiptapInlineEditor content={content} onChange={onChange} />
    )
    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(hrefs(root)).toEqual(markupLikeTargets))

    rerender(<TiptapInlineEditor content="Other" onChange={onChange} />)
    await waitFor(() => expect(root.textContent).toBe('Other'))
    rerender(<TiptapInlineEditor content={content} onChange={onChange} />)
    await waitFor(() => expect(hrefs(root)).toEqual(markupLikeTargets))
  })
})

describe('editing what was loaded', () => {
  it('saves styled paragraphs of a Text block apart, with their style', async () => {
    const onChange = vi.fn()
    const { container } = render(
      <TiptapRichEditor
        content={'<p style="text-align: center">First</p><p style="text-align: center">Second</p>'}
        onChange={onChange}
      />
    )
    const root = await waitFor(() => editorRoot(container)) as HTMLElement & {
      editor: import('@tiptap/react').Editor
    }
    act(() => {
      root.editor.commands.insertContentAt(root.editor.state.doc.content.size - 1, '!')
    })
    expect(onChange).toHaveBeenLastCalledWith(
      '<p style="text-align: center">First</p><p style="text-align: center">Second!</p>'
    )
  })

  it('shows the text of a button held in nested blocks once', async () => {
    const { container } = render(
      <TiptapInlineEditor content="<div><p>Order now</p></div>" onChange={vi.fn()} />
    )
    const root = await waitFor(() => editorRoot(container))
    await waitFor(() => expect(root.textContent).toBe('Order now'))
  })

  it('keeps the images and line breaks of a button', async () => {
    const onChange = vi.fn()
    const { container } = render(
      <TiptapInlineEditor content={'<img src="https://x.example/i.png" alt="icon"> Buy<br>now'} onChange={onChange} />
    )
    const root = (await waitFor(() => editorRoot(container))) as HTMLElement & {
      editor: import('@tiptap/react').Editor
    }
    act(() => {
      root.editor.commands.insertContentAt(root.editor.state.doc.content.size, '!')
    })
    expect(onChange).toHaveBeenLastCalledWith('<img src="https://x.example/i.png" alt="icon"> Buy<br>now!')
  })
})
