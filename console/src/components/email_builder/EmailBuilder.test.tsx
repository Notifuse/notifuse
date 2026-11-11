import React from 'react'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { act, render, waitFor, screen, fireEvent } from '@testing-library/react'
import EmailBuilder from './EmailBuilder'
import type { EmailBlock } from './types'

// Mock the heavy child panels so the test exercises only EmailBuilder's own
// compile orchestration (not Preview's iframe, the tree/edit/settings panels,
// or overlay scrollbars).
// The tree panel's props carry the editor's add and move handlers.
let mockTreePanelProps: Record<string, unknown> = {}
vi.mock('./panels/TreePanel', () => ({
  TreePanel: (p: Record<string, unknown>) => {
    mockTreePanelProps = p
    return null
  }
}))
vi.mock('./panels/EditPanel', () => ({ EditPanel: () => null }))
vi.mock('./panels/SettingsPanel', () => ({ SettingsPanel: () => null }))
vi.mock('./panels/Preview', () => ({
  // forwardRef so EmailBuilder passing templateDataRef doesn't warn
  Preview: React.forwardRef(() => null)
}))
vi.mock('overlayscrollbars-react', () => ({
  OverlayScrollbarsComponent: ({ children }: { children?: React.ReactNode }) => <>{children}</>
}))

// Minimal valid email tree; `marker` lets each version differ so JSON.stringify
// (the recompile key) changes between renders.
const makeTree = (marker: string): EmailBlock =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      { id: 'head', type: 'mj-head', children: [] },
      {
        id: 'body',
        type: 'mj-body',
        attributes: {},
        children: [{ id: 'text-1', type: 'mj-text', content: marker, children: [] }]
      }
    ]
  } as unknown as EmailBlock)

const props = (
  tree: EmailBlock,
  onCompile: Mock,
  forcedViewMode: 'edit' | 'preview' | null
) => ({
  tree,
  onTreeChange: vi.fn(),
  onCompile,
  testData: undefined,
  onTestDataChange: vi.fn(),
  onSaveBlock: vi.fn(),
  forcedViewMode
})

describe('EmailBuilder preview auto-recompile', () => {
  let onCompile: Mock

  beforeEach(() => {
    onCompile = vi.fn().mockResolvedValue({ html: '<html></html>', mjml: '<mjml></mjml>' })
  })

  it('recompiles the preview when the tree changes while in preview mode', async () => {
    const { rerender } = render(<EmailBuilder {...props(makeTree('one'), onCompile, 'preview')} />)
    // Flush the initial mount compile, then isolate the tree-change behavior.
    await waitFor(() => expect(onCompile).toHaveBeenCalled())
    onCompile.mockClear()

    const updated = makeTree('two')
    rerender(<EmailBuilder {...props(updated, onCompile, 'preview')} />)

    await waitFor(() => expect(onCompile).toHaveBeenCalledWith(updated, undefined))
  })

  it('does NOT recompile when the tree changes in edit mode', async () => {
    const { rerender } = render(<EmailBuilder {...props(makeTree('one'), onCompile, null)} />)
    // Edit mode must not compile on mount...
    expect(onCompile).not.toHaveBeenCalled()

    rerender(<EmailBuilder {...props(makeTree('two'), onCompile, null)} />)
    // ...nor after a tree change, even past the debounce window.
    await new Promise((resolve) => setTimeout(resolve, 600))
    expect(onCompile).not.toHaveBeenCalled()
  })

  it('coalesces a rapid burst of tree changes into a single recompile (debounce)', async () => {
    const { rerender } = render(<EmailBuilder {...props(makeTree('a'), onCompile, 'preview')} />)
    await waitFor(() => expect(onCompile).toHaveBeenCalled())
    onCompile.mockClear()

    // Three quick edits (mimics a multi-step AI burst) before the debounce fires.
    rerender(<EmailBuilder {...props(makeTree('b'), onCompile, 'preview')} />)
    rerender(<EmailBuilder {...props(makeTree('c'), onCompile, 'preview')} />)
    const last = makeTree('d')
    rerender(<EmailBuilder {...props(last, onCompile, 'preview')} />)

    await waitFor(() => expect(onCompile).toHaveBeenCalledTimes(1))
    expect(onCompile).toHaveBeenCalledWith(last, undefined)

    // No further compiles after the debounce settles.
    await new Promise((resolve) => setTimeout(resolve, 250))
    expect(onCompile).toHaveBeenCalledTimes(1)
  })
})

