import {
  DeleteObjectCommand,
  ListObjectsV2Command,
  type S3Client,
  type _Object
} from '@aws-sdk/client-s3'
import { i18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import { filesize } from 'filesize'
import GetContentType from './fileExtensions'
import type { FileManagerSettings, StorageObject } from './interfaces'

// The file manager loads the whole bucket in the browser, a page of 1,000 objects per
// request: past this many objects it stops and says so.
export const MAX_OBJECTS = 10_000

export interface ListAllObjectsResult {
  objects: _Object[]
  // true when the listing stopped at maxObjects while the bucket holds more
  capped: boolean
  // true when the provider said there was more but gave no token to read it
  cutShort: boolean
}

// listAllObjects reads the objects of a bucket, under a prefix when one is given,
// following the continuation tokens until the last page or maxObjects objects.
export const listAllObjects = async (
  client: S3Client,
  bucket: string,
  options: { prefix?: string; maxObjects?: number } = {}
): Promise<ListAllObjectsResult> => {
  const maxObjects = options.maxObjects ?? MAX_OBJECTS
  const objects: _Object[] = []
  let token: string | undefined

  for (;;) {
    const response = await client.send(
      new ListObjectsV2Command({
        Bucket: bucket,
        Prefix: options.prefix,
        ContinuationToken: token
      })
    )
    for (const object of response.Contents ?? []) {
      if (objects.length >= maxObjects) return { objects, capped: true, cutShort: false }
      objects.push(object)
    }
    if (!response.IsTruncated) return { objects, capped: false, cutShort: false }
    // A provider that sets IsTruncated without a token would make the loop run forever:
    // stop, and say the listing is not whole.
    if (!response.NextContinuationToken) return { objects, capped: false, cutShort: true }
    if (objects.length >= maxObjects) return { objects, capped: true, cutShort: false }
    token = response.NextContinuationToken
  }
}

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

const toStorageItem = (object: _Object, baseUrl: string): StorageObject => {
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
  objects: _Object[],
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

// deleteFolder deletes every object under a folder prefix, its marker included, one
// object at a time like the multi-delete. DeleteObjects is not used: several
// S3-compatible providers refuse the checksum header the SDK sends with it.
export const deleteFolder = async (
  client: S3Client,
  bucket: string,
  prefix: string
): Promise<DeleteFolderResult> => {
  // An empty prefix is the whole bucket, and "a" would also match "ab/".
  if (prefix === '' || !prefix.endsWith('/')) {
    throw new Error(i18n._(msg`Not a folder prefix: "${prefix}"`))
  }

  const { objects, cutShort } = await listAllObjects(client, bucket, {
    prefix,
    maxObjects: Number.POSITIVE_INFINITY
  })
  // Deleting part of the folder and reporting success would leave files behind unseen.
  if (cutShort) {
    throw new Error(i18n._(msg`The storage did not return the whole content of "${prefix}"`))
  }

  const result: DeleteFolderResult = { deleted: 0, failed: 0, deletedFiles: 0, failedFiles: 0 }
  for (const object of objects) {
    // A provider that ignored Prefix must not make this delete objects outside the folder.
    if (!object.Key?.startsWith(prefix)) continue
    const isFile = !object.Key.endsWith('/')
    try {
      await client.send(new DeleteObjectCommand({ Bucket: bucket, Key: object.Key }))
      result.deleted++
      if (isFile) result.deletedFiles++
    } catch (error) {
      console.error(`Failed to delete ${object.Key}:`, error)
      result.failed++
      if (isFile) result.failedFiles++
    }
  }
  return result
}
