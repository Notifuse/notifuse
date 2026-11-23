import { useCallback, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import { filesApi } from '../../services/api/files'
import type { FileManagerSettings, StorageObject } from './interfaces'
import { fromStoredObjects, toStorageItems, type DeleteFolderResult } from './storageObjects'

// Spaces become dashes and the extension is lowercased, as uploads always have been.
export const sanitizeFilename = (filename: string): string => {
  const lastDotIndex = filename.lastIndexOf('.')
  if (lastDotIndex === -1) return filename.replace(/\s+/g, '-')
  const name = filename.substring(0, lastDotIndex)
  const ext = filename.substring(lastDotIndex + 1)
  return name.replace(/\s+/g, '-') + '.' + ext.toLowerCase()
}

export type UploadStatus = 'uploading' | 'done' | 'failed'

// The most keys one files.delete call takes (domain.MaxDeletedStorageKeys).
export const DELETE_BATCH_SIZE = 1000

export interface UseStorageOptions {
  workspaceId: string
  settings?: FileManagerSettings
}

export interface FolderFileCount {
  count: number
  /** More files than the server lists: count is a floor. */
  capped: boolean
}

// A fetch that never reached the bucket (network, CORS) throws a TypeError; the message
// says where to look, since the bucket must accept PUT from the console's origin.
const uploadError = (error: unknown): string =>
  error instanceof TypeError
    ? i18n._(
        msg`The storage did not answer. Check that the bucket accepts PUT requests from this origin (CORS).`
      )
    : String(error)

/**
 * The bucket of the workspace, through the API: the server lists it (cached for a minute),
 * creates folders and deletes, and signs a short-lived URL the browser uploads each file to.
 */
export function useStorage({ workspaceId, settings }: UseStorageOptions) {
  const queryClient = useQueryClient()
  const endpoint = settings?.endpoint ?? ''
  const bucket = settings?.bucket ?? ''
  const accessKey = settings?.access_key ?? ''
  const cdnEndpoint = settings?.cdn_endpoint
  const configured = endpoint !== '' && bucket !== ''

  // Shared by the file manager page and the image picker. The endpoint and the bucket are in
  // the key, so a settings change lists the new bucket.
  const query = useQuery({
    queryKey: ['storage-objects', workspaceId, bucket, endpoint],
    queryFn: () => filesApi.list(workspaceId),
    enabled: configured,
    staleTime: 60_000
  })

  // The raw objects are cached; file URLs depend on the CDN endpoint, so they are built here.
  const items: StorageObject[] = useMemo(
    () =>
      query.data
        ? toStorageItems(fromStoredObjects(query.data.objects), {
            endpoint,
            bucket,
            access_key: accessKey,
            cdn_endpoint: cdnEndpoint
          })
        : [],
    [query.data, endpoint, bucket, accessKey, cdnEndpoint]
  )

  // Every listing of the workspace, so a settings change also drops the old bucket's.
  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ['storage-objects', workspaceId] }),
    [queryClient, workspaceId]
  )

  /**
   * Uploads the files into a folder one after the other, reporting each one's status, then lists
   * the bucket again. Returns the keys written; an abort stops the file being sent and the next.
   */
  const upload = useCallback(
    async (
      files: File[],
      folder: string,
      options: {
        signal?: AbortSignal
        onStatus?: (index: number, status: UploadStatus, error?: string) => void
      } = {}
    ): Promise<string[]> => {
      const written: string[] = []
      try {
        for (let index = 0; index < files.length; index++) {
          if (options.signal?.aborted) break
          const file = files[index]
          const key = folder + sanitizeFilename(file.name)
          options.onStatus?.(index, 'uploading')
          try {
            const signed = await filesApi.presignUpload({
              workspace_id: workspaceId,
              key,
              content_type: file.type,
              size: file.size
            })
            // The API call takes no signal: an abort during it is honoured here.
            if (options.signal?.aborted) break
            const response = await fetch(signed.url, {
              method: 'PUT',
              headers: signed.headers,
              body: file,
              signal: options.signal
            })
            if (!response.ok) {
              throw new Error(`${response.status} ${response.statusText}`.trim())
            }
            written.push(key)
            options.onStatus?.(index, 'done')
          } catch (error) {
            if ((error as Error).name === 'AbortError') break
            options.onStatus?.(index, 'failed', uploadError(error))
          }
        }
      } finally {
        // Awaited: the caller can find the new files in the listing once this returns.
        await invalidate()
      }
      return written
    },
    [workspaceId, invalidate]
  )

  /** Creates a folder marker; "exists" when something already lives under that prefix. */
  const createFolder = useCallback(
    async (folderKey: string): Promise<'created' | 'exists'> => {
      const { status } = await filesApi.createFolder(workspaceId, folderKey)
      if (status === 'created') void invalidate()
      return status
    },
    [workspaceId, invalidate]
  )

  const deleteFile = useCallback(
    async (key: string) => {
      const result = await filesApi.delete(workspaceId, [key])
      void invalidate()
      if (result.failed > 0) throw new Error(i18n._(msg`The storage refused to delete ${key}`))
    },
    [workspaceId, invalidate]
  )

  /** Deletes files in batches the server accepts, counting the failures. */
  const deleteFiles = useCallback(
    async (keys: string[]): Promise<{ deleted: number; failed: number }> => {
      const total = { deleted: 0, failed: 0 }
      try {
        for (let start = 0; start < keys.length; start += DELETE_BATCH_SIZE) {
          const batch = await filesApi.delete(workspaceId, keys.slice(start, start + DELETE_BATCH_SIZE))
          total.deleted += batch.deleted
          total.failed += batch.failed
        }
        return total
      } finally {
        void invalidate()
      }
    },
    [workspaceId, invalidate]
  )

  /** Deletes everything under a folder; the server lists it, files first, then the folders. */
  const deleteFolder = useCallback(
    async (prefix: string): Promise<DeleteFolderResult> => {
      try {
        const result = await filesApi.deleteFolder(workspaceId, prefix)
        return {
          deleted: result.deleted,
          failed: result.failed,
          deletedFiles: result.deleted_files,
          failedFiles: result.failed_files
        }
      } finally {
        void invalidate()
      }
    },
    [workspaceId, invalidate]
  )

  /** The number of files under a folder (markers left out), from a listing of its own. */
  const countFolderFiles = useCallback(
    async (prefix: string): Promise<FolderFileCount> => {
      const listing = await filesApi.list(workspaceId, prefix)
      return {
        count: listing.objects.filter((object) => !object.key.endsWith('/')).length,
        capped: listing.capped
      }
    },
    [workspaceId]
  )

  return {
    items,
    isLoading: query.isLoading,
    isSuccess: query.isSuccess,
    isFetching: query.isFetching,
    error: query.error,
    isCapped: query.data?.capped ?? false,
    cutShort: query.data?.cut_short ?? false,
    refetch: query.refetch,
    invalidate,
    upload,
    createFolder,
    deleteFile,
    deleteFiles,
    deleteFolder,
    countFolderFiles
  }
}

export type Storage = ReturnType<typeof useStorage>
