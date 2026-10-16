import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebhookConfigForm } from './WebhookConfigForm'
import type { WebhookNodeConfig } from '../../../services/api/automation'

const renderForm = (url: string) =>
  render(
    <I18nProvider i18n={i18n}>
      <WebhookConfigForm config={{ url } as WebhookNodeConfig} onChange={vi.fn()} />
    </I18nProvider>
  )

const refusal = /points at a private or internal address/

describe('WebhookConfigForm', () => {
  const original = window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS
  afterEach(() => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = original
  })

  // Where the URL is typed is where the user can fix it, including one saved before
  // the server started refusing it: the form says so as soon as the node is opened.
  it('shows an error for a URL aimed at an address the server refuses', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = false
    renderForm('http://10.0.0.5/hook')

    expect(screen.getByText(/private or internal address \(10\.0\.0\.5\)/)).toBeInTheDocument()
    expect(screen.getByText(/WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS=true/)).toBeInTheDocument()
  })

  it('says nothing on a server that delivers to its own network', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = true
    renderForm('http://10.0.0.5/hook')

    expect(screen.queryByText(refusal)).not.toBeInTheDocument()
  })

  it('says nothing for a public receiver', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = false
    renderForm('https://hooks.example.com/hook')

    expect(screen.queryByText(refusal)).not.toBeInTheDocument()
  })

  it('reports a URL without an http(s) scheme first', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = false
    renderForm('ftp://10.0.0.5/hook')

    expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument()
    expect(screen.queryByText(refusal)).not.toBeInTheDocument()
  })
})
