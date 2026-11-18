import { describe, it, expect } from 'vitest'
import { paddingShorthand } from './MjSocialElementBlock'
import { EmailBlockClass } from '../EmailBlockClass'
import { SOCIAL_INHERITED_ATTRIBUTES } from '../mjml-defaults'

describe('paddingShorthand', () => {
  it.each([
    ['4px', {}, '4px 4px 4px 4px'],
    ['4px 8px', { top: '10px' }, '10px 8px 4px 8px'],
    ['1px 2px 3px', { left: '9px' }, '1px 2px 3px 9px'],
    ['1px 2px 3px 4px', { right: '0px', bottom: '7px' }, '1px 0px 7px 4px']
  ])('%s with %o is %s', (shorthand, sides, expected) => {
    expect(paddingShorthand(shorthand, sides)).toBe(expected)
  })
})

describe('a new social element', () => {
  it('writes none of the attributes its mj-social passes, so that what the mj-social sets reaches it', () => {
    const attributes = EmailBlockClass.createBlock('mj-social-element').attributes as Record<string, unknown>
    for (const key of [...SOCIAL_INHERITED_ATTRIBUTES, 'padding']) {
      expect(attributes[key], key).toBeUndefined()
    }
  })

  it('keeps the default icons of a new social block free of them too', () => {
    const socialBlock = EmailBlockClass.createBlock('mj-social')
    for (const child of socialBlock.children ?? []) {
      expect((child.attributes as Record<string, unknown>).borderRadius).toBeUndefined()
    }
  })
})

describe('a new social block', () => {
  it('writes no icon height, so that its icons follow their size as in MJML', () => {
    expect((EmailBlockClass.createBlock('mj-social').attributes as Record<string, unknown>).iconHeight).toBeUndefined()
  })
})
