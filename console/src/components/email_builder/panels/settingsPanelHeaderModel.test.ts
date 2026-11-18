import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { blockPath, headerActions, headerTone } from './settingsPanelHeaderModel'

const b = (id: string, type: string, children?: EmailBlock[]) =>
  ({ id, type, attributes: {}, ...(children ? { children } : {}) }) as unknown as EmailBlock

const tree = b('root', 'mjml', [
  b('head', 'mj-head', [b('attrs', 'mj-attributes'), b('font', 'mj-font'), b('css', 'mj-style')]),
  b('body', 'mj-body', [b('section', 'mj-section', [b('column', 'mj-column', [b('button', 'mj-button'), b('raw', 'mj-raw')])])])
])

const types = (path: EmailBlock[]) => path.map((block) => block.type)
const pathOf = (id: string) => blockPath(tree, id)

describe('blockPath', () => {
  it('goes from the body down to the block, without the email root', () => {
    expect(types(pathOf('button'))).toEqual(['mj-body', 'mj-section', 'mj-column', 'mj-button'])
    expect(types(pathOf('font'))).toEqual(['mj-head', 'mj-font'])
    expect(types(pathOf('body'))).toEqual(['mj-body'])
  })

  it('is empty for the root and for a block that is not in the email', () => {
    expect(pathOf('root')).toEqual([])
    expect(pathOf('missing')).toEqual([])
  })
})

describe('headerActions', () => {
  it('offers clone, save and delete for a block of the body', () => {
    expect(headerActions(pathOf('button'))).toEqual({ clone: true, save: true, remove: true })
    expect(headerActions(pathOf('section'))).toEqual({ clone: true, save: true, remove: true })
  })

  it('offers delete only for a head block, and nothing for the default attributes', () => {
    expect(headerActions(pathOf('font'))).toEqual({ clone: false, save: false, remove: true })
    expect(headerActions(pathOf('css'))).toEqual({ clone: false, save: false, remove: true })
    expect(headerActions(pathOf('attrs'))).toEqual({ clone: false, save: false, remove: false })
  })

  it('offers nothing for the body, the head or the email', () => {
    const none = { clone: false, save: false, remove: false }
    expect(headerActions(pathOf('body'))).toEqual(none)
    expect(headerActions(pathOf('head'))).toEqual(none)
    expect(headerActions(pathOf('root'))).toEqual(none)
  })
})

describe('headerTone', () => {
  it('tells content, structure, head and code blocks apart', () => {
    expect(headerTone(pathOf('button'))).toBe('content')
    expect(headerTone(pathOf('column'))).toBe('structure')
    expect(headerTone(pathOf('body'))).toBe('structure')
    expect(headerTone(pathOf('font'))).toBe('head')
    expect(headerTone(pathOf('head'))).toBe('head')
    expect(headerTone(pathOf('raw'))).toBe('code')
  })
})
