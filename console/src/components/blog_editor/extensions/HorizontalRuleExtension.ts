import { mergeAttributes } from '@tiptap/react'
import TiptapHorizontalRule from '@tiptap/extension-horizontal-rule'

/**
 * HorizontalRule Extension
 *
 * Extends the standard HorizontalRule extension to wrap the <hr> element
 * in a <div> container with a data-type attribute for better styling control.
 *
 * HTML Output: <div data-type="horizontalRule"><hr /></div>
 */
export const HorizontalRuleExtension = TiptapHorizontalRule.extend({
  name: 'horizontalRule',

  renderHTML({ HTMLAttributes }: { HTMLAttributes: Record<string, unknown> }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-type': this.name }), ['hr']]
  }
})
