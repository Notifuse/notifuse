import { useState, useRef, useEffect } from 'react'
import {
  Button,
  Drawer,
  Upload,
  Select,
  Progress,
  Space,
  Typography,
  Alert,
  Modal,
  Tag,
  Table,
  Divider,
  List as AntList,
  App,
  Radio,
  Form
} from 'antd'
import {
  UploadOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  CloseCircleOutlined
} from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { UploadProps, UploadFile, ButtonProps } from 'antd'
import Papa from 'papaparse'
import { contactsApi } from '../../services/api/contacts'
import { contactListApi } from '../../services/api/contact_list'
import { List } from '../../services/api/types'
import { useQueryClient } from '@tanstack/react-query'

const { Text } = Typography
const { Dragger } = Upload

// Progress save interval
const PROGRESS_SAVE_INTERVAL = 5000 // 5 seconds
const DELAY_BETWEEN_REQUESTS = 100 // 100ms delay between API calls

// Function to generate a unique storage key for each workspace+file combination
const getProgressStorageKey = (
  workspaceId: string,
  fileName: string,
  operation: string
): string => {
  return `bulk_update_progress_${workspaceId}_${fileName}_${operation}`
}

interface BulkUpdateDrawerProps {
  workspaceId: string
  lists: List[]
  buttonProps?: ButtonProps
}

interface CSVData {
  headers: string[]
  rows: string[][]
  emailColumnIndex: number
  emails: string[]
  fileName: string
}

interface BulkOperationResult {
  email: string
  success: boolean
  error?: string
}

interface BulkOperationProgress {
  total: number
  processed: number
  successful: number
  failed: number
  results: BulkOperationResult[]
  isRunning: boolean
}

interface SavedProgress {
  fileName: string
  operation: string
  listId?: string
  currentIndex: number
  totalEmails: number
  results: BulkOperationResult[]
  timestamp: number
}

