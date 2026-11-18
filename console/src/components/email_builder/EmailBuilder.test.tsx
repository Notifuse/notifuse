import React, { useState } from 'react'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { act, render as rtlRender, waitFor, screen, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import EmailBuilder from './EmailBuilder'

// The editor shows its messages through antd's App
const render = (ui: React.ReactElement) => rtlRender(ui, { wrapper: App })
import type {
  DeleteSavedBlockHandler,
  EmailBlock,
  RenameSavedBlockHandler,
  SaveBlockHandler,
  SavedBlock
} from './types'
import { EmailBlockClass } from './EmailBlockClass'

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
// The canvas's props carry the editor's update handler and its save dialog opener.
let mockEditPanelProps: Record<string, unknown> = {}
vi.mock('./panels/EditPanel', () => ({
  EditPanel: (p: Record<string, unknown>) => {
    mockEditPanelProps = p
    return null
  }
}))
// The settings panel's props carry the editor's attribute writer.
let mockSettingsPanelProps: Record<string, unknown> = {}
vi.mock('./panels/SettingsPanel', () => ({
  SettingsPanel: (p: Record<string, unknown>) => {
    mockSettingsPanelProps = p
    return null
  }
}))
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
    expect((screen.getByTitle('Undo').parentElement as HTMLElement).style.opacity).toBe('1')

    fireEvent.click(screen.getByText('Text version'))

    await waitFor(() => expect(screen.getByTestId('text-version').textContent).toBe('Hello Camille\n'))
    expect(onCompile).toHaveBeenCalled()
    // Undo and redo only apply to editing: their group is hidden in this mode. (A fresh
    // builder has no history, so the buttons' disabled state would prove nothing.)
    expect((screen.getByTitle('Undo').parentElement as HTMLElement).style.opacity).toBe('0')
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

describe('EmailBuilder moving and changing columns', () => {
  interface Node {
    id: string
    type: string
    attributes: Record<string, unknown>
    content?: string
    children?: Node[]
  }
  const col = (id: string, width: string): Node => ({
    id,
    type: 'mj-column',
    attributes: { width },
    children: [{ id: `${id}-text`, type: 'mj-text', attributes: {}, content: `<p>${id}</p>` }]
  })
  const section = (id: string, children: Node[]): Node => ({ id, type: 'mj-section', attributes: {}, children })

  // SA: 20/30/50, SB: 30/70, SC holds a 30/70 group, SD two columns and a group
  const makeColumnsTree = () =>
    ({
      id: 'root',
      type: 'mjml',
      children: [
        { id: 'head', type: 'mj-head', children: [] },
        {
          id: 'body',
          type: 'mj-body',
          attributes: {},
          children: [
            section('SA', [col('a1', '20%'), col('a2', '30%'), col('a3', '50%')]),
            section('SB', [col('b1', '30%'), col('b2', '70%')]),
            section('SC', [
              { id: 'G', type: 'mj-group', attributes: {}, children: [col('g1', '30%'), col('g2', '70%')] }
            ]),
            // SD: a 30/70 section that made room for a width-less group
            section('SD', [
              col('d1', '20%'),
              col('d2', '46.67%'),
              { id: 'G2', type: 'mj-group', attributes: {}, children: [col('h1', '50%'), col('h2', '50%')] }
            ])
          ]
        }
      ]
    }) as unknown as EmailBlock

  const setup = (initialTree: EmailBlock = makeColumnsTree()) => {
    mockTreePanelProps = {}
    const onTreeChange = vi.fn()
    render(<EmailBuilder {...props(initialTree, vi.fn(), null)} onTreeChange={onTreeChange} />)
    const call = (name: string, ...args: unknown[]) =>
      act(() => {
        (mockTreePanelProps[name] as (...a: unknown[]) => void)(...args)
      })
    const latest = () => onTreeChange.mock.calls[onTreeChange.mock.calls.length - 1][0] as EmailBlock
    const children = (id: string) => EmailBlockClass.findBlockById(latest(), id)!.children!
    const widths = (id: string) =>
      children(id).map((child) => (child.attributes as Record<string, unknown>).width)
    return { onTreeChange, call, children, widths }
  }

  it("gives the settings panel's header the canvas actions: clone, delete, save, select", () => {
    const { call, children } = setup()
    call('onSelectBlock', 'a1-text')
    const header = (name: string, ...args: unknown[]) =>
      act(() => {
        (mockSettingsPanelProps[name] as (...a: unknown[]) => void)(...args)
      })
    // The tree is the test's prop, which onTreeChange does not update: each action starts from it
    header('onCloneBlock', 'a1-text')
    expect(children('a1')).toHaveLength(2)
    header('onDeleteBlock', 'a1-text')
    expect(children('a1')).toHaveLength(0)
    header('onSelectBlock', 'SA')
    expect((mockSettingsPanelProps.selectedBlock as EmailBlock).id).toBe('SA')
    header('onSaveBlock', 'SA')
    expect(screen.getByRole('dialog', { name: 'Save to library' })).toBeInTheDocument()
  })

  it('adds no undo step for a move that changes nothing', () => {
    const { onTreeChange, call } = setup()
    call('onMoveBlock', 'a1', 'SA', 0)
    expect(onTreeChange).not.toHaveBeenCalled()
    expect(screen.getByTitle('Undo').closest('button')).toBeDisabled()
  })

  it('keeps proportions on both sides when a column moves to another section', () => {
    const { call, widths, children } = setup()
    call('onMoveBlock', 'a3', 'SB', 2)
    expect(widths('SA')).toEqual(['40%', '60%'])
    expect(children('SB').map((c) => c.id)).toEqual(['b1', 'b2', 'a3'])
    expect(widths('SB')).toEqual(['20%', '46.67%', '33.33%'])
  })

  it('keeps the widths of columns reordered inside a group', () => {
    const { call, widths, children } = setup()
    call('onMoveBlock', 'g2', 'G', 0)
    expect(children('G').map((c) => c.id)).toEqual(['g2', 'g1'])
    expect(widths('G')).toEqual(['70%', '30%'])
  })

  it('keeps proportions when a column is added, with its default text', () => {
    const { call, widths, children } = setup()
    call('onAddBlock', 'SB', 'mj-column', 2)
    expect(widths('SB')).toEqual(['20%', '46.67%', '33.33%'])
    expect(JSON.stringify(children('SB')[2].children)).toContain('Column 3')
  })

  it('keeps proportions when a column is duplicated or deleted', () => {
    const duplicated = setup()
    duplicated.call('onCloneBlock', 'b1')
    expect(duplicated.widths('SB')).toEqual(['20%', '33.33%', '46.67%'])

    const deleted = setup()
    deleted.call('onDeleteBlock', 'a3')
    expect(deleted.widths('SA')).toEqual(['40%', '60%'])
  })

  it('gives a saved column inserted into a section its share', () => {
    const { call, widths } = setup()
    call('onAddSavedBlock', 'SB', col('saved', '25%'), 2)
    expect(widths('SB')).toEqual(['20%', '46.67%', '33.33%'])
  })

  it('makes room for a group added to a section, and gives it back when the group is deleted', () => {
    const added = setup()
    added.call('onAddBlock', 'SB', 'mj-group', 2)
    expect(added.widths('SB')).toEqual(['20%', '46.67%', undefined])

    const deleted = setup()
    deleted.call('onDeleteBlock', 'G2')
    expect(deleted.widths('SD')).toEqual(['30%', '70%'])
  })

  it('makes room for a group moved, duplicated or inserted from the saved blocks', () => {
    const moved = setup()
    moved.call('onMoveBlock', 'G', 'SB', 2)
    expect(moved.children('SB').map((c) => c.id)).toEqual(['b1', 'b2', 'G'])
    expect(moved.widths('SB')).toEqual(['20%', '46.67%', undefined])
    // The group's own columns keep their widths
    expect(moved.widths('G')).toEqual(['30%', '70%'])

    const duplicated = setup()
    duplicated.call('onCloneBlock', 'G2')
    expect(duplicated.widths('SD')).toEqual(['15%', '35%', undefined, undefined])

    const saved = setup()
    saved.call('onAddSavedBlock', 'SB', { id: 'sg', type: 'mj-group', attributes: {}, children: [] }, 0)
    expect(saved.widths('SB')).toEqual([undefined, '20%', '46.67%'])
  })

  it("counts a group at the width the template's default attributes give it", () => {
    const tree = makeColumnsTree() as unknown as { children: Node[] }
    tree.children[0].children = [
      {
        id: 'attrs',
        type: 'mj-attributes',
        attributes: {},
        children: [{ id: 'group-defaults', type: 'mj-group', attributes: { width: '40%' } }]
      }
    ]
    const { call, widths } = setup(tree as unknown as EmailBlock)
    call('onMoveBlock', 'G', 'SB', 2)
    expect(widths('SB')).toEqual(['18%', '42%', undefined])
  })

  // Characterisation: the AI assistant moves blocks through moveBlockInTree with a final index
  it('moveBlockInTree puts the block at the final index it is given', () => {
    const moved = EmailBlockClass.moveBlockInTree(makeColumnsTree(), 'a1', 'SA', 2)!
    expect(EmailBlockClass.findBlockById(moved, 'SA')!.children!.map((c) => c.id)).toEqual(['a2', 'a3', 'a1'])
  })
})

describe('EmailBuilder library', () => {
  const blk = (id: string, type: string, children?: EmailBlock[], extra: Record<string, unknown> = {}) =>
    ({ id, type, attributes: {}, ...(children ? { children } : {}), ...extra }) as unknown as EmailBlock

  const libraryTree = () =>
    blk('root', 'mjml', [
      blk('head', 'mj-head', []),
      blk('body', 'mj-body', [
        blk('section-1', 'mj-section', [blk('column-1', 'mj-column', [blk('text-1', 'mj-text', undefined, { content: '<p>Hi</p>' })])]),
        blk('section-2', 'mj-section', [blk('column-2', 'mj-column', [])])
      ])
    ])

  const ctaBlock: SavedBlock = {
    id: 'cta',
    name: 'CTA',
    block: blk('saved-button', 'mj-button', undefined, { content: 'Shop' }),
    created: '2026-10-01T00:00:00Z'
  }
  const headerBlock: SavedBlock = {
    id: 'header',
    name: 'Header',
    block: blk('saved-section', 'mj-section', [blk('saved-col', 'mj-column', [])]),
    created: '2026-09-01T00:00:00Z'
  }

  // Enough more saved blocks for the library to show its search, matched by none of the searches below
  const moreBlocks: SavedBlock[] = Array.from({ length: 9 }, (_, i) => ({
    id: `block-${i + 1}`,
    name: `Block ${i + 1}`,
    block: blk(`saved-text-${i + 1}`, 'mj-text', undefined, { content: 'Text' }),
    created: '2020-01-01T00:00:00Z'
  }))

  // Holds the tree, the selection and the saved blocks the way the template drawer does
  let current: { tree: EmailBlock; selected: string | null; onSelectBlock: Mock }
  function Drawer({
    onSaveBlock,
    initialTree = libraryTree,
    canManage = false,
    onRename,
    onDelete,
    otherSavedBlocks = []
  }: {
    onSaveBlock?: (add: (saved: SavedBlock) => void) => SaveBlockHandler
    initialTree?: () => EmailBlock
    canManage?: boolean
    onRename?: RenameSavedBlockHandler
    onDelete?: DeleteSavedBlockHandler
    otherSavedBlocks?: SavedBlock[]
  }) {
    const [tree, setTree] = useState(initialTree)
    const [selected, setSelected] = useState<string | null>(null)
    const [savedBlocks, setSavedBlocks] = useState<SavedBlock[]>([ctaBlock, headerBlock, ...otherSavedBlocks])
    const onSelectBlock = React.useMemo(() => vi.fn((id: string | null) => setSelected(id)), [])
    React.useEffect(() => {
      current = { tree, selected, onSelectBlock }
    })
    const add = (saved: SavedBlock) => setSavedBlocks((prev) => [...prev, saved])
    return (
      <App>
        <EmailBuilder
          tree={tree}
          onTreeChange={setTree}
          onCompile={vi.fn()}
          onTestDataChange={vi.fn()}
          onSaveBlock={onSaveBlock ? onSaveBlock(add) : vi.fn()}
          savedBlocks={savedBlocks}
          selectedBlockId={selected}
          onSelectBlock={onSelectBlock}
          forcedViewMode={null}
          canManageSavedBlocks={canManage}
          onRenameSavedBlock={onRename}
          onDeleteSavedBlock={onDelete}
        />
      </App>
    )
  }

  const childIds = (id: string) =>
    (EmailBlockClass.findBlockById(current.tree, id)?.children ?? []).map((child) => child.id)
  const select = (id: string | null) => act(() => (mockTreePanelProps.onSelectBlock as (id: string | null) => void)(id))
  const openLibrary = () => fireEvent.click(screen.getByTestId('library-tab'))
  const card = (key: string) => document.querySelector(`[data-library-card="${key}"]`) as HTMLElement
  const insertCard = (key: string) => fireEvent.click(within(card(key)).getByRole('button', { name: 'Insert' }))
  const tile = (type: string) => document.querySelector(`[data-library-tile="${type}"]`) as HTMLElement
  const undoLink = () => screen.getByTestId('library-insert-undo')
  const successToasts = () => document.querySelectorAll('.ant-message-notice-success')

  beforeEach(() => {
    mockTreePanelProps = {}
    mockEditPanelProps = {}
    document.body.innerHTML = ''
  })

  it('opens on the structure, with no count on the library tab', () => {
    render(<Drawer />)
    expect(screen.getByTestId('library-tab')).toHaveTextContent(/^Library$/)
    expect(mockTreePanelProps.emailTree).toBeDefined()
    expect(screen.queryByTestId('library-panel')).not.toBeInTheDocument()

    openLibrary()
    expect(screen.getByTestId('library-panel')).toBeInTheDocument()
  })

  it('opens the structure when a block is selected on the canvas, keeping the selection', () => {
    render(<Drawer />)
    openLibrary()
    expect(screen.getByTestId('library-panel')).toBeInTheDocument()

    act(() => (mockEditPanelProps.onSelectBlock as (id: string | null) => void)('text-1'))

    expect(screen.queryByTestId('library-panel')).not.toBeInTheDocument()
    expect(mockTreePanelProps.selectedBlockId).toBe('text-1')
    expect(current.selected).toBe('text-1')
  })

  it('stays on the library when the canvas clears the selection', () => {
    render(<Drawer />)
    select('text-1')
    openLibrary()

    act(() => (mockEditPanelProps.onSelectBlock as (id: string | null) => void)(null))

    expect(screen.getByTestId('library-panel')).toBeInTheDocument()
  })

  it('stays on the library when it inserts a block, which selects it', () => {
    render(<Drawer />)
    select('text-1')
    openLibrary()
    insertCard('cta')

    expect(current.selected).not.toBe('text-1')
    expect(screen.getByTestId('library-panel')).toBeInTheDocument()
  })

  it('selects the inserted block through the selection the drawer holds', () => {
    render(<Drawer />)
    select('text-1')
    openLibrary()
    insertCard('cta')

    const [, inserted] = childIds('column-1')
    expect(inserted).toBeDefined()
    expect(inserted).not.toBe('saved-button')
    expect(current.onSelectBlock).toHaveBeenLastCalledWith(inserted)
    expect(current.selected).toBe(inserted)
  })

  it('keeps the order of two insertions in a row', () => {
    render(<Drawer />)
    select('text-1')
    openLibrary()
    insertCard('cta')
    const first = current.selected
    insertCard('cta')
    const second = current.selected

    expect(childIds('column-1')).toEqual(['text-1', first, second])
  })

  it('undoes only the insertion from its message, and clears the selection', async () => {
    render(<Drawer />)
    select('text-1')
    openLibrary()
    insertCard('cta')
    expect(childIds('column-1')).toHaveLength(2)
    expect(successToasts()).toHaveLength(1)

    fireEvent.click(undoLink())

    expect(childIds('column-1')).toEqual(['text-1'])
    expect(current.selected).toBeNull()
  })

  it('a refused tile neither inserts its block nor shows a message', () => {
    render(<Drawer />)
    openLibrary()
    // A text needs a column, and nothing is selected
    expect(tile('mj-text')).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(tile('mj-text'))

    expect(successToasts()).toHaveLength(0)
    expect(childIds('body')).toEqual(['section-1', 'section-2'])
  })

  it('inserts an empty wrapper where it is put, leaving the sections as they are', () => {
    render(<Drawer />)
    select('section-1')
    openLibrary()
    fireEvent.click(tile('mj-wrapper'))

    const ids = childIds('body')
    expect(ids).toHaveLength(3)
    expect(ids[0]).toBe('section-1')
    expect(ids[2]).toBe('section-2')
    const wrapper = EmailBlockClass.findBlockById(current.tree, ids[1])!
    expect(wrapper.type).toBe('mj-wrapper')
    expect(wrapper.children ?? []).toHaveLength(0)
  })

  it('gives a section inserted into a wrapper from the library its default column', () => {
    const wrapped = () =>
      blk('root', 'mjml', [
        blk('head', 'mj-head', []),
        blk('body', 'mj-body', [
          blk('wrapper-1', 'mj-wrapper', [blk('section-w', 'mj-section', [blk('column-w', 'mj-column', [])])])
        ])
      ])
    render(<Drawer initialTree={wrapped} />)
    select('column-w')
    openLibrary()
    // After the selected column's section, in the wrapper
    fireEvent.click(tile('mj-section'))

    const [, added] = EmailBlockClass.findBlockById(current.tree, 'wrapper-1')!.children!
    expect(added.type).toBe('mj-section')
    expect(added.children).toHaveLength(1)
    expect(added.children![0].type).toBe('mj-column')
  })

  it("keeps the tree's \"+\": a first wrapper takes the body's sections", () => {
    render(<Drawer />)
    act(() => (mockTreePanelProps.onAddBlock as (p: string, t: string, i?: number) => void)('body', 'mj-wrapper', 0))

    const ids = childIds('body')
    expect(ids).toHaveLength(1)
    const wrapper = EmailBlockClass.findBlockById(current.tree, ids[0])!
    expect(wrapper.type).toBe('mj-wrapper')
    expect(wrapper.children).toHaveLength(2)
  })

  it("undoes the insertion and keeps the edit made before it, and Redo puts the insertion back", () => {
    render(<Drawer />)
    const text = EmailBlockClass.findBlockById(current.tree, 'text-1')!
    act(() =>
      (mockEditPanelProps.onUpdateBlock as (id: string, block: EmailBlock) => void)('text-1', {
        ...text,
        content: '<p>Edited</p>'
      } as EmailBlock)
    )
    openLibrary()
    insertCard('header')
    expect(childIds('body')).toHaveLength(3)

    fireEvent.click(undoLink())
    expect(childIds('body')).toEqual(['section-1', 'section-2'])
    expect((EmailBlockClass.findBlockById(current.tree, 'text-1') as { content?: string }).content).toBe(
      '<p>Edited</p>'
    )

    fireEvent.click(screen.getByTitle('Redo').closest('button')!)
    expect(childIds('body')).toHaveLength(3)
  })

  it('undoes only the insertion after a coalesced write, and keeps the write', () => {
    render(<Drawer />)
    const write = (value: string) =>
      act(() =>
        (mockSettingsPanelProps.onUpdateAttributes as (id: string, u: Record<string, unknown>, o?: { coalesceKey?: string }) => void)(
          'text-1',
          { paddingTop: value },
          { coalesceKey: 'text-1:paddingTop:1' }
        )
      )
    write('1px')
    write('12px')
    openLibrary()
    insertCard('header')
    expect(childIds('body')).toHaveLength(3)

    fireEvent.click(undoLink())
    expect(childIds('body')).toEqual(['section-1', 'section-2'])
    expect(EmailBlockClass.findBlockById(current.tree, 'text-1')!.attributes).toMatchObject({ paddingTop: '12px' })
  })

  it('changes nothing from the message once another change followed the insertion', async () => {
    render(<Drawer />)
    openLibrary()
    insertCard('header')
    const undo = undoLink()
    const text = EmailBlockClass.findBlockById(current.tree, 'text-1')!
    act(() =>
      (mockEditPanelProps.onUpdateBlock as (id: string, block: EmailBlock) => void)('text-1', {
        ...text,
        content: '<p>Edited</p>'
      } as EmailBlock)
    )

    fireEvent.click(undo)
    expect(childIds('body')).toHaveLength(3)
    expect((EmailBlockClass.findBlockById(current.tree, 'text-1') as { content?: string }).content).toBe(
      '<p>Edited</p>'
    )
    await waitFor(() => expect(screen.queryByTestId('library-insert-undo')).not.toBeInTheDocument())
  })

  it('shows a block just saved in the library, search and filter cleared, tagged "New"', async () => {
    const created: SavedBlock = {
      id: 'new-id',
      name: 'Products',
      block: blk('saved-products', 'mj-section', []),
      created: '2026-11-11T00:00:00Z'
    }
    render(
      <Drawer
        otherSavedBlocks={moreBlocks}
        onSaveBlock={(add) => async () => {
          add(created)
          return 'new-id'
        }}
      />
    )
    openLibrary()
    await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'nothing like it')
    act(() => (mockEditPanelProps.onOpenSaveDialog as (id: string) => void)('section-1'))
    await userEvent.click(await screen.findByRole('button', { name: 'Save' }))

    await waitFor(() => expect(card('new-id')).not.toBeNull())
    expect(screen.getByPlaceholderText('Search saved blocks')).toHaveValue('')
    expect(within(card('new-id')).getByText('New')).toBeInTheDocument()

    // Until the tab changes
    fireEvent.click(screen.getByText('Structure'))
    openLibrary()
    expect(screen.queryByText('New')).not.toBeInTheDocument()
  })

  it('tags nothing "New" after replacing a saved block', async () => {
    render(<Drawer onSaveBlock={() => async () => 'header'} />)
    act(() => (mockEditPanelProps.onOpenSaveDialog as (id: string) => void)('section-1'))
    await userEvent.click(await screen.findByRole('radio', { name: /Replace an existing block/ }))
    await userEvent.click(screen.getByRole('combobox'))
    const option = await screen.findAllByText('Header', { selector: '.ant-select-item-option-content' })
    await userEvent.click(option[option.length - 1])
    await userEvent.click(screen.getByRole('button', { name: 'Replace' }))

    await waitFor(() => expect((document.querySelector('.ant-modal-wrap') as HTMLElement).style.display).toBe('none'))
    expect(screen.queryByTestId('library-panel')).not.toBeInTheDocument()
    openLibrary()
    expect(screen.queryByText('New')).not.toBeInTheDocument()
  })

  describe('managing saved blocks', () => {
    const menuOf = async (key: string, name: string) => {
      await userEvent.click(
        within(card(key)).getByRole('button', { name: `More actions for ${name}` })
      )
    }
    const menuItem = (name: string) =>
      screen.getAllByRole('menuitem', { name: new RegExp(name) }).at(-1)!
    const confirmButton = async (name: string) =>
      (await screen.findAllByRole('button', { name })).at(-1)!

    it('shows no menu unless the member may change the library', () => {
      render(<Drawer />)
      openLibrary()
      expect(screen.queryByRole('button', { name: /More actions/ })).toBeNull()
    })

    it('duplicates a block as "(copy)" and tags the copy "New", keeping the search', async () => {
      const handler = vi.fn(async (block: EmailBlock, _op: string, name: string) => {
        addSaved({ id: 'cta-copy', name, block, created: '2026-11-01T00:00:00Z' })
        return 'cta-copy'
      })
      let addSaved: (saved: SavedBlock) => void = () => {}
      render(
        <Drawer
          canManage
          otherSavedBlocks={moreBlocks}
          onSaveBlock={(add) => {
            addSaved = add
            return handler as unknown as SaveBlockHandler
          }}
        />
      )
      openLibrary()
      await userEvent.type(screen.getByPlaceholderText('Search saved blocks'), 'CTA')
      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Duplicate'))

      await waitFor(() => expect(card('cta-copy')).not.toBeNull())
      expect(handler).toHaveBeenCalledWith(ctaBlock.block, 'create', 'CTA (copy)')
      expect(within(card('cta-copy')).getByText('New')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('Search saved blocks')).toHaveValue('CTA')
    })

    it('replaces a saved block with the selected one after confirming, without a "New" tag', async () => {
      const handler = vi.fn(async () => 'header')
      render(<Drawer canManage onSaveBlock={() => handler as unknown as SaveBlockHandler} />)
      select('section-2')
      openLibrary()
      await menuOf('header', 'Header')
      await userEvent.click(menuItem('Replace with the selected block'))
      // The dialog names its title for assistive technology too
      await screen.findAllByText('Replace "Header" with the selected block?')
      expect(document.querySelectorAll('.ant-modal-confirm')).toHaveLength(1)
      await userEvent.click(await confirmButton('Replace'))

      await waitFor(() => expect(handler).toHaveBeenCalledTimes(1))
      const [block, operation, id] = handler.mock.calls[0] as unknown as [
        EmailBlock,
        string,
        string
      ]
      expect([block.id, operation, id]).toEqual(['section-2', 'update', 'header'])
      await waitFor(() => expect(card('header').className).toContain('border-primary-500'))
      expect(within(card('header')).queryByText('New')).toBeNull()
    })

    it('deletes a saved block after confirming, and nothing when cancelled', async () => {
      const onDelete = vi.fn(async () => {})
      render(<Drawer canManage onDelete={onDelete} />)
      openLibrary()
      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Delete'))
      await screen.findAllByText('Delete "CTA" from the library?')
      await userEvent.click(await confirmButton('Cancel'))
      expect(onDelete).not.toHaveBeenCalled()

      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Delete'))
      await userEvent.click(await confirmButton('Delete'))
      await waitFor(() => expect(onDelete).toHaveBeenCalledWith('cta'))
    })

    it('renames a saved block', async () => {
      const onRename = vi.fn(async () => {})
      render(<Drawer canManage onRename={onRename} />)
      openLibrary()
      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Rename'))
      const input = await screen.findByRole('textbox', { name: 'Block name' })
      await userEvent.clear(input)
      await userEvent.type(input, 'Buy now{Enter}')
      await waitFor(() => expect(onRename).toHaveBeenCalledWith('cta', 'Buy now'))
    })

    it('runs one library change at a time', async () => {
      let finish: () => void = () => {}
      const onDelete = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)))
      render(<Drawer canManage onDelete={onDelete} />)
      openLibrary()
      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Delete'))
      await userEvent.click(await confirmButton('Delete'))
      await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1))

      await menuOf('header', 'Header')
      expect(menuItem('Duplicate')).toHaveAttribute('aria-disabled', 'true')
      await act(async () => finish())
      await menuOf('header', 'Header')
      await waitFor(() =>
        expect(menuItem('Duplicate')).not.toHaveAttribute('aria-disabled', 'true')
      )
    })

    it('lets the next change run after one failed', async () => {
      const onDelete = vi.fn(async () => {
        throw new Error('Server down')
      })
      render(<Drawer canManage onDelete={onDelete} />)
      openLibrary()
      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Delete'))
      await userEvent.click(await confirmButton('Delete'))
      await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1))
      await menuOf('header', 'Header')
      await waitFor(() =>
        expect(menuItem('Duplicate')).not.toHaveAttribute('aria-disabled', 'true')
      )
    })

    it('holds the save dialog while another library change runs', async () => {
      let finish: () => void = () => {}
      const onDelete = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)))
      const save = vi.fn(async () => 'new-id')
      render(<Drawer canManage onDelete={onDelete} onSaveBlock={() => save} />)
      openLibrary()
      await menuOf('cta', 'CTA')
      await userEvent.click(menuItem('Delete'))
      await userEvent.click(await confirmButton('Delete'))
      await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1))

      act(() => (mockEditPanelProps.onOpenSaveDialog as (id: string) => void)('section-1'))
      // Save waits for the running change, Enter in the name included
      const saveButton = await screen.findByRole('button', { name: /Save/ })
      expect(saveButton).toBeDisabled()
      fireEvent.keyDown(screen.getByPlaceholderText('Enter a name for this block'), { key: 'Enter', keyCode: 13 })
      expect(save).not.toHaveBeenCalled()

      await act(async () => finish())
      await waitFor(() => expect(screen.getByRole('button', { name: /Save/ })).toBeEnabled())
      await userEvent.click(screen.getByRole('button', { name: /Save/ }))
      await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
    })
  })
})

