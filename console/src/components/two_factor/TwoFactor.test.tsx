import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App as AntApp, Form } from 'antd'
import type { ReactNode } from 'react'
import { ApiError } from '../../services/api/errors'
import { AuthContext } from '../../contexts/AuthContext'
import type { TwoFactorState } from '../../services/api/auth'
import { TwoFactorChallengeForm } from './TwoFactorChallengeForm'
import { SetupWizard } from './SetupWizard'
import { AccountSecurityPanel } from './AccountSecurityPanel'
import { SystemSecuritySettings } from '../settings/SystemSecuritySettings'
import { groupKey } from './groupKey'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

// The API client pulls the router and every page; the calls go through this one function.
vi.mock('../../services/api/client', () => ({ api: { post } }))
vi.mock('qrcode.react', () => ({ QRCodeSVG: ({ value }: { value: string }) => <svg data-qr={value} /> }))

const refusal = (code: string, status: number, extra: Record<string, unknown> = {}) =>
  new ApiError(code, status, { error: code, ...extra })

const CODES = Array.from({ length: 10 }, (_, i) => `aaaaaa-bbbbbb-cccccc-00000${i}`)

function withAuth(children: ReactNode, twoFactor: TwoFactorState | null = null, sessionPassed = true) {
  return (
    <AntApp>
      <AuthContext.Provider
        value={{
          user: { id: 'u1', email: 'ann@example.com' },
          workspaces: [],
          isAuthenticated: true,
          signin: vi.fn(),
          signout: vi.fn(),
          loading: false,
          refreshWorkspaces: vi.fn().mockResolvedValue(undefined),
          licenseEntitlements: null,
          checkForUpdates: false,
          serverVersion: null,
          twoFactor,
          twoFactorRequired: false,
          sessionTwoFactorPassed: sessionPassed,
          rootSessionWithoutCode: false
        }}
      >
        {children}
      </AuthContext.Provider>
    </AntApp>
  )
}

beforeEach(() => {
  post.mockReset()
})

describe('groupKey', () => {
  it('groups the key by four characters', () => {
    expect(groupKey('JBSWY3DPEHPK3PXP')).toBe('JBSW Y3DP EHPK 3PXP')
  })
})

describe('TwoFactorChallengeForm', () => {
  const renderForm = (onSignedIn = vi.fn(), onRestart = vi.fn()) => {
    render(withAuth(<TwoFactorChallengeForm challengeToken="challenge" onSignedIn={onSignedIn} onRestart={onRestart} />))
    return { onSignedIn, onRestart }
  }
  const submit = (value: string) => {
    fireEvent.change(screen.getByRole('textbox'), { target: { value } })
    fireEvent.click(screen.getByRole('button', { name: 'Verify' }))
  }

  it('sends the code with the challenge and signs in', async () => {
    post.mockResolvedValue({ token: 'jwt' })
    const { onSignedIn } = renderForm()
    submit('123 456')
    await waitFor(() => expect(onSignedIn).toHaveBeenCalledWith({ token: 'jwt' }))
    expect(post).toHaveBeenCalledWith('/api/user.twoFactor.verify', { challenge_token: 'challenge', code: '123456' })
  })

  it('stays on the step after a wrong code', async () => {
    post.mockRejectedValue(refusal('two_factor_invalid_code', 422))
    renderForm()
    submit('000000')
    expect(await screen.findByText(/That code didn't work/)).toBeTruthy()
  })

  it('says how long to wait', async () => {
    post.mockRejectedValue(refusal('two_factor_locked', 429, { retry_after: 230 }))
    renderForm()
    submit('000000')
    expect(await screen.findByText(/Try again in 4 minutes/)).toBeTruthy()
  })

  // Only a recovery code gets through: offered straight away.
  it('switches to a recovery code when authenticator codes are blocked', async () => {
    post.mockRejectedValue(refusal('two_factor_totp_locked', 429))
    renderForm()
    submit('000000')
    expect(await screen.findByText('Recovery code')).toBeTruthy()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: CODES[0] } })
    post.mockResolvedValue({ token: 'jwt' })
    fireEvent.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() =>
      expect(post).toHaveBeenLastCalledWith('/api/user.twoFactor.verify', { challenge_token: 'challenge', recovery_code: CODES[0] })
    )
  })

  it('asks for what the field takes: the app code, then a recovery code', () => {
    renderForm()
    expect(screen.getByText('Enter the code from your authenticator app.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Use a recovery code instead' }))
    expect(
      screen.getByText('Enter one of the recovery codes you saved when you turned on two-factor authentication.')
    ).toBeInTheDocument()
    expect(screen.queryByText('Enter the code from your authenticator app.')).not.toBeInTheDocument()
  })

  it('says the code accepts the invitation when it comes from one', () => {
    render(withAuth(<TwoFactorChallengeForm invitation challengeToken="challenge" onSignedIn={vi.fn()} onRestart={vi.fn()} />))
    expect(screen.getByText('Enter the code from your authenticator app to accept the invitation.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Use a recovery code instead' }))
    expect(screen.getByText('Enter one of your recovery codes to accept the invitation.')).toBeInTheDocument()
  })

  it('sends the person back to the email step when the challenge is gone', async () => {
    post.mockRejectedValue(refusal('two_factor_challenge_invalid', 410))
    const { onRestart } = renderForm()
    submit('123456')
    fireEvent.click(await screen.findByRole('button', { name: 'Back to sign in' }))
    expect(onRestart).toHaveBeenCalled()
  })
})

