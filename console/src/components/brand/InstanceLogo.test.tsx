import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { InstanceLogo } from './InstanceLogo'
import {
  DEFAULT_INSTANCE_BRANDING,
  resetBrandImageFailures,
  type InstanceBranding
} from './instanceBranding'

const LOGO = 'https://cdn.example.com/logo.png'
const ICON = 'https://cdn.example.com/icon.png'

const brand = (overrides: Partial<InstanceBranding>): InstanceBranding => ({
  ...DEFAULT_INSTANCE_BRANDING,
  custom: true,
  name: 'Acme',
  favicon_src: '/console/neutral-icon.png',
  splash_logo_src: '',
  ...overrides
})

afterEach(() => {
  resetBrandImageFailures()
  delete window.INSTANCE_BRANDING
})

describe('InstanceLogo', () => {
  it('draws Notifuse in default mode', () => {
    render(<InstanceLogo variant="full" maxWidth={150} maxHeight={24} />)
    expect(screen.getByRole('img', { name: 'Notifuse' }).tagName.toLowerCase()).toBe('svg')
  })

  it('reads the identity the server resolved', () => {
    window.INSTANCE_BRANDING = brand({ logo_url: LOGO })
    render(<InstanceLogo variant="full" maxWidth={150} maxHeight={24} />)
    expect(screen.getByRole('img', { name: 'Acme' })).toHaveAttribute('src', LOGO)
  })

  // The five valid combinations of the spec's matrix, for both variants.
  it.each([
    ['name only', brand({}), 'text', 'initial'],
    ['name and logo', brand({ logo_url: LOGO }), LOGO, 'initial'],
    ['name and icon', brand({ icon_url: ICON }), 'text', ICON],
    ['name, logo and icon', brand({ logo_url: LOGO, icon_url: ICON }), LOGO, ICON]
  ])('%s', (_label, identity, full, compact) => {
    const { unmount } = render(<InstanceLogo variant="full" maxWidth={150} maxHeight={24} branding={identity} />)
    if (full === 'text') {
      expect(screen.getByText('Acme')).toBeInTheDocument()
    } else {
      expect(screen.getByRole('img', { name: 'Acme' })).toHaveAttribute('src', full)
    }
    unmount()

    render(<InstanceLogo variant="compact" maxWidth={24} maxHeight={24} branding={identity} />)
    if (compact === 'initial') {
      expect(screen.getByRole('img', { name: 'Acme' })).toHaveTextContent('A')
    } else {
      expect(screen.getByRole('img', { name: 'Acme' })).toHaveAttribute('src', compact)
    }
  })

  it('falls back to the name when the image fails, and tries a new URL again', () => {
    const { rerender } = render(
      <InstanceLogo variant="full" maxWidth={150} maxHeight={24} branding={brand({ logo_url: LOGO })} />
    )
    fireEvent.error(screen.getByRole('img', { name: 'Acme' }))
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByText('Acme')).toHaveAttribute('title', 'Acme')

    const repaired = 'https://cdn.example.com/logo-v2.png'
    rerender(<InstanceLogo variant="full" maxWidth={150} maxHeight={24} branding={brand({ logo_url: repaired })} />)
    expect(screen.getByRole('img', { name: 'Acme' })).toHaveAttribute('src', repaired)
  })

  it('falls back to the initial when the icon fails', () => {
    render(<InstanceLogo variant="compact" maxWidth={24} maxHeight={24} branding={brand({ icon_url: ICON })} />)
    fireEvent.error(screen.getByRole('img', { name: 'Acme' }))
    expect(screen.getByRole('img', { name: 'Acme' })).toHaveTextContent('A')
  })

  it('bounds the image, keeps its proportions and sends no Referer', () => {
    render(<InstanceLogo variant="full" maxWidth={150} maxHeight={24} branding={brand({ logo_url: LOGO })} />)
    const img = screen.getByRole('img', { name: 'Acme' })
    expect(img).toHaveAttribute('referrerpolicy', 'no-referrer')
    expect(img.style.maxWidth).toBe('150px')
    expect(img.style.maxHeight).toBe('24px')
    expect(img.style.objectFit).toBe('contain')
  })

  it('truncates a long name and keeps it whole for assistive technology', () => {
    const long = 'A'.repeat(80)
    render(<InstanceLogo variant="full" maxWidth={150} maxHeight={24} branding={brand({ name: long })} />)
    const text = screen.getByText(long)
    expect(text).toHaveClass('truncate')
    expect(text).toHaveClass('min-w-0')
    expect(text).toHaveAttribute('title', long)
  })
})
