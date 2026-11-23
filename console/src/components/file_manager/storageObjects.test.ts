import { describe, it, expect } from 'vitest'
import { fromStoredObjects, toStorageItems, MAX_OBJECTS, type BucketObject } from './storageObjects'
import type { FileManagerSettings } from './interfaces'

const settings: FileManagerSettings = {
  endpoint: 'https://s3.example.com',
  access_key: 'key',
  bucket: 'media'
}

const obj = (key: string, lastModified = '2026-11-01T00:00:00Z', size = 10): BucketObject => ({
  Key: key,
  LastModified: new Date(lastModified),
  Size: size
})

describe('MAX_OBJECTS', () => {
  it('mirrors the server, which lists ten thousand objects at most', () => {
    expect(MAX_OBJECTS).toBe(10_000)
  })
})

describe('fromStoredObjects', () => {
  it('turns the server listing into objects with S3 field names and dates', () => {
    expect(
      fromStoredObjects([{ key: 'docs/a.png', size: 12, last_modified: '2026-11-01T10:00:00Z' }])
    ).toEqual([{ Key: 'docs/a.png', Size: 12, LastModified: new Date('2026-11-01T10:00:00Z') }])
  })

  it('gives toStorageItems the keys, sizes and dates it builds items from', () => {
    const [file] = toStorageItems(
      fromStoredObjects([{ key: 'a.png', size: 2048, last_modified: '2026-11-01T00:00:00Z' }]),
      settings
    )
    expect(file).toMatchObject({
      key: 'a.png',
      last_modified: new Date('2026-11-01T00:00:00Z'),
      file_info: { size: 2048, url: 'https://s3.example.com/media/a.png' }
    })
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
    const [file] = toStorageItems([obj('a.png')], {
      ...settings,
      cdn_endpoint: 'https://cdn.example.com/'
    })
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

    expect(items.find((i) => i.key === 'a/')?.last_modified).toEqual(
      new Date('2026-11-05T00:00:00Z')
    )
    expect(items.find((i) => i.key === 'a/b/')?.last_modified).toEqual(
      new Date('2026-11-03T00:00:00Z')
    )
  })

  it('dates an implicit folder by its newest file whatever the listing order', () => {
    const items = toStorageItems(
      [obj('a/new.png', '2026-11-09T00:00:00Z'), obj('a/old.png', '2026-11-02T00:00:00Z')],
      settings
    )

    expect(items.find((i) => i.key === 'a/')?.last_modified).toEqual(
      new Date('2026-11-09T00:00:00Z')
    )
  })

  it('marks the folders that exist without a marker', () => {
    const items = toStorageItems([obj('a/'), obj('a/b/c.png')], settings)

    expect(items.find((i) => i.key === 'a/')?.implicit).toBeUndefined()
    expect(items.find((i) => i.key === 'a/b/')?.implicit).toBe(true)
  })

  it('does not duplicate a folder that has its marker', () => {
    const items = toStorageItems([obj('a/'), obj('a/c.png')], settings)

    expect(items.filter((i) => i.key === 'a/')).toHaveLength(1)
  })

  it('creates the parent folder of a nested folder marker', () => {
    const items = toStorageItems([obj('a/b/')], settings)

    expect(
      items
        .filter((i) => i.is_folder)
        .map((f) => f.key)
        .sort()
    ).toEqual(['a/', 'a/b/'])
  })

  it('creates no nameless folder for a key with an empty segment', () => {
    const items = toStorageItems([obj('/x.png'), obj('a//y.png')], settings)

    expect(items.filter((i) => i.is_folder && i.name === '')).toEqual([])
  })

  it('places a key with an empty segment in the folder its other segments name', () => {
    const items = toStorageItems([obj('/x.png'), obj('a//y.png'), obj('b//')], settings)

    expect(items.find((i) => i.key === 'a//y.png')).toMatchObject({ name: 'y.png', path: 'a/' })
    expect(items.find((i) => i.key === '/x.png')).toMatchObject({ name: 'x.png', path: '' })
    // The folders a/ and b/, each once: the marker "b//" is the folder b/
    expect(
      items
        .filter((i) => i.is_folder)
        .map((i) => [i.name, i.path])
        .sort()
    ).toEqual([
      ['a', ''],
      ['b', '']
    ])
  })
})
