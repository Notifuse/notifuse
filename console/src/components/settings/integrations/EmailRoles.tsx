import { App, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { Workspace } from '../../../services/api/types'
import { Panel } from '../../layout'
import { SettingsRow, SettingsRows } from '../SettingsRow'
import type { EmailIntegration } from './EmailProviderCard'
import { transactionalEmailOnly } from './providerDetails'

export type EmailRole = 'marketing' | 'transactional'

export interface EmailRolesProps {
  workspace: Workspace
  emailIntegrations: EmailIntegration[]
  isOwner: boolean
  onSetDefault: (id: string, purpose: EmailRole) => Promise<void>
}

/**
 * Which provider sends what: the two roles of the workspace settings, each a provider picker. A
 * role without a provider shows "Not set" in an orange picker. Changing one asks first, as the card menus
 * do: every email of that kind goes through the new provider from then on.
 */
export function EmailRoles({ workspace, emailIntegrations, isOwner, onSetDefault }: EmailRolesProps) {
  const { t } = useLingui()
  const { modal } = App.useApp()

  const confirm = (purpose: EmailRole, id: string) =>
    modal.confirm({
      title: purpose === 'marketing' ? t`Set as marketing email provider?` : t`Set as transactional email provider?`,
      content:
        purpose === 'marketing'
          ? t`All marketing emails (broadcasts, campaigns) will be sent through this provider from now on.`
          : t`All transactional emails (notifications, password resets, etc.) will be sent through this provider from now on.`,
      okText: t`Yes`,
      cancelText: t`No`,
      onOk: () => onSetDefault(id, purpose)
    })

  const row = (purpose: EmailRole) => {
    // The server refuses Mailjet for marketing (workspace_service.go), and blocks its sends.
    const options = emailIntegrations
      .filter((integration) => purpose === 'transactional' || !transactionalEmailOnly.includes(integration.email_provider.kind))
      .map((integration) => ({ value: integration.id, label: integration.name }))
    const storedId =
      purpose === 'marketing'
        ? workspace.settings.marketing_email_provider_id
        : workspace.settings.transactional_email_provider_id
    // An id that names no listed provider shows as not set rather than as a raw id.
    const value = options.some((option) => option.value === storedId) ? storedId : undefined
    const label = purpose === 'marketing' ? t`Marketing emails` : t`Transactional emails`
    const help = purpose === 'marketing' ? t`Broadcasts and automation emails` : t`Notifications sent through the API, Zapier or Supabase`

    return (
      <SettingsRow
        label={label}
        help={help}
        htmlFor={`email-role-${purpose}`}
      >
        <Select
          id={`email-role-${purpose}`}
          aria-label={label}
          style={{ width: '100%' }}
          value={value}
          placeholder={t`Not set`}
          status={value ? undefined : 'warning'}
          options={options}
          disabled={!isOwner || options.length === 0}
          onChange={(id: string) => {
            if (id !== value) confirm(purpose, id)
          }}
          notFoundContent={t`No email provider yet`}
        />
      </SettingsRow>
    )
  }

  return (
    <Panel title={t`Email sending`} extra={<span className="text-xs text-ink-3">{t`Which provider sends what`}</span>}>
      <SettingsRows>
        {row('marketing')}
        {row('transactional')}
      </SettingsRows>
    </Panel>
  )
}
