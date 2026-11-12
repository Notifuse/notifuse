import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from '../App'

/**
 * Dates formatted with dayjs carry no Lingui subscription: they change language only because
 * App remounts everything under it (key={locale}) once a locale is activated. The page below
 * stands in for a route — memoised, as the router's own matches are — and reads neither Lingui nor
 * the locale context, so the remount is the only thing that can repaint it.
 */
vi.mock('../i18n/locales/en.po', () => ({ messages: {} }))
vi.mock('../i18n/locales/fr.po', () => ({ messages: {} }))
vi.mock('../router', () => ({ router: {} }))
vi.mock('../services/api/auth', () => ({
  authService: {
    getCurrentUser: vi.fn().mockRejectedValue(new Error('signed out')),
    updateLanguage: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn(),
  },
  isRootUser: () => false,
}))
vi.mock('../services/api/workspace', () => ({ workspaceService: {} }))

vi.mock('@tanstack/react-router', async () => {
  const { memo } = await import('react')
  const { default: dayjs } = await import('dayjs')
  const { useLocale } = await import('../contexts/LocaleContext')
  const DatedPage = memo(function DatedPage() {
    return <span data-testid="month">{dayjs('2026-03-05').format('MMMM')}</span>
  })
  function SwitchToFrench() {
    const { setLocale } = useLocale()
    return <button onClick={() => void setLocale('fr')}>French</button>
  }
  return {
    RouterProvider: () => (
      <>
        <SwitchToFrench />
        <DatedPage />
      </>
    ),
  }
})

describe('App and the dayjs locale', () => {
  it('repaints dayjs dates when the language changes', async () => {
    render(<App />)

    expect(await screen.findByTestId('month')).toHaveTextContent('March')

    await userEvent.click(screen.getByRole('button', { name: 'French' }))

    await waitFor(() => expect(screen.getByTestId('month')).toHaveTextContent('mars'))
  })
})
