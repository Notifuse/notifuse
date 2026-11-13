import { zipSync } from 'fflate'
import type { StorageObject } from './interfaces'

const saveBlob = (blob: Blob, name: string) => {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = name
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

/** Saves one file under its own name. */
export async function downloadFile(item: StorageObject) {
  const response = await fetch(item.file_info.url)
  if (!response.ok) throw new Error(`Failed to fetch ${item.name}`)
  saveBlob(await response.blob(), item.name)
}

/** Saves the files in one zip archive. */
export async function downloadZip(items: StorageObject[]) {
  const entries: Record<string, Uint8Array> = {}
  for (const item of items) {
    const response = await fetch(item.file_info.url)
    if (!response.ok) throw new Error(`Failed to fetch ${item.name}`)
    entries[item.name] = new Uint8Array(await response.arrayBuffer())
  }
  const zipped = zipSync(entries)
  saveBlob(new Blob([zipped as BlobPart], { type: 'application/zip' }), `files-${Date.now()}.zip`)
}
