import React, { useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { useQueryClient } from '@tanstack/react-query'
import { Button, Modal, App, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFileExport, faFileImport, faTriangleExclamation } from '@fortawesome/free-solid-svg-icons'
import {
  transactionalNotificationsApi,
  TransactionalNotification
} from '../../services/api/transactional_notifications'
import { templatesApi, Template } from '../../services/api/template'
import { ApiError } from '../../services/api/client'

// Bundle format written to / read from the exported JSON file (a single notification).
const BUNDLE_TYPE = 'transactional-notification'
// Collection format wrapping many notifications in a single file.
const COLLECTION_TYPE = 'transactional-notification-collection'
const BUNDLE_VERSION = '1.0'

// A notification plus its (optional) email template, self-contained for re-import.
interface NotificationEntry {
  notification: TransactionalNotification
  template: Template | null
}

interface TransactionalBundle extends NotificationEntry {
  type: typeof BUNDLE_TYPE
  version: string
  exportedAt: string
}

interface TransactionalCollection {
  type: typeof COLLECTION_TYPE
  version: string
  exportedAt: string
  notifications: NotificationEntry[]
}

// Sanitize a name for use as a download filename.
const sanitizeFilename = (name: string): string => {
  const cleaned = name
    .replace(/[/\\?%*:|"<>]/g, '-')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .toLowerCase()
  return cleaned || 'transactional-notification'
}

// Trigger a browser download for the given text content.
const downloadFile = (content: string, filename: string, contentType: string) => {
  const blob = new Blob([content], { type: contentType })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

// Returns the existing resource if found, or null on a 404. Other errors propagate.
const getOrNull = async <T,>(promise: Promise<T>): Promise<T | null> => {
  try {
    return await promise
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return null
    }
    throw error
  }
}

// Fetch one notification and its email template as a self-contained entry.
// The template is managed separately in the UI but bundled here so an import recreates both.
const buildEntry = async (
  workspaceId: string,
  notificationId: string
): Promise<NotificationEntry> => {
  const notificationResponse = await transactionalNotificationsApi.get({
    workspace_id: workspaceId,
    id: notificationId
  })

  let template: Template | null = null
  const templateId = notificationResponse.notification.channels?.email?.template_id
  if (templateId) {
    const templateResponse = await templatesApi.get({
      workspace_id: workspaceId,
      id: templateId
    })
    template = templateResponse.template
  }

  return { notification: notificationResponse.notification, template }
}

// === SINGLE EXPORT (per-card) ===

interface ExportNotificationButtonProps {
  workspaceId: string
  notification: TransactionalNotification
}

export const ExportNotificationButton: React.FC<ExportNotificationButtonProps> = ({
  workspaceId,
  notification
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [loading, setLoading] = useState(false)

  const handleExport = async () => {
    setLoading(true)
    try {
      const entry = await buildEntry(workspaceId, notification.id)
      const bundle: TransactionalBundle = {
        type: BUNDLE_TYPE,
        version: BUNDLE_VERSION,
        exportedAt: new Date().toISOString(),
        ...entry
      }
      downloadFile(
        JSON.stringify(bundle, null, 2),
        `${sanitizeFilename(notification.name)}.json`,
        'application/json'
      )
      message.success(t`Notification exported successfully`)
    } catch (error) {
      console.error('Transactional export failed:', error)
      message.error(t`Failed to export notification`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Tooltip title={t`Export`}>
      <Button type="text" size="small" loading={loading} onClick={handleExport}>
        <FontAwesomeIcon icon={faFileExport} style={{ opacity: 0.7 }} />
      </Button>
    </Tooltip>
  )
}

// === BULK EXPORT (selected notifications) ===

interface ExportSelectedButtonProps {
  workspaceId: string
  // Notifications currently selected on the page (id + name for the filename).
  selected: { id: string; name: string }[]
  onExported?: () => void
}

// Exports the selected notifications into a single collection file wrapping every entry.
export const ExportSelectedButton: React.FC<ExportSelectedButtonProps> = ({
  workspaceId,
  selected,
  onExported
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [loading, setLoading] = useState(false)

  const handleExport = async () => {
    setLoading(true)
    try {
      const entries = await Promise.all(selected.map((n) => buildEntry(workspaceId, n.id)))
      const collection: TransactionalCollection = {
        type: COLLECTION_TYPE,
        version: BUNDLE_VERSION,
        exportedAt: new Date().toISOString(),
        notifications: entries
      }
      const stamp = new Date().toISOString().slice(0, 10)
      downloadFile(
        JSON.stringify(collection, null, 2),
        `transactional-notifications-${stamp}.json`,
        'application/json'
      )
      message.success(t`Exported ${entries.length} notifications`)
      onExported?.()
    } catch (error) {
      console.error('Transactional bulk export failed:', error)
      message.error(t`Failed to export notifications`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Button loading={loading} disabled={selected.length === 0} onClick={handleExport}>
      <FontAwesomeIcon icon={faFileExport} className="mr-2" />
      {t`Export selected (${selected.length})`}
    </Button>
  )
}

// === IMPORT ===

interface ImportNotificationButtonProps {
  workspaceId: string
  disabled?: boolean
}

// Validate a single notification entry (notification present with id + name).
const isValidEntry = (raw: unknown): raw is NotificationEntry => {
  if (!raw || typeof raw !== 'object') return false
  const obj = raw as Record<string, unknown>
  const notification = obj.notification as TransactionalNotification | undefined
  return (
    !!notification &&
    typeof notification.id === 'string' &&
    typeof notification.name === 'string'
  )
}

// Parse a file's content into one or more entries. Accepts a single bundle or a
// collection wrapping many notifications.
const parseEntries = (parsed: unknown): { entries?: NotificationEntry[]; error?: string } => {
  if (!parsed || typeof parsed !== 'object') {
    return { error: 'Invalid file: not a JSON object' }
  }
  const obj = parsed as Record<string, unknown>

  // Collection file: { type: 'transactional-notification-collection', notifications: [...] }
  if (obj.type === COLLECTION_TYPE || Array.isArray(obj.notifications)) {
    const rawList = obj.notifications
    if (!Array.isArray(rawList) || rawList.length === 0) {
      return { error: 'Invalid file: collection has no notifications' }
    }
    if (!rawList.every(isValidEntry)) {
      return { error: 'Invalid file: a notification entry is malformed' }
    }
    return { entries: rawList }
  }

  // Single bundle file: { type: 'transactional-notification', notification, template }
  if (obj.type === BUNDLE_TYPE) {
    if (!isValidEntry(obj)) {
      return { error: 'Invalid file: missing notification data' }
    }
    return { entries: [{ notification: obj.notification, template: (obj.template as Template) ?? null }] }
  }

  return { error: 'Invalid file: not a transactional notification export' }
}

export const ImportNotificationButton: React.FC<ImportNotificationButtonProps> = ({
  workspaceId,
  disabled
}) => {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [loading, setLoading] = useState(false)

  const triggerFilePicker = () => fileInputRef.current?.click()

  // Upsert one entry: template first (so the notification's channel reference is valid),
  // then the notification. Uses update when the id already exists, create otherwise.
  const importEntry = async (entry: NotificationEntry) => {
    if (entry.template) {
      const tpl = entry.template
      const existing = await getOrNull(templatesApi.get({ workspace_id: workspaceId, id: tpl.id }))
      const templatePayload = {
        workspace_id: workspaceId,
        id: tpl.id,
        name: tpl.name,
        channel: tpl.channel,
        email: tpl.email,
        web: tpl.web,
        category: tpl.category,
        template_macro_id: tpl.template_macro_id,
        test_data: tpl.test_data,
        settings: tpl.settings,
        translations: tpl.translations
      }
      if (existing) {
        await templatesApi.update(templatePayload)
      } else {
        await templatesApi.create(templatePayload)
      }
    }

    const n = entry.notification
    const existingNotification = await getOrNull(
      transactionalNotificationsApi.get({ workspace_id: workspaceId, id: n.id })
    )
    if (existingNotification) {
      await transactionalNotificationsApi.update({
        workspace_id: workspaceId,
        id: n.id,
        updates: {
          name: n.name,
          description: n.description,
          channels: n.channels,
          tracking_settings: n.tracking_settings,
          metadata: n.metadata
        }
      })
    } else {
      await transactionalNotificationsApi.create({
        workspace_id: workspaceId,
        notification: {
          id: n.id,
          name: n.name,
          description: n.description,
          // Integration-managed notifications import as plain notifications;
          // integration_id is intentionally dropped (cannot be reattached here).
          channels: n.channels,
          tracking_settings: n.tracking_settings,
          metadata: n.metadata
        }
      })
    }
  }

  // Import every entry sequentially, reporting a per-batch success/failure summary.
  const performImport = async (entries: NotificationEntry[]) => {
    setLoading(true)
    let succeeded = 0
    const failed: string[] = []
    for (const entry of entries) {
      try {
        await importEntry(entry)
        succeeded++
      } catch (error) {
        console.error('Transactional import failed for', entry.notification.id, error)
        failed.push(entry.notification.name || entry.notification.id)
      }
    }
    setLoading(false)

    if (succeeded > 0) {
      queryClient.invalidateQueries({ queryKey: ['transactional-notifications', workspaceId] })
      message.success(t`Imported ${succeeded} notification(s)`)
    }
    if (failed.length > 0) {
      Modal.error({
        title: t`Some imports failed`,
        content: (
          <ul className="mt-2 ml-4 list-disc">
            {failed.map((name, i) => (
              <li key={i}>{name}</li>
            ))}
          </ul>
        )
      })
    }
  }

  // Detect existing notifications/templates across all entries and confirm before overwriting.
  const confirmAndImport = async (entries: NotificationEntry[]) => {
    setLoading(true)
    const conflicts: string[] = []
    try {
      for (const entry of entries) {
        const existingNotification = (
          await getOrNull(
            transactionalNotificationsApi.get({
              workspace_id: workspaceId,
              id: entry.notification.id
            })
          )
        )?.notification
        if (existingNotification) {
          conflicts.push(
            t`Notification "${existingNotification.name}" (${entry.notification.id})`
          )
        }

        if (entry.template) {
          const existingTemplate = (
            await getOrNull(
              templatesApi.get({ workspace_id: workspaceId, id: entry.template.id })
            )
          )?.template
          if (existingTemplate) {
            conflicts.push(t`Template "${existingTemplate.name}" (${entry.template.id})`)
          }
        }
      }
    } catch (error) {
      console.error('Transactional import conflict check failed:', error)
      message.error(t`Failed to read existing data`)
      setLoading(false)
      return
    }
    setLoading(false)

    if (conflicts.length === 0) {
      await performImport(entries)
      return
    }

    modal.confirm({
      title: t`Overwrite existing data?`,
      icon: <FontAwesomeIcon icon={faTriangleExclamation} className="text-orange-500 mr-2" />,
      width: 520,
      content: (
        <div>
          <p>{t`The following already exist and will be updated:`}</p>
          <ul className="mt-2 ml-4 list-disc max-h-60 overflow-y-auto">
            {conflicts.map((c, i) => (
              <li key={i}>{c}</li>
            ))}
          </ul>
        </div>
      ),
      okText: t`Yes, Update`,
      cancelText: t`Cancel`,
      onOk: () => performImport(entries)
    })
  }

  const handleFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = '' // allow re-selecting the same file
    if (!file) return

    const reader = new FileReader()
    reader.onload = (e) => {
      try {
        const parsed = JSON.parse(e.target?.result as string)
        const { entries, error } = parseEntries(parsed)
        if (error || !entries) {
          Modal.error({
            title: t`Import Failed`,
            content: error || t`Invalid file format`
          })
          return
        }
        confirmAndImport(entries)
      } catch (err) {
        console.error('Failed to parse import file:', err)
        Modal.error({
          title: t`Import Failed`,
          content: t`Could not parse the file. Please select a valid JSON export.`
        })
      }
    }
    reader.onerror = () => message.error(t`Failed to read the file`)
    reader.readAsText(file)
  }

  return (
    <>
      <Button loading={loading} disabled={disabled} onClick={triggerFilePicker}>
        <FontAwesomeIcon icon={faFileImport} className="mr-2" />
        {t`Import`}
      </Button>
      <input
        ref={fileInputRef}
        type="file"
        accept=".json,application/json"
        style={{ display: 'none' }}
        onChange={handleFileInputChange}
      />
    </>
  )
}
