import { useState, useEffect } from 'react'
import { App } from 'antd'
import { FileManager } from '../components/file_manager/fileManager'
import { FileManagerProps } from '../components/file_manager/interfaces'
import { StorageObject } from '../components/file_manager/interfaces'
import { useParams } from '@tanstack/react-router'
import { useAuth } from '../contexts/AuthContext'
import { workspaceService } from '../services/api/workspace'
import { Workspace, FileManagerSettings } from '../services/api/types'
import { useWorkspacePermissions } from '../contexts/AuthContext'

export function FileManagerPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { workspaces, refreshWorkspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace | null>(null)
  const { message } = App.useApp()

  // Initialize settings from the current workspace
  useEffect(() => {
    if (workspaceId && workspaces.length > 0) {
      const workspace = workspaces.find((w) => w.id === workspaceId)
      if (workspace) {
        setCurrentWorkspace(workspace)
      }
    }
  }, [workspaceId, workspaces])

  const handleError = (error: any) => {
    console.error('File manager error:', error)
    message.error('An error occurred with the file manager')
  }

  const handleSelect = (items: StorageObject[]) => {
    console.log('Selected items:', items)
    // Handle selected items as needed
  }

  const handleUpdateSettings = async (newSettings: FileManagerSettings) => {
    try {
      if (!currentWorkspace || !workspaceId) {
        message.error('Workspace not found')
        return
      }

      // Update the workspace settings
      await workspaceService.update({
        ...currentWorkspace,
        settings: {
          ...currentWorkspace.settings,
          file_manager: {
            provider: newSettings.provider,
            endpoint: newSettings.endpoint,
            access_key: newSettings.access_key,
            bucket: newSettings.bucket,
            region: newSettings.region,
            secret_key: newSettings.secret_key,
            cdn_endpoint: newSettings.cdn_endpoint,
            force_path_style: newSettings.force_path_style
          }
        } as any // Use type assertion to bypass the type checking
      })

      // Refresh workspaces to get the updated data
      await refreshWorkspaces()

      message.success('File manager settings updated successfully')
    } catch (error) {
      console.error('Error updating settings:', error)
      message.error('Failed to update file manager settings')
    }
  }

  const fileManagerProps: FileManagerProps = {
    currentPath: '',
    onError: handleError,
    onSelect: handleSelect,
    height: 600,
    acceptFileType: '*',
    acceptItem: () => true,
    withSelection: true,
    multiple: true,
    settings: {
      provider: currentWorkspace?.settings?.file_manager?.provider,
      endpoint: currentWorkspace?.settings?.file_manager?.endpoint || '',
      access_key: currentWorkspace?.settings?.file_manager?.access_key || '',
      bucket: currentWorkspace?.settings?.file_manager?.bucket || '',
      region: currentWorkspace?.settings?.file_manager?.region || '',
      secret_key: currentWorkspace?.settings?.file_manager?.secret_key || '',
      cdn_endpoint: currentWorkspace?.settings?.file_manager?.cdn_endpoint || '',
      force_path_style: currentWorkspace?.settings?.file_manager?.force_path_style
    },
    onUpdateSettings: handleUpdateSettings,
    readOnly: !permissions?.templates?.write
  }

  // console.log('fileManagerProps', fileManagerProps)
  // console.log('currentWorkspace', currentWorkspace)

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">File Manager</div>
      </div>

      <div className="border border-gray-200 rounded-md p-4">
        <FileManager {...fileManagerProps} />
      </div>
    </div>
  )
}
