import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react'
import { TextVersionPanel } from './TextVersionPanel'
import type { CustomTextVersion } from './TextVersionPanel'

describe('TextVersionPanel', () => {
  it('shows the text verbatim, keeping line breaks and indentation', () => {
    const text = 'Hello Camille\n\n- One\n  - Sub\n\nShop (https://example.com/shop)\n'
    render(<TextVersionPanel text={text} />)

    expect(screen.getByTestId('text-version').textContent).toBe(text)
  })

  it('says the email goes out as HTML only when there is no text', () => {
    render(<TextVersionPanel text="" />)

    expect(screen.getByText('No text version: this email will be sent as HTML only.')).toBeTruthy()
    expect(screen.queryByTestId('text-version')).toBeNull()
  })

  it('shows the compile error instead of a text', () => {
    render(<TextVersionPanel text="stale" errors={[{ message: 'mj-text: unclosed tag' }]} />)

    expect(screen.getByText('mj-text: unclosed tag')).toBeTruthy()
    expect(screen.queryByTestId('text-version')).toBeNull()
  })
})

describe('TextVersionPanel with a custom text', () => {
  const setup = (custom: Partial<CustomTextVersion> = {}, props: Partial<React.ComponentProps<typeof TextVersionPanel>> = {}) => {
    const onChange = vi.fn()
    const onSeed = vi.fn().mockResolvedValue({ text: 'Seeded {{ unsubscribe_url }}' })
    const utils = render(
      <TextVersionPanel
        text="Automatic text"
        custom={{ onChange, onSeed, category: 'transactional', ...custom }}
        {...props}
      />
    )
    return { onChange, onSeed, ...utils }
  }

  it('offers Automatic and Custom, Automatic selected when there is no text', () => {
    setup()
    expect((screen.getByRole('radio', { name: /Automatic/ }) as HTMLInputElement).checked).toBe(true)
    expect((screen.getByRole('radio', { name: /Custom/ }) as HTMLInputElement).checked).toBe(false)
    expect(screen.getByTestId('text-version').textContent).toBe('Automatic text')
    expect(screen.queryByTestId('custom-text-input')).toBeNull()
  })

  it('choosing Custom starts from the automatic version', async () => {
    const { onChange, onSeed } = setup()
    fireEvent.click(screen.getByRole('radio', { name: /Custom/ }))
    expect(onChange).toHaveBeenCalledWith('')
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith('Seeded {{ unsubscribe_url }}'))
    expect(onSeed).toHaveBeenCalledTimes(1)
  })

  it('shows the text as written and reports each change', () => {
    const { onChange } = setup({ value: 'Hi {{ contact.first_name }}' })
    const area = screen.getByTestId('custom-text-input') as HTMLTextAreaElement
    expect(area.value).toBe('Hi {{ contact.first_name }}')
    fireEvent.change(area, { target: { value: 'Hello' } })
    expect(onChange).toHaveBeenCalledWith('Hello')
  })

  it('keeps the text area when the email does not compile', () => {
    setup({ value: 'Hi' }, { errors: [{ message: 'mj-text: unclosed tag' }] })
    expect(screen.getByTestId('custom-text-input')).toBeTruthy()
  })

  it('asks before going back to Automatic when that loses text', async () => {
    const { onChange } = setup({ value: 'Mine' })
    fireEvent.click(screen.getByRole('radio', { name: /Automatic/ }))
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'Use the automatic version' }))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('goes back to Automatic at once when the text is empty', () => {
    const { onChange } = setup({ value: '' })
    fireEvent.click(screen.getByRole('radio', { name: /Automatic/ }))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('starts again from the automatic version after a confirmation', async () => {
    const { onChange, onSeed } = setup({ value: 'Mine' })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Replace' }))
    await waitFor(() => expect(onChange).toHaveBeenCalledWith('Seeded {{ unsubscribe_url }}'))
    expect(onSeed).toHaveBeenCalledTimes(1)
  })

  it('drops a starting point that arrives after going back to Automatic', async () => {
    let resolve: (v: { text: string }) => void = () => {}
    const onSeed = vi.fn().mockReturnValue(new Promise((r) => (resolve = r)))
    const { onChange } = setup({ value: '', onSeed })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    fireEvent.click(screen.getByRole('radio', { name: /Automatic/ }))
    expect(onChange).toHaveBeenLastCalledWith(undefined)

    await act(async () => resolve({ text: 'Late seed' }))
    expect(onChange).not.toHaveBeenCalledWith('Late seed')
  })

  it('drops a starting point that arrives after the panel is gone', async () => {
    let resolve: (v: { text: string }) => void = () => {}
    const onSeed = vi.fn().mockReturnValue(new Promise((r) => (resolve = r)))
    const { onChange, unmount } = setup({ value: '', onSeed })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    unmount()

    await act(async () => resolve({ text: 'Late seed' }))
    expect(onChange).not.toHaveBeenCalledWith('Late seed')
  })

  it('locks the text area while the automatic version is generated', async () => {
    let resolve: (v: { text: string }) => void = () => {}
    const onSeed = vi.fn().mockReturnValue(new Promise((r) => (resolve = r)))
    setup({ value: '', onSeed })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    await waitFor(() => expect((screen.getByTestId('custom-text-input') as HTMLTextAreaElement).disabled).toBe(true))
    resolve({ text: 'Done' })
    await waitFor(() => expect((screen.getByTestId('custom-text-input') as HTMLTextAreaElement).disabled).toBe(false))
  })

  it('keeps the text and shows why when the automatic version cannot be generated', async () => {
    const onSeed = vi.fn().mockResolvedValue({ error: 'XML syntax error' })
    const { onChange } = setup({ value: '', onSeed })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    expect(await screen.findByText(/XML syntax error/)).toBeTruthy()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('warns when conditions or loops between blocks were not carried over', async () => {
    const onSeed = vi.fn().mockResolvedValue({ text: '- {{ item.name }}', lostLiquid: true })
    setup({ value: '', onSeed })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    expect(await screen.findByText(/Conditions and loops placed between blocks are not carried over/)).toBeTruthy()
  })

  it('shows a Liquid problem found in the starting point', async () => {
    const onSeed = vi.fn().mockResolvedValue({ text: '{% if vip %}VIP', textError: "'if' tag was never closed" })
    setup({ value: '', onSeed })
    fireEvent.click(screen.getByRole('button', { name: 'Start again from the automatic version' }))
    expect(await screen.findByText(/'if' tag was never closed/)).toBeTruthy()
  })

  it('reminds a marketing template to keep its unsubscribe link', () => {
    setup({ value: 'Hi', category: 'marketing' })
    expect(screen.getByText(/Keep the unsubscribe link/)).toBeTruthy()
    expect(screen.getByText(/Links in a custom text are not tracked/)).toBeTruthy()
  })

  it('reminds an opt-in template to keep its confirmation link', () => {
    setup({ value: 'Hi', category: 'opt_in' })
    expect(screen.getByText(/Keep the confirmation link/)).toBeTruthy()
    expect(screen.queryByText(/Keep the unsubscribe link/)).toBeNull()
  })

  it('gives a transactional template no unsubscribe advice', () => {
    setup({ value: 'Hi', category: 'transactional' })
    expect(screen.queryByText(/Keep the unsubscribe link/)).toBeNull()
    expect(screen.queryByText(/Keep the confirmation link/)).toBeNull()
    expect(screen.getByText(/Links in a custom text are not tracked/)).toBeTruthy()
  })

  it('tells the default language where translations get their text', () => {
    setup({ value: 'Hi' })
    expect(screen.getByText(/Each translation has its own text in the Translations tab/)).toBeTruthy()
  })

  it('tells a translation that without its own text it gets the automatic version', () => {
    setup({ isTranslation: true })
    expect(screen.getByText(/Without its own text, this language receives the automatic version/)).toBeTruthy()
  })

  it('can be read-only', () => {
    setup({ value: 'Hi', disabled: true })
    expect((screen.getByTestId('custom-text-input') as HTMLTextAreaElement).disabled).toBe(true)
    expect((screen.getByRole('radio', { name: /Automatic/ }) as HTMLInputElement).disabled).toBe(true)
  })
})
