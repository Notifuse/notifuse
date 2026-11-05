import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Form } from 'antd'
import { OperatorEquals } from './operator_equals'
import { ContactsTableSchema } from './table_schemas'

const renderValueInput = (fieldName: string) => {
  const Wrapper = () => {
    const [form] = Form.useForm()
    return <Form form={form}>{new OperatorEquals().renderFormItems('string', fieldName, form)}</Form>
  }
  return render(<Wrapper />)
}

describe('tracking consent segment filters', () => {
  it('lists the consent fields on the contact schema', () => {
    const fields = ContactsTableSchema.fields
    expect(fields.tracking_opens_consent?.type).toBe('string')
    expect(fields.tracking_clicks_consent?.type).toBe('string')
    expect(fields.tracking_consent_source?.type).toBe('string')
    expect(fields.tracking_consent_updated_at?.type).toBe('time')
  })

  it.each(['tracking_opens_consent', 'tracking_clicks_consent'])(
    'offers granted and denied as the values of %s',
    async (fieldName) => {
      renderValueInput(fieldName)
      await userEvent.click(screen.getByRole('combobox'))
      expect(screen.getByText('Granted')).toBeTruthy()
      expect(screen.getByText('Denied')).toBeTruthy()
    }
  )

  it('offers the known sources as the values of tracking_consent_source', async () => {
    renderValueInput('tracking_consent_source')
    await userEvent.click(screen.getByRole('combobox'))
    expect(screen.getByText('Notification center')).toBeTruthy()
    expect(screen.getByText('Email link')).toBeTruthy()
  })
})
