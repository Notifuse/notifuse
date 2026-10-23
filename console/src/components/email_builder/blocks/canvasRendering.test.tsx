import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, createEvent, fireEvent, waitFor, act } from '@testing-library/react'
import { MjTextBlock } from './MjTextBlock'
import { MjButtonBlock } from './MjButtonBlock'
import { MjImageBlock } from './MjImageBlock'
import { MjSocialElementBlock } from './MjSocialElementBlock'
import { MjRawBlock } from './MjRawBlock'
import type { EmailBlock } from '../types'
import type { PreviewProps } from './BaseEmailBlock'

const previewProps = (): PreviewProps =>
  ({
    selectedBlockId: null,
    onSelectBlock: vi.fn(),
    onUpdateBlock: vi.fn(),
    onCloneBlock: vi.fn(),
    onDeleteBlock: vi.fn(),
    attributeDefaults: {},
    emailTree: {} as EmailBlock,
    onSaveBlock: vi.fn()
  }) as unknown as PreviewProps

const content =
  '<p style="color: #333">Hi <strong>{{ contact.first_name }}</strong>' +
  '<img src="x" onerror="window.__probe = 1">' +
  '<a href="javascript:window.__probe = 1">x</a>' +
  '<a href="{{ cta_url }}" target="_blank">go</a></p>'

describe('editor canvas renders block HTML without handlers or script links', () => {
  it.each([
    ['mj-text', MjTextBlock],
    ['mj-button', MjButtonBlock]
  ])('%s', (type, Block) => {
    const block = { id: `b-${type}`, type, attributes: {}, content } as unknown as EmailBlock
    const props = previewProps()
    const { container } = render(<>{new Block(block).getEdit(props)}</>)

    const html = container.innerHTML
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('javascript:')
    expect(html).toContain('{{ contact.first_name }}')
    expect(html).toContain('href="{{ cta_url }}"')
    expect(html).toContain('style="color: #333"')
    // Only the rendering is cleaned: the block keeps its content as saved.
    expect((block as unknown as { content: string }).content).toBe(content)
    expect(props.onUpdateBlock).not.toHaveBeenCalled()
  })
})

describe('editor canvas keeps block content inside its block', () => {
  it.each([
    ['mj-text', MjTextBlock],
    ['mj-button', MjButtonBlock]
  ])('%s', (type, Block) => {
    const overlay =
      '<p>Hi <span data-overlay="" style="position: fixed; inset: 0; z-index: 2147483647">x</span></p>'
    const block = {
      id: `b-${type}`,
      type,
      attributes: { height: '40px', width: '200px' },
      content: overlay
    } as unknown as EmailBlock
    const { container } = render(<>{new Block(block).getEdit(previewProps())}</>)

    // Paint containment clips the content to the block and makes the block the
    // box a fixed element is laid out against, so nothing covers the console
    const contained = container.querySelector('[data-overlay]')?.closest('[style*="contain"]')
    expect((contained as HTMLElement | null)?.style.contain).toBe('paint')
    expect(contained?.closest(`[data-block-id="b-${type}"]`)).not.toBeNull()
    // What the content draws just past its box, a shadow or a list marker, still
    // shows in the block's padding, as in the email
    expect((contained as HTMLElement).style.overflowClipMargin).toBe('25px')
    // It sizes to its content, so a block given a fixed size still shows all of
    // its text, as the email does
    expect((contained as HTMLElement).style.height).toBe('')
    expect((contained as HTMLElement).style.width).toBe('')
  })

  it.each([
    ['mj-text', MjTextBlock],
    ['mj-button', MjButtonBlock]
  ])('%s open in its editor: contains the editing area, not the toolbar', async (type, Block) => {
    const block = { id: `b-${type}`, type, attributes: {}, content } as unknown as EmailBlock
    const props = { ...previewProps(), selectedBlockId: `b-${type}` } as PreviewProps
    const { container } = render(<>{new Block(block).getEdit(props)}</>)

    const editing = await waitFor(() => {
      const element = container.querySelector('.ProseMirror')
      expect(element).not.toBeNull()
      return element as HTMLElement
    })
    const contained = editing.closest('[style*="contain"]') as HTMLElement | null
    expect(contained?.style.contain).toBe('paint')
    expect(contained?.style.overflowClipMargin).toBe('25px')
    expect(contained?.closest(`[data-block-id="b-${type}"]`)).not.toBeNull()
    const toolbar = container.querySelector('.tiptap-toolbar')
    expect(toolbar).not.toBeNull()
    expect(toolbar?.closest('[style*="contain"]')).toBeNull()
  })
})

