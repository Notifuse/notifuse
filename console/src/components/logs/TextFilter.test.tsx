import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { TextFilter } from './TextFilter'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderFilter = (onApply: (value: string) => void, initial = '') =>
  render(
    <I18nProvider i18n={i18n}>
      <TextFilter initial={initial} placeholder="Exact value" onApply={onApply} />
    </I18nProvider>
  )

describe('TextFilter', () => {
  it('applies the trimmed text on Enter', async () => {
    const onApply = vi.fn()
    renderFilter(onApply)
    await userEvent.type(screen.getByPlaceholderText('Exact value'), '  tpl_welcome {Enter}')
    expect(onApply).toHaveBeenCalledWith('tpl_welcome')
  })

  it('applies with its button, starting from the current value', async () => {
    const onApply = vi.fn()
    renderFilter(onApply, 'tpl')
    await userEvent.type(screen.getByPlaceholderText('Exact value'), '_2')
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }))
    expect(onApply).toHaveBeenCalledWith('tpl_2')
  })
})
