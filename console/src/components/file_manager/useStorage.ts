import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  DeleteObjectCommand,
  ListObjectsV2Command,
  PutObjectCommand,
  S3Client
} from '@aws-sdk/client-s3'
import type { FileManagerSettings, StorageObject } from './interfaces'
import {
  deleteFolder as deleteFolderObjects,
  listAllObjects,
  MAX_OBJECTS,
  toStorageItems,
  type DeleteFolderResult
} from './storageObjects'

// Spaces become dashes and the extension is lowercased, as uploads always have been.
export const sanitizeFilename = (filename: string): string => {
  const lastDotIndex = filename.lastIndexOf('.')
  if (lastDotIndex === -1) return filename.replace(/\s+/g, '-')
  const name = filename.substring(0, lastDotIndex)
  const ext = filename.substring(lastDotIndex + 1)
  return name.replace(/\s+/g, '-') + '.' + ext.toLowerCase()
}

export type UploadStatus = 'uploading' | 'done' | 'failed'

export interface UseStorageOptions {
  workspaceId: string
  settings?: FileManagerSettings
  /** Objects read at most; the file manager's cap unless a test gives a smaller one. */
  maxObjects?: number
}

/**
 * The bucket of the workspace: its objects, read through every page of ListObjectsV2 and cached
 * for a minute, and the writes, each of which lists the bucket again.
 */
export function useStorage({ workspaceId, settings, maxObjects = MAX_OBJECTS }: UseStorageOptions) {
  const queryClient = useQueryClient()
  const endpoint = settings?.endpoint ?? ''
  const bucket = settings?.bucket ?? ''
  const accessKey = settings?.access_key ?? ''
  const secretKey = settings?.secret_key ?? ''
  const region = settings?.region || 'us-east-1'
  const forcePathStyle = settings?.force_path_style ?? false
  const cdnEndpoint = settings?.cdn_endpoint

  // Rebuilt only when a connection setting changes, not on every new settings object.
  const client = useMemo(
    () =>
      endpoint
        ? new S3Client({
            endpoint,
            credentials: { accessKeyId: accessKey, secretAccessKey: secretKey },
            region,
            forcePathStyle
          })
        : undefined,
    [endpoint, accessKey, secretKey, region, forcePathStyle]
  )

  // Shared by the file manager page and the image picker.
  const query = useQuery({
    queryKey: ['storage-objects', workspaceId, bucket, endpoint],
    queryFn: () => listAllObjects(client as S3Client, bucket, { maxObjects }),
    enabled: client !== undefined && bucket !== '',
    staleTime: 60_000
  })

  // The raw objects are cached; file URLs depend on the CDN endpoint, so they are built here.
  const items: StorageObject[] = useMemo(
    () =>
      query.data
        ? toStorageItems(query.data.objects, {
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

  // New connection settings (keys, region, path style are not in the key): list again with the
  // client they build, once it exists. Skipped on mount, where the query lists on its own.
  const firstClientRef = useRef(client)
  useEffect(() => {
    if (client === firstClientRef.current) return
    firstClientRef.current = client
    void invalidate()
  }, [client, invalidate])

  const requireClient = useCallback((): S3Client => {
    if (!client) throw new Error('S3 client is not initialized.')
    return client
  }, [client])

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
      const s3 = requireClient()
      const written: string[] = []
      try {
        for (let index = 0; index < files.length; index++) {
          if (options.signal?.aborted) break
          const file = files[index]
          const key = folder + sanitizeFilename(file.name)
          options.onStatus?.(index, 'uploading')
          try {
            const body = new Uint8Array(await file.arrayBuffer())
            await s3.send(
              new PutObjectCommand({
                Bucket: bucket,
                Key: key,
                Body: body,
                ContentType: file.type
              }),
              { abortSignal: options.signal }
            )
            written.push(key)
            options.onStatus?.(index, 'done')
          } catch (error) {
            if ((error as Error).name === 'AbortError') break
            options.onStatus?.(index, 'failed', String(error))
          }
        }
      } finally {
        // Awaited: the caller can find the new files in the listing once this returns.
        await invalidate()
      }
      return written
    },
    [requireClient, bucket, invalidate]
  )

  /** Creates a folder marker; "exists" when something already lives under that prefix. */
  const createFolder = useCallback(
    async (folderKey: string): Promise<'created' | 'exists'> => {
      const s3 = requireClient()
      const existing = await s3.send(
        new ListObjectsV2Command({ Bucket: bucket, Prefix: folderKey, MaxKeys: 1 })
      )
      if (existing.Contents && existing.Contents.length > 0) return 'exists'
      await s3.send(new PutObjectCommand({ Bucket: bucket, Key: folderKey, Body: '' }))
      void invalidate()
      return 'created'
    },
    [requireClient, bucket, invalidate]
  )

  const deleteFile = useCallback(
    async (key: string) => {
      const s3 = requireClient()
      await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }))
      void invalidate()
    },
    [requireClient, bucket, invalidate]
  )

  /** Deletes files one at a time, as the folder delete does, counting the failures. */
  const deleteFiles = useCallback(
    async (keys: string[]): Promise<{ deleted: number; failed: number }> => {
      const s3 = requireClient()
      let deleted = 0
      let failed = 0
      try {
        for (const key of keys) {
          try {
            await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }))
            deleted++
          } catch (error) {
            console.error(`Failed to delete ${key}:`, error)
            failed++
          }
        }
      } finally {
        void invalidate()
      }
      return { deleted, failed }
    },
    [requireClient, bucket, invalidate]
  )

  /** Deletes everything under a folder, read from its own listing rather than the cached one. */
  const deleteFolder = useCallback(
    async (prefix: string): Promise<DeleteFolderResult> => {
      try {
        return await deleteFolderObjects(requireClient(), bucket, prefix)
      } finally {
        void invalidate()
      }
    },
    [requireClient, bucket, invalidate]
  )

  /** The number of files under a folder (markers left out), from a listing of its own. */
  const countFolderFiles = useCallback(
    async (prefix: string): Promise<number> => {
      const { objects } = await listAllObjects(requireClient(), bucket, {
        prefix,
        maxObjects: Number.POSITIVE_INFINITY
      })
      return objects.filter((object) => object.Key && !object.Key.endsWith('/')).length
    },
    [requireClient, bucket]
  )

  return {
    items,
    isLoading: query.isLoading,
    isSuccess: query.isSuccess,
    isFetching: query.isFetching,
    error: query.error,
    isCapped: query.data?.capped ?? false,
    cutShort: query.data?.cutShort ?? false,
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
