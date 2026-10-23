import { describe, it, expect } from 'vitest'
import { truncateIdentifier } from './identifiers'

describe('truncateIdentifier', () => {
  it('returns an id within the limit unchanged, trailing separator included', () => {
    expect(truncateIdentifier('welcome-email', 32)).toBe('welcome-email')
    expect(truncateIdentifier('a'.repeat(32), 32)).toBe('a'.repeat(32))
    expect(truncateIdentifier('_', 32)).toBe('_')
    expect(truncateIdentifier('welcome_', 32)).toBe('welcome_')
  })

  it('cuts a longer id to the limit', () => {
    const cut = truncateIdentifier('a'.repeat(40), 32)
    expect(cut).toBe('a'.repeat(32))
  })

  it('drops the hyphens and underscores the cut leaves at the end', () => {
    // 'welcome-email-for-new-customers-' is the first 32 characters
    expect(truncateIdentifier('welcome-email-for-new-customers-in-france', 32)).toBe(
      'welcome-email-for-new-customers'
    )
    // 'welcome_email_for_new_customer__' is the first 32 characters
    expect(truncateIdentifier('welcome_email_for_new_customer__in_france', 32)).toBe(
      'welcome_email_for_new_customer'
    )
  })

  it('keeps an empty id empty', () => {
    expect(truncateIdentifier('', 32)).toBe('')
  })
})
