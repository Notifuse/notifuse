import { test, expect } from '../fixtures/auth'
import {
  auditBody,
  block,
  column,
  find,
  openTreeEditor,
  section,
  shape,
  text,
  type TreeBlock
} from '../fixtures/tree-drag'

// Structure tree of the template editor, dragged with the mouse. The pointer goes straight down from where the
// block was grabbed, plus a sideways offset that picks the level. Each case saves the template and reads the tree
// sent to the API. Offsets stay off ±12 px, where the level changes.

const children = (b: TreeBlock | undefined) => (b?.children ?? []).map((child) => child.id)

// The audit tree with empty containers in front
const withEmpties = (): TreeBlock[] => [
  section('ES', []),
  section('S5', [
    column('EC', []),
    column('C5', [text('T5', 'Echo'), block('ESO', 'mj-social', [])])
  ]),
  block('EW', 'mj-wrapper', []),
  section('S6', [block('EG', 'mj-group', []), column('C6', [text('T6', 'Foxtrot')])]),
  ...auditBody()
]

const withWidths = (): TreeBlock[] => {
  const body = auditBody()
  const s1 = body[0]
  s1.children![0].attributes = { width: '30%' }
  s1.children![1].attributes = { width: '70%' }
  body[3] = section('S3', [
    column('C3', [text('T3', 'Charlie')], '40%'),
    column('C4', [text('T5', 'Echo')], '60%')
  ])
  return body
}

const widths = (b: TreeBlock | undefined) =>
  (b?.children ?? []).map((child) => `${child.id} ${child.attributes?.width}`)

