import { api } from './client'
import type { FileManagerSettings } from '../../components/file_manager/interfaces'

// Types mirror internal/domain/file_storage.go. The bucket's secret stays on the server: it
// lists, creates folders and deletes, and signs a short-lived URL for each upload.

export interface StoredObject {
  key: string
  size: number
  /** RFC3339. */
  last_modified: string
}

export interface StoredObjectList {
  objects: StoredObject[]
  /** The listing stopped at the server's maximum while the bucket holds more. */
  capped: boolean
  /** The storage stopped answering before the end of the listing. */
  cut_short: boolean
}

export interface PresignUploadRequest {
  workspace_id: string
  key: string
  content_type: string
  size: number
}

export interface PresignedUpload {
  url: string
  /** Headers the PUT must carry exactly as given: the signature covers them. */
  headers: Record<string, string>
  /** RFC3339. */
  expires_at: string
}

export interface CreateFolderResult {
  status: 'created' | 'exists'
}

export interface DeleteFilesResult {
  deleted: number
  failed: number
}

export interface DeleteFolderResult {
  deleted: number
  failed: number
  deleted_files: number
  failed_files: number
}

export const filesApi = {
  list(workspaceId: string, prefix = ''): Promise<StoredObjectList> {
    const params = new URLSearchParams({ workspace_id: workspaceId })
    if (prefix) params.append('prefix', prefix)
    return api.get<StoredObjectList>(`/api/files.list?${params.toString()}`)
  },

  presignUpload(request: PresignUploadRequest): Promise<PresignedUpload> {
    return api.post<PresignedUpload>('/api/files.presignUpload', request)
  },

  createFolder(workspaceId: string, key: string): Promise<CreateFolderResult> {
    return api.post<CreateFolderResult>('/api/files.createFolder', { workspace_id: workspaceId, key })
  },

  delete(workspaceId: string, keys: string[]): Promise<DeleteFilesResult> {
    return api.post<DeleteFilesResult>('/api/files.delete', { workspace_id: workspaceId, keys })
  },

  deleteFolder(workspaceId: string, prefix: string): Promise<DeleteFolderResult> {
    return api.post<DeleteFolderResult>('/api/files.deleteFolder', { workspace_id: workspaceId, prefix })
  },

  /** Owner only. An empty secret_key tests with the stored secret, for the same access key and host. */
  testSettings(workspaceId: string, settings: FileManagerSettings): Promise<{ success: boolean }> {
    return api.post<{ success: boolean }>('/api/workspaces.testFileManager', {
      workspace_id: workspaceId,
      settings
    })
  }
}
