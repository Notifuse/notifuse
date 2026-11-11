import { test, expect } from '../fixtures/auth'
import { find, block, section, column, text } from '../fixtures/tree-drag'
import {
  baseBody,
  dropCase,
  longBody,
  openLibraryEditor,
  librarySavedBlocks
} from '../fixtures/library'
import { mockApi, openTemplateEditor } from '../fixtures/template-editor'
import { mockTemplates, mockUserMeResponse, mockWorkspace } from '../fixtures/mock-data'

// The library tab of the email editor: dragging saved and basic blocks onto the canvas with real mouse gestures,
// and inserting them with the "Insert" button. Each case checks the tree sent to the API on save.

test.describe('Dropping a block from the library on the canvas', () => {
  test.describe('sections at body level', () => {
    for (const [label, over, fy, index] of [
      ['between two sections (top half of S3)', 'S3', 0.25, 6],
      ['before the first section', 'S1', 0.1, 0],
      ['after the last section', 'S3', 0.9, 7]
    ] as const) {
      test(`section ${label}`, async ({ authenticatedPage: page }) => {
        const editor = await openLibraryEditor(page, baseBody())
        await dropCase(editor, 'lib-section', over, 0.5, fy, { parentId: 'B', index, kind: 'line' })
      })
    }

    test('section below the email, on the canvas background, goes last', async ({
      authenticatedPage: page
    }) => {
      const editor = await openLibraryEditor(page, baseBody())
      await editor.reveal('S3')
      const b = (await editor.el('S3').boundingBox())!
      await editor.grab('lib-section')
      await editor.moveTo(b.x + b.width / 2, b.y + b.height + 15)
      expect(await editor.shown()).toMatchObject({ parentId: 'B', index: 7 })
      await editor.release()
      expect(await editor.inserted()).toMatchObject([{ parentId: 'B', index: 7 }])
    })
  })

  test('section into an empty wrapper, shown as a box', async ({ authenticatedPage: page }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await dropCase(editor, 'lib-section', 'EW', 0.5, 0.5, { parentId: 'EW', index: 0, kind: 'box' })
  })

  test('section between the sections of a wrapper', async ({ authenticatedPage: page }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await dropCase(editor, 'lib-section', 'SW2', 0.5, 0.2, { parentId: 'W', index: 1 })
  })

  test('basic wrapper between two sections stays empty and leaves the sections alone', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    const added = await dropCase(editor, 'mj-wrapper', 'S3', 0.5, 0.2, { parentId: 'B', index: 6 })
    const body = editor.lastSaved()
    expect(find(body, added.id)!.children ?? []).toHaveLength(0)
    expect(body.children!.map((c) => c.id)).toEqual([
      'S1',
      'W',
      'EW',
      'S2',
      'R',
      'SC',
      added.id,
      'S3'
    ])
  })

  test.describe('content blocks in columns', () => {
    for (const [label, key, over, fy, parentId, index] of [
      ['text between two blocks', 'lib-text', 'BT1', 0.2, 'C1', 1],
      ['text at the top of a column', 'lib-text', 'T1', 0.1, 'C1', 0],
      ['button at the bottom of a column', 'lib-button', 'BT1', 0.9, 'C1', 2],
      ['button into an empty column', 'lib-button', 'EC', 0.5, 'EC', 0],
      ['basic text between two texts', 'mj-text', 'T5', 0.3, 'C4', 1]
    ] as const) {
      test(label, async ({ authenticatedPage: page }) => {
        const editor = await openLibraryEditor(page, baseBody())
        await dropCase(editor, key, over, 0.5, fy, { parentId, index })
      })
    }

    test('text under the shorter of two columns goes into that column', async ({
      authenticatedPage: page
    }) => {
      const editor = await openLibraryEditor(page, baseBody())
      const c1 = (await editor.el('C1').boundingBox())!
      const c2 = (await editor.el('C2').boundingBox())!
      await editor.grab('lib-text')
      // Level with the bottom of the taller column, under the shorter one: the section is there, not a column
      await editor.moveTo(c2.x + c2.width / 2, c1.y + c1.height - 4)
      expect(await editor.shown()).toMatchObject({ parentId: 'C2', index: 1 })
      await editor.release()
      expect(await editor.inserted()).toMatchObject([{ parentId: 'C2', index: 1 }])
    })
  })

  test('text between sections is refused with the reason, and inserts nothing', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    const s2 = (await editor.el('S2').boundingBox())!
    await editor.grab('lib-text')
    // On the canvas background left of the email, level with S2: body level
    await editor.moveTo(s2.x - 40, s2.y + s2.height / 2)
    expect(await editor.shown()).toBeNull()
    await expect(editor.refusal).toHaveText('Goes inside a column')
    await editor.release()
    await expect(editor.refusal).toHaveCount(0)
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
    expect(await editor.inserted()).toEqual([])
  })

  test('column beside the columns of a section, the widths rebalanced', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.grab('lib-column')
    await editor.moveOver('T2', 0.8, 0.5)
    expect(await editor.shown()).toMatchObject({ parentId: 'S1', index: 2 })
    await editor.release()
    const s1 = find(await editor.saved(), 'S1')!
    expect(s1.children).toHaveLength(3)
    const sum = s1.children!.reduce(
      (total, c) => total + parseFloat(String(c.attributes?.width)),
      0
    )
    expect(Math.abs(sum - 100)).toBeLessThan(0.1)
  })

  test.describe('raw HTML block (iframe)', () => {
    test('section dropped over the raw block goes before or after it', async ({
      authenticatedPage: page
    }) => {
      const editor = await openLibraryEditor(page, baseBody())
      await editor.reveal('R')
      await expect(editor.el('R').locator('iframe')).toBeVisible()
      await editor.grab('lib-section')
      await editor.moveOver('R', 0.5, 0.3)
      expect(await editor.shown()).toMatchObject({ parentId: 'B', index: 4 })
      // Inside the iframe's box, the line still follows the pointer
      await editor.moveOver('R', 0.5, 0.8)
      expect(await editor.shown()).toMatchObject({ parentId: 'B', index: 5 })
      await editor.release()
      expect(await editor.inserted()).toMatchObject([{ parentId: 'B', index: 5 }])
    })

    test('without its overlay and inert, the drag still follows over the iframe', async ({
      authenticatedPage: page
    }) => {
      const editor = await openLibraryEditor(page, baseBody())
      await editor.reveal('R')
      await page.evaluate(() => {
        const raw = document.querySelector('[data-canvas-root] [data-block-id="R"]')!
        raw.querySelector('iframe')!.removeAttribute('inert')
        ;(raw.firstElementChild as HTMLElement).style.display = 'none'
      })
      await editor.grab('lib-section')
      await editor.moveOver('S2', 0.5, 0.2)
      await editor.moveOver('R', 0.5, 0.8)
      expect(await editor.shown()).toMatchObject({ parentId: 'B', index: 5 })
      await editor.release()
      expect(await editor.inserted()).toMatchObject([{ parentId: 'B', index: 5 }])
    })
  })

  test('Liquid over a block with a display condition is refused with the reason', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.reveal('TC')
    await editor.grab('lib-liquid')
    await editor.moveOver('TC', 0.5, 0.5)
    expect(await editor.shown()).toBeNull()
    await expect(editor.refusal).toHaveText('Not allowed inside a conditional block')
    await editor.release()
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
  })

  test('long email: holding the pointer near the bottom edge scrolls the canvas to the end', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, longBody())
    const vp = (await editor.viewport.boundingBox())!
    await expect(editor.el('LT10')).not.toBeInViewport()
    await editor.grab('lib-text')
    await editor.moveTo(
      vp.x + vp.width / 2,
      Math.min(vp.y + vp.height, page.viewportSize()!.height) - 15
    )
    await expect
      .poll(() => editor.viewport.evaluate((v) => v.scrollHeight - v.clientHeight - v.scrollTop), {
        timeout: 15000
      })
      .toBeLessThan(1)
    // Below the email, a text has no place: the reason shows, no line
    await expect(editor.refusal).toHaveText('Goes inside a column')
    expect(await editor.shown()).toBeNull()
    // Back over the last text: the line follows the scrolled canvas, and the drop lands there
    await editor.moveOver('LT10', 0.5, 0.8)
    expect(await editor.shown()).toMatchObject({ parentId: 'LC10', index: 1 })
    await editor.release()
    expect(await editor.inserted()).toMatchObject([{ parentId: 'LC10', index: 1, type: 'mj-text' }])
  })

  test('long email: the canvas scrolls up near its top edge', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, longBody())
    await editor.viewport.evaluate((v) => (v.scrollTop = v.scrollHeight))
    await page.waitForTimeout(200)
    const vp = (await editor.viewport.boundingBox())!
    await editor.grab('lib-section')
    await editor.moveTo(vp.x + vp.width / 2, vp.y + 10)
    await expect(editor.el('S1')).toBeInViewport({ timeout: 15000 })
    await editor.moveOver('S1', 0.5, 0.1)
    expect(await editor.shown()).toMatchObject({ parentId: 'B', index: 0 })
    await editor.release()
    expect(await editor.inserted()).toMatchObject([{ parentId: 'B', index: 0 }])
  })

  test('the bottom of a long email can be reached', async ({ authenticatedPage: page }) => {
    const editor = await openLibraryEditor(page, longBody())
    await editor.viewport.evaluate((v) => (v.scrollTop = v.scrollHeight))
    await page.waitForTimeout(300)
    const last = (await editor.el('LS10').boundingBox())!
    expect(last.y + last.height).toBeLessThanOrEqual(page.viewportSize()!.height)
  })

  test('leaving the canvas clears the line and inserts nothing', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.grab('lib-section')
    await editor.moveOver('S2', 0.5, 0.3)
    expect(await editor.shown()).not.toBeNull()
    const settings = (await editor.drawer.locator('.w-96').first().boundingBox())!
    await editor.moveTo(settings.x + settings.width / 2, settings.y + 300)
    expect(await editor.shown()).toBeNull()
    await editor.release()
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
  })

  test('Escape cancels the drag and inserts nothing', async ({ authenticatedPage: page }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.grab('lib-text')
    await editor.moveOver('T3', 0.5, 0.7)
    expect(await editor.shown()).not.toBeNull()
    await page.keyboard.press('Escape')
    await page.waitForTimeout(200)
    expect(await editor.shown()).toBeNull()
    await expect(page.getByTestId('library-drag-ghost')).toHaveCount(0)
    await editor.release()
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
  })

  test('one undo removes the insertion, and the inserted block is selected', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.reveal('T3')
    await editor.grab('lib-button')
    await editor.moveOver('T3', 0.5, 0.8)
    await editor.release()
    const selected = await editor.canvas
      .locator('[data-block-id].selected')
      .getAttribute('data-block-id')
    await editor.drawer.getByTitle('Undo').click()
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
    expect(selected).toBeTruthy()
    await expect(editor.canvas.locator(`[data-block-id="${selected}"]`)).toHaveCount(0)
  })

  test("the message's Undo removes the insertion", async ({ authenticatedPage: page }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.reveal('S1')
    await editor.grab('lib-section')
    await editor.moveOver('S1', 0.5, 0.1)
    expect(await editor.shown()).toMatchObject({ parentId: 'B', index: 0 })
    await editor.release()
    await expect(editor.canvas.getByText('Lib section')).toHaveCount(1)
    await page.getByTestId('library-insert-undo').click()
    await expect(editor.canvas.getByText('Lib section')).toHaveCount(0)
    await expect(editor.drawer.getByTitle('Undo')).toBeDisabled()
    expect(await editor.inserted()).toEqual([])
  })
})

