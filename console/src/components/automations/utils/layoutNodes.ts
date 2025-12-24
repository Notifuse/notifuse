import type { Node } from '@xyflow/react'
import type { ABTestNodeConfig, FilterNodeConfig, NodeType } from '../../../services/api/automation'

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
  nodeWidth: 300, // Matches BaseNode minWidth
  startX: 400,
  startY: 50
}

// Node height from BaseNode component
const NODE_HEIGHT = 86

/**
 * Reorganize nodes in a clean hierarchical layout.
 * Supports DAG structure (nodes with multiple parents).
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

  // === PASS 1: Build parent/child maps ===

  // Build child → parents map
  const parents = new Map<string, string[]>()
  edges.forEach((e) => {
    const existing = parents.get(e.target) || []
    parents.set(e.target, [...existing, e.source])
  })

  // Build parent → children map with edge info for ordering
  const childrenWithHandles = new Map<string, { target: string; sourceHandle?: string }[]>()
  edges.forEach((e) => {
    if (!childrenWithHandles.has(e.source)) childrenWithHandles.set(e.source, [])
    childrenWithHandles.get(e.source)!.push({ target: e.target, sourceHandle: e.sourceHandle })
  })

  // Get ordered children for a node (A/B Test and Filter children sorted by variant/branch order)
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

    if (node?.data.nodeType === 'list_status_branch') {
      return childEdges
        .sort((a, b) => {
          const order: Record<string, number> = { not_in_list: 0, active: 1, non_active: 2 }
          const aOrder = order[a.sourceHandle || ''] ?? 3
          const bOrder = order[b.sourceHandle || ''] ?? 3
          return aOrder - bOrder
        })
        .map((e) => e.target)
    }

    return childEdges.map((e) => e.target)
  }

  // Build children map with ordering
  const children = new Map<string, string[]>()
  nodes.forEach((n) => {
    children.set(n.id, getOrderedChildren(n.id))
  })

  // === PASS 2: Calculate levels (depth) for each node ===
  // Level = max(parent levels) + 1 (ensures multi-parent nodes are below ALL parents)

  const levels = new Map<string, number>()

  const calculateLevel = (nodeId: string, visiting: Set<string> = new Set()): number => {
    if (levels.has(nodeId)) return levels.get(nodeId)!
    if (visiting.has(nodeId)) return 0 // Cycle detection
    visiting.add(nodeId)

    const nodeParents = parents.get(nodeId) || []
    if (nodeParents.length === 0) {
      // No parents = root node (trigger)
      levels.set(nodeId, 0)
      return 0
    }

    const maxParentLevel = Math.max(...nodeParents.map((p) => calculateLevel(p, new Set(visiting))))
    const level = maxParentLevel + 1
    levels.set(nodeId, level)
    return level
  }

  // Calculate levels for all nodes
  nodes.forEach((n) => calculateLevel(n.id))

  // === PASS 3: Group nodes by level ===

  const nodesByLevel = new Map<number, string[]>()
  nodes.forEach((n) => {
    const level = levels.get(n.id) ?? 0
    const existing = nodesByLevel.get(level) || []
    nodesByLevel.set(level, [...existing, n.id])
  })

  // === PASS 4: Position nodes level by level ===

  const newPositions = new Map<string, { x: number; y: number }>()
  const maxLevel = Math.max(...Array.from(levels.values()), 0)

  // Track child order for consistent horizontal positioning
  const childOrderIndex = new Map<string, number>()

  // Get X offset from node center for a specific handle
  const getHandleOffsetX = (nodeType: NodeType, handleId: string | undefined): number => {
    const halfWidth = nodeWidth / 2

    if (nodeType === 'list_status_branch') {
      // Handles at 20%, 50%, 80% from left edge
      const offsets: Record<string, number> = {
        not_in_list: -halfWidth * 0.6, // 20% from left = 40% left of center
        active: 0, // 50% = center
        non_active: halfWidth * 0.6 // 80% from left = 40% right of center
      }
      return offsets[handleId || ''] ?? 0
    }

    if (nodeType === 'filter') {
      const offsets: Record<string, number> = {
        continue: -halfWidth * 0.4,
        yes: -halfWidth * 0.4,
        exit: halfWidth * 0.4,
        no: halfWidth * 0.4
      }
      return offsets[handleId || ''] ?? 0
    }

    if (nodeType === 'ab_test') {
      // A/B test handles are evenly distributed - handled by child order
      return 0
    }

    return 0
  }

  // Get target X for a node based on highest-level parent's handle position
  // Also checks for skip edges and clears intermediate obstacles (Sugiyama-style)
  const getTargetX = (nodeId: string): number => {
    const incomingEdges = edges.filter((e) => e.target === nodeId)
    if (incomingEdges.length === 0) return startX

    const myLevel = levels.get(nodeId) ?? 0

    // Find highest-level parent for base position
    let bestEdge = incomingEdges[0]
    let bestLevel = levels.get(bestEdge.source) ?? 999
    for (const edge of incomingEdges) {
      const lvl = levels.get(edge.source) ?? 999
      if (lvl < bestLevel) {
        bestLevel = lvl
        bestEdge = edge
      }
    }

    const parentPos = newPositions.get(bestEdge.source)
    if (!parentPos) return startX

    const parentNode = nodes.find((n) => n.id === bestEdge.source)
    const handleOffset = getHandleOffsetX(parentNode?.data.nodeType || 'trigger', bestEdge.sourceHandle)
    let targetX = parentPos.x + handleOffset

    // Check for skip edges (long edges spanning >1 level) and clear intermediate obstacles
    for (const edge of incomingEdges) {
      const sourceLevel = levels.get(edge.source) ?? 0
      const levelGap = myLevel - sourceLevel

      // Skip edge = more than 1 level gap
      if (levelGap > 1) {
        const edgeParentPos = newPositions.get(edge.source)
        if (!edgeParentPos) continue

        const edgeParentNode = nodes.find((n) => n.id === edge.source)
        const edgeHandleOffset = getHandleOffsetX(
          edgeParentNode?.data.nodeType || 'trigger',
          edge.sourceHandle
        )
        const edgeHandleX = edgeParentPos.x + edgeHandleOffset

        // Check all intermediate levels for obstacles
        for (let lvl = sourceLevel + 1; lvl < myLevel; lvl++) {
          const nodesAtLevel = nodesByLevel.get(lvl) || []

          for (const intermediateId of nodesAtLevel) {
            // Skip if this intermediate node is also a parent (edge connects to it, not crosses it)
            const isParent = incomingEdges.some((e) => e.source === intermediateId)
            if (isParent) continue

            const intermediatePos = newPositions.get(intermediateId)
            if (!intermediatePos) continue

            // Check if edge would cross this node's bounding box
            const nodeLeft = intermediatePos.x - nodeWidth / 2
            const nodeRight = intermediatePos.x + nodeWidth / 2

            if (edgeHandleX >= nodeLeft && edgeHandleX <= nodeRight) {
              // Edge would cross! Position to the right of this obstacle
              const clearX = nodeRight + horizontalSpacing
              if (clearX > targetX) {
                targetX = clearX
              }
            }
          }
        }
      }
    }

    return targetX
  }

  for (let level = 0; level <= maxLevel; level++) {
    const nodesAtLevel = nodesByLevel.get(level) || []
    const y = startY + level * verticalSpacing

    if (level === 0) {
      // Root level (trigger) - position at center
      nodesAtLevel.forEach((nodeId) => {
        newPositions.set(nodeId, { x: startX, y })
      })
      // Record child order for next level
      nodesAtLevel.forEach((nodeId) => {
        const kids = children.get(nodeId) || []
        kids.forEach((kid, idx) => {
          if (!childOrderIndex.has(kid)) {
            childOrderIndex.set(kid, idx)
          }
        })
      })
    } else {
      // Sort nodes by target X (based on parent handle position)
      const sortedNodes = [...nodesAtLevel].sort((a, b) => {
        const aTargetX = getTargetX(a)
        const bTargetX = getTargetX(b)

        // Primary sort: by target X position
        if (Math.abs(aTargetX - bTargetX) > 10) {
          return aTargetX - bTargetX
        }

        // Secondary sort: by child order (for siblings from same parent)
        const aOrder = childOrderIndex.get(a) ?? 999
        const bOrder = childOrderIndex.get(b) ?? 999
        return aOrder - bOrder
      })

      // Position each node at its target X with overlap prevention
      const positionedX: number[] = []
      sortedNodes.forEach((nodeId) => {
        let targetX = getTargetX(nodeId)

        // Prevent overlap with previously positioned nodes at this level
        for (const prevX of positionedX) {
          if (Math.abs(targetX - prevX) < nodeWidth + horizontalSpacing) {
            // Push right to avoid overlap
            targetX = prevX + nodeWidth + horizontalSpacing
          }
        }

        positionedX.push(targetX)
        newPositions.set(nodeId, { x: targetX, y })
      })

      // Record child order for next level
      sortedNodes.forEach((nodeId) => {
        const kids = children.get(nodeId) || []
        kids.forEach((kid, idx) => {
          if (!childOrderIndex.has(kid)) {
            childOrderIndex.set(kid, idx)
          }
        })
      })
    }
  }

  // === PASS 5: Handle orphan nodes ===

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

  // === PASS 6: Edge-aware obstacle resolution ===
  // Check all edges and move intermediate nodes that would be crossed by edges
  // This handles the case where a long edge (spanning multiple levels) passes through a node

  // Helper: Check if a vertical line segment intersects a rectangle
  const lineIntersectsNode = (
    lineX: number,
    lineY1: number,
    lineY2: number,
    nodeX: number,
    nodeY: number
  ): boolean => {
    const nodeLeft = nodeX - nodeWidth / 2 - horizontalSpacing / 4
    const nodeRight = nodeX + nodeWidth / 2 + horizontalSpacing / 4
    const nodeTop = nodeY
    const nodeBottom = nodeY + NODE_HEIGHT

    // Check if line X is within node's horizontal bounds
    if (lineX < nodeLeft || lineX > nodeRight) return false

    // Check if line's Y range overlaps with node's Y range
    const minY = Math.min(lineY1, lineY2)
    const maxY = Math.max(lineY1, lineY2)
    return !(maxY < nodeTop || minY > nodeBottom)
  }

  // Iterate multiple times to resolve cascading conflicts
  for (let iteration = 0; iteration < 3; iteration++) {
    let changed = false

    // Check each edge for intersections with non-connected nodes
    for (const edge of edges) {
      const sourcePos = newPositions.get(edge.source)
      const targetPos = newPositions.get(edge.target)
      if (!sourcePos || !targetPos) continue

      const sourceLevel = levels.get(edge.source) ?? 0
      const targetLevel = levels.get(edge.target) ?? 0
      const levelGap = targetLevel - sourceLevel

      // Only check edges that span more than 1 level (skip edges)
      if (levelGap <= 1) continue

      // Calculate edge X position (from source handle)
      const sourceNode = nodes.find((n) => n.id === edge.source)
      const handleOffset = getHandleOffsetX(sourceNode?.data.nodeType || 'trigger', edge.sourceHandle)
      const edgeX = sourcePos.x + handleOffset

      // Check all intermediate levels for obstacles
      for (let lvl = sourceLevel + 1; lvl < targetLevel; lvl++) {
        const nodesAtLevel = nodesByLevel.get(lvl) || []

        for (const intermediateId of nodesAtLevel) {
          // Skip if this node is connected to the edge
          if (intermediateId === edge.source || intermediateId === edge.target) continue

          const intermediatePos = newPositions.get(intermediateId)
          if (!intermediatePos) continue

          const levelY = startY + lvl * verticalSpacing

          // Check if edge would cross this node
          if (lineIntersectsNode(edgeX, sourcePos.y, targetPos.y, intermediatePos.x, levelY)) {
            // Move the intermediate node out of the way
            // Determine direction: move away from edge
            const nodeCenter = intermediatePos.x
            const moveDirection = edgeX <= nodeCenter ? 1 : -1 // Move right if edge is on left, vice versa
            const clearanceNeeded = nodeWidth / 2 + horizontalSpacing

            // Calculate new X position
            let newX: number
            if (moveDirection > 0) {
              // Move right: position node so its left edge clears the edge
              newX = edgeX + clearanceNeeded
            } else {
              // Move left: position node so its right edge clears the edge
              newX = edgeX - clearanceNeeded
            }

            // Only move if it's actually different
            if (Math.abs(newX - intermediatePos.x) > 10) {
              newPositions.set(intermediateId, { x: newX, y: intermediatePos.y })
              changed = true
            }
          }
        }
      }
    }

    // Also check for same-level overlaps after moving nodes
    for (let level = 1; level <= maxLevel; level++) {
      const nodesAtLevel = nodesByLevel.get(level) || []
      const positions = nodesAtLevel
        .map((id) => ({ id, x: newPositions.get(id)?.x ?? 0 }))
        .sort((a, b) => a.x - b.x)

      for (let i = 1; i < positions.length; i++) {
        const prev = positions[i - 1]
        const curr = positions[i]
        const minDistance = nodeWidth + horizontalSpacing

        if (curr.x - prev.x < minDistance) {
          // Push current node right
          const newX = prev.x + minDistance
          const currPos = newPositions.get(curr.id)
          if (currPos) {
            newPositions.set(curr.id, { x: newX, y: currPos.y })
            curr.x = newX // Update for next iteration
            changed = true
          }
        }
      }
    }

    if (!changed) break
  }

  // === Apply new positions ===
  // Convert from center coordinates to left-edge for ReactFlow

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
