import type { Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testSegmentData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

// antd 6 dropped .ant-drawer-content / .ant-modal-content: the drawer's role="dialog" now sits on
// div.ant-drawer-section and takes its accessible name from the drawer title. Addressing the
// dialog by role+name therefore pins both "the drawer is open" and "it is the right drawer".
const segmentDrawer = (page: Page, title: 'New segment' | 'Edit segment') =>
  page.getByRole('dialog', { name: title, exact: true })

// The full-screen drawer's bar carries Create (a new segment) or Save (an existing one); the
// condition editor has its own Confirm in the body.
const drawerSubmit = (drawer: ReturnType<typeof segmentDrawer>) =>
  drawer.getByRole('button', { name: /^(Create|Save)$/ })

// The name field sits in the bar.
const nameField = (drawer: ReturnType<typeof segmentDrawer>) => drawer.getByRole('textbox', { name: 'Segment name' })

// What still blocks the save, as the summary lists it.
const summary = (drawer: ReturnType<typeof segmentDrawer>) => drawer.locator('[data-summary-tone]')

const EMPTY_TREE = { kind: 'branch', branch: { operator: 'and', leaves: [] } }

// The debug page keeps the builder state in a <pre>, which is the only place the produced tree is
// observable, so it doubles as the assertion surface for the builder tests.
const readTree = async (page: Page) => JSON.parse((await page.locator('pre').innerText()).trim())

// The workspace header carries a select of its own, so the builder assertions are scoped to the
// debug page's form.
const builderForm = (page: Page) => page.locator('form')

// The all / at least one control of the root group.
const rootOperator = (page: Page) => builderForm(page).locator('.ant-segmented').first()

// The add menu lists one entry per condition type the page passes in.
const addConditionMenu = async (page: Page) => {
  await builderForm(page).getByRole('button', { name: 'Condition', exact: true }).click()
  // Scoped to the dropdown: the sidebar is a menu too
  return page.locator('.ant-dropdown:not(.ant-dropdown-hidden)').getByRole('menuitem')
}

// The field picker of the filter row being edited: typing narrows it, then the option is clicked.
const pickField = async (page: Page, search: string, label: string) => {
  const picker = page.locator('.ant-select').filter({ hasText: 'Choose a field' }).first()
  await picker.click()
  await page.keyboard.type(search)
  await page.locator('.ant-select-item-option').filter({ hasText: label }).first().click()
}

const pickOperator = async (page: Page, label: RegExp) => {
  await page.locator('.ant-select').filter({ hasText: 'Operator' }).last().click()
  await page.locator('.ant-select-item-option').filter({ hasText: label }).first().click()
}

test.describe('Segments Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads contacts page with segment button', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // The rail's Segments section always offers the create button, even with no segment yet
      await expect(page.getByRole('button', { name: 'New segment', exact: true })).toBeVisible()
      await expect(page.getByText('No segments yet')).toBeVisible()
    })

    test('loads debug segment page', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      await expect(page.getByRole('heading', { name: 'Debug Segment Builder' })).toBeVisible()
      await expect(page.getByText('Segment Conditions')).toBeVisible()
    })

    test('loads segment page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // The builder starts on an empty AND branch, and the JSON card mirrors it
      await expect(page.getByText('Current Segment Tree JSON')).toBeVisible()
      expect(await readTree(page)).toEqual(EMPTY_TREE)
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create segment drawer from contacts page', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New segment', exact: true }).click()

      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // The creation form: name and colour in the bar, timezone and the condition builder
      await expect(nameField(drawer)).toBeVisible()
      await expect(drawer.getByLabel('Timezone used for dates')).toBeVisible()
      await expect(drawer.getByRole('button', { name: 'Condition', exact: true })).toBeVisible()
    })

    test('creates a new segment with required fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New segment', exact: true }).click()

      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      const nameInput = nameField(drawer)
      await nameInput.fill('Active Subscribers')
      await expect(nameInput).toHaveValue('Active Subscribers')

      // Without a condition, Create waits and the summary says why
      await expect(drawerSubmit(drawer)).toBeDisabled()
      await expect(summary(drawer).filter({ hasText: 'Add at least one condition.' })).toBeVisible()
    })
  })

  test.describe('Segment Builder', () => {
    test('shows segment builder interface', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // The root group: its all / at least one control on « all », its sentence, and the add button
      await expect(rootOperator(page).locator('.ant-segmented-item-selected')).toHaveText('all')
      await expect(page.getByText('of the following conditions', { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Condition', exact: true })).toBeEnabled()
    })

    test('displays segment rules', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // Adding a nested group needs no further form, so it shows the builder writing a rule
      // into the tree.
      await builderForm(page).getByRole('button', { name: 'Group', exact: true }).click()

      await expect
        .poll(() => readTree(page))
        .toEqual({
          kind: 'branch',
          branch: { operator: 'and', leaves: [EMPTY_TREE] }
        })

      // The nested group is drawn too, empty
      await expect(page.getByText('Group:')).toBeVisible()
      await expect(page.getByText('Empty: add a condition or a subgroup.')).toBeVisible()
    })
  })

  test.describe('Rule Building', () => {
    test('shows condition fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // One entry per table schema the debug page passes in; groups have their own button
      const items = await addConditionMenu(page)
      await expect(items).toHaveCount(3)
      await expect(items.filter({ hasText: 'Contact property' })).toBeVisible()
      await expect(items.filter({ hasText: 'List subscription' })).toBeVisible()
      await expect(items.filter({ hasText: 'Activity' })).toBeVisible()
    })

    test('shows operator selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // Switching the group operator has to reach the produced tree, not just the label
      await rootOperator(page).getByText('at least one', { exact: true }).click()

      await expect(rootOperator(page).locator('.ant-segmented-item-selected')).toHaveText('at least one')
      await expect.poll(() => readTree(page)).toEqual({
        kind: 'branch',
        branch: { operator: 'or', leaves: [] }
      })
    })
  })

  test.describe('Segment Status', () => {
    test('displays segment status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Every segment in the rail carries its colour dot and, once computed, its contacts count
      const segmentItem = page.locator('[data-rail-item]').filter({ hasText: 'Active Users' })
      await expect(segmentItem).toBeVisible()
      await expect(segmentItem.locator('span.rounded-full').first()).toBeVisible()
    })
  })

  test.describe('Contact Count', () => {
    test('shows matching contacts', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // The count card offers to count from the start but cannot until the tree is queryable,
      // so on a fresh drawer the button is present and refuses to run.
      const previewButton = drawer.getByRole('button', { name: 'Preview', exact: true })
      await expect(previewButton).toBeVisible()
      await expect(previewButton).toBeDisabled()
    })
  })

  test.describe('Integration', () => {
    test('segments are listed in the contacts rail', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      // Start at contacts
      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // The segments the workspace owns are views in the rail, beside the lists
      for (const name of ['Active Users', 'US Customers', 'Enterprise Plans']) {
        await expect(page.locator('[data-rail-item]').filter({ hasText: name })).toBeVisible()
      }
    })
  })

  test.describe('Navigation', () => {
    test('navigates to debug segment', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      // Navigate to debug segment
      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // Should be on debug segment page
      await expect(page).toHaveURL(/debug-segment/)
      await expect(page.getByRole('heading', { name: 'Debug Segment Builder' })).toBeVisible()
    })
  })

  test.describe('Form Elements', () => {
    test('shows add condition button', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      await expect(page.getByRole('button', { name: 'Condition', exact: true })).toBeEnabled()
      // Nothing is editing yet, so the empty tree offers no delete/confirm controls
      await expect(page.getByRole('button', { name: 'Confirm', exact: true })).toHaveCount(0)
    })

    test('shows logical operators', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // The group operator offers exactly the two logical operators
      const options = rootOperator(page).locator('.ant-segmented-item')
      await expect(options).toHaveCount(2)
      await expect(options.nth(0)).toHaveText('all')
      await expect(options.nth(1)).toHaveText('at least one')
    })
  })

  test.describe('Edit Form Prefill', () => {
    // A segment opens as a view from the rail; its « Defined by » bar carries « Edit segment ».
    const openSegmentEditor = async (page: Page, segmentName: string) => {
      await page.locator('[data-rail-item]').filter({ hasText: segmentName }).click()
      await page.getByRole('button', { name: 'Edit segment', exact: true }).click()

      const drawer = segmentDrawer(page, 'Edit segment')
      await expect(drawer).toBeVisible()
      return drawer
    }

    test('edit segment drawer shows existing segment name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openSegmentEditor(page, 'Active Users')

      await expect(nameField(drawer)).toHaveValue('Active Users')
    })

    test('edit segment preserves color selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openSegmentEditor(page, 'Active Users')

      // The colour dot sits before the name; an existing segment opens on its own colour
      await expect(drawer.getByRole('button', { name: 'Colour: Blue', exact: true })).toBeVisible()
    })

    test('edit segment preserves timezone selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openSegmentEditor(page, 'Active Users')

      // Timezone falls back to the workspace timezone when the segment carries none
      await expect(drawer.getByLabel('Timezone used for dates')).toBeVisible()
      await expect(
        drawer.locator('.ant-form-item').filter({ hasText: 'Timezone used for dates' }).locator('.ant-select-content')
      ).toHaveText('UTC')
    })
  })

  test.describe('Closing with changes', () => {
    test('Escape asks before discarding a renamed segment', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)
      await page.locator('[data-rail-item]').filter({ hasText: 'Active Users' }).click()
      await page.getByRole('button', { name: 'Edit segment', exact: true }).click()

      const drawer = segmentDrawer(page, 'Edit segment')
      await nameField(drawer).fill('Active Users renamed')
      await page.keyboard.press('Escape')

      // One confirmation, not two (× and the blocker share one question)
      await expect(page.locator('.ant-modal-confirm')).toHaveCount(1)
      await expect(page.locator('.ant-modal-confirm')).toContainText('Discard your changes?')
      await page.getByRole('button', { name: 'Keep editing' }).click()
      await expect(drawer).toBeVisible()
      await expect(nameField(drawer)).toHaveValue('Active Users renamed')
    })
  })

  test.describe('Form Validation', () => {
    test('requires segment name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // An untouched form cannot be submitted: the summary names what is missing
      await expect(drawerSubmit(drawer)).toBeDisabled()
      await expect(summary(drawer).filter({ hasText: 'Give the segment a name.' })).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.SEGMENT_CREATE)).toBe(0)
    })

    test('requires tree conditions', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      await nameField(drawer).fill('Test Segment')

      // A named segment with an empty tree cannot be created, and says so (it used to do nothing)
      await expect(drawerSubmit(drawer)).toBeDisabled()
      await expect(summary(drawer).filter({ hasText: 'Add at least one condition.' })).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.SEGMENT_CREATE)).toBe(0)
    })
  })

  test.describe('JSON Field Type Handling (Issue #140)', () => {
    test('preserves number field_type for JSON fields (not overwritten to json)', async ({
      authenticatedPage
    }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // Add a contact condition: it opens on an empty filter row
      const items = await addConditionMenu(page)
      await items.filter({ hasText: 'Contact property' }).click()

      await pickField(page, 'json', 'Custom JSON 1')

      // The path to read: one key
      await page.getByRole('button', { name: /add path/i }).click()
      await page.getByPlaceholder('key or index').fill('test_number')
      await page.keyboard.press('Enter')

      // Read the value as a number - THIS IS THE KEY STEP
      await builderForm(page).locator('.ant-segmented-item').filter({ hasText: /^Number$/ }).click()

      await pickOperator(page, /^equals$/)

      const numberInput = page.getByPlaceholder('Enter a number')
      await expect(numberInput).toBeVisible()
      await numberInput.fill('42')

      // Confirm the leaf condition form
      const leafConfirmBtn = page.getByRole('button', { name: 'Confirm', exact: true })
      await expect(leafConfirmBtn).toBeVisible()
      await leafConfirmBtn.click()

      // Verify the JSON output
      await expect.poll(async () => (await readTree(page)).branch.leaves.length).toBe(1)
      const filter = (await readTree(page)).branch.leaves[0].leaf.contact.filters[0]

      // KEY ASSERTION: field_type should be "number" (NOT "json")
      // A JSON row saves the type its value is read as, never the column's « json ».
      expect(filter).toBeDefined()
      expect(filter.field_type).toBe('number')
      expect(filter.number_values).toBeDefined()
      expect(filter.number_values).toContain(42)
      expect(filter.json_path).toContain('test_number')
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('creates segment with name and verifies payload', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Open segment drawer
      await page.getByRole('button', { name: 'New segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // Fill segment name
      await nameField(drawer).fill(testSegmentData.name)

      // A segment is only submittable with a queryable tree, so build the simplest complete
      // condition: contact email is set.
      await drawer.getByRole('button', { name: 'Condition', exact: true }).click()
      await page
        .locator('.ant-dropdown:not(.ant-dropdown-hidden)')
        .getByRole('menuitem')
        .filter({ hasText: 'Contact property' })
        .click()

      await pickField(page, 'email', 'Email')
      await pickOperator(page, /^is set$/)

      // Confirm the condition, then the segment
      // While the condition is open, Create waits for it
      await expect(drawerSubmit(drawer)).toBeDisabled()
      await expect(summary(drawer).filter({ hasText: 'A condition is open: confirm or cancel it.' })).toBeVisible()

      await drawer.locator('.ant-drawer-body').getByRole('button', { name: 'Confirm', exact: true }).click()
      await expect(drawerSubmit(drawer)).toBeEnabled()
      await drawerSubmit(drawer).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.SEGMENT_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      // Verify segment data was sent
      const request = requestCapture.getLastRequest(API_PATTERNS.SEGMENT_CREATE)
      expect(request?.body, 'Segment create body should not be empty').toBeTruthy()
      const body = request!.body as Record<string, unknown>
      expect(body.name).toBe(testSegmentData.name)
      expect(body.id).toBe('e2e_test_segment')
      expect(body.timezone).toBe('UTC')
      expect(body.tree).toEqual({
        kind: 'branch',
        branch: {
          operator: 'and',
          leaves: [
            {
              kind: 'leaf',
              leaf: {
                source: 'contacts',
                contact: {
                  filters: [{ field_name: 'email', field_type: 'string', operator: 'is_set' }]
                }
              }
            }
          ]
        }
      })
    })
  })
})
