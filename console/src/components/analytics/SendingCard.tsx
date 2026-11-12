import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../layout'
import { emailProviders } from '../integrations/EmailProviders'
import type { Integration, Workspace } from '../../services/api/workspace'

interface SendingCardProps {
  workspace: Workspace
  /** Shows the « Configure » link to the integrations settings. */
  canConfigure: boolean
}

interface ProviderUse {
  key: string
  label: string
  providerId?: string
}

/** The provider and default sender of each kind of email, as the workspace settings choose them. */
export function SendingCard({ workspace, canConfigure }: SendingCardProps) {
  const { t } = useLingui()
  const uses: ProviderUse[] = [
    { key: 'marketing', label: t`Marketing`, providerId: workspace.settings?.marketing_email_provider_id },
    { key: 'transactional', label: t`Transactional`, providerId: workspace.settings?.transactional_email_provider_id }
  ]

  const provider = (id?: string): Integration | undefined =>
    id ? workspace.integrations?.find((integration) => integration.id === id) : undefined

  return (
    <Panel title={t`Sending`}>
      <div className="flex flex-col gap-3">
        {uses.map((use) => {
          const integration = provider(use.providerId)
          const kind = integration?.email_provider?.kind
          const senders = integration?.email_provider?.senders ?? []
          const sender = senders.find((candidate) => candidate.is_default) ?? senders[0]
          return (
            <div key={use.key} className="grid grid-cols-[110px_minmax(0,1fr)] items-baseline gap-3 text-sm">
              <span className="text-ink-3">{use.label}</span>
              {integration ? (
                <div className="min-w-0">
                  <div className="truncate font-medium text-ink">
                    {emailProviders.find((info) => info.kind === kind)?.name ?? integration.name}
                  </div>
                  {sender ? <div className="truncate text-xs text-ink-3">{sender.email}</div> : null}
                </div>
              ) : (
                <div className="flex items-baseline gap-2">
                  <span className="text-ink-4">{t`Not configured`}</span>
                  {canConfigure ? (
                    <Link
                      to="/console/workspace/$workspaceId/settings/$section"
                      params={{ workspaceId: workspace.id, section: 'integrations' }}
                    >
                      {t`Configure`}
                    </Link>
                  ) : null}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </Panel>
  )
}
