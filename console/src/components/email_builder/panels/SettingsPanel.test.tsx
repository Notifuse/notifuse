import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SettingsPanel } from './SettingsPanel'
import type { EmailBlock } from '../types'

const vip = {
  match: 'all' as const,
  rules: [{ variable: 'contact.vip', operator: 'is_true' as const }]
}

const tree = {
  id: 'root',
  type: 'mjml',
  children: [
    {
      id: 'head',
      type: 'mj-head',
      children: [
        {
          id: 'attrs',
          type: 'mj-attributes',
          children: [{ id: 'defaults-text', type: 'mj-text', attributes: {} }]
        }
      ]
    },
    {
      id: 'body',
      type: 'mj-body',
      children: [
        {
          id: 'sec',
          type: 'mj-section',
          attributes: {},
          children: [
            {
              id: 'col',
              type: 'mj-column',
              attributes: {},
              children: [
                {
                  id: 'spacer-1',
                  type: 'mj-spacer',
                  attributes: { height: '20px' },
                  display_condition: vip
                },
                { id: 'spacer-2', type: 'mj-spacer', attributes: { height: '30px' } },
                { id: 'liquid', type: 'mj-liquid', content: '{% if x %}' }
              ]
            }
          ]
        }
      ]
    }
  ]
} as unknown as EmailBlock

const find = (id: string, block: EmailBlock = tree): EmailBlock | null => {
  if (block.id === id) return block
  for (const child of (block.children as EmailBlock[] | undefined) ?? []) {
    const found = find(id, child)
    if (found) return found
  }
  return null
}

const renderPanel = (id: string, onUpdateBlock = vi.fn()) =>
  render(
    <SettingsPanel
      key={id}
      selectedBlock={find(id)}
      onUpdateBlock={onUpdateBlock}
      attributeDefaults={{}}
      emailTree={tree}
      testData={{ contact: { vip: true } }}
    />
  )

describe('SettingsPanel display condition', () => {
  it('shows the summary for eligible blocks only', () => {
    for (const id of ['spacer-1', 'sec']) {
      const { unmount } = renderPanel(id)
      expect(screen.queryByTestId('display-condition-summary'), id).not.toBeNull()
      unmount()
    }
    for (const id of ['col', 'liquid', 'defaults-text']) {
      const { unmount } = renderPanel(id)
      expect(screen.queryByTestId('display-condition-summary'), id).toBeNull()
      unmount()
    }
  })

  it("shows the selected block's own condition", () => {
    const { unmount } = renderPanel('spacer-1')
    expect(screen.getByText('contact.vip')).toBeTruthy()
    unmount()
    renderPanel('spacer-2')
    expect(screen.getByText('Always shown')).toBeTruthy()
  })

  it('removes the condition through onUpdateBlock, keeping the attributes', async () => {
    const onUpdateBlock = vi.fn()
    renderPanel('spacer-1', onUpdateBlock)
    await userEvent.click(screen.getByRole('button', { name: 'Remove the condition' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Remove' }))
    expect(onUpdateBlock).toHaveBeenCalledTimes(1)
    const [id, updated] = onUpdateBlock.mock.calls[0]
    expect(id).toBe('spacer-1')
    expect(updated.display_condition).toBeUndefined()
    expect(updated.attributes).toEqual({ height: '20px' })
    // the tree itself is untouched
    expect(find('spacer-1')?.display_condition).toEqual(vip)
  })
})
