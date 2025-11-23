import { useContext } from 'react'
import { EditorContext } from '@tiptap/react'
import { FloatingToolbar } from './FloatingToolbar'
import { ToolbarButton } from './ToolbarButton'
import { ToolbarSection } from './ToolbarSection'
import { TurnIntoDropdown, LinkPopover, ColorPicker, MoreMenu } from './components'
import { useFloatingToolbar } from './useFloatingToolbar'
import { useControls } from '../core/state/useControls'
import { CENTER_SECTION_ACTIONS } from './config'

/**
 * SelectionToolbar - Complete floating toolbar for text selections
 * Integrates all toolbar components in the default layout
 */
export function SelectionToolbar() {
  const { editor } = useContext(EditorContext)!
  const { isDragging } = useControls(editor)
  const { shouldShow, getAnchorRect } = useFloatingToolbar(editor, {
    extraHideWhen: isDragging
  })

  if (!editor) {
    return null
  }

  return (
    <FloatingToolbar shouldShow={shouldShow} getAnchorRect={getAnchorRect}>
      {/* Left Section: Turn Into Dropdown */}
      <ToolbarSection>
        <TurnIntoDropdown hideWhenUnavailable={true} />
      </ToolbarSection>

      {/* Center Section: Text Formatting Marks */}
      <ToolbarSection>
        {CENTER_SECTION_ACTIONS.map((actionId) => (
          <ToolbarButton key={actionId} actionId={actionId} hideWhenUnavailable={true} />
        ))}
      </ToolbarSection>

      {/* Right Section: Link, Color, More */}
      <ToolbarSection showDivider={false}>
        <LinkPopover hideWhenUnavailable={true} />
        <ColorPicker hideWhenUnavailable={true} />
        <MoreMenu hideWhenUnavailable={true} />
      </ToolbarSection>
    </FloatingToolbar>
  )
}
