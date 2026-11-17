import { describe, it, expect } from 'vitest'
import {
  getStyleProperty,
  mergeStyles,
  normalizeStyle,
  omitStyleProperties,
  pickStyleProperties,
  setStyleProperty
} from './inlineStyle'

describe('getStyleProperty', () => {
  it('reads a value as written', () => {
    expect(getStyleProperty('color: #31C48D; font-size: 20px', 'color')).toBe('#31C48D')
    expect(getStyleProperty('COLOR:red', 'color')).toBe('red')
  })

  it('reads the last declaration, as CSS applies it', () => {
    expect(getStyleProperty('color: red; color: blue', 'color')).toBe('blue')
  })

  it('reads a Liquid value whole, semicolons and colons included', () => {
    expect(getStyleProperty('color: {{ a | default: "#fff;" }}; margin: 0', 'color')).toBe(
      '{{ a | default: "#fff;" }}'
    )
  })

  it('does not mistake a longer property for the one asked', () => {
    expect(getStyleProperty('background-color: red', 'color')).toBe('')
  })

  it('reads nothing from an empty or missing style', () => {
    expect(getStyleProperty(null, 'color')).toBe('')
    expect(getStyleProperty('', 'color')).toBe('')
  })
})

describe('setStyleProperty', () => {
  it('replaces the property in place and leaves the rest as written', () => {
    expect(setStyleProperty('font-size:20px;color:red;margin:0', 'color', '#00f')).toBe(
      'font-size:20px; color: #00f; margin:0'
    )
  })

  it('appends a property the style lacks', () => {
    expect(setStyleProperty('font-size: 20px', 'color', 'red')).toBe('font-size: 20px; color: red')
    expect(setStyleProperty(null, 'color', 'red')).toBe('color: red')
  })

  it('removes every declaration of the property when the value is empty', () => {
    expect(setStyleProperty('color: red; margin: 0; color: blue', 'color', '')).toBe('margin: 0')
    expect(setStyleProperty('color: red', 'color', null)).toBeNull()
  })

  it('keeps a semicolon inside url() and quotes', () => {
    const style = 'background-image: url("data:image/png;base64,AA"); font-family: "A;B"'
    expect(setStyleProperty(style, 'color', 'red')).toBe(`${style}; color: red`)
  })

  it('never cuts a style holding a Liquid tag: it appends, and removes nothing', () => {
    const style = '{% if vip %}color: gold; margin: 0{% endif %}'
    expect(setStyleProperty(style, 'color', 'red')).toBe(`${style}; color: red`)
    expect(setStyleProperty(style, 'color', null)).toBe(style)
    expect(getStyleProperty(style, 'color')).toBe('')
    expect(pickStyleProperties(style, ['color'])).toBeNull()
    expect(omitStyleProperties(style, ['color'])).toBe(style)
    expect(mergeStyles('font-size: 2px', style)).toBe(`font-size: 2px; ${style}`)
  })
})

describe('styles with Liquid blocks and declarations around them', () => {
  it('edits the declarations outside a Liquid block and keeps the block whole', () => {
    const block = '{% if vip %}font-size: 20px; color: gold{% endif %}'
    const first = setStyleProperty(block, 'color', '#f00')
    expect(first).toBe(`${block}; color: #f00`)
    expect(getStyleProperty(first, 'color')).toBe('#f00')
    expect(setStyleProperty(first, 'color', '#00f')).toBe(`${block}; color: #00f`)
    expect(setStyleProperty(first, 'color', null)).toBe(block)
    expect(getStyleProperty(`padding: 1px; ${block}`, 'padding')).toBe('1px')
  })

  it('keeps nested Liquid blocks whole', () => {
    const block = '{% if a %}{% unless b %}color: red; margin: 0{% endunless %}; padding: 0{% endif %}'
    expect(setStyleProperty(`${block}; margin: 4px`, 'margin', '8px')).toBe(`${block}; margin: 8px`)
  })
})

describe('normalizeStyle', () => {
  it('drops empty declarations and returns null for nothing', () => {
    expect(normalizeStyle(' color: red ;; ')).toBe('color: red')
    expect(normalizeStyle('  ')).toBeNull()
    expect(normalizeStyle(null)).toBeNull()
  })
})

describe('pickStyleProperties', () => {
  it('keeps only the properties asked, in their order', () => {
    expect(pickStyleProperties('margin: 0; color: red; font-size: 2px', ['font-size', 'color'])).toBe(
      'color: red; font-size: 2px'
    )
    expect(pickStyleProperties('margin: 0', ['color'])).toBeNull()
  })
})

describe('mergeStyles', () => {
  it('lets the second style win property by property', () => {
    expect(mergeStyles('font-size: 24px; color: black', 'color: red; margin: 0')).toBe(
      'font-size: 24px; color: red; margin: 0'
    )
  })

  it('returns either side when the other is empty', () => {
    expect(mergeStyles(null, 'color: red')).toBe('color: red')
    expect(mergeStyles('color: red', null)).toBe('color: red')
    expect(mergeStyles(null, null)).toBeNull()
  })
})