describe('SetupWizard', () => {
  const setup = { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/Notifuse:ann@example.com?secret=JBSWY3DPEHPK3PXP' }

  it('scans, confirms, and holds the codes until they are marked as saved', async () => {
    post.mockImplementation((url: string) =>
      Promise.resolve(url.endsWith('setup') ? setup : { recovery_codes: CODES })
    )
    const onFinished = vi.fn()
    const onLockChange = vi.fn()
    render(
      withAuth(
        <SetupWizard email="ann@example.com" onFinished={onFinished} onCancel={vi.fn()} onReauthenticate={vi.fn()} onLockChange={onLockChange} />
      )
    )
    expect(await screen.findByText('JBSW Y3DP EHPK 3PXP')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Turn on' }))

    expect(await screen.findByText(CODES[3])).toBeTruthy()
    expect(post).toHaveBeenCalledWith('/api/user.twoFactor.enable', { code: '123456', sign_out_other_sessions: true })
    expect(onLockChange).toHaveBeenLastCalledWith(true)
    const done = screen.getByRole('button', { name: 'Done' })
    expect((done as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(screen.getByRole('checkbox', { name: 'I saved my recovery codes somewhere safe' }))
    // Still locked until Done.
    expect(onLockChange).toHaveBeenLastCalledWith(true)
    fireEvent.click(done)
    expect(onFinished).toHaveBeenCalled()
  })

  it('starts again with a new QR code when the setup expired', async () => {
    let setups = 0
    post.mockImplementation((url: string) => {
      if (url.endsWith('setup')) {
        setups++
        return Promise.resolve(setup)
      }
      return Promise.reject(refusal('two_factor_setup_expired', 410))
    })
    render(withAuth(<SetupWizard onFinished={vi.fn()} onCancel={vi.fn()} onReauthenticate={vi.fn()} />))
    fireEvent.click(await screen.findByRole('button', { name: 'Next' }))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Turn on' }))
    expect(await screen.findByText(/This setup expired after 15 minutes/)).toBeTruthy()
    expect(setups).toBe(2)
  })

  it('asks for a recent sign-in when the session is not fresh', async () => {
    post.mockRejectedValue(refusal('reauthentication_required', 403))
    const onReauthenticate = vi.fn()
    render(withAuth(<SetupWizard onFinished={vi.fn()} onCancel={vi.fn()} onReauthenticate={onReauthenticate} />))
    await waitFor(() => expect(onReauthenticate).toHaveBeenCalled())
  })

  // Replacing the app keeps the recovery codes: no third step. The proof was checked before.
  it('replaces the app on the setup it was given, without new codes', async () => {
    post.mockResolvedValue({ recovery_codes: [] })
    const onFinished = vi.fn()
    const onEnabled = vi.fn()
    render(
      withAuth(
        <SetupWizard replacing initialSetup={setup} onFinished={onFinished} onEnabled={onEnabled} onCancel={vi.fn()} onReauthenticate={vi.fn()} />
      )
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Next' }))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Replace app' }))
    await waitFor(() => expect(onFinished).toHaveBeenCalled())
    expect(onEnabled).toHaveBeenCalled()
    expect(post).toHaveBeenCalledTimes(1)
  })

  // Its proof is used up: a replacement that expired asks for a new one.
  it('ends a replacement that expired, without sending the spent proof again', async () => {
    post.mockRejectedValue(refusal('two_factor_setup_expired', 410))
    const onCancel = vi.fn()
    render(withAuth(<SetupWizard replacing initialSetup={setup} onFinished={vi.fn()} onCancel={onCancel} onReauthenticate={vi.fn()} />))
    fireEvent.click(await screen.findByRole('button', { name: 'Next' }))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Replace app' }))
    expect(await screen.findByText(/Start the replacement again/)).toBeTruthy()
    expect(post).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCancel).toHaveBeenCalled()
  })
})

describe('AccountSecurityPanel', () => {
  it('offers to turn it on when it is off', () => {
    render(withAuth(<AccountSecurityPanel />, { enabled: false, recovery_codes_remaining: 0, required_by_instance: false }))
    expect(screen.getByRole('button', { name: 'Turn on two-factor authentication' })).toBeTruthy()
  })

  it('shows the codes left, and keeps it on while the instance requires it', () => {
    render(
      withAuth(<AccountSecurityPanel />, {
        enabled: true,
        enabled_at: '2026-11-23T10:00:00Z',
        recovery_codes_remaining: 7,
        required_by_instance: true
      })
    )
    expect(screen.getByText('7 of 10 left')).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Turn off' }) as HTMLButtonElement).disabled).toBe(true)
  })

  // The proof is checked while the modal is open: a wrong code stays there.
  it('keeps a wrong code for Replace app in the modal', async () => {
    post.mockRejectedValue(refusal('two_factor_invalid_code', 422))
    render(withAuth(<AccountSecurityPanel />, { enabled: true, recovery_codes_remaining: 10, required_by_instance: false }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace app' }))
    fireEvent.change(await screen.findByRole('textbox'), { target: { value: '000000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(await screen.findByText(/That code didn't work/)).toBeTruthy()
    expect(post).toHaveBeenCalledWith('/api/user.twoFactor.setup', { code: '000000' })
    expect(screen.getByRole('dialog')).toBeTruthy()
  })

  it('opens the wizard on the setup the proof started', async () => {
    post.mockResolvedValue({ secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/x' })
    render(withAuth(<AccountSecurityPanel />, { enabled: true, recovery_codes_remaining: 10, required_by_instance: false }))
    fireEvent.click(screen.getByRole('button', { name: 'Replace app' }))
    fireEvent.change(await screen.findByRole('textbox'), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(await screen.findByText('JBSW Y3DP EHPK 3PXP')).toBeTruthy()
    expect(post).toHaveBeenCalledTimes(1)
  })

  // Outside sign-in, a recovery code waits like an authenticator code: no hint to use one.
  it('says how long to wait, without offering a recovery code', async () => {
    post.mockRejectedValue(refusal('two_factor_locked', 429, { retry_after: 120 }))
    render(withAuth(<AccountSecurityPanel />, { enabled: true, recovery_codes_remaining: 10, required_by_instance: false }))
    fireEvent.click(screen.getByRole('button', { name: 'Turn off' }))
    fireEvent.change(await screen.findByRole('textbox'), { target: { value: '123456' } })
    const buttons = screen.getAllByRole('button', { name: 'Turn off' })
    fireEvent.click(buttons[buttons.length - 1])
    expect(await screen.findByText('Too many wrong codes. Try again in 2 minutes.')).toBeTruthy()
  })

  it('turns it off with a code', async () => {
    post.mockResolvedValue({ success: true })
    render(withAuth(<AccountSecurityPanel />, { enabled: true, recovery_codes_remaining: 10, required_by_instance: false }))
    fireEvent.click(screen.getByRole('button', { name: 'Turn off' }))
    fireEvent.change(await screen.findByRole('textbox'), { target: { value: '123456' } })
    const buttons = screen.getAllByRole('button', { name: 'Turn off' })
    fireEvent.click(buttons[buttons.length - 1])
    await waitFor(() => expect(post).toHaveBeenCalledWith('/api/user.twoFactor.disable', { code: '123456' }))
  })
})

describe('SystemSecuritySettings', () => {
  const renderSection = (props: Parameters<typeof SystemSecuritySettings>[0], own: TwoFactorState | null, sessionPassed = own?.enabled === true) =>
    render(
      withAuth(
        <Form initialValues={{ require_two_factor: false }}>
          <SystemSecuritySettings {...props} />
        </Form>,
        own,
        sessionPassed
      )
    )

  it('is read-only when REQUIRE_TWO_FACTOR sets it', () => {
    renderSection({ twoFactor: { required: true, from_env: true, users_without: [] } }, { enabled: true, recovery_codes_remaining: 10, required_by_instance: true })
    expect(screen.getByRole('switch').hasAttribute('disabled')).toBe(true)
    expect(screen.getByText(/REQUIRE_TWO_FACTOR environment variable/)).toBeTruthy()
  })

  it('asks a root user without it to turn it on first', () => {
    renderSection({ twoFactor: { required: false, from_env: false, users_without: ['lee@acme.io'] } }, { enabled: false, recovery_codes_remaining: 0, required_by_instance: false })
    expect(screen.getByRole('switch').hasAttribute('disabled')).toBe(true)
    expect(screen.getByText(/so you don't lock yourself out/)).toBeTruthy()
    expect(screen.getByText('lee@acme.io')).toBeTruthy()
  })

  // Single sign-on owns multi-factor authentication: the server accepts it, the switch too.
  it('lets a root user signed in with single sign-on switch it on without an authenticator', () => {
    renderSection({ twoFactor: { required: false, from_env: false, users_without: [] } }, { enabled: false, recovery_codes_remaining: 0, required_by_instance: false }, true)
    expect(screen.getByRole('switch').hasAttribute('disabled')).toBe(false)
  })

  it('lets a root user with it switch the requirement on', () => {
    renderSection({ twoFactor: { required: false, from_env: false, users_without: [] } }, { enabled: true, recovery_codes_remaining: 10, required_by_instance: false })
    expect(screen.getByRole('switch').hasAttribute('disabled')).toBe(false)
  })
})
