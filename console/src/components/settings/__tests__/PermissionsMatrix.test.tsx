import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { PermissionsMatrix } from '../PermissionsMatrix'
import {
  ALL_PERMISSION_RESOURCES,
  PERMISSION_DESCRIPTORS,
  createEmptyPermissions
} from '../../../services/api/permissions'
import type { PermissionResource } from '../../../services/api/permissions'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderMatrix = () => {
  const onChange = vi.fn()
  const utils = render(
    <I18nProvider i18n={i18n}>
      <App>
        <PermissionsMatrix value={createEmptyPermissions()} onChange={onChange} />
      </App>
    </I18nProvider>
  )
  return { ...utils, onChange }
}

// The expander's accessible name names the resource it belongs to, so the fourteen of them are
// told apart by a screen reader — and here, by an exact-name query.
const expander = (label: string) =>
  screen.getByRole('button', { name: `Endpoints gated by ${label}` })

const rowFor = (resource: PermissionResource) => {
  const row = document.querySelector(`tr[data-row-key="${resource}"]`)
  expect(row).not.toBeNull()
  return row as HTMLElement
}

// The resource name as it is read on screen, found by its text rather than through the control
// that wraps it — which is the point: clicking the name has to be clicking the expander.
const resourceName = (resource: PermissionResource, label: string) =>
  within(rowFor(resource)).getByText(label)

// The details render in the table's own flow: rc-table puts the expanded row immediately after
// the row it belongs to. Nothing is portalled, so nothing has to be hunted for on the document.
const detailsFor = (resource: PermissionResource) => {
  const expanded = rowFor(resource).nextElementSibling as HTMLElement
  expect(expanded).not.toBeNull()
  expect(expanded.className).toContain('ant-table-expanded-row')
  expect(expanded.style.display).not.toBe('none')
  return expanded
}

const expand = (resource: PermissionResource, label: string) => {
  fireEvent.click(expander(label))
  return detailsFor(resource)
}

// The Read switch is the first of the row, the Write switch the second.
const switchesFor = (resource: PermissionResource) => {
  const [read, write] = Array.from(
    rowFor(resource).querySelectorAll<HTMLButtonElement>('button[role="switch"]')
  )
  return { read, write }
}

describe('permission descriptors', () => {
  it('describes every resource the matrix renders a row for', () => {
    expect(Object.keys(PERMISSION_DESCRIPTORS).sort()).toEqual(
      [...ALL_PERMISSION_RESOURCES].sort()
    )
  })
})