test.describe('Inserting a block from the library', () => {
  test('with a text selected, "Insert" puts the block after it and selects it', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.reveal('T3')
    await editor.el('T3').click()
    await expect(editor.el('T3')).toHaveClass(/selected/)
    const card = editor.drawer.locator('[data-library-card="lib-button"]')
    await card.hover()
    await card.getByRole('button', { name: 'Insert' }).click()

    const selected = await editor.canvas
      .locator('[data-block-id].selected')
      .getAttribute('data-block-id')
    const added = await editor.inserted()
    expect(added).toMatchObject([{ parentId: 'C3', index: 1, type: 'mj-button' }])
    expect(selected).toBe(added[0].id)
  })

  test('the keyboard reaches a card\'s "Insert", and Enter inserts', async ({
    authenticatedPage: page
  }) => {
    const editor = await openLibraryEditor(page, baseBody())
    await editor.drawer.getByPlaceholder('Search saved blocks').focus()
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press('Tab')
      const focused = await page.evaluate(() => document.activeElement?.textContent ?? '')
      if (focused === 'Insert') break
    }
    expect(await page.evaluate(() => document.activeElement?.textContent)).toBe('Insert')
    await page.keyboard.press('Enter')
    // The first card is the newest saved block, a section: it goes at the end of the body
    expect(await editor.inserted()).toMatchObject([{ parentId: 'B', index: 7, type: 'mj-section' }])
  })
})

