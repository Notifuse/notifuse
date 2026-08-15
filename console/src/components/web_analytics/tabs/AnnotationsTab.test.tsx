import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Annotation } from '../../../services/api/annotation'

// src/lib/timezones.ts reads window.TIMEZONES once, at module load: the real list
// arrives from /config.js, which never runs here. Seeded before the imports below
// are evaluated so the Select has options to pick from.
vi.hoisted(() => {
  window.TIMEZONES = ['UTC', 'Asia/Tokyo', 'America/New_York']
})

vi.mock('../../../services/api/annotation', async () => {
  const actual = await vi.importActual<
    typeof import('../../../services/api/annotation')
  >('../../../services/api/annotation')
  return {
    ...actual,
    annotationService: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn()
    }
  }
})

const webAnalyticsContext = {
  workspaceId: 'ws1',
  timezone: 'UTC'
}
vi.mock('../context', () => ({
  useWebAnalytics: () => webAnalyticsContext
}))

import { annotationService } from '../../../services/api/annotation'
import { AnnotationsTab } from './AnnotationsTab'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const tokyoAnnotation: Annotation = {
  id: 'a1',
  annotated_at: '2026-08-15T00:00:00Z',
  timezone: 'Asia/Tokyo',
  title: 'Product launch',
  description: 'Version 2 went live',
  color: '#22c55e',
  source: 'manual',
  created_at: '2026-08-14T10:00:00Z',
  updated_at: '2026-08-14T10:00:00Z'
}

const broadcastAnnotation: Annotation = {
  id: 'a2',
  annotated_at: '2026-08-10T08:00:00Z',
  timezone: 'UTC',
  title: 'Summer sale',
  color: '#7763f1',
  source: 'broadcast',
  source_id: 'bcast1',
  created_at: '2026-08-10T08:00:00Z',
  updated_at: '2026-08-10T08:00:00Z'
}

function renderTab() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <AnnotationsTab />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const desktop = () => within(screen.getByTestId('annotations-desktop'))

