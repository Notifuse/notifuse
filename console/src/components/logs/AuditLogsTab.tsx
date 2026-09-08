import { Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { forwardRef, useImperativeHandle, useRef, useState } from 'react'
import type { AuditScope } from '../../services/api/audit_log'
import { LicenceGateNotice } from '../license/LicenceGateNotice'
import { AuditLogTable, type AuditLogTableHandle } from '../settings/audit_logs/AuditLogTable'
import {
  AuditFilterBuilder,
  EMPTY_AUDIT_FILTERS,
  type AuditFilters
} from '../settings/audit_logs/AuditFilterBuilder'

interface AuditLogsTabProps {
  workspaceId: string
  timezone?: string
  // A root user also gets the deployment-wide view: every workspace plus the
  // entries that belong to none (sign-ins, the licence, the system settings).
  isRoot: boolean
}

// The Logs → Audit logs tab. Browsing lives here; the retention setting stays in
// Settings → Audit logs. The tab bar's Refresh and Export reach the table that
// is showing through the handle.
export const AuditLogsTab = forwardRef<AuditLogTableHandle, AuditLogsTabProps>(function AuditLogsTab(
  { workspaceId, timezone, isRoot },
  ref
) {
  const { t } = useLingui()
  const [scope, setScope] = useState<AuditScope>('workspace')
  const [filters, setFilters] = useState<AuditFilters>(EMPTY_AUDIT_FILTERS)
  const workspaceTable = useRef<AuditLogTableHandle>(null)
  const deploymentTable = useRef<AuditLogTableHandle>(null)

  // Rebuilt on every scope change so the page's tab-bar buttons always act on
  // the table that is showing.
  useImperativeHandle(
    ref,
    () => {
      const activeTable = () => (scope === 'deployment' ? deploymentTable : workspaceTable).current
      return {
        refresh: () => activeTable()?.refresh(),
        exportAs: async (format) => {
          await activeTable()?.exportAs(format)
        }
      }
    },
    [scope]
  )

  return (
    <div className="space-y-4" data-testid="audit-logs-tab">
      <LicenceGateNotice feature="audit_logs" workspaceId={workspaceId} />
      <div className="flex flex-wrap items-center gap-3">
        <AuditFilterBuilder
          filters={filters}
          onChange={(next) => setFilters((previous) => ({ ...previous, ...next }))}
        />
        {isRoot && (
          <Segmented<AuditScope>
            size="small"
            value={scope}
            onChange={(value) => setScope(value)}
            options={[
              { label: t`This workspace`, value: 'workspace' },
              { label: t`Whole deployment`, value: 'deployment' }
            ]}
          />
        )}
      </div>
      {scope === 'workspace' ? (
        <AuditLogTable
          ref={workspaceTable}
          scope="workspace"
          workspaceId={workspaceId}
          timezone={timezone}
          filters={filters}
        />
      ) : (
        <AuditLogTable ref={deploymentTable} scope="deployment" timezone={timezone} filters={filters} />
      )}
    </div>
  )
})
