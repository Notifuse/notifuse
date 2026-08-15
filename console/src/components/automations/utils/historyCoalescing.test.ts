import { describe, it, expect } from 'vitest'
import { textEditCoalesceKey } from './historyCoalescing'

describe('textEditCoalesceKey', () => {
  it('names the single text field that was retyped', () => {
    expect(
      textEditCoalesceKey('n1', { duration: 2, description: 'Wel' }, { duration: 2, description: 'Welc' })
    ).toBe('n1:description')
  })

  it('coalesces the first keystroke, when the field does not exist yet', () => {
    // The panel spreads the config, so the very first character adds the key rather than changing
    // it. Without this the opening keystroke of every description would be its own undo step.
    expect(textEditCoalesceKey('n1', { duration: 2 }, { duration: 2, description: 'W' })).toBe(
      'n1:description'
    )
  })

  it('coalesces clearing a field', () => {
    expect(
      textEditCoalesceKey('n1', { description: 'Welcome' }, { description: undefined })
    ).toBe('n1:description')
  })

  it('keys by node as well as field, so two nodes never share a run', () => {
    const edit = (nodeId: string) =>
      textEditCoalesceKey(nodeId, { description: 'a' }, { description: 'ab' })

    expect(edit('n1')).not.toBe(edit('n2'))
  })

  it('covers text fields other than the description', () => {
    expect(textEditCoalesceKey('n1', { url: 'https://ex' }, { url: 'https://exa' })).toBe('n1:url')
  })

  it('declines when more than one field moved', () => {
    // A structural edit that happens to touch a string too must stay its own undo step.
    expect(
      textEditCoalesceKey(
        'n1',
        { description: 'a', continue_node_id: '' },
        { description: 'ab', continue_node_id: 'n2' }
      )
    ).toBeUndefined()
  })

  it('declines for non-text values', () => {
    // Numbers: a stepper click is a discrete action. Objects/arrays: never keystrokes.
    expect(textEditCoalesceKey('n1', { duration: 2 }, { duration: 3 })).toBeUndefined()
    expect(
      textEditCoalesceKey('n1', { variants: [] }, { variants: [{ id: 'A' }] })
    ).toBeUndefined()
  })

  it('declines when nothing changed', () => {
    expect(textEditCoalesceKey('n1', { description: 'a' }, { description: 'a' })).toBeUndefined()
  })

  it('declines when there is no previous config to compare', () => {
    expect(textEditCoalesceKey('n1', undefined, { description: 'a' })).toBeUndefined()
  })
})
