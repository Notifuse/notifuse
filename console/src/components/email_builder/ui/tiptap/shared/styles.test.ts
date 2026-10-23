import { describe, it, expect, afterEach } from 'vitest'
import { injectTiptapStyles } from './styles'

afterEach(() => {
  document.head.querySelector('#tiptap-shared-styles')?.remove()
  document.body.innerHTML = ''
})

describe('injectTiptapStyles', () => {
  it('adds the editor styles once', () => {
    injectTiptapStyles()
    injectTiptapStyles()
    expect(document.head.querySelectorAll('style#tiptap-shared-styles')).toHaveLength(1)
  })

  it('adds them even when content on the page carries their id', () => {
    document.body.innerHTML = '<span id="tiptap-shared-styles"></span>'
    injectTiptapStyles()
    expect(document.head.querySelector('style#tiptap-shared-styles')).not.toBeNull()
  })
})
