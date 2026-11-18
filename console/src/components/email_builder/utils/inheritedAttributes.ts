import type { EmailBlock, MJMLComponentType } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { mergeWithDefaults } from '../mjml-defaults'
import { socialInheritedAttributes } from '../blocks/MjSocialBlock'

type AttributeDefaults = Partial<Record<MJMLComponentType, Record<string, unknown>>>

// inheritedAttributes: what a block shows when it sets nothing itself, in the canvas's order —
// the editor's MJML defaults, then the type's mj-attributes (mj-all and mj-class are not read, as
// on the canvas). Two exceptions:
// - a social element inherits what its mj-social passes, between its defaults and the
//   mj-social-element mj-attributes (MJML's order, as MjSocialElementBlock draws it);
// - a block inside mj-attributes holds the defaults themselves: it inherits the MJML defaults only.
export function inheritedAttributes(
  tree: EmailBlock,
  block: EmailBlock,
  attributeDefaults: AttributeDefaults
): Record<string, unknown> {
  const type = block.type as MJMLComponentType
  if (EmailBlockClass.isChildOf(tree, block.id, 'mj-attributes')) {
    return mergeWithDefaults(type, {}) as Record<string, unknown>
  }
  if (type === 'mj-social-element') {
    const ancestors = EmailBlockClass.getAncestorIds(tree, block.id)
    const parent = EmailBlockClass.findBlockById(tree, ancestors[ancestors.length - 1] ?? '')
    const passed =
      parent?.type === 'mj-social'
        ? socialInheritedAttributes(
            parent.attributes as Record<string, unknown> | undefined,
            attributeDefaults['mj-social']
          )
        : {}
    return EmailBlockClass.mergeWithAllDefaults('mj-social-element', {}, {
      ...attributeDefaults,
      'mj-social-element': { ...passed, ...attributeDefaults['mj-social-element'] }
    }) as Record<string, unknown>
  }
  return EmailBlockClass.mergeWithAllDefaults(type, {}, attributeDefaults) as Record<string, unknown>
}
