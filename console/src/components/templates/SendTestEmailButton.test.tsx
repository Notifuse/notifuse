import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import SendTestEmailButton from './SendTestEmailButton'
import type { StoredPermissions } from '../../services/api/permissions'

const sender: StoredPermissions = {
  templates: { read: true, write: false },
  transactional: { read: false, write: true }
}

// What the Templates and Broadcasts pages asked for before: it does not let the endpoint send.
const oldCheck: StoredPermissions = {
  templates: { read: true, write: false },
  contacts: { read: true, write: true }
}

describe('SendTestEmailButton', () => {
  it('is enabled with templates read and transactional write, without any contacts permission', () => {
    const onClick = vi.fn()
    render(<SendTestEmailButton permissions={sender} onClick={onClick} />)

    const button = screen.getByRole('button')
    expect(button).toBeEnabled()
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('is disabled without transactional write and names the permissions it needs', async () => {
    const onClick = vi.fn()
    render(<SendTestEmailButton permissions={oldCheck} onClick={onClick} />)

    const button = screen.getByRole('button')
    expect(button).toBeDisabled()
    // user-event refuses to hover a disabled element, so fire the events the tooltip listens to.
    fireEvent.pointerEnter(button)
    fireEvent.mouseEnter(button)
    expect(
      await screen.findByText(
        'You need Templates read and Transactional write permissions to send test emails'
      )
    ).toBeInTheDocument()
  })

  it('is disabled without permissions', () => {
    render(<SendTestEmailButton permissions={null} onClick={() => {}} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })
})
