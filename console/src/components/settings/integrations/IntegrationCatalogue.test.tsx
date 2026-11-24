import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { IntegrationCatalogue, type CatalogueSelection } from './IntegrationCatalogue'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderCatalogue = () => {
  const onSelect = vi.fn<(selection: CatalogueSelection) => void>()
  render(
    <I18nProvider i18n={i18n}>
      <IntegrationCatalogue open onClose={vi.fn()} onSelect={onSelect} />
    </I18nProvider>
  )
  return { onSelect, catalogue: screen.getByRole('dialog', { name: 'Add integration' }) }
}

describe('IntegrationCatalogue', () => {
  // The AI models and Firecrawl are added from Settings › AI › Assistant.
  it('offers the email providers and the connected apps, and no AI model nor Firecrawl', () => {
    const { catalogue } = renderCatalogue()

    const keys = Array.from(catalogue.querySelectorAll('[data-catalogue-entry]')).map((entry) =>
      entry.getAttribute('data-catalogue-entry')
    )
    expect(keys).toEqual([
      'email-ses',
      'email-smtp',
      'email-sparkpost',
      'email-postmark',
      'email-mailgun',
      'email-mailjet',
      'email-sendgrid',
      'supabase',
      'zapier'
    ])
    expect(within(catalogue).queryByText('AI assistant')).toBeNull()
    for (const name of [/^Anthropic/, /^OpenAI/, /^Google Gemini/, /^Firecrawl/]) {
      expect(within(catalogue).queryByRole('button', { name })).toBeNull()
    }
  })

  it('reports the entry picked', () => {
    const { catalogue, onSelect } = renderCatalogue()

    fireEvent.click(within(catalogue).getByRole('button', { name: /^Zapier/ }))
    expect(onSelect).toHaveBeenCalledWith({ type: 'zapier' })
  })
})
