import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmCard } from './ConfirmCard'

const renderCard = (props: Partial<Parameters<typeof ConfirmCard>[0]> = {}) => {
  const onConfirm = vi.fn()
  render(
    <ConfirmCard
      listName="Newsletter"
      prompt="Do you want to receive emails from this list?"
      buttonLabel="Confirm subscription"
      processingLabel="Processing..."
      processing={false}
      tone="primary"
      testId="confirm-subscription"
      onConfirm={onConfirm}
      {...props}
    />
  )
  return onConfirm
}

describe('ConfirmCard', () => {
  it('names the list, asks, and names the button after the list', () => {
    renderCard()

    const card = screen.getByTestId('confirm-subscription')
    expect(card).toHaveTextContent('Newsletter')
    expect(card).toHaveTextContent('Do you want to receive emails from this list?')
    expect(screen.getByRole('button', { name: 'Confirm subscription: Newsletter' })).toBeEnabled()
  })

  it('goes unnamed when the list is not known', () => {
    renderCard({ listName: '' })

    expect(screen.getByRole('button', { name: 'Confirm subscription' })).toBeInTheDocument()
    expect(screen.getByTestId('confirm-subscription')).not.toHaveTextContent('Newsletter')
  })

  it('calls onConfirm once per click', async () => {
    const user = userEvent.setup()
    const onConfirm = renderCard()

    await user.click(screen.getByRole('button'))

    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('is disabled and says so while the request runs', async () => {
    const user = userEvent.setup()
    const onConfirm = renderCard({ processing: true })

    const button = screen.getByRole('button')
    expect(button).toBeDisabled()
    expect(button).toHaveTextContent('Processing...')
    await user.click(button)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('marks leaving a list in red and joining one in blue', () => {
    const { unmount } = render(
      <ConfirmCard
        listName="Newsletter"
        prompt="Leave?"
        buttonLabel="Unsubscribe"
        processingLabel="Processing..."
        processing={false}
        tone="danger"
        testId="confirm-unsubscribe"
        onConfirm={() => {}}
      />
    )
    expect(screen.getByRole('button').className).toContain('text-red-500')
    unmount()

    renderCard()
    expect(screen.getByRole('button').className).toContain('text-blue-500')
  })
})
