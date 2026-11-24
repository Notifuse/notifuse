import { describe, it, expect, afterEach } from 'vitest'
import { i18n } from '@lingui/core'
import { generateMessageId } from '@lingui/message-utils/generateMessageId'
import { EmailBlockClass } from '../EmailBlockClass'
import type { EmailBlock } from '../types'

const labelOf = (type: string) =>
  EmailBlockClass.from({ id: 'b', type, attributes: {} } as unknown as EmailBlock).getLabel()

// The block names the tree, the breadcrumb, the library and the settings panel show
describe('EmailBlockClass labels', () => {
  afterEach(() => {
    i18n.activate('en')
  })

  it('names each block in English with the English catalogue', () => {
    expect(
      ['mj-head', 'mj-body', 'mj-section', 'mj-column', 'mj-text', 'mj-raw', 'mj-attributes'].map(
        labelOf
      )
    ).toEqual(['Head', 'Body', 'Section', 'Column', 'Text', 'Raw HTML', 'Default attributes'])
  })

  // "Text" and "Section" name other things in the console too: the block names carry
  // their own context, so a catalogue can translate them apart.
  it("follows the console's language, under the email block context", () => {
    i18n.load('fr', {
      [generateMessageId('Text', 'email block')]: 'Texte',
      [generateMessageId('Default attributes', 'email block')]: 'Attributs par défaut'
    })
    i18n.activate('fr')

    expect(labelOf('mj-text')).toBe('Texte')
    expect(labelOf('mj-attributes')).toBe('Attributs par défaut')
    // A name the catalogue lacks falls back to English
    expect(labelOf('mj-spacer')).toBe('Spacer')
  })

  it('keeps the name of a social network as it is', () => {
    const element = {
      id: 'se',
      type: 'mj-social-element',
      attributes: { name: 'facebook-noshare' }
    } as unknown as EmailBlock
    expect(EmailBlockClass.from(element).getLabel()).toBe('Facebook')
  })
})
