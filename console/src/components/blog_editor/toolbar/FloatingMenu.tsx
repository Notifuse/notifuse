import React, { useState } from 'react'
import { FloatingMenu as TiptapFloatingMenu } from '@tiptap/react'
import { Editor } from '@tiptap/react'
import { PlusIcon } from 'lucide-react'
import { commands, CommandItem } from '../extensions/SlashCommands'

interface FloatingMenuProps {
  editor: Editor
}

export const FloatingMenu: React.FC<FloatingMenuProps> = ({ editor }) => {
  const [showMenu, setShowMenu] = useState(false)

  if (!editor) {
    return null
  }

  const handleCommand = (item: CommandItem) => {
    const { view } = editor
    const { from } = view.state.selection
    
    item.command({ 
      editor, 
      range: { from, to: from }
    })
    
    setShowMenu(false)
  }

  // Group items by category
  const groupedItems = commands.reduce((acc, item) => {
    if (!acc[item.category]) {
      acc[item.category] = []
    }
    acc[item.category].push(item)
    return acc
  }, {} as Record<string, CommandItem[]>)

  return (
    <TiptapFloatingMenu
      editor={editor}
      tippyOptions={{ 
        duration: 100,
        placement: 'left',
        offset: [-8, 0]
      }}
      className="floating-menu"
      shouldShow={({ state }) => {
        const { $from } = state.selection
        const isEmptyParagraph = $from.parent.type.name === 'paragraph' && 
                                 $from.parent.nodeSize === 2
        return isEmptyParagraph
      }}
    >
      <div className="floating-menu-wrapper">
        <button
          className="floating-menu-button"
          onClick={() => setShowMenu(!showMenu)}
          title="Add block"
        >
          <PlusIcon size={16} />
        </button>
        
        {showMenu && (
          <div className="floating-menu-dropdown">
            {Object.entries(groupedItems).map(([category, items]) => (
              <div key={category} className="floating-menu-category">
                <div className="floating-menu-category-title">{category}</div>
                {items.map((item, index) => (
                  <button
                    key={index}
                    className="floating-menu-item"
                    onClick={() => handleCommand(item)}
                  >
                    <div className="floating-menu-icon">{item.icon}</div>
                    <div className="floating-menu-content">
                      <div className="floating-menu-title">{item.title}</div>
                      <div className="floating-menu-description">{item.description}</div>
                    </div>
                  </button>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </TiptapFloatingMenu>
  )
}

