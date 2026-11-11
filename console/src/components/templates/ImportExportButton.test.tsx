import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import { ImportExportButton } from './ImportExportButton'
import type { EmailBlock } from '../email_builder/types'

vi.mock('../../services/api/template', () => ({ templatesApi: {} }))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const tree: EmailBlock = { id: 'mjml-1', type: 'mjml', children: [] } as EmailBlock

const mjml = (between: string) => `<mjml>
  <mj-body>
    ${between}
    <mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section>
    ${between ? '{% endif %}' : ''}
  </mj-body>
</mjml>`

function importMjml(content: string) {
  const onImport = vi.fn()
  const { container } = render(
    <I18nProvider i18n={i18n}>
      <App>
        <ImportExportButton onImport={onImport} tree={tree} workspaceId="ws1" />
      </App>
    </I18nProvider>
  )
  const input = container.querySelector('input[accept^=".mjml"]') as HTMLInputElement
  fireEvent.change(input, {
    target: { files: [new File([content], 'email.mjml', { type: 'text/plain' })] }
  })
  return onImport
}

// An export of a template with conditions round-trips to the visual editor without them,
// which would send the blocks to everyone: the import has to say so.
describe('ImportExportButton MJML import', () => {
  it('warns that display conditions are not imported', async () => {
    const onImport = importMjml(
      mjml('{% assign __dc_s1 = true %}{% if contact.vip == true %}{% endif %}{% if __dc_s1 %}')
    )
    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1))
    expect((await screen.findAllByText('Display conditions not imported')).length).toBeGreaterThan(
      0
    )
  })

  it('shows no warning for MJML without display conditions', async () => {
    const onImport = importMjml(mjml(''))
    await waitFor(() => expect(onImport).toHaveBeenCalledTimes(1))
    expect(screen.queryByText('Display conditions not imported')).not.toBeInTheDocument()
  })

  // A tree from a JSON file reaches the editor as is: a malformed or misplaced
  // condition would break it, or be refused only on save.
  it('refuses a JSON tree with a condition the server would refuse', async () => {
    const onImport = vi.fn()
    const { container } = render(
      <I18nProvider i18n={i18n}>
        <App>
          <ImportExportButton onImport={onImport} tree={tree} workspaceId="ws1" />
        </App>
      </I18nProvider>
    )
    const bad = {
      id: 'root',
      type: 'mjml',
      children: [
        {
          id: 'body',
          type: 'mj-body',
          children: [
            {
              id: 'sec',
              type: 'mj-section',
              children: [
                {
                  id: 'col-1',
                  type: 'mj-column',
                  display_condition: {
                    match: 'all',
                    rules: [{ variable: 'a', operator: 'is_true' }]
                  },
                  children: []
                }
              ]
            }
          ]
        }
      ]
    }
    const input = container.querySelector('input[accept^=".json"]') as HTMLInputElement
    fireEvent.change(input, {
      target: {
        files: [new File([JSON.stringify(bad)], 'email.json', { type: 'application/json' })]
      }
    })
    expect(
      await screen.findByText(/col-1 \(mj-column\) cannot have a display condition/)
    ).toBeInTheDocument()
    expect(onImport).not.toHaveBeenCalled()
  })
})
