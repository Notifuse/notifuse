import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SetupRestartView, type RestartPhase } from './SetupRestartView'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderView = (phase: RestartPhase, handlers = { onSignIn: vi.fn(), onReload: vi.fn() }) => {
  render(
    <I18nProvider i18n={i18n}>
      <SetupRestartView phase={phase} elapsed={12} rootEmail="admin@acme.com" {...handlers} />
    </I18nProvider>
  )
  return handlers
}

describe('SetupRestartView', () => {
  it('waits with the sign-in button disabled and the seconds spent', () => {
    renderView('waiting')
    expect(screen.getByText('Applying your configuration')).toBeInTheDocument()
    expect(screen.getByText('12 s')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Go to sign in/ })).toBeDisabled()
  })

  it('says who signs in once the server is back', () => {
    const { onSignIn } = renderView('ready')
    expect(screen.getByText('Notifuse is ready')).toBeInTheDocument()
    expect(screen.getByText('Sign in with admin@acme.com.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Go to sign in/ }))
    expect(onSignIn).toHaveBeenCalled()
  })

  it('offers a reload after the timeout, saying the configuration is saved', () => {
    const { onReload } = renderView('timeout')
    expect(screen.getByText('The server has not come back after 60 seconds.')).toBeInTheDocument()
    expect(screen.getByText(/Your configuration is already saved/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Go to sign in/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reload' }))
    expect(onReload).toHaveBeenCalled()
  })
})
