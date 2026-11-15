import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ListStatusBranchConfigForm } from './ListStatusBranchConfigForm'

vi.mock('../context', () => ({ useAutomation: () => ({ lists: [{ id: 'news', name: 'Newsletter' }] }) }))

describe('ListStatusBranchConfigForm', () => {
  it('explains the outputs in a plain info box, without an icon', () => {
    render(
      <I18nProvider i18n={i18n}>
        <ListStatusBranchConfigForm
          config={{ list_id: 'news', not_in_list_node_id: '', active_node_id: '', non_active_node_id: '' }}
          onChange={vi.fn()}
        />
      </I18nProvider>
    )
    expect(screen.getByText('Branch Logic')).toBeInTheDocument()
    expect(document.querySelector('.ant-alert')).toBeInTheDocument()
    expect(document.querySelector('.ant-alert-icon')).toBeNull()
  })
})
