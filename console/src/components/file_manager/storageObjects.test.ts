import { describe, it, expect, vi } from 'vitest'
import {
  DeleteObjectCommand,
  ListObjectsV2Command,
  type S3Client,
  type _Object
} from '@aws-sdk/client-s3'
import { deleteFolder, listAllObjects, toStorageItems, MAX_OBJECTS } from './storageObjects'
import type { FileManagerSettings } from './interfaces'

const settings: FileManagerSettings = {
  endpoint: 'https://s3.example.com',
  access_key: 'key',
  bucket: 'media'
}

const obj = (key: string, lastModified = '2026-11-01T00:00:00Z', size = 10): _Object => ({
  Key: key,
  LastModified: new Date(lastModified),
  Size: size
})

// A client whose send answers each ListObjectsV2 call with the next page, and each
// DeleteObject with the given handler.
const fakeClient = (
  pages: { Contents?: _Object[]; IsTruncated?: boolean; NextContinuationToken?: string }[],
  onDelete: (key: string) => Promise<unknown> = async () => ({})
) => {
  let page = 0
  const send = vi.fn(async (command: unknown) => {
    if (command instanceof ListObjectsV2Command) return pages[page++] ?? { Contents: [] }
    if (command instanceof DeleteObjectCommand) return onDelete(command.input.Key as string)
    throw new Error('unexpected command')
  })
  return { client: { send } as unknown as S3Client, send }
}

const listInputs = (send: ReturnType<typeof vi.fn>) =>
  send.mock.calls
    .map(([command]) => command)
    .filter((command): command is ListObjectsV2Command => command instanceof ListObjectsV2Command)
    .map((command) => command.input)

describe('listAllObjects', () => {
  it('reads every page, passing each continuation token to the next call', async () => {
    const { client, send } = fakeClient([
      { Contents: [obj('a.png')], IsTruncated: true, NextContinuationToken: 't1' },
      { Contents: [obj('b.png')], IsTruncated: true, NextContinuationToken: 't2' },
      { Contents: [obj('c.png')], IsTruncated: false }
    ])

    const result = await listAllObjects(client, 'media')

    expect(result.objects.map((o) => o.Key)).toEqual(['a.png', 'b.png', 'c.png'])
    expect(result.capped).toBe(false)
    expect(listInputs(send)).toEqual([
      { Bucket: 'media', Prefix: undefined, ContinuationToken: undefined },
      { Bucket: 'media', Prefix: undefined, ContinuationToken: 't1' },
      { Bucket: 'media', Prefix: undefined, ContinuationToken: 't2' }
    ])
  })

  it('stops at the cap and says so', async () => {
    const { client, send } = fakeClient([
      { Contents: [obj('a.png'), obj('b.png')], IsTruncated: true, NextContinuationToken: 't1' },
      { Contents: [obj('c.png'), obj('d.png')], IsTruncated: false }
    ])

    const result = await listAllObjects(client, 'media', { maxObjects: 2 })

    expect(result.objects.map((o) => o.Key)).toEqual(['a.png', 'b.png'])
    expect(result.capped).toBe(true)
    expect(send).toHaveBeenCalledTimes(1)
  })

  it('cuts a page that goes past the cap', async () => {
    const { client } = fakeClient([
      { Contents: [obj('a.png'), obj('b.png'), obj('c.png')], IsTruncated: false }
    ])

    const result = await listAllObjects(client, 'media', { maxObjects: 2 })

    expect(result.objects.map((o) => o.Key)).toEqual(['a.png', 'b.png'])
    expect(result.capped).toBe(true)
  })

  it('is not capped when the bucket holds exactly the cap', async () => {
    const { client } = fakeClient([{ Contents: [obj('a.png'), obj('b.png')], IsTruncated: false }])

    const result = await listAllObjects(client, 'media', { maxObjects: 2 })

    expect(result.capped).toBe(false)
  })

  it('stops when a truncated page carries no continuation token', async () => {
    const { client, send } = fakeClient([{ Contents: [obj('a.png')], IsTruncated: true }])

    const result = await listAllObjects(client, 'media')

    expect(result.objects).toHaveLength(1)
    expect(send).toHaveBeenCalledTimes(1)
  })

  it('caps the listing of the file manager at ten thousand files', () => {
    expect(MAX_OBJECTS).toBe(10_000)
  })
})

