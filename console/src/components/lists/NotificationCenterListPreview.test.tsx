import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { NotificationCenterListPreview } from './NotificationCenterListPreview'

const preview = () => screen.getByRole('region', { name: 'In the notification center' })

describe('NotificationCenterListPreview', () => {
  it('shows the name and the description', () => {
    render(<NotificationCenterListPreview name="Product news" description="Once a month." />)
    expect(within(preview()).getByText('Product news')).toBeInTheDocument()
    expect(within(preview()).getByText('Once a month.')).toBeInTheDocument()
  })

  it('shows no description line without a description', () => {
    render(<NotificationCenterListPreview name="Product news" description="  " />)
    expect(preview().querySelector('p.mt-1')).toBeNull()
  })

  it('shows a placeholder while the name is empty', () => {
    render(<NotificationCenterListPreview name="" />)
    expect(within(preview()).getByText('List name')).toBeInTheDocument()
  })

  it('draws the subscribe button without making it a control', () => {
    render(<NotificationCenterListPreview name="Product news" />)
    expect(within(preview()).queryByRole('button')).toBeNull()
    const drawn = preview().querySelector('[data-preview-subscribe]')
    expect(drawn).toHaveTextContent('Subscribe')
    expect(drawn).toHaveAttribute('aria-hidden', 'true')
  })
})
