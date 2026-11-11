import { afterEach, describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AddBlockPopover, ADD_BLOCK_TAB_KEY } from './AddBlockPopover'
import type { EmailBlock, MJMLComponentType, SavedBlock } from '../types'
import { b, condition, email, text } from '../utils/libraryTestTrees'

const saved = (id: string, name: string, block: EmailBlock, created: string): SavedBlock => ({
  id,
  name,
  block,
  created
})

const cta = saved('cta', 'CTA text', text('cta-block'), '2026-08-01T00:00:00Z')
const intro = saved('intro', 'Intro text', text('intro-block'), '2026-09-01T00:00:00Z')
const header = saved(
  'header',
  'Header',
  b('header-block', 'mj-section', [b('header-col', 'mj-column', [])]),
  '2026-10-01T00:00:00Z'
)
const column = saved(
  'column',
  'Two thirds',
  b('column-block', 'mj-column', []),
  '2026-07-01T00:00:00Z'
)
const liquid = saved(
  'liquid',
  'Loop',
  b('liquid-block', 'mj-liquid', undefined, {
    content: '{% if a %}{% endif %}'
  } as Partial<EmailBlock>),
  '2026-06-01T00:00:00Z'
)
const style = saved('style', 'Brand CSS', b('style-block', 'mj-style'), '2026-06-01T00:00:00Z')

const tree = email([
  b('section-1', 'mj-section', [
    b('column-1', 'mj-column', [text('text-1')]),
    b('column-2', 'mj-column', [], condition)
  ])
])

interface Options {
  mode?: 'header' | 'content'
  parentId?: string
  parentType?: MJMLComponentType
  savedBlocks?: SavedBlock[]
  hiddenBlocks?: string[]
  label?: string
}

const popover = (
  options: Options,
  handlers: { onAddBlock: () => void; onAddSavedBlock: () => void }
) => (
  <AddBlockPopover
    mode={options.mode ?? 'content'}
    parentId={options.parentId ?? 'column-1'}
    parentType={options.parentType ?? 'mj-column'}
    position={1}
    emailTree={tree}
    savedBlocks={options.savedBlocks ?? [cta, intro, header, column]}
    hiddenBlocks={options.hiddenBlocks}
    {...handlers}
  >
    <button type="button">{options.label ?? 'Add'}</button>
  </AddBlockPopover>
)

const renderPopover = (options: Options = {}) => {
  const handlers = { onAddBlock: vi.fn(), onAddSavedBlock: vi.fn() }
  render(popover(options, handlers))
  return handlers
}

