import { describe, expect, it } from 'vitest'
import { clearHandleTarget } from './clearHandleTarget'

describe('clearHandleTarget', () => {
  it('clears the path of a branch, the default path included, and only that one', () => {
    const config = {
      default_path_id: 'otherwise',
      paths: [
        { id: 'vip', name: 'VIP', next_node_id: 'n2' },
        { id: 'otherwise', name: 'Otherwise', next_node_id: 'n3' }
      ]
    }
    expect(clearHandleTarget('branch', config, 'vip')).toEqual({
      default_path_id: 'otherwise',
      paths: [
        { id: 'vip', name: 'VIP', next_node_id: '' },
        { id: 'otherwise', name: 'Otherwise', next_node_id: 'n3' }
      ]
    })
    expect(clearHandleTarget('branch', config, 'otherwise')).toMatchObject({
      paths: [{ next_node_id: 'n2' }, { next_node_id: '' }]
    })
    expect(clearHandleTarget('branch', config, 'unknown')).toBeUndefined()
  })

  it('clears the variant of an A/B test', () => {
    const config = { variants: [{ id: 'a', next_node_id: 'n2' }, { id: 'b', next_node_id: 'n3' }] }
    expect(clearHandleTarget('ab_test', config, 'b')).toEqual({
      variants: [{ id: 'a', next_node_id: 'n2' }, { id: 'b', next_node_id: '' }]
    })
  })

  it('clears the continue or the exit of a filter', () => {
    const config = { continue_node_id: 'n2', exit_node_id: 'n3' }
    expect(clearHandleTarget('filter', config, 'continue')).toEqual({ continue_node_id: '', exit_node_id: 'n3' })
    expect(clearHandleTarget('filter', config, 'exit')).toEqual({ continue_node_id: 'n2', exit_node_id: '' })
  })

  it('clears each output of a list status branch', () => {
    const config = { list_id: 'l', not_in_list_node_id: 'a', active_node_id: 'b', non_active_node_id: 'c' }
    expect(clearHandleTarget('list_status_branch', config, 'not_in_list')).toMatchObject({ not_in_list_node_id: '', active_node_id: 'b' })
    expect(clearHandleTarget('list_status_branch', config, 'active')).toMatchObject({ active_node_id: '', non_active_node_id: 'c' })
    expect(clearHandleTarget('list_status_branch', config, 'non_active')).toMatchObject({ non_active_node_id: '', not_in_list_node_id: 'a' })
  })

  it('leaves a single-child node alone', () => {
    expect(clearHandleTarget('delay', { duration: 1 }, 'any')).toBeUndefined()
  })
})
