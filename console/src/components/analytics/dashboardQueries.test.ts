import { describe, expect, it } from 'vitest'
import { contactsTotalQuery, latestContactsQuery } from './dashboardQueries'
import { emailTemplatesQuery, membersQuery } from '../../services/api/sharedQueries'

// Other screens invalidate these keys (the CSV import under ['contacts', ws], a template saved
// under ['templates', ws], the team settings under ['workspace-members', ws]): they must not drift.
describe('shared query keys', () => {
  it('keeps the dashboard contacts under the contacts key', () => {
    expect(latestContactsQuery('ws1').queryKey).toEqual(['contacts', 'ws1', 'dashboard-latest'])
    expect(contactsTotalQuery('ws1', 'Europe/Paris').queryKey).toEqual(['contacts', 'ws1', 'dashboard-total'])
  })

  it('keeps the email templates and the members under the keys their screens use', () => {
    expect(emailTemplatesQuery('ws1').queryKey).toEqual(['templates', 'ws1', 'email'])
    expect(membersQuery('ws1', 'u1').queryKey).toEqual(['workspace-members', 'ws1', 'u1'])
  })
})
