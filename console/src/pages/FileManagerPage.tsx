import { App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FileManager } from '../components/file_manager/fileManager'
import type { FileManagerProps } from '../components/file_manager/interfaces'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { useAuth } from '../contexts/AuthContext'
import { workspaceService } from '../services/api/workspace'
import { Workspace, FileManagerSettings } from '../services/api/types'
import { useWorkspacePermissions } from '../contexts/AuthContext'
import { workspaceFileManagerRoute } from '../router'
import type { FileManagerSearch } from '../lib/searchParams'

export function FileManagerPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const search = useSearch({ from: workspaceFileManagerRoute.id })
  const navigate = useNavigate()
  const { workspaces, refreshWorkspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { message } = App.useApp()
  const currentWorkspace: Workspace | null = workspaces.find((w) => w.id === workspaceId) ?? null

  const handleError = (error: Error) => {
    console.error('File manager error:', error)
    message.error(t`An error occurred with the file manager`)
  }

  // Opening a folder closes the preview: its file belongs to the folder left.
  const handlePathChange = (newPath: string) => {
    navigate({
      to: workspaceFileManagerRoute.to,
      params: { workspaceId },
      search: { path: newPath || undefined }
    })
  }

  // The previewed file, replaced rather than pushed: J and K would fill the history.
  const handleFileChange = (key: string | undefined) => {
    navigate({
      to: workspaceFileManagerRoute.to,
      params: { workspaceId },
      search: (prev: FileManagerSearch) => ({ ...prev, file: key }),
      replace: true
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
    workspaceId,
    controlledPath: search.path || '',
    onPathChange: handlePathChange,
    controlledFile: search.file,
    onFileChange: handleFileChange,
    onError: handleError,
    // The page inserts nothing: files are only managed here.
    onSelect: () => {},
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

  return <FileManager {...fileManagerProps} />
}
