import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { useContext } from 'react'
import { ConfigProvider } from 'antd'
import { FieldsAboveSwitchRow, SwitchRow } from './SwitchRow'

describe('SwitchRow', () => {
  it('names the switch after its title, and describes it by its hint without help', () => {
    const onChange = vi.fn()
    render(<SwitchRow title="Track opens" hint="Short hint" checked={false} onChange={onChange} />)
    const toggle = screen.getByRole('switch', { name: 'Track opens', description: 'Short hint' })
    fireEvent.click(toggle)
    expect(onChange).toHaveBeenCalledWith(true)
  })

  it('describes it by the long explanation once there is one, behind a "?"', () => {
    render(
      <SwitchRow
        title="Exclude"
        hint="Short hint"
        help={{ label: 'About excluding', content: 'The long explanation' }}
        checked
        onChange={vi.fn()}
      />
    )
    expect(screen.getByRole('switch', { name: 'Exclude', description: 'The long explanation' })).toBeChecked()
    expect(screen.getByRole('button', { name: 'About excluding' })).toBeInTheDocument()
    expect(screen.getByText('Short hint')).toBeVisible()
  })

  it('gives the fields above it a 12 px bottom margin through the antd theme', () => {
    const Probe = () => {
      const { theme } = useContext(ConfigProvider.ConfigContext)
      return <span data-testid="margin">{String(theme?.components?.Form?.itemMarginBottom)}</span>
    }
    render(
      <FieldsAboveSwitchRow>
        <Probe />
      </FieldsAboveSwitchRow>
    )
    expect(screen.getByTestId('margin')).toHaveTextContent('12')
  })
})
