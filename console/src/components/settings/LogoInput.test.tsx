import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider, Form } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { LogoInput } from './LogoInput'
import { workspaceService } from '../../services/api/workspace'

vi.mock('../../services/api/workspace', () => ({
  workspaceService: { detectFavicon: vi.fn() }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderLogo = (initial: { website_url?: string; logo_url?: string }) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <Form initialValues={initial}>
            <Form.Item name="website_url" hidden>
              <input />
            </Form.Item>
            <Form.Item name="logo_url">
              <LogoInput workspaceName="Acme" />
            </Form.Item>
          </Form>
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

const detect = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
  fireEvent.click(await screen.findByRole('button', { name: /Detect from website/ }))
}

describe('LogoInput', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows the initial of the workspace without a logo, and no Remove', () => {
    renderLogo({})

    expect(document.querySelector('[data-workspace-badge]')).toHaveTextContent('A')
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull()
  })

  it('detects the icon of the website', async () => {
    vi.mocked(workspaceService.detectFavicon).mockResolvedValue({ iconUrl: 'https://acme.io/icon.png' })
    renderLogo({ website_url: 'https://acme.io' })

    await detect()

    await waitFor(() =>
      expect(document.querySelector('[data-logo-preview]')).toHaveAttribute('src', 'https://acme.io/icon.png')
    )
    expect(workspaceService.detectFavicon).toHaveBeenCalledWith('https://acme.io')
  })

  it('asks for a website before detecting', async () => {
    renderLogo({})

    await detect()

    expect(await screen.findByText('Please enter a website URL first')).toBeInTheDocument()
    expect(workspaceService.detectFavicon).not.toHaveBeenCalled()
  })

  it('says when the website has no icon', async () => {
    vi.mocked(workspaceService.detectFavicon).mockResolvedValue({ iconUrl: '' })
    renderLogo({ website_url: 'https://acme.io' })

    await detect()

    expect(await screen.findByText('No icon found')).toBeInTheDocument()
  })
})
