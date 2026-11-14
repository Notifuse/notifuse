import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import { useRef, type ReactNode } from 'react'
import { useDrawerNavKeys, type DrawerNavKeysOptions } from './useDrawerNavKeys'

/** A stand-in for an antd drawer: the hook finds its overlay with `closest('.ant-drawer')`. */
function Harness(props: Omit<DrawerNavKeysOptions, 'anchorRef'> & { children?: ReactNode }) {
  const anchorRef = useRef<HTMLDivElement>(null)
  useDrawerNavKeys({ ...props, anchorRef })
  return (
    <div className="ant-drawer ant-drawer-open">
      <div ref={anchorRef}>
        <input aria-label="field" />
        <textarea aria-label="area" />
        <div contentEditable suppressContentEditableWarning data-testid="editable">
          <span data-testid="editable-child">text</span>
        </div>
        <button type="button">plain</button>
        {props.children}
      </div>
    </div>
  )
}

function setup(overrides: Partial<Omit<DrawerNavKeysOptions, 'anchorRef'>> & { children?: ReactNode } = {}) {
  const onNext = vi.fn()
  const onPrev = vi.fn()
  const utils = render(<Harness enabled onNext={onNext} onPrev={onPrev} {...overrides} />)
  return { onNext, onPrev, ...utils }
}

describe('useDrawerNavKeys', () => {
  it('calls onNext on J and onPrev on K', () => {
    const { onNext, onPrev } = setup()
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document.body, { key: 'k' })
    expect(onPrev).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['popconfirm', 'ant-popover ant-popconfirm', 'ant-popover ant-popconfirm ant-popover-hidden'],
    ['dropdown menu', 'ant-dropdown', 'ant-dropdown ant-dropdown-hidden']
  ])('leaves the keys to an open %s, not to a hidden one', (_, shown, hidden) => {
    const { onNext } = setup()
    const open = document.createElement('div')
    open.className = shown
    document.body.appendChild(open)
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
    open.className = hidden
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    open.remove()
  })

  it('keeps the keys while a popover only shows on hover', () => {
    const { onNext } = setup()
    const popover = document.createElement('div')
    popover.className = 'ant-popover'
    document.body.appendChild(popover)
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    popover.remove()
  })

  it('leaves the keys to a popover holding the focus', () => {
    const { onNext } = setup()
    const popover = document.createElement('div')
    popover.className = 'ant-popover'
    const button = document.createElement('button')
    popover.appendChild(button)
    document.body.appendChild(popover)
    button.focus()
    fireEvent.keyDown(button, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
    popover.remove()
  })

  it('leaves the keys to a popover whose trigger holds the focus, as after a click', () => {
    const { onNext, getByRole } = setup()
    const popover = document.createElement('div')
    popover.className = 'ant-popover'
    document.body.appendChild(popover)
    const trigger = getByRole('button', { name: 'plain' })
    trigger.classList.add('ant-popover-open')
    trigger.focus()
    fireEvent.keyDown(trigger, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
    trigger.classList.remove('ant-popover-open')
    fireEvent.keyDown(trigger, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    popover.remove()
  })

  it('does nothing while disabled', () => {
    const { onNext } = setup({ enabled: false })
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('does nothing without the matching handler', () => {
    const { onPrev } = setup({ onNext: undefined })
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onPrev).not.toHaveBeenCalled()
  })

  it.each(['field', 'area'])('ignores the keys typed in the %s', (label) => {
    const { onNext, getByLabelText } = setup()
    fireEvent.keyDown(getByLabelText(label), { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('ignores the keys typed in editable content', () => {
    const { onNext, getByTestId } = setup()
    fireEvent.keyDown(getByTestId('editable-child'), { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('still answers when the focus is on a button', () => {
    const { onNext, getByText } = setup()
    fireEvent.keyDown(getByText('plain'), { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
  })

  it.each(['metaKey', 'ctrlKey', 'altKey', 'shiftKey'])('ignores J with %s held', (modifier) => {
    const { onNext } = setup()
    fireEvent.keyDown(document.body, { key: 'j', [modifier]: true })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('ignores the keys while a modal is open on top', () => {
    const { onNext } = setup()
    const modal = document.createElement('div')
    modal.className = 'ant-modal-wrap'
    document.body.appendChild(modal)
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
    modal.style.display = 'none'
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    modal.remove()
  })

  it('ignores the keys while another drawer is open on top', () => {
    const { onNext } = setup()
    const drawer = document.createElement('div')
    drawer.className = 'ant-drawer ant-drawer-open'
    document.body.appendChild(drawer)
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
    drawer.classList.remove('ant-drawer-open')
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    drawer.remove()
  })

  it('ignores the keys under a drawer nested inside its own', () => {
    const { onNext } = setup({
      children: <div className="ant-drawer ant-drawer-open" data-testid="nested" />
    })
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })

  it('keeps answering for an overlay opened before its own', () => {
    const modal = document.createElement('div')
    modal.className = 'ant-modal-wrap'
    document.body.appendChild(modal)
    const { onNext } = setup()
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).toHaveBeenCalledTimes(1)
    modal.remove()
  })

  it('stops listening once unmounted', () => {
    const { onNext, unmount } = setup()
    unmount()
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNext).not.toHaveBeenCalled()
  })
})
