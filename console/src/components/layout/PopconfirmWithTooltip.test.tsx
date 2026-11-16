import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Button } from 'antd'
import { PopconfirmWithTooltip } from './PopconfirmWithTooltip'

const visibleTooltips = () =>
  [...document.querySelectorAll('.ant-tooltip:not(.ant-tooltip-hidden)')].map((el) => el.textContent)

describe('PopconfirmWithTooltip', () => {
  it('names the button on hover, hides the name while confirming, and confirms', async () => {
    const onConfirm = vi.fn()
    render(
      <PopconfirmWithTooltip tooltip="Delete" title="Delete this file?" okText="Yes" onConfirm={onConfirm}>
        <Button aria-label="Delete">x</Button>
      </PopconfirmWithTooltip>
    )
    const button = screen.getByRole('button', { name: 'Delete' })
    fireEvent.mouseEnter(button)
    await waitFor(() => expect(visibleTooltips()).toContain('Delete'))

    fireEvent.click(button)
    expect(await screen.findByText('Delete this file?')).toBeInTheDocument()
    await waitFor(() => expect(visibleTooltips()).not.toContain('Delete'))

    fireEvent.click(screen.getByRole('button', { name: 'Yes' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })
})
