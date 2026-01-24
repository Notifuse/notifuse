import { useState, useEffect } from 'react'
import { App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FileManager } from '../components/file_manager/fileManager'
import { FileManagerProps } from '../components/file_manager/interfaces'
import { StorageObject } from '../components/file_manager/interfaces'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { useAuth } from '../contexts/AuthContext'
import { workspaceService } from '../services/api/workspace'
import { Workspace, FileManagerSettings } from '../services/api/types'
import { useWorkspacePermissions } from '../contexts/AuthContext'
import { workspaceFileManagerRoute } from '../router'

export function FileManagerPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const search = useSearch({ from: workspaceFileManagerRoute.id })
  const navigate = useNavigate()
  const { workspaces, refreshWorkspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace | null>(null)
  const { message } = App.useApp()

  // Initialize settings from the current workspace
  useEffect(() => {
    if (workspaceId && workspaces.length > 0) {
      const workspace = workspaces.find((w) => w.id === workspaceId)
      if (workspace) {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        setCurrentWorkspace(workspace)
      }
    }
  }, [workspaceId, workspaces])

  const handleError = (error: Error) => {
    console.error('File manager error:', error)
    message.error(t`An error occurred with the file manager`)
  }

  const handleSelect = (items: StorageObject[]) => {
    console.log('Selected items:', items)
    // Handle selected items as needed
  }

  const handlePathChange = (newPath: string) => {
    navigate({
      to: workspaceFileManagerRoute.to,
      params: { workspaceId },
      search: { path: newPath || undefined }
    })
  }

  const handleUpdateSettings = async (newSettings: FileManagerSettings) => {
    try {
      if (!currentWorkspace || !workspaceId) {
        message.error(t`Workspace not found`)
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
        }
      })

      // Refresh workspaces to get the updated data
      await refreshWorkspaces()

      message.success(t`File manager settings updated successfully`)
    } catch (error) {
      console.error('Error updating settings:', error)
      message.error(t`Failed to update file manager settings`)
    }
  }

  const fileManagerProps: FileManagerProps = {
    currentPath: '',
    controlledPath: search.path || '',
    onPathChange: handlePathChange,
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
        <div className="text-2xl font-medium">{t`File Manager`}</div>
      </div>

      <div className="border border-gray-200 rounded-md p-4">
        <FileManager {...fileManagerProps} />
      </div>
    </div>
  )
}
