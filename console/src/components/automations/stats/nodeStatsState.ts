import { createContext, useContext } from 'react'
import type { NodeStats } from '../../../services/api/automation'

export type StatsPeriod = 'last30' | 'all'

/** The step results the editor holds; read by the cards and the edges, kept out of node data. */
export interface NodeStatsState {
  /** "With stats" is on: the cards and the edges show them. */
  enabled: boolean
  period: StatsPeriod
  loading: boolean
  error: boolean
  nodes?: Record<string, NodeStats>
  /** Steps as saved: an unsaved one has nothing to count yet. */
  savedNodeIds: Set<string>
  canReadHistory: boolean
  /** A draft no contact entered: the step panel shows no results block. */
  neverRan: boolean
}

export const NO_NODE_STATS: NodeStatsState = {
  enabled: false,
  period: 'last30',
  loading: false,
  error: false,
  savedNodeIds: new Set(),
  canReadHistory: false,
  neverRan: true
}

export const NodeStatsContext = createContext<NodeStatsState>(NO_NODE_STATS)

export const useNodeStatsState = () => useContext(NodeStatsContext)
