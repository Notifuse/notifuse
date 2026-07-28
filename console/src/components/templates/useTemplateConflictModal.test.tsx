import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useTemplateConflictModal, type ConflictInfo } from './useTemplateConflictModal'

function Harness({ info }: { info: ConflictInfo }) {
  const { conflictModal, showConflict } = useTemplateConflictModal()
  return (
    <div>
      <button onClick={() => showConflict(info)}>open-conflict</button>
      {conflictModal}
    </div>
  )
}

describe('useTemplateConflictModal', () => {
  let onOverwrite: ReturnType<typeof vi.fn>
  let onReload: ReturnType<typeof vi.fn>

  beforeEach(() => {
    onOverwrite = vi.fn()
    onReload = vi.fn()
  })

  const open = async () => {
    render(<Harness info={{ latestVersion: 9, baseVersion: 4, onOverwrite, onReload }} />)
    await userEvent.click(screen.getByRole('button', { name: 'open-conflict' }))
    await waitFor(() => expect(screen.getByText(/Template changed by someone else/i)).toBeInTheDocument())
  }

  it('shows the three explicit actions and reports the latest version', async () => {
    await open()
    expect(screen.getByRole('button', { name: 'Overwrite with mine' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reload latest' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Keep editing' })).toBeInTheDocument()
    // The body should mention the server's version so the user understands the conflict.
    expect(screen.getByText(/9/)).toBeInTheDocument()
  })

  it('"Overwrite with mine" invokes only onOverwrite', async () => {
    await open()
    await userEvent.click(screen.getByRole('button', { name: 'Overwrite with mine' }))
    expect(onOverwrite).toHaveBeenCalledTimes(1)
    expect(onReload).not.toHaveBeenCalled()
  })

  it('"Reload latest" invokes only onReload', async () => {
    await open()
    await userEvent.click(screen.getByRole('button', { name: 'Reload latest' }))
    expect(onReload).toHaveBeenCalledTimes(1)
    expect(onOverwrite).not.toHaveBeenCalled()
  })

  it('"Keep editing" dismisses without invoking any destructive action', async () => {
    await open()
    await userEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    expect(onOverwrite).not.toHaveBeenCalled()
    expect(onReload).not.toHaveBeenCalled()
  })

  it('pressing ESC is a safe no-op — it never triggers the destructive reload/overwrite', async () => {
    await open()
    // The conflict dialog maps ESC (and the close icon) to a plain dismiss, so a reflexive
    // ESC can never discard the user's work via the "Reload latest" path or force an overwrite.
    await userEvent.keyboard('{Escape}')
    expect(onReload).not.toHaveBeenCalled()
    expect(onOverwrite).not.toHaveBeenCalled()
  })
})
