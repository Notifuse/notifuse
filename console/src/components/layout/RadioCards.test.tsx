import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { RadioCards, type RadioCardOption } from './RadioCards'

type Op = 'unsubscribe' | 'delete'

const options: RadioCardOption<Op>[] = [
  { value: 'unsubscribe', title: 'Unsubscribe from a list', description: 'Reversible.' },
  { value: 'delete', title: 'Delete contacts', description: 'Cannot be undone.', danger: true }
]

function Harness({ initial, onChange }: { initial?: Op; onChange?: (v: Op) => void }) {
  const [value, setValue] = useState<Op | undefined>(initial)
  return (
    <RadioCards
      name="op"
      ariaLabel="Action"
      options={options}
      value={value}
      onChange={(v) => {
        setValue(v)
        onChange?.(v)
      }}
    />
  )
}

const card = (name: string) => screen.getByRole('radio', { name: new RegExp(name) }).closest('label')!

describe('RadioCards', () => {
  it('checks the card the value names, and only that one', () => {
    render(<Harness initial="unsubscribe" />)
    expect(screen.getByRole('radio', { name: /Unsubscribe from a list/ })).toBeChecked()
    expect(screen.getByRole('radio', { name: /Delete contacts/ })).not.toBeChecked()
    expect(card('Unsubscribe')).toHaveAttribute('data-checked')
    expect(card('Delete')).not.toHaveAttribute('data-checked')
  })

  it('turns a danger card red only once it is checked', async () => {
    render(<Harness initial="unsubscribe" />)
    expect(card('Delete').className).not.toContain('border-danger-fg')
    await userEvent.click(screen.getByRole('radio', { name: /Delete contacts/ }))
    expect(card('Delete').className).toContain('border-danger-fg')
  })

  it('moves the choice with the arrow keys', async () => {
    const onChange = vi.fn()
    render(<Harness initial="unsubscribe" onChange={onChange} />)
    screen.getByRole('radio', { name: /Unsubscribe from a list/ }).focus()
    await userEvent.keyboard('{ArrowDown}')
    expect(onChange).toHaveBeenLastCalledWith('delete')
    expect(screen.getByRole('radio', { name: /Delete contacts/ })).toBeChecked()
  })

  it('leaves a disabled card unchecked when clicked, and none checked without a value', async () => {
    render(
      <RadioCards
        name="op2"
        ariaLabel="Action"
        options={[{ ...options[0], disabled: true, disabledReason: 'No permission' }, options[1]]}
        value={undefined}
        onChange={vi.fn()}
      />
    )
    const unsubscribe = screen.getByRole('radio', { name: /Unsubscribe from a list/ })
    expect(unsubscribe).toBeDisabled()
    await userEvent.click(unsubscribe)
    expect(screen.getAllByRole('radio').some((radio) => (radio as HTMLInputElement).checked)).toBe(false)
  })
})
