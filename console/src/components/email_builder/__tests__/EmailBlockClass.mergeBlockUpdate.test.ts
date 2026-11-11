import { describe, expect, it } from 'vitest'
import { EmailBlockClass } from '../EmailBlockClass'
import type { DisplayCondition, EmailBlock } from '../types'

const condition: DisplayCondition = {
  match: 'all',
  rules: [{ variable: 'contact.first_name', operator: 'is_present' }]
}

const tree = (withCondition: boolean): EmailBlock =>
  ({
    id: 'mjml-1',
    type: 'mjml',
    children: [
      {
        id: 'body-1',
        type: 'mj-body',
        children: [
          {
            id: 'text-1',
            type: 'mj-text',
            content: '<p>Hi</p>',
            attributes: { color: '#000000', align: 'left' },
            ...(withCondition ? { display_condition: condition } : {})
          }
        ]
      }
    ]
  }) as unknown as EmailBlock

const text = (root: EmailBlock) => EmailBlockClass.findBlockById(root, 'text-1') as EmailBlock

describe('EmailBlockClass.mergeBlockUpdate', () => {
  it('keeps the display condition when the update does not mention it', () => {
    const merged = EmailBlockClass.mergeBlockUpdate(tree(true), 'text-1', {
      attributes: { color: '#ff0000' },
      content: '<p>Hello</p>'
    })
    expect(text(merged).display_condition).toEqual(condition)
    expect(text(merged).content).toBe('<p>Hello</p>')
    expect(text(merged).attributes).toEqual({ color: '#ff0000', align: 'left' })
  })

  it('sets a display condition', () => {
    const merged = EmailBlockClass.mergeBlockUpdate(tree(false), 'text-1', {
      display_condition: condition
    })
    expect(text(merged).display_condition).toEqual(condition)
  })

  it('removes the display condition on null', () => {
    const merged = EmailBlockClass.mergeBlockUpdate(tree(true), 'text-1', {
      display_condition: null
    })
    expect(text(merged)).not.toHaveProperty('display_condition')
  })

  it('leaves the original tree untouched', () => {
    const original = tree(true)
    EmailBlockClass.mergeBlockUpdate(original, 'text-1', { display_condition: null })
    expect(text(original).display_condition).toEqual(condition)
  })

  it('returns the tree itself when the block is not found', () => {
    const original = tree(false)
    expect(EmailBlockClass.mergeBlockUpdate(original, 'nope', { content: 'x' })).toBe(original)
  })
})
