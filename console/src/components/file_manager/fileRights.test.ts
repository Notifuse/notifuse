import { describe, it, expect } from 'vitest'
import { canEditStorageSettings, canReadFiles, canWriteFiles } from './fileRights'

describe('file rights', () => {
  it('gives an owner and root every right', () => {
    for (const role of ['owner', 'root'] as const) {
      expect(canReadFiles(null, role)).toBe(true)
      expect(canWriteFiles(null, role)).toBe(true)
      expect(canEditStorageSettings(role)).toBe(true)
    }
  })

  it('follows templates and the blog for a member', () => {
    const none = {}
    const contacts = { contacts: { read: true, write: true } }
    const templatesRead = { templates: { read: true, write: false } }
    const blogWriteOnly = { blog: { read: false, write: true } }

    expect(canReadFiles(none, 'member')).toBe(false)
    expect(canReadFiles(contacts, 'member')).toBe(false)
    expect(canReadFiles(templatesRead, 'member')).toBe(true)
    expect(canWriteFiles(templatesRead, 'member')).toBe(false)
    // Writing implies seeing the files: the blog settings pick an image with blog:write alone.
    expect(canReadFiles(blogWriteOnly, 'member')).toBe(true)
    expect(canWriteFiles(blogWriteOnly, 'member')).toBe(true)
    expect(canEditStorageSettings('member')).toBe(false)
  })

  it('grants nothing while the role is unknown', () => {
    expect(canReadFiles(null, undefined)).toBe(false)
    expect(canEditStorageSettings(undefined)).toBe(false)
  })
})
