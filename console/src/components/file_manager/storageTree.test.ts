import { describe, it, expect } from 'vitest'
import type { _Object } from '@aws-sdk/client-s3'
import { toStorageItems } from './storageObjects'
import {
  ancestorFolders,
  childrenOf,
  fileCounts,
  folderName,
  folderTree,
  parentFolder,
  searchFiles,
  totalSize
} from './storageTree'

const settings = { endpoint: 'https://s3.example.com', access_key: 'k', bucket: 'media' }

const object = (key: string, day: number, size = 0): _Object => ({
  Key: key,
  LastModified: new Date(Date.UTC(2026, 0, day)),
  Size: size
})

// "photos/" has a marker; "docs/" and "docs/2026/" exist only through the keys under them.
const items = toStorageItems(
  [
    object('photos/', 1),
    object('photos/beach.png', 3, 2000),
    object('photos/old.png', 2, 1000),
    object('photos/trips/', 1),
    object('docs/2026/report.pdf', 5, 500),
    object('logo.png', 4, 300),
    object('notes.txt', 6, 200)
  ],
  settings
)

describe('fileCounts', () => {
  it('counts the files under each folder, sub-folders included, and the total at the root', () => {
    const counts = fileCounts(items)
    expect(counts.get('')).toBe(5)
    expect(counts.get('photos/')).toBe(2)
    expect(counts.get('docs/')).toBe(1)
    expect(counts.get('docs/2026/')).toBe(1)
  })

  it('does not count folder markers', () => {
    expect(fileCounts(items).get('photos/trips/')).toBe(0)
  })
})

describe('totalSize', () => {
  it('adds the sizes of the files', () => {
    expect(totalSize(items)).toBe(4000)
  })
})

describe('childrenOf', () => {
  it('lists folders first, then files from the newest', () => {
    expect(childrenOf(items, '').map((item) => item.key)).toEqual([
      'docs/',
      'photos/',
      'notes.txt',
      'logo.png'
    ])
    expect(childrenOf(items, 'photos/').map((item) => item.key)).toEqual([
      'photos/trips/',
      'photos/beach.png',
      'photos/old.png'
    ])
  })

  it('shows an implicit folder like a folder with a marker', () => {
    const docs = childrenOf(items, 'docs/')
    expect(docs.map((item) => item.key)).toEqual(['docs/2026/'])
    expect(docs[0].is_folder).toBe(true)
  })
})

describe('folderTree', () => {
  it('nests every folder under its parent, sorted by name', () => {
    const tree = folderTree(items)
    expect(tree.map((node) => node.key)).toEqual(['docs/', 'photos/'])
    expect(tree[0].children.map((node) => node.key)).toEqual(['docs/2026/'])
    expect(tree[1].children.map((node) => node.name)).toEqual(['trips'])
    expect(tree[1].children[0].children).toEqual([])
  })
})

describe('searchFiles', () => {
  it('finds files whose name contains the text, whatever the case, in every folder', () => {
    expect(searchFiles(items, 'PNG').map((item) => item.key)).toEqual([
      'logo.png',
      'photos/beach.png',
      'photos/old.png'
    ])
  })

  it('leaves folders out and matches the name, not the folder', () => {
    expect(searchFiles(items, 'photos')).toEqual([])
  })

  it('finds nothing for blank text', () => {
    expect(searchFiles(items, '  ')).toEqual([])
  })
})

describe('folder paths', () => {
  it('gives the parent of a folder, the root above a top folder', () => {
    expect(parentFolder('docs/2026/')).toBe('docs/')
    expect(parentFolder('docs/')).toBe('')
  })

  it('lists the folders from the root down to a folder', () => {
    expect(ancestorFolders('docs/2026/')).toEqual(['docs/', 'docs/2026/'])
    expect(ancestorFolders('')).toEqual([])
  })

  it('names a folder after its last segment', () => {
    expect(folderName('docs/2026/')).toBe('2026')
    expect(folderName('')).toBe('')
  })
})
