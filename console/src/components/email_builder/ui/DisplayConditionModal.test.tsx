import { describe, it, expect, vi } from 'vitest'
import { render, screen, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DisplayConditionModal } from './DisplayConditionModal'
import type { DisplayCondition, EmailBlock } from '../types'

const image = { id: 'img-1', type: 'mj-image', attributes: {} } as unknown as EmailBlock
const testData = { order: { has_image: true, total: 120 } }

const renderModal = (initial?: DisplayCondition) => {
  const onApply = vi.fn()
  const onCancel = vi.fn()
  render(
    <DisplayConditionModal
      block={image}
      initial={initial}
      testData={testData}
      onApply={onApply}
      onCancel={onCancel}
    />
  )
  return { onApply, onCancel }
}

const apply = () => screen.getByRole('button', { name: 'Apply' })
const confirm = () => screen.getByRole('button', { name: 'Confirm' })

// Picks an option of the antd Select labelled label
async function choose(label: string, option: string) {
  await userEvent.click(screen.getByLabelText(label))
  const options = await screen.findAllByTitle(option)
  await userEvent.click(options[options.length - 1])
}

async function addRule(variable: string) {
  await userEvent.click(screen.getByRole('button', { name: /Condition/ }))
  fireEvent.change(screen.getByLabelText('Variable'), { target: { value: variable } })
}

describe('DisplayConditionModal', () => {
  it('adds a rule and applies the condition', async () => {
    const { onApply } = renderModal()
    expect(apply()).toBeDisabled()
    await addRule('order.has_image')
    expect(apply()).toBeDisabled() // while the rule is being edited
    await userEvent.click(confirm())
    await userEvent.click(apply())
    expect(onApply).toHaveBeenCalledWith({
      match: 'all',
      rules: [{ variable: 'order.has_image', operator: 'is_true' }]
    })
  })

  it('drops the value when the operator takes none', async () => {
    const { onApply } = renderModal({
      match: 'any',
      rules: [{ variable: 'order.total', operator: 'gt', value: '100' }]
    })
    await userEvent.click(screen.getByRole('button', { name: 'Edit rule' }))
    await choose('Operator', 'is set')
    expect(screen.queryByLabelText('Value')).toBeNull()
    await userEvent.click(confirm())
    await userEvent.click(apply())
    expect(onApply).toHaveBeenCalledWith({
      match: 'any',
      rules: [{ variable: 'order.total', operator: 'is_present' }]
    })
  })

  it('cannot confirm an order operator without a number', async () => {
    renderModal({
      match: 'all',
      rules: [{ variable: 'order.total', operator: 'gt', value: '100' }]
    })
    await userEvent.click(screen.getByRole('button', { name: 'Edit rule' }))
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'lots' } })
    expect(confirm()).toBeDisabled()
    expect(screen.getByText(/Enter a number/)).toBeTruthy()
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: '99.5' } })
    expect(confirm()).toBeEnabled()
  })

  it('deletes a rule', async () => {
    const { onApply } = renderModal({
      match: 'all',
      rules: [
        { variable: 'order.has_image', operator: 'is_true' },
        { variable: 'order.total', operator: 'gte', value: '100' }
      ]
    })
    await userEvent.click(
      within(screen.getByTestId('condition-rule-0')).getByRole('button', { name: 'Delete rule' })
    )
    await userEvent.click(apply())
    expect(onApply).toHaveBeenCalledWith({
      match: 'all',
      rules: [{ variable: 'order.total', operator: 'gte', value: '100' }]
    })
  })

  it('cancels without applying', async () => {
    const { onApply, onCancel } = renderModal({
      match: 'all',
      rules: [{ variable: 'order.has_image', operator: 'is_true' }]
    })
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCancel).toHaveBeenCalled()
    expect(onApply).not.toHaveBeenCalled()
  })

  it('warns about a variable missing from the test data without blocking Apply', () => {
    renderModal({
      match: 'all',
      rules: [{ variable: 'order.coupon_code', operator: 'is_present' }]
    })
    expect(screen.getByText(/Not in the test data/)).toBeTruthy()
    expect(apply()).toBeEnabled()
  })

  it('shows each rule and the condition with the test data', () => {
    renderModal({
      match: 'all',
      rules: [
        { variable: 'order.has_image', operator: 'is_true' },
        { variable: 'order.total', operator: 'gt', value: '200' }
      ]
    })
    expect(within(screen.getByTestId('condition-rule-0')).getByText('true')).toBeTruthy()
    expect(within(screen.getByTestId('condition-rule-1')).getByText('false')).toBeTruthy()
    expect(screen.getByText('block hidden')).toBeTruthy()
  })
})