describe('EmailBuilder heading styles', () => {
  let latestTree: EmailBlock
  const Controlled = ({ initial }: { initial: EmailBlock }) => {
    const [tree, setTree] = useState(initial)
    React.useEffect(() => {
      latestTree = tree
    })
    return <EmailBuilder {...props(tree, vi.fn(), null)} onTreeChange={setTree} />
  }
  const treeWithText = (content: string) =>
    ({
      id: 'root',
      type: 'mjml',
      children: [
        { id: 'head', type: 'mj-head', children: [] },
        {
          id: 'body',
          type: 'mj-body',
          attributes: {},
          children: [{ id: 'text-1', type: 'mj-text', attributes: {}, content }]
        }
      ]
    }) as unknown as EmailBlock
  const update = (changes: Record<string, unknown>) => {
    const text = EmailBlockClass.findBlockById(latestTree, 'text-1')!
    act(() =>
      (mockEditPanelProps.onUpdateBlock as (id: string, block: EmailBlock) => void)('text-1', {
        ...text,
        ...changes
      } as EmailBlock)
    )
  }
  const managedStyles = () =>
    (EmailBlockClass.findBlockById(latestTree, 'head')?.children ?? []).filter((child) =>
      ((child as { content?: string }).content ?? '').startsWith('/* notifuse:headings')
    )

  it('adds the heading styles with the first heading, in the same undo step', () => {
    render(<Controlled initial={treeWithText('<p>Title</p>')} />)
    update({ content: '<h2>Title</h2>' })
    expect(managedStyles()).toHaveLength(1)

    fireEvent.click(screen.getByTitle('Undo').closest('button')!)
    expect((EmailBlockClass.findBlockById(latestTree, 'text-1') as { content?: string }).content).toBe('<p>Title</p>')
    expect(managedStyles()).toHaveLength(0)
  })

  it('adds nothing to an email without headings', () => {
    render(<Controlled initial={treeWithText('<p>Title</p>')} />)
    update({ content: '<p>Other</p>' })
    expect(managedStyles()).toHaveLength(0)
  })

  it('adds nothing to an email that already had headings, on an unrelated edit', () => {
    render(<Controlled initial={treeWithText('<h1>Title</h1>')} />)
    update({ attributes: { paddingTop: '20px' } })
    expect(managedStyles()).toHaveLength(0)
  })
})

