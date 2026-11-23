import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useStorage, type UploadStatus } from './useStorage'
import { fakeFiles } from './fakeFilesApi'
import type { PresignedUpload } from '../../services/api/files'

// The bucket lives behind the API: an in-memory one here, and the presigned PUTs go to fetch.
vi.mock('../../services/api/files', async () => ({
  filesApi: (await import('./fakeFilesApi')).fakeFiles.api
}))

const settings = { endpoint: 'https://s3.example.com', access_key: 'key', bucket: 'media' }

const wrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

const at = (day: number) => new Date(Date.UTC(2026, 0, day))

const renderStorage = () =>
  renderHook(() => useStorage({ workspaceId: 'ws', settings }), { wrapper: wrapper() })

// Runs an upload, collecting each "index:status" and each failure's message.
const uploadAll = async (
  storage: ReturnType<typeof useStorage>,
  files: File[],
  signal?: AbortSignal
) => {
  const statuses: string[] = []
  const errors: (string | undefined)[] = []
  let written: string[] = []
  await act(async () => {
    written = await storage.upload(files, 'photos/', {
      signal,
      onStatus: (index: number, status: UploadStatus, error?: string) => {
        statuses.push(`${index}:${status}`)
        if (status === 'failed') errors.push(error)
      }
    })
  })
  return { statuses, errors, written }
}

const png = (name: string) => new File(['x'], name, { type: 'image/png' })

