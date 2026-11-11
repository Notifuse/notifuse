import { describe, it, expect, vi, afterEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { BlockThumb } from './BlockThumb'
import { headKeyOf } from '../utils/library'
import { b, email, text } from '../utils/libraryTestTrees'

const section = b('s', 'mj-section', [b('c', 'mj-column', [text('t', '<p>Spring sale</p>')])])
const tree = email([section])

const renderThumb = () =>
  render(<BlockThumb block={section} emailTree={tree} headKey={headKeyOf(tree)} width={240} />)

describe('BlockThumb', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("renders again when the email's head changes", () => {
    const withColor = (color: string) =>
      email(
        [section],
        [
          b('attrs', 'mj-attributes', [
            b('text-defaults', 'mj-text', undefined, { attributes: { color } } as never)
          ])
        ]
      )
    const colorShown = () => {
      let element: HTMLElement | null = screen.getByText('Spring sale')
      while (element && !element.style.color) element = element.parentElement
      return element?.style.color
    }
    const red = withColor('#ff0000')
    const { rerender } = render(
      <BlockThumb block={section} emailTree={red} headKey={headKeyOf(red)} width={240} />
    )
    expect(colorShown()).toBe('rgb(255, 0, 0)')
    const green = withColor('#00ff00')
    rerender(
      <BlockThumb block={section} emailTree={green} headKey={headKeyOf(green)} width={240} />
    )
    expect(colorShown()).toBe('rgb(0, 255, 0)')
  })

  it("renders the block's content", () => {
    renderThumb()
    expect(screen.getByText('Spring sale')).toBeInTheDocument()
  })

  it('is inert, hidden from assistive technology and never focusable', () => {
    renderThumb()
    const thumb = screen.getByTestId('block-thumb')
    expect(thumb).toHaveAttribute('inert')
    expect(thumb).toHaveAttribute('aria-hidden', 'true')
    expect(thumb.style.pointerEvents).toBe('none')
  })

  it("renders with ids of its own, so the canvas's block ids stay unique", () => {
    renderThumb()
    const ids = Array.from(
      screen.getByTestId('block-thumb').querySelectorAll('[data-block-id]')
    ).map((el) => el.getAttribute('data-block-id'))
    expect(ids.length).toBeGreaterThan(0)
    expect(ids).not.toContain('s')
    expect(ids).not.toContain('t')
  })

  it('shows a placeholder until the thumbnail comes near the viewport', () => {
    let callback: IntersectionObserverCallback = () => {}
    class StubObserver {
      constructor(cb: IntersectionObserverCallback) {
        callback = cb
      }
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal('IntersectionObserver', StubObserver)

    renderThumb()
    expect(screen.getByTestId('block-thumb-placeholder')).toBeInTheDocument()
    expect(screen.queryByText('Spring sale')).not.toBeInTheDocument()

    act(() =>
      callback([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver)
    )
    expect(screen.getByText('Spring sale')).toBeInTheDocument()
  })

  it('renders at once where IntersectionObserver does not exist', () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    renderThumb()
    expect(screen.queryByTestId('block-thumb-placeholder')).not.toBeInTheDocument()
    expect(screen.getByText('Spring sale')).toBeInTheDocument()
  })
})
