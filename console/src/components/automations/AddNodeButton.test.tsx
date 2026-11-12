import { describe, it, expect, vi, afterEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ADD_NODE_MENU_ITEMS, AddNodeButton } from './AddNodeButton'
import { NODE_TYPES } from './nodeTypes'

const renderButton = (hasListSelected = true) => {
  const onSelect = vi.fn()
  render(
    <I18nProvider i18n={i18n}>
      <AddNodeButton onSelectNodeType={onSelect} hasListSelected={hasListSelected} />
    </I18nProvider>
  )
  fireEvent.click(screen.getByRole('button'))
  return onSelect
}

const menuLabels = () => Array.from(document.querySelectorAll('.ant-popover button')).map((b) => b.textContent)

afterEach(() => i18n.activate('en'))

describe('AddNodeButton', () => {
  it('lists the steps in the menu order, with the labels of the step table', () => {
    renderButton()
    expect(menuLabels()).toEqual(ADD_NODE_MENU_ITEMS.map(({ key }) => i18n._(NODE_TYPES[key].label)))
  })

  it('translates the labels', () => {
    const { id } = NODE_TYPES.delay.label
    i18n.load('fr', { [id!]: 'Attente' })
    i18n.activate('fr')
    renderButton()
    expect(menuLabels()).toContain('Attente')
  })

  it('adds the chosen step', () => {
    const onSelect = renderButton()
    fireEvent.click(screen.getByRole('button', { name: i18n._(NODE_TYPES.filter.label) }))
    expect(onSelect).toHaveBeenCalledWith('filter')
  })

  it('refuses email without a list', () => {
    const onSelect = renderButton(false)
    fireEvent.click(screen.getByRole('button', { name: i18n._(NODE_TYPES.email.label) }))
    expect(onSelect).not.toHaveBeenCalled()
  })
})
