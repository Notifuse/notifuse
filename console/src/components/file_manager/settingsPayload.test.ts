import { describe, it, expect } from 'vitest'
import { savedFileManagerSettings } from './settingsPayload'

describe('savedFileManagerSettings', () => {
  const received = {
    provider: 'other',
    endpoint: 'https://s3.example.com',
    access_key: 'AK',
    bucket: 'assets',
    region: 'us-east-1',
    cdn_endpoint: 'https://cdn.example.com',
    force_path_style: true,
    has_secret_key: true,
    secret_key_hint: 'abcd'
  }

  it('never sends the response-only fields, nor an empty secret', () => {
    const saved = savedFileManagerSettings({ ...received, secret_key: '' })
    expect(saved).not.toHaveProperty('has_secret_key')
    expect(saved).not.toHaveProperty('secret_key_hint')
    expect(saved).not.toHaveProperty('secret_key')
    expect(saved.endpoint).toBe('https://s3.example.com')
    expect(saved.force_path_style).toBe(true)
  })

  it('sends a typed secret', () => {
    expect(savedFileManagerSettings({ ...received, secret_key: 'new' }).secret_key).toBe('new')
  })
})
