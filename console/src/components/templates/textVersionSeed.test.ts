import { describe, it, expect } from 'vitest'
import { customTextOrUndefined, missingRequiredLink } from './textVersionSeed'

describe('customTextOrUndefined', () => {
  it('keeps a text as written', () => {
    expect(customTextOrUndefined('  Hi\n')).toBe('  Hi\n')
  })

  it('treats an absent or blank text as the automatic version', () => {
    expect(customTextOrUndefined(undefined)).toBeUndefined()
    expect(customTextOrUndefined(null)).toBeUndefined()
    expect(customTextOrUndefined(' \n\t')).toBeUndefined()
  })
})

describe('missingRequiredLink', () => {
  it('names the unsubscribe link a marketing text misses', () => {
    expect(missingRequiredLink('marketing', 'Hi')).toBe('unsubscribe_url')
    expect(missingRequiredLink('marketing', 'Bye: {{ unsubscribe_url }}')).toBeUndefined()
  })

  it('names the confirmation link an opt-in text misses', () => {
    expect(missingRequiredLink('opt_in', 'Hi')).toBe('confirm_subscription_url')
    expect(missingRequiredLink('opt_in', '{{confirm_subscription_url}}')).toBeUndefined()
  })

  it('asks nothing of other categories, nor of the automatic version', () => {
    expect(missingRequiredLink('transactional', 'Hi')).toBeUndefined()
    expect(missingRequiredLink(undefined, 'Hi')).toBeUndefined()
    expect(missingRequiredLink('marketing', undefined)).toBeUndefined()
    expect(missingRequiredLink('marketing', '  ')).toBeUndefined()
  })
})
