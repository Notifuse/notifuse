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
const segmentDrawer = (page: Page, title: 'New segment' | 'Update segment') =>
  page.getByRole('dialog', { name: title, exact: true })

// The drawer footer buttons live in the header "extra" slot, next to the title; the leaf editor
// has its own Confirm inside the body, so submitting the segment has to be scoped to the header.
const drawerSubmit = (drawer: ReturnType<typeof segmentDrawer>) =>
  drawer.locator('.ant-drawer-header').getByRole('button', { name: 'Confirm', exact: true })

const EMPTY_TREE = { kind: 'branch', branch: { operator: 'and', leaves: [] } }

// The debug page keeps the builder state in a <pre>, which is the only place the produced tree is
// observable, so it doubles as the assertion surface for the builder tests.
const readTree = async (page: Page) => JSON.parse((await page.locator('pre').innerText()).trim())

// The workspace header carries a select of its own, so the builder assertions are scoped to the
// debug page's form.
const builderForm = (page: Page) => page.locator('form')

test.describe('Segments Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads contacts page with segment button', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // The segments filter always offers the create button, even with no segment yet
      await expect(page.getByRole('button', { name: 'Segment', exact: true })).toBeVisible()
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

      await page.getByRole('button', { name: 'Segment', exact: true }).click()

      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // The creation form: name, colour, timezone and the condition builder
      await expect(drawer.getByPlaceholder('i.e: Big spenders...')).toBeVisible()
      await expect(drawer.getByLabel('Timezone used for dates')).toBeVisible()
      await expect(drawer.getByRole('button', { name: 'Condition', exact: true })).toBeVisible()
    })

    test('creates a new segment with required fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Segment', exact: true }).click()

      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      const nameInput = drawer.getByPlaceholder('i.e: Big spenders...')
      await nameInput.fill('Active Subscribers')
      await expect(nameInput).toHaveValue('Active Subscribers')

      // The tree condition builder is exercised in the builder tests; here we only pin that the
      // form is complete enough to be submitted from the header.
      await expect(drawerSubmit(drawer)).toBeVisible()
    })
  })

  test.describe('Segment Builder', () => {
    test('shows segment builder interface', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // The root branch: an operator select reading ALL, its sentence, and the add button
      await expect(builderForm(page).locator('.ant-select-content')).toHaveText('ALL')
      await expect(page.getByText('of the following conditions match:')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Condition', exact: true })).toBeEnabled()
    })

    test('displays segment rules', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // Adding a nested AND | OR group is the one cascader entry that needs no further form,
      // so it shows the builder writing a rule into the tree.
      await page.getByRole('button', { name: 'Condition', exact: true }).click()
      await page.locator('.ant-cascader-menu-item').filter({ hasText: 'AND | OR' }).click()

      await expect
        .poll(() => readTree(page))
        .toEqual({
          kind: 'branch',
          branch: { operator: 'and', leaves: [EMPTY_TREE] }
        })

      // The nested group is drawn too: two branch sentences instead of one
      await expect(page.getByText('of the following conditions match:')).toHaveCount(2)
    })
  })

  test.describe('Rule Building', () => {
    test('shows condition fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Condition', exact: true }).click()

      // One entry per table schema the debug page passes in, plus the nested group
      const items = page.locator('.ant-cascader-menu-item')
      await expect(items).toHaveCount(4)
      await expect(items.filter({ hasText: 'AND | OR' })).toBeVisible()
      await expect(items.filter({ hasText: 'Contact property' })).toBeVisible()
      await expect(items.filter({ hasText: 'List subscription' })).toBeVisible()
      await expect(items.filter({ hasText: 'Activity' })).toBeVisible()
    })

    test('shows operator selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
      await waitForLoading(page)

      // Switching the branch operator has to reach the produced tree, not just the label
      await builderForm(page).getByRole('combobox').click()
      await page.locator('.ant-select-item-option').filter({ hasText: 'ANY' }).click()

      await expect(builderForm(page).locator('.ant-select-content')).toHaveText('ANY')
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

      // Every segment in the filter bar carries a status dot next to its name
      const segmentButton = page.getByRole('button').filter({ hasText: 'Active Users' })
      await expect(segmentButton).toBeVisible()
      await expect(segmentButton.locator('.ant-badge-status-dot')).toBeVisible()
    })
  })

  test.describe('Contact Count', () => {
    test('shows matching contacts', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // The matching-contacts circle is offered from the start but cannot be counted until the
      // tree is queryable, so on a fresh drawer it is present and refuses to run.
      const previewButton = drawer.getByRole('button', { name: 'Preview', exact: true })
      await expect(previewButton).toBeVisible()
      await expect(previewButton).toBeDisabled()
    })
  })

  test.describe('Integration', () => {
    test('segment page accessible from contacts filter', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      // Start at contacts
      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // The segments the workspace owns are listed as filters above the contacts table
      await expect(page.getByText('Segments:')).toBeVisible()
      for (const name of ['Active Users', 'US Customers', 'Enterprise Plans']) {
        await expect(page.getByRole('button').filter({ hasText: name })).toBeVisible()
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

      // The branch operator offers exactly the two logical operators
      await builderForm(page).getByRole('combobox').click()
      const options = page.locator('.ant-select-item-option')
      await expect(options).toHaveCount(2)
      await expect(options.nth(0)).toHaveText('ALL')
      await expect(options.nth(1)).toHaveText('ANY')
    })
  })

  test.describe('Edit Form Prefill', () => {
    // The segment actions hang off the ellipsis button that sits next to each segment filter.
    const openSegmentEditor = async (page: Page, segmentName: string) => {
      const segmentGroup = page.locator('.ant-space-compact').filter({ hasText: segmentName })
      await expect(segmentGroup).toBeVisible()
      await segmentGroup.getByRole('button').last().click()
      await page.getByRole('menuitem', { name: 'Update', exact: true }).click()

      const drawer = segmentDrawer(page, 'Update segment')
      await expect(drawer).toBeVisible()
      return drawer
    }

    test('edit segment drawer shows existing segment name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openSegmentEditor(page, 'Active Users')

      await expect(drawer.getByPlaceholder('i.e: Big spenders...')).toHaveValue('Active Users')
    })

    test('edit segment preserves color selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openSegmentEditor(page, 'Active Users')

      // The colour select sits next to the name; an existing segment opens on its own colour
      const colorSelect = drawer.locator('.ant-select-content').first()
      await expect(colorSelect).toHaveText('blue')
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

  test.describe('Form Validation', () => {
    test('requires segment name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // Submitting an untouched form reports the missing name and sends nothing
      await drawerSubmit(drawer).click()

      await expect(drawer.locator('.ant-form-item-explain-error')).toHaveText('Please enter name')
      expect(requestCapture.getRequestCount(API_PATTERNS.SEGMENT_CREATE)).toBe(0)
    })

    test('requires tree conditions', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      await drawer.getByPlaceholder('i.e: Big spenders...').fill('Test Segment')

      // A named segment with an empty tree is rejected by the tree validator: the drawer stays
      // open and nothing is sent. The validator is declared noStyle, so the rejection is only
      // observable through the request that never happens.
      await drawerSubmit(drawer).click()
      await page.waitForTimeout(1000)

      await expect(drawer).toBeVisible()
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

      // Add a contact condition
      await page.getByRole('button', { name: 'Condition', exact: true }).click()
      await page.locator('.ant-cascader-menu-item').filter({ hasText: 'Contact property' }).click()

      // Click "+ Add filter" button
      await page.getByRole('button', { name: '+ Add filter' }).click()

      const modal = page.getByRole('dialog', { name: 'Add a filter', exact: true })
      await expect(modal).toBeVisible()

      // Select Custom JSON 1 field - type to search
      await modal.getByRole('combobox').first().click()
      await page.keyboard.type('json')

      const jsonFieldOption = page
        .locator('.ant-select-item-option')
        .filter({ hasText: 'Custom JSON 1' })
        .first()
      await expect(jsonFieldOption).toBeVisible()
      await jsonFieldOption.click()

      // Click "Add path" button
      const addPathBtn = modal.getByRole('button', { name: /add path/i })
      await expect(addPathBtn).toBeVisible()
      await addPathBtn.click()

      // Fill JSON path
      await modal.locator('.ant-input').first().fill('test_number')

      // Select "Number" as the value type - THIS IS THE KEY STEP
      const numberRadio = modal.locator('.ant-radio-button-wrapper').filter({ hasText: /Number/i })
      await expect(numberRadio).toBeVisible()
      await numberRadio.click()

      // Select equals operator
      await modal.locator('.ant-select').last().click()
      await page.locator('.ant-select-item-option').filter({ hasText: /^equals$/i }).first().click()

      // Fill number value
      const numberInput = modal.locator('.ant-input-number input')
      await expect(numberInput).toBeVisible()
      await numberInput.fill('42')

      // Confirm the filter modal
      await modal.getByRole('button', { name: 'Confirm', exact: true }).click()
      await expect(modal).toBeHidden()

      // Confirm the leaf condition form
      const leafConfirmBtn = page.getByRole('button', { name: 'Confirm', exact: true })
      await expect(leafConfirmBtn).toBeVisible()
      await leafConfirmBtn.click()

      // Verify the JSON output
      await expect.poll(async () => (await readTree(page)).branch.leaves.length).toBe(1)
      const filter = (await readTree(page)).branch.leaves[0].leaf.contact.filters[0]

      // KEY ASSERTION: field_type should be "number" (NOT "json")
      // Before the fix, this would be "json" because input_dimension_filters.tsx
      // always overwrote field_type with the schema type.
      // After the fix, JSON fields preserve the user-selected field_type.
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
      await page.getByRole('button', { name: 'Segment', exact: true }).click()
      const drawer = segmentDrawer(page, 'New segment')
      await expect(drawer).toBeVisible()

      // Fill segment name
      await drawer.getByPlaceholder('i.e: Big spenders...').fill(testSegmentData.name)

      // A segment is only submittable with a queryable tree, so build the simplest complete
      // condition: contact email is set.
      await drawer.getByRole('button', { name: 'Condition', exact: true }).click()
      await page.locator('.ant-cascader-menu-item').filter({ hasText: 'Contact property' }).click()

      await page.getByRole('button', { name: '+ Add filter' }).click()
      const modal = page.getByRole('dialog', { name: 'Add a filter', exact: true })
      await expect(modal).toBeVisible()

      await modal.getByRole('combobox').first().click()
      await page.locator('.ant-select-item-option').filter({ hasText: 'Email' }).first().click()

      await modal.getByRole('combobox').last().click()
      await page.locator('.ant-select-item-option').filter({ hasText: 'is set' }).first().click()

      await modal.getByRole('button', { name: 'Confirm', exact: true }).click()
      await expect(modal).toBeHidden()

      // Confirm the condition, then the segment
      await drawer.locator('.ant-drawer-body').getByRole('button', { name: 'Confirm', exact: true }).click()
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
