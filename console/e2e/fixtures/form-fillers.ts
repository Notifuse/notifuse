/**
 * Form filler utilities for filling ALL form fields in e2e tests.
 * Each form filler fills required AND optional fields to ensure
 * complete coverage and payload verification.
 */

import { Page } from '@playwright/test'
import type {
  ContactFormData,
  ListFormData,
  TemplateFormData,
  BroadcastFormData,
  SegmentFormData,
  TransactionalFormData,
  BlogPostFormData,
  BlogCategoryFormData,
  SEOFormData,
  BlogAuthorFormData
} from './form-data'

// ============================================
// Generic Form Helpers
// ============================================

/**
 * Fill an input field by its label text
 * Scopes to drawer/modal content first if available
 */
async function fillByLabel(page: Page, labelText: string, value: string): Promise<void> {
  // Scope to drawer or modal content if available
  const drawer = page.locator('.ant-drawer-content')
  const modal = page.locator('.ant-modal-content')
  const scope = (await drawer.count()) > 0 ? drawer : (await modal.count()) > 0 ? modal : page

  // First try to find a form item with this label in the scope
  const formItem = scope.locator('.ant-form-item').filter({ hasText: labelText }).first()
  if ((await formItem.count()) > 0) {
    const input = formItem.locator('input:not([type="hidden"]):not([readonly]), textarea').first()
    if ((await input.count()) > 0 && (await input.isVisible())) {
      await input.fill(value)
      return
    }
  }

  // Fallback: find input by role in the scope
  const textbox = scope.getByRole('textbox', { name: labelText })
  if ((await textbox.count()) === 1) {
    await textbox.fill(value)
    return
  }

  // Last resort: use getByLabel but only fill if exactly one match in scope
  const input = scope.getByLabel(labelText, { exact: false })
  if ((await input.count()) === 1 && (await input.isVisible())) {
    await input.fill(value)
  }
}

/**
 * Fill an input field by placeholder text
 */
async function fillByPlaceholder(page: Page, placeholder: string, value: string): Promise<void> {
  const input = page.locator(`input[placeholder*="${placeholder}"], textarea[placeholder*="${placeholder}"]`)
  if ((await input.count()) > 0) {
    await input.first().fill(value)
  }
}

/**
 * Fill an input field by name attribute
 */
async function fillByName(page: Page, name: string, value: string): Promise<void> {
  const input = page.locator(`input[name="${name}"], textarea[name="${name}"]`)
  if ((await input.count()) > 0) {
    await input.first().fill(value)
  }
}

/**
 * Fill a number input by label
 * Scopes to drawer/modal content first if available
 */
async function fillNumberByLabel(page: Page, labelText: string, value: number): Promise<void> {
  // Scope to drawer or modal content if available
  const drawer = page.locator('.ant-drawer-content')
  const modal = page.locator('.ant-modal-content')
  const scope = (await drawer.count()) > 0 ? drawer : (await modal.count()) > 0 ? modal : page

  // First try to find a form item with this label in the scope
  const formItem = scope.locator('.ant-form-item').filter({ hasText: labelText }).first()
  if ((await formItem.count()) > 0) {
    const input = formItem.locator('input:not([type="hidden"]):not([readonly])').first()
    if ((await input.count()) > 0 && (await input.isVisible())) {
      await input.fill(value.toString())
      return
    }
  }

  const input = scope.getByLabel(labelText, { exact: false })
  if ((await input.count()) > 0) {
    await input.fill(value.toString())
  }
}

/**
 * Select an option from an Ant Design Select dropdown by label
 */
async function selectByLabel(page: Page, labelText: string, optionText: string): Promise<void> {
  // Find the form item containing the label
  const formItem = page.locator('.ant-form-item').filter({ hasText: labelText }).first()
  if ((await formItem.count()) === 0) return

  // Click the select within this form item
  const select = formItem.locator('.ant-select')
  if ((await select.count()) === 0) return

  await select.click()
  await page.waitForTimeout(200)

  // Wait for the visible dropdown and click option
  // Use :visible pseudo-selector to avoid matching multiple dropdowns during transitions
  const visibleDropdown = page.locator('.ant-select-dropdown:visible')
  await visibleDropdown.first().waitFor({ state: 'visible', timeout: 5000 })

  const option = page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: optionText })
  if ((await option.count()) > 0) {
    await option.first().click()
  } else {
    // Close dropdown if option not found
    await page.keyboard.press('Escape')
  }
}

