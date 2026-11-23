import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { AccountMenu } from './AccountMenu'

vi.mock('../../contexts/LocaleContext', () => ({
  useLocale: () => ({ locale: 'en', setLocale: vi.fn() })
}))

// Account security opens from the account menu, under the email.
describe('AccountMenu account security entry', () => {
  it('opens account security', async () => {
    const onOpenSecurity = vi.fn()
    render(
      <AccountMenu
        user={{ email: 'ann@example.com' }}
        collapsed={false}
        onSignout={vi.fn()}
        onOpenSecurity={onOpenSecurity}
        getPopupContainer={() => document.body}
      />
    )
    fireEvent.click(screen.getByRole('button'))
    fireEvent.click(await screen.findByText('Account security'))
    expect(onOpenSecurity).toHaveBeenCalled()
  })

  it('has no entry without a handler', async () => {
    render(<AccountMenu user={{ email: 'ann@example.com' }} collapsed={false} onSignout={vi.fn()} getPopupContainer={() => document.body} />)
    fireEvent.click(screen.getByRole('button'))
    await screen.findByText('Logout')
    expect(screen.queryByText('Account security')).toBeNull()
  })
})
