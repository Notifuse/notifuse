import type { Page } from '@playwright/test'
import { expect } from './auth'
import { openTemplateEditor } from './template-editor'
import { mockTemplates } from './mock-data'

// Drag-and-drop in the template editor's structure tree, with real mouse gestures

export interface TreeBlock {
  id: string
  type: string
  attributes?: Record<string, unknown>
  content?: string
  children?: TreeBlock[]
}

export const text = (id: string, label: string): TreeBlock => ({
  id,
  type: 'mj-text',
  attributes: {},
  content: `<p>${label}</p>`
})
export const column = (id: string, children: TreeBlock[], width?: string): TreeBlock => ({
  id,
  type: 'mj-column',
  attributes: width ? { width } : {},
  children
})
export const section = (id: string, children: TreeBlock[]): TreeBlock => ({
  id,
  type: 'mj-section',
  attributes: { backgroundColor: '#ffffff' },
  children
})
export const block = (id: string, type: string, children?: TreeBlock[]): TreeBlock => ({
  id,
  type,
  attributes: {},
  ...(children ? { children } : {})
})

// body > S1[C1[T1,BT1,IM1], C2[T2,SO[SE1,SE2]]], W[SW[CW[TW]]], S2[G[G1[TG1],G2[TG2]]], S3[C3[D,SP,T3,T4]], R, L
export const auditBody = (): TreeBlock[] => [
  section('S1', [
    column(
      'C1',
      [
        text('T1', 'Alpha'),
        { id: 'BT1', type: 'mj-button', attributes: {}, content: 'Shop' },
        { id: 'IM1', type: 'mj-image', attributes: { src: 'https://placehold.co/300x100' } }
      ],
      '50%'
    ),
    column(
      'C2',
      [
        text('T2', 'Bravo'),
        block('SO', 'mj-social', [
          {
            id: 'SE1',
            type: 'mj-social-element',
            attributes: { name: 'facebook', href: '#' },
            content: 'Facebook'
          },
          {
            id: 'SE2',
            type: 'mj-social-element',
            attributes: { name: 'twitter', href: '#' },
            content: 'Twitter'
          }
        ])
      ],
      '50%'
    )
  ]),
  block('W', 'mj-wrapper', [section('SW', [column('CW', [text('TW', 'Wrapped')])])]),
  section('S2', [
    block('G', 'mj-group', [
      column('G1', [text('TG1', 'Left')], '50%'),
      column('G2', [text('TG2', 'Right')], '50%')
    ])
  ]),
  section('S3', [
    column('C3', [
      block('D', 'mj-divider'),
      block('SP', 'mj-spacer'),
      text('T3', 'Charlie'),
      text('T4', 'Delta')
    ])
  ]),
  { id: 'R', type: 'mj-raw', attributes: {}, content: '<div>raw</div>' },
  { id: 'L', type: 'mj-liquid', attributes: {}, content: '{{ contact.email }}' }
]

// Visible rows: rc-tree also renders a hidden measurer with the row class, without the treeitem role
const ROW = '[role="treeitem"]'

export const shape = (b: TreeBlock): string =>
  b.children && b.children.length ? `${b.id}[${b.children.map(shape).join(',')}]` : b.id

export const find = (b: TreeBlock, id: string): TreeBlock | undefined =>
  b.id === id ? b : b.children?.map((child) => find(child, id)).find(Boolean)

export const openTreeEditor = async (page: Page, body: TreeBlock[]) => {
  const root: TreeBlock = {
    id: 'ROOT',
    type: 'mjml',
    children: [block('HEAD', 'mj-head', []), block('B', 'mj-body', body)]
  }
  const template = {
    ...mockTemplates[0],
    email: { ...mockTemplates[0].email, visual_editor_tree: root }
  }
  const saves: TreeBlock[] = []
  await page.route('https://localapi.notifuse.com:4000/api/templates.update*', async (route) => {
    saves.push(route.request().postDataJSON().email.visual_editor_tree)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ template })
    })
  })
  await page.setViewportSize({ width: 1280, height: 1300 })
  const drawer = await openTemplateEditor(page, template)
  const tree = drawer.locator('.ant-tree').nth(1)

  // The editor opens with every container that has children expanded
  const countRows = (b: TreeBlock): number =>
    1 + (b.children?.length ? b.children.reduce((sum, child) => sum + countRows(child), 0) : 0)
  const allExpanded = countRows(root.children![1])
  await expect(tree.locator(ROW)).toHaveCount(allExpanded)

  const row = (id: string) =>
    tree.locator(ROW).filter({ has: page.locator(`[data-block-id="${id}"]`) })
  const handle = (id: string) =>
    row(id).getByRole('img', { name: 'Drag to move this block in the tree' })

  const collapse = async (id: string) => {
    const before = await tree.locator(ROW).count()
    await row(id).locator('.ant-tree-switcher').click()
    await expect(tree.locator(ROW)).not.toHaveCount(before)
  }

  // Where the block was grabbed: the pointer then moves straight down from there, plus dx
  let grabX = 0
  const grab = async (from: string, by: 'label' | 'handle' = 'label') => {
    const target =
      by === 'label' ? row(from).locator('.ant-tree-node-content-wrapper') : handle(from)
    const s = (await target.boundingBox())!
    grabX = by === 'label' ? s.x + 20 : s.x + s.width / 2
    await page.mouse.move(grabX, s.y + s.height / 2)
    await page.mouse.down()
    await page.mouse.move(grabX + 5, s.y + s.height / 2 + 3, { steps: 3 })
  }
  // Moves over `to` at a height of its row (0 top, 1 bottom), plus yPx below that
  const moveTo = async (to: string, yFrac: number, dx = 0, yPx = 0) => {
    const d = (await row(to).boundingBox())!
    const y = d.y + d.height * yFrac + yPx
    await page.mouse.move(grabX + dx - 1, y, { steps: 8 })
    await page.waitForTimeout(150)
    await page.mouse.move(grabX + dx, y, { steps: 2 })
    await page.waitForTimeout(150)
  }
  const hover = async (
    from: string,
    to: string,
    yFrac: number,
    dx = 0,
    options: { by?: 'label' | 'handle'; yPx?: number } = {}
  ) => {
    await grab(from, options.by)
    await moveTo(to, yFrac, dx, options.yPx)
  }
  const line = drawer.getByTestId('tree-drop-line')
  const lineShown = async () => (await line.count()) > 0
  const lineLeft = async () => (await line.boundingBox())!.x
  const contentLeft = async (id: string) =>
    (await row(id).locator('.ant-tree-node-content-wrapper').boundingBox())!.x
  const release = async () => {
    await page.mouse.up()
    await page.waitForTimeout(300)
  }
  const drag = async (
    from: string,
    to: string,
    yFrac: number,
    dx = 0,
    options: { by?: 'label' | 'handle'; yPx?: number } = {}
  ) => {
    await hover(from, to, yFrac, dx, options)
    await release()
  }

  const saved = async () => {
    const count = saves.length
    await drawer.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => saves.length).toBe(count + 1)
    return find(saves[saves.length - 1], 'B')!
  }

  return {
    drawer,
    tree,
    rows: tree.locator(ROW),
    allExpanded,
    row,
    collapse,
    hover,
    moveTo,
    lineShown,
    lineLeft,
    contentLeft,
    release,
    drag,
    saved,
    original: shape(root.children![1])
  }
}
