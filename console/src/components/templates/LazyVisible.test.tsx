import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { LazyVisible } from './LazyVisible'

describe('LazyVisible', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders its children at once without IntersectionObserver', () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    render(<LazyVisible>shown</LazyVisible>)
    expect(screen.getByText('shown')).toBeInTheDocument()
  })

  it('renders its children once they scroll into view', () => {
    let notify: (entries: { isIntersecting: boolean }[]) => void = () => undefined
    const disconnect = vi.fn()
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(callback: (entries: { isIntersecting: boolean }[]) => void) {
          notify = callback
        }
        observe() {}
        disconnect = disconnect
      }
    )
    render(<LazyVisible>shown</LazyVisible>)
    expect(screen.queryByText('shown')).toBeNull()
    act(() => notify([{ isIntersecting: false }]))
    expect(screen.queryByText('shown')).toBeNull()
    act(() => notify([{ isIntersecting: true }]))
    expect(screen.getByText('shown')).toBeInTheDocument()
    expect(disconnect).toHaveBeenCalled()
  })
})
