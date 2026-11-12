import {
  DeleteObjectCommand,
  ListObjectsV2Command,
  type S3Client,
  type _Object
} from '@aws-sdk/client-s3'
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

const toStorageItem = (object: _Object, baseUrl: string): StorageObject => {
  const key = object.Key as string
  const isFolder = key.endsWith('/')

  const name = isFolder
    ? key
        .split('/')
        .filter((x) => x !== '')
        .pop() || ''
    : key.split('/').pop() || ''

  let itemPath = isFolder
    ? key.split('/').slice(0, -2).join('/') + '/'
    : key.split('/').slice(0, -1).join('/') + '/'
  if (itemPath === '/') itemPath = ''

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
// prefix of a key without its marker becomes a folder too, dated by its newest object.
export const toStorageItems = (
  objects: _Object[],
  settings: FileManagerSettings
): StorageObject[] => {
  const baseUrl = fileBaseUrl(settings)
  const withKeys = objects.filter((object) => typeof object.Key === 'string' && object.Key !== '')
  const keys = new Set(withKeys.map((object) => object.Key as string))
  const implicit = new Map<string, Date | undefined>()

  for (const object of withKeys) {
    const parts = (object.Key as string).split('/')
    for (let i = 1; i < parts.length; i++) {
      // An empty segment ("/x.png", "a//y.png") would make a folder without a name.
      if (parts[i - 1] === '') break
      const prefix = parts.slice(0, i).join('/') + '/'
      if (keys.has(prefix)) continue
      const modified = object.LastModified
      const newest = implicit.get(prefix)
      if (!implicit.has(prefix)) {
        implicit.set(prefix, modified)
      } else if (modified && (!newest || modified > newest)) {
        implicit.set(prefix, modified)
      }
    }
  }

  const items = withKeys.map((object) => toStorageItem(object, baseUrl))
  for (const [prefix, lastModified] of implicit) {
    items.push(toStorageItem({ Key: prefix, LastModified: lastModified }, baseUrl))
  }
  return items
}

export interface DeleteFolderResult {
  deleted: number
  failed: number
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
    throw new Error(`Not a folder prefix: "${prefix}"`)
  }

  const { objects, cutShort } = await listAllObjects(client, bucket, {
    prefix,
    maxObjects: Number.POSITIVE_INFINITY
  })
  // Deleting part of the folder and reporting success would leave files behind unseen.
  if (cutShort) {
    throw new Error(`The storage did not return the whole content of "${prefix}"`)
  }

  let deleted = 0
  let failed = 0
  for (const object of objects) {
    // A provider that ignored Prefix must not make this delete objects outside the folder.
    if (!object.Key?.startsWith(prefix)) continue
    try {
      await client.send(new DeleteObjectCommand({ Bucket: bucket, Key: object.Key }))
      deleted++
    } catch (error) {
      console.error(`Failed to delete ${object.Key}:`, error)
      failed++
    }
  }
  return { deleted, failed }
}
