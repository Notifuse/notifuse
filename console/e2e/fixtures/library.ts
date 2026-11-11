import type { Page } from '@playwright/test'
import { expect } from './auth'
import { mockApi, openTemplateEditor, type EditorLabels } from './template-editor'
import { mockTemplates, mockUserMeResponse, mockWorkspace } from './mock-data'
import { block, column, find, section, text, type TreeBlock } from './tree-drag'

// The email editor's library: dragging a saved or basic block onto the canvas with real mouse gestures, then
// reading the tree sent to the API on save.

const conditioned = (b: TreeBlock): TreeBlock =>
  ({
    ...b,
    display_condition: {
      match: 'all',
      rules: [{ variable: 'contact.first_name', operator: 'is_present' }]
    }
  }) as TreeBlock

// body > S1[C1[T1,BT1],C2[T2]], W[SW1[CW1[TW1]],SW2[CW2[TW2]]], EW, S2[EC,C3[T3]], R, SC?[CC[TC]], S3[C4[T4,T5]]
export const baseBody = (): TreeBlock[] => [
  section('S1', [
    column(
      'C1',
      [text('T1', 'Alpha'), { id: 'BT1', type: 'mj-button', attributes: {}, content: 'Shop' }],
      '50%'
    ),
    column('C2', [text('T2', 'Bravo')], '50%')
  ]),
  block('W', 'mj-wrapper', [
    section('SW1', [column('CW1', [text('TW1', 'Wrapped one')])]),
    section('SW2', [column('CW2', [text('TW2', 'Wrapped two')])])
  ]),
  block('EW', 'mj-wrapper', []),
  section('S2', [column('EC', [], '50%'), column('C3', [text('T3', 'Charlie')], '50%')]),
  {
    id: 'R',
    type: 'mj-raw',
    attributes: {},
    content:
      '<div style="height:120px;background:#fafafa;border:1px solid #ddd">raw html in an iframe</div>'
  },
  conditioned(section('SC', [column('CC', [text('TC', 'Only with a first name')])])),
  section('S3', [column('C4', [text('T4', 'Delta'), text('T5', 'Echo')])])
]

const tall = (i: number): TreeBlock =>
  section(`L${i}`, [
    column(`LC${i}`, [
      text(`LT${i}`, `Long ${i}`),
      { id: `LS${i}`, type: 'mj-spacer', attributes: { height: '150px' } }
    ])
  ])
export const longBody = (): TreeBlock[] => [
  ...baseBody(),
  ...Array.from({ length: 10 }, (_, i) => tall(i + 1))
]

const saved = (id: string, name: string, b: TreeBlock, created: string) => ({
  id,
  name,
  block: b,
  created,
  updated: created
})

export const librarySavedBlocks = [
  saved(
    'lib-section',
    'Promo section',
    section('ls', [column('lsc', [text('lst', 'Lib section')], '100%')]),
    '2026-10-08T00:00:00Z'
  ),
  saved(
    'lib-section-2col',
    'Two columns',
    section('l2', [
      column('l2a', [text('l2at', 'Lib left')], '50%'),
      column('l2b', [text('l2bt', 'Lib right')], '50%')
    ]),
    '2026-10-07T00:00:00Z'
  ),
  saved('lib-text', 'Signature', text('lt', 'Lib text'), '2026-10-06T00:00:00Z'),
  saved(
    'lib-button',
    'Shop button',
    {
      id: 'lb',
      type: 'mj-button',
      attributes: { href: 'https://example.com' },
      content: 'Lib button'
    },
    '2026-10-05T00:00:00Z'
  ),
  saved(
    'lib-raw',
    'Raw banner',
    {
      id: 'lr',
      type: 'mj-raw',
      attributes: {},
      content: '<div style="padding:8px;background:#e6f4ff">Lib raw</div>'
    },
    '2026-10-04T00:00:00Z'
  ),
  saved(
    'lib-liquid',
    'Greeting logic',
    {
      id: 'll',
      type: 'mj-liquid',
      attributes: {},
      content: '{% if contact.first_name %}Hi{% endif %}'
    },
    '2026-10-03T00:00:00Z'
  ),
  saved(
    'lib-column',
    'Side column',
    column('lc', [text('lct', 'Lib column')]),
    '2026-10-02T00:00:00Z'
  ),
  saved(
    'lib-wrapper',
    'Wrapped block',
    block('lw', 'mj-wrapper', [
      section('lws', [column('lwc', [text('lwt', 'Lib wrapped')], '100%')])
    ]),
    '2026-10-01T00:00:00Z'
  )
]

const ids = (b: TreeBlock, out = new Set<string>()) => {
  out.add(b.id)
  b.children?.forEach((c) => ids(c, out))
  return out
}

// The top-most blocks of the saved body that were not in the original tree
const insertedIn = (body: TreeBlock, original: Set<string>) => {
  const found: { id: string; type: string; parentId: string; index: number }[] = []
  const walk = (b: TreeBlock) =>
    (b.children ?? []).forEach((c, index) => {
      if (!original.has(c.id)) found.push({ id: c.id, type: c.type, parentId: b.id, index })
      else walk(c)
    })
  walk(body)
  return found
}

interface OpenOptions {
  viewport?: { width: number; height: number }
  savedBlocks?: unknown[]
  // For a console in another language
  labels?: EditorLabels
}

export const workspaceWithBlocks = (savedBlocks: unknown[]) => ({
  ...mockWorkspace,
  settings: { ...mockWorkspace.settings, template_blocks: savedBlocks }
})