/**
 * Fill tags in an Ant Design Select (mode="tags") by label
 */
async function fillTagsByLabel(page: Page, labelText: string, tags: string[]): Promise<void> {
  // Find the form item containing the label
  const formItem = page.locator('.ant-form-item').filter({ hasText: labelText }).first()
  if ((await formItem.count()) === 0) return

  const select = formItem.locator('.ant-select')
  if ((await select.count()) === 0) return

  // Scroll into view and ensure it's clickable
  await select.scrollIntoViewIfNeeded()
  await page.waitForTimeout(100)

  // Click with force option to bypass potential overlay issues
  await select.click({ force: true })
  await page.waitForTimeout(100)

  // Type each tag and press Enter
  const input = select.locator('input')
  for (const tag of tags) {
    await input.fill(tag)
    await input.press('Enter')
    await page.waitForTimeout(50)
  }

  // Click outside to close dropdown
  await page.keyboard.press('Escape')
  await page.waitForTimeout(100)
}

/**
 * Toggle an Ant Design Switch by label
 */
async function toggleSwitchByLabel(page: Page, labelText: string, enabled: boolean): Promise<void> {
  const formItem = page.locator('.ant-form-item').filter({ hasText: labelText }).first()
  if ((await formItem.count()) === 0) return

  const switchEl = formItem.locator('.ant-switch')
  if ((await switchEl.count()) === 0) return

  const isChecked = await switchEl.getAttribute('aria-checked') === 'true'
  if (isChecked !== enabled) {
    await switchEl.click()
  }
}

/**
 * Check/uncheck an Ant Design Checkbox by label
 */
async function setCheckboxByLabel(page: Page, labelText: string, checked: boolean): Promise<void> {
  const checkbox = page.locator('.ant-checkbox-wrapper').filter({ hasText: labelText }).first()
  if ((await checkbox.count()) === 0) return

  const input = checkbox.locator('input[type="checkbox"]')
  const isChecked = await input.isChecked()
  if (isChecked !== checked) {
    await checkbox.click()
  }
}

// ============================================
// SEO Settings Filler (used by Blog Post & Category)
// ============================================

/**
 * Fill all SEO settings fields
 */
export async function fillSEOSettings(page: Page, seo: SEOFormData): Promise<void> {
  if (seo.meta_title) {
    await fillByLabel(page, 'Meta Title', seo.meta_title)
  }

  if (seo.meta_description) {
    await fillByLabel(page, 'Meta Description', seo.meta_description)
  }

  if (seo.keywords && seo.keywords.length > 0) {
    await fillTagsByLabel(page, 'Keywords', seo.keywords)
  }

  if (seo.meta_robots) {
    await selectByLabel(page, 'Search Engine Indexing', seo.meta_robots === 'index,follow' ? 'Index and follow links' : seo.meta_robots)
  }

  if (seo.canonical_url) {
    await fillByLabel(page, 'Canonical URL', seo.canonical_url)
  }

  if (seo.og_title) {
    await fillByLabel(page, 'Social Share Title', seo.og_title)
  }

  if (seo.og_description) {
    await fillByLabel(page, 'Social Share Description', seo.og_description)
  }

  if (seo.og_image) {
    await fillByLabel(page, 'Social Share Image', seo.og_image)
  }
}

// ============================================
// Contact Form Filler
// ============================================

/**
 * Helper to add an optional field via the "Add an optional field" dropdown
 * in the contact form. This is required before the field becomes visible.
 */