describe('editor canvas button text', () => {
  it('carries the button text decoration into its contained box', () => {
    const block = {
      id: 'b-decorated',
      type: 'mj-button',
      attributes: { textDecoration: 'underline' },
      content: '<p>Buy now</p>'
    } as unknown as EmailBlock
    const { container } = render(<>{new MjButtonBlock(block).getEdit(previewProps())}</>)

    // An inline-block takes no decoration from its parent, so it carries the
    // button's own
    const text = container.querySelector('[style*="contain"]') as HTMLElement | null
    expect(text?.style.display).toBe('inline-block')
    expect(text?.style.textDecoration).toBe('inherit')
    expect((text?.parentElement as HTMLElement).style.textDecoration).toBe('underline')
  })
})

describe('a click on editor canvas content only selects the block', () => {
  // A link, or a label that would click a console field of the same id
  it.each([
    ['mj-text', 'a strong', MjTextBlock],
    ['mj-button', 'a strong', MjButtonBlock],
    ['mj-text', 'label', MjTextBlock],
    ['mj-button', 'label', MjButtonBlock]
  ])('%s, click on %s', (type, selector, Block) => {
    const block = {
      id: `b-${type}`,
      type,
      attributes: {},
      content:
        '<p>Buy <a href="https://example.com/x"><strong>now</strong></a> ' +
        '<label for="email_sender_id">here</label></p>'
    } as unknown as EmailBlock
    const props = previewProps()
    const { container } = render(<>{new Block(block).getEdit(props)}</>)

    const target = container.querySelector(selector) as HTMLElement
    const click = createEvent.click(target)
    fireEvent(target, click)

    expect(click.defaultPrevented).toBe(true)
    expect(props.onSelectBlock).toHaveBeenCalledWith(`b-${type}`)
  })
})

describe('a middle click on an editor canvas link opens nothing', () => {
  it.each([
    ['mj-text', MjTextBlock],
    ['mj-button', MjButtonBlock],
    ['mj-image', MjImageBlock],
    ['mj-social-element', MjSocialElementBlock]
  ])('%s', (type, Block) => {
    const href = 'https://example.com/x'
    const block = {
      id: `b-${type}`,
      type,
      attributes: { href, src: 'https://example.com/i.png', name: 'facebook' },
      content: type === 'mj-image' ? undefined : `<p>Buy <a href="${href}">now</a></p>`
    } as unknown as EmailBlock
    const { container } = render(<>{new Block(block).getEdit(previewProps())}</>)

    const links = Array.from(container.querySelectorAll<HTMLElement>(`a[href="${href}"]`))
    expect(links).not.toEqual([])
    for (const link of links) {
      const middleClick = new MouseEvent('auxclick', { bubbles: true, cancelable: true, button: 1 })
      link.dispatchEvent(middleClick)
      expect(middleClick.defaultPrevented).toBe(true)
    }
  })
})

