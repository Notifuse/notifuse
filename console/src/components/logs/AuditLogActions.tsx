import { Button, Dropdown, Space, Tooltip, message } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faRefresh } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import { useState } from 'react'
import { AUDIT_LOG_EXPORT_MAX_ROWS, type AuditExportFormat } from '../../services/api/audit_log'

interface AuditLogActionsProps {
  onRefresh: () => void
  onExport: (format: AuditExportFormat) => Promise<void>
}

// Refresh and Export for the audit tab, rendered in the Logs page's tab bar.
export function AuditLogActions({ onRefresh, onExport }: AuditLogActionsProps) {
  const { t } = useLingui()
  const [exporting, setExporting] = useState(false)

  const exportAs = async (format: AuditExportFormat) => {
    setExporting(true)
    try {
      await onExport(format)
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`Export failed`)
    } finally {
      setExporting(false)
    }
  }

  return (
    <Space>
      <Tooltip title={t`Refresh`}>
        <Button
          type="text"
          size="small"
          icon={<FontAwesomeIcon icon={faRefresh} />}
          aria-label={t`Refresh`}
          onClick={onRefresh}
          className="opacity-70 hover:opacity-100"
        />
      </Tooltip>
      <Dropdown
        menu={{
          items: [
            { key: 'csv', label: 'CSV', onClick: () => exportAs('csv') },
            { key: 'ndjson', label: 'NDJSON', onClick: () => exportAs('ndjson') }
          ]
        }}
      >
        <Tooltip title={t`Export the current filter as CSV or NDJSON, up to ${AUDIT_LOG_EXPORT_MAX_ROWS} rows`}>
          <Button size="small" icon={<DownloadOutlined />} loading={exporting} data-testid="audit-export">
            {t`Export`}
          </Button>
        </Tooltip>
      </Dropdown>
    </Space>
  )
}
