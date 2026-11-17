import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { StepStats } from './StepStats'
import type { NodeStats } from '../../../services/api/automation'

const stats = {
  node_type: 'email',
  visits: 10,
  completed: 7,
  failed_attempts: 0,
  skipped: 0,
  skip_reasons: {},
  errors: 0,
  failed_journeys: 2,
  in_progress: 1,
  waiting: 0,
  retrying: 0,
  branches: {}
} as NodeStats

describe('StepStats', () => {
  it('names the journeys that failed, apart from the emails that failed beside it', () => {
    // An email step showed two different "failed" numbers side by side: journeys and emails.
    render(
      <I18nProvider i18n={i18n}>
        <StepStats nodeType="email" config={{}} stats={stats} />
      </I18nProvider>
    )
    expect(screen.getByText('Failed journeys')).toBeInTheDocument()
    expect(screen.queryByText('Failed')).toBeNull()
  })
})
