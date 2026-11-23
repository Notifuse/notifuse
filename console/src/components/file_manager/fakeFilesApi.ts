import { vi } from 'vitest'
import type {
  filesApi,
  PresignedUpload,
  PresignUploadRequest,
  StoredObject,
  StoredObjectList
} from '../../services/api/files'

// Test helper: an in-memory bucket behind filesApi, and the fetch the browser PUTs uploads with.
// A test file mocks the API module with it:
//
//   vi.mock('../../services/api/files', async () => ({
//     filesApi: (await import('./fakeFilesApi')).fakeFiles.api
//   }))
//
// then calls fakeFiles.reset(objects) before each test and vi.stubGlobal('fetch', fakeFiles.fetch).

type FilesApi = typeof filesApi

export interface FakeObject {
  key: string
  size?: number
  lastModified?: Date
}

export function createFakeFiles() {
  let objects: StoredObject[] = []
  let flags = { capped: false, cut_short: false }
  // Keys the storage refuses to delete.
  const undeletable = new Set<string>()
  // Presigned URL -> the key a PUT to it writes.
  const signed = new Map<string, string>()

  const stored = ({ key, size = 0, lastModified = new Date(0) }: FakeObject): StoredObject => ({
    key,
    size,
    last_modified: lastModified.toISOString()
  })

  const remove = (key: string): boolean => {
    if (undeletable.has(key)) return false
    objects = objects.filter((object) => object.key !== key)
    return true
  }

  // The server's behaviour, as each test starts with it; a test may override one with mockImplementation.
  const impl: FilesApi = {
    list: async (_workspaceId: string, prefix = ''): Promise<StoredObjectList> => ({
      objects: objects.filter((object) => object.key.startsWith(prefix)),
      ...flags
    }),
    presignUpload: async (request: PresignUploadRequest): Promise<PresignedUpload> => {
      const url = `https://upload.example.com/media/${request.key}?X-Amz-Signature=sig`
      signed.set(url, request.key)
      return {
        url,
        headers: { 'Content-Type': request.content_type },
        expires_at: '2026-01-01T00:15:00Z'
      }
    },
    createFolder: async (_workspaceId: string, key: string) => {
      if (objects.some((object) => object.key.startsWith(key))) return { status: 'exists' }
      objects.push(stored({ key }))
      return { status: 'created' }
    },
    delete: async (_workspaceId: string, keys: string[]) => {
      let deleted = 0
      let failed = 0
      for (const key of keys) {
        if (remove(key)) deleted++
        else failed++
      }
      return { deleted, failed }
    },
    deleteFolder: async (_workspaceId: string, prefix: string) => {
      const result = { deleted: 0, failed: 0, deleted_files: 0, failed_files: 0 }
      for (const { key } of objects.filter((object) => object.key.startsWith(prefix))) {
        const isFile = !key.endsWith('/')
        if (remove(key)) {
          result.deleted++
          if (isFile) result.deleted_files++
        } else {
          result.failed++
          if (isFile) result.failed_files++
        }
      }
      return result
    },
    testSettings: async () => ({ success: true })
  }

  const api = {
    list: vi.fn<FilesApi['list']>(),
    presignUpload: vi.fn<FilesApi['presignUpload']>(),
    createFolder: vi.fn<FilesApi['createFolder']>(),
    delete: vi.fn<FilesApi['delete']>(),
    deleteFolder: vi.fn<FilesApi['deleteFolder']>(),
    testSettings: vi.fn<FilesApi['testSettings']>()
  }

  // A PUT to a presigned URL stores the file under its key.
  const putImpl = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input)
    const key = signed.get(url)
    if (init?.method !== 'PUT' || key === undefined) {
      throw new Error(`unexpected fetch ${init?.method ?? 'GET'} ${url}`)
    }
    const size = init.body instanceof Blob ? init.body.size : 0
    objects.push(stored({ key, size, lastModified: new Date() }))
    return new Response(null, { status: 200 })
  }
  const fetch = vi.fn<typeof globalThis.fetch>()

  const reset = (initial: FakeObject[] = []) => {
    objects = initial.map(stored)
    flags = { capped: false, cut_short: false }
    undeletable.clear()
    signed.clear()
    api.list.mockReset().mockImplementation(impl.list)
    api.presignUpload.mockReset().mockImplementation(impl.presignUpload)
    api.createFolder.mockReset().mockImplementation(impl.createFolder)
    api.delete.mockReset().mockImplementation(impl.delete)
    api.deleteFolder.mockReset().mockImplementation(impl.deleteFolder)
    api.testSettings.mockReset().mockImplementation(impl.testSettings)
    fetch.mockReset().mockImplementation(putImpl)
  }
  reset()

  return {
    api,
    fetch,
    impl,
    reset,
    /** The keys the bucket holds now. */
    keys: () => objects.map((object) => object.key),
    /** The storage refuses to delete these keys. */
    refuseDelete: (...keys: string[]) => keys.forEach((key) => undeletable.add(key)),
    /** The flags of every listing from now on. */
    setListing: (next: Partial<typeof flags>) => {
      flags = { ...flags, ...next }
    }
  }
}

export type FakeFiles = ReturnType<typeof createFakeFiles>

/** One bucket per test file: the module registry gives the mock factory and the test the same one. */
export const fakeFiles = createFakeFiles()
