import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { KeyValueList } from './KeyValueList'

describe('KeyValueList', () => {
  it('lays labels and values out in two columns, a dash for an empty value', () => {
    const { container } = render(
      <KeyValueList
        items={[
          { label: 'List', value: 'Newsletter' },
          { label: 'Reply-to', value: null },
          { label: 'Sender', value: '' }
        ]}
      />
    )
    expect(screen.getByText('List')).toBeInTheDocument()
    expect(screen.getByText('Newsletter')).toBeInTheDocument()
    expect(screen.getAllByText('—')).toHaveLength(2)
    expect((container.firstChild as HTMLElement).style.gridTemplateColumns).toBe('140px minmax(0, 1fr)')
  })

  it('marks the list, its labels and its values data-unreset against the margins of the antd reset', () => {
    const { container } = render(<KeyValueList items={[{ label: 'Email', value: 'a@b.c' }]} />)
    expect(container.querySelector('dl')).toHaveAttribute('data-unreset')
    expect(container.querySelector('dt')).toHaveAttribute('data-unreset')
    expect(container.querySelector('dd')).toHaveAttribute('data-unreset')
  })

  it('takes another label width', () => {
    const { container } = render(<KeyValueList labelWidth={100} items={[{ label: 'A', value: 'B' }]} />)
    expect((container.firstChild as HTMLElement).style.gridTemplateColumns).toBe('100px minmax(0, 1fr)')
  })

  it('adds a hint under a value', () => {
    render(<KeyValueList items={[{ label: 'Opens', value: '52%', hint: 'Partial tracking' }]} />)
    expect(screen.getByText('Partial tracking')).toBeInTheDocument()
  })
})

describe('KeyValueList stacked', () => {
  it('puts each label above its value, in one column by default', () => {
    const { container } = render(
      <KeyValueList layout="stacked" items={[{ label: 'Region', value: 'eu-west-3' }, { label: 'Port', value: null }]} />
    )
    const list = container.querySelector('dl') as HTMLElement
    expect(list.style.gridTemplateColumns).toBe('')
    expect(list).toHaveAttribute('data-columns', '1')
    expect(screen.getByText('Region')).toBeInTheDocument()
    expect(screen.getByText('eu-west-3')).toBeInTheDocument()
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it.each([2, 3] as const)('lays the entries out in %i columns', (columns) => {
    const { container } = render(
      <KeyValueList layout="stacked" columns={columns} items={[{ label: 'A', value: 'B' }]} />
    )
    expect(container.querySelector('dl')).toHaveAttribute('data-columns', String(columns))
  })

  it('keeps the inline layout by default', () => {
    const { container } = render(<KeyValueList items={[{ label: 'A', value: 'B' }]} />)
    expect(container.firstChild).not.toHaveAttribute('data-columns')
  })

  it('shows a hint under a stacked value', () => {
    render(<KeyValueList layout="stacked" items={[{ label: 'Limit', value: '20', hint: 'per hour' }]} />)
    expect(screen.getByText('per hour')).toBeInTheDocument()
  })
})