test.describe('Structure tree drag and drop', () => {
  test.describe('a block moved down lands where the line shows', () => {
    for (const [from, to, parent, expected] of [
      ['D', 'T3', 'C3', 'C3[SP,T3,D,T4]'],
      ['T1', 'BT1', 'C1', 'C1[BT1,T1,IM1]'],
      ['S1', 'R', 'B', 'W,S2,S3,R,S1,L']
    ] as const) {
      test(`${from} below ${to}`, async ({ authenticatedPageWithData: page }) => {
        const editor = await openTreeEditor(page, auditBody())
        await editor.hover(from, to, 0.8)
        expect(await editor.lineShown()).toBe(true)
        await editor.release()
        const body = await editor.saved()
        const result = parent === 'B' ? children(body).join(',') : shape(find(body, parent)!)
        expect(result).toBe(expected)
      })
    }
  })

  test.describe('moves up and across containers', () => {
    for (const [from, to, y, parent, expected] of [
      ['T4', 'SP', 0.2, 'C3', 'C3[D,T4,SP,T3]'],
      ['T1', 'C2', 0.7, 'C2', 'C2[T1,T2,SO[SE1,SE2]]'],
      ['C2', 'S3', 0.7, 'S3', 'S3[C2[T2,SO[SE1,SE2]],C3[D,SP,T3,T4]]']
    ] as const) {
      test(`${from} onto ${to} at ${y}`, async ({ authenticatedPageWithData: page }) => {
        const editor = await openTreeEditor(page, auditBody())
        await editor.drag(from, to, y)
        expect(shape(find(await editor.saved(), parent)!)).toBe(expected)
      })
    }
  })

  test.describe('refused places show no line and change nothing', () => {
    for (const [from, to, label] of [
      ['T2', 'SE1', 'text between social elements'],
      ['T1', 'L', 'text at body level'],
      ['S1', 'T3', 'section at the level of a text']
    ] as const) {
      test(label, async ({ authenticatedPageWithData: page }) => {
        const editor = await openTreeEditor(page, auditBody())
        await editor.hover(from, to, 0.8)
        expect(await editor.lineShown()).toBe(false)
        await editor.release()
        expect(shape(await editor.saved())).toBe(editor.original)
      })
    }
  })

  test.describe('a section dropped on the top half of the next section lands before it', () => {
    // The body is the only level of that gap that takes a section
    for (const dx of [0, -24, 24, 48]) {
      test(`cursor moved ${dx}px sideways`, async ({ authenticatedPageWithData: page }) => {
        const editor = await openTreeEditor(page, auditBody())
        await editor.hover('S1', 'S3', 0.2, dx)
        expect(await editor.lineShown()).toBe(true)
        await editor.release()
        expect(children(await editor.saved())).toEqual(['W', 'S2', 'S1', 'S3', 'R', 'L'])
      })
    }
  })

  test.describe('empty containers take a block dropped on them', () => {
    for (const [from, to, label] of [
      ['T1', 'EC', 'column'],
      ['C1', 'ES', 'section'],
      ['SE1', 'ESO', 'social block'],
      ['S1', 'EW', 'wrapper'],
      ['C1', 'EG', 'group']
    ] as const) {
      test(label, async ({ authenticatedPageWithData: page }) => {
        const editor = await openTreeEditor(page, withEmpties())
        await editor.hover(from, to, 0.8)
        expect(await editor.lineShown()).toBe(true)
        await editor.release()
        expect(children(find(await editor.saved(), to))).toEqual([from])
      })
    }

    test('a collapsed column takes a text and opens', async ({
      authenticatedPageWithData: page
    }) => {
      const editor = await openTreeEditor(page, auditBody())
      await editor.collapse('C3')
      await editor.drag('T1', 'C3', 0.8)
      // Every row is shown again, C3's children included
      await expect(editor.rows).toHaveCount(editor.allExpanded)
      expect(shape(find(await editor.saved(), 'C3')!)).toBe('C3[D,SP,T3,T4,T1]')
    })
  })

  test('the wrapper moves by its handle', async ({ authenticatedPageWithData: page }) => {
    const editor = await openTreeEditor(page, auditBody())
    await expect(
      editor.row('W').getByRole('img', { name: 'Drag to move this block in the tree' })
    ).toBeVisible()
    await editor.drag('W', 'L', 0.8, 0, { by: 'handle' })
    expect(children(await editor.saved())).toEqual(['S1', 'S2', 'S3', 'R', 'L', 'W'])
  })

  test('the Body row cannot be dragged', async ({ authenticatedPageWithData: page }) => {
    const editor = await openTreeEditor(page, auditBody())
    await editor.hover('B', 'S3', 0.5)
    // No drag starts. Before this fix the row was dragged and its drops were refused afterwards, by rc-tree's
    // descendant check and, for a drop inside a row, by the nesting rules
    await expect(editor.tree.locator('.ant-tree-treenode.dragging')).toHaveCount(0)
    expect(await editor.lineShown()).toBe(false)
    await editor.release()
    expect(shape(await editor.saved())).toBe(editor.original)
  })

  test('a drop on the block’s own place adds no undo step', async ({
    authenticatedPageWithData: page
  }) => {
    const editor = await openTreeEditor(page, auditBody())
    await editor.hover('SP', 'D', 0.8)
    expect(await editor.lineShown()).toBe(false)
    await editor.release()
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
  })

  test('a column moved to another section keeps the proportions on both sides', async ({
    authenticatedPageWithData: page
  }) => {
    const editor = await openTreeEditor(page, withWidths())
    await editor.drag('C2', 'S3', 0.7)
    const body = await editor.saved()
    expect(widths(find(body, 'S1'))).toEqual(['C1 100%'])
    expect(widths(find(body, 'S3'))).toEqual(['C2 33.33%', 'C3 26.67%', 'C4 40%'])
  })

  test.describe('the pointer’s sideways move picks the level', () => {
    test('a section dropped below a wrapper goes after it', async ({
      authenticatedPageWithData: page
    }) => {
      const editor = await openTreeEditor(page, auditBody())
      await editor.drag('S1', 'TW', 0.8)
      expect(children(await editor.saved())).toEqual(['W', 'S1', 'S2', 'S3', 'R', 'L'])
    })

    test('a section dropped below a wrapper one indent right goes into it', async ({
      authenticatedPageWithData: page
    }) => {
      const editor = await openTreeEditor(page, auditBody())
      await editor.drag('S1', 'TW', 0.8, 24)
      const body = await editor.saved()
      expect(children(body)).toEqual(['W', 'S2', 'S3', 'R', 'L'])
      expect(children(find(body, 'W'))).toEqual(['SW', 'S1'])
    })

    test('a section dropped on an empty wrapper fills it, or goes after it one indent left', async ({
      authenticatedPageWithData: page
    }) => {
      const editor = await openTreeEditor(page, withEmpties())
      await editor.drag('S1', 'EW', 0.8, -24)
      const body = await editor.saved()
      expect(children(body).slice(0, 5)).toEqual(['ES', 'S5', 'EW', 'S1', 'S6'])
      expect(children(find(body, 'EW'))).toEqual([])
    })

    test('Liquid dropped below the last text of a section stays at body level', async ({
      authenticatedPageWithData: page
    }) => {
      const editor = await openTreeEditor(page, auditBody())
      await editor.drag('L', 'T4', 0.8)
      const body = await editor.saved()
      expect(children(body)).toEqual(['S1', 'W', 'S2', 'S3', 'L', 'R'])
      expect(children(find(body, 'C3'))).toEqual(['D', 'SP', 'T3', 'T4'])
    })

    test('the line is indented at the level where the block goes', async ({
      authenticatedPageWithData: page
    }) => {
      const editor = await openTreeEditor(page, auditBody())
      await editor.hover('S1', 'TW', 0.8)
      expect(Math.abs((await editor.lineLeft()) - (await editor.contentLeft('S2')))).toBeLessThan(2)
      await editor.moveTo('TW', 0.8, 24)
      expect(Math.abs((await editor.lineLeft()) - (await editor.contentLeft('SW')))).toBeLessThan(2)
      await editor.release()
    })
  })

  test('the line goes away on the half of a row where the block cannot go', async ({
    authenticatedPageWithData: page
  }) => {
    const editor = await openTreeEditor(page, auditBody())
    // Top half of the social block: after T2. Bottom half: inside the social block, which refuses a text
    await editor.hover('T4', 'SO', 0.25)
    expect(await editor.lineShown()).toBe(true)
    await editor.moveTo('SO', 0.75)
    expect(await editor.lineShown()).toBe(false)
    await editor.release()
    expect(shape(await editor.saved())).toBe(editor.original)
  })

  test('a drop in the space between two rows lands between them', async ({
    authenticatedPageWithData: page
  }) => {
    const editor = await openTreeEditor(page, auditBody())
    await editor.hover('D', 'T3', 1, 0, { yPx: 2 })
    expect(await editor.lineShown()).toBe(true)
    await editor.release()
    expect(shape(find(await editor.saved(), 'C3')!)).toBe('C3[SP,T3,D,T4]')
  })

  test.describe('grabbed by the handle, a block fills an empty container', () => {
    for (const [from, to, label] of [
      ['S1', 'EW', 'wrapper'],
      ['C1', 'EG', 'group']
    ] as const) {
      test(label, async ({ authenticatedPageWithData: page }) => {
        const editor = await openTreeEditor(page, withEmpties())
        await editor.hover(from, to, 0.8, 0, { by: 'handle' })
        expect(await editor.lineShown()).toBe(true)
        await editor.release()
        expect(children(find(await editor.saved(), to))).toEqual([from])
      })
    }
  })

  test('leaving the tree removes the line and drops nothing', async ({
    authenticatedPageWithData: page
  }) => {
    const editor = await openTreeEditor(page, auditBody())
    await editor.hover('D', 'T3', 0.8)
    expect(await editor.lineShown()).toBe(true)
    await editor.moveTo('T3', 0.8, 600)
    expect(await editor.lineShown()).toBe(false)
    await editor.release()
    expect(shape(await editor.saved())).toBe(editor.original)
  })
})