describe('EmailBuilder text version mode', () => {
  const textProps = (onCompile: Mock) => ({
    ...props(makeTree('one'), onCompile, null),
    textVersion: { enabled: true as const }
  })

  it('offers no text version mode unless asked to', () => {
    const onCompile = vi.fn()
    render(<EmailBuilder {...props(makeTree('one'), onCompile, null)} />)

    expect(screen.getByText('Edit')).toBeTruthy()
    expect(screen.getByText('Preview')).toBeTruthy()
    expect(screen.queryByText('Text version')).toBeNull()
  })

  it('compiles and shows the text version when that mode is selected', async () => {
    const onCompile = vi.fn().mockResolvedValue({
      html: '<p>Hello</p>',
      mjml: '<mjml></mjml>',
      errors: [],
      text: 'Hello Camille\n'
    })
    render(<EmailBuilder {...textProps(onCompile)} />)
    expect(onCompile).not.toHaveBeenCalled()
    expect((screen.getByTitle('Undo').closest('.ant-space') as HTMLElement).style.opacity).toBe('1')

    fireEvent.click(screen.getByText('Text version'))

    await waitFor(() => expect(screen.getByTestId('text-version').textContent).toBe('Hello Camille\n'))
    expect(onCompile).toHaveBeenCalled()
    // Undo and redo only apply to editing: their group is hidden in this mode. (A fresh
    // builder has no history, so the buttons' disabled state would prove nothing.)
    expect((screen.getByTitle('Undo').closest('.ant-space') as HTMLElement).style.opacity).toBe('0')
  })

  it('shows the compile error, not a stale text', async () => {
    const broken = makeTree('two')
    const onCompile = vi.fn(async (tree: EmailBlock) =>
      tree === broken
        ? { html: '', mjml: '', errors: [{ message: 'Broken template' }] }
        : { html: '<p>a</p>', mjml: '', errors: [], text: 'First text\n' }
    )
    const { rerender } = render(<EmailBuilder {...textProps(onCompile)} />)

    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() => expect(screen.getByTestId('text-version')).toBeTruthy())

    // An edit made outside the toolbar (the AI assistant) recompiles in this mode too.
    rerender(<EmailBuilder {...textProps(onCompile)} tree={broken} />)

    await waitFor(() => expect(screen.getByText('Broken template')).toBeTruthy())
    expect(screen.queryByText('First text')).toBeNull()
  })
})

describe('EmailBuilder text version mode with a custom text', () => {
  const customProps = (onCompile: Mock, value: string | undefined) => ({
    ...props(makeTree('one'), onCompile, null),
    textVersion: {
      enabled: true as const,
      custom: { value, onChange: vi.fn(), onSeed: vi.fn().mockResolvedValue({ text: '' }), category: 'marketing' }
    }
  })

  it('shows the text area while the email is still compiling', async () => {
    const onCompile = vi.fn(() => new Promise(() => {}))
    render(<EmailBuilder {...customProps(onCompile as unknown as Mock, 'Mine')} />)

    fireEvent.click(screen.getAllByText('Text version')[0])

    await waitFor(() => expect((screen.getByTestId('custom-text-input') as HTMLTextAreaElement).value).toBe('Mine'))
  })

  it('shows the text area when the email does not compile', async () => {
    const onCompile = vi.fn().mockResolvedValue({ html: '', mjml: '', errors: [{ message: 'Broken template' }] })
    render(<EmailBuilder {...customProps(onCompile, 'Mine')} />)

    fireEvent.click(screen.getAllByText('Text version')[0])

    await waitFor(() => expect(onCompile).toHaveBeenCalled())
    expect(screen.getByTestId('custom-text-input')).toBeTruthy()
  })

  it('shows the automatic text, not the custom one, after going back to Automatic', async () => {
    const onCompile = vi.fn().mockResolvedValue({ html: '<p>a</p>', mjml: '', errors: [], text: 'Automatic text' })
    const { rerender } = render(<EmailBuilder {...customProps(onCompile, 'Mine')} />)
    fireEvent.click(screen.getAllByText('Text version')[0])
    await waitFor(() => expect(screen.getByTestId('custom-text-input')).toBeTruthy())

    rerender(<EmailBuilder {...customProps(onCompile, undefined)} />)

    await waitFor(() => expect(screen.getByTestId('text-version').textContent).toBe('Automatic text'))
    expect(screen.queryByText('Mine')).toBeNull()
  })
})

// A condition hides its block with every mj-liquid in it, which can hold half of a tag
// pair, so the server refuses the tree: the editor must not build it.
describe('EmailBuilder Liquid blocks under a display condition', () => {
  const conditionedTree = (): EmailBlock =>
    ({
      id: 'root',
      type: 'mjml',
      children: [
        {
          id: 'body',
          type: 'mj-body',
          attributes: {},
          children: [
            {
              id: 'sec-1',
              type: 'mj-section',
              display_condition: { match: 'all', rules: [{ variable: 'contact.vip', operator: 'is_true' }] },
              children: [{ id: 'col-1', type: 'mj-column', children: [] }]
            },
            { id: 'liq-1', type: 'mj-liquid', content: '{% endif %}' }
          ]
        }
      ]
    }) as unknown as EmailBlock

  const renderBuilder = () => {
    const onTreeChange = vi.fn()
    render(
      <EmailBuilder
        {...props(conditionedTree(), vi.fn().mockResolvedValue({ html: '', mjml: '' }), 'edit')}
        onTreeChange={onTreeChange}
      />
    )
    return onTreeChange
  }

  it('refuses to add a Liquid block inside the block with the condition', () => {
    const onTreeChange = renderBuilder()
    const add = mockTreePanelProps.onAddBlock as (parentId: string, type: string, position?: number) => void
    act(() => add('col-1', 'mj-liquid', 0))
    expect(onTreeChange).not.toHaveBeenCalled()
    act(() => add('col-1', 'mj-text', 0))
    expect(onTreeChange).toHaveBeenCalledTimes(1)
  })

  it('refuses to move a Liquid block into it', () => {
    const onTreeChange = renderBuilder()
    const move = mockTreePanelProps.onMoveBlock as (id: string, parentId: string, position: number) => void
    act(() => move('liq-1', 'col-1', 0))
    expect(onTreeChange).not.toHaveBeenCalled()
  })
})
