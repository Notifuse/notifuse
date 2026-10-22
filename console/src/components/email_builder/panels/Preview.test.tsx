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
})