describe('permission row details', () => {
  it('expands on the resource row and names an endpoint together with what it does', () => {
    renderMatrix()

    const details = expand('contacts', 'Contacts')

    expect(within(details).getByText('/api/contacts.import')).toBeInTheDocument()
    expect(details.textContent).toContain(
      'Bulk create contacts and overwrite every field of every contact in the payload'
    )
    // The scope sentence frames the list.
    expect(details.textContent).toContain(
      'Contact records and their fields, contact timelines, and custom events'
    )
  })

  it('separates the read endpoints from the write ones', () => {
    renderMatrix()

    const details = expand('broadcasts', 'Broadcasts')

    expect(within(details).getByText('Read')).toBeInTheDocument()
    expect(within(details).getByText('Write')).toBeInTheDocument()
    expect(within(details).getByText('/api/broadcasts.schedule')).toBeInTheDocument()
    expect(details.textContent).toContain(
      'Schedule or immediately send a broadcast to its entire audience'
    )
  })

  it('renders the caveat of a resource that reaches further than its name', () => {
    renderMatrix()

    const details = expand('lists', 'Lists')

    expect(within(details).getByText('Watch out')).toBeInTheDocument()
    expect(details.textContent).toContain('Lists write also grants contact writing')
    expect(details.textContent).toContain(
      "creates or overwrites that contact's whole record"
    )
  })

  it('names the delivery log a webhook subscription grant can read', () => {
    renderMatrix()

    const details = expand('webhook_subscriptions', 'Webhook Subscriptions')

    expect(within(details).getByText('/api/webhookSubscriptions.deliveries')).toBeInTheDocument()
    expect(details.textContent).toContain(
      "Read the delivery log, including each event payload and the endpoint's response status and body"
    )
    // The neighbours on the same routes that answer to something else are called out.
    expect(details.textContent).toContain('regenerateSecret is owner-only')
  })

  it('explains an unenforceable verb instead of showing it an empty list', () => {
    renderMatrix()

    const details = expand('llm', 'LLM')

    expect(details.textContent).toContain('This verb gates nothing today')
    expect(details.textContent).toContain(
      '/api/llm.chat is the only LLM endpoint and it answers to LLM write'
    )
    // And says why the switch cannot be turned off.
    expect(details.textContent).toContain('granted and locked')
  })

  it('explains the unenforced verb the matrix leaves switchable', () => {
    renderMatrix()

    const details = expand('webhook_events', 'Webhook Events')

    expect(details.textContent).toContain('This verb gates nothing today')
    expect(details.textContent).toContain(
      'Exactly one inbound webhook event call is permission-checked and it is a read'
    )
  })

  it('reaches the details from the keyboard', async () => {
    const user = userEvent.setup()
    renderMatrix()

    const trigger = expander('Templates')
    trigger.focus()
    expect(document.activeElement).toBe(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'false')

    await user.keyboard('{Enter}')

    expect(expander('Templates')).toHaveAttribute('aria-expanded', 'true')
    expect(
      within(detailsFor('templates')).getByText('/api/templateBlocks.update')
    ).toBeInTheDocument()
  })

  it('renders the endpoints in the table flow rather than in an overlay that can fall off-screen', () => {
    renderMatrix()

    const details = expand('contacts', 'Contacts')

    // Inside the table body, so the host modal's own scroll carries it — there is no overlay left
    // to be positioned, and nothing caps the height into a second scroll region.
    expect(details.closest('table')).not.toBeNull()
    expect(document.querySelector('.ant-popover')).toBeNull()
    expect(details.querySelector('[style*="max-height"]')).toBeNull()

    // The endpoint is plain selectable monospace text, not the label of a control.
    const endpoint = within(details).getByText('/api/contacts.import')
    expect(endpoint.tagName).toBe('CODE')
    expect(endpoint.className).toContain('font-mono')
    expect(endpoint.closest('button')).toBeNull()
    expect(endpoint.closest('[aria-hidden="true"]')).toBeNull()
  })

  it('keeps one resource open at a time, and closes it again on a second click', () => {
    renderMatrix()

    expand('contacts', 'Contacts')
    expect(expander('Contacts')).toHaveAttribute('aria-expanded', 'true')

    expand('lists', 'Lists')
    expect(expander('Contacts')).toHaveAttribute('aria-expanded', 'false')
    expect(expander('Lists')).toHaveAttribute('aria-expanded', 'true')
    // Closed means out of the flow, not merely scrolled past.
    const contacts = document.querySelector('tr[data-row-key="contacts"]')!
      .nextElementSibling as HTMLElement
    expect(contacts.style.display).toBe('none')

    fireEvent.click(expander('Lists'))
    expect(expander('Lists')).toHaveAttribute('aria-expanded', 'false')
  })

  it('toggles the row from the resource name, which is the expander rather than a second control', () => {
    renderMatrix()

    // One control per row, not a chevron and a label that happen to agree: the name is inside
    // the button that carries aria-expanded and the row's accessible name.
    expect(resourceName('templates', 'Templates').closest('button')).toBe(expander('Templates'))
    expect(within(rowFor('templates')).getAllByRole('button')).toHaveLength(1)

    fireEvent.click(resourceName('templates', 'Templates'))

    expect(expander('Templates')).toHaveAttribute('aria-expanded', 'true')
    expect(
      within(detailsFor('templates')).getByText('/api/templateBlocks.update')
    ).toBeInTheDocument()

    fireEvent.click(resourceName('templates', 'Templates'))

    expect(expander('Templates')).toHaveAttribute('aria-expanded', 'false')
    // Collapsed means out of the flow, not merely scrolled past.
    expect((rowFor('templates').nextElementSibling as HTMLElement).style.display).toBe('none')
  })

  it('operates the Read and Write switches without toggling the row', () => {
    const { onChange } = renderMatrix()

    // Collapsed: flipping a switch grants the verb and leaves the row shut.
    fireEvent.click(switchesFor('contacts').read)

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange.mock.calls[0][0].contacts).toEqual({ read: true, write: false })
    expect(expander('Contacts')).toHaveAttribute('aria-expanded', 'false')
    expect(document.querySelector('.ant-table-expanded-row')).toBeNull()

    // Expanded: flipping a switch does not close it either.
    fireEvent.click(resourceName('contacts', 'Contacts'))
    expect(expander('Contacts')).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(switchesFor('contacts').write)

    expect(onChange).toHaveBeenCalledTimes(2)
    expect(onChange.mock.calls[1][0].contacts).toEqual({ read: false, write: true })
    expect(expander('Contacts')).toHaveAttribute('aria-expanded', 'true')
    expect(within(detailsFor('contacts')).getByText('/api/contacts.import')).toBeInTheDocument()
  })

  it('leaves the read-only matrix without expand controls, since it is itself a popover body', () => {
    const { container } = render(
      <I18nProvider i18n={i18n}>
        <App>
          <PermissionsMatrix value={{ contacts: { read: true, write: false } }} />
        </App>
      </I18nProvider>
    )

    // Click-to-expand nested inside the Team page's hover Popover would be unreachable: the
    // overlay closes the moment the pointer leaves the tag.
    expect(screen.queryByRole('button')).toBeNull()
    expect(container.querySelector('[aria-expanded]')).toBeNull()
    expect(container.querySelector('.ant-table-row-expand-icon')).toBeNull()
    expect(container.querySelector('tr.ant-table-expanded-row')).toBeNull()
  })
})
