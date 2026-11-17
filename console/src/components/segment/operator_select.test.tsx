import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Form } from 'antd'
import { FieldTypeString } from './type_string'
import { FieldTypeNumber } from './type_number'
import { FieldTypeTime } from './type_time'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver

describe('the operator of a condition', () => {
  it.each([
    ['string', new FieldTypeString()],
    ['number', new FieldTypeNumber()],
    ['time', new FieldTypeTime()]
  ] as const)('asks for an operator, not a value, on a %s field', (fieldType, renderer) => {
    const Wrapper = () => {
      const [form] = Form.useForm()
      return <Form form={form}>{renderer.renderFormItems(fieldType, 'country', form)}</Form>
    }
    render(<Wrapper />)
    expect(screen.getByText('Select an operator')).toBeInTheDocument()
    expect(screen.queryByText('select a value')).toBeNull()
  })
})