describe('editor canvas links keep only navigable targets', () => {
  const renderLink = (type: 'mj-image' | 'mj-social-element', href: string) => {
    const block = {
      id: `b-${type}`,
      type,
      attributes: { href, src: 'https://example.com/i.png', name: 'facebook' },
      content: type === 'mj-social-element' ? 'Follow' : undefined
    } as unknown as EmailBlock
    const Block = type === 'mj-image' ? MjImageBlock : MjSocialElementBlock
    const { container, unmount } = render(<>{new Block(block).getEdit(previewProps())}</>)
    const hrefs = Array.from(container.querySelectorAll('a')).map((a) => a.getAttribute('href'))
    unmount()
    return hrefs
  }

  it.each(['mj-image', 'mj-social-element'] as const)('%s', (type) => {
    const unsafe = renderLink(type, 'javascript:window.__probe = 1')
    expect(unsafe.length).toBeGreaterThan(0)
    expect(unsafe.filter((href) => href?.includes('javascript'))).toEqual([])

    expect(renderLink(type, 'https://example.com/a')).toContain('https://example.com/a')
    expect(renderLink(type, '{{ profile_url }}')).toContain('{{ profile_url }}')
    // A link that would hand the member's click to an app on their computer
    expect(renderLink(type, 'search-ms:query=invoice')).not.toContain('search-ms:query=invoice')
  })
})

describe('editor canvas raw HTML', () => {
  it.each([
    ['not selected', null],
    ['selected', 'b-raw']
  ])('shows it, %s, in a frame that opens no window and takes no click or key', (_, selected) => {
    const block = {
      id: 'b-raw',
      type: 'mj-raw',
      attributes: {},
      content: '<a href="https://example.com" target="_blank">x</a>'
    } as unknown as EmailBlock
    const props = { ...previewProps(), selectedBlockId: selected } as PreviewProps
    const { container } = render(<>{new MjRawBlock(block).getEdit(props)}</>)
    const iframe = container.querySelector('iframe')

    // Same origin only, for the canvas to size the frame to its content
    expect(iframe?.getAttribute('sandbox')).toBe('allow-same-origin')
    // Inert: no pointer or key reaches the frame, or a frame within it
    expect(iframe).toHaveAttribute('inert', '')
  })
})

