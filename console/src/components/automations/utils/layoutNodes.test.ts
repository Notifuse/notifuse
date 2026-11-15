import { describe, it, expect } from 'vitest'
import { layoutNodes } from './layoutNodes'
import type { NodeType } from '../../../services/api/automation'

const node = (id: string, nodeType: NodeType, height?: number) => ({
  id,
  position: { x: 0, y: 0 },
  measured: height ? { width: 300, height } : undefined,
  data: { nodeType, config: {} as Record<string, unknown> }
})
const at = (laid: ReturnType<typeof layoutNodes<ReturnType<typeof node>>>, id: string) =>
  laid.find((n) => n.id === id)!.position

describe('layoutNodes', () => {
  it('starts each level 70px under the tallest step of the level above', () => {
    const laid = layoutNodes(
      [node('t', 'trigger', 120), node('d', 'delay', 90), node('e', 'email')],
      [
        { source: 't', target: 'd' },
        { source: 'd', target: 'e' }
      ]
    )
    expect(at(laid, 'd').y - at(laid, 't').y).toBe(120 + 70)
    expect(at(laid, 'e').y - at(laid, 'd').y).toBe(90 + 70)
  })

  it('leaves 110px under a branching step, for the shares on its links', () => {
    const laid = layoutNodes(
      [
        node('t', 'trigger', 100),
        node('ls', 'list_status_branch', 125),
        node('a', 'email', 100),
        node('b', 'email', 100)
      ],
      [
        { source: 't', target: 'ls' },
        { source: 'ls', target: 'a', sourceHandle: 'active' },
        { source: 'ls', target: 'b', sourceHandle: 'non_active' }
      ]
    )
    expect(at(laid, 'a').y - at(laid, 'ls').y).toBe(125 + 110)
    expect(at(laid, 'b').y).toBe(at(laid, 'a').y)
  })

  it('keeps steps of one level 80px apart, side by side', () => {
    const laid = layoutNodes(
      [node('t', 'trigger'), node('f', 'filter'), node('y', 'email'), node('n', 'email')],
      [
        { source: 't', target: 'f' },
        { source: 'f', target: 'y', sourceHandle: 'continue' },
        { source: 'f', target: 'n', sourceHandle: 'exit' }
      ]
    )
    expect(Math.abs(at(laid, 'n').x - at(laid, 'y').x)).toBeGreaterThanOrEqual(300 + 80)
  })

  it('lays out a flow whose link names a step missing from the canvas, without throwing', () => {
    const laid = layoutNodes(
      [node('t', 'trigger', 100), node('d', 'delay', 90)],
      [
        { source: 't', target: 'd' },
        { source: 'd', target: 'gone' }
      ]
    )
    expect(at(laid, 'd').y - at(laid, 't').y).toBe(100 + 70)
  })

  it('takes 86px for a step not measured yet', () => {
    const laid = layoutNodes([node('t', 'trigger'), node('d', 'delay')], [{ source: 't', target: 'd' }])
    expect(at(laid, 'd').y - at(laid, 't').y).toBe(86 + 70)
  })
})
