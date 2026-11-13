import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { LogsSearchBox } from './LogsSearchBox'

i18n.loadAndActivate({ locale: 'en', messages: {} })

type Key = 'contact_email' | 'id' | 'external_id'
const KEYS = [
  { key: 'contact_email', label: 'Email', email: true },
  { key: 'id', label: 'Message ID' },
  { key: 'external_id', label: 'External ID' }
] as const

const cleared = { contact_email: undefined, id: undefined, external_id: undefined }

const renderBox = (values: Partial<Record<Key, string>> = {}) => {
  const onWrite = vi.fn()
  const ui = (current: Partial<Record<Key, string>>) => (
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <LogsSearchBox<Key>
          keys={KEYS}
          values={current}
          onWrite={onWrite}
          ariaLabel="Search the messages"
          placeholder="Search"
        />
      </ConfigProvider>
    </I18nProvider>
  )
  const view = render(ui(values))
  // The URL changing, by the box's own write or from outside (Clear filters, Back).
  const setUrl = (next: Partial<Record<Key, string>>) => view.rerender(ui(next))
  return { onWrite, setUrl, ...view }
}

const pick = (label: string) => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Search by' }))
  fireEvent.click(screen.getByRole('option', { name: label }))
}

const box = () => screen.getByRole('textbox', { name: 'Search the messages' })
const type = (value: string) => fireEvent.change(box(), { target: { value } })
const chosenKey = () => document.querySelector('.ant-select-content')?.textContent

describe('LogsSearchBox', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('writes a full email lower-cased after the pause, clearing the other keys', () => {
    const { onWrite } = renderBox()
    type('Camille@Example.com')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenCalledWith({ ...cleared, contact_email: 'camille@example.com' })
  })

  it('neither writes nor clears an incomplete email, and says why', () => {
    const { onWrite } = renderBox({ contact_email: 'ann@example.com' })
    type('camille@')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).not.toHaveBeenCalled()
    expect(screen.getByText('Enter a full email address')).toBeInTheDocument()
  })

  it('switches to Email when an "@" is typed', () => {
    const { onWrite } = renderBox({ id: 'msg_1' })
    expect(chosenKey()).toBe('Message ID')
    type('ann@example.com')
    expect(chosenKey()).toBe('Email')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenCalledWith({ ...cleared, contact_email: 'ann@example.com' })
  })

  it('writes at once on Enter', () => {
    const { onWrite } = renderBox({ external_id: 'x' })
    type('order-42')
    fireEvent.keyDown(box(), { key: 'Enter', code: 'Enter', keyCode: 13 })
    expect(onWrite).toHaveBeenCalledWith({ ...cleared, external_id: 'order-42' })
  })

  it('keeps the case of an identifier', () => {
    const { onWrite } = renderBox({ id: 'x' })
    type('Msg_ABC')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenCalledWith({ ...cleared, id: 'Msg_ABC' })
  })

  it('clears every key when the text is emptied', () => {
    const { onWrite } = renderBox({ id: 'msg_1' })
    type('')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenCalledWith(cleared)
  })

  it('shows the key and value of the URL', () => {
    renderBox({ external_id: 'order-42' })
    expect(box()).toHaveValue('order-42')
    expect(chosenKey()).toBe('External ID')
  })

  it('moves the text typed to the parameter chosen, at once', () => {
    const { onWrite } = renderBox({ id: 'order-42' })
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Search by' }))
    fireEvent.click(screen.getByRole('option', { name: 'External ID' }))
    expect(chosenKey()).toBe('External ID')
    expect(onWrite).toHaveBeenCalledWith({ ...cleared, external_id: 'order-42' })
  })

  it('keeps a key the member picked when an ID holds an "@", until the box is emptied', () => {
    const { onWrite } = renderBox()
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Search by' }))
    fireEvent.click(screen.getByRole('option', { name: 'Message ID' }))
    type('CAFabc+1@mail.gmail.com')
    expect(chosenKey()).toBe('Message ID')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenLastCalledWith({ ...cleared, id: 'CAFabc+1@mail.gmail.com' })
    type('')
    type('ann@example.com')
    expect(chosenKey()).toBe('Email')
  })

  it('keeps the key of a link whose value already holds an "@"', () => {
    const { onWrite } = renderBox({ external_id: 'order@shop' })
    type('order@shop2')
    expect(chosenKey()).toBe('External ID')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenLastCalledWith({ ...cleared, external_id: 'order@shop2' })
  })

  it('keeps a picked key across its own write to the URL', () => {
    const { onWrite, setUrl } = renderBox()
    pick('External ID')
    type('order')
    act(() => vi.advanceTimersByTime(300))
    setUrl({ external_id: 'order' })
    type('order@shop')
    expect(chosenKey()).toBe('External ID')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenLastCalledWith({ ...cleared, external_id: 'order@shop' })
  })

  it('forgets a picked key when the URL empties the box', () => {
    const { onWrite, setUrl } = renderBox()
    pick('Message ID')
    type('msg_1')
    act(() => vi.advanceTimersByTime(300))
    setUrl({ id: 'msg_1' })
    setUrl({})
    expect(box()).toHaveValue('')
    type('ann@example.com')
    expect(chosenKey()).toBe('Email')
    act(() => vi.advanceTimersByTime(300))
    expect(onWrite).toHaveBeenLastCalledWith({ ...cleared, contact_email: 'ann@example.com' })
  })

  it('forgets a picked key when the URL brings another one', () => {
    const { setUrl } = renderBox()
    pick('External ID')
    type('order-1')
    act(() => vi.advanceTimersByTime(300))
    setUrl({ external_id: 'order-1' })
    setUrl({ id: 'msg_9' })
    expect(chosenKey()).toBe('Message ID')
    type('ann@example.com')
    expect(chosenKey()).toBe('Email')
  })

  it('keeps a picked key while the box holds only spaces', () => {
    renderBox()
    pick('Message ID')
    type(' ')
    type(' CAFabc@mail.gmail.com')
    expect(chosenKey()).toBe('Message ID')
  })
})