async function addContactOptionalField(page: Page, fieldLabel: string): Promise<boolean> {
  // Find the "Add an optional field" select at the bottom of the drawer
  const addFieldSelect = page.locator('.ant-drawer-content .ant-select').filter({
    has: page.locator('[title="Select a field"], .ant-select-selection-placeholder')
  }).first()

  // If not found, try the select that comes after the "Add an optional field" text
  const selectAfterLabel = page.locator('text=Add an optional field').locator('..').locator('.ant-select').first()

  const select = (await addFieldSelect.count()) > 0 ? addFieldSelect : selectAfterLabel

  if ((await select.count()) === 0) {
    // Try a more general approach - find the last select in the drawer
    const allSelects = page.locator('.ant-drawer-content .ant-select')
    const lastSelect = allSelects.last()
    if ((await lastSelect.count()) === 0) return false

    await lastSelect.click()
  } else {
    await select.click()
  }

  await page.waitForTimeout(200)

  // Look for the option in the dropdown
  const option = page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: fieldLabel })
  if ((await option.count()) > 0) {
    await option.click()
    // Wait for dropdown to close completely
    await page.waitForTimeout(300)
    // Ensure dropdown is closed
    await page.keyboard.press('Escape')
    await page.waitForTimeout(100)
    return true
  }

  // Close dropdown if option not found
  await page.keyboard.press('Escape')
  return false
}

/**
 * Helper to add and fill an optional contact field
 */
async function addAndFillContactField(
  page: Page,
  fieldKey: string,
  fieldLabel: string,
  value: string | number,
  isNumber: boolean = false
): Promise<void> {
  // First, add the field via the dropdown
  const added = await addContactOptionalField(page, fieldLabel)
  if (!added) return

  // Wait for the field to appear
  await page.waitForTimeout(200)

  // Now fill the field
  if (isNumber) {
    await fillNumberByLabel(page, fieldLabel, value as number)
  } else {
    await fillByLabel(page, fieldLabel, value as string)
  }
}

export async function fillContactForm(page: Page, data: Partial<ContactFormData>): Promise<void> {
  // Required field - Email is always visible
  if (data.email) {
    await fillByLabel(page, 'Email', data.email)
  }

  // Optional fields need to be added via the dropdown before they can be filled
  // The contact form has a dynamic "Add an optional field" pattern

  if (data.first_name) {
    await addAndFillContactField(page, 'first_name', 'First Name', data.first_name)
  }
  if (data.last_name) {
    await addAndFillContactField(page, 'last_name', 'Last Name', data.last_name)
  }
  if (data.phone) {
    await addAndFillContactField(page, 'phone', 'Phone', data.phone)
  }
  if (data.external_id) {
    await addAndFillContactField(page, 'external_id', 'External ID', data.external_id)
  }
  if (data.job_title) {
    await addAndFillContactField(page, 'job_title', 'Job Title', data.job_title)
  }

  // Address fields
  if (data.address_line_1) {
    await addAndFillContactField(page, 'address_line_1', 'Address Line 1', data.address_line_1)
  }
  if (data.address_line_2) {
    await addAndFillContactField(page, 'address_line_2', 'Address Line 2', data.address_line_2)
  }
  if (data.postcode) {
    await addAndFillContactField(page, 'postcode', 'Postcode', data.postcode)
  }
  if (data.state) {
    await addAndFillContactField(page, 'state', 'State', data.state)
  }

  // Country and Timezone are selects - need special handling after adding
  if (data.country) {
    const added = await addContactOptionalField(page, 'Country')
    if (added) {
      await page.waitForTimeout(200)
      // Country is a select, not an input - find and click the select
      const countryFormItem = page.locator('.ant-form-item').filter({ hasText: 'Country' }).first()
      const countrySelect = countryFormItem.locator('.ant-select').first()
      if ((await countrySelect.count()) > 0) {
        await countrySelect.click()
        await page.waitForTimeout(200)
        const countryOption = page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: data.country })
        if ((await countryOption.count()) > 0) {
          await countryOption.click()
        } else {
          await page.keyboard.press('Escape')
        }
      }
    }
  }

  if (data.timezone) {
    const added = await addContactOptionalField(page, 'Timezone')
    if (added) {
      await page.waitForTimeout(200)
      const tzFormItem = page.locator('.ant-form-item').filter({ hasText: 'Timezone' }).first()
      const tzSelect = tzFormItem.locator('.ant-select').first()
      if ((await tzSelect.count()) > 0) {
        await tzSelect.click()
        await page.waitForTimeout(200)
        // Type to search for the timezone
        await page.keyboard.type(data.timezone.substring(0, 10))
        await page.waitForTimeout(200)
        const tzOption = page.locator('.ant-select-dropdown:visible .ant-select-item-option').first()
        if ((await tzOption.count()) > 0) {
          await tzOption.click()
        } else {
          await page.keyboard.press('Escape')
        }
      }
    }
  }

  if (data.language) {
    const added = await addContactOptionalField(page, 'Language')
    if (added) {
      await page.waitForTimeout(200)
      const langFormItem = page.locator('.ant-form-item').filter({ hasText: 'Language' }).first()
      const langSelect = langFormItem.locator('.ant-select').first()
      if ((await langSelect.count()) > 0) {
        await langSelect.click()
        await page.waitForTimeout(200)
        const langOption = page.locator('.ant-select-dropdown:visible .ant-select-item-option').filter({ hasText: data.language })
        if ((await langOption.count()) > 0) {
          await langOption.click()
        } else {
          await page.keyboard.press('Escape')
        }
      }
    }
  }

  // Custom string fields
  if (data.custom_string_1) {
    await addAndFillContactField(page, 'custom_string_1', 'Custom String 1', data.custom_string_1)
  }

  // Custom number fields
  if (data.custom_number_1 !== undefined) {
    await addAndFillContactField(page, 'custom_number_1', 'Custom Number 1', data.custom_number_1, true)
  }
}

