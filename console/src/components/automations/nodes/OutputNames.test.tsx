import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeType } from '../../../services/api/automation'
import { OutputNames } from './OutputNames'
import { outputLeft } from './outputLayout'

const renderNames = (nodeType: NodeType, config: Record<string, unknown> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <OutputNames nodeType={nodeType} config={config} />
    </I18nProvider>
  )

const names = () =>
  Array.from(document.querySelectorAll<HTMLElement>('[data-output-name]')).map((el) => ({
    text: el.textContent,
    left: el.style.left,
    tone: el.className.match(/text-(success|warning|info)-fg|text-ink-3/)?.[0]
  }))

describe('OutputNames', () => {
  it('names the filter outputs above their points, Yes at 30 % and No at 70 %', () => {
    renderNames('filter')
    expect(names()).toEqual([
      { text: 'Yes', left: '30%', tone: 'text-success-fg' },
      { text: 'No', left: '70%', tone: 'text-warning-fg' }
    ])
  })

  it('names the list status outputs at 20, 50 and 80 %', () => {
    renderNames('list_status_branch')
    expect(names()).toEqual([
      { text: 'Not in list', left: '20%', tone: 'text-ink-3' },
      { text: 'Active', left: '50%', tone: 'text-success-fg' },
      { text: 'Non-active', left: '80%', tone: 'text-warning-fg' }
    ])
  })

  it('names A/B variants on one line and gives their weight on the next, spread like their points', () => {
    renderNames('ab_test', {
      variants: [
        { id: 'a', name: 'Variant A', weight: 50 },
        { id: 'b', name: 'Variant B', weight: 25 },
        { id: 'c', name: 'Variant C', weight: 25 }
      ]
    })
    const variants = Array.from(document.querySelectorAll<HTMLElement>('[data-output-name]')).map((el) => [
      el.firstElementChild?.textContent,
      el.querySelector('[data-output-weight]')?.textContent,
      el.style.left
    ])
    expect(variants).toEqual([
      ['Variant A', '50%', '20%'],
      ['Variant B', '25%', '50%'],
      ['Variant C', '25%', '80%']
    ])
    // The whole name stays reachable on hover.
    expect(document.querySelector('[data-output-name="a"]')).toHaveAttribute('title', 'Variant A (50 %)')
  })

  it('shows nothing for a step without outputs to name', () => {
    renderNames('ab_test', { variants: [] })
    expect(document.querySelector('[data-output-names]')).toBeNull()
  })

  it('places a lone output in the middle', () => {
    expect(outputLeft('branch', 0, 1)).toBe(50)
  })
})
