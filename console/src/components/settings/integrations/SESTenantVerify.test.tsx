import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { SESTenantVerifyResult } from './SESTenantVerify'
import type { SESTenantVerifyOutcome } from './useSESTenantVerify'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const show = (outcome: SESTenantVerifyOutcome | null) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <SESTenantVerifyResult outcome={outcome} />
      </App>
    </I18nProvider>
  )

const verification = (overrides: Record<string, unknown> = {}): SESTenantVerifyOutcome => ({
  tenant: 'team',
  verification: { tenant_name: 'team', exists: true, configuration_set_associated: true, ...overrides }
})

describe('SESTenantVerifyResult', () => {
  it('shows nothing before a check', () => {
    show(null)
    expect(document.querySelector('.ant-alert')).toBeNull()
  })

  it('confirms a tenant whose configuration set is associated', () => {
    show(verification({ configuration_set_name: 'notifuse-int_1' }))
    expect(
      screen.getByText('Tenant team exists and configuration set notifuse-int_1 is associated with it.')
    ).toBeInTheDocument()
  })

  it('says when there is no configuration set to check yet', () => {
    show(verification({ configuration_set_associated: false }))
    expect(
      screen.getByText('Tenant team exists. No configuration set to check it against yet: register webhooks or name one.')
    ).toBeInTheDocument()
  })

  it('warns about a tenant SES paused', () => {
    show(verification({ configuration_set_name: 'cs', sending_status: 'DISABLED' }))
    expect(screen.getByText('SES paused sending for this tenant.')).toBeInTheDocument()
  })

  it('names the permissions a denied check needs', () => {
    show({ tenant: 'team', denied: true })
    expect(screen.getByText(/needs ses:GetTenant and ses:ListTenantResources/)).toBeInTheDocument()
  })

  it('shows the failure the server reported', () => {
    show({ tenant: 'team', error: 'throttled' })
    expect(screen.getByText('throttled')).toBeInTheDocument()
  })
})
