import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { flip, offset, shift, size } from '@floating-ui/react'
import { PluginKey } from '@tiptap/pm/state'
import { Suggestion } from '@tiptap/suggestion'
import type { SuggestionKeyDownProps, SuggestionProps } from '@tiptap/suggestion'
import type { Editor } from '@tiptap/react'
import { Menu } from 'antd'
import type { MenuProps } from 'antd'

// --- Hooks ---
import { useFloatingMenu } from '../../hooks/useFloatingMenu'
import { useMenuKeyboard } from '../../hooks/useMenuKeyboard'
import { useNotifuseEditor } from '../../hooks/useEditor'

// --- Local Types and Config ---
import { emojiConfig } from './configs/emoji-config'
import type { SuggestionItem } from './types'
import type { EmojiItem } from '@tiptap/extension-emoji'
import './suggestion-menu-core.css'

interface EmojiMenuProps {
  /** Optional editor instance (if not using context) */
  editor?: Editor | null
}

/**
 * Emoji suggestion menu component for the editor
 * Triggered by ':' character
 */
export const EmojiMenu = ({ editor: providedEditor }: EmojiMenuProps) => {
  const { t } = useLingui()
  const { editor } = useNotifuseEditor(providedEditor)

  const [show, setShow] = useState<boolean>(false)
  const [internalDecorationNode, setInternalDecorationNode] = useState<HTMLElement | null>(null)
  const [internalCommand, setInternalCommand] = useState<((item: SuggestionItem<EmojiItem>) => void) | null>(null)
  const [internalItems, setInternalItems] = useState<SuggestionItem<EmojiItem>[]>([])
  const [internalQuery, setInternalQuery] = useState<string>('')

  const configRef = useRef(emojiConfig)

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
              const maxHeightValue = emojiConfig.maxHeight
                ? Math.min(emojiConfig.maxHeight, availableHeight)
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

    const pluginKey = new PluginKey(emojiConfig.pluginKey)

    const existingPlugin = editor.state.plugins.find((plugin) => plugin.spec.key === pluginKey)
    if (existingPlugin) {
      editor.unregisterPlugin(pluginKey)
    }

    const suggestion = Suggestion({
      pluginKey,
      editor,
      char: emojiConfig.char,

      allow(props) {
        const $from = editor.state.doc.resolve(props.range.from)

        // Check if we're inside an image node
        for (let depth = $from.depth; depth > 0; depth--) {
          if ($from.node(depth).type.name === 'image') {
            return false // Don't allow emoji inside image (since we support captions)
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
        configRef.current.onSelect(props as SuggestionItem<EmojiItem>, editorInstance, rangeToUse)
      },

      render: () => {
        return {
          onStart: (props: SuggestionProps<SuggestionItem<EmojiItem>>) => {
            setInternalDecorationNode((props.decorationNode as HTMLElement) ?? null)
            setInternalCommand(() => props.command)
            setInternalItems(props.items)
            setInternalQuery(props.query)
            setShow(true)
          },

          onUpdate: (props: SuggestionProps<SuggestionItem<EmojiItem>>) => {
            setInternalDecorationNode((props.decorationNode as HTMLElement) ?? null)
            setInternalCommand(() => props.command)
            setInternalItems(props.items)
            setInternalQuery(props.query)
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
    (item: SuggestionItem<EmojiItem>) => {
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

  // Group items by their group property (emojis typically don't have groups)
  const groupItems = (items: SuggestionItem<EmojiItem>[]) => {
    if (!items.some((item) => item.group)) {
      return { ungrouped: items }
    }

    const grouped: Record<string, SuggestionItem<EmojiItem>[]> = {}

    items.forEach((item) => {
      const group: string = emojiConfig.groupBy?.(item) ?? item.group ?? 'ungrouped'
      if (!grouped[group]) {
        grouped[group] = []
      }
      grouped[group].push(item)
    })

    return grouped
  }

  // Convert items to Antd Menu items format
  const menuItems: MenuProps['items'] = useMemo(() => {
    const grouped = groupItems(internalItems)
    const groupNames = Object.keys(grouped)
    let currentIndex = 0

    const result: MenuProps['items'] = []

    groupNames.forEach((groupName) => {
      const groupItems = grouped[groupName]
      const isUngrouped = groupName === 'ungrouped'

      const items = groupItems.map((item) => {
        const globalIndex = currentIndex++
        const isSelected = globalIndex === selectedIndex

        return {
          key: item.id,
          label: emojiConfig.renderItem ? (
            emojiConfig.renderItem(item, isSelected)
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', width: '100%' }}>
              {item.icon && (
                <span style={{ display: 'flex', alignItems: 'center', fontSize: '18px' }}>
                  {typeof item.icon === 'string' ? (
                    item.icon
                  ) : (
                    <item.icon style={{ width: '18px', height: '18px' }} />
                  )}
                </span>
              )}
              <span style={{ flex: 1, wordBreak: 'break-word' }}>{item.label}</span>
            </div>
          ),
          onClick: () => onSelect(item),
          onMouseDown: (e: React.MouseEvent) => e.preventDefault()
        }
      })

      if (isUngrouped) {
        result.push(...items)
      } else {
        // Add as a group
        result.push({
          type: 'group',
          label: groupName,
          children: items
        })
      }
    })

    return result
  }, [internalItems, selectedIndex, onSelect])

  const selectedItem = selectedIndex !== undefined ? internalItems[selectedIndex] : undefined
  const selectedKey = selectedItem?.id

  if (!isMounted || !show || !editor) {
    return null
  }

  return (
    <div
      ref={ref}
      style={style}
      {...getFloatingProps()}
      data-selector="emoji-menu"
      className="tiptap-suggestion-menu"
      role="listbox"
      aria-label={t`Emoji Suggestions`}
      onPointerDown={(e) => e.preventDefault()}
    >
      <Menu
        className="suggestion-menu-core"
        data-suggestion-menu
        mode="vertical"
        selectedKeys={selectedKey ? [selectedKey] : []}
        items={menuItems}
        style={{ maxHeight: emojiConfig.maxHeight || 384, overflowY: 'auto', border: 'none' }}
      />
    </div>
  )
}
