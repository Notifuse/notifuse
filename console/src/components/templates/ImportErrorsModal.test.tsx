import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { ImportErrorsModal } from './ImportErrorsModal'
import { excerptLines } from './importExcerpt'

const source = [
  '<mjml>',
  '  <mj-body>',
  '    <mj-wrapper>',
  '      <mj-column>',
  '        <mj-text>Hi',
  '      </mj-column>'
].join('\n')

const renderModal = (props: Partial<Parameters<typeof ImportErrorsModal>[0]> = {}) =>
  render(
    <ImportErrorsModal
      open
      title="MJML Syntax Error"
      errors={['error on line 6 at column 19: Opening and ending tag mismatch', 'a second error']}
      onClose={vi.fn()}
      {...props}
    />
  )

describe('ImportErrorsModal', () => {
  it('lists each error on its own row', () => {
    renderModal()
    const rows = screen.getAllByRole('listitem')
    expect(rows.map((row) => row.textContent)).toEqual([
      'error on line 6 at column 19: Opening and ending tag mismatch',
      'a second error'
    ])
  })

  it('shows the faulty line of the file and the two before it, the faulty one marked', () => {
    renderModal({ source, line: 6 })
    const excerpt = screen.getByTestId('import-excerpt')
    expect(excerpt.textContent).toBe('4      <mj-column>5        <mj-text>Hi6      </mj-column>')
    expect(excerpt.querySelector('[data-faulty]')?.textContent).toBe('6      </mj-column>')
  })

  it('starts the excerpt at the first line when the error is on it', () => {
    expect(excerptLines(source, 1)).toEqual([{ number: 1, text: '<mjml>' }])
  })

  it('shows no excerpt without a line, without the file, or for a line past its end', () => {
    const { rerender } = renderModal({ source })
    expect(screen.queryByTestId('import-excerpt')).toBeNull()
    rerender(<ImportErrorsModal open title="t" errors={['e']} onClose={vi.fn()} line={6} />)
    expect(screen.queryByTestId('import-excerpt')).toBeNull()
  })

  it('offers a retry only when it is given one, and closes', () => {
    const onClose = vi.fn()
    const onRetry = vi.fn()
    const { rerender } = renderModal({ onClose })
    expect(screen.queryByRole('button', { name: 'Retry with a corrected file' })).toBeNull()
    // The footer's Close, not the corner cross
    fireEvent.click(screen.getByText('Close', { selector: '.ant-modal-footer button span' }))
    expect(onClose).toHaveBeenCalledTimes(1)

    rerender(<ImportErrorsModal open title="t" errors={['e']} onClose={onClose} onRetry={onRetry} />)
    const dialog = screen.getByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Retry with a corrected file' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
