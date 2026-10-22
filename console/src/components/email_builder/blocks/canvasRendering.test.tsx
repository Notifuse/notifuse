import { describe, it, expect, vi } from 'vitest'
import { render, createEvent, fireEvent, waitFor } from '@testing-library/react'
import { MjTextBlock } from './MjTextBlock'
import { MjButtonBlock } from './MjButtonBlock'
import { MjImageBlock } from './MjImageBlock'
import { MjSocialElementBlock } from './MjSocialElementBlock'
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
    expect(contained?.closest(`[data-block-id="b-${type}"]`)).not.toBeNull()
    const toolbar = container.querySelector('.tiptap-toolbar')
    expect(toolbar).not.toBeNull()
    expect(toolbar?.closest('[style*="contain"]')).toBeNull()
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
  })
})
