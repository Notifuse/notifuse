import { useState } from 'react'
import { App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { Integration, Workspace } from '../../../services/api/types'
import { workspaceService } from '../../../services/api/workspace'
import dayjs from '../../../lib/dayjs'

/** The integrations whose drawer body builds the whole record, settings included. */
export type BuiltIntegrationType = 'llm' | 'firecrawl' | 'supabase'

export interface SaveIntegrationOptions {
  /** Updates the integration when true, creates it otherwise. */
  editing: boolean
  success: string
  failure: string
  /** Runs once the integration is saved and the workspace refreshed: the drawer closes. */
  onSaved: () => void
}

export interface DeleteConfirmTexts {
  title: string
  content: string
}

// A record can be hand-written into the integrations column and carry no usable created_at, and an
// invalid date formats as the literal "Invalid Date" rather than throwing. The day is in the
// console's language.
export const formatConnectedOn = (value?: string): string => {
  const at = value ? dayjs(value) : null
  return at?.isValid() ? at.format('ll') : '—'
}

/**
 * What the settings pages that list integrations share: refreshing the workspace after a change,
 * saving an integration built by its drawer, and deleting one after a confirmation.
 */
export function useIntegrationActions(
  workspace: Workspace | null,
  onSave: (updatedWorkspace: Workspace) => Promise<void>
) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const [saving, setSaving] = useState(false)
  const workspaceId = workspace?.id ?? ''

  const refreshWorkspace = async () => {
    const response = await workspaceService.get(workspaceId)
    await onSave(response.workspace)
  }

  // Create or update, refresh, then onSaved. Rethrows so the drawer body keeps its form open on
  // failure.
  const saveIntegration = async (
    type: BuiltIntegrationType,
    integration: Integration,
    { editing, success, failure, onSaved }: SaveIntegrationOptions
  ) => {
    setSaving(true)
    try {
      const settings =
        type === 'llm'
          ? { llm_provider: integration.llm_provider }
          : type === 'firecrawl'
            ? { firecrawl_settings: integration.firecrawl_settings }
            : { supabase_settings: integration.supabase_settings }
      if (editing) {
        await workspaceService.updateIntegration({
          workspace_id: workspaceId,
          integration_id: integration.id,
          name: integration.name,
          ...settings
        })
      } else {
        await workspaceService.createIntegration({
          workspace_id: workspaceId,
          name: integration.name,
          type,
          ...settings
        })
      }
      await refreshWorkspace()
      onSaved()
      message.success(success)
    } catch (error) {
      console.error(`Error saving ${type} integration:`, error)
      message.error(failure)
      throw error
    } finally {
      setSaving(false)
    }
  }

  const deleteIntegration = async (integrationId: string) => {
    try {
      await workspaceService.deleteIntegration({ workspace_id: workspaceId, integration_id: integrationId })
      await refreshWorkspace()
      message.success(t`Integration deleted successfully`)
    } catch (error) {
      console.error('Error deleting integration', error)
      message.error(t`Failed to delete integration`)
    }
  }

  const confirmDelete = (integrationId: string, texts?: DeleteConfirmTexts) =>
    modal.confirm({
      title: texts?.title ?? t`Delete this integration?`,
      content: texts?.content ?? t`This action cannot be undone.`,
      okText: t`Delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      onOk: () => deleteIntegration(integrationId)
    })

  return { saving, setSaving, refreshWorkspace, saveIntegration, deleteIntegration, confirmDelete }
}