describe('toStorageItems', () => {
  it('maps files and folder markers as before', () => {
    const items = toStorageItems([obj('docs/'), obj('docs/My File.PDF', undefined, 2048)], settings)

    const folder = items.find((i) => i.key === 'docs/')
    expect(folder).toMatchObject({ name: 'docs', path: '', is_folder: true })
    expect(folder?.file_info).toBeUndefined()

    const file = items.find((i) => i.key === 'docs/My File.PDF')
    expect(file).toMatchObject({ name: 'My File.PDF', path: 'docs/', is_folder: false })
    expect(file?.file_info).toMatchObject({
      size: 2048,
      url: 'https://s3.example.com/media/docs/My%20File.PDF'
    })
  })

  it('prefers the CDN endpoint for file URLs', () => {
    const [file] = toStorageItems([obj('a.png')], { ...settings, cdn_endpoint: 'https://cdn.example.com/' })
    expect(file.file_info.url).toBe('https://cdn.example.com/a.png')
  })

  it('creates the folders of a file that has no folder markers', () => {
    const items = toStorageItems(
      [obj('a/b/c.png', '2026-11-03T00:00:00Z'), obj('a/d.png', '2026-11-05T00:00:00Z')],
      settings
    )

    const folders = items.filter((i) => i.is_folder)
    expect(folders.map((f) => f.key).sort()).toEqual(['a/', 'a/b/'])
    expect(items.find((i) => i.key === 'a/')).toMatchObject({ name: 'a', path: '' })
    expect(items.find((i) => i.key === 'a/b/')).toMatchObject({ name: 'b', path: 'a/' })
  })

  it('dates an implicit folder by its newest file', () => {
    const items = toStorageItems(
      [obj('a/b/c.png', '2026-11-03T00:00:00Z'), obj('a/d.png', '2026-11-05T00:00:00Z')],
      settings
    )

    expect(items.find((i) => i.key === 'a/')?.last_modified).toEqual(new Date('2026-11-05T00:00:00Z'))
    expect(items.find((i) => i.key === 'a/b/')?.last_modified).toEqual(new Date('2026-11-03T00:00:00Z'))
  })

  it('does not duplicate a folder that has its marker', () => {
    const items = toStorageItems([obj('a/'), obj('a/c.png')], settings)

    expect(items.filter((i) => i.key === 'a/')).toHaveLength(1)
  })

  it('creates the parent folder of a nested folder marker', () => {
    const items = toStorageItems([obj('a/b/')], settings)

    expect(items.filter((i) => i.is_folder).map((f) => f.key).sort()).toEqual(['a/', 'a/b/'])
  })

  it('creates no nameless folder for a key with an empty segment', () => {
    const items = toStorageItems([obj('/x.png'), obj('a//y.png')], settings)

    expect(items.filter((i) => i.is_folder && i.name === '')).toEqual([])
  })
})

describe('deleteFolder', () => {
  it('deletes every object under the prefix, its marker included', async () => {
    const { client, send } = fakeClient([
      { Contents: [obj('a/'), obj('a/b.png')], IsTruncated: true, NextContinuationToken: 't1' },
      { Contents: [obj('a/c/d.png')], IsTruncated: false }
    ])

    const result = await deleteFolder(client, 'media', 'a/')

    expect(result).toEqual({ deleted: 3, failed: 0 })
    expect(listInputs(send).every((input) => input.Prefix === 'a/')).toBe(true)
    const deletedKeys = send.mock.calls
      .map(([command]) => command)
      .filter((command): command is DeleteObjectCommand => command instanceof DeleteObjectCommand)
      .map((command) => command.input)
    expect(deletedKeys).toEqual([
      { Bucket: 'media', Key: 'a/' },
      { Bucket: 'media', Key: 'a/b.png' },
      { Bucket: 'media', Key: 'a/c/d.png' }
    ])
  })

  it('deletes one object at a time', async () => {
    let running = 0
    let peak = 0
    const { client } = fakeClient(
      [{ Contents: [obj('a/1'), obj('a/2'), obj('a/3')], IsTruncated: false }],
      async () => {
        running++
        peak = Math.max(peak, running)
        await new Promise((resolve) => setTimeout(resolve, 1))
        running--
        return {}
      }
    )

    await deleteFolder(client, 'media', 'a/')

    expect(peak).toBe(1)
  })

  it('counts the objects it could not delete and goes on', async () => {
    const { client } = fakeClient(
      [{ Contents: [obj('a/1'), obj('a/2'), obj('a/3')], IsTruncated: false }],
      async (key) => {
        if (key === 'a/2') throw new Error('denied')
        return {}
      }
    )

    const result = await deleteFolder(client, 'media', 'a/')

    expect(result).toEqual({ deleted: 2, failed: 1 })
  })

  it.each(['', 'a', 'a/b'])('refuses the prefix %j, which is not a folder', async (prefix) => {
    const { client, send } = fakeClient([])

    await expect(deleteFolder(client, 'media', prefix)).rejects.toThrow()
    expect(send).not.toHaveBeenCalled()
  })
})
