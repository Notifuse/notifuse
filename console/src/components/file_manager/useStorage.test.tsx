import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  DeleteObjectCommand,
  ListObjectsV2Command,
  PutObjectCommand
} from '@aws-sdk/client-s3'
import { useStorage } from './useStorage'

// Every S3Client the hook builds answers through this send.
const s3 = vi.hoisted(() => ({ send: vi.fn() }))
vi.mock('@aws-sdk/client-s3', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@aws-sdk/client-s3')>()
  return {
    ...actual,
    S3Client: class {
      send = s3.send
    }
  }
})

const settings = { endpoint: 'https://s3.example.com', access_key: 'key', bucket: 'media' }

const wrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

const listCalls = () =>
  s3.send.mock.calls.filter(([command]) => command instanceof ListObjectsV2Command)

const at = (day: number) => new Date(Date.UTC(2026, 0, day))

describe('useStorage', () => {
  beforeEach(() => {
    s3.send.mockReset()
  })

  it('reads every page of the bucket until the last one', async () => {
    s3.send.mockImplementation(async (command: ListObjectsV2Command) => {
      if (!command.input.ContinuationToken) {
        return {
          Contents: [{ Key: 'a.png', LastModified: at(1), Size: 10 }],
          IsTruncated: true,
          NextContinuationToken: 'next'
        }
      }
      return { Contents: [{ Key: 'b.png', LastModified: at(2), Size: 20 }], IsTruncated: false }
    })

    const { result } = renderHook(() => useStorage({ workspaceId: 'ws', settings }), {
      wrapper: wrapper()
    })

    await waitFor(() => expect(result.current.items).toHaveLength(2))
    expect(s3.send).toHaveBeenCalledTimes(2)
    expect(result.current.isCapped).toBe(false)
    expect(result.current.items.map((item) => item.file_info.url)).toEqual([
      'https://s3.example.com/media/a.png',
      'https://s3.example.com/media/b.png'
    ])
  })

  it('stops at the cap it is given and says so', async () => {
    s3.send.mockResolvedValue({
      Contents: ['a.png', 'b.png', 'c.png'].map((Key) => ({ Key, LastModified: at(1) })),
      IsTruncated: false
    })

    const { result } = renderHook(
      () => useStorage({ workspaceId: 'ws', settings, maxObjects: 2 }),
      { wrapper: wrapper() }
    )

    await waitFor(() => expect(result.current.isCapped).toBe(true))
    expect(result.current.items).toHaveLength(2)
  })

  it('lists nothing while the storage has no endpoint', async () => {
    const { result } = renderHook(
      () => useStorage({ workspaceId: 'ws', settings: { ...settings, endpoint: '' } }),
      { wrapper: wrapper() }
    )

    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(s3.send).not.toHaveBeenCalled()
    expect(result.current.items).toEqual([])
    expect(result.current.isLoading).toBe(false)
  })

  it('lists the bucket again once an upload ends, with each file under the folder', async () => {
    s3.send.mockResolvedValue({ Contents: [], IsTruncated: false })
    const { result } = renderHook(() => useStorage({ workspaceId: 'ws', settings }), {
      wrapper: wrapper()
    })
    await waitFor(() => expect(listCalls()).toHaveLength(1))

    const statuses: string[] = []
    const file = new File(['x'], 'My Photo.PNG', { type: 'image/png' })
    // jsdom's File has no arrayBuffer(); browsers do.
    Object.defineProperty(file, 'arrayBuffer', { value: async () => new ArrayBuffer(1) })
    let uploaded: string[] = []
    await act(async () => {
      uploaded = await result.current.upload([file], 'photos/', {
        onStatus: (index, status) => statuses.push(`${index}:${status}`)
      })
    })

    expect(statuses).toEqual(['0:uploading', '0:done'])
    const put = s3.send.mock.calls.find(([command]) => command instanceof PutObjectCommand)
    expect(put?.[0].input).toMatchObject({
      Bucket: 'media',
      Key: 'photos/My-Photo.png',
      ContentType: 'image/png'
    })
    expect(uploaded).toEqual(['photos/My-Photo.png'])
    await waitFor(() => expect(listCalls()).toHaveLength(2))
  })

  it('counts the files of a folder it could not delete, and lists the bucket again', async () => {
    s3.send.mockImplementation(async (command: unknown) => {
      if (command instanceof ListObjectsV2Command) {
        return {
          Contents: [
            { Key: 'docs/', LastModified: at(1) },
            { Key: 'docs/a.png', LastModified: at(1) },
            { Key: 'docs/b.png', LastModified: at(1) }
          ],
          IsTruncated: false
        }
      }
      if (command instanceof DeleteObjectCommand && command.input.Key === 'docs/b.png') {
        throw new Error('denied')
      }
      return {}
    })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderHook(() => useStorage({ workspaceId: 'ws', settings }), {
      wrapper: wrapper()
    })
    await waitFor(() => expect(result.current.items.length).toBeGreaterThan(0))
    const before = listCalls().length

    let outcome: { deleted: number; failed: number } | undefined
    await act(async () => {
      outcome = await result.current.deleteFolder('docs/')
    })

    // The marker is deleted with the files, and counted apart from them.
    expect(outcome).toEqual({ deleted: 2, failed: 1, deletedFiles: 1, failedFiles: 1 })
    // One listing of the folder for the delete, then the bucket again.
    await waitFor(() => expect(listCalls().length).toBe(before + 2))
  })

  it('counts the files of a folder from its own listing, markers left out', async () => {
    s3.send.mockResolvedValue({
      Contents: [
        { Key: 'docs/', LastModified: at(1) },
        { Key: 'docs/sub/', LastModified: at(1) },
        { Key: 'docs/sub/a.png', LastModified: at(1) },
        { Key: 'docs/b.pdf', LastModified: at(1) }
      ],
      IsTruncated: false
    })
    const { result } = renderHook(() => useStorage({ workspaceId: 'ws', settings }), {
      wrapper: wrapper()
    })

    let count = 0
    await act(async () => {
      count = await result.current.countFolderFiles('docs/')
    })

    expect(count).toBe(2)
    expect(listCalls().some(([command]) => command.input.Prefix === 'docs/')).toBe(true)
  })

  it('deletes the files one by one and counts the failures', async () => {
    s3.send.mockImplementation(async (command: unknown) => {
      if (command instanceof ListObjectsV2Command) return { Contents: [], IsTruncated: false }
      if (command instanceof DeleteObjectCommand && command.input.Key === 'b.png') {
        throw new Error('denied')
      }
      return {}
    })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderHook(() => useStorage({ workspaceId: 'ws', settings }), {
      wrapper: wrapper()
    })

    let outcome: { deleted: number; failed: number } | undefined
    await act(async () => {
      outcome = await result.current.deleteFiles(['a.png', 'b.png', 'c.png'])
    })

    expect(outcome).toEqual({ deleted: 2, failed: 1 })
  })

  it('refuses a new folder that already exists', async () => {
    s3.send.mockImplementation(async (command: ListObjectsV2Command) => {
      if (command.input.Prefix === 'docs/') {
        return { Contents: [{ Key: 'docs/a.png' }], IsTruncated: false }
      }
      return { Contents: [], IsTruncated: false }
    })
    const { result } = renderHook(() => useStorage({ workspaceId: 'ws', settings }), {
      wrapper: wrapper()
    })

    await expect(result.current.createFolder('docs/')).resolves.toBe('exists')
    expect(s3.send.mock.calls.some(([command]) => command instanceof PutObjectCommand)).toBe(false)
  })
})
