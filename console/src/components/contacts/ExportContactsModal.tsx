import React, { useState, useRef } from 'react'
import { Modal, Button, Tag, Space, Alert, Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'
import Papa from 'papaparse'
import { ContactsSearch } from '../../router'
import { Workspace, List, Segment } from '../../services/api/types'
import { contactsApi, Contact, ListContactsRequest } from '../../services/api/contacts'
import { getCustomFieldLabel } from '../../hooks/useCustomFieldLabel'

interface ExportContactsModalProps {
  visible: boolean
  onCancel: () => void
  workspaceId: string
  workspace: Workspace | undefined
  filters: ContactsSearch
  segmentsData: Segment[]
  listsData: List[]
}

export function ExportContactsModal({
  visible,
  onCancel,
  workspaceId,
  workspace,
  filters,
  segmentsData,
  listsData
}: ExportContactsModalProps) {
  const { t } = useLingui()
  const [isExporting, setIsExporting] = useState(false)
  const [fetchedCount, setFetchedCount] = useState(0)
  const [startTime, setStartTime] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  // Get display label for a filter value
  const getFilterDisplayValue = (key: string, value: string): string => {
    if (key === 'list_id') {
      const list = listsData.find((l) => l.id === value)
      return list?.name || value
    }
    return value
  }

  // Build list of active filters for display
  const getActiveFilters = () => {
    const activeFilters: { key: string; label: string; value: string }[] = []

    if (filters.email) {
      activeFilters.push({ key: 'email', label: t`Email`, value: filters.email })
    }
    if (filters.external_id) {
      activeFilters.push({ key: 'external_id', label: t`External ID`, value: filters.external_id })
    }
    if (filters.first_name) {
      activeFilters.push({ key: 'first_name', label: t`First Name`, value: filters.first_name })
    }
    if (filters.last_name) {
      activeFilters.push({ key: 'last_name', label: t`Last Name`, value: filters.last_name })
    }
    if (filters.phone) {
      activeFilters.push({ key: 'phone', label: t`Phone`, value: filters.phone })
    }
    if (filters.country) {
      activeFilters.push({ key: 'country', label: t`Country`, value: filters.country })
    }
    if (filters.language) {
      activeFilters.push({ key: 'language', label: t`Language`, value: filters.language })
    }
    if (filters.list_id) {
      activeFilters.push({
        key: 'list_id',
        label: t`List`,
        value: getFilterDisplayValue('list_id', filters.list_id)
      })
    }
    if (filters.contact_list_status) {
      activeFilters.push({
        key: 'contact_list_status',
        label: t`List Status`,
        value: filters.contact_list_status
      })
    }
    if (filters.segments && filters.segments.length > 0) {
      filters.segments.forEach((segmentId) => {
        const segment = segmentsData.find((s) => s.id === segmentId)
        activeFilters.push({
          key: 'segment',
          label: t`Segment`,
          value: segment?.name || segmentId
        })
      })
    }

    return activeFilters
  }

  // Download file helper
  const downloadFile = (content: string, filename: string) => {
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  }

  // Format elapsed time
  const formatElapsedTime = (ms: number): string => {
    const seconds = Math.floor(ms / 1000)
    if (seconds < 60) {
      return seconds !== 1 ? t`${seconds} seconds` : t`${seconds} second`
    }
    const minutes = Math.floor(seconds / 60)
    const remainingSeconds = seconds % 60
    return t`${minutes}m ${remainingSeconds}s`
  }

  // Handle export
  const handleExport = async () => {
    setIsExporting(true)
    setFetchedCount(0)
    setStartTime(Date.now())
    setError(null)

    abortControllerRef.current = new AbortController()
    const allContacts: Contact[] = []
    let cursor: string | undefined = undefined

    try {
      // Fetch all contacts with pagination
      while (true) {
        // Check if aborted
        if (abortControllerRef.current?.signal.aborted) {
          break
        }

        const request: ListContactsRequest = {
          workspace_id: workspaceId,
          cursor,
          limit: 100,
          email: filters.email,
          external_id: filters.external_id,
          first_name: filters.first_name,
          last_name: filters.last_name,
          phone: filters.phone,
          country: filters.country,
          language: filters.language,
          list_id: filters.list_id,
          contact_list_status: filters.contact_list_status,
          segments: filters.segments,
          with_contact_lists: false // Skip for performance
        }

        const response = await contactsApi.list(request)

        // Check if aborted after fetch
        if (abortControllerRef.current?.signal.aborted) {
          break
        }

        allContacts.push(...response.contacts)
        setFetchedCount(allContacts.length)

        if (!response.next_cursor) {
          break
        }
        cursor = response.next_cursor
      }

      // If aborted, don't generate CSV
      if (abortControllerRef.current?.signal.aborted) {
        return
      }

      // Check if there are any contacts
      if (allContacts.length === 0) {
        setError(t`No contacts match the current filters`)
        setIsExporting(false)
        return
      }

      // Generate CSV headers
      const headers = [
        'email',
        'external_id',
        'first_name',
        'last_name',
        'full_name',
        'phone',
        'timezone',
        'language',
        'country',
        'address_line_1',
        'address_line_2',
        'state',
        'postcode',
        'job_title',
        getCustomFieldLabel('custom_string_1', workspace),
        getCustomFieldLabel('custom_string_2', workspace),
        getCustomFieldLabel('custom_string_3', workspace),
        getCustomFieldLabel('custom_string_4', workspace),
        getCustomFieldLabel('custom_string_5', workspace),
        getCustomFieldLabel('custom_number_1', workspace),
        getCustomFieldLabel('custom_number_2', workspace),
        getCustomFieldLabel('custom_number_3', workspace),
        getCustomFieldLabel('custom_number_4', workspace),
        getCustomFieldLabel('custom_number_5', workspace),
        getCustomFieldLabel('custom_datetime_1', workspace),
        getCustomFieldLabel('custom_datetime_2', workspace),
        getCustomFieldLabel('custom_datetime_3', workspace),
        getCustomFieldLabel('custom_datetime_4', workspace),
        getCustomFieldLabel('custom_datetime_5', workspace),
        getCustomFieldLabel('custom_json_1', workspace),
        getCustomFieldLabel('custom_json_2', workspace),
        getCustomFieldLabel('custom_json_3', workspace),
        getCustomFieldLabel('custom_json_4', workspace),
        getCustomFieldLabel('custom_json_5', workspace),
        'created_at',
        'updated_at'
      ]

      // Generate CSV data
      const data = allContacts.map((c) => [
        c.email,
        c.external_id || '',
        c.first_name || '',
        c.last_name || '',
        c.full_name || '',
        c.phone || '',
        c.timezone || '',
        c.language || '',
        c.country || '',
        c.address_line_1 || '',
        c.address_line_2 || '',
        c.state || '',
        c.postcode || '',
        c.job_title || '',
        c.custom_string_1 || '',
        c.custom_string_2 || '',
        c.custom_string_3 || '',
        c.custom_string_4 || '',
        c.custom_string_5 || '',
        c.custom_number_1 !== undefined ? String(c.custom_number_1) : '',
        c.custom_number_2 !== undefined ? String(c.custom_number_2) : '',
        c.custom_number_3 !== undefined ? String(c.custom_number_3) : '',
        c.custom_number_4 !== undefined ? String(c.custom_number_4) : '',
        c.custom_number_5 !== undefined ? String(c.custom_number_5) : '',
        c.custom_datetime_1 || '',
        c.custom_datetime_2 || '',
        c.custom_datetime_3 || '',
        c.custom_datetime_4 || '',
        c.custom_datetime_5 || '',
        c.custom_json_1 ? JSON.stringify(c.custom_json_1) : '',
        c.custom_json_2 ? JSON.stringify(c.custom_json_2) : '',
        c.custom_json_3 ? JSON.stringify(c.custom_json_3) : '',
        c.custom_json_4 ? JSON.stringify(c.custom_json_4) : '',
        c.custom_json_5 ? JSON.stringify(c.custom_json_5) : '',
        c.created_at,
        c.updated_at
      ])

      // Generate CSV using PapaParse
      const csv = Papa.unparse({
        fields: headers,
        data
      })

      // Generate filename with date
      const date = new Date().toISOString().split('T')[0]
      const filename = `contacts_export_${date}.csv`

      // Download the file
      downloadFile(csv, filename)

      // Close modal on success
      handleClose()
    } catch (err) {
      if (!abortControllerRef.current?.signal.aborted) {
        setError(err instanceof Error ? err.message : t`An error occurred during export`)
      }
    } finally {
      if (!abortControllerRef.current?.signal.aborted) {
        setIsExporting(false)
      }
    }
  }

  // Handle cancel/close
  const handleClose = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
    setIsExporting(false)
    setFetchedCount(0)
    setStartTime(null)
    setError(null)
    onCancel()
  }

  const activeFilters = getActiveFilters()
  const elapsedTime = startTime ? Date.now() - startTime : 0

  return (
    <Modal
      title={t`Export Contacts to CSV`}
      open={visible}
      onCancel={handleClose}
      footer={[
        <Button key="cancel" onClick={handleClose}>
          {t`Cancel`}
        </Button>,
        <Button
          key="export"
          type="primary"
          onClick={handleExport}
          loading={isExporting}
          disabled={isExporting}
        >
          {t`Export`}
        </Button>
      ]}
    >
      <div className="py-4">
        {/* Filter summary */}
        <div className="mb-4">
          <div className="text-sm font-medium mb-2">{t`Active filters`}:</div>
          {activeFilters.length > 0 ? (
            <Space wrap>
              {activeFilters.map((filter, index) => (
                <Tag key={`${filter.key}-${index}`} color="blue">
                  {filter.label}: {filter.value}
                </Tag>
              ))}
            </Space>
          ) : (
            <div className="text-gray-500">{t`Exporting all contacts`}</div>
          )}
        </div>

        {/* Progress display */}
        {isExporting && (
          <div className="my-4">
            <Space align="center">
              <Spin size="small" />
              <span>{t`Fetched ${fetchedCount.toLocaleString()} contacts...`}</span>
            </Space>
            {elapsedTime > 0 && (
              <div className="text-gray-500 text-sm mt-2">
                {t`Elapsed`}: {formatElapsedTime(elapsedTime)}
              </div>
            )}
          </div>
        )}

        {/* Error display */}
        {error && (
          <Alert message={error} type={error.includes('No contacts') ? 'info' : 'error'} showIcon />
        )}
      </div>
    </Modal>
  )
}
