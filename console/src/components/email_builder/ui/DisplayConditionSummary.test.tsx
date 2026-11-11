import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DisplayConditionSummary } from './DisplayConditionSummary'
import type { EmailBlock } from '../types'

const block = (withCondition: boolean) =>
  ({
    id: 'sec-1',
    type: 'mj-section',
    children: [],
    ...(withCondition
      ? {
          display_condition: {
            match: 'all',
            rules: [
              { variable: 'contact.custom_string_1', operator: 'eq', value: 'vip' },
              { variable: 'order.total', operator: 'gte', value: '100' }
            ]
          }
        }
      : {})
  }) as unknown as EmailBlock

describe('DisplayConditionSummary', () => {
  it('offers to add a condition when there is none', async () => {
    render(<DisplayConditionSummary block={block(false)} onChange={vi.fn()} />)
    expect(screen.getByText('Always shown')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: /Add a condition/ }))
    expect(await screen.findByText(/of the following conditions match/)).toBeTruthy()
  })

  it('reads the rules and their result with the test data', () => {
    const { rerender } = render(
      <DisplayConditionSummary
        block={block(true)}
        testData={{ contact: { custom_string_1: 'vip' }, order: { total: 150 } }}
        onChange={vi.fn()}
      />
    )
    expect(screen.getByText('contact.custom_string_1')).toBeTruthy()
    expect(screen.getByText('equals')).toBeTruthy()
    expect(screen.getByText('vip')).toBeTruthy()
    expect(screen.getByText('and')).toBeTruthy()
    expect(screen.getByText('shown')).toBeTruthy()

    rerender(
      <DisplayConditionSummary
        block={block(true)}
        testData={{ contact: { custom_string_1: 'vip' }, order: { total: 50 } }}
        onChange={vi.fn()}
      />
    )
    expect(screen.getByText('hidden')).toBeTruthy()
  })

  it('removes the condition after confirmation', async () => {
    const onChange = vi.fn()
    render(<DisplayConditionSummary block={block(true)} onChange={onChange} />)
    await userEvent.click(screen.getByRole('button', { name: 'Remove the condition' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Remove' }))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('opens the modal on Edit', async () => {
    render(<DisplayConditionSummary block={block(true)} onChange={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Edit' }))
    expect(await screen.findByRole('button', { name: 'Apply' })).toBeTruthy()
  })

  // The server refuses a condition on a block holding an mj-liquid: hiding the block
  // would drop half of a Liquid tag pair.
  it('does not offer a condition on a block holding a Liquid block', () => {
    const section = {
      id: 'sec-2',
      type: 'mj-section',
      children: [
        {
          id: 'col',
          type: 'mj-column',
          children: [{ id: 'liq', type: 'mj-liquid', content: '{% endif %}' }]
        }
      ]
    } as unknown as EmailBlock
    render(<DisplayConditionSummary block={section} onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Add a condition/ })).toBeDisabled()
  })

  it('says why an existing condition on such a block is refused', () => {
    const section = {
      ...block(true),
      children: [
        { id: 'col', type: 'mj-column', children: [{ id: 'liq', type: 'mj-liquid', content: '' }] }
      ]
    } as unknown as EmailBlock
    render(<DisplayConditionSummary block={section} onChange={vi.fn()} />)
    expect(screen.getByTestId('display-condition-liquid')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Remove the condition' })).toBeEnabled()
  })
})
