import { describe, it, expect, vi } from 'vitest'
import { render, createEvent, fireEvent } from '@testing-library/react'
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

describe('a link click on the editor canvas selects the block instead of following the link', () => {
  it.each([
    ['mj-text', MjTextBlock],
    ['mj-button', MjButtonBlock]
  ])('%s', (type, Block) => {
    const block = {
      id: `b-${type}`,
      type,
      attributes: {},
      content: '<p>Buy <a href="https://example.com/x"><strong>now</strong></a></p>'
    } as unknown as EmailBlock
    const props = previewProps()
    const { container } = render(<>{new Block(block).getEdit(props)}</>)

    const target = container.querySelector('a strong') as HTMLElement
    const click = createEvent.click(target)
    fireEvent(target, click)

    expect(click.defaultPrevented).toBe(true)
    expect(props.onSelectBlock).toHaveBeenCalledWith(`b-${type}`)
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