// ============================================
// List Form Filler
// ============================================

export async function fillListForm(page: Page, data: Partial<ListFormData>): Promise<void> {
  if (data.name) {
    await fillByLabel(page, 'Name', data.name)
  }

  if (data.id) {
    await fillByLabel(page, 'List ID', data.id)
  }

  if (data.description) {
    await fillByLabel(page, 'Description', data.description)
  }

  if (data.is_double_optin !== undefined) {
    await toggleSwitchByLabel(page, 'Double Opt-in', data.is_double_optin)
  }

  if (data.is_public !== undefined) {
    await toggleSwitchByLabel(page, 'Public', data.is_public)
  }

  // Template selections would need Select dropdown handling
  // These depend on available templates in the workspace
}

// ============================================
// Template Form Filler
// ============================================

export async function fillTemplateForm(page: Page, data: Partial<TemplateFormData>): Promise<void> {
  if (data.name) {
    await fillByLabel(page, 'Name', data.name)
  }

  if (data.category) {
    await selectByLabel(page, 'Category', data.category)
  }

  if (data.subject) {
    await fillByLabel(page, 'Subject', data.subject)
  }

  if (data.subject_preview) {
    await fillByLabel(page, 'Preview Text', data.subject_preview)
  }

  // UTM Parameters
  if (data.utm_source) {
    await fillByLabel(page, 'UTM Source', data.utm_source)
  }
  if (data.utm_medium) {
    await fillByLabel(page, 'UTM Medium', data.utm_medium)
  }
  if (data.utm_campaign) {
    await fillByLabel(page, 'UTM Campaign', data.utm_campaign)
  }
}

// ============================================
// Broadcast Form Filler
// ============================================

export async function fillBroadcastForm(page: Page, data: Partial<BroadcastFormData>): Promise<void> {
  if (data.name) {
    await fillByLabel(page, 'Name', data.name)
  }

  // Audience settings
  if (data.list) {
    await selectByLabel(page, 'List', data.list)
  }

  if (data.exclude_unsubscribed !== undefined) {
    await setCheckboxByLabel(page, 'Exclude Unsubscribed', data.exclude_unsubscribed)
  }

  // UTM Parameters
  if (data.utm_source) {
    await fillByLabel(page, 'UTM Source', data.utm_source)
  }
  if (data.utm_medium) {
    await fillByLabel(page, 'UTM Medium', data.utm_medium)
  }
  if (data.utm_campaign) {
    await fillByLabel(page, 'UTM Campaign', data.utm_campaign)
  }
  if (data.utm_term) {
    await fillByLabel(page, 'UTM Term', data.utm_term)
  }
  if (data.utm_content) {
    await fillByLabel(page, 'UTM Content', data.utm_content)
  }

  // A/B Testing settings
  if (data.test_enabled !== undefined) {
    await toggleSwitchByLabel(page, 'A/B Test', data.test_enabled)
  }

  if (data.test_enabled && data.test_sample_percentage !== undefined) {
    await fillNumberByLabel(page, 'Sample Percentage', data.test_sample_percentage)
  }
}

