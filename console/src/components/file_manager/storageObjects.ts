import { filesize } from 'filesize'
import type { StoredObject } from '../../services/api/files'
import GetContentType from './fileExtensions'
import type { FileManagerSettings, StorageObject } from './interfaces'

// The server lists at most this many objects (domain.MaxListedStorageObjects); past it the
// listing says it is capped.
export const MAX_OBJECTS = 10_000

// An object as toStorageItems reads it, with the S3 field names.
export interface BucketObject {
  Key?: string
  Size?: number
  LastModified?: Date
}

// fromStoredObjects turns the server's listing into BucketObjects.
export const fromStoredObjects = (objects: StoredObject[]): BucketObject[] =>
  objects.map((object) => ({
    Key: object.key,
    Size: object.size,
    LastModified: new Date(object.last_modified)
  }))

const fileBaseUrl = (settings: FileManagerSettings): string => {
  if (settings.cdn_endpoint && settings.cdn_endpoint.trim() !== '') {
    return settings.cdn_endpoint.replace(/\/$/, '')
  }
  if (settings.endpoint && settings.bucket) {
    return `${settings.endpoint.replace(/\/$/, '')}/${settings.bucket}`
  }
  return ''
}

// The non-empty segments of a key: "a//y.png" sits in "a/", like "a/y.png", and "/x.png" at the root.
const segmentsOf = (key: string): string[] => key.split('/').filter((segment) => segment !== '')

// The folder prefix a marker stands for once its empty segments are dropped: "a//" is "a/".
const folderPrefixOf = (key: string): string => segmentsOf(key).join('/') + '/'

const toStorageItem = (object: BucketObject, baseUrl: string): StorageObject => {
  const key = object.Key as string
  const isFolder = key.endsWith('/')

  const segments = segmentsOf(key)
  const name = segments.at(-1) ?? ''
  const parents = segments.slice(0, -1)
  const itemPath = parents.length > 0 ? parents.join('/') + '/' : ''

  const item = {
    key,
    name,
    path: itemPath,
    is_folder: isFolder,
    last_modified: object.LastModified
  } as StorageObject

  if (!isFolder) {
    // Encode each path segment separately: spaces and special characters are encoded,
    // slashes are kept.
    const encodedKey = key
      .split('/')
      .map((segment) => encodeURIComponent(segment))
      .join('/')
    item.file_info = {
      size: object.Size as number,
      size_human: filesize(object.Size || 0, { round: 0 }),
      content_type: GetContentType(key),
      url: baseUrl ? `${baseUrl}/${encodedKey}` : encodedKey
    }
  }

  return item
}

// toStorageItems maps the bucket's objects to file manager items. A folder exists in S3
// only as a prefix: Notifuse writes an empty "folder/" marker, other tools do not. Each
// prefix of a key without its marker becomes a folder too, dated by its newest object. Empty
// segments are dropped: a marker such as "a//" is the folder "a/", shown once.
export const toStorageItems = (
  objects: BucketObject[],
  settings: FileManagerSettings
): StorageObject[] => {
  const baseUrl = fileBaseUrl(settings)
  const withKeys = objects.filter(
    (object) => typeof object.Key === 'string' && segmentsOf(object.Key).length > 0
  )
  // The markers shown as such: those without an empty segment
  const isPlainMarker = (key: string) => key.endsWith('/') && folderPrefixOf(key) === key
  const markers = new Set(withKeys.map((object) => object.Key as string).filter(isPlainMarker))
  const implicit = new Map<string, Date | undefined>()

  for (const object of withKeys) {
    const key = object.Key as string
    const parts = segmentsOf(key)
    // A file's folders are the prefixes above it; a marker with an empty segment is its own too.
    const last = key.endsWith('/') ? parts.length : parts.length - 1
    for (let i = 1; i <= last; i++) {
      const prefix = parts.slice(0, i).join('/') + '/'
      if (markers.has(prefix)) continue
      const modified = object.LastModified
      const newest = implicit.get(prefix)
      if (!implicit.has(prefix)) {
        implicit.set(prefix, modified)
      } else if (modified && (!newest || modified > newest)) {
        implicit.set(prefix, modified)
      }
    }
  }

  const items = withKeys
    .filter((object) => !(object.Key as string).endsWith('/') || markers.has(object.Key as string))
    .map((object) => toStorageItem(object, baseUrl))
  for (const [prefix, lastModified] of implicit) {
    items.push({ ...toStorageItem({ Key: prefix, LastModified: lastModified }, baseUrl), implicit: true })
  }
  return items
}

export interface DeleteFolderResult {
  deleted: number
  failed: number
  // The same counts without the folder markers, for a message that speaks of files.
  deletedFiles: number
  failedFiles: number
}
