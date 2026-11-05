import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { TrackingConsentEditor, type TrackingConsentChanges } from './TrackingConsent'

function renderEditor(props: Partial<Parameters<typeof TrackingConsentEditor>[0]> = {}) {
  const onSave = props.onSave ?? vi.fn<(changes: TrackingConsentChanges) => Promise<void>>().mockResolvedValue()
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <TrackingConsentEditor
          opens="granted"
          clicks="denied"
          updatedAt="2026-11-05T17:04:00Z"
          source="notification_center"
          {...props}
          onSave={onSave}
        />
      </App>
    </I18nProvider>
  )
  return onSave
}

// Picks an option of the antd select labelled `label`.
async function choose(label: string, option: string) {
  await userEvent.click(screen.getByRole('combobox', { name: label }))
  const options = await screen.findAllByText(option, { selector: '.ant-select-item-option-content' })
  await userEvent.click(options[options.length - 1])
}

describe('TrackingConsentEditor', () => {
  it('shows both consents as values, not inputs, with the last change in a tooltip', async () => {
    renderEditor()

    expect(screen.getByText('Granted')).toBeInTheDocument()
    expect(screen.getByText('Denied')).toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(screen.queryByText(/Last changed/)).not.toBeInTheDocument()

    await userEvent.hover(screen.getByTestId('tracking-consent-changed'))
    expect(await screen.findByText(/Last changed .* from Notification center/)).toBeInTheDocument()
  })

  it('has no change icon for a contact whose consent was never recorded', () => {
    renderEditor({ opens: null, clicks: null, updatedAt: undefined, source: undefined })

    expect(screen.getAllByText('Unknown')).toHaveLength(2)
    expect(screen.queryByTestId('tracking-consent-changed')).not.toBeInTheDocument()
  })

  it('saves only the consents that changed, in one write, unknown as null', async () => {
    const onSave = renderEditor()

    await userEvent.click(screen.getByRole('button', { name: 'Edit tracking consent' }))
    await choose('Opens', 'Unknown')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    expect(onSave).toHaveBeenCalledWith({ tracking_opens_consent: null })
    await waitFor(() => expect(screen.queryByRole('combobox')).not.toBeInTheDocument())
  })

  it('writes nothing when no consent changed, or on cancel', async () => {
    const onSave = renderEditor()

    await userEvent.click(screen.getByRole('button', { name: 'Edit tracking consent' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Edit tracking consent' }))
    await choose('Clicks', 'Granted')
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText('Denied')).toBeInTheDocument()
  })

  it('stays in edit mode and says why when the write fails', async () => {
    const onSave = vi.fn<(changes: TrackingConsentChanges) => Promise<void>>().mockRejectedValue(new Error('boom'))
    renderEditor({ onSave })

    await userEvent.click(screen.getByRole('button', { name: 'Edit tracking consent' }))
    await choose('Clicks', 'Granted')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('boom')).toBeInTheDocument()
    const clicks = screen.getByRole('combobox', { name: 'Clicks' }).closest('.ant-select') as HTMLElement
    expect(within(clicks).getByText('Granted')).toBeInTheDocument()
  })

  it('cannot be edited while the contact loads', () => {
    renderEditor({ disabled: true })

    expect(screen.getByRole('button', { name: 'Edit tracking consent' })).toBeDisabled()
  })
})
