import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SMTPBridgeSettings } from './SMTPBridgeSettings'

// Minimal i18n setup — Lingui's macro falls back to message IDs if no messages are loaded.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderComponent = () =>
  render(
    <I18nProvider i18n={i18n}>
      <SMTPBridgeSettings />
    </I18nProvider>,
  )

describe('SMTPBridgeSettings', () => {
  const originalBridge = window.SMTP_BRIDGE_ENABLED
  const originalDomain = window.SMTP_BRIDGE_DOMAIN
  const originalPort = window.SMTP_BRIDGE_PORT
  const originalMode = window.SMTP_BRIDGE_TLS_MODE

  beforeEach(() => {
    window.SMTP_BRIDGE_ENABLED = true
    window.SMTP_BRIDGE_DOMAIN = 'smtp.example.com'
    window.SMTP_BRIDGE_PORT = 587
  })

  afterEach(() => {
    window.SMTP_BRIDGE_ENABLED = originalBridge
    window.SMTP_BRIDGE_DOMAIN = originalDomain
    window.SMTP_BRIDGE_PORT = originalPort
    window.SMTP_BRIDGE_TLS_MODE = originalMode
  })

  it('shows STARTTLS label when mode is starttls', () => {
    window.SMTP_BRIDGE_TLS_MODE = 'starttls'
    renderComponent()
    expect(screen.getByText('STARTTLS')).toBeInTheDocument()
  })

  it('shows Implicit TLS label when mode is implicit', () => {
    window.SMTP_BRIDGE_TLS_MODE = 'implicit'
    renderComponent()
    expect(screen.getByText('Implicit TLS')).toBeInTheDocument()
  })

  it('shows Off (plaintext) label when mode is off', () => {
    window.SMTP_BRIDGE_TLS_MODE = 'off'
    renderComponent()
    expect(screen.getByText('Off (plaintext)')).toBeInTheDocument()
  })

  it('shows fallback when bridge is disabled', () => {
    window.SMTP_BRIDGE_ENABLED = false
    window.SMTP_BRIDGE_TLS_MODE = 'off'
    renderComponent()
    expect(screen.getByText(/not configured/i)).toBeInTheDocument()
  })
})