describe('useStorage', () => {
  beforeEach(() => {
    fakeFiles.reset()
    vi.stubGlobal('fetch', fakeFiles.fetch)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the bucket from the API and builds each file’s URL from the settings', async () => {
    fakeFiles.reset([
      { key: 'a.png', size: 10, lastModified: at(1) },
      { key: 'docs/b.png', size: 20, lastModified: at(2) }
    ])
    const { result } = renderStorage()

    await waitFor(() => expect(result.current.items.length).toBeGreaterThan(0))
    expect(fakeFiles.api.list).toHaveBeenCalledWith('ws')
    expect(result.current.isCapped).toBe(false)
    expect(result.current.cutShort).toBe(false)
    expect(
      result.current.items.filter((item) => !item.is_folder).map((item) => item.file_info.url)
    ).toEqual(['https://s3.example.com/media/a.png', 'https://s3.example.com/media/docs/b.png'])
  })

  it('lists the bucket again once the endpoint or the bucket changes', async () => {
    const { rerender } = renderHook(
      ({ bucket }) => useStorage({ workspaceId: 'ws', settings: { ...settings, bucket } }),
      { wrapper: wrapper(), initialProps: { bucket: 'media' } }
    )
    await waitFor(() => expect(fakeFiles.api.list).toHaveBeenCalledTimes(1))

    rerender({ bucket: 'other' })
    await waitFor(() => expect(fakeFiles.api.list).toHaveBeenCalledTimes(2))
  })

  it('says when the server capped the listing or the storage cut it short', async () => {
    fakeFiles.reset([{ key: 'a.png', lastModified: at(1) }])
    fakeFiles.setListing({ capped: true, cut_short: true })
    const { result } = renderStorage()

    await waitFor(() => expect(result.current.isCapped).toBe(true))
    expect(result.current.cutShort).toBe(true)
  })

  it('lists nothing while the storage has no endpoint', async () => {
    const { result } = renderHook(
      () => useStorage({ workspaceId: 'ws', settings: { ...settings, endpoint: '' } }),
      { wrapper: wrapper() }
    )

    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(fakeFiles.api.list).not.toHaveBeenCalled()
    expect(result.current.items).toEqual([])
    expect(result.current.isLoading).toBe(false)
  })

  it('signs each upload, PUTs the file to the signed URL with its headers, then lists again', async () => {
    const { result } = renderStorage()
    await waitFor(() => expect(fakeFiles.api.list).toHaveBeenCalledTimes(1))

    const file = png('My Photo.PNG')
    const { statuses, written } = await uploadAll(result.current, [file])

    expect(statuses).toEqual(['0:uploading', '0:done'])
    expect(written).toEqual(['photos/My-Photo.png'])
    expect(fakeFiles.api.presignUpload).toHaveBeenCalledWith({
      workspace_id: 'ws',
      key: 'photos/My-Photo.png',
      content_type: 'image/png',
      size: file.size
    })
    const signed = (await fakeFiles.api.presignUpload.mock.results[0].value) as PresignedUpload
    expect(fakeFiles.fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fakeFiles.fetch.mock.calls[0]
    expect(url).toBe(signed.url)
    expect(init).toMatchObject({ method: 'PUT', headers: signed.headers })
    expect(init?.body).toBe(file)
    // Signed before it was sent.
    expect(fakeFiles.api.presignUpload.mock.invocationCallOrder[0]).toBeLessThan(
      fakeFiles.fetch.mock.invocationCallOrder[0]
    )
    await waitFor(() => expect(fakeFiles.api.list).toHaveBeenCalledTimes(2))
  })

  it('stops at an abort during a PUT and sends no further file', async () => {
    const { result } = renderStorage()
    const controller = new AbortController()
    fakeFiles.fetch.mockImplementationOnce(async () => {
      controller.abort()
      throw new DOMException('The operation was aborted.', 'AbortError')
    })

    const { statuses, written } = await uploadAll(
      result.current,
      [png('a.png'), png('b.png')],
      controller.signal
    )

    expect(statuses).toEqual(['0:uploading'])
    expect(written).toEqual([])
    expect(fakeFiles.api.presignUpload).toHaveBeenCalledTimes(1)
    expect(fakeFiles.fetch.mock.calls[0][1]?.signal).toBe(controller.signal)
  })

  it('sends nothing when the abort comes while the upload is being signed', async () => {
    const { result } = renderStorage()
    const controller = new AbortController()
    fakeFiles.api.presignUpload.mockImplementationOnce(async (request) => {
      controller.abort()
      return fakeFiles.impl.presignUpload(request)
    })

    const { statuses, written } = await uploadAll(
      result.current,
      [png('a.png'), png('b.png')],
      controller.signal
    )

    expect(fakeFiles.fetch).not.toHaveBeenCalled()
    expect(fakeFiles.api.presignUpload).toHaveBeenCalledTimes(1)
    expect(statuses).toEqual(['0:uploading'])
    expect(written).toEqual([])
  })

  it('marks a file the storage refused as failed and goes on with the next', async () => {
    const { result } = renderStorage()
    fakeFiles.fetch.mockResolvedValueOnce(new Response(null, { status: 403, statusText: 'Forbidden' }))

    const { statuses, errors, written } = await uploadAll(result.current, [
      png('a.png'),
      png('b.png')
    ])

    expect(statuses).toEqual(['0:uploading', '0:failed', '1:uploading', '1:done'])
    expect(errors).toEqual(['Error: 403 Forbidden'])
    expect(written).toEqual(['photos/b.png'])
    expect(fakeFiles.keys()).toEqual(['photos/b.png'])
  })

  it('points at the bucket’s CORS rules when the PUT never reaches the storage', async () => {
    const { result } = renderStorage()
    fakeFiles.fetch.mockRejectedValueOnce(new TypeError('Failed to fetch'))

    const { statuses, errors } = await uploadAll(result.current, [png('a.png')])

    expect(statuses).toEqual(['0:uploading', '0:failed'])
    expect(errors).toEqual([
      'The storage did not answer. Check that the bucket accepts PUT requests from this origin (CORS).'
    ])
  })

  it('deletes a folder on the server, maps its counts, and lists the bucket again', async () => {
    fakeFiles.reset([
      { key: 'docs/', lastModified: at(1) },
      { key: 'docs/a.png', lastModified: at(1) },
      { key: 'docs/b.png', lastModified: at(1) }
    ])
    fakeFiles.refuseDelete('docs/b.png')
    const { result } = renderStorage()
    await waitFor(() => expect(result.current.items.length).toBeGreaterThan(0))
    const before = fakeFiles.api.list.mock.calls.length

    let outcome: Awaited<ReturnType<typeof result.current.deleteFolder>> | undefined
    await act(async () => {
      outcome = await result.current.deleteFolder('docs/')
    })

    expect(fakeFiles.api.deleteFolder).toHaveBeenCalledWith('ws', 'docs/')
    // The marker is deleted with the files, and counted apart from them.
    expect(outcome).toEqual({ deleted: 2, failed: 1, deletedFiles: 1, failedFiles: 1 })
    await waitFor(() => expect(fakeFiles.api.list.mock.calls.length).toBe(before + 1))
  })

  it('counts the files of a folder from its own listing, markers left out', async () => {
    fakeFiles.reset([
      { key: 'docs/', lastModified: at(1) },
      { key: 'docs/sub/', lastModified: at(1) },
      { key: 'docs/sub/a.png', lastModified: at(1) },
      { key: 'docs/b.pdf', lastModified: at(1) },
      { key: 'other.png', lastModified: at(1) }
    ])
    const { result } = renderStorage()

    let count: Awaited<ReturnType<typeof result.current.countFolderFiles>> | undefined
    await act(async () => {
      count = await result.current.countFolderFiles('docs/')
    })

    expect(count).toEqual({ count: 2, capped: false })
    expect(fakeFiles.api.list).toHaveBeenCalledWith('ws', 'docs/')
  })

  it('says a folder count is a floor when the server capped the folder’s listing', async () => {
    fakeFiles.reset([{ key: 'docs/a.png', lastModified: at(1) }])
    fakeFiles.setListing({ capped: true })
    const { result } = renderStorage()

    await expect(result.current.countFolderFiles('docs/')).resolves.toEqual({ count: 1, capped: true })
  })

  it('deletes the files in one call and counts the failures', async () => {
    fakeFiles.reset([{ key: 'a.png' }, { key: 'b.png' }, { key: 'c.png' }])
    fakeFiles.refuseDelete('b.png')
    const { result } = renderStorage()

    let outcome: { deleted: number; failed: number } | undefined
    await act(async () => {
      outcome = await result.current.deleteFiles(['a.png', 'b.png', 'c.png'])
    })

    expect(fakeFiles.api.delete).toHaveBeenCalledTimes(1)
    expect(fakeFiles.api.delete).toHaveBeenCalledWith('ws', ['a.png', 'b.png', 'c.png'])
    expect(outcome).toEqual({ deleted: 2, failed: 1 })
  })

  it('sends a large selection in batches the server accepts and adds up the counts', async () => {
    const keys = Array.from({ length: 2500 }, (_, i) => `f${i}.png`)
    fakeFiles.reset(keys.map((key) => ({ key })))
    fakeFiles.refuseDelete('f2499.png')
    const { result } = renderStorage()

    let outcome: { deleted: number; failed: number } | undefined
    await act(async () => {
      outcome = await result.current.deleteFiles(keys)
    })

    expect(fakeFiles.api.delete.mock.calls.map(([, batch]) => batch.length)).toEqual([1000, 1000, 500])
    expect(fakeFiles.api.delete.mock.calls.flatMap(([, batch]) => batch)).toEqual(keys)
    expect(outcome).toEqual({ deleted: 2499, failed: 1 })
  })

  it('throws when the storage refuses to delete a single file', async () => {
    fakeFiles.reset([{ key: 'a.png' }])
    fakeFiles.refuseDelete('a.png')
    const { result } = renderStorage()

    await expect(result.current.deleteFile('a.png')).rejects.toThrow(
      'The storage refused to delete a.png'
    )
    expect(fakeFiles.api.delete).toHaveBeenCalledWith('ws', ['a.png'])
  })

  it('refuses a new folder that already exists', async () => {
    fakeFiles.reset([{ key: 'docs/a.png' }])
    const { result } = renderStorage()

    await expect(result.current.createFolder('docs/')).resolves.toBe('exists')
    expect(fakeFiles.api.createFolder).toHaveBeenCalledWith('ws', 'docs/')
    expect(fakeFiles.keys()).toEqual(['docs/a.png'])
  })
})
