import React, { useState, useRef, useEffect } from 'react'
import {
  Button,
  Drawer,
  Upload,
  Select,
  Progress,
  Space,
  Typography,
  Alert,
  message,
  Modal,
  Tag
} from 'antd'
import {
  UploadOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  CloseCircleOutlined,
  UserAddOutlined
} from '@ant-design/icons'
import type { UploadProps } from 'antd'
import Papa from 'papaparse'
import type { ParseResult } from 'papaparse'
import { Contact } from '../../services/api/contacts'
import { contactsApi } from '../../services/api/contacts'
import { List } from '../../services/api/types'
import { listsApi } from '../../services/api/list'
import { useAuth } from '../../contexts/AuthContext'
import { getCustomFieldLabel } from '../../hooks/useCustomFieldLabel'

const { Text } = Typography
const { Option } = Select
const { Dragger } = Upload

// Batch size for processing
const BATCH_SIZE = 25
const PREVIEW_ROWS = 15
const PROGRESS_SAVE_INTERVAL = 10000 // 10 seconds

// Function to generate a unique storage key for each workspace+file combination
const getProgressStorageKey = (workspaceId: string, fileName: string): string => {
  return `csv_upload_progress_${workspaceId}_${fileName}`
}

export interface ContactsCsvUploadDrawerProps {
  workspaceId: string
  lists?: List[]
  selectedList?: string
  onSuccess?: () => void
  isVisible: boolean
  onClose: () => void
}

// Create context is moved to ContactsCsvUploadProvider.tsx

interface CsvData {
  headers: string[]
  rows: any[][]
  preview: any[][]
}

interface SavedProgress {
  fileName: string
  currentRow: number
  totalRows: number
  currentBatch: number
  totalBatches: number
  mappings: Record<string, string>
  selectedListIds: string[]
  timestamp: number
}

