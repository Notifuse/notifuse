import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import FileSrc from './FileSrc'

// The file manager's button, replaced by one that picks a fixed file
vi.mock('../../file_manager/context', () => ({
  useFileManager: () => ({
    SelectFileButton: ({ onSelect, buttonText }: { onSelect: (url: string) => void; buttonText: string }) => (
      <button type="button" onClick={() => onSelect('https://cdn.test/picked.png')}>
        {buttonText}
      </button>
    )
  })
}))

describe('FileSrc', () => {
  it('writes a typed URL once, on blur, not at every keystroke', () => {
    const onChange = vi.fn()
    render(<FileSrc value="https://cdn.test/a.png" onChange={onChange} aria-label="Image URL" />)
    const input = screen.getByLabelText('Image URL')
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'https://cdn.test/b' } })
    fireEvent.change(input, { target: { value: 'https://cdn.test/b.png' } })
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.blur(input)
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('https://cdn.test/b.png')
  })

  it('writes the URL of a file picked in the file manager, once', () => {
    const onChange = vi.fn()
    render(<FileSrc value="" onChange={onChange} aria-label="Image URL" />)
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('https://cdn.test/picked.png')
  })
})