describe('AnnotationsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(annotationService.list).mockResolvedValue([])
    // Only Date is faked: the real timers keep antd's animations and RTL's
    // waitFor working, while the "today" the form prefills stays fixed.
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-08-15T00:30:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows an empty state when the workspace has no annotations', async () => {
    renderTab()

    await waitFor(() => expect(annotationService.list).toHaveBeenCalled())
    expect(annotationService.list).toHaveBeenCalledWith({ workspace_id: 'ws1', limit: 1000 })
    expect(desktop().getByText('No annotations yet')).toBeInTheDocument()
  })

  it('renders one row per annotation, in the timezone it was entered in', async () => {
    vi.mocked(annotationService.list).mockResolvedValue([tokyoAnnotation, broadcastAnnotation])
    renderTab()

    // 00:00 UTC is 09:00 in Tokyo: the row must read back what was typed, not
    // the test runner's local equivalent.
    expect(await desktop().findByText('Aug 15, 2026 09:00')).toBeInTheDocument()
    expect(desktop().getByText('Product launch')).toBeInTheDocument()
    expect(desktop().getByText('Version 2 went live')).toBeInTheDocument()
    expect(desktop().getByText('Asia/Tokyo')).toBeInTheDocument()
    expect(desktop().getByText('Aug 10, 2026 08:00')).toBeInTheDocument()

    // Read the inline style rather than toHaveStyle: the suite's setup stubs
    // getComputedStyle out, so the computed value is always empty.
    const dots = desktop().getAllByTestId('annotation-color')
    expect(dots).toHaveLength(2)
    expect(dots[0].style.backgroundColor).toBe('rgb(34, 197, 94)')
  })

  it('creates an annotation at the instant the selected timezone implies', async () => {
    renderTab()
    await waitFor(() => expect(annotationService.list).toHaveBeenCalled())

    fireEvent.click(screen.getByRole('button', { name: /Add/ }))

    // The form opens on the chart timezone (UTC here) and today's date in it.
    expect(await screen.findByLabelText('Timezone')).toBeInTheDocument()
    expect(screen.getByLabelText('Date')).toHaveValue('2026-08-15')
    expect(screen.getByLabelText('Time')).toHaveValue('12:00')

    fireEvent.mouseDown(screen.getByLabelText('Timezone'))
    fireEvent.click(await screen.findByTitle('Asia/Tokyo'))

    fireEvent.click(screen.getByRole('button', { name: '#ef4444' }))
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Keynote' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() => expect(annotationService.create).toHaveBeenCalled())
    expect(annotationService.create).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      // 12:00 in Tokyo, not 12:00 wherever the suite happens to run.
      annotated_at: '2026-08-15T03:00:00.000Z',
      timezone: 'Asia/Tokyo',
      title: 'Keynote',
      description: undefined,
      color: '#ef4444'
    })
  })

  it('reopens an edited annotation on its own timezone', async () => {
    vi.mocked(annotationService.list).mockResolvedValue([tokyoAnnotation])
    renderTab()

    fireEvent.click(await desktop().findByRole('button', { name: 'Edit' }))

    expect(await screen.findByLabelText('Title')).toHaveValue('Product launch')
    // The suite runs outside Tokyo; the form still shows the 09:00 that was typed.
    expect(screen.getByLabelText('Date')).toHaveValue('2026-08-15')
    expect(screen.getByLabelText('Time')).toHaveValue('09:00')
    // antd keeps the selection in a label span, not in the search input.
    expect(screen.getByLabelText('Timezone').closest('.ant-select')).toHaveTextContent('Asia/Tokyo')

    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Product launch v2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(annotationService.update).toHaveBeenCalled())
    expect(annotationService.update).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      id: 'a1',
      annotated_at: '2026-08-15T00:00:00.000Z',
      timezone: 'Asia/Tokyo',
      title: 'Product launch v2',
      description: 'Version 2 went live',
      color: '#22c55e'
    })
  })

  it('offers the broadcast purple as a preset, selected, when editing a broadcast annotation', async () => {
    // The colour the backend writes every broadcast annotation with has to be in
    // the swatch row: without it the form opened on nothing selected, and picking
    // any other colour left no way back to the one the row started on.
    vi.mocked(annotationService.list).mockResolvedValue([broadcastAnnotation])
    renderTab()

    fireEvent.click(await desktop().findByRole('button', { name: 'Edit' }))

    const purple = await screen.findByRole('button', { name: '#7763f1' })
    expect(purple.className).toContain('outline-offset-2')

    // And it survives a round trip through another colour.
    fireEvent.click(screen.getByRole('button', { name: '#ef4444' }))
    fireEvent.click(purple)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(annotationService.update).toHaveBeenCalled())
    expect(annotationService.update).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'a2', color: '#7763f1' })
    )
  })

  it('marks a broadcast annotation and warns differently before deleting it', async () => {
    vi.mocked(annotationService.list).mockResolvedValue([broadcastAnnotation, tokyoAnnotation])
    renderTab()

    expect(await desktop().findByText('Broadcast')).toBeInTheDocument()

    const deleteButtons = desktop().getAllByRole('button', { name: 'Delete' })
    fireEvent.click(deleteButtons[0])

    expect(
      await screen.findByText(
        'This annotation was created automatically when a broadcast started. Deleting it will not affect the broadcast.'
      )
    ).toBeInTheDocument()

    // The confirm button of the open Popconfirm, not the row triggers.
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' }).slice(-1)[0])
    await waitFor(() => expect(annotationService.delete).toHaveBeenCalledWith('ws1', 'a2'))
  })

  it('warns that a manual deletion is final', async () => {
    vi.mocked(annotationService.list).mockResolvedValue([tokyoAnnotation])
    renderTab()

    fireEvent.click(await desktop().findByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('This action cannot be undone.')).toBeInTheDocument()
  })

  it('surfaces a failed load instead of the empty state', async () => {
    // An install whose database predates the annotations table answers with an
    // error; without this the table would claim the workspace has none.
    vi.mocked(annotationService.list).mockRejectedValue(new Error('relation does not exist'))
    renderTab()

    expect(await screen.findByText('Could not load the annotations')).toBeInTheDocument()
  })

  it('narrows the list to a single source', async () => {
    vi.mocked(annotationService.list).mockResolvedValue([tokyoAnnotation, broadcastAnnotation])
    renderTab()

    expect(await desktop().findByText('Product launch')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('radio', { name: 'Broadcast' }))

    expect(desktop().queryByText('Product launch')).not.toBeInTheDocument()
    expect(desktop().getByText('Summer sale')).toBeInTheDocument()
  })
})