export function ContactsCsvUploadDrawer({
  workspaceId,
  lists = [],
  selectedList,
  onSuccess,
  isVisible,
  onClose
}: ContactsCsvUploadDrawerProps) {
  // Get workspace for custom field labels
  const { workspaces } = useAuth()
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Define contact fields for mapping with custom labels
  const contactFields = [
    { key: 'email', label: 'Email', required: true },
    { key: 'external_id', label: 'External ID' },
    { key: 'first_name', label: 'First Name' },
    { key: 'last_name', label: 'Last Name' },
    { key: 'phone', label: 'Phone' },
    { key: 'country', label: 'Country' },
    { key: 'timezone', label: 'Timezone' },
    { key: 'language', label: 'Language' },
    { key: 'address_line_1', label: 'Address Line 1' },
    { key: 'address_line_2', label: 'Address Line 2' },
    { key: 'postcode', label: 'Postcode' },
    { key: 'state', label: 'State' },
    { key: 'job_title', label: 'Job Title' },
    { key: 'lifetime_value', label: 'Lifetime Value' },
    { key: 'orders_count', label: 'Orders Count' },
    { key: 'last_order_at', label: 'Last Order At' },
    { key: 'custom_string_1', label: getCustomFieldLabel('custom_string_1', currentWorkspace) },
    { key: 'custom_string_2', label: getCustomFieldLabel('custom_string_2', currentWorkspace) },
    { key: 'custom_string_3', label: getCustomFieldLabel('custom_string_3', currentWorkspace) },
    { key: 'custom_string_4', label: getCustomFieldLabel('custom_string_4', currentWorkspace) },
    { key: 'custom_string_5', label: getCustomFieldLabel('custom_string_5', currentWorkspace) },
    { key: 'custom_number_1', label: getCustomFieldLabel('custom_number_1', currentWorkspace) },
    { key: 'custom_number_2', label: getCustomFieldLabel('custom_number_2', currentWorkspace) },
    { key: 'custom_number_3', label: getCustomFieldLabel('custom_number_3', currentWorkspace) },
    { key: 'custom_number_4', label: getCustomFieldLabel('custom_number_4', currentWorkspace) },
    { key: 'custom_number_5', label: getCustomFieldLabel('custom_number_5', currentWorkspace) },
    { key: 'custom_datetime_1', label: getCustomFieldLabel('custom_datetime_1', currentWorkspace) },
    { key: 'custom_datetime_2', label: getCustomFieldLabel('custom_datetime_2', currentWorkspace) },
    { key: 'custom_datetime_3', label: getCustomFieldLabel('custom_datetime_3', currentWorkspace) },
    { key: 'custom_datetime_4', label: getCustomFieldLabel('custom_datetime_4', currentWorkspace) },
    { key: 'custom_datetime_5', label: getCustomFieldLabel('custom_datetime_5', currentWorkspace) },
    { key: 'custom_json_1', label: getCustomFieldLabel('custom_json_1', currentWorkspace) },
    { key: 'custom_json_2', label: getCustomFieldLabel('custom_json_2', currentWorkspace) },
    { key: 'custom_json_3', label: getCustomFieldLabel('custom_json_3', currentWorkspace) },
    { key: 'custom_json_4', label: getCustomFieldLabel('custom_json_4', currentWorkspace) },
    { key: 'custom_json_5', label: getCustomFieldLabel('custom_json_5', currentWorkspace) }
  ]
  // Replace form with direct state management
  const [mappings, setMappings] = useState<Record<string, string>>({})
  const [selectedListIds, setSelectedListIds] = useState<string[]>(
    selectedList ? [selectedList] : []
  )

  const [csvData, setCsvData] = useState<CsvData | null>(null)
  // Add a ref to store parsed CSV data temporarily during parsing/restoration
  const parsedCsvDataRef = useRef<CsvData | null>(null)
  const [fileName, setFileName] = useState<string>('')
  const [uploading, setUploading] = useState<boolean>(false)
  const [uploadProgress, setUploadProgress] = useState<number>(0)
  const [currentBatch, setCurrentBatch] = useState<number>(0)
  const [totalBatches, setTotalBatches] = useState<number>(0)
  const [currentRow, setCurrentRow] = useState<number>(0)
  const [totalRows, setTotalRows] = useState<number>(0)
  const [paused, setPaused] = useState<boolean>(false)
  const [processingCancelled, setProcessingCancelled] = useState<boolean>(false)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const [savedProgressExists, setSavedProgressExists] = useState<boolean>(false)
  const [uploadComplete, setUploadComplete] = useState<boolean>(false)
  const [successCount, setSuccessCount] = useState<number>(0)
  const [failureCount, setFailureCount] = useState<number>(0)
  const [errorDetails, setErrorDetails] = useState<
    Array<{ line: number; email: string; error: string }>
  >([])
  const progressSaveInterval = useRef<number | null>(null)
  const uploadRef = useRef<{
    abort: () => void
    resume: () => void
    isPaused: boolean
  }>({
    abort: () => {},
    resume: () => {},
    isPaused: false
  })

  // Initialize with selectedList when provided
  useEffect(() => {
    if (selectedList) {
      setSelectedListIds([selectedList])
    }
  }, [selectedList])

  // Save progress to localStorage
  const saveProgress = () => {
    if (!fileName || !csvData) return

    try {
      const progressData: SavedProgress = {
        fileName,
        currentRow,
        totalRows,
        currentBatch,
        totalBatches,
        mappings,
        selectedListIds,
        timestamp: Date.now()
      }

      localStorage.setItem(
        getProgressStorageKey(workspaceId, fileName),
        JSON.stringify(progressData)
      )
    } catch (error) {
      console.error('Error saving progress:', error)
    }
  }

  // Check for saved progress
  const checkForSavedProgress = (filename: string) => {
    try {
      const savedData = localStorage.getItem(getProgressStorageKey(workspaceId, filename))
      if (savedData) {
        const savedProgress: SavedProgress = JSON.parse(savedData)

        // Check if the filename matches and it's recent (within 7 days)
        const isRecent = Date.now() - savedProgress.timestamp < 7 * 24 * 60 * 60 * 1000

        // Validate the saved progress data
        const isValid =
          savedProgress &&
          typeof savedProgress.fileName === 'string' &&
          typeof savedProgress.currentRow === 'number' &&
          typeof savedProgress.totalRows === 'number' &&
          typeof savedProgress.currentBatch === 'number' &&
          typeof savedProgress.totalBatches === 'number' &&
          typeof savedProgress.mappings === 'object' &&
          Array.isArray(savedProgress.selectedListIds)

        if (savedProgress.fileName === filename && isRecent && isValid) {
          // Additional validation: ensure the currentRow is within bounds
          if (savedProgress.currentRow >= 0 && savedProgress.currentRow < savedProgress.totalRows) {
            setSavedProgressExists(true)
            return savedProgress
          }
        }
      }
    } catch (error) {
      console.error('Error checking for saved progress:', error)
    }

    setSavedProgressExists(false)
    return null
  }

  // Handle restore progress with direct CSV data parameter
  const handleRestoreProgress = (savedProgress: SavedProgress, directCsvData?: CsvData) => {
    // console.log('handleRestoreProgress csvData state', csvData)
    // console.log('handleRestoreProgress parsedCsvDataRef', parsedCsvDataRef.current)
    // console.log('handleRestoreProgress directCsvData', directCsvData)
    // console.log('handleRestoreProgress savedProgress', savedProgress)

    // Use data from parameter, ref, or state in that order of preference
    const dataToUse = directCsvData || parsedCsvDataRef.current || csvData

    // Safety check
    if (!dataToUse) {
      console.error('Cannot restore progress: CSV data is not available from any source')
      message.error('Cannot restore progress: CSV data is not available')
      return
    }

    if (!savedProgress) {
      console.error('Cannot restore progress: No saved progress data')
      return
    }

    // Validate mappings against current CSV headers
    const validMappings: Record<string, string> = {}
    if (savedProgress.mappings) {
      Object.entries(savedProgress.mappings).forEach(([field, column]) => {
        // Ensure the column exists in the current CSV file
        if (column && typeof column === 'string' && dataToUse.headers.includes(column)) {
          validMappings[field] = column
        }
      })
    }

    // Ensure email mapping is still valid
    if (!validMappings.email) {
      message.warning(
        'Email mapping from previous session is invalid for this CSV. Please map fields manually.'
      )
      // Set minimal valid state for continuation
      setCurrentRow(0)
      setTotalRows(dataToUse.rows.length)
      setCurrentBatch(1)
      setTotalBatches(Math.ceil(dataToUse.rows.length / BATCH_SIZE))
      setUploadProgress(0)
      return
    }

    // Update state with validated mappings
    setMappings(validMappings)
    setSelectedListIds(savedProgress.selectedListIds || [])

    // Validate current row is within bounds
    const startRow =
      savedProgress.currentRow >= 0 && savedProgress.currentRow < dataToUse.rows.length
        ? savedProgress.currentRow
        : 0

    // Set state for resuming
    setCurrentRow(startRow)
    setTotalRows(dataToUse.rows.length)
    setCurrentBatch(savedProgress.currentBatch > 0 ? savedProgress.currentBatch : 1)
    setTotalBatches(Math.ceil(dataToUse.rows.length / BATCH_SIZE))
    setUploadProgress(Math.round((startRow / dataToUse.rows.length) * 100))

    message.success('Previous upload progress restored')
  }

  const handleCloseDrawer = () => {
    if (uploading) {
      Modal.confirm({
        title: 'Cancel Upload?',
        content:
          'Are you sure you want to cancel the upload process? Progress will be saved and you can resume later.',
        onOk: () => {
          if (uploading && !processingCancelled) {
            saveProgress()
          }
          cancelUpload()
          onClose()
        }
      })
    } else {
      // If upload is complete, make sure to clear saved progress
      if (uploadComplete && fileName) {
        clearSavedProgress(fileName)
      }
      onClose()
    }
  }

  const beforeUpload = (file: File) => {
    const isCsv = file.type === 'text/csv' || file.name.endsWith('.csv')
    if (!isCsv) {
      message.error('You can only upload CSV files!')
      return Upload.LIST_IGNORE
    }

    setFileName(file.name)

    // Parse CSV file first
    Papa.parse<string[]>(file, {
      header: false,
      complete: (results: ParseResult<string[]>) => {
        if (results.data && results.data.length > 0) {
          const headers = results.data[0]
          const rows = results.data.slice(1)
          const preview = rows.slice(0, PREVIEW_ROWS)

          const csvDataObj = {
            headers,
            rows,
            preview
          }

          // Store data in both state and ref
          setCsvData(csvDataObj)
          parsedCsvDataRef.current = csvDataObj

          // Set total rows
          setTotalRows(rows.length)

          // Auto-map fields if column names match contact field names
          const initialMappings: Record<string, string> = {}
          headers.forEach((header) => {
            const matchingField = contactFields.find(
              (field) =>
                field.key.toLowerCase() === header.toLowerCase() ||
                field.label.toLowerCase() === header.toLowerCase()
            )
            if (matchingField) {
              initialMappings[matchingField.key] = header
            }
          })

          // Apply initial mappings first
          setMappings(initialMappings)

          // Now that CSV data is available in the ref, check for saved progress
          const savedProgress = checkForSavedProgress(file.name)
          // console.log('savedProgress', savedProgress)

          // If there's saved progress, show modal to ask user
          if (savedProgress && !savedProgressExists) {
            Modal.confirm({
              title: 'Resume Previous Upload',
              content: `A previous upload for "${file.name}" was found (${new Date(savedProgress.timestamp).toLocaleString()}). Would you like to resume from where you left off?`,
              okText: 'Resume',
              cancelText: 'Start New',
              onOk: () => {
                if (savedProgress) {
                  handleRestoreProgress(savedProgress, csvDataObj)
                }
              },
              onCancel: () => {
                // Start fresh - clear saved progress
                // console.log('Starting fresh, clearing progress for:', file.name)

                // Use our improved clearSavedProgress function
                clearSavedProgress(file.name)

                // Set the initial mappings
                setMappings(initialMappings)
              }
            })
          }
        } else {
          message.error('The CSV file appears to be empty or invalid.')
        }
      },
      error: (error: Error) => {
        message.error(`Error parsing CSV: ${error.message}`)
      }
    })

    return false // Prevent default upload behavior
  }

  const uploadProps: UploadProps = {
    name: 'file',
    multiple: false,
    accept: '.csv',
    showUploadList: false,
    beforeUpload
  }

  // Clear saved progress
  const clearSavedProgress = (specificFileName?: string) => {
    try {
      const fileToUse = specificFileName || fileName

      if (!fileToUse) {
        console.warn('No filename provided to clearSavedProgress')
        return
      }

      // console.log('Clearing progress for:', fileToUse)
      localStorage.removeItem(getProgressStorageKey(workspaceId, fileToUse))
      setSavedProgressExists(false)

      // Reset progress-related state
      setCurrentRow(0)
      const dataToUse = csvData || parsedCsvDataRef.current
      if (dataToUse) {
        setTotalRows(dataToUse.rows.length)
        setTotalBatches(Math.ceil(dataToUse.rows.length / BATCH_SIZE))
      }
      setCurrentBatch(1)
      setUploadProgress(0)

      // console.log('Successfully cleared progress')
    } catch (error) {
      console.error('Error clearing saved progress:', error)
    }
  }

  useEffect(() => {
    // If CSV data is updated after initial processing, sync the ref
    if (csvData) {
      parsedCsvDataRef.current = csvData
    }
  }, [csvData])

  const startUpload = async () => {
    try {
      // Validate email mapping is set
      if (!mappings.email) {
        message.error('Email field mapping is required')
        return
      }

      // Use data from either state or ref
      const dataToUse = csvData || parsedCsvDataRef.current

      if (!dataToUse) {
        message.error('No CSV data available')
        return
      }

      // Ensure mappings use valid headers from the current CSV data
      const validMappings: Record<string, string> = {}
      Object.entries(mappings).forEach(([field, column]) => {
        if (column && dataToUse.headers.includes(column)) {
          validMappings[field] = column
        }
      })

      // Check if email mapping is still valid
      if (!validMappings.email) {
        message.error('Email mapping is missing or invalid')
        return
      }

      setUploading(true)
      setProcessingCancelled(false)
      setPaused(false)
      setUploadError(null)
      setSuccessCount(0)
      setFailureCount(0)
      setErrorDetails([])
      setUploadComplete(false)

      // Calculate total rows and batches if starting fresh
      let startRow = currentRow
      if (startRow === 0) {
        const totalBatches = Math.ceil(dataToUse.rows.length / BATCH_SIZE)
        setTotalRows(dataToUse.rows.length)
        setTotalBatches(totalBatches)
      }

      // Start auto-saving progress
      startProgressSaveInterval()

      // Process in batches
      let rowIndex = startRow
      let batch = currentBatch || 1
      let successCount = 0
      let failureCount = 0
      let errors: Array<{ line: number; email: string; error: string }> = []

      const processNextBatch = async () => {
        if (processingCancelled) {
          uploadRef.current.isPaused = false
          setUploading(false)
          return
        }

        setCurrentBatch(batch)
        const end = Math.min(rowIndex + BATCH_SIZE, dataToUse.rows.length)

        // Safeguard against out-of-bounds access
        if (rowIndex >= dataToUse.rows.length) {
          setUploadComplete(true)
          setUploading(false)
          stopProgressSaveInterval()
          clearSavedProgress(fileName) // Clear progress when reaching the end

          // Call onSuccess callback if provided
          if (onSuccess) {
            onSuccess()
          }

          message.success(
            `Imported ${successCount.toLocaleString()} contacts${
              failureCount > 0
                ? ` (${failureCount.toLocaleString()} failed, see details below)`
                : ''
            }`
          )
          return
        }

        const batchRows = dataToUse.rows.slice(rowIndex, end)

        const contacts: Partial<Contact>[] = batchRows.map((row) => {
          const contact: Partial<Contact> = {}

          // Map CSV columns to contact fields using validated mappings
          Object.entries(validMappings).forEach(([contactField, csvColumn]) => {
            if (csvColumn) {
              const columnIndex = dataToUse.headers.indexOf(csvColumn)
              if (columnIndex !== -1 && row[columnIndex] !== undefined) {
                let value = row[columnIndex]

                // Handle special field types
                if (contactField.startsWith('custom_json_') && value) {
                  try {
                    value = JSON.parse(value)
                  } catch (e) {
                    // Set to null if not valid JSON
                    value = null
                  }
                } else if (
                  contactField.startsWith('custom_number_') ||
                  contactField === 'lifetime_value' ||
                  contactField === 'orders_count'
                ) {
                  if (value && value.trim && value.trim() !== '') {
                    value = Number(value)
                    // Handle NaN values
                    if (isNaN(value)) value = null
                  } else {
                    value = null
                  }
                }

                ;(contact as any)[contactField] = value !== '' ? value : null
              }
            }
          })

          return contact
        })

        // Filter out contacts without email
        const validContacts = contacts.filter((contact) => contact.email)

        // Process each contact
        for (let i = 0; i < validContacts.length; i++) {
          const contact = validContacts[i]
          const csvLineNumber = rowIndex + i + 2 // +2 because headers are line 1, and rowIndex is 0-based
          if (processingCancelled) break

          // Check if paused
          while (uploadRef.current.isPaused && !processingCancelled) {
            await new Promise((resolve) => setTimeout(resolve, 100))
          }

          try {
            if (selectedListIds && selectedListIds.length > 0) {
              // Use the single subscribe endpoint that handles both contact upsert and list subscription
              await listsApi.subscribe({
                workspace_id: workspaceId,
                contact: contact as Contact,
                list_ids: selectedListIds
              })
            } else {
              // If no lists selected, just do a contact upsert
              await contactsApi.upsert({
                workspace_id: workspaceId,
                contact
              })
            }

            successCount++
            setSuccessCount(successCount)
          } catch (error) {
            console.error('Error upserting contact:', error)
            failureCount++
            setFailureCount(failureCount)

            // Store error details (limit to 100)
            if (errors.length < 100) {
              errors.push({
                line: csvLineNumber,
                email: contact.email || 'Unknown',
                error: error instanceof Error ? error.message : 'Unknown error'
              })
              setErrorDetails(errors)
            }
            // Continue with next contact
          }
        }

        rowIndex = end
        setCurrentRow(rowIndex)
        const progress = Math.min(Math.round((rowIndex / totalRows) * 100), 100)
        setUploadProgress(progress)

        // Save progress after each batch
        saveProgress()

        if (rowIndex < totalRows && !processingCancelled) {
          batch++
          // Use a try/catch here to prevent unhandled errors during batch processing
          try {
            setTimeout(processNextBatch, 0) // Continue with next batch
          } catch (batchError) {
            console.error('Error processing batch:', batchError)
            setUploadError(
              `Batch processing error: ${batchError instanceof Error ? batchError.message : 'Unknown error'}`
            )
            setUploading(false)
            stopProgressSaveInterval()
          }
        } else {
          // Upload is complete
          setUploadComplete(true)
          setUploading(false)
          stopProgressSaveInterval()
          clearSavedProgress(fileName) // Clear progress as it's now complete

          // Call onSuccess callback if provided
          if (onSuccess) {
            onSuccess()
          }

          message.success(
            `Imported ${successCount.toLocaleString()} contacts${
              failureCount > 0
                ? ` (${failureCount.toLocaleString()} failed, see details below)`
                : ''
            }`
          )
        }
      }

      // Setup abort and resume controls
      uploadRef.current = {
        abort: () => {
          setProcessingCancelled(true)
          saveProgress() // Save progress on abort
        },
        resume: () => {
          uploadRef.current.isPaused = false
          setPaused(false)
        },
        isPaused: false
      }

      try {
        await processNextBatch()
      } catch (batchError) {
        console.error('Error starting batch processing:', batchError)
        setUploadError(
          `Batch processing error: ${batchError instanceof Error ? batchError.message : 'Unknown error'}`
        )
        setUploading(false)
        stopProgressSaveInterval()
      }
    } catch (error) {
      console.error('CSV upload failed:', error)
      stopProgressSaveInterval()
      setUploading(false)
      setUploadError(`Upload failed: ${error instanceof Error ? error.message : 'Unknown error'}`)
      message.error('Upload failed. Please try again.')
    }
  }

  const pauseUpload = () => {
    uploadRef.current.isPaused = true
    setPaused(true)
    saveProgress() // Save progress on pause
  }

  const resumeUpload = () => {
    uploadRef.current.resume()
  }

  const cancelUpload = () => {
    saveProgress() // Save progress before cancelling
    uploadRef.current.abort()
    setUploading(false)
    stopProgressSaveInterval()
  }

  // Start progress auto-save interval
  const startProgressSaveInterval = () => {
    if (progressSaveInterval.current) {
      clearInterval(progressSaveInterval.current)
    }

    progressSaveInterval.current = window.setInterval(() => {
      if (uploading && !processingCancelled) {
        saveProgress()
      }
    }, PROGRESS_SAVE_INTERVAL)
  }

  // Stop progress auto-save interval
  const stopProgressSaveInterval = () => {
    if (progressSaveInterval.current) {
      clearInterval(progressSaveInterval.current)
      progressSaveInterval.current = null
    }
  }

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopProgressSaveInterval()

      // Clear saved progress on unmount if upload was completed
      if (uploadComplete && fileName) {
        clearSavedProgress(fileName)
      }
    }
  }, [uploadComplete, fileName])

  return (
    <Drawer
      title="Import Contacts from CSV"
      placement="right"
      onClose={handleCloseDrawer}
      open={isVisible}
      width={700}
      maskClosable={false}
      styles={{
        body: {
          padding: '24px'
        }
      }}
      extra={
        <Space>
          {!uploading && !uploadComplete && csvData && (
            <Button type="primary" onClick={startUpload}>
              {currentRow > 0 ? 'Resume Upload' : 'Start Upload'}
            </Button>
          )}
          {uploading && !paused && (
            <Button icon={<PauseCircleOutlined />} onClick={pauseUpload}>
              Pause
            </Button>
          )}
          {uploading && paused && (
            <Button icon={<PlayCircleOutlined />} onClick={resumeUpload}>
              Resume
            </Button>
          )}
          {uploading && (
            <Button danger icon={<CloseCircleOutlined />} onClick={cancelUpload}>
              Cancel
            </Button>
          )}
          {uploadComplete && (
            <Button
              type="primary"
              onClick={() => {
                // Ensure progress is cleared from localStorage
                clearSavedProgress(fileName)

                onClose()
              }}
            >
              Close
            </Button>
          )}
        </Space>
      }
      footer={null}
    >
      {!csvData && !uploading && !uploadComplete && (
        <Dragger {...uploadProps}>
          <p className="ant-upload-drag-icon">
            <UploadOutlined />
          </p>
          <p className="ant-upload-text">Click or drag a CSV file to this area to upload</p>
          <p className="ant-upload-hint">The CSV file should have headers in the first row.</p>
        </Dragger>
      )}

      {savedProgressExists && csvData && !uploading && !uploadComplete && (
        <Alert
          description={`You're continuing a previous upload session of "${fileName}". The upload will resume from row ${currentRow + 1} of ${totalRows}.`}
          type="info"
          showIcon
          style={{ marginBottom: 24 }}
          action={
            <Button
              size="small"
              danger
              onClick={() => {
                clearSavedProgress()
              }}
            >
              Start Fresh
            </Button>
          }
        />
      )}

      {uploading && (
        <div style={{ marginTop: 24, marginBottom: 24 }}>
          <Progress percent={uploadProgress} status={paused ? 'exception' : undefined} />
          <div style={{ marginTop: 12, textAlign: 'center' }}>
            <Text>
              Processing batch {currentBatch} of {totalBatches} ({currentRow.toLocaleString()} of{' '}
              {totalRows.toLocaleString()} rows)
              {paused && ' (Paused)'}
            </Text>
          </div>
        </div>
      )}

      {uploadError && (
        <Alert
          message="Upload Error"
          description={uploadError}
          type="error"
          style={{ marginTop: 16, marginBottom: 24 }}
        />
      )}

      {uploadComplete && (
        <div style={{ marginTop: 24, marginBottom: 24 }}>
          <Alert
            message="Upload Complete"
            description={`Processed ${totalRows.toLocaleString()} contacts: ${successCount.toLocaleString()} successful, ${failureCount.toLocaleString()} failed`}
            type={failureCount === 0 ? 'success' : 'warning'}
            showIcon
            style={{ marginBottom: 24 }}
          />

          {failureCount > 0 && (
            <>
              <Typography.Title level={4}>
                Errors ({Math.min(failureCount, 100)} of {failureCount.toLocaleString()} shown)
              </Typography.Title>
              <div style={{ overflowY: 'auto', maxHeight: '400px' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr>
                      <th
                        style={{
                          border: '1px solid #f0f0f0',
                          padding: '8px',
                          background: '#fafafa',
                          textAlign: 'left'
                        }}
                      >
                        Line
                      </th>
                      <th
                        style={{
                          border: '1px solid #f0f0f0',
                          padding: '8px',
                          background: '#fafafa',
                          textAlign: 'left'
                        }}
                      >
                        Email
                      </th>
                      <th
                        style={{
                          border: '1px solid #f0f0f0',
                          padding: '8px',
                          background: '#fafafa',
                          textAlign: 'left'
                        }}
                      >
                        Error
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {errorDetails.map((error, index) => (
                      <tr key={index}>
                        <td style={{ border: '1px solid #f0f0f0', padding: '8px' }}>
                          {error.line}
                        </td>
                        <td style={{ border: '1px solid #f0f0f0', padding: '8px' }}>
                          {error.email}
                        </td>
                        <td style={{ border: '1px solid #f0f0f0', padding: '8px' }}>
                          {error.error}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}

      {csvData && !uploading && !uploadComplete && (
        <div
          style={
            {
              '--form-item-margin-bottom': '12px',
              '--label-font-weight': '500'
            } as React.CSSProperties
          }
        >
          {fileName && (
            <div style={{ marginBottom: 24 }}>
              <Text strong>File: </Text>
              <Text>{fileName}</Text>
              <Text style={{ marginLeft: 8 }}>({csvData.rows.length.toLocaleString()} rows)</Text>
              {currentRow > 0 && (
                <Text type="success" style={{ marginLeft: 8 }}>
                  Will resume from row {currentRow.toLocaleString()}
                </Text>
              )}
            </div>
          )}

          {lists && lists.length > 0 && (
            <div
              style={{
                background: '#f8f8f8',
                padding: '16px',
                borderRadius: '4px',
                marginBottom: '24px'
              }}
            >
              <div>
                <label
                  style={{
                    display: 'block',
                    marginBottom: '8px',
                    fontWeight: 500
                  }}
                >
                  <UserAddOutlined /> Add to Lists
                </label>
                <Select
                  mode="multiple"
                  placeholder="Select lists to add contacts to"
                  style={{ width: '100%' }}
                  allowClear
                  value={selectedListIds}
                  onChange={(values) => setSelectedListIds(values)}
                  optionFilterProp="children"
                  tagRender={(props) => {
                    const { label, closable, onClose } = props
                    return (
                      <Tag
                        color="blue"
                        closable={closable}
                        onClose={onClose}
                        style={{ marginRight: 3 }}
                      >
                        {label}
                      </Tag>
                    )
                  }}
                >
                  {lists.map((list) => (
                    <Option key={list.id} value={list.id}>
                      {list.name}
                    </Option>
                  ))}
                </Select>
                <div style={{ fontSize: '12px', color: '#8c8c8c', marginTop: '4px' }}>
                  Contacts will be added to these lists on import (optional)
                </div>
              </div>
            </div>
          )}

          <div
            style={{
              background: '#f8f8f8',
              padding: '16px',
              borderRadius: '4px',
              marginBottom: '12px'
            }}
          >
            <p style={{ marginBottom: '16px' }}>
              <Text>
                Map your CSV columns to contact fields. The <Text strong>Email</Text> field is
                required.
              </Text>
            </p>

            {/* Validation indicator for email field */}
            {!mappings.email && (
              <Alert message="Email mapping required" type="warning" showIcon className="!mb-4" />
            )}

            {csvData.headers.map((header, headerIndex) => {
              // Check if this header is currently mapped to the email field
              const isEmailMapped = mappings.email === header

              // Find contact field this header is mapped to (if any)
              const mappedToField = Object.entries(mappings).find(
                ([_, value]) => value === header
              )?.[0]

              // Get up to 5 sample values for this column
              const sampleValues = csvData.preview
                .slice(0, 5)
                .map((row) => row[headerIndex])
                .filter((val) => val !== undefined && val !== null && val !== '')

              return (
                <div key={header} style={{ marginBottom: '16px' }}>
                  <div style={{ marginBottom: '8px' }}>
                    <label
                      style={{
                        display: 'block',
                        fontWeight: 500,
                        marginBottom: '8px'
                      }}
                    >
                      <Text strong>{header}</Text>
                      {isEmailMapped && (
                        <Tag bordered={false} color="red" style={{ marginLeft: 8 }}>
                          Email (Required)
                        </Tag>
                      )}
                    </label>
                    <div
                      style={{ display: 'flex', flexDirection: 'row', alignItems: 'flex-start' }}
                    >
                      <Select
                        placeholder="Select field to map to"
                        value={mappedToField}
                        style={{ width: '200px', marginRight: '12px' }}
                        status={header === mappings.email ? '' : ''}
                        onChange={(value) => {
                          const currentMappings = { ...mappings }

                          // Remove this column from any existing mappings
                          Object.keys(currentMappings).forEach((key) => {
                            if (currentMappings[key] === header) {
                              delete currentMappings[key]
                            }
                          })

                          // Add the new mapping if a field is selected
                          if (value) {
                            currentMappings[value] = header
                          }

                          setMappings(currentMappings)
                        }}
                        allowClear
                      >
                        <Select.OptGroup label="Required Fields">
                          <Option
                            key="email"
                            value="email"
                            disabled={mappings.email && mappings.email !== header}
                          >
                            Email
                          </Option>
                        </Select.OptGroup>

                        <Select.OptGroup label="Basic Information">
                          {contactFields
                            .filter(
                              (field) => field.key !== 'email' && !field.key.startsWith('custom_')
                            )
                            .map((field) => (
                              <Option
                                key={field.key}
                                value={field.key}
                                disabled={mappings[field.key] && mappings[field.key] !== header}
                              >
                                {field.label}
                              </Option>
                            ))}
                        </Select.OptGroup>

                        <Select.OptGroup label="Custom Fields">
                          {contactFields
                            .filter((field) => field.key.startsWith('custom_'))
                            .map((field) => (
                              <Option
                                key={field.key}
                                value={field.key}
                                disabled={mappings[field.key] && mappings[field.key] !== header}
                              >
                                {field.label}
                              </Option>
                            ))}
                        </Select.OptGroup>
                      </Select>

                      <div
                        style={{
                          minWidth: '300px',
                          background: 'white',
                          border: '1px solid #f0f0f0',
                          borderRadius: '4px',
                          padding: '4px 8px'
                        }}
                      >
                        {sampleValues.length > 0 ? (
                          <>
                            <Text
                              type="secondary"
                              style={{ fontSize: '12px', display: 'block', marginBottom: '4px' }}
                            >
                              Sample values:
                            </Text>
                            {sampleValues.map((value, i) => (
                              <div
                                key={i}
                                style={{
                                  fontSize: '13px',
                                  color: '#333',
                                  whiteSpace: 'nowrap',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  marginBottom: '2px'
                                }}
                              >
                                {String(value).substring(0, 40)}
                                {String(value).length > 40 ? '...' : ''}
                              </div>
                            ))}
                          </>
                        ) : (
                          <Text type="secondary" style={{ fontSize: '12px' }}>
                            No sample values available
                          </Text>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </Drawer>
  )
}
