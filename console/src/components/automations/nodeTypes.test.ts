import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { ADDABLE_NODE_TYPES, NODE_TYPES, PALETTE_GROUPS } from './nodeTypes'
import type { NodeType } from '../../services/api/automation'

const ALL: Record<NodeType, true> = {
  trigger: true, delay: true, email: true, branch: true, filter: true, add_to_list: true,
  remove_from_list: true, ab_test: true, webhook: true, list_status_branch: true
}

describe('NODE_TYPES', () => {
  it('gives every node type an icon, a chip and a label', () => {
    for (const type of Object.keys(ALL) as NodeType[]) {
      expect(NODE_TYPES[type].icon).toBeTruthy()
      expect(NODE_TYPES[type].chip).toMatch(/^bg-\S+ text-\S+$/)
      expect(i18n._(NODE_TYPES[type].label)).not.toBe('')
    }
  })

  it('offers actions and logic in the palette, without trigger and branch', () => {
    expect(PALETTE_GROUPS.map((g) => g.key)).toEqual(['actions', 'logic'])
    expect(ADDABLE_NODE_TYPES).toEqual(['email', 'add_to_list', 'remove_from_list', 'webhook', 'delay', 'filter', 'list_status_branch', 'ab_test'])
  })
})