export const openLibraryEditor = async (
  page: Page,
  body: TreeBlock[],
  options: OpenOptions = {}
) => {
  const root: TreeBlock = {
    id: 'ROOT',
    type: 'mjml',
    children: [block('HEAD', 'mj-head', []), block('B', 'mj-body', body)]
  }
  const template = {
    ...mockTemplates[0],
    email: { ...mockTemplates[0].email, visual_editor_tree: root }
  }
  const workspace = workspaceWithBlocks(options.savedBlocks ?? librarySavedBlocks)
  await mockApi(page, 'user.me', { ...mockUserMeResponse, workspaces: [workspace] })
  await mockApi(page, 'workspace.get', { workspace })
  const saves: TreeBlock[] = []
  await page.route('https://localapi.notifuse.com:4000/api/templates.update*', async (route) => {
    saves.push(route.request().postDataJSON().email.visual_editor_tree)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ template })
    })
  })
  await page.setViewportSize(options.viewport ?? { width: 1600, height: 1000 })
  const drawer = await openTemplateEditor(page, template, options.labels)
  const canvas = drawer.locator('[data-canvas-root]')
  await expect(canvas.locator('[data-block-id="S1"]')).toBeVisible()
  await drawer.getByTestId('library-tab').click()
  await expect(drawer.getByTestId('library-panel')).toBeVisible()
  const original = ids(root.children![1])

  const handle = (key: string) =>
    key.startsWith('mj-')
      ? drawer.locator(`[data-library-tile="${key}"]`)
      : drawer.locator(`[data-library-card="${key}"] [data-library-drag-handle]`)
  const el = (id: string) => canvas.locator(`[data-block-id="${id}"]`).first()
  const indicator = page.getByTestId('canvas-drop-indicator')
  const refusal = page.getByTestId('canvas-drop-refusal')

  // Presses on a library card (or a basic block's tile, by its type) and moves a few pixels, which starts the drag
  const grab = async (key: string) => {
    await handle(key).scrollIntoViewIfNeeded()
    const b = (await handle(key).boundingBox())!
    await page.mouse.move(b.x + b.width / 2, b.y + Math.min(b.height / 2, 30))
    await page.mouse.down()
    await page.mouse.move(b.x + b.width / 2 + 8, b.y + Math.min(b.height / 2, 30) + 6, { steps: 4 })
  }
  // Moves to a point of a canvas block: fx, fy are fractions of its box (0 left/top, 1 right/bottom)
  const moveOver = async (id: string, fx: number, fy: number, dy = 0) => {
    const b = (await el(id).boundingBox())!
    await page.mouse.move(b.x + b.width * fx, b.y + b.height * fy + dy, { steps: 12 })
    await page.waitForTimeout(200)
  }
  const moveTo = async (x: number, y: number) => {
    await page.mouse.move(x, y, { steps: 12 })
    await page.waitForTimeout(200)
  }
  // Scrolls the canvas so the block shows in the middle of the window, before a gesture
  const reveal = async (id: string) => {
    await canvas.evaluate((root, id) => {
      const vp = root.closest('[data-overlayscrollbars-viewport]') as HTMLElement
      const target = root.querySelector(`[data-block-id="${id}"]`)!.getBoundingClientRect()
      vp.scrollTop += target.top + target.height / 2 - window.innerHeight / 2
    }, id)
    await page.waitForTimeout(150)
  }
  const shown = async () => {
    if ((await indicator.count()) === 0) return null
    return {
      parentId: await indicator.getAttribute('data-parent-id'),
      index: Number(await indicator.getAttribute('data-index')),
      kind: await indicator.getAttribute('data-kind')
    }
  }
  const release = async () => {
    await page.mouse.up()
    await page.waitForTimeout(300)
  }
  // Saving closes the editor: one save per test
  const saved = async () => {
    const count = saves.length
    await drawer.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => saves.length).toBe(count + 1)
    return find(saves[saves.length - 1], 'B')!
  }
  const inserted = async () => insertedIn(await saved(), original)
  // The body sent by the last save
  const lastSaved = () => find(saves[saves.length - 1], 'B')!
  const viewport = canvas.locator('xpath=ancestor::*[@data-overlayscrollbars-viewport][1]')
  return {
    page,
    drawer,
    canvas,
    el,
    handle,
    grab,
    moveOver,
    moveTo,
    reveal,
    shown,
    release,
    saved,
    inserted,
    lastSaved,
    indicator,
    refusal,
    viewport,
    saves
  }
}

export type LibraryEditor = Awaited<ReturnType<typeof openLibraryEditor>>

// A drop on the canvas: grab the card, hover, check the line, release, check the tree
export const dropCase = async (
  editor: LibraryEditor,
  key: string,
  over: string,
  fx: number,
  fy: number,
  expected: { parentId: string; index: number; kind?: string }
) => {
  await editor.reveal(over)
  await editor.grab(key)
  await editor.moveOver(over, fx, fy)
  const line = await editor.shown()
  expect(line, 'indicator while hovering').toMatchObject({
    parentId: expected.parentId,
    index: expected.index
  })
  if (expected.kind) expect(line!.kind).toBe(expected.kind)
  await editor.release()
  expect(await editor.indicator.count()).toBe(0)
  const result = await editor.inserted()
  expect(result).toHaveLength(1)
  expect(result[0]).toMatchObject({ parentId: expected.parentId, index: expected.index })
  return result[0]
}