export function BulkUpdateDrawer({ workspaceId, lists, buttonProps }: BulkUpdateDrawerProps) {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const [csvData, setCsvData] = useState<CSVData | null>(null)
  const [fileList, setFileList] = useState<UploadFile[]>([])
  const [operation, setOperation] = useState<string>('')
  const [selectedListId, setSelectedListId] = useState<string>('')
  const [progress, setProgress] = useState<BulkOperationProgress>({
    total: 0,
    processed: 0,
    successful: 0,
    failed: 0,
    results: [],
    isRunning: false
  })
  const [paused, setPaused] = useState(false)
  const [processingCancelled, setProcessingCancelled] = useState(false)
  const [uploadComplete, setUploadComplete] = useState(false)

  const progressSaveInterval = useRef<number | null>(null)
  const queryClient = useQueryClient()
  const processingRef = useRef<{
    abort: () => void
    resume: () => void
    isPaused: boolean
    isCancelled: boolean
  }>({
    abort: () => {},
    resume: () => {},
    isPaused: false,
    isCancelled: false
  })

  const { message: messageApi } = App.useApp()

  // Parse CSV content and detect email column
  const parseCSV = (csvText: string, fileName: string): CSVData | null => {
    const parseResult = Papa.parse<string[]>(csvText, {
      header: false,
      skipEmptyLines: true
    })

    if (!parseResult.data || parseResult.data.length < 2) {
      messageApi.error(t`CSV must have at least a header row and one data row`)
      return null
    }

    // Parse headers
    const headers = parseResult.data[0].map((h) => h.trim().replace(/['"]/g, ''))

    // Find email column (case insensitive)
    const emailColumnIndex = headers.findIndex(
      (header) =>
        header.toLowerCase().includes('email') ||
        header.toLowerCase().includes('e-mail') ||
        header.toLowerCase().includes('mail')
    )

    if (emailColumnIndex === -1) {
      messageApi.error(
        t`No email column found. Please ensure your CSV has a column with "email" in the name`
      )
      return null
    }

    // Parse data rows
    const rows = parseResult.data
      .slice(1)
      .map((line) => line.map((cell) => cell.trim().replace(/['"]/g, '')))

    // Extract emails
    const emails = rows
      .map((row) => row[emailColumnIndex])
      .filter((email) => email && email.includes('@'))
      .map((email) => email.toLowerCase().trim())

    // Remove duplicates
    const uniqueEmails = [...new Set(emails)]

    return {
      headers,
      rows,
      emailColumnIndex,
      emails: uniqueEmails,
      fileName
    }
  }

  // Save progress to localStorage
  const saveProgress = () => {
    if (!csvData || !operation) return

    try {
      const progressData: SavedProgress = {
        fileName: csvData.fileName,
        operation,
        listId: selectedListId,
        currentIndex: progress.processed,
        totalEmails: progress.total,
        results: progress.results,
        timestamp: Date.now()
      }

      localStorage.setItem(
        getProgressStorageKey(workspaceId, csvData.fileName, operation),
        JSON.stringify(progressData)
      )
    } catch (error) {
      console.error('Error saving progress:', error)
    }
  }

  // Check for saved progress
  const checkForSavedProgress = (fileName: string, op: string, listId?: string) => {
    try {
      const savedData = localStorage.getItem(getProgressStorageKey(workspaceId, fileName, op))
      if (savedData) {
        const savedProgress: SavedProgress = JSON.parse(savedData)

        // Check if it's recent (within 7 days) and matches current operation
        const isRecent = Date.now() - savedProgress.timestamp < 7 * 24 * 60 * 60 * 1000
        const operationMatches = savedProgress.operation === op
        const listMatches = op === 'delete' || savedProgress.listId === listId

        if (savedProgress.fileName === fileName && isRecent && operationMatches && listMatches) {
          return savedProgress
        }
      }
    } catch (error) {
      console.error('Error checking for saved progress:', error)
    }

    return null
  }

  // Clear saved progress
  const clearSavedProgress = (fileName?: string, op?: string) => {
    try {
      const fileToUse = fileName || csvData?.fileName
      const operationToUse = op || operation

      if (!fileToUse || !operationToUse) return

      localStorage.removeItem(getProgressStorageKey(workspaceId, fileToUse, operationToUse))
    } catch (error) {
      console.error('Error clearing saved progress:', error)
    }
  }

  // Handle file upload
  const handleFileUpload: UploadProps['customRequest'] = ({ file, onSuccess }) => {
    const reader = new FileReader()
    reader.onload = (e) => {
      const csvText = e.target?.result as string
      const parsedData = parseCSV(csvText, (file as File).name)
      if (parsedData) {
        setCsvData(parsedData)
        messageApi.success(t`Found ${parsedData.emails.length} email addresses`)
      }
      onSuccess?.('ok')
    }
    reader.readAsText(file as File)
  }

  const handleFileRemove = () => {
    setCsvData(null)
    setFileList([])
    setProgress({
      total: 0,
      processed: 0,
      successful: 0,
      failed: 0,
      results: [],
      isRunning: false
    })
  }

  // Process emails with API calls
  const processEmails = async (emails: string[], op: string, listId?: string) => {
    setProgress({
      total: emails.length,
      processed: 0,
      successful: 0,
      failed: 0,
      results: [],
      isRunning: true
    })
    setProcessingCancelled(false)
    setPaused(false)
    processingRef.current.isCancelled = false

    // Start progress save interval
    startProgressSaveInterval()

    const results: BulkOperationResult[] = []

    for (let i = 0; i < emails.length; i++) {
      const email = emails[i]
      const result: BulkOperationResult = { email, success: false }

      // Check if processing is cancelled using ref (persists across re-renders)
      if (processingRef.current.isCancelled) {
        break
      }

      // Check if paused - do this BEFORE making the API call
      while (processingRef.current.isPaused && !processingRef.current.isCancelled) {
        await new Promise((resolve) => setTimeout(resolve, 100))
      }

      // Check again after potential pause to see if cancelled during pause
      if (processingRef.current.isCancelled) {
        break
      }

      try {
        if (op === 'delete') {
          await contactsApi.delete({
            workspace_id: workspaceId,
            email: email
          })
        } else if (op === 'unsubscribe' && listId) {
          await contactListApi.updateStatus({
            workspace_id: workspaceId,
            email: email,
            list_id: listId,
            status: 'unsubscribed'
          })
        }
        result.success = true
      } catch (error: unknown) {
        // Handle specific case where contact is not subscribed to the list
        const errorMessage = error instanceof Error ? error.message : 'Unknown error'
        if (
          op === 'unsubscribe' &&
          errorMessage &&
          errorMessage.includes('contact list not found')
        ) {
          // Contact is not subscribed to this list, which means they're already "unsubscribed"
          result.success = true
          result.error = 'Not subscribed (skipped)'
        } else {
          result.error = errorMessage
        }
      }

      results.push(result)

      setProgress((prev) => ({
        ...prev,
        processed: i + 1,
        successful: prev.successful + (result.success ? 1 : 0),
        failed: prev.failed + (result.success ? 0 : 1),
        results: [...prev.results, result]
      }))

      // Small delay to prevent overwhelming the server
      if (i < emails.length - 1) {
        await new Promise((resolve) => setTimeout(resolve, DELAY_BETWEEN_REQUESTS))
      }
    }

    setProgress((prev) => ({ ...prev, isRunning: false }))
    setUploadComplete(true)
    stopProgressSaveInterval()

    // Clear saved progress when complete
    if (csvData) {
      clearSavedProgress(csvData.fileName, op)
    }

    // Invalidate contacts query to refresh the list
    queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })

    // Show completion message
    const successful = results.filter((r) => r.success).length
    const failed = results.filter((r) => !r.success).length

    if (failed === 0) {
      messageApi.success(t`Successfully processed ${successful} contacts`)
    } else if (successful === 0) {
      messageApi.error(t`Failed to process all ${emails.length} contacts`)
    } else {
      messageApi.warning(t`Processed ${successful} contacts successfully, ${failed} failed`)
    }
  }

  const onFinish = async () => {
    if (!csvData || csvData.emails.length === 0) {
      messageApi.error(t`Please upload a CSV file with email addresses`)
      return
    }

    if (!operation) {
      messageApi.error(t`Please select an operation`)
      return
    }

    if (operation === 'unsubscribe' && !selectedListId) {
      messageApi.error(t`Please select a list for unsubscribe operation`)
      return
    }

    // Check for saved progress
    const savedProgress = checkForSavedProgress(csvData.fileName, operation, selectedListId)

    if (savedProgress) {
      Modal.confirm({
        title: t`Resume Previous Operation`,
        content: t`A previous ${operation} operation for "${csvData.fileName}" was found. Would you like to resume from where you left off?`,
        okText: t`Resume`,
        cancelText: t`Start New`,
        onOk: () => {
          // Restore progress and continue
          setProgress({
            total: savedProgress.totalEmails,
            processed: savedProgress.currentIndex,
            successful: savedProgress.results.filter((r) => r.success).length,
            failed: savedProgress.results.filter((r) => !r.success).length,
            results: savedProgress.results,
            isRunning: false
          })

          // Continue from where we left off
          const remainingEmails = csvData.emails.slice(savedProgress.currentIndex)
          if (remainingEmails.length > 0) {
            processEmails(remainingEmails, operation, selectedListId)
          }
        },
        onCancel: () => {
          clearSavedProgress(csvData.fileName, operation)
          processEmails(csvData.emails, operation, selectedListId)
        }
      })
    } else {
      await processEmails(csvData.emails, operation, selectedListId)
    }
  }

  const onClose = () => {
    // If processing is running and not cancelled, show confirmation
    if (progress.isRunning && !processingCancelled) {
      Modal.confirm({
        title: t`Cancel Operation?`,
        content:
          t`Are you sure you want to cancel the operation? Progress will be saved and you can resume later.`,
        onOk: () => {
          if (progress.isRunning && !processingCancelled) {
            saveProgress()
          }
          setProcessingCancelled(true)
          processingRef.current.isPaused = false
          stopProgressSaveInterval()
          setOpen(false)
          resetState()
        }
      })
      return
    }

    setOpen(false)
    resetState()
  }

  const resetState = () => {
    setCsvData(null)
    setFileList([])
    setOperation('')
    setSelectedListId('')
    setProgress({
      total: 0,
      processed: 0,
      successful: 0,
      failed: 0,
      results: [],
      isRunning: false
    })
    setPaused(false)
    setProcessingCancelled(false)
    setUploadComplete(false)
    processingRef.current.isCancelled = false
    processingRef.current.isPaused = false
  }

  const pauseProcessing = () => {
    processingRef.current.isPaused = true
    setPaused(true)
    saveProgress()
  }

  const resumeProcessing = () => {
    processingRef.current.isPaused = false
    setPaused(false)
  }

  const cancelProcessing = () => {
    setProcessingCancelled(true)
    processingRef.current.isCancelled = true
    processingRef.current.isPaused = false
    stopProgressSaveInterval()

    // Save progress before closing
    if (csvData) {
      saveProgress()
    }

    // Wait a bit to ensure the processing loop sees the cancellation
    setTimeout(() => {
      setOpen(false)
      resetState()
    }, 100)
  }

  // Start progress auto-save interval
  const startProgressSaveInterval = () => {
    if (progressSaveInterval.current) {
      clearInterval(progressSaveInterval.current)
    }

    progressSaveInterval.current = window.setInterval(() => {
      if (progress.isRunning && !processingCancelled) {
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

  // Setup processing controls
  useEffect(() => {
    processingRef.current.isPaused = paused
  }, [paused])

  // Setup cancellation control
  useEffect(() => {
    processingRef.current.isCancelled = processingCancelled
  }, [processingCancelled])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopProgressSaveInterval()
      if (uploadComplete && csvData) {
        clearSavedProgress(csvData.fileName, operation)
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- clearSavedProgress is stable
  }, [uploadComplete, csvData, operation])

  const isDangerousOperation = ['delete', 'unsubscribe'].includes(operation)
  const canSubmit = csvData && csvData.emails.length > 0 && operation && !progress.isRunning

  return (
    <>
      <Button {...buttonProps} onClick={() => setOpen(true)} />

      <Drawer
        title={t`Bulk Update Contacts`}
        width={800}
        onClose={onClose}
        open={open}
        closable={!progress.isRunning}
        maskClosable={!progress.isRunning}
        extra={
          <Space>
            {!progress.isRunning && !uploadComplete && canSubmit && (
              <Button type="primary" danger={isDangerousOperation} onClick={onFinish}>
                {operation === 'delete'
                  ? t`DELETE`
                  : operation === 'unsubscribe'
                    ? t`UNSUBSCRIBE`
                    : t`Start`}
              </Button>
            )}
            {progress.isRunning && !paused && (
              <Button icon={<PauseCircleOutlined />} onClick={pauseProcessing}>
                {t`Pause`}
              </Button>
            )}
            {progress.isRunning && paused && (
              <Button danger icon={<CloseCircleOutlined />} onClick={cancelProcessing}>
                {t`Cancel`}
              </Button>
            )}
            {progress.isRunning && paused && (
              <Button icon={<PlayCircleOutlined />} onClick={resumeProcessing}>
                {t`Resume`}
              </Button>
            )}
          </Space>
        }
      >
        {/* Operation Selection Form - Always show but disable when processing */}
        <Form layout="horizontal" labelCol={{ span: 3 }} wrapperCol={{ span: 18 }}>
          <Form.Item label={t`Action`}>
            <Radio.Group
              value={operation}
              onChange={(e) => setOperation(e.target.value)}
              disabled={progress.isRunning || uploadComplete}
            >
              <Radio.Button value="unsubscribe">{t`Unsubscribe from list`}</Radio.Button>
              <Radio.Button value="delete">{t`Delete contacts`}</Radio.Button>
            </Radio.Group>
          </Form.Item>

          {/* List Selection for Unsubscribe - Always show when unsubscribe is selected */}
          {operation === 'unsubscribe' && (
            <Form.Item label={t`List`}>
              <Select
                placeholder={t`Select list`}
                value={selectedListId}
                onChange={setSelectedListId}
                disabled={progress.isRunning || uploadComplete}
                options={lists.map((list) => ({
                  label: list.name,
                  value: list.id
                }))}
              />
            </Form.Item>
          )}
        </Form>

        {/* Warning Alert - Show at top when CSV is uploaded and operation is selected */}
        {csvData && operation && !progress.isRunning && !uploadComplete && (
          <Alert
            type="warning"
            message={operation === 'delete'
              ? t`This action will permanently delete ${csvData.emails.length} contact(s)`
              : t`This action will unsubscribe ${csvData.emails.length} contact(s)`}
            showIcon
            className="mb-4"
          />
        )}

        <Divider />

        {/* File Upload Section */}
        {!csvData && !progress.isRunning && !uploadComplete && (
          <div className="mb-6">
            <Dragger
              accept=".csv"
              customRequest={handleFileUpload}
              onRemove={handleFileRemove}
              fileList={fileList}
              onChange={({ fileList }) => setFileList(fileList)}
              maxCount={1}
            >
              <p className="ant-upload-drag-icon">
                <UploadOutlined />
              </p>
              <p className="ant-upload-text">{t`Click or drag a CSV file to this area to upload`}</p>
              <p className="ant-upload-hint">
                {t`The CSV file should have a column containing email addresses`}
              </p>
            </Dragger>
          </div>
        )}

        {/* CSV Preview Section */}
        {csvData && !progress.isRunning && !uploadComplete && (
          <>
            <div className="mt-8">
              <Text strong className="text-sm">
                {t`Preview (first 10 emails)`}:
              </Text>
              <Table
                size="small"
                dataSource={csvData.emails
                  .slice(0, 10)
                  .map((email, index) => ({ key: index, email }))}
                columns={[{ title: t`Email`, dataIndex: 'email', key: 'email' }]}
                pagination={false}
                className="mt-2"
              />
              {csvData.emails.length > 10 && (
                <Text type="secondary" className="text-xs">
                  {t`... and ${csvData.emails.length - 10} more emails`}
                </Text>
              )}
            </div>
          </>
        )}

        {/* Progress Section */}
        {progress.isRunning && (
          <>
            <Divider />
            <div className="mb-4">
              <div className="flex justify-between items-center mb-2">
                <Text strong>{t`Processing contacts...`}</Text>
                <Text type="secondary">
                  {progress.processed} / {progress.total}
                </Text>
              </div>
              <Progress
                percent={Math.round((progress.processed / progress.total) * 100)}
                status={progress.isRunning ? 'active' : 'success'}
              />
              <div className="flex justify-between mt-2">
                <Text type="success">{t`Successful`}: {progress.successful}</Text>
                <Text type="danger">{t`Failed`}: {progress.failed}</Text>
              </div>
              {paused && (
                <Alert
                  type="warning"
                  message={t`Processing paused`}
                  description={t`Click Resume to continue processing`}
                  className="mt-2"
                />
              )}
            </div>
          </>
        )}

        {/* Results Section */}
        {progress.results.length > 0 && (
          <>
            <Divider />
            <div className="mb-4">
              <Text strong>{t`Results`}:</Text>
              <div className="mt-2 max-h-60 overflow-y-auto">
                <AntList
                  size="small"
                  dataSource={progress.results}
                  renderItem={(item) => (
                    <AntList.Item>
                      <div className="flex justify-between items-center w-full">
                        <Text className="text-sm">{item.email}</Text>
                        <div>
                          {item.success ? (
                            item.error ? (
                              <Tag color="warning" title={item.error}>
                                {t`Skipped`}
                              </Tag>
                            ) : (
                              <Tag color="success">{t`Success`}</Tag>
                            )
                          ) : (
                            <Tag color="error" title={item.error}>
                              {t`Failed`}
                            </Tag>
                          )}
                        </div>
                      </div>
                    </AntList.Item>
                  )}
                />
              </div>
            </div>
          </>
        )}

        {/* Completion Section */}
        {uploadComplete && (
          <Alert
            type={progress.failed === 0 ? 'success' : 'warning'}
            message={t`Operation Complete`}
            description={t`Processed ${progress.total} contacts: ${progress.successful} successful, ${progress.failed} failed`}
            showIcon
            className="mb-4"
          />
        )}
      </Drawer>
    </>
  )
}
