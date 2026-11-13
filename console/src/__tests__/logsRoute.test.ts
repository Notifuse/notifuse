import { describe, expect, it } from 'vitest'
import { workspaceLogsRoute } from '../router'
import { parseLogsSearch } from '../components/logs/logsSearch'

describe('logs route', () => {
  it('normalises its search with parseLogsSearch', () => {
    expect(workspaceLogsRoute.options.validateSearch).toBe(parseLogsSearch)
  })
})
