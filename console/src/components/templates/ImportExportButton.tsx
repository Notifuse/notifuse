import React, { useRef } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Dropdown, App } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronDown } from '@fortawesome/free-solid-svg-icons'
import type { EmailBlock } from '../email_builder/types'
import { templatesApi } from '../../services/api/template'
import { importTemplateFile, type TemplateFileKind } from './importTemplateFile'
import { ImportErrorsModal } from './ImportErrorsModal'

interface ImportExportButtonProps {
  // Import props
  onImport: (tree: EmailBlock, testData?: Record<string, unknown>) => void
  onTestDataImport?: (testData: Record<string, unknown>) => void
  // Export props
  tree: EmailBlock
  testData?: Record<string, unknown>
  // Workspace ID for API calls
  workspaceId: string
  // Template name used as the download filename
  templateName?: string
}

// Sanitize a template name for use as a filename
const sanitizeFilename = (name: string): string => {
  return name
    .replace(/[/\\?%*:|"<>]/g, '-') // replace illegal filename chars
    .replace(/\s+/g, '-') // replace spaces with dashes
    .replace(/-+/g, '-') // collapse multiple dashes
    .replace(/^-|-$/g, '') // trim leading/trailing dashes
    .toLowerCase()
}

export const ImportExportButton: React.FC<ImportExportButtonProps> = ({
  onImport,
  onTestDataImport,
  tree,
  testData,
  workspaceId,
  templateName
}) => {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()

  const fileInputRef = useRef<HTMLInputElement>(null)
  const mjmlFileInputRef = useRef<HTMLInputElement>(null)
  const [isErrorModalVisible, setIsErrorModalVisible] = React.useState(false)
  const [validationErrors, setValidationErrors] = React.useState<string[]>([])
  const [errorTitle, setErrorTitle] = React.useState<string>('')

  // === IMPORT FUNCTIONS ===

  // Function to show validation errors in a modal
  const showErrorModal = (title: string, errors: string[]) => {
    setErrorTitle(title)
    setValidationErrors(errors)
    setIsErrorModalVisible(true)
  }

  // Read, check and hand over a picked file; the checks live in importTemplateFile.
  const handleFile = async (file: File, kind: TemplateFileKind) => {
    const result = await importTemplateFile(file, kind, i18n)
    if (!result.ok) {
      if (result.title) showErrorModal(result.title, result.errors)
      else message.error(result.errors[0])
      return
    }
    onImport(result.tree, result.testData)
    // If test data exists and handler is provided, import it separately
    if (result.testData && onTestDataImport) {
      onTestDataImport(result.testData)
    }
    message.success(
      kind === 'mjml'
        ? t`MJML template imported and converted successfully`
        : t`Template imported successfully`
    )
    for (const warning of result.warnings) {
      modal.warning({ title: warning.title, content: warning.content })
    }
  }

  // Handle JSON import
  const handleImportJSON = () => {
    if (fileInputRef.current) {
      fileInputRef.current.click()
    }
  }

  // Handle MJML import
  const handleImportMJML = () => {
    if (mjmlFileInputRef.current) {
      mjmlFileInputRef.current.click()
    }
  }

  // Handle file input change
  const handleFileInputChange = (
    event: React.ChangeEvent<HTMLInputElement>,
    kind: TemplateFileKind
  ) => {
    const file = event.target.files?.[0]
    if (file) void handleFile(file, kind)
    // Reset input so same file can be selected again
    event.target.value = ''
  }

  // === EXPORT FUNCTIONS ===

  // Function to download a file
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

  // Export HTML
  const handleExportHTML = async () => {
    try {
      const response = await templatesApi.compile({
        workspace_id: workspaceId,
        message_id: 'export',
        visual_editor_tree: tree as EmailBlock,
        test_data: testData || {}
      })

      if (response.error) {
        showErrorModal(t`HTML Export Failed`, [response.error.message])
        return
      }

      if (response.html) {
        const baseName = templateName ? sanitizeFilename(templateName) : 'email-template'
        downloadFile(response.html, `${baseName}.html`, 'text/html')
        message.success(t`HTML exported successfully`)
      } else {
        message.error(t`No HTML content to export`)
      }
    } catch (error) {
      console.error('HTML export failed:', error)
      message.error(t`Failed to export HTML`)
    }
  }

  // Export MJML
  const handleExportMJML = async () => {
    try {
      const response = await templatesApi.compile({
        workspace_id: workspaceId,
        message_id: 'export',
        visual_editor_tree: tree as EmailBlock,
        test_data: testData || {},
        preserve_liquid: true // Preserve Liquid syntax like {{contact.external_id}} in MJML export
      })

      if (response.error) {
        showErrorModal(t`MJML Export Failed`, [response.error.message])
        return
      }

      if (response.mjml) {
        const baseName = templateName ? sanitizeFilename(templateName) : 'email-template'
        downloadFile(response.mjml, `${baseName}.mjml`, 'text/xml')
        message.success(t`MJML exported successfully`)
      } else {
        message.error(t`No MJML content to export`)
      }
    } catch (error) {
      console.error('MJML export failed:', error)
      message.error(t`Failed to export MJML`)
    }
  }

  // Export JSON
  const handleExportJSON = () => {
    try {
      const exportData = {
        emailTree: tree,
        testData: testData || null,
        exportedAt: new Date().toISOString(),
        version: '1.0'
      }
      const jsonContent = JSON.stringify(exportData, null, 2)
      const baseName = templateName ? sanitizeFilename(templateName) : 'email-template'
      downloadFile(jsonContent, `${baseName}.json`, 'application/json')

      message.success(t`JSON exported successfully`)
    } catch (error) {
      console.error('JSON export failed:', error)
      message.error(t`Failed to export JSON`)
    }
  }

  // Grouped dropdown menu items
  const menuItems = [
    {
      type: 'group' as const,
      label: t`Import`,
      children: [
        {
          key: 'import-json',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">{t`JSON`}</span>
              <span className="text-xs text-gray-500">{t`Load saved template`}</span>
            </div>
          ),
          onClick: handleImportJSON
        },
        {
          key: 'import-mjml',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">{t`MJML`}</span>
              <span className="text-xs text-gray-500">{t`Import MJML markup`}</span>
            </div>
          ),
          onClick: handleImportMJML
        }
      ]
    },
    {
      type: 'divider' as const
    },
    {
      type: 'group' as const,
      label: t`Export`,
      children: [
        {
          key: 'export-html',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">{t`HTML`}</span>
              <span className="text-xs text-gray-500">{t`Ready to send`}</span>
            </div>
          ),
          onClick: handleExportHTML
        },
        {
          key: 'export-mjml',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">{t`MJML`}</span>
              <span className="text-xs text-gray-500">{t`Editable markup`}</span>
            </div>
          ),
          onClick: handleExportMJML
        },
        {
          key: 'export-json',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">{t`JSON`}</span>
              <span className="text-xs text-gray-500">{t`Save for later import`}</span>
            </div>
          ),
          onClick: handleExportJSON
        }
      ]
    }
  ]

  return (
    <div style={{ height: '24px' }}>
      <Dropdown menu={{ items: menuItems }} placement="bottomRight" trigger={['click']}>
        <Button size="small" type="primary" ghost>
          <span>{t`Import / Export`}</span>
          <FontAwesomeIcon icon={faChevronDown} className="ml-1" size="sm" />
        </Button>
      </Dropdown>

      {/* Hidden file input for JSON */}
      <input
        ref={fileInputRef}
        type="file"
        accept=".json,application/json"
        style={{ display: 'none' }}
        onChange={(event) => handleFileInputChange(event, 'json')}
      />

      {/* Hidden file input for MJML */}
      <input
        ref={mjmlFileInputRef}
        type="file"
        accept=".mjml,.xml,text/xml,application/xml,text/plain"
        style={{ display: 'none' }}
        onChange={(event) => handleFileInputChange(event, 'mjml')}
      />

      <ImportErrorsModal
        open={isErrorModalVisible}
        title={errorTitle}
        errors={validationErrors}
        onClose={() => setIsErrorModalVisible(false)}
      />
    </div>
  )
}
