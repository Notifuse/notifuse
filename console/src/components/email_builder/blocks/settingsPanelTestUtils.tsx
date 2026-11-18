// Helpers for the block settings panel tests.
import { vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { App } from 'antd'
import type { EmailBlock, MJMLComponentType } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { EmailBlockFactory } from './EmailBlockFactory'
import { inheritedAttributes } from '../utils/inheritedAttributes'

// A minimal email holding one block, with optional mj-attributes for its type.
export const emailWith = (block: EmailBlock, defaults: Record<string, unknown> = {}): EmailBlock =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      {
        id: 'head',
        type: 'mj-head',
        children: [
          { id: 'attrs', type: 'mj-attributes', children: [{ id: 'defaults', type: block.type, attributes: defaults }] }
        ]
      },
      {
        id: 'body',
        type: 'mj-body',
        attributes: {},
        children: [
          {
            id: 'section',
            type: 'mj-section',
            attributes: {},
            children: [{ id: 'column', type: 'mj-column', attributes: {}, children: [block] }]
          }
        ]
      }
    ]
  }) as unknown as EmailBlock

export const renderPanel = (block: EmailBlock, defaults: Record<string, unknown> = {}) => {
  const tree = emailWith(block, defaults)
  const onUpdate = vi.fn()
  const inherited = inheritedAttributes(tree, block, EmailBlockClass.extractAttributeDefaults(tree))
  const panel = EmailBlockFactory.createBlock(block).renderSettingsPanel(onUpdate, inherited as never, tree)
  render(<App>{panel}</App>)
  return { onUpdate }
}

export const fresh = (type: MJMLComponentType) => EmailBlockClass.createBlock(type, `${type}-1`, 'Content')

// Opens a section unless it is open already
export const openSection = (name: RegExp) => {
  const header = screen.getByRole('button', { name })
  if (header.getAttribute('aria-expanded') === 'false') fireEvent.click(header)
}

export const blur = (label: string, text: string) => {
  const input = screen.getByLabelText(label)
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value: text } })
  fireEvent.blur(input)
}

// The rows marked « set » (their value differs from the inherited one)
export const setRows = () => screen.queryAllByTestId('settings-row').filter((row) => row.dataset.set === 'true')
