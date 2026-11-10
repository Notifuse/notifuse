import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TextVersionPanel } from './TextVersionPanel'

describe('TextVersionPanel', () => {
  it('shows the text verbatim, keeping line breaks and indentation', () => {
    const text = 'Hello Camille\n\n- One\n  - Sub\n\nShop (https://example.com/shop)\n'
    render(<TextVersionPanel text={text} />)

    expect(screen.getByTestId('text-version').textContent).toBe(text)
  })

  it('says the email goes out as HTML only when there is no text', () => {
    render(<TextVersionPanel text="" />)

    expect(screen.getByText('No text version: this email will be sent as HTML only.')).toBeTruthy()
    expect(screen.queryByTestId('text-version')).toBeNull()
  })

  it('shows the compile error instead of a text', () => {
    render(<TextVersionPanel text="stale" errors={[{ message: 'mj-text: unclosed tag' }]} />)

    expect(screen.getByText('mj-text: unclosed tag')).toBeTruthy()
    expect(screen.queryByTestId('text-version')).toBeNull()
  })
})
