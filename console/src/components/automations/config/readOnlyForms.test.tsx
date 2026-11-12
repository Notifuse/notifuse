import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Workspace } from '../../../services/api/types'
import { EmailConfigForm } from './EmailConfigForm'
import { ABTestConfigForm } from './ABTestConfigForm'

// The real template field: its own `disabled` wins over a ConfigProvider, which is the point here.
vi.mock('../../../services/api/template', () => ({
  templatesApi: {
    list: vi.fn().mockResolvedValue({ templates: [] }),
    get: vi.fn().mockResolvedValue({ template: { id: 'tpl-welcome', name: 'Welcome' } })
  }
}))
vi.mock('../../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace' }] })
}))

const workspace = { id: 'ws1', integrations: [] } as unknown as Workspace

// As in the step panel: read-only puts the forms under a disabled ConfigProvider, and tells them.
const readOnly = (children: ReactNode) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider componentDisabled>{children}</ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )

describe('step forms read-only', () => {
  it('locks the email template, and opens no picker', () => {
    readOnly(<EmailConfigForm config={{ template_id: 'tpl-welcome' }} onChange={vi.fn()} workspaceId="ws1" workspace={workspace} readOnly />)
    const inputs = screen.getAllByRole('textbox')
    inputs.forEach((input) => expect(input).toBeDisabled())
    fireEvent.click(inputs[0])
    expect(screen.queryByText('Select Template')).not.toBeInTheDocument()
  })

  it('locks the removal of an A/B variant', () => {
    const variants = ['A', 'B', 'C'].map((id) => ({ id, name: `Variant ${id}`, weight: id === 'A' ? 34 : 33, next_node_id: '' }))
    const { container } = readOnly(<ABTestConfigForm config={{ variants }} onChange={vi.fn()} readOnly />)
    const remove = container.querySelectorAll('button.ant-btn-dangerous')
    expect(remove.length).toBe(3)
    remove.forEach((button) => expect(button).toBeDisabled())
  })

  it('lets a variant be removed off read-only', () => {
    const variants = ['A', 'B', 'C'].map((id) => ({ id, name: `Variant ${id}`, weight: id === 'A' ? 34 : 33, next_node_id: '' }))
    const { container } = render(
      <I18nProvider i18n={i18n}>
        <ABTestConfigForm config={{ variants }} onChange={vi.fn()} />
      </I18nProvider>
    )
    container.querySelectorAll('button.ant-btn-dangerous').forEach((button) => expect(button).toBeEnabled())
  })
})
