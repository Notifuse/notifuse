import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Preview } from './Preview'

describe('Preview', () => {
  it('shows the email in a frame that runs no script, in an origin of its own', () => {
    render(
      <Preview
        html="<p>Hello</p>"
        mjml="<mjml><mj-body></mj-body></mjml>"
        onTestDataChange={vi.fn()}
      />
    )

    const frame = screen.getByTitle('Email Preview')
    expect(frame.tagName).toBe('IFRAME')
    expect(frame.getAttribute('srcdoc')).toBe('<base target="_blank"><p>Hello</p>')
    // Links in the email still open in a new tab; nothing else is allowed.
    expect(frame.getAttribute('sandbox')).toBe('allow-popups allow-popups-to-escape-sandbox')
  })

  it('opens every link of the email in a new tab, as a mail client does', () => {
    const html =
      '<!doctype html><html><head><title>x</title></head>' +
      '<body><a href="https://example.com">x</a></body></html>'
    render(<Preview html={html} mjml="<mjml></mjml>" onTestDataChange={vi.fn()} />)

    const srcdoc = screen.getByTitle('Email Preview').getAttribute('srcdoc') ?? ''
    expect(srcdoc).toBe('<!doctype html><base target="_blank">' + html.slice(15))
    const doc = new DOMParser().parseFromString(srcdoc, 'text/html')
    expect(doc.compatMode).toBe('CSS1Compat')
    expect(doc.querySelector('head > base')?.getAttribute('target')).toBe('_blank')
  })

  it('says how many blocks the test data hides', () => {
    const { rerender } = render(
      <Preview
        html="<p>x</p>"
        mjml="<mjml></mjml>"
        onTestDataChange={vi.fn()}
        hiddenBlockCount={0}
      />
    )
    expect(screen.queryByTestId('preview-hidden-blocks')).toBeNull()

    rerender(
      <Preview
        html="<p>x</p>"
        mjml="<mjml></mjml>"
        onTestDataChange={vi.fn()}
        hiddenBlockCount={1}
      />
    )
    expect(screen.getByText('1 block hidden by its condition with this test data.')).toBeTruthy()

    rerender(
      <Preview
        html="<p>x</p>"
        mjml="<mjml></mjml>"
        onTestDataChange={vi.fn()}
        hiddenBlockCount={3}
      />
    )
    expect(screen.getByText('3 blocks hidden by their condition with this test data.')).toBeTruthy()
  })

  it('lists the headings that skip a level, and nothing when none does', () => {
    const { rerender } = render(
      <Preview
        html="<h1>Title</h1><h2>Section</h2><h4>Quota details</h4><h2>Next</h2>"
        mjml="<mjml></mjml>"
        onTestDataChange={vi.fn()}
      />
    )
    const box = screen.getByTestId('preview-heading-skips')
    expect(box).toHaveTextContent('Heading structure')
    expect(box).toHaveTextContent('“Quota details”: H4 after H2 (H3 expected)')

    rerender(<Preview html="<h1>Title</h1><h2>Section</h2><h3>Detail</h3>" mjml="<mjml></mjml>" onTestDataChange={vi.fn()} />)
    expect(screen.queryByTestId('preview-heading-skips')).toBeNull()
  })
})
