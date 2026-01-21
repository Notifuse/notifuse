import React, { useRef } from 'react'
import { Button, Dropdown, Modal, App } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronDown, faExclamationTriangle } from '@fortawesome/free-solid-svg-icons'
import type { EmailBlock } from '../email_builder/types'
import { EmailBlockClass } from '../email_builder/EmailBlockClass'
import { convertMjmlToJsonBrowser } from '../mjml-converter/mjml-to-json-browser'
import { templatesApi } from '../../services/api/template'

interface ImportExportButtonProps {
  // Import props
  onImport: (tree: EmailBlock, testData?: Record<string, unknown>) => void
  onTestDataImport?: (testData: Record<string, unknown>) => void
  // Export props
  tree: EmailBlock
  testData?: Record<string, unknown>
  // Workspace ID for API calls
  workspaceId: string
}

interface ImportedData {
  emailTree: EmailBlock
  testData?: Record<string, unknown>
  exportedAt?: string
  version?: string
}

export const ImportExportButton: React.FC<ImportExportButtonProps> = ({
  onImport,
  onTestDataImport,
  tree,
  testData,
  workspaceId
}) => {
  const { message } = App.useApp()

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

  // Function to validate imported email tree structure
  const validateEmailTree = (tree: unknown): { isValid: boolean; errors: string[] } => {
    // Basic type and structure validation
    if (!tree || typeof tree !== 'object') {
      return { isValid: false, errors: ['Invalid tree structure: not an object'] }
    }

    // Type guard to check if tree has the required properties
    const hasRequiredProperties = (
      obj: object
    ): obj is { id: unknown; type: unknown; children?: unknown } => {
      return 'id' in obj && 'type' in obj
    }

    if (!hasRequiredProperties(tree)) {
      return { isValid: false, errors: ['Invalid tree structure: missing id or type'] }
    }

    // Check required properties types
    if (typeof tree.id !== 'string' || typeof tree.type !== 'string') {
      return { isValid: false, errors: ['Invalid tree structure: missing id or type'] }
    }

    // Check if it's a valid MJML root
    if (tree.type !== 'mjml') {
      return { isValid: false, errors: ['Invalid tree structure: root must be mjml type'] }
    }

    // Check if children exist and are arrays
    if (tree.children && !Array.isArray(tree.children)) {
      return { isValid: false, errors: ['Invalid tree structure: children must be an array'] }
    }

    // Use comprehensive EmailBlockClass validation
    const structureErrors = EmailBlockClass.validateStructure(tree as EmailBlock)

    // If validation fails, provide more helpful error messages
    if (structureErrors.length > 0) {
      const helpfulErrors = structureErrors.map((error) => {
        // Check for common mj-raw placement issues
        if (error.includes('mj-raw cannot be placed inside mj-wrapper')) {
          return `${error}\n\nℹ️ Note: mj-raw components can only be placed directly in mj-body or mj-head, not inside mj-wrapper. Consider moving the mj-raw content outside the wrapper or converting it to appropriate MJML components.`
        }

        // Check for HTML elements inside mj-raw (which shouldn't happen with our fixed parser)
        if (error.includes('cannot be placed inside mj-raw')) {
          return `${error}\n\nℹ️ Note: This might be caused by an incorrectly structured MJML file. HTML content inside mj-raw should be stored as text content, not as child elements.`
        }

        return error
      })

      return {
        isValid: false,
        errors: helpfulErrors
      }
    }

    return {
      isValid: true,
      errors: []
    }
  }

  // Handle file selection and parsing
  const handleFileUpload = (file: File) => {
    const reader = new FileReader()

    reader.onload = (e) => {
      try {
        const content = e.target?.result as string

        // Check if it's an MJML file
        if (
          file.name.endsWith('.mjml') ||
          file.type === 'text/xml' ||
          file.type === 'application/xml'
        ) {
          handleMjmlContent(content)
          return
        }

        // Handle JSON files
        const parsedData = JSON.parse(content)

        // Check if it's the full export format or just a tree
        let emailTree: EmailBlock
        let testData: Record<string, unknown> | undefined = undefined

        if (parsedData.emailTree) {
          // It's the full export format
          const importedData = parsedData as ImportedData
          emailTree = importedData.emailTree
          testData = importedData.testData
        } else {
          // It's just a tree
          emailTree = parsedData as EmailBlock
        }

        // Validate the email tree
        const validation = validateEmailTree(emailTree)
        if (!validation.isValid) {
          showErrorModal('Import Validation Failed', validation.errors)
          return
        }

        // Import the data
        onImport(emailTree, testData)

        // If test data exists and handler is provided, import it separately
        if (testData && onTestDataImport) {
          onTestDataImport(testData)
        }

        message.success('Template imported successfully')
      } catch (error) {
        console.error('Import failed:', error)

        if (error instanceof Error && error.message.includes('Validation failed:')) {
          // Extract validation errors from the error message
          const errorMessage = error.message.replace('Validation failed: ', '')
          const errors = errorMessage.split(', ')
          showErrorModal('Import Validation Failed', errors)
        } else {
          message.error('Failed to import template. Please check the file format.')
        }
      }
    }

    reader.onerror = () => {
      message.error('Failed to read the file')
    }

    reader.readAsText(file)
  }

  // Handle MJML content parsing
  const handleMjmlContent = (mjmlContent: string) => {
    try {
      // Convert MJML to EmailBlock format using browser-compatible parser
      const emailTree = convertMjmlToJsonBrowser(mjmlContent)

      // Validate the email tree
      const validation = validateEmailTree(emailTree)
      if (!validation.isValid) {
        showErrorModal('MJML Import Validation Failed', validation.errors)
        return
      }

      // Import the converted tree
      onImport(emailTree)

      message.success('MJML template imported and converted successfully')
    } catch (error) {
      console.error('MJML import failed:', error)

      if (error instanceof Error) {
        // Check if it's a syntax error from MJML parsing
        if (
          error.message.includes('Invalid MJML syntax') ||
          error.message.includes('Root element must be')
        ) {
          showErrorModal('MJML Syntax Error', [error.message])
        } else if (error.message.includes('Validation failed:')) {
          // Extract validation errors from the error message
          const errorMessage = error.message.replace('Validation failed: ', '')
          const errors = errorMessage.split(', ')
          showErrorModal('MJML Validation Failed', errors)
        } else {
          showErrorModal('MJML Import Error', [error.message])
        }
      } else {
        message.error('Failed to import MJML template. Please check the MJML syntax.')
      }
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
  const handleFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (file) {
      if (file.type === 'application/json' || file.name.endsWith('.json')) {
        handleFileUpload(file)
      } else {
        message.error('Please select a JSON file')
      }
    }
    // Reset input so same file can be selected again
    event.target.value = ''
  }

  // Handle MJML file input change
  const handleMjmlFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (file) {
      if (
        file.name.endsWith('.mjml') ||
        file.type === 'text/xml' ||
        file.type === 'application/xml' ||
        file.type === 'text/plain'
      ) {
        handleFileUpload(file)
      } else {
        message.error('Please select an MJML file')
      }
    }
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
        showErrorModal('HTML Export Failed', [response.error.message])
        return
      }

      if (response.html) {
        downloadFile(response.html, 'email-template.html', 'text/html')
        message.success('HTML exported successfully')
      } else {
        message.error('No HTML content to export')
      }
    } catch (error) {
      console.error('HTML export failed:', error)
      message.error('Failed to export HTML')
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
        showErrorModal('MJML Export Failed', [response.error.message])
        return
      }

      if (response.mjml) {
        downloadFile(response.mjml, 'email-template.mjml', 'text/xml')
        message.success('MJML exported successfully')
      } else {
        message.error('No MJML content to export')
      }
    } catch (error) {
      console.error('MJML export failed:', error)
      message.error('Failed to export MJML')
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
      downloadFile(jsonContent, 'email-template.json', 'application/json')

      message.success('JSON exported successfully')
    } catch (error) {
      console.error('JSON export failed:', error)
      message.error('Failed to export JSON')
    }
  }

  // Grouped dropdown menu items
  const menuItems = [
    {
      type: 'group' as const,
      label: 'Import',
      children: [
        {
          key: 'import-json',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">JSON</span>
              <span className="text-xs text-gray-500">Load saved template</span>
            </div>
          ),
          onClick: handleImportJSON
        },
        {
          key: 'import-mjml',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">MJML</span>
              <span className="text-xs text-gray-500">Import MJML markup</span>
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
      label: 'Export',
      children: [
        {
          key: 'export-html',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">HTML</span>
              <span className="text-xs text-gray-500">Ready to send</span>
            </div>
          ),
          onClick: handleExportHTML
        },
        {
          key: 'export-mjml',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">MJML</span>
              <span className="text-xs text-gray-500">Editable markup</span>
            </div>
          ),
          onClick: handleExportMJML
        },
        {
          key: 'export-json',
          label: (
            <div className="flex flex-col">
              <span className="font-medium">JSON</span>
              <span className="text-xs text-gray-500">Save for later import</span>
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
          <span>Import / Export</span>
          <FontAwesomeIcon icon={faChevronDown} className="ml-1" size="sm" />
        </Button>
      </Dropdown>

      {/* Hidden file input for JSON */}
      <input
        ref={fileInputRef}
        type="file"
        accept=".json,application/json"
        style={{ display: 'none' }}
        onChange={handleFileInputChange}
      />

      {/* Hidden file input for MJML */}
      <input
        ref={mjmlFileInputRef}
        type="file"
        accept=".mjml,.xml,text/xml,application/xml,text/plain"
        style={{ display: 'none' }}
        onChange={handleMjmlFileInputChange}
      />

      {/* Error Modal */}
      <Modal
        title={
          <div className="flex items-center gap-2">
            <FontAwesomeIcon icon={faExclamationTriangle} className="text-red-500" />
            <span>{errorTitle}</span>
          </div>
        }
        open={isErrorModalVisible}
        onCancel={() => setIsErrorModalVisible(false)}
        footer={[
          <Button key="ok" type="primary" onClick={() => setIsErrorModalVisible(false)}>
            OK
          </Button>
        ]}
        width={600}
      >
        <div className="space-y-3">
          <p className="text-gray-600">
            The following validation errors were found in your import:
          </p>

          <div className="bg-red-50 border border-red-200 rounded-md p-3 max-h-60 overflow-y-auto">
            <ul className="space-y-2">
              {validationErrors.map((error, index) => (
                <li key={index} className="flex items-start gap-2">
                  <span className="text-red-500 font-bold mt-0.5">•</span>
                  <span className="text-red-700 text-sm">{error}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="text-sm text-gray-500">
            <p className="font-medium">Tips for fixing these errors:</p>
            <ul className="mt-1 ml-4 space-y-1">
              <li>• Check that your MJML structure follows the proper hierarchy</li>
              <li>• Ensure all required components are properly nested</li>
              <li>• Verify that component attributes are valid</li>
              <li>• Make sure all XML tags are properly closed</li>
            </ul>
          </div>
        </div>
      </Modal>
    </div>
  )
}
