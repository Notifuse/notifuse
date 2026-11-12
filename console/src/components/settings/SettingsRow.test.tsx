import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Form, Input } from 'antd'
import { SettingsRow, SettingsRows } from './SettingsRow'

describe('SettingsRow', () => {
  it('binds its label to the field and shows the help under it', () => {
    render(
      <SettingsRow label="Name" help="Shown in emails" htmlFor="ws-name">
        <input id="ws-name" />
      </SettingsRow>
    )
    expect(screen.getByLabelText('Name')).toBe(document.getElementById('ws-name'))
    expect(screen.getByText('Shown in emails')).toBeInTheDocument()
  })

  it('renders the label as text when no field id is given', () => {
    render(
      <SettingsRow label="Opens">
        <span>switch</span>
      </SettingsRow>
    )
    expect(screen.getByText('Opens').tagName).not.toBe('LABEL')
  })

  it('stacks in one column under 720px of its container, and marks its field cell', () => {
    const { container } = render(
      <SettingsRow label="Name">
        <input />
      </SettingsRow>
    )
    const row = container.querySelector('[data-settings-row]') as HTMLElement
    expect(row.className).toContain('@max-[720px]:grid-cols-1')
    expect(row.className).toContain('grid-cols-[minmax(0,2fr)_minmax(0,3fr)]')
    expect(container.querySelector('[data-settings-field]')).toBeInTheDocument()
  })

  it('places an action beside the label', () => {
    render(
      <SettingsRow label="Logo" action={<button>Remove</button>}>
        <span />
      </SettingsRow>
    )
    expect(screen.getByRole('button', { name: 'Remove' })).toBeInTheDocument()
  })

  it('shows the validation message of a form item under its field', async () => {
    const Harness = () => {
      const [form] = Form.useForm()
      return (
        <Form form={form}>
          <SettingsRows>
            <SettingsRow label="Name" htmlFor="name">
              <Form.Item name="name" rules={[{ required: true, message: 'Name is required' }]}>
                <Input id="name" />
              </Form.Item>
            </SettingsRow>
          </SettingsRows>
          <button type="button" onClick={() => form.validateFields().catch(() => {})}>
            validate
          </button>
        </Form>
      )
    }
    render(<Harness />)
    screen.getByRole('button', { name: 'validate' }).click()
    expect(await screen.findByText('Name is required')).toBeInTheDocument()
  })
})
