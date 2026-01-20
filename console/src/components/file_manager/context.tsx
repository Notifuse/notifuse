import React, { createContext, useContext, useState } from 'react'
import { Modal, App, Button } from 'antd'
import { FileManager } from './fileManager'
import type { FileManagerSettings, StorageObject } from './interfaces'

interface FileManagerContextValue {
  SelectFileButton: React.FC<SelectFileButtonProps>
}

interface SelectFileButtonProps {
  onSelect: (url: string) => void
  acceptFileType?: string
  acceptItem?: (item: StorageObject) => boolean
  buttonText?: string
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
  block?: boolean
  type?: 'primary' | 'default' | 'dashed' | 'link' | 'text'
  ghost?: boolean
  style?: React.CSSProperties
}

interface FileManagerProviderProps {
  children: React.ReactNode
  settings?: FileManagerSettings
  onUpdateSettings: (settings: FileManagerSettings) => Promise<void>
  settingsInfo?: React.ReactNode
  readOnly?: boolean
}

const FileManagerContext = createContext<FileManagerContextValue | undefined>(undefined)

export const FileManagerProvider: React.FC<FileManagerProviderProps> = ({
  children,
  settings,
  onUpdateSettings,
  settingsInfo,
  readOnly = false
}) => {
  const { message } = App.useApp()

  const [isModalVisible, setIsModalVisible] = useState(false)
  const [currentOptions, setCurrentOptions] = useState<{
    onSelect: (url: string) => void
    acceptFileType?: string
    acceptItem?: (item: StorageObject) => boolean
  } | null>(null)

  // Close file manager modal
  const closeModal = () => {
    setIsModalVisible(false)
    setCurrentOptions(null)
  }

  // Handle file selection from file manager
  const handleFileSelect = (items: StorageObject[]) => {
    if (currentOptions?.onSelect && items.length > 0) {
      const selectedFile = items[0]
      if (selectedFile.file_info?.url) {
        currentOptions.onSelect(selectedFile.file_info.url)
        message.success(`Selected: ${selectedFile.name}`)
        closeModal()
      }
    }
  }

  // Handle file manager errors
  const handleFileManagerError = (error: Error) => {
    console.error('File manager error:', error)
    message.error('File manager error: ' + error.toString())
  }

  // SelectFileButton component
  const SelectFileButton: React.FC<SelectFileButtonProps> = ({
    onSelect,
    acceptFileType = 'image/*',
    acceptItem = (item) => !item.is_folder && item.file_info?.content_type?.startsWith('image/'),
    buttonText = 'Browse Files',
    disabled = false,
    size = 'small',
    block = false,
    type = 'primary',
    ghost = false,
    style
  }) => {
    const handleOpenFileManager = () => {
      setCurrentOptions({
        onSelect,
        acceptFileType,
        acceptItem
      })
      setIsModalVisible(true)
    }

    return (
      <Button
        block={block}
        size={size}
        type={type}
        ghost={ghost}
        disabled={disabled}
        onClick={handleOpenFileManager}
        style={style}
      >
        {buttonText}
      </Button>
    )
  }

  const contextValue: FileManagerContextValue = {
    SelectFileButton
  }

  return (
    <FileManagerContext.Provider value={contextValue}>
      {children}

      {/* File Manager Modal */}
      <Modal
        title="File Manager"
        open={isModalVisible}
        onCancel={closeModal}
        footer={null}
        width={1000}
        style={{ top: 20 }}
        styles={{ body: { padding: 0 } }}
        zIndex={1300}
      >
        {currentOptions && (
          <FileManager
            key={`filemanager-${readOnly}-${!!currentOptions}`}
            settings={settings}
            onUpdateSettings={onUpdateSettings}
            onSelect={handleFileSelect}
            onError={handleFileManagerError}
            acceptFileType={currentOptions.acceptFileType || 'image/*'}
            acceptItem={
              currentOptions.acceptItem ||
              ((item) => !item.is_folder && item.file_info?.content_type?.startsWith('image/'))
            }
            height={600}
            withSelection={true}
            multiple={false}
            settingsInfo={settingsInfo}
            readOnly={readOnly}
          />
        )}
      </Modal>
    </FileManagerContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with provider
export const useFileManager = (): FileManagerContextValue => {
  const context = useContext(FileManagerContext)
  if (context === undefined) {
    throw new Error('useFileManager must be used within a FileManagerProvider')
  }
  return context
}

export default FileManagerProvider
