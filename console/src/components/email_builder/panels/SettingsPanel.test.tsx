import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
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
      onUpdateAttributes={vi.fn()}
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

  it('lets the default attributes panel edit the head, through onUpdateBlock', () => {
    renderPanel('attrs')
    // The Headings section only shows when it can write the head
    expect(screen.getByRole('radio', { name: 'H1' })).toBeTruthy()
  })
})

describe('SettingsPanel writes and inherited values', () => {
  const withDefaults = (spacerDefaults: Record<string, unknown>) =>
    ({
      ...tree,
      children: [
        {
          id: 'head',
          type: 'mj-head',
          children: [
            { id: 'attrs', type: 'mj-attributes', children: [{ id: 'def-spacer', type: 'mj-spacer', attributes: spacerDefaults }] }
          ]
        },
        (tree.children as EmailBlock[])[1]
      ]
    }) as unknown as EmailBlock

  it('shows the empty state for the email root', () => {
    render(
      <SettingsPanel
        selectedBlock={tree}
        onUpdateBlock={vi.fn()}
        onUpdateAttributes={vi.fn()}
        attributeDefaults={{}}
        emailTree={tree}
      />
    )
    expect(screen.getByText('Select a block to edit its attributes')).toBeTruthy()
  })

  it("writes a field's keys through onUpdateAttributes, with the block's id", () => {
    const onUpdateAttributes = vi.fn()
    const spacer = { id: 'spacer-3', type: 'mj-spacer', attributes: {} } as unknown as EmailBlock
    render(
      <SettingsPanel
        selectedBlock={spacer}
        onUpdateBlock={vi.fn()}
        onUpdateAttributes={onUpdateAttributes}
        attributeDefaults={{}}
        emailTree={tree}
      />
    )
    fireEvent.change(screen.getAllByRole('spinbutton')[0], { target: { value: '40' } })
    expect(onUpdateAttributes).toHaveBeenCalled()
    const [id, updates] = onUpdateAttributes.mock.calls[0]
    expect(id).toBe('spacer-3')
    expect(updates).toEqual({ height: '40px' })
  })

  it("gives the panel the type's mj-attributes over the editor defaults as inherited values", () => {
    const emailTree = withDefaults({ height: '44px' })
    const spacer = { id: 'spacer-3', type: 'mj-spacer', attributes: {} } as unknown as EmailBlock
    render(
      <SettingsPanel
        selectedBlock={spacer}
        onUpdateBlock={vi.fn()}
        onUpdateAttributes={vi.fn()}
        attributeDefaults={{ 'mj-spacer': { height: '44px' } }}
        emailTree={emailTree}
      />
    )
    expect(screen.getAllByRole('spinbutton')[0]).toHaveAttribute('placeholder', '44')
  })
})

describe('SettingsPanel header', () => {
  it('heads every block panel, with the actions passed to it, also where no condition applies', () => {
    const onCloneBlock = vi.fn()
    const onSelectBlock = vi.fn()
    const { unmount } = render(
      <SettingsPanel
        selectedBlock={find('spacer-2')}
        onUpdateBlock={vi.fn()}
        onUpdateAttributes={vi.fn()}
        attributeDefaults={{}}
        emailTree={tree}
        onCloneBlock={onCloneBlock}
        onSelectBlock={onSelectBlock}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Clone Block' }))
    expect(onCloneBlock).toHaveBeenCalledWith('spacer-2')
    fireEvent.click(screen.getByRole('button', { name: 'Section' }))
    expect(onSelectBlock).toHaveBeenCalledWith('sec')
    unmount()

    // The default attributes take no display condition: the header is there all the same
    renderPanel('attrs')
    expect(screen.getByTestId('settings-panel-header')).toBeInTheDocument()
  })

  it('has no header on the empty state', () => {
    renderPanel('root')
    expect(screen.queryByTestId('settings-panel-header')).toBeNull()
  })
})

