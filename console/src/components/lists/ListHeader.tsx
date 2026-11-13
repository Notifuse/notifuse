import { useQuery } from '@tanstack/react-query'
import { Button, Tooltip } from 'antd'
import { Pencil, SlidersHorizontal } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { templatesApi } from '../../services/api/template'
import type { List, TemplateReference, UserPermissions, Workspace } from '../../services/api/types'
import { KpiStrip, Mono, Pill, useNumberFormat, type KpiItem } from '../layout'
import TemplatePreviewDrawer from '../templates/TemplatePreviewDrawer'
import { CreateTemplateDrawer } from '../templates/CreateTemplateDrawer'
import { LIST_STATUSES, LIST_STAT_KEYS, useListStats, type ListStatus } from './listStats'

export interface ListHeaderProps {
  workspace: Workspace
  list: List
  permissions: UserPermissions | null
  /** The open tab (contact_list_status); absent for All. */
  status?: string
  onStatusChange: (status: ListStatus) => void
  onEditList: () => void
}

/** The KPIs of a list, then its settings in one read-only bar. */
export function ListHeader({
  workspace,
  list,
  permissions,
  status,
  onStatusChange,
  onEditList
}: ListHeaderProps) {
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()
  const { data, isLoading } = useListStats(workspace.id, list.id)
  const stats = data?.stats

  const total = stats ? LIST_STATUSES.reduce((sum, key) => sum + stats[LIST_STAT_KEYS[key]], 0) : 0
  const share = (value: number) => (total > 0 ? percent(value / total) : '—')

  const labels: Record<ListStatus, string> = {
    active: t`Active`,
    pending: t`Pending`,
    unsubscribed: t`Unsubscribed`,
    bounced: t`Bounced`,
    complained: t`Complained`
  }

  const items: KpiItem[] = LIST_STATUSES.map((key) => {
    const value = stats ? stats[LIST_STAT_KEYS[key]] : 0
    const withShare = key !== 'active' && key !== 'pending'
    return {
      key,
      label: labels[key],
      value: int(value),
      sub: withShare && stats ? share(value) : undefined,
      tone: (key === 'bounced' || key === 'complained') && value > 0 ? 'negative' : undefined,
      onClick: () => onStatusChange(key),
      active: status === key
    }
  })

  const canWriteLists = !!permissions?.lists?.write

  return (
    <div className="flex flex-col gap-2">
      <KpiStrip items={items} loading={isLoading} />
      <div className="flex flex-wrap items-center gap-3 rounded-lg border border-line bg-surface px-3 py-2 text-sm">
        <Tooltip title={list.is_public ? t`Visible in the notification center` : undefined}>
          <span className="flex">
            <Pill tone={list.is_public ? 'success' : 'neutral'}>
              {list.is_public ? t`Public` : t`Private`}
            </Pill>
          </span>
        </Tooltip>
        {list.is_double_optin ? (
          <>
            <Pill tone="primary">{t`Double opt-in`}</Pill>
            {list.double_optin_template ? (
              <span className="flex min-w-0 items-center gap-1.5">
                <span className="text-ink-3">{t`Template`}</span>
                <OptInTemplate
                  workspace={workspace}
                  templateRef={list.double_optin_template}
                  canRead={!!permissions?.templates?.read}
                  canEdit={!!permissions?.templates?.write}
                />
              </span>
            ) : null}
          </>
        ) : null}
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="text-ink-3">{t`ID`}</span>
          <Mono copy>{list.id}</Mono>
        </span>
        <div className="ml-auto">
          <Tooltip title={canWriteLists ? undefined : t`You don't have write permission for lists`}>
            <Button
              size="small"
              icon={<SlidersHorizontal size={14} aria-hidden="true" />}
              disabled={!canWriteLists}
              onClick={onEditList}
            >
              {t`Edit list`}
            </Button>
          </Tooltip>
        </div>
      </div>
    </div>
  )
}

interface OptInTemplateProps {
  workspace: Workspace
  templateRef: TemplateReference
  /** templates.get needs templates.read: without it the template's ID is shown. */
  canRead: boolean
  canEdit: boolean
}

/** The confirmation template's name, opening its preview, and a pencil to edit it. */
function OptInTemplate({ workspace, templateRef, canRead, canEdit }: OptInTemplateProps) {
  const { t } = useLingui()
  const { data: template } = useQuery({
    queryKey: ['template', workspace.id, templateRef.id, templateRef.version],
    queryFn: async () => {
      const response = await templatesApi.get({
        workspace_id: workspace.id,
        id: templateRef.id,
        version: templateRef.version
      })
      return response.template
    },
    enabled: canRead,
    staleTime: 1000 * 60 * 5
  })

  if (!template) return <Mono>{templateRef.id}</Mono>

  return (
    <>
      <TemplatePreviewDrawer record={template} workspace={workspace}>
        <Button type="link" size="small">
          {template.name}
        </Button>
      </TemplatePreviewDrawer>
      {canEdit ? (
        <CreateTemplateDrawer
          template={template}
          workspace={workspace}
          canManageSavedBlocks
          buttonContent={<Pencil size={13} aria-hidden="true" />}
          buttonProps={{ type: 'text', size: 'small', 'aria-label': t`Edit template` }}
        />
      ) : null}
    </>
  )
}
