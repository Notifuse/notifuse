import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { MainLayout } from './MainLayout'
import { DEFAULT_INSTANCE_BRANDING, resetBrandImageFailures } from '../components/brand/instanceBranding'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const background = 'https://cdn.example.com/bg.jpg?sig=a(b)c'

function renderLayout() {
  return render(
    <I18nProvider i18n={i18n}>
      <MainLayout>
        <p>Sign in form</p>
      </MainLayout>
    </I18nProvider>
  )
}

function layoutElement(container: HTMLElement): HTMLElement {
  return container.querySelector('.ant-layout') as HTMLElement
}

afterEach(() => {
  delete window.INSTANCE_BRANDING
  resetBrandImageFailures()
})

describe('MainLayout background', () => {
  it('shows the Notifuse photo and its credit by default', () => {
    const { container } = renderLayout()

    expect(layoutElement(container).style.backgroundImage).toContain('signin-background')
    expect(screen.getByRole('link', { name: 'Photo by Sean Oulashin' })).toBeInTheDocument()
    expect(screen.queryByTestId('custom-background')).toBeNull()
    expect(screen.getByText('Sign in form')).toBeInTheDocument()
  })

  it('shows the same photo when config.js comes from a build without the background', () => {
    const older: InstanceBrandingGlobal = { ...DEFAULT_INSTANCE_BRANDING }
    delete older.background_url
    window.INSTANCE_BRANDING = older
    renderLayout()

    expect(screen.getByRole('link', { name: 'Photo by Sean Oulashin' })).toBeInTheDocument()
    expect(screen.queryByTestId('custom-background')).toBeNull()
  })

  it('shows the instance background instead, without a referrer and without the credit', () => {
    window.INSTANCE_BRANDING = { ...DEFAULT_INSTANCE_BRANDING, background_url: background }
    const { container } = renderLayout()

    const image = screen.getByTestId('custom-background')
    expect(image).toHaveAttribute('src', background)
    expect(image).toHaveAttribute('referrerpolicy', 'no-referrer')
    expect(image).toHaveAttribute('alt', '')
    expect(layoutElement(container).style.backgroundImage).toBe('')
    expect(screen.queryByRole('link', { name: 'Photo by Sean Oulashin' })).toBeNull()
    expect(screen.getByText('Sign in form')).toBeInTheDocument()
  })

  it('falls back to the plain page colour, never the Notifuse photo, when the image fails', () => {
    window.INSTANCE_BRANDING = { ...DEFAULT_INSTANCE_BRANDING, background_url: background }
    const first = renderLayout()

    fireEvent.error(screen.getByTestId('custom-background'))
    expect(screen.queryByTestId('custom-background')).toBeNull()
    expect(layoutElement(first.container).style.backgroundImage).toBe('')
    expect(screen.queryByRole('link', { name: 'Photo by Sean Oulashin' })).toBeNull()
    first.unmount()

    // The next page of the flow (invitation, workspace list) does not request it again.
    renderLayout()
    expect(screen.queryByTestId('custom-background')).toBeNull()
  })
})