describe('editor canvas raw HTML frame document', () => {
  const rawBlock = {
    id: 'b-raw',
    type: 'mj-raw',
    attributes: {},
    content: '<p>Raw <b>block</b></p>'
  } as unknown as EmailBlock

  // A template whose head holds these blocks, and whose body holds the raw block
  const treeWith = (head: object[]) =>
    ({
      id: 'mjml',
      type: 'mjml',
      attributes: {},
      children: [
        { id: 'head', type: 'mj-head', attributes: {}, children: head },
        { id: 'body', type: 'mj-body', attributes: {}, children: [rawBlock] }
      ]
    }) as unknown as EmailBlock

  const font = (name: string | undefined, href: string) => ({
    id: `font-${href}`,
    type: 'mj-font',
    attributes: { name, href }
  })

  const frameSource = (head: object[]) => {
    const props = { ...previewProps(), emailTree: treeWith(head) } as PreviewProps
    const { container, unmount } = render(<>{new MjRawBlock(rawBlock).getEdit(props)}</>)
    const source = container.querySelector('iframe')?.getAttribute('srcdoc') ?? ''
    unmount()
    return source
  }
  const parse = (source: string) => new DOMParser().parseFromString(source, 'text/html')

  it('links the fonts of the template before any style, where the browser loads them', () => {
    const source = frameSource([
      font('Inter', 'https://fonts.googleapis.com/css2?family=Inter&display=swap'),
      font('Other', 'https://fonts.example.com/b.css')
    ])

    const inter =
      '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter&amp;display=swap">'
    const other = '<link rel="stylesheet" href="https://fonts.example.com/b.css">'
    expect(source).toContain(inter)
    expect(source).toContain(other)
    expect(source.indexOf(inter)).toBeLessThan(source.indexOf('<style'))
    expect(source.indexOf(other)).toBeLessThan(source.indexOf('<style'))
    // An @import after other rules is ignored by the browser
    expect(source).not.toContain('@import url(')
    expect(
      Array.from(parse(source).querySelectorAll('link[rel="stylesheet"]')).map((link) =>
        link.getAttribute('href')
      )
    ).toEqual([
      'https://fonts.googleapis.com/css2?family=Inter&display=swap',
      'https://fonts.example.com/b.css'
    ])
  })

  it.each([
    ['Liquid', '{{ font_url }}'],
    ['a script', 'javascript:alert(1)'],
    ['a relative address', '/fonts/a.css'],
    ["the console's own origin", `${window.location.origin}/fonts/a.css`]
  ])('links no font whose address is %s', (_, href) => {
    expect(frameSource([font('Inter', href)])).not.toContain('<link')
  })

  it('links no font that has no name', () => {
    expect(frameSource([font(undefined, 'https://fonts.example.com/a.css')])).not.toContain('<link')
  })

  it('keeps a font address inside its attribute', () => {
    const source = frameSource([font('Inter', 'https://fonts.example.com/a.css?x="><b id="out">')])

    expect(source).not.toContain('"><b')
    const document = parse(source)
    expect(document.querySelectorAll('link')).toHaveLength(1)
    expect(document.querySelector('link')?.getAttribute('href')).toBe(
      'https://fonts.example.com/a.css?x=%22%3E%3Cb%20id=%22out%22%3E'
    )
    expect(document.getElementById('out')).toBeNull()
  })

  it('puts the mj-style content in a style element of its own, so an @import at its top applies', () => {
    const imported = "@import url('https://fonts.example.com/c.css');"
    const source = frameSource([
      { id: 'style', type: 'mj-style', attributes: {}, content: `${imported}\n.x { color: red; }` }
    ])

    const styles = Array.from(parse(source).querySelectorAll('style')).map((style) =>
      (style.textContent ?? '').replace(/\/\*[\s\S]*?\*\//g, '').trim()
    )
    expect(styles.length).toBeGreaterThan(1)
    expect(styles[styles.length - 1].startsWith(imported)).toBe(true)
    expect(styles[styles.length - 1]).toContain('.x { color: red; }')
    // The canvas's own rules come first, so the template's still override them
    expect(styles.slice(0, -1).join('\n')).toContain('font-family: Arial')
  })

  it('links a protocol-relative font address as a web address', () => {
    expect(frameSource([font('Lato', '//fonts.googleapis.com/css?family=Lato')])).toContain(
      '<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Lato">'
    )
  })

  it("sends no Referer from the frame, for its fonts as for the content's images", () => {
    const source = frameSource([font('Inter', 'https://fonts.example.com/b.css')])

    const meta = '<meta name="referrer" content="no-referrer">'
    expect(source).toContain(meta)
    expect(source.indexOf(meta)).toBeLessThan(source.indexOf('<link'))
  })
})

describe('editor canvas raw HTML frame height', () => {
  // Records what the component observes, for a test to report a size change
  class RecordingResizeObserver {
    static last: RecordingResizeObserver | undefined
    targets: Element[] = []
    disconnected = false
    constructor(readonly callback: ResizeObserverCallback) {
      RecordingResizeObserver.last = this
    }
    observe(target: Element) {
      this.targets.push(target)
    }
    unobserve() {}
    disconnect() {
      this.disconnected = true
    }
    resize(width: number) {
      const entry = { target: this.targets[0], contentRect: { width } } as unknown as ResizeObserverEntry
      this.callback([entry], this as unknown as ResizeObserver)
    }
  }

  // jsdom lays nothing out and loads no srcdoc: the frame keeps its first document,
  // whose body height a test sets
  const stubDocument = (document: Document) => {
    const state = { height: 0, reads: 0 }
    Object.defineProperty(document.body, 'scrollHeight', {
      configurable: true,
      get: () => {
        state.reads += 1
        return state.height
      }
    })
    const fonts = new EventTarget()
    Object.defineProperty(document, 'fonts', { configurable: true, value: fonts })
    return { state, fonts }
  }

  const renderFrame = () => {
    vi.useFakeTimers()
    vi.stubGlobal('ResizeObserver', RecordingResizeObserver)
    const block = {
      id: 'b-raw',
      type: 'mj-raw',
      attributes: {},
      content: '<p>Raw</p>'
    } as unknown as EmailBlock
    const view = render(<>{new MjRawBlock(block).getEdit(previewProps())}</>)
    const iframe = view.container.querySelector('iframe') as HTMLIFrameElement
    const observer = RecordingResizeObserver.last as RecordingResizeObserver
    return { ...view, iframe, observer }
  }

  const settle = () =>
    act(() => {
      vi.advanceTimersByTime(60)
    })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('measures it again when its width changes, and only then', () => {
    const { iframe, observer, unmount } = renderFrame()
    const { state } = stubDocument(iframe.contentDocument as Document)
    expect(observer.targets).toEqual([iframe])

    state.height = 80
    act(() => observer.resize(600))
    settle()
    expect(iframe.style.height).toBe('100px')

    // Taller content at the same width: a height change is the canvas's own
    state.height = 300
    act(() => observer.resize(600))
    settle()
    expect(iframe.style.height).toBe('100px')

    // A narrower column rewraps the content
    act(() => observer.resize(300))
    settle()
    expect(iframe.style.height).toBe('310px')

    unmount()
    expect(observer.disconnected).toBe(true)
  })

  it('measures it collapsed, its block holding its height, then restores it', () => {
    const { iframe } = renderFrame()
    const parent = iframe.parentElement as HTMLElement
    Object.defineProperty(parent, 'offsetHeight', { configurable: true, get: () => 250 })
    const seen: string[][] = []
    Object.defineProperty((iframe.contentDocument as Document).body, 'scrollHeight', {
      configurable: true,
      get: () => {
        seen.push([iframe.style.height, iframe.style.minHeight, parent.style.minHeight])
        return 120
      }
    })

    act(() => {
      RecordingResizeObserver.last?.resize(600)
    })
    settle()

    // Content sized on the frame's viewport is measured without it: the measure
    // does not depend on the height the canvas gave the frame
    expect(seen).toEqual([['0px', '0px', '250px']])
    expect(iframe.style.height).toBe('130px')
    expect(iframe.style.minHeight).toBe('100px')
    expect(parent.style.minHeight).toBe('')
  })

  it('measures it again when fonts of its current document arrive', () => {
    const { iframe, unmount } = renderFrame()
    const first = stubDocument(iframe.contentDocument as Document)
    fireEvent.load(iframe)
    settle()

    first.state.height = 400
    act(() => {
      first.fonts.dispatchEvent(new Event('loadingdone'))
    })
    settle()
    expect(iframe.style.height).toBe('410px')

    // The frame loads a new document: fonts of the previous one no longer count
    const replacement = document.implementation.createHTMLDocument('')
    Object.defineProperty(iframe, 'contentDocument', { configurable: true, get: () => replacement })
    const second = stubDocument(replacement)
    second.state.height = 500
    fireEvent.load(iframe)
    settle()
    expect(iframe.style.height).toBe('510px')

    second.state.height = 600
    act(() => {
      first.fonts.dispatchEvent(new Event('loadingdone'))
    })
    settle()
    expect(iframe.style.height).toBe('510px')

    act(() => {
      second.fonts.dispatchEvent(new Event('loadingdone'))
    })
    settle()
    expect(iframe.style.height).toBe('610px')

    unmount()
    const reads = second.state.reads
    second.fonts.dispatchEvent(new Event('loadingdone'))
    vi.advanceTimersByTime(60)
    expect(second.state.reads).toBe(reads)
  })
})