describe('EmailBuilder settings writes and undo steps', () => {
  let latestTree: EmailBlock
  let setOutside: (tree: EmailBlock) => void = () => {}
  const Controlled = ({ initial }: { initial: EmailBlock }) => {
    const [tree, setTree] = useState(initial)
    React.useEffect(() => {
      latestTree = tree
    })
    React.useEffect(() => {
      setOutside = (next) => setTree(next)
    }, [])
    return <EmailBuilder {...props(tree, vi.fn(), null)} onTreeChange={setTree} />
  }
  const initial = () =>
    ({
      id: 'root',
      type: 'mjml',
      children: [
        { id: 'head', type: 'mj-head', children: [] },
        {
          id: 'body',
          type: 'mj-body',
          attributes: {},
          children: [{ id: 'text-1', type: 'mj-text', attributes: { color: '#111111' }, content: '<p>Hi</p>' }]
        }
      ]
    }) as unknown as EmailBlock
  const write = (updates: Record<string, unknown>, coalesceKey?: string, blockId = 'text-1') =>
    act(() =>
      (mockSettingsPanelProps.onUpdateAttributes as (id: string, u: Record<string, unknown>, o?: { coalesceKey?: string }) => void)(
        blockId,
        updates,
        coalesceKey ? { coalesceKey } : undefined
      )
    )
  const attrs = () => EmailBlockClass.findBlockById(latestTree, 'text-1')!.attributes as Record<string, unknown>
  const undo = () => fireEvent.click(screen.getByTitle('Undo').closest('button')!)
  const redo = () => fireEvent.click(screen.getByTitle('Redo').closest('button')!)
  const undoDisabled = () => (screen.getByTitle('Undo').closest('button') as HTMLButtonElement).disabled

  it('makes one undo step of writes with the same key, back to the value before the first', () => {
    render(<Controlled initial={initial()} />)
    write({ paddingTop: '1px' }, 'k1')
    write({ paddingTop: '12px' }, 'k1')
    write({ paddingTop: '120px' }, 'k1')
    expect(attrs().paddingTop).toBe('120px')

    undo()
    expect(attrs().paddingTop).toBeUndefined()
    expect(undoDisabled()).toBe(true)
  })

  it('makes a step per key, and a new step for a new key on the same field', () => {
    render(<Controlled initial={initial()} />)
    write({ paddingTop: '10px' }, 'top:1')
    write({ paddingLeft: '5px' }, 'left:1')
    write({ paddingTop: '20px' }, 'top:2')

    undo()
    expect(attrs()).toMatchObject({ paddingTop: '10px', paddingLeft: '5px' })
    undo()
    expect(attrs().paddingLeft).toBeUndefined()
    expect(attrs().paddingTop).toBe('10px')
  })

  it('always adds a step for a write without a key', () => {
    render(<Controlled initial={initial()} />)
    write({ align: 'left' })
    write({ align: 'center' })
    undo()
    expect(attrs().align).toBe('left')
  })

  it('adds a new step for a write after an undo, even with the same key', () => {
    render(<Controlled initial={initial()} />)
    write({ paddingTop: '10px' }, 'k')
    write({ paddingTop: '20px' }, 'k')
    undo()
    redo()
    write({ paddingTop: '30px' }, 'k')
    undo()
    expect(attrs().paddingTop).toBe('20px')
  })

  it('writes only its keys into the email as it is now, keeping a change made from outside', () => {
    render(<Controlled initial={initial()} />)
    const outside = JSON.parse(JSON.stringify(latestTree)) as EmailBlock
    ;(EmailBlockClass.findBlockById(outside, 'text-1') as { content?: string }).content = '<p>From the assistant</p>'
    act(() => setOutside(outside))

    write({ color: '#222222' })
    const text = EmailBlockClass.findBlockById(latestTree, 'text-1') as { content?: string; attributes?: Record<string, unknown> }
    expect(text.content).toBe('<p>From the assistant</p>')
    expect(text.attributes?.color).toBe('#222222')
  })

  it('undoes a change made from outside on its own, then the editor change before it', () => {
    render(<Controlled initial={initial()} />)
    write({ color: '#222222' })
    const outside = JSON.parse(JSON.stringify(latestTree)) as EmailBlock
    ;(EmailBlockClass.findBlockById(outside, 'text-1') as { content?: string }).content = '<p>From the assistant</p>'
    act(() => setOutside(outside))

    const content = () => (EmailBlockClass.findBlockById(latestTree, 'text-1') as { content?: string }).content
    undo()
    expect(content()).toBe('<p>Hi</p>')
    expect(attrs().color).toBe('#222222')
    undo()
    expect(attrs().color).toBe('#111111')
    expect(undoDisabled()).toBe(true)

    redo()
    redo()
    expect(content()).toBe('<p>From the assistant</p>')
    expect(attrs().color).toBe('#222222')
  })

  it('does not fold a write into the step before a change made from outside', () => {
    render(<Controlled initial={initial()} />)
    write({ paddingTop: '10px' }, 'k')
    const outside = JSON.parse(JSON.stringify(latestTree)) as EmailBlock
    ;(EmailBlockClass.findBlockById(outside, 'text-1') as { content?: string }).content = '<p>From the assistant</p>'
    act(() => setOutside(outside))
    write({ paddingTop: '20px' }, 'k')

    undo()
    expect(attrs().paddingTop).toBe('10px')
    expect((EmailBlockClass.findBlockById(latestTree, 'text-1') as { content?: string }).content).toBe(
      '<p>From the assistant</p>'
    )
  })

  it('removes an attribute written as undefined', () => {
    render(<Controlled initial={initial()} />)
    write({ color: undefined })
    expect('color' in attrs()).toBe(false)
  })

  it('writes nothing for a block that is no longer in the email', () => {
    render(<Controlled initial={initial()} />)
    write({ color: '#222222' }, undefined, 'gone')
    expect(attrs().color).toBe('#111111')
    expect(undoDisabled()).toBe(true)
  })

  it('writes nothing once the editor is unmounted', () => {
    const onTreeChange = vi.fn()
    const { rerender } = render(<EmailBuilder {...props(initial(), vi.fn(), null)} onTreeChange={onTreeChange} />)
    const writeLater = mockSettingsPanelProps.onUpdateAttributes as (id: string, u: Record<string, unknown>) => void
    // The page that held the editor stays, the editor goes (a closed drawer)
    rerender(<div />)
    writeLater('text-1', { color: '#222222' })
    expect(onTreeChange).not.toHaveBeenCalled()
  })
})
