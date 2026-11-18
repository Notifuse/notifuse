import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { BackgroundFields, type BackgroundValues } from './BackgroundFields'

// FileSrc needs the file manager; its own tests cover it. Here it is a plain URL input.
vi.mock('../FileSrc', () => ({
  default: ({ value, onChange }: { value?: string; onChange: (value: string | undefined) => void }) => (
    <input aria-label="Image URL" value={value} onChange={(e) => onChange(e.target.value || undefined)} />
  )
}))

const renderFields = (value: BackgroundValues = {}, inherited: BackgroundValues = {}) => {
  const onChange = vi.fn()
  const view = render(<BackgroundFields value={value} inherited={inherited} onChange={onChange} />)
  return { onChange, view }
}

const typeControl = () => screen.getByLabelText('Background type', { selector: '.ant-segmented' })
const pickType = (name: 'Color' | 'Image') => fireEvent.click(within(typeControl()).getByText(name))

describe('BackgroundFields', () => {
  it('keeps the colour when switching to Image, shown as the fallback colour', () => {
    const { onChange } = renderFields({ backgroundColor: '#6a5af9' })
    expect(screen.queryByLabelText('Image URL')).toBeNull()
    pickType('Image')
    // Nothing is cleared: the colour becomes the image's fallback.
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Image URL')).toBeInTheDocument()
    expect(screen.getByText('Fallback color')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Fallback color' })).toHaveTextContent('#6A5AF9')
    expect(screen.getByRole('button', { name: 'About Fallback color' })).toBeInTheDocument()
  })

  it('clears every image key when switching to Colour, and keeps the colour', () => {
    const { onChange } = renderFields({
      backgroundColor: '#6a5af9',
      backgroundUrl: 'https://cdn.example.com/hero.jpg',
      backgroundSize: 'cover',
      backgroundRepeat: 'repeat',
      backgroundPosition: 'top left',
      backgroundPositionX: '10%',
      backgroundPositionY: '20%'
    })
    pickType('Color')
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith({
      backgroundUrl: undefined,
      backgroundSize: undefined,
      backgroundRepeat: undefined,
      backgroundPosition: undefined,
      backgroundPositionX: undefined,
      backgroundPositionY: undefined
    })
  })

  it('bounds a custom size to 1-2000 px and 1-500 %', () => {
    const { view } = renderFields({ backgroundUrl: 'https://cdn.example.com/a.jpg', backgroundSize: '300px' })
    let size = screen.getByRole('spinbutton', { name: 'Custom background size' })
    expect(size).toHaveAttribute('aria-valuemin', '1')
    expect(size).toHaveAttribute('aria-valuemax', '2000')
    expect(size).toHaveValue('300')

    view.rerender(
      <BackgroundFields value={{ backgroundUrl: 'https://cdn.example.com/a.jpg', backgroundSize: '80%' }} onChange={vi.fn()} />
    )
    size = screen.getByRole('spinbutton', { name: 'Custom background size' })
    expect(size).toHaveAttribute('aria-valuemax', '500')
    expect(size).toHaveValue('80')
  })

  it('starts a custom size at 100px or 100%, as BackgroundInput did', async () => {
    const { onChange } = renderFields({ backgroundUrl: 'https://cdn.example.com/a.jpg', backgroundSize: 'cover' })
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Background size' }))
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
    const option = [...document.querySelectorAll('.ant-select-item-option')].find((el) => el.textContent === 'Custom (%)')
    fireEvent.click(option!)
    expect(onChange).toHaveBeenLastCalledWith({ backgroundSize: '100%' })
  })

  it('writes a typed custom size with a coalesce key', () => {
    const { onChange } = renderFields({ backgroundUrl: 'https://cdn.example.com/a.jpg', backgroundSize: '300px' })
    const size = screen.getByRole('spinbutton', { name: 'Custom background size' })
    fireEvent.focus(size)
    fireEvent.change(size, { target: { value: '640' } })
    expect(onChange).toHaveBeenLastCalledWith({ backgroundSize: '640px' }, { coalesceKey: expect.any(String) })
  })

  it('writes the position a grid cell names', () => {
    const { onChange } = renderFields({ backgroundUrl: 'https://cdn.example.com/a.jpg' })
    const grid = screen.getByRole('group', { name: 'Background position' })
    // Unset: MJML's top center shows selected.
    expect(within(grid).getByRole('button', { name: 'Top center' })).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(within(grid).getByRole('button', { name: 'Bottom right' }))
    expect(onChange).toHaveBeenLastCalledWith({ backgroundPosition: 'bottom right' })
    expect(screen.queryByRole('spinbutton', { name: 'Position X' })).toBeNull()
  })

  it('writes X and Y as one position in percent, as BackgroundInput did', () => {
    const { onChange, view } = renderFields({ backgroundUrl: 'https://cdn.example.com/a.jpg', backgroundPosition: 'top left' })
    fireEvent.click(screen.getByRole('button', { name: 'Custom' }))
    expect(onChange).toHaveBeenLastCalledWith({ backgroundPosition: '50% 50%' })

    view.rerender(
      <BackgroundFields
        value={{ backgroundUrl: 'https://cdn.example.com/a.jpg', backgroundPosition: '30% 70%' }}
        onChange={onChange}
      />
    )
    expect(screen.getByRole('spinbutton', { name: 'Position X' })).toHaveValue('30')
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Position X' }), { target: { value: '10' } })
    expect(onChange).toHaveBeenLastCalledWith({ backgroundPosition: '10% 70%' }, expect.anything())
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Position Y' }), { target: { value: '90' } })
    expect(onChange).toHaveBeenLastCalledWith({ backgroundPosition: '30% 90%' }, expect.anything())
  })

  it('keeps the repeat list of BackgroundInput', async () => {
    const { onChange } = renderFields({ backgroundUrl: 'https://cdn.example.com/a.jpg' })
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Background repeat' }))
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
    const options = [...document.querySelectorAll('.ant-select-item-option')]
    expect(options.map((el) => el.textContent)).toEqual(['Repeat', 'Repeat X', 'Repeat Y', 'No repeat', 'Space', 'Round'])
    fireEvent.click(options[1])
    expect(onChange).toHaveBeenLastCalledWith({ backgroundRepeat: 'repeat-x' })
  })

  it('shows only the colour when images are not offered', () => {
    render(<BackgroundFields value={{}} onChange={vi.fn()} showBackgroundImage={false} />)
    expect(screen.queryByLabelText('Background type', { selector: '.ant-segmented' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Background color' })).toBeInTheDocument()
  })
})