// Closed popovers stay in the document, hidden: the open one is the last
const lastTab = (name: RegExp) => screen.getAllByRole('tab', { name }).at(-1)!
const open = async (label = 'Add') => userEvent.click(screen.getByRole('button', { name: label }))
const openSaved = async (label = 'Add') => {
  await open(label)
  await userEvent.click(lastTab(/Saved blocks/))
}
const row = (id: string) => document.querySelector(`[data-saved-row="${id}"]`) as HTMLElement
const rowIds = (selector: string) =>
  Array.from(document.querySelectorAll(`${selector} [data-saved-row]`)).map((el) =>
    el.getAttribute('data-saved-row')
  )

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('AddBlockPopover saved blocks', () => {
  it('always shows the saved blocks tab with every saved block counted', async () => {
    renderPopover()
    await open()
    expect(lastTab(/Saved blocks/)).toHaveTextContent('Saved blocks (4)')
  })

  it('shows the saved blocks tab with no saved block, and says how to save one', async () => {
    renderPopover({ savedBlocks: [] })
    await openSaved()
    expect(lastTab(/Saved blocks/)).toHaveTextContent('Saved blocks (0)')
    expect(screen.getByText('No saved blocks yet')).toBeInTheDocument()
    expect(
      screen.getByText('Save a block from its toolbar to reuse it in every template.')
    ).toBeInTheDocument()
  })

  it('does not count or list the blocks of a hidden type', async () => {
    renderPopover({ hiddenBlocks: ['mj-section'] })
    await openSaved()
    expect(lastTab(/Saved blocks/)).toHaveTextContent('Saved blocks (3)')
    expect(row('header')).toBeNull()
  })

  it('lists first the blocks that can go here, newest first, then the others under "Not available here"', async () => {
    renderPopover()
    await openSaved()
    expect(rowIds('[data-testid="saved-available"]')).toEqual(['intro', 'cta'])
    expect(screen.getByText('Not available here')).toBeInTheDocument()
    expect(rowIds('[data-testid="saved-unavailable"]')).toEqual(['header', 'column'])
  })

  it('gives the reason a block cannot go here, as visible text and as its title', async () => {
    renderPopover()
    await openSaved()
    expect(row('column')).toHaveTextContent('Goes inside a section')
    expect(row('column')).toHaveAttribute('title', 'Goes inside a section')
    expect(row('header')).toHaveTextContent('Not allowed here')
  })

  it('gives the reason a text cannot go in a section', async () => {
    renderPopover({ parentId: 'section-1', parentType: 'mj-section' })
    await openSaved()
    expect(row('cta')).toHaveTextContent('Goes inside a column')
    expect(row('column')).not.toHaveAttribute('aria-disabled')
  })

  it('refuses a Liquid block in a column with a display condition', async () => {
    renderPopover({ parentId: 'column-2', savedBlocks: [liquid, cta] })
    await openSaved()
    expect(row('liquid')).toHaveAttribute('aria-disabled', 'true')
    expect(row('liquid')).toHaveTextContent('Not allowed inside a conditional block')
    expect(row('cta')).not.toHaveAttribute('aria-disabled')
  })

  it('does not insert a block that cannot go here, nor preview it', async () => {
    const { onAddSavedBlock } = renderPopover()
    await openSaved()
    expect(row('header')).toHaveAttribute('aria-disabled', 'true')
    fireEvent.mouseEnter(row('header'))
    fireEvent.click(row('header'))
    expect(onAddSavedBlock).not.toHaveBeenCalled()
    expect(screen.queryByText(/A copy is inserted/)).toBeNull()
  })

  it('shows a thumbnail on each block that can go here', async () => {
    renderPopover()
    await openSaved()
    expect(within(row('cta')).getByTestId('block-thumb').style.width).toBe('56px')
    expect(within(row('header')).queryByTestId('block-thumb')).toBeNull()
  })

  it('previews a hovered block with its type, date and what inserting does', async () => {
    renderPopover()
    await openSaved()
    fireEvent.mouseEnter(row('cta'))
    const preview = screen.getByTestId('saved-preview')
    expect(within(preview).getByText('CTA text')).toBeInTheDocument()
    expect(within(preview).getByTestId('block-thumb').style.width).toBe('266px')
    expect(preview).toHaveTextContent('Text')
    expect(preview).toHaveTextContent('Aug 1, 2026')
    expect(preview).toHaveTextContent(
      'A copy is inserted. Later changes to the saved block do not change it.'
    )
  })

  it('inserts a saved block from its row and from the preview', async () => {
    const { onAddSavedBlock } = renderPopover()
    await openSaved()
    fireEvent.click(row('cta'))
    expect(onAddSavedBlock).toHaveBeenCalledWith('column-1', cta.block, 1)

    await openSaved()
    fireEvent.mouseEnter(row('intro'))
    fireEvent.click(within(screen.getByTestId('saved-preview')).getByRole('button'))
    expect(onAddSavedBlock).toHaveBeenLastCalledWith('column-1', intro.block, 1)
  })

  it('opens on the tab used last, in every "+" of the email', async () => {
    const handlers = { onAddBlock: vi.fn(), onAddSavedBlock: vi.fn() }
    render(
      <>
        {popover({ label: 'First' }, handlers)}
        {popover({ label: 'Second', parentId: 'column-2' }, handlers)}
      </>
    )
    await openSaved('First')
    expect(localStorage.getItem(ADD_BLOCK_TAB_KEY)).toBe('saved')
    await open('Second')
    expect(lastTab(/Saved blocks/)).toHaveAttribute('aria-selected', 'true')
  })

  it('opens on the blocks tab when the storage cannot be read or written', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    renderPopover()
    await open()
    expect(lastTab(/^Blocks$/)).toHaveAttribute('aria-selected', 'true')
    await userEvent.click(lastTab(/Saved blocks/))
    expect(lastTab(/Saved blocks/)).toHaveAttribute('aria-selected', 'true')
  })

  it('ignores a stored value that is not a tab', async () => {
    localStorage.setItem(ADD_BLOCK_TAB_KEY, 'other')
    renderPopover()
    await open()
    expect(lastTab(/^Blocks$/)).toHaveAttribute('aria-selected', 'true')
  })
})

describe('AddBlockPopover in the head', () => {
  it('offers the saved head blocks only, and only when there are some', async () => {
    renderPopover({ mode: 'header', parentId: 'head', parentType: 'mj-head' })
    await open()
    expect(screen.queryByRole('tab', { name: /Saved/ })).toBeNull()
  })

  it('opens on the blocks tab whatever the tab used last in the body', async () => {
    localStorage.setItem(ADD_BLOCK_TAB_KEY, 'saved')
    renderPopover({
      mode: 'header',
      parentId: 'head',
      parentType: 'mj-head',
      savedBlocks: [style, cta]
    })
    await open()
    expect(lastTab(/^Blocks$/)).toHaveAttribute('aria-selected', 'true')
    expect(lastTab(/Saved/)).toHaveTextContent('Saved (1)')
    // The head's tab is not remembered
    localStorage.clear()
    await userEvent.click(lastTab(/Saved/))
    expect(localStorage.getItem(ADD_BLOCK_TAB_KEY)).toBeNull()
    expect(screen.getByText('Brand CSS')).toBeInTheDocument()
  })
})
