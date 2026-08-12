import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SettingsSaveBar } from './SettingsSaveBar'

// The bar guards navigation away from unsaved edits; the suite renders it
// outside a router, so the blocker is stubbed. shouldBlockFn is captured so the
// guard's own condition can be asserted.
const shouldBlock = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useBlocker: (opts: { shouldBlockFn: () => boolean }) => {
    shouldBlock.mockImplementation(opts.shouldBlockFn)
    return { status: 'idle', proceed: undefined, reset: undefined }
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderBar = (props: Partial<Parameters<typeof SettingsSaveBar>[0]> = {}) => {
  const onSave = vi.fn()
  const onDiscard = vi.fn()
  const result = render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <SettingsSaveBar
            dirty
            saving={false}
            onSave={onSave}
            onDiscard={onDiscard}
            leaveWarning="Leaving discards them."
            {...props}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  return { ...result, onSave, onDiscard }
}

describe('SettingsSaveBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stays out of the way while the form is pristine', () => {
    renderBar({ dirty: false })
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
  })

  it('offers Save and Discard once there are edits', () => {
    const { onSave, onDiscard } = renderBar()

    expect(screen.getByText('You have unsaved changes')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))
    expect(onSave).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))
    expect(onDiscard).toHaveBeenCalledTimes(1)
  })

  it('stays up through the save with Discard locked out', () => {
    renderBar({ saving: true })
    // The bar owns the spinner, so hiding it mid-request would drop the only
    // feedback that the save is running.
    expect(screen.getByText('You have unsaved changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Discard$/i })).toBeDisabled()
  })

  it('saves on Cmd/Ctrl+S only while there is something to save', () => {
    const { onSave, rerender } = renderBar({ dirty: false })

    fireEvent.keyDown(window, { key: 's', metaKey: true })
    expect(onSave).not.toHaveBeenCalled()

    rerender(
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <SettingsSaveBar
              dirty
              saving={false}
              onSave={onSave}
              onDiscard={vi.fn()}
              leaveWarning="Leaving discards them."
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    )

    fireEvent.keyDown(window, { key: 's', ctrlKey: true })
    expect(onSave).toHaveBeenCalledTimes(1)
  })

  it('releases the shortcut and the leave guard while the save is in flight', () => {
    // A second Cmd+S mid-request would submit the form twice.
    const { onSave } = renderBar({ saving: true })

    fireEvent.keyDown(window, { key: 's', metaKey: true })

    expect(onSave).not.toHaveBeenCalled()
    expect(shouldBlock()).toBe(false)
  })

  it('blocks navigation while edits are pending', () => {
    renderBar()
    expect(shouldBlock()).toBe(true)
  })
})
