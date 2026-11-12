import { describe, it, expect } from 'vitest'
import { contactDisplayName, contactInitials, LIST_STATUS_TONE } from './contactDisplay'

describe('contactDisplay', () => {
  it('names a contact by full name, else first and last name, else nothing', () => {
    expect(contactDisplayName({ email: 'a@x.com', full_name: 'Ann Lee', first_name: 'A' })).toBe('Ann Lee')
    expect(contactDisplayName({ email: 'a@x.com', first_name: 'Ann', last_name: 'Lee' })).toBe('Ann Lee')
    expect(contactDisplayName({ email: 'a@x.com', last_name: 'Lee' })).toBe('Lee')
    expect(contactDisplayName({ email: 'a@x.com' })).toBe('')
  })

  it('takes two initials from the name, else from the email', () => {
    expect(contactInitials({ email: 'a@x.com', first_name: 'camille', last_name: 'Martin' })).toBe('CM')
    expect(contactInitials({ email: 'a@x.com', full_name: 'Jean-Pierre de la Tour' })).toBe('JT')
    expect(contactInitials({ email: 'zoe@x.com', full_name: 'Zoé' })).toBe('Z')
    expect(contactInitials({ email: 'zoe.martin@x.com' })).toBe('ZM')
    expect(contactInitials({ email: 'zoe@x.com' })).toBe('Z')
  })

  it('gives each list status its tone', () => {
    expect(LIST_STATUS_TONE).toEqual({
      active: 'success',
      pending: 'warning',
      unsubscribed: 'neutral',
      bounced: 'danger',
      complained: 'danger'
    })
  })
})
