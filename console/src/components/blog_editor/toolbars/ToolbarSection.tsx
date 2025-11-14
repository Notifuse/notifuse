import type { ReactNode } from 'react'
import { Divider } from 'antd'

export interface ToolbarSectionProps {
  /**
   * Children to render inside the section
   */
  children: ReactNode
  /**
   * Whether to show a divider after this section
   * @default true
   */
  showDivider?: boolean
}

/**
 * ToolbarSection - Groups related toolbar buttons together
 * Optionally shows a divider after the section
 */
export function ToolbarSection({ children, showDivider = true }: ToolbarSectionProps) {
  return (
    <>
      <div className="notifuse-editor-toolbar-section">{children}</div>
      {showDivider && <Divider type="vertical" style={{ height: '20px', margin: '0 4px' }} />}
    </>
  )
}
