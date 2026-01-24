import { useCallback, useEffect, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { flip, offset, shift, size } from '@floating-ui/react'
import { PluginKey } from '@tiptap/pm/state'
import { Suggestion } from '@tiptap/suggestion'
import type { SuggestionKeyDownProps, SuggestionProps } from '@tiptap/suggestion'
import type { Editor } from '@tiptap/react'

// --- Hooks ---
import { useFloatingMenu } from '../../hooks/useFloatingMenu'
import { useMenuKeyboard } from '../../hooks/useMenuKeyboard'
import { useNotifuseEditor } from '../../hooks/useEditor'

// --- Components ---
import { SlashPopoverContent } from './SlashPopoverContent'

// --- Local Types and Config ---
import { slashConfig } from './configs/slash-config'
import type { SuggestionItem } from './types'
import type { ActionDefinition } from '../../core/registry/ActionRegistry'

interface SlashMenuProps {
  /** Optional editor instance (if not using context) */
  editor?: Editor | null
}

/**
 * Slash command menu component for the editor
 * Triggered by '/' character
 */
export const SlashMenu = ({ editor: providedEditor }: SlashMenuProps) => {
  const { t } = useLingui()
  const { editor } = useNotifuseEditor(providedEditor)

  const [show, setShow] = useState<boolean>(false)
  const [internalDecorationNode, setInternalDecorationNode] = useState<HTMLElement | null>(null)
  const [internalCommand, setInternalCommand] = useState<((item: SuggestionItem<unknown>) => void) | null>(null)
  const [internalItems, setInternalItems] = useState<SuggestionItem<unknown>[]>([])
  const [internalQuery, setInternalQuery] = useState<string>('')

  const configRef = useRef(slashConfig)

  // Track query length to auto-hide menu when no results after typing 5+ chars
  const noResultsQueryLengthRef = useRef<number>(0)

  const { ref, style, getFloatingProps, isMounted } = useFloatingMenu(
    show,
    internalDecorationNode,
    1000,
    {
      placement: 'bottom-start',
      middleware: [
        offset(10),
        flip({
          mainAxis: true,
          crossAxis: false
        }),
        shift(),
        size({
          apply({ availableHeight, elements }) {
            if (elements.floating) {
              const maxHeightValue = slashConfig.maxHeight
                ? Math.min(slashConfig.maxHeight, availableHeight)
                : availableHeight

              elements.floating.style.setProperty(
                '--suggestion-menu-max-height',
                `${maxHeightValue}px`
              )
            }
          }
        })
      ],
      onOpenChange(open) {
        if (!open) {
          setShow(false)
        }
      }
    }
  )

  const closePopup = useCallback(() => {
    setShow(false)
  }, [])

  useEffect(() => {
    if (!editor || editor.isDestroyed) {
      return
    }

    const pluginKey = new PluginKey(slashConfig.pluginKey)

    const existingPlugin = editor.state.plugins.find((plugin) => plugin.spec.key === pluginKey)
    if (existingPlugin) {
      editor.unregisterPlugin(pluginKey)
    }

    const suggestion = Suggestion({
      pluginKey,
      editor,
      char: slashConfig.char,

      allow(props) {
        const $from = editor.state.doc.resolve(props.range.from)

        // Check if we're inside an image node
        for (let depth = $from.depth; depth > 0; depth--) {
          if ($from.node(depth).type.name === 'image') {
            return false // Don't allow slash command inside image (since we support captions)
          }
        }

        return true
      },

      items: async ({ query, editor: editorInstance }) => {
        const items = await configRef.current.getItems(query, editorInstance)
        return items
      },

      command({ editor: editorInstance, range, props }) {
        if (!range || !props) {
          return
        }

        const { view } = editorInstance

        const nodeAfter = view.state.selection.$to.nodeAfter
        const overrideSpace = nodeAfter?.text?.startsWith(' ')

        const rangeToUse = { ...range }

        if (overrideSpace) {
          rangeToUse.to += 1
        }

        // Call the config's onSelect handler
        configRef.current.onSelect(props as SuggestionItem<ActionDefinition | { type: string }>, editorInstance, rangeToUse)
      },

      render: () => {
        return {
          onStart: (props: SuggestionProps<SuggestionItem<unknown>>) => {
            setInternalDecorationNode((props.decorationNode as HTMLElement) ?? null)
            setInternalCommand(() => props.command)
            setInternalItems(props.items)
            setInternalQuery(props.query)
            setShow(true)
            // Reset tracking when menu starts
            noResultsQueryLengthRef.current = 0
          },

          onUpdate: (props: SuggestionProps<SuggestionItem<unknown>>) => {
            setInternalDecorationNode((props.decorationNode as HTMLElement) ?? null)
            setInternalCommand(() => props.command)
            setInternalItems(props.items)
            setInternalQuery(props.query)

            // Auto-hide menu if user typed 5+ characters with no results
            if (props.items.length === 0 && props.query.length >= 5) {
              // Close the menu
              closePopup()
            } else if (props.items.length > 0) {
              // Reset the tracking when we have results
              noResultsQueryLengthRef.current = 0
            }
          },

          onKeyDown: (props: SuggestionKeyDownProps) => {
            if (props.event.key === 'Escape') {
              closePopup()
              return true
            }
            return false
          },

          onExit: () => {
            setInternalDecorationNode(null)
            setInternalCommand(null)
            setInternalItems([])
            setInternalQuery('')
            setShow(false)
            noResultsQueryLengthRef.current = 0
          }
        }
      }
    })

    editor.registerPlugin(suggestion)

    return () => {
      if (!editor.isDestroyed) {
        editor.unregisterPlugin(pluginKey)
      }
    }
  }, [editor, closePopup])

  const onSelect = useCallback(
    (item: SuggestionItem<unknown>) => {
      closePopup()

      if (internalCommand) {
        internalCommand(item)
      }
    },
    [closePopup, internalCommand]
  )

  const { selectedIndex } = useMenuKeyboard({
    editor: editor,
    query: internalQuery,
    items: internalItems,
    onSelect
  })

  if (!isMounted || !show || !editor) {
    return null
  }

  return (
    <div
      ref={ref}
      style={style}
      {...getFloatingProps()}
      data-selector="slash-menu"
      className="tiptap-suggestion-menu"
      role="listbox"
      aria-label={t`Slash Commands`}
      onPointerDown={(e) => e.preventDefault()}
    >
      <SlashPopoverContent
        items={internalItems}
        selectedIndex={selectedIndex}
        onSelect={onSelect}
      />
    </div>
  )
}
