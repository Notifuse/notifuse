import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { emailProviders, getProviderIcon } from './EmailProviders'

describe('the SMTP icon', () => {
  it('is a server pictogram named SMTP, not a made-up text logo', () => {
    const smtp = emailProviders.find((provider) => provider.kind === 'smtp')!
    const { container } = render(<>{smtp.getIcon('', 14)}</>)
    const svg = container.querySelector('svg')
    expect(svg).toHaveAttribute('aria-label', 'SMTP')
    expect(container.textContent).toBe('')
  })

  it('stands in for an unknown provider too', () => {
    const { container } = render(<>{getProviderIcon('something-else')}</>)
    expect(container.querySelector('svg')).not.toBeNull()
    expect(container.textContent).toBe('')
  })
})
