import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ABTestConfigForm } from './ABTestConfigForm'
import type { ABTestNodeConfig } from '../../../services/api/automation'

const renderForm = (config: Partial<ABTestNodeConfig>) => {
  const onChange = vi.fn()
  render(
    <I18nProvider i18n={i18n}>
      <ABTestConfigForm config={config as ABTestNodeConfig} onChange={onChange} />
    </I18nProvider>
  )
  return onChange
}

describe('ABTestConfigForm', () => {
  it('seeds the default variants for a node that has none', () => {
    const onChange = renderForm({})

    expect(onChange).toHaveBeenCalledTimes(1)
    const seeded = onChange.mock.calls[0][0] as ABTestNodeConfig
    expect(seeded.variants).toHaveLength(2)
    expect(seeded.variants.reduce((total, v) => total + v.weight, 0)).toBe(100)
  })

  it('keeps the rest of the config when it seeds those defaults', () => {
    // The seeding effect used to replace the whole config object rather than spread it, so a
    // node carrying a description but no variants lost the description the moment its panel
    // opened — the config bag holds more than this form's own fields.
    const onChange = renderForm({ description: 'Subject line test' })

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange.mock.calls[0][0]).toMatchObject({ description: 'Subject line test' })
  })

  it('leaves an already-configured node alone', () => {
    const onChange = renderForm({
      description: 'Subject line test',
      variants: [
        { id: 'A', name: 'Variant A', weight: 70, next_node_id: '' },
        { id: 'B', name: 'Variant B', weight: 30, next_node_id: '' }
      ]
    })

    expect(onChange).not.toHaveBeenCalled()
  })
})
