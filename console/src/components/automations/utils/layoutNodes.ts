import type { Node } from '@xyflow/react'
import type { ABTestNodeConfig, NodeType } from '../../../services/api/automation'

interface LayoutOptions {
  horizontalSpacing?: number
  verticalSpacing?: number
  nodeWidth?: number
  startX?: number
  startY?: number
}

interface NodeWithType {
  id: string
  data: {
    nodeType: NodeType
    config?: Record<string, unknown>
  }
}

const DEFAULT_OPTIONS: Required<LayoutOptions> = {
  horizontalSpacing: 80,
  verticalSpacing: 200,
  nodeWidth: 220,
  startX: 400,
  startY: 50
}

/**
 * Reorganize nodes in a clean hierarchical layout.
 * Works with center coordinates internally, converts to left-edge for ReactFlow.
 */
export function layoutNodes<T extends NodeWithType>(
  nodes: T[],
  edges: { source: string; target: string; sourceHandle?: string }[],
  options: LayoutOptions = {}
): T[] {
  const opts = { ...DEFAULT_OPTIONS, ...options }
  const { horizontalSpacing, verticalSpacing, nodeWidth, startX, startY } = opts

  const triggerNode = nodes.find((n) => n.data.nodeType === 'trigger')
  if (!triggerNode) return nodes

  // Build adjacency list (parent → children) with edge info for ordering
  const childrenWithHandles = new Map<string, { target: string; sourceHandle?: string }[]>()
  edges.forEach((e) => {
    if (!childrenWithHandles.has(e.source)) childrenWithHandles.set(e.source, [])
    childrenWithHandles.get(e.source)!.push({ target: e.target, sourceHandle: e.sourceHandle })
  })

  // Get ordered children for a node (A/B Test children sorted by variant order)
  const getOrderedChildren = (nodeId: string): string[] => {
    const node = nodes.find((n) => n.id === nodeId)
    const childEdges = childrenWithHandles.get(nodeId) || []

    if (node?.data.nodeType === 'ab_test') {
      const config = node.data.config as ABTestNodeConfig | undefined
      const variantOrder = config?.variants?.map((v) => v.id) || []
      return childEdges
        .sort((a, b) => {
          const aIndex = variantOrder.indexOf(a.sourceHandle || '')
          const bIndex = variantOrder.indexOf(b.sourceHandle || '')
          return aIndex - bIndex
        })
        .map((e) => e.target)
    }

    if (node?.data.nodeType === 'filter') {
      return childEdges
        .sort((a, b) => {
          const order: Record<string, number> = { yes: 0, continue: 0, no: 1, exit: 1 }
          const aOrder = order[a.sourceHandle || ''] ?? 2
          const bOrder = order[b.sourceHandle || ''] ?? 2
          return aOrder - bOrder
        })
        .map((e) => e.target)
    }

    return childEdges.map((e) => e.target)
  }

  // Build children map
  const children = new Map<string, string[]>()
  nodes.forEach((n) => {
    children.set(n.id, getOrderedChildren(n.id))
  })

  // Calculate subtree widths (bottom-up)
  const subtreeWidthCache = new Map<string, number>()
  const getSubtreeWidth = (nodeId: string, visited: Set<string> = new Set()): number => {
    if (visited.has(nodeId)) return 0
    visited.add(nodeId)

    if (subtreeWidthCache.has(nodeId)) return subtreeWidthCache.get(nodeId)!

    const kids = children.get(nodeId) || []
    let width: number
    if (kids.length === 0) {
      width = nodeWidth
    } else {
      width =
        kids.reduce((sum, kid) => sum + getSubtreeWidth(kid, new Set(visited)), 0) +
        (kids.length - 1) * horizontalSpacing
    }
    subtreeWidthCache.set(nodeId, width)
    return width
  }

  // Assign positions (top-down) - using CENTER x coordinates
  const newPositions = new Map<string, { x: number; y: number }>()

  const layoutNode = (nodeId: string, x: number, y: number, visited: Set<string> = new Set()) => {
    if (visited.has(nodeId)) return
    visited.add(nodeId)

    newPositions.set(nodeId, { x, y })

    const kids = children.get(nodeId) || []
    if (kids.length === 0) return

    const childWidths = kids.map((k) => getSubtreeWidth(k))
    const totalWidth =
      childWidths.reduce((a, b) => a + b, 0) + (kids.length - 1) * horizontalSpacing

    let childX = x - totalWidth / 2 + childWidths[0] / 2
    const childY = y + verticalSpacing

    kids.forEach((kid, i) => {
      layoutNode(kid, childX, childY, new Set(visited))
      if (i < kids.length - 1) {
        childX += childWidths[i] / 2 + horizontalSpacing + childWidths[i + 1] / 2
      }
    })
  }

  // Start layout from trigger at top-center
  layoutNode(triggerNode.id, startX, startY)

  // Handle orphan nodes
  const orphanNodes = nodes.filter((n) => !newPositions.has(n.id))
  if (orphanNodes.length > 0) {
    let maxX = startX
    newPositions.forEach((pos) => {
      if (pos.x > maxX) maxX = pos.x
    })

    let orphanX = maxX + 400
    let orphanY = startY
    orphanNodes.forEach((node) => {
      newPositions.set(node.id, { x: orphanX, y: orphanY })
      orphanY += verticalSpacing
    })
  }

  // Apply new positions - convert from center coordinates to left-edge
  return nodes.map((n) => {
    const centerPos = newPositions.get(n.id)
    if (!centerPos) return n
    return {
      ...n,
      position: {
        x: centerPos.x - nodeWidth / 2,
        y: centerPos.y
      }
    }
  })
}
