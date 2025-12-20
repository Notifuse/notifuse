import React, { useState, useRef, useEffect } from 'react'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  type EdgeProps
} from '@xyflow/react'
import { Tooltip } from 'antd'
import { Plus, X } from 'lucide-react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faHourglass, faEnvelope } from '@fortawesome/free-regular-svg-icons'
import type { NodeType } from '../../../services/api/automation'

// Menu items for adding nodes
const ADD_NODE_MENU_ITEMS: { key: NodeType; label: string; icon: React.ReactNode }[] = [
  { key: 'delay', label: 'Delay', icon: <FontAwesomeIcon icon={faHourglass} style={{ color: '#faad14' }} /> },
  { key: 'email', label: 'Email', icon: <FontAwesomeIcon icon={faEnvelope} style={{ color: '#1890ff' }} /> }
]

export interface AutomationEdgeData {
  onDelete?: () => void
  onInsert?: (nodeType: NodeType) => void
  hasListSelected?: boolean
}

export const AutomationEdge: React.FC<EdgeProps<AutomationEdgeData>> = ({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  data
}) => {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition
  })

  const [menuOpen, setMenuOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const menuOpenRef = useRef(menuOpen)

  // Keep ref in sync with state
  useEffect(() => {
    menuOpenRef.current = menuOpen
  }, [menuOpen])

  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (!menuOpenRef.current) return
      if (containerRef.current && !containerRef.current.contains(e.target as globalThis.Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside, true)
    return () => document.removeEventListener('mousedown', handleClickOutside, true)
  }, [])

  const hasButtons = data?.onInsert || data?.onDelete

  return (
    <>
      <BaseEdge path={edgePath} markerEnd={markerEnd} style={style} />
      {hasButtons && (
        <EdgeLabelRenderer>
          <div
            ref={containerRef}
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: 'all',
              zIndex: 1
            }}
            className="nodrag nopan"
          >
            {/* Buttons container - visible on hover */}
            <div
              className="flex items-center gap-1 opacity-0 hover:opacity-100 transition-opacity duration-150"
              style={{ padding: '4px' }}
            >
              {/* Plus button */}
              {data?.onInsert && (
                <div className="relative">
                  <Tooltip title="Add node" placement="left">
                    <button
                      className="add-node-button flex items-center justify-center w-6 h-6 rounded-full shadow-md border-2 border-white cursor-pointer transition-transform hover:scale-110"
                      onClick={() => setMenuOpen(!menuOpen)}
                    >
                      <Plus size={14} color="white" />
                    </button>
                  </Tooltip>

                  {/* Dropdown menu */}
                  {menuOpen && (
                    <div
                      className="absolute top-full left-1/2 mt-1 bg-white rounded-md shadow-lg border border-gray-200 py-1 min-w-[120px]"
                      style={{ transform: 'translateX(-50%)', zIndex: 10000 }}
                    >
                      {ADD_NODE_MENU_ITEMS.map((item) => {
                        const isDisabled = item.key === 'email' && !data.hasListSelected
                        const button = (
                          <button
                            key={item.key}
                            className={`w-full px-3 py-2 text-left text-sm flex items-center gap-2 ${
                              isDisabled
                                ? 'opacity-50 cursor-not-allowed'
                                : 'hover:bg-gray-100 cursor-pointer'
                            }`}
                            onClick={() => {
                              if (isDisabled) return
                              data.onInsert?.(item.key)
                              setMenuOpen(false)
                            }}
                          >
                            {item.icon}
                            {item.label}
                          </button>
                        )
                        return isDisabled ? (
                          <Tooltip key={item.key} title="Select a list to enable email nodes" placement="right">
                            {button}
                          </Tooltip>
                        ) : (
                          button
                        )
                      })}
                    </div>
                  )}
                </div>
              )}

              {/* Delete button */}
              {data?.onDelete && (
                <Tooltip title="Delete edge" placement="right">
                  <button
                    className="flex items-center justify-center w-6 h-6 rounded-full bg-white hover:bg-red-50 shadow-md border border-gray-200 cursor-pointer transition-transform hover:scale-110"
                    onClick={() => data.onDelete?.()}
                  >
                    <X size={14} className="text-gray-400 hover:text-red-500" />
                  </button>
                </Tooltip>
              )}
            </div>
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

// Simple smooth step edge for the default (no delete button)
export const SmoothStepEdge: React.FC<EdgeProps> = (props) => {
  return <AutomationEdge {...props} />
}