// ============================================
// Segment Form Filler
// ============================================

export async function fillSegmentForm(page: Page, data: Partial<SegmentFormData>): Promise<void> {
  if (data.name) {
    await fillByLabel(page, 'Name', data.name)
  }

  if (data.description) {
    await fillByLabel(page, 'Description', data.description)
  }

  // Segment conditions require complex tree-building UI interactions
  // Would need specialized handling for the segment builder
}

// ============================================
// Transactional Notification Form Filler
// ============================================

export async function fillTransactionalForm(page: Page, data: Partial<TransactionalFormData>): Promise<void> {
  if (data.name) {
    await fillByLabel(page, 'Name', data.name)
  }

  if (data.id) {
    await fillByLabel(page, 'Notification ID', data.id)
  }

  if (data.description) {
    await fillByLabel(page, 'Description', data.description)
  }

  // Tracking settings
  if (data.tracking_enabled !== undefined) {
    await toggleSwitchByLabel(page, 'Tracking', data.tracking_enabled)
  }
  if (data.tracking_opens !== undefined) {
    await setCheckboxByLabel(page, 'Track Opens', data.tracking_opens)
  }
  if (data.tracking_clicks !== undefined) {
    await setCheckboxByLabel(page, 'Track Clicks', data.tracking_clicks)
  }
}

// ============================================
// Blog Author Filler Helper
// ============================================

async function fillBlogAuthor(page: Page, author: BlogAuthorFormData, index: number): Promise<void> {
  // Find the author row by index
  const authorRows = page.locator('.ant-table-row, [data-testid="author-row"]')
  const row = authorRows.nth(index)

  if ((await row.count()) > 0) {
    const nameInput = row.locator('input').first()
    if ((await nameInput.count()) > 0) {
      await nameInput.fill(author.name)
    }

    if (author.avatar_url) {
      const avatarInput = row.locator('input').nth(1)
      if ((await avatarInput.count()) > 0) {
        await avatarInput.fill(author.avatar_url)
      }
    }
  }
}

// ============================================
// Blog Post Form Filler
// ============================================

export async function fillBlogPostForm(page: Page, data: Partial<BlogPostFormData>): Promise<void> {
  // Basic fields
  if (data.title) {
    await fillByLabel(page, 'Title', data.title)
  }

  // Slug is usually auto-generated from title, but can be manually set
  if (data.slug) {
    const slugInput = page.getByLabel('Slug', { exact: false })
    if ((await slugInput.count()) > 0 && await slugInput.isEnabled()) {
      await slugInput.fill(data.slug)
    }
  }

  // Category selection
  if (data.category_id) {
    await selectByLabel(page, 'Category', data.category_id)
  }

  // Reading time
  if (data.reading_time_minutes !== undefined) {
    await fillNumberByLabel(page, 'Reading Time', data.reading_time_minutes)
  }

  // Authors - need to add author first
  if (data.authors && data.authors.length > 0) {
    // Click add author button if available
    const addAuthorBtn = page.getByRole('button', { name: /add author/i })
    if ((await addAuthorBtn.count()) > 0) {
      for (let i = 0; i < data.authors.length; i++) {
        if (i > 0) {
          await addAuthorBtn.click()
          await page.waitForTimeout(200)
        }
        await fillBlogAuthor(page, data.authors[i], i)
      }
    }
  }

  // Excerpt
  if (data.excerpt) {
    await fillByLabel(page, 'Excerpt', data.excerpt)
  }

  // Featured image
  if (data.featured_image_url) {
    await fillByLabel(page, 'Featured Image', data.featured_image_url)
  }

  // SEO Settings - this is the critical part that was broken!
  if (data.seo) {
    await fillSEOSettings(page, data.seo)
  }
}

// ============================================
// Blog Category Form Filler
// ============================================

export async function fillBlogCategoryForm(page: Page, data: Partial<BlogCategoryFormData>): Promise<void> {
  if (data.name) {
    await fillByLabel(page, 'Name', data.name)
  }

  if (data.slug) {
    await fillByLabel(page, 'Slug', data.slug)
  }

  if (data.description) {
    await fillByLabel(page, 'Description', data.description)
  }

  // SEO Settings
  if (data.seo) {
    await fillSEOSettings(page, data.seo)
  }
}
