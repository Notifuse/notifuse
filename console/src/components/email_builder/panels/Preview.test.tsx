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
    expect(frame.getAttribute('srcdoc')).toBe('<p>Hello</p>')
    // Links in the email still open in a new tab; nothing else is allowed.
    expect(frame.getAttribute('sandbox')).toBe('allow-popups allow-popups-to-escape-sandbox')
  })
})
