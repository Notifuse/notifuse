import { useEffect, useRef, useState } from 'react'
import { App, Button, Dropdown, Empty } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Blocks, MoreHorizontal, Plus } from 'lucide-react'

import type { EmailProviderKind, Integration, IntegrationType, Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { listsApi } from '../../services/api/list'
import { getProviderName } from '../integrations/EmailProviders'
import { SupabaseIntegration } from '../integrations/SupabaseIntegration'
import { zapierProvider } from '../integrations/ZapierProviders'
import { supabaseWebhookURL } from '../integrations/supabaseWebhookURL'
import { ZapierSettings } from './ZapierSettings'
import { KeyValueList, Mono, PageHeader, Panel, Pill } from '../layout'
import { IntegrationCard } from './integrations/IntegrationCard'
import { EmailProviderCard, type EmailIntegration } from './integrations/EmailProviderCard'
import { EmailProviderDrawer } from './integrations/EmailProviderDrawer'
import { EmailRoles, type EmailRole } from './integrations/EmailRoles'
import { IntegrationCatalogue, type CatalogueSelection } from './integrations/IntegrationCatalogue'
import { IntegrationDrawer } from './integrations/IntegrationDrawer'
import { TestEmailModal, type TestEmailTarget } from './integrations/TestEmailModal'
import { maskedHint } from './integrations/providerDetails'
import { credentialHint } from './integrations/secrets'
import {
  formatConnectedOn,
  useIntegrationActions,
  type DeleteConfirmTexts
} from './integrations/useIntegrationActions'

interface IntegrationsProps {
  workspace: Workspace | null
  onSave: (updatedWorkspace: Workspace) => Promise<void>
  loading: boolean
  isOwner: boolean
}

type FormRef = { submit: () => void } | null

/** The drawer on screen, and the integration it edits (none to add one). */
type OpenDrawer =
  | { type: 'email'; kind: EmailProviderKind; integration?: EmailIntegration; addSender?: boolean }
  | { type: 'supabase'; integration?: Integration }
  | { type: 'zapier'; integration?: Integration }

/**
 * Settings › Integrations: the two sending roles, then the connected apps, and one "Add
 * integration" catalogue. Each form opens in a full-screen drawer. The AI models and Firecrawl
 * live in Settings › AI › Assistant.
 */
export function Integrations({ workspace, onSave, loading, isOwner }: IntegrationsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [catalogueOpen, setCatalogueOpen] = useState(false)
  const [drawer, setDrawer] = useState<OpenDrawer | null>(null)
  const { saving, setSaving, refreshWorkspace, saveIntegration, deleteIntegration, confirmDelete } =
    useIntegrationActions(workspace, onSave)
  const [testTarget, setTestTarget] = useState<TestEmailTarget | null>(null)
  const formRef = useRef<FormRef>(null)

  // Lists state for the Supabase card
  const [lists, setLists] = useState<{ id: string; name: string }[]>([])

  useEffect(() => {
    const fetchLists = async () => {
      if (!workspace) return
      try {
        const listsResponse = await listsApi.list({ workspace_id: workspace.id })
        setLists(listsResponse.lists || [])
      } catch (error) {
        console.error('Failed to fetch lists:', error)
        setLists([])
      }
    }
    fetchLists()
  // eslint-disable-next-line react-hooks/exhaustive-deps -- Only re-run on workspace change
  }, [workspace?.id])

  if (!workspace) {
    return null
  }

  const integrations = workspace.integrations ?? []
  const getIntegrationById = (id: string): Integration | undefined => integrations.find((i) => i.id === id)

  const closeDrawer = () => setDrawer(null)

  // Set integration as default for a purpose
  const setIntegrationAsDefault = async (id: string, purpose: EmailRole) => {
    try {
      await workspaceService.update({
        ...workspace,
        settings: {
          ...workspace.settings,
          ...(purpose === 'marketing'
            ? { marketing_email_provider_id: id }
            : { transactional_email_provider_id: id })
        }
      })
      await refreshWorkspace()
      message.success(
        purpose === 'marketing'
          ? t`Set as default marketing email provider`
          : t`Set as default transactional email provider`
      )
    } catch (error) {
      console.error('Error setting default provider', error)
      message.error(t`Failed to set default provider`)
    }
  }

  const startTestEmailProvider = (integrationId: string) => {
    const integration = getIntegrationById(integrationId)
    if (!integration || integration.type !== 'email' || !integration.email_provider) {
      message.error(t`Integration not found or not an email provider`)
      return
    }
    setTestTarget({ provider: integration.email_provider, integrationId })
  }

  const editEmailProvider = (integration: Integration, addSender = false) => {
    if (integration.type !== 'email' || !integration.email_provider) return
    setDrawer({
      type: 'email',
      kind: integration.email_provider.kind,
      integration: integration as EmailIntegration,
      addSender
    })
  }

  const selectFromCatalogue = (selection: CatalogueSelection) => {
    setCatalogueOpen(false)
    if (selection.type === 'email') {
      setDrawer({ type: 'email', kind: selection.kind })
    } else {
      setDrawer({ type: selection.type })
    }
  }

  // Connecting writes the card server-side, so all this does is pull the new list. It must not
  // close the drawer: the token comes back in exactly one response and nothing can reissue it, so
  // the panel showing it stands until the user dismisses it. Failures are swallowed on purpose —
  // the connection succeeded whatever the refetch did, and rethrowing would reach the drawer body
  // as a failed connect.
  const refreshAfterZapierConnect = async () => {
    try {
      await refreshWorkspace()
    } catch (error) {
      console.error('Error refreshing workspace after connecting Zapier:', error)
      message.error(t`Zapier is connected, but the integration list could not be refreshed`)
    }
  }

  // Renames only. The minted address is server-owned and the update request carries no settings
  // field, so a rename cannot disturb it — which is why a renamed card and its key address can
  // read differently, and why the card shows both.
  const saveZapierIntegration = async (integration: Integration) => {
    setSaving(true)
    try {
      await workspaceService.updateIntegration({
        workspace_id: workspace.id,
        integration_id: integration.id,
        name: integration.name
      })
      await refreshWorkspace()
      closeDrawer()
      message.success(t`Zapier integration saved successfully`)
    } catch (error) {
      console.error('Error saving Zapier integration:', error)
      message.error(t`Failed to save Zapier integration`)
      throw error
    } finally {
      setSaving(false)
    }
  }

  // Edit and the menu of a card, for the owner.
  const cardActions = (
    integration: Integration,
    onEdit: () => void,
    remove: { label: string; confirm: DeleteConfirmTexts }
  ) =>
    isOwner ? (
      <>
        <Dropdown
          trigger={['click']}
          placement="bottomRight"
          menu={{
            items: [
              { key: 'edit', label: t`Edit`, onClick: onEdit },
              { type: 'divider' as const },
              {
                key: 'delete',
                label: remove.label,
                danger: true,
                onClick: () => confirmDelete(integration.id, remove.confirm)
              }
            ]
          }}
        >
          <Button
            type="text"
            aria-label={t`More actions`}
            icon={<MoreHorizontal size={16} aria-hidden="true" />}
          />
        </Dropdown>
      </>
    ) : null

  const configured = (
    <Pill tone="success" dot={false}>
      {t`Configured`}
    </Pill>
  )

  // A hook of Supabase: configured, with the end of its secret and the endpoint to paste into
  // Supabase, or not.
  const supabaseHook = (configuredHook: boolean, hint: string | undefined, url: string) =>
    configuredHook ? (
      <div className="flex min-w-0 flex-col items-start gap-1">
        <span className="flex items-center gap-2">
          {configured}
          {hint ? (
            <span className="text-xs text-ink-3">
              {t`secret`} <Mono>{maskedHint(hint)}</Mono>
            </span>
          ) : null}
        </span>
        <Mono copy>{url}</Mono>
      </div>
    ) : (
      <Pill tone="neutral" dot={false}>
        {t`Not configured`}
      </Pill>
    )

  const renderOtherIntegration = (integration: Integration) => {
    if (integration.type === 'supabase') {
      // The encrypted form, not the plaintext: credentials are no longer
      // served to clients, so keying off signature_key would show every
      // configured hook as unconfigured.
      const hasAuthEmailHook =
        !!integration.supabase_settings?.auth_email_hook?.encrypted_signature_key ||
        !!integration.supabase_settings?.auth_email_hook?.signature_key
      const hasBeforeUserCreatedHook =
        !!integration.supabase_settings?.before_user_created_hook?.encrypted_signature_key ||
        !!integration.supabase_settings?.before_user_created_hook?.signature_key
      const addToLists = integration.supabase_settings?.before_user_created_hook?.add_user_to_lists || []
      const customJsonField = integration.supabase_settings?.before_user_created_hook?.custom_json_field
      const rejectDisposableEmail = integration.supabase_settings?.before_user_created_hook?.reject_disposable_email

      return (
        <IntegrationCard
          key={integration.id}
          icon={<img src="/console/supabase.png" alt="Supabase" className="h-3.5" />}
          title={integration.name}
          titleTooltip={integration.id}
          subtitle={t`Authentication`}
          actions={cardActions(integration, () => setDrawer({ type: 'supabase', integration }), {
            label: t`Delete`,
            confirm: {
              title: t`Delete this integration?`,
              // Creating the integration created its templates and notifications
              // (supabase_templates.go, supabase_notifications.go); deleting it deletes them.
              content: t`Its 6 templates and 6 transactional notifications are deleted with it, and Supabase's hooks stop working. This cannot be undone.`
            }
          })}
        >
          <KeyValueList
            layout="stacked"
            items={[
              {
                label: t`Auth Email Hook`,
                value: supabaseHook(
                  hasAuthEmailHook,
                  credentialHint(integration, 'supabase.auth_email_hook.signature_key'),
                  supabaseWebhookURL('auth-email', workspace.id, integration.id)
                )
              },
              {
                label: t`Before User Created Hook`,
                value: supabaseHook(
                  hasBeforeUserCreatedHook,
                  credentialHint(integration, 'supabase.before_user_created_hook.signature_key'),
                  supabaseWebhookURL('before-user-created', workspace.id, integration.id)
                )
              },
              ...(hasBeforeUserCreatedHook && addToLists.length > 0
                ? [
                    {
                      label: t`Auto-subscribe to Lists`,
                      value: addToLists.map((listId) => lists.find((l) => l.id === listId)?.name || listId).join(', ')
                    }
                  ]
                : []),
              ...(hasBeforeUserCreatedHook
                ? [
                    {
                      label: t`Reject Disposable Email`,
                      value: rejectDisposableEmail ? t`Enabled` : t`Disabled`
                    }
                  ]
                : []),
              ...(hasBeforeUserCreatedHook && customJsonField
                ? [
                    {
                      label: t`User Metadata Field`,
                      value: workspace.settings?.custom_field_labels?.[customJsonField] || customJsonField
                    }
                  ]
                : [])
            ]}
          />
        </IntegrationCard>
      )
    }

    // Guarded on the settings object, not on the type alone: a record written before this
    // release, or by a build that has been rolled back, has none, and reading
    // api_key_email off it would take the whole screen down. It degrades to the card below.
    if (integration.type === 'zapier' && integration.zapier_settings) {
      return (
        <IntegrationCard
          key={integration.id}
          icon={zapierProvider.getIcon('', 14)}
          title={integration.name}
          titleTooltip={integration.id}
          subtitle={t`Triggers and actions`}
          status={
            <Pill tone="success" dot={false}>
              {t`Connected`}
            </Pill>
          }
          actions={cardActions(integration, () => setDrawer({ type: 'zapier', integration }), {
            label: t`Disconnect`,
            confirm: {
              title: t`Delete this connection?`,
              content: t`Deleting revokes the API key this connection minted, so any Zap still using it stops working. This action cannot be undone.`
            }
          })}
        >
          <KeyValueList
            layout="stacked"
            columns={2}
            items={[
              // Shown alongside the name because the two can disagree: renaming the card
              // never re-mints the key, so the address keeps the label it was created
              // under, and this is where an owner matches it to Settings → Team & API keys.
              {
                label: t`API key address`,
                value: <Mono>{integration.zapier_settings.api_key_email}</Mono>
              },
              { label: t`Connected`, value: formatConnectedOn(integration.created_at) }
            ]}
          />
        </IntegrationCard>
      )
    }

    // Any other type: its readable name (product names stay as they are), else the raw type
    const typeNames: Record<IntegrationType, string> = {
      email: t`Email`,
      sms: t`SMS`,
      whatsapp: 'WhatsApp',
      supabase: 'Supabase',
      llm: t`AI model`,
      firecrawl: 'Firecrawl',
      zapier: 'Zapier'
    }
    const typeName = typeNames[integration.type] ?? integration.type
    return (
      <IntegrationCard
        key={integration.id}
        icon={<Blocks size={14} aria-hidden="true" className="text-ink-3" />}
        title={integration.name}
        titleTooltip={integration.id}
        subtitle={t`Type: ${typeName}`}
      />
    )
  }

  const emailIntegrations = integrations.filter(
    (integration): integration is EmailIntegration => integration.type === 'email' && Boolean(integration.email_provider)
  )
  // The AI models and Firecrawl are listed in Settings › AI › Assistant.
  const appIntegrations = integrations.filter(
    (i) => !(i.type === 'email' && i.email_provider) && i.type !== 'llm' && i.type !== 'firecrawl'
  )

  const addIntegration = isOwner ? (
    <Button type="primary" icon={<Plus size={14} aria-hidden="true" />} onClick={() => setCatalogueOpen(true)}>
      {t`Add integration`}
    </Button>
  ) : null

  const drawerTitle = (editing: boolean, name: string) =>
    editing ? t`Edit ${name} integration` : t`Add ${name} integration`

  const saveButton = (
    <Button type="primary" onClick={() => formRef.current?.submit()} loading={saving} disabled={!isOwner}>
      {t`Save`}
    </Button>
  )

  const renderDrawer = () => {
    if (!drawer) return null
    switch (drawer.type) {
      case 'email':
        return (
          <EmailProviderDrawer
            workspace={workspace}
            kind={drawer.kind}
            integration={drawer.integration}
            addSenderOnOpen={drawer.addSender}
            isOwner={isOwner}
            loading={loading}
            onClose={closeDrawer}
            onSaved={refreshWorkspace}
            onTest={setTestTarget}
          />
        )
      case 'supabase':
        return (
          <IntegrationDrawer
            onClose={closeDrawer}
            title={drawerTitle(Boolean(drawer.integration), getProviderName('supabase'))}
            topActions={saveButton}
          >
            <SupabaseIntegration
              integration={drawer.integration}
              workspace={workspace}
              onSave={(integration) =>
                saveIntegration('supabase', integration, {
                  editing: Boolean(drawer.integration),
                  success: t`Supabase integration saved successfully`,
                  failure: t`Failed to save Supabase integration`,
                  onSaved: closeDrawer
                })
              }
              isOwner={isOwner}
              formRef={formRef}
            />
          </IntegrationDrawer>
        )
      case 'zapier':
        return (
          <IntegrationDrawer
            onClose={closeDrawer}
            title={drawer.integration ? t`Edit Zapier connection` : t`Connect Zapier`}
            // Edit mode only. The connect action stays in the body, where it can disable itself
            // while in flight; a bar Save submitting the same form would be a second, always
            // enabled way to mint a key, and nothing server-side refuses the second one.
            topActions={drawer.integration ? saveButton : undefined}
          >
            <ZapierSettings
              workspaceId={workspace.id}
              integration={drawer.integration}
              onSave={saveZapierIntegration}
              onConnected={refreshAfterZapierConnect}
              onDone={closeDrawer}
              isOwner={isOwner}
              formRef={formRef}
            />
          </IntegrationDrawer>
        )
    }
  }

  return (
    <>
      <PageHeader
        title={t`Integrations`}
        description={t`Email providers and connected services`}
        actions={addIntegration}
      />

      <section data-integrations-block="email" className="flex flex-col gap-3">
        <EmailRoles
          workspace={workspace}
          emailIntegrations={emailIntegrations}
          isOwner={isOwner}
          onSetDefault={setIntegrationAsDefault}
        />
        {emailIntegrations.length === 0 ? (
          <div className="rounded-lg border border-line bg-surface py-8 shadow-card">
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No email provider yet`}>
              {addIntegration}
            </Empty>
          </div>
        ) : (
          emailIntegrations.map((integration) => (
            <EmailProviderCard
              key={integration.id}
              integration={integration}
              workspace={workspace}
              isOwner={isOwner}
              onEdit={(target) => editEmailProvider(target)}
              onAddSender={(target) => editEmailProvider(target, true)}
              onTest={startTestEmailProvider}
              onSetDefault={setIntegrationAsDefault}
              onDelete={deleteIntegration}
            />
          ))
        )}
      </section>

      {appIntegrations.length > 0 ? (
        <section data-integrations-block="apps">
          <Panel title={t`Connected apps`}>
            <div className="flex flex-col gap-3">{appIntegrations.map(renderOtherIntegration)}</div>
          </Panel>
        </section>
      ) : null}

      <IntegrationCatalogue
        open={catalogueOpen}
        onClose={() => setCatalogueOpen(false)}
        onSelect={selectFromCatalogue}
      />

      {renderDrawer()}

      <TestEmailModal workspaceId={workspace.id} target={testTarget} onClose={() => setTestTarget(null)} />
    </>
  )
}
