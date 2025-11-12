import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import { GripVerticalIcon } from 'lucide-react'
import { createRoot } from 'react-dom/client'

export const DragHandle = Extension.create({
  name: 'dragHandle',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('dragHandle'),
        
        props: {
          decorations(state) {
            const decorations: Decoration[] = []
            const { doc } = state

            doc.descendants((node, pos) => {
              // Add drag handle decoration to block-level nodes
              if (
                node.isBlock &&
                node.type.name !== 'doc' &&
                pos !== 0
              ) {
                const decoration = Decoration.widget(pos, () => {
                  const dragHandleEl = document.createElement('div')
                  dragHandleEl.className = 'drag-handle-wrapper'
                  dragHandleEl.contentEditable = 'false'
                  dragHandleEl.draggable = true

                  const dragHandle = document.createElement('div')
                  dragHandle.className = 'drag-handle'
                  dragHandle.title = 'Drag to move'

                  // Create React icon
                  const iconContainer = document.createElement('span')
                  const root = createRoot(iconContainer)
                  root.render(<GripVerticalIcon size={14} />)
                  dragHandle.appendChild(iconContainer)

                  dragHandleEl.appendChild(dragHandle)

                  // Drag and drop handlers
                  let draggedNode: { node: any; pos: number } | null = null

                  dragHandleEl.addEventListener('dragstart', (e) => {
                    draggedNode = { node, pos }
                    if (e.dataTransfer) {
                      e.dataTransfer.effectAllowed = 'move'
                      e.dataTransfer.setData('text/html', 'dragging')
                    }
                    dragHandleEl.classList.add('dragging')
                  })

                  dragHandleEl.addEventListener('dragend', () => {
                    draggedNode = null
                    dragHandleEl.classList.remove('dragging')
                  })

                  // Store draggedNode on the element for drop handler
                  ;(dragHandleEl as any)._draggedNode = () => draggedNode

                  return dragHandleEl
                })

                decorations.push(decoration)
              }

              return true
            })

            return DecorationSet.create(doc, decorations)
          },

          handleDOMEvents: {
            dragover(view, event) {
              event.preventDefault()
              return false
            },

            drop(view, event) {
              event.preventDefault()

              // Find the drag handle element that was clicked
              const dragHandles = document.querySelectorAll('.drag-handle-wrapper')
              let draggedNode: { node: any; pos: number } | null = null

              dragHandles.forEach((handle) => {
                const getData = (handle as any)._draggedNode
                if (getData) {
                  const data = getData()
                  if (data) {
                    draggedNode = data
                  }
                }
              })

              if (!draggedNode) return false

              const pos = view.posAtCoords({
                left: event.clientX,
                top: event.clientY
              })

              if (!pos) return false

              const { state, dispatch } = view
              const { tr } = state

              // Calculate the new position
              const $pos = tr.doc.resolve(pos.pos)
              let insertPos = $pos.before($pos.depth)

              // Don't drop on itself
              if (insertPos === draggedNode.pos) return false

              // Delete from old position
              const deleteFrom = draggedNode.pos
              const deleteTo = draggedNode.pos + draggedNode.node.nodeSize

              // Adjust insert position if we're moving forward
              if (insertPos > deleteFrom) {
                insertPos -= draggedNode.node.nodeSize
              }

              // Perform the move
              tr.delete(deleteFrom, deleteTo)
              tr.insert(insertPos, draggedNode.node)

              dispatch(tr)
              return true
            }
          }
        }
      })
    ]
  }
})

