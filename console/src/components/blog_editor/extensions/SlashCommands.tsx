import React, { useState, useEffect, forwardRef, useImperativeHandle } from 'react'
import { ReactRenderer } from '@tiptap/react'
import { Extension } from '@tiptap/core'
import { Suggestion, SuggestionOptions, SuggestionProps } from '@tiptap/suggestion'
import tippy, { Instance as TippyInstance } from 'tippy.js'
import { Editor } from '@tiptap/react'
import { 
  FileTextIcon, 
  Heading1Icon, 
  Heading2Icon, 
  Heading3Icon,
  ListIcon,
  ListOrderedIcon,
  CheckSquareIcon,
  QuoteIcon,
  CodeIcon,
  ImageIcon,
  TableIcon,
  MinusIcon,
  TypeIcon
} from 'lucide-react'

export interface CommandItem {
  title: string
  description: string
  icon: React.ReactNode
  command: (props: { editor: Editor; range: any }) => void
  category: string
}

export const commands: CommandItem[] = [
  // Text blocks
  {
    title: 'Text',
    description: 'Just start typing with plain text',
    icon: <TypeIcon size={18} />,
    category: 'Basic',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setParagraph().run()
    }
  },
  {
    title: 'Heading 1',
    description: 'Big section heading',
    icon: <Heading1Icon size={18} />,
    category: 'Basic',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setNode('heading', { level: 1 }).run()
    }
  },
  {
    title: 'Heading 2',
    description: 'Medium section heading',
    icon: <Heading2Icon size={18} />,
    category: 'Basic',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setNode('heading', { level: 2 }).run()
    }
  },
  {
    title: 'Heading 3',
    description: 'Small section heading',
    icon: <Heading3Icon size={18} />,
    category: 'Basic',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setNode('heading', { level: 3 }).run()
    }
  },

  // Lists
  {
    title: 'Bullet List',
    description: 'Create a simple bullet list',
    icon: <ListIcon size={18} />,
    category: 'Lists',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleBulletList().run()
    }
  },
  {
    title: 'Numbered List',
    description: 'Create a list with numbering',
    icon: <ListOrderedIcon size={18} />,
    category: 'Lists',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleOrderedList().run()
    }
  },
  {
    title: 'Task List',
    description: 'Track tasks with a checklist',
    icon: <CheckSquareIcon size={18} />,
    category: 'Lists',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).toggleTaskList().run()
    }
  },

  // Advanced blocks
  {
    title: 'Quote',
    description: 'Capture a quote',
    icon: <QuoteIcon size={18} />,
    category: 'Advanced',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setBlockquote().run()
    }
  },
  {
    title: 'Code Block',
    description: 'Display code with syntax highlighting',
    icon: <CodeIcon size={18} />,
    category: 'Advanced',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setCodeBlock().run()
    }
  },
  {
    title: 'Table',
    description: 'Insert a table',
    icon: <TableIcon size={18} />,
    category: 'Advanced',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
    }
  },
  {
    title: 'Divider',
    description: 'Visually divide blocks',
    icon: <MinusIcon size={18} />,
    category: 'Advanced',
    command: ({ editor, range }) => {
      editor.chain().focus().deleteRange(range).setHorizontalRule().run()
    }
  },
  {
    title: 'Image',
    description: 'Insert an image',
    icon: <ImageIcon size={18} />,
    category: 'Media',
    command: ({ editor, range }) => {
      const url = window.prompt('Enter image URL:')
      if (url) {
        editor.chain().focus().deleteRange(range).setImage({ src: url }).run()
      }
    }
  }
]

interface CommandListProps {
  items: CommandItem[]
  command: (item: CommandItem) => void
}

export const CommandList = forwardRef<any, CommandListProps>((props, ref) => {
  const [selectedIndex, setSelectedIndex] = useState(0)

  const selectItem = (index: number) => {
    const item = props.items[index]
    if (item) {
      props.command(item)
    }
  }

  const upHandler = () => {
    setSelectedIndex((selectedIndex + props.items.length - 1) % props.items.length)
  }

  const downHandler = () => {
    setSelectedIndex((selectedIndex + 1) % props.items.length)
  }

  const enterHandler = () => {
    selectItem(selectedIndex)
  }

  useEffect(() => setSelectedIndex(0), [props.items])

  useImperativeHandle(ref, () => ({
    onKeyDown: ({ event }: { event: KeyboardEvent }) => {
      if (event.key === 'ArrowUp') {
        upHandler()
        return true
      }

      if (event.key === 'ArrowDown') {
        downHandler()
        return true
      }

      if (event.key === 'Enter') {
        enterHandler()
        return true
      }

      return false
    }
  }))

  // Group items by category
  const groupedItems = props.items.reduce((acc, item) => {
    if (!acc[item.category]) {
      acc[item.category] = []
    }
    acc[item.category].push(item)
    return acc
  }, {} as Record<string, CommandItem[]>)

  let currentIndex = 0

  return (
    <div className="slash-command-menu">
      {Object.entries(groupedItems).map(([category, items]) => (
        <div key={category} className="slash-command-category">
          <div className="slash-command-category-title">{category}</div>
          {items.map((item) => {
            const itemIndex = currentIndex++
            return (
              <button
                key={itemIndex}
                className={`slash-command-item ${itemIndex === selectedIndex ? 'selected' : ''}`}
                onClick={() => selectItem(itemIndex)}
              >
                <div className="slash-command-icon">{item.icon}</div>
                <div className="slash-command-content">
                  <div className="slash-command-title">{item.title}</div>
                  <div className="slash-command-description">{item.description}</div>
                </div>
              </button>
            )
          })}
        </div>
      ))}
    </div>
  )
})

CommandList.displayName = 'CommandList'

const suggestion: Omit<SuggestionOptions, 'editor'> = {
  items: ({ query }: { query: string }) => {
    return commands.filter((item) =>
      item.title.toLowerCase().includes(query.toLowerCase()) ||
      item.description.toLowerCase().includes(query.toLowerCase())
    )
  },

  render: () => {
    let component: ReactRenderer
    let popup: TippyInstance[]

    return {
      onStart: (props: SuggestionProps) => {
        component = new ReactRenderer(CommandList, {
          props,
          editor: props.editor
        })

        if (!props.clientRect) {
          return
        }

        popup = tippy('body', {
          getReferenceClientRect: props.clientRect as any,
          appendTo: () => document.body,
          content: component.element,
          showOnCreate: true,
          interactive: true,
          trigger: 'manual',
          placement: 'bottom-start',
          maxWidth: 400
        })
      },

      onUpdate(props: SuggestionProps) {
        component.updateProps(props)

        if (!props.clientRect) {
          return
        }

        popup[0].setProps({
          getReferenceClientRect: props.clientRect as any
        })
      },

      onKeyDown(props: { event: KeyboardEvent }) {
        if (props.event.key === 'Escape') {
          popup[0].hide()
          return true
        }

        return component.ref?.onKeyDown(props) || false
      },

      onExit() {
        popup[0].destroy()
        component.destroy()
      }
    }
  }
}

export const SlashCommands = Extension.create({
  name: 'slashCommands',

  addOptions() {
    return {
      suggestion: {
        char: '/',
        command: ({ editor, range, props }: { editor: Editor; range: any; props: CommandItem }) => {
          props.command({ editor, range })
        }
      }
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion({
        editor: this.editor,
        ...this.options.suggestion,
        ...suggestion
      })
    ]
  }
})

