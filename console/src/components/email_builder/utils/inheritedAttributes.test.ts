import { describe, it, expect } from 'vitest'
import { inheritedAttributes } from './inheritedAttributes'
import { EmailBlockClass } from '../EmailBlockClass'
import { mergeWithDefaults } from '../mjml-defaults'
import type { EmailBlock } from '../types'

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
          children: [
            { id: 'def-text', type: 'mj-text', attributes: { color: '#ff0000' } },
            { id: 'def-social-el', type: 'mj-social-element', attributes: { iconSize: '30px' } }
          ]
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
          children: [
            {
              id: 'col',
              type: 'mj-column',
              children: [
                { id: 'text', type: 'mj-text', attributes: {} },
                {
                  id: 'social',
                  type: 'mj-social',
                  attributes: { iconSize: '24px', color: '#00ff00', innerPadding: '6px' },
                  children: [{ id: 'el', type: 'mj-social-element', attributes: { name: 'x' } }]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
} as unknown as EmailBlock

const block = (id: string) => EmailBlockClass.findBlockById(tree, id)!
const defaults = () => EmailBlockClass.extractAttributeDefaults(tree)

describe('inheritedAttributes', () => {
  it("reads the type's mj-attributes over the editor defaults", () => {
    expect(inheritedAttributes(tree, block('text'), defaults()).color).toBe('#ff0000')
  })

  it('gives a social element what its mj-social passes, under the mj-social-element defaults', () => {
    const inherited = inheritedAttributes(tree, block('el'), defaults())
    expect(inherited.color).toBe('#00ff00')
    expect(inherited.padding).toBe('6px')
    // mj-attributes of mj-social-element win over what the parent passes
    expect(inherited.iconSize).toBe('30px')
  })

  it('gives a block inside mj-attributes the MJML defaults only, not itself', () => {
    const inherited = inheritedAttributes(tree, block('def-text'), defaults())
    expect(inherited.color).toBe((mergeWithDefaults('mj-text', {}) as Record<string, unknown>).color)
    expect(inherited.color).not.toBe('#ff0000')
  })
})
