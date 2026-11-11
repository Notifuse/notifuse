import { describe, it, expect, vi, type Mock } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import MjmlCodeEditor from './MjmlCodeEditor'

// Monaco does not run in jsdom: a textarea stands in for it.
vi.mock('@monaco-editor/react', () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (v: string) => void }) => (
    <textarea data-testid="monaco" value={value} onChange={(e) => onChange?.(e.target.value)} />
  )
}))

const source = '<mjml><mj-body><mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>'

const renderEditor = (onCompile: Mock) =>
  render(
    <App>
      <MjmlCodeEditor
        mjmlSource={source}
        onMjmlSourceChange={vi.fn()}
        onCompile={onCompile}
        onTestDataChange={vi.fn()}
        height={600}
      />
    </App>
  )

describe('MjmlCodeEditor text version tab', () => {
  it('shows the text version of the compiled email', async () => {
    const onCompile = vi.fn().mockResolvedValue({
      html: '<p>Hi</p>',
      mjml: source,
      text: 'Hi\n\nShop (https://example.com/shop)\n'
    })
    renderEditor(onCompile)
    await waitFor(() => expect(onCompile).toHaveBeenCalled())

    fireEvent.click(screen.getByText('Text version'))

    await waitFor(() =>
      expect(screen.getByTestId('text-version').textContent).toBe(
        'Hi\n\nShop (https://example.com/shop)\n'
      )
    )
  })

  it('says the email goes out as HTML only when the compile has no text', async () => {
    const onCompile = vi.fn().mockResolvedValue({ html: '<img src="x.gif">', mjml: source, text: '' })
    renderEditor(onCompile)
    await waitFor(() => expect(onCompile).toHaveBeenCalled())

    fireEvent.click(screen.getByText('Text version'))

    await waitFor(() =>
      expect(screen.getByText('No text version: this email will be sent as HTML only.')).toBeTruthy()
    )
  })

  it('drops the text of a previous compile when the next one fails', async () => {
    const onCompile = vi
      .fn()
      .mockResolvedValueOnce({ html: '<p>Hi</p>', mjml: source, text: 'Old text\n' })
      .mockResolvedValue({ html: '', mjml: '', error: { message: 'Broken', details: [] } })
    renderEditor(onCompile)
    await waitFor(() => expect(onCompile).toHaveBeenCalledTimes(1))

    // An edit in the code tab recompiles, after a debounce.
    fireEvent.change(screen.getByTestId('monaco'), { target: { value: '<mjml><mj-body>' } })
    await waitFor(() => expect(onCompile).toHaveBeenCalledTimes(2), { timeout: 2000 })

    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() => expect(screen.getByText('The email could not be compiled')).toBeTruthy())
    expect(screen.queryByText('Old text')).toBeNull()
  })
})

describe('MjmlCodeEditor text version tab with a custom text', () => {
  it('lets the text be written when the editor is given a custom text', async () => {
    const onCompile = vi.fn().mockResolvedValue({ html: '', mjml: '', error: { message: 'Broken', details: [] } })
    const onChange = vi.fn()
    render(
      <App>
        <MjmlCodeEditor
          mjmlSource={source}
          onMjmlSourceChange={vi.fn()}
          onCompile={onCompile}
          onTestDataChange={vi.fn()}
          height={600}
          textVersion={{ custom: { value: 'Mine', onChange, onSeed: vi.fn(), category: 'transactional' } }}
        />
      </App>
    )
    await waitFor(() => expect(onCompile).toHaveBeenCalled())

    fireEvent.click(screen.getAllByText('Text version')[0])
    const area = (await screen.findByTestId('custom-text-input')) as HTMLTextAreaElement
    expect(area.value).toBe('Mine')
    fireEvent.change(area, { target: { value: 'Mine, edited' } })
    expect(onChange).toHaveBeenCalledWith('Mine, edited')
  })

  it('stays read-only without a custom text', async () => {
    const onCompile = vi.fn().mockResolvedValue({ html: '<p>Hi</p>', mjml: source, text: 'Hi\n' })
    renderEditor(onCompile)
    fireEvent.click(screen.getByText('Text version'))
    await waitFor(() => expect(screen.getByTestId('text-version')).toBeTruthy())
    expect(screen.queryByTestId('custom-text-input')).toBeNull()
    expect(screen.queryByRole('radio', { name: /Custom/ })).toBeNull()
  })
})
