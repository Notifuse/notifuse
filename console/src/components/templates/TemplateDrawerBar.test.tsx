import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TemplateBarActions, TemplateBarTitle, TemplateStepSwitch } from './TemplateDrawerBar'

describe('TemplateBarTitle', () => {
  it('says a template not created yet is new, with no version or save time', () => {
    render(<TemplateBarTitle name="Welcome" dirty={false} />)
    expect(screen.getByTestId('template-bar-name')).toHaveTextContent('Welcome')
    expect(screen.getByText('New')).toBeInTheDocument()
    expect(screen.queryByText(/^v\d/)).toBeNull()
    expect(screen.queryByText(/^Saved/)).toBeNull()
  })

  it('names an untitled new template', () => {
    render(<TemplateBarTitle name="  " dirty={false} />)
    expect(screen.getByTestId('template-bar-name')).toHaveTextContent('New template')
  })

  it('shows the version and when it was saved, and changes waiting only once there are some', () => {
    const updatedAt = new Date(Date.now() - 2 * 3600 * 1000).toISOString()
    const { rerender } = render(<TemplateBarTitle name="Welcome" version={3} updatedAt={updatedAt} dirty={false} />)
    expect(screen.getByText('v3')).toBeInTheDocument()
    expect(screen.getByText(/^Saved .*2 hours ago/)).toBeInTheDocument()
    expect(screen.queryByText('Unsaved changes')).toBeNull()
    expect(screen.queryByText('New')).toBeNull()

    rerender(<TemplateBarTitle name="Welcome" version={3} updatedAt={updatedAt} dirty />)
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
  })
})

describe('TemplateStepSwitch', () => {
  it('opens any step, and lists Translations only with other languages', async () => {
    const onChange = vi.fn()
    const { rerender } = render(
      <TemplateStepSwitch step="settings" onChange={onChange} showTranslations={false} translationCount={0} stepsWithErrors={[]} />
    )
    expect(screen.queryByText('Translations')).toBeNull()
    await userEvent.click(screen.getByText('Content'))
    expect(onChange).toHaveBeenCalledWith('template')

    rerender(
      <TemplateStepSwitch step="settings" onChange={onChange} showTranslations translationCount={2} stepsWithErrors={[]} />
    )
    expect(screen.getByText('Translations').closest('[data-step]')).toHaveTextContent('Translations2')
  })

  it('marks a step holding an error with a dot and words a screen reader reads', () => {
    render(
      <TemplateStepSwitch step="template" onChange={vi.fn()} showTranslations translationCount={0} stepsWithErrors={['settings']} />
    )
    const settings = document.querySelector('[data-step="settings"]')!
    expect(settings.querySelector('[data-step-error]')).not.toBeNull()
    expect(settings).toHaveTextContent('(has errors)')
    expect(document.querySelector('[data-step="template"] [data-step-error]')).toBeNull()
  })
})

describe('TemplateBarActions', () => {
  it('creates a new template, and saves the next version of an existing one', () => {
    const { rerender } = render(<TemplateBarActions saving={false} onSave={vi.fn()} onSendTest={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Create template/ })).toBeInTheDocument()
    rerender(<TemplateBarActions version={3} saving={false} onSave={vi.fn()} onSendTest={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Save v4/ })).toBeInTheDocument()
  })

  it('disables the test send with its reason, and sends otherwise', async () => {
    const onSendTest = vi.fn()
    const { rerender } = render(
      <TemplateBarActions saving={false} onSave={vi.fn()} onSendTest={onSendTest} testDisabledReason="Create it first" />
    )
    expect(screen.getByRole('button', { name: /Send test/ })).toBeDisabled()

    rerender(<TemplateBarActions version={1} saving={false} onSave={vi.fn()} onSendTest={onSendTest} />)
    await userEvent.click(screen.getByRole('button', { name: /Send test/ }))
    expect(onSendTest).toHaveBeenCalledTimes(1)
  })
})