test('a drop in the translation editor lands in the translation, not in the hidden main editor', async ({
  authenticatedPage: page
}) => {
  const root = {
    id: 'ROOT',
    type: 'mjml',
    children: [
      block('HEAD', 'mj-head', []),
      block('B', 'mj-body', [section('S1', [column('C1', [text('T1', 'Alpha')])])])
    ]
  }
  const template = {
    ...mockTemplates[0],
    email: { ...mockTemplates[0].email, visual_editor_tree: root }
  }
  const workspace = {
    ...mockWorkspace,
    settings: {
      ...mockWorkspace.settings,
      languages: ['en', 'fr'],
      default_language: 'en',
      template_blocks: librarySavedBlocks
    }
  }
  await mockApi(page, 'user.me', {
    ...mockUserMeResponse,
    workspaces: [workspace],
    entitlements: {
      tier: 'pro',
      org: 'Test',
      sub: 'test',
      max_workspaces: -1,
      features: ['template_i18n'],
      state: 'active',
      expires_at: '2099-01-01T00:00:00Z'
    }
  })
  await mockApi(page, 'workspace.get', { workspace })
  const saves: Record<string, unknown>[] = []
  await page.route('https://localapi.notifuse.com:4000/api/templates.update*', async (route) => {
    saves.push(route.request().postDataJSON())
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ template })
    })
  })
  await page.setViewportSize({ width: 1600, height: 1000 })
  const drawer = await openTemplateEditor(page, template)
  await expect(drawer.locator('[data-canvas-root] [data-block-id="S1"]')).toBeVisible()
  await drawer.getByRole('button', { name: 'Next' }).click()
  await drawer.getByRole('switch').first().click()
  await drawer.getByRole('button', { name: 'Open email editor' }).click()
  const translation = page.getByRole('dialog', { name: /translation/ })
  const canvas = translation.locator('[data-canvas-root]')
  await expect(canvas.locator('[data-block-id="S1"]')).toBeVisible()
  await translation.getByTestId('library-tab').click()

  const handle = translation.locator('[data-library-card="lib-section"] [data-library-drag-handle]')
  const h = (await handle.boundingBox())!
  await page.mouse.move(h.x + h.width / 2, h.y + 20)
  await page.mouse.down()
  await page.mouse.move(h.x + h.width / 2 + 8, h.y + 26, { steps: 4 })
  const s1 = (await canvas.locator('[data-block-id="S1"]').boundingBox())!
  await page.mouse.move(s1.x + s1.width / 2, s1.y + s1.height * 0.9, { steps: 12 })
  await page.waitForTimeout(200)
  await expect(page.getByTestId('canvas-drop-indicator')).toHaveAttribute('data-parent-id', 'B')
  await expect(page.getByTestId('canvas-drop-indicator')).toHaveAttribute('data-index', '1')
  await page.mouse.up()
  await page.waitForTimeout(300)

  await expect(canvas.getByText('Lib section')).toHaveCount(1)
  await expect(drawer.locator('[data-canvas-root]').getByText('Lib section')).toHaveCount(0)
})
