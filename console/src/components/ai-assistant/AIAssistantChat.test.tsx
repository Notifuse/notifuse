import { describe, it, expect, vi } from 'vitest'
import { useState } from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AIAssistantChat } from './AIAssistantChat'
import type {
  AIAssistantChatProps,
  AIAssistantConfig,
  AIAssistantSuggestion,
  BubbleItem
} from './types'
import type { Integration, Workspace } from '../../services/api/workspace'

// The Sender's auto-sizing textarea mounts a ResizeObserver; jsdom has none.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// Bubble.List watches a sentinel to decide whether it is scrolled to the bottom.
class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}
vi.stubGlobal('IntersectionObserver', IntersectionObserverStub)

// services/api/client imports the router, which imports every page and so cycles
// back into the module under test. Stubbing the client keeps that graph out.
vi.mock('../../services/api/client', () => ({
  api: { post: vi.fn().mockResolvedValue({}), get: vi.fn().mockResolvedValue({}) }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const config: AIAssistantConfig = {
  title: 'AI Assistant',
  icon: null,
  iconButton: null,
  iconLarge: null,
  iconColor: '#000',
  avatarColor: '#722ed1',
  placeholder: 'Ask anything',
  maxTokens: 1024,
  notConfiguredGradient: 'linear-gradient(#000, #fff)'
}

const workspace = { id: 'ws1', name: 'My WS' } as unknown as Workspace

const llmIntegration = {
  id: 'llm1',
  name: 'Claude',
  type: 'llm',
  llm_provider: { kind: 'anthropic' }
} as unknown as Integration

const baseProps: AIAssistantChatProps = {
  workspace,
  config,
  open: true,
  setOpen: vi.fn(),
  messages: [],
  inputValue: '',
  setInputValue: vi.fn(),
  isStreaming: false,
  costs: { input: 0, output: 0, total: 0 },
  inputContainerRef: { current: null },
  llmIntegration,
  llmIntegrations: [llmIntegration],
  setSelectedLLMIntegrationId: vi.fn(),
  handleCancel: vi.fn(),
  handleSend: vi.fn().mockResolvedValue(undefined),
  bubbleItems: [],
  resetConversation: vi.fn()
}

const renderChat = (overrides: Partial<AIAssistantChatProps> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <AIAssistantChat {...baseProps} {...overrides} />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

describe('AIAssistantChat', () => {
  it('renders a user bubble and an assistant bubble from the message list', () => {
    const bubbleItems: BubbleItem[] = [
      { key: 'm1', role: 'user', content: 'Write me a welcome email' },
      { key: 'm2', role: 'ai', content: 'Here is a draft.' }
    ]
    const { container } = renderChat({ bubbleItems })

    expect(screen.getByText('Write me a welcome email')).toBeInTheDocument()
    expect(screen.getByText('Here is a draft.')).toBeInTheDocument()

    // The user bubble is placed at the end of the row, the assistant one at the start.
    expect(container.querySelectorAll('.ant-bubble-end')).toHaveLength(1)
    expect(container.querySelectorAll('.ant-bubble-start')).toHaveLength(1)
  })

  it('renders assistant content as markdown', () => {
    renderChat({
      bubbleItems: [{ key: 'm1', role: 'ai', content: '**Bold answer**' }]
    })

    const rendered = screen.getByText('Bold answer')
    expect(rendered.tagName).toBe('STRONG')
  })

  it('renders tool output as a plain start-placed bubble, not the centered system banner', () => {
    const { container } = renderChat({
      bubbleItems: [{ key: 'm1', role: 'system', content: 'Opened https://example.com/report' }]
    })

    // Bubble.List routes the built-in "system" role to Bubble.System; tool results
    // must keep the ordinary bubble layout.
    expect(container.querySelector('.ant-bubble-system')).toBeNull()
    expect(container.querySelectorAll('.ant-bubble-start')).toHaveLength(1)

    const link = screen.getByRole('link', { name: 'https://example.com/report' })
    expect(link).toHaveAttribute('href', 'https://example.com/report')
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('linkifies every URL in a tool line, whatever the length of the one before it', () => {
    // Classification used to run a /g regex's .test() once per part inside the map, so
    // each verdict inherited the previous part's lastIndex. Two URLs in one line are
    // the shortest reproduction of "the answer depends on what came before".
    renderChat({
      bubbleItems: [
        {
          key: 'm1',
          role: 'system',
          content: 'Compared https://example.com/very/long/report/path/2026 and https://ex.io/b'
        }
      ]
    })

    expect(
      screen.getByRole('link', { name: 'https://example.com/very/long/report/path/2026' })
    ).toHaveAttribute('href', 'https://example.com/very/long/report/path/2026')
    expect(screen.getByRole('link', { name: 'https://ex.io/b' })).toHaveAttribute(
      'href',
      'https://ex.io/b'
    )
    // The prose around them stays prose.
    expect(screen.getAllByRole('link')).toHaveLength(2)
  })

  it('renders a borderless step line without the filled bubble treatment', () => {
    // The hook asks for the quiet variant per item; the panel must pass it through, or
    // every step goes back to carrying the same weight as the answer.
    const { container } = renderChat({
      bubbleItems: [
        {
          key: 'm1',
          role: 'system',
          content: 'Channel Group - 10 rows',
          variant: 'borderless',
          avatar: { icon: null, size: 16, style: { background: 'transparent' } }
        }
      ]
    })

    expect(container.querySelector('.ant-bubble-content-borderless')).toBeInTheDocument()
    expect(container.querySelector('.ant-bubble-content-filled')).toBeNull()
    // The avatar keeps the line indented in the step column.
    expect(container.querySelector('.ant-avatar')).toBeInTheDocument()
  })

  it('keeps a failed step loud, with the filled bubble the hook gives it', () => {
    const { container } = renderChat({
      bubbleItems: [
        {
          key: 'm1',
          role: 'system',
          content: 'Channel Group - failed',
          styles: { content: { background: '#fff2f0' } }
        }
      ]
    })

    const content = container.querySelector<HTMLElement>('.ant-bubble-content')
    expect(content).toBeInTheDocument()
    expect(container.querySelector('.ant-bubble-content-borderless')).toBeNull()
    expect(content?.style.background).toBe('rgb(255, 242, 240)')
  })

  it('collapses reasoning into a Thinking disclosure', () => {
    renderChat({
      bubbleItems: [{ key: 'm1-thinking', role: 'thinking', content: 'Weighing two subject lines' }]
    })

    expect(screen.getByText('Thinking')).toBeInTheDocument()
    expect(screen.getByText('Weighing two subject lines')).toBeInTheDocument()
  })

  it('submits the typed value through the Sender', () => {
    const handleSend = vi.fn().mockResolvedValue(undefined)
    const setInputValue = vi.fn()

    // The Sender is controlled by the hook, so drive it from real state to make
    // the submitted value observable.
    function Harness() {
      const [value, setValue] = useState('')
      return (
        <AIAssistantChat
          {...baseProps}
          inputValue={value}
          setInputValue={(next) => {
            setInputValue(next)
            setValue(next)
          }}
          handleSend={handleSend}
        />
      )
    }

    render(
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <Harness />
          </App>
        </ConfigProvider>
      </I18nProvider>
    )

    const textarea = screen.getByRole('textbox')
    fireEvent.change(textarea, { target: { value: 'Draft a welcome email' } })
    expect(setInputValue).toHaveBeenCalledWith('Draft a welcome email')
    expect(textarea).toHaveValue('Draft a welcome email')

    fireEvent.keyDown(textarea, { key: 'Enter', code: 'Enter', keyCode: 13 })
    expect(handleSend).toHaveBeenCalledTimes(1)
  })

  it('shows the loading affordances while streaming', () => {
    const { container } = renderChat({
      isStreaming: true,
      bubbleItems: [{ key: 'm1', role: 'ai', content: '', loading: true }]
    })

    // Sender swaps its send button for a cancel-while-loading button.
    expect(container.querySelector('[class*="loading-button"]')).toBeInTheDocument()
    // The pending assistant bubble shows the typing dots instead of content.
    expect(container.querySelector('.ant-bubble-dot')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'New conversation' })).toBeDisabled()
  })

  it('cancels the stream from the Sender', () => {
    const handleCancel = vi.fn()
    const { container } = renderChat({ isStreaming: true, handleCancel })

    const loadingButton = container.querySelector<HTMLElement>('[class*="loading-button"]')
    expect(loadingButton).not.toBeNull()
    fireEvent.click(loadingButton as HTMLElement)
    expect(handleCancel).toHaveBeenCalledTimes(1)
  })
})

describe('AIAssistantChat empty-state suggestions', () => {
  const suggestions: AIAssistantSuggestion[] = [
    { key: 'top-pages', label: 'Top pages', prompt: 'What are my top pages this week?' },
    { key: 'sources', label: 'Traffic sources', prompt: 'Where is my traffic coming from?' }
  ]

  // The chips are siblings of the bubble list inside the messages area, so that
  // element is the scope in which "is there a chip row?" can be answered without
  // catching the composer's own buttons.
  const messagesArea = (container: HTMLElement) => {
    const list = container.querySelector('.ant-bubble-list')
    expect(list).not.toBeNull()
    return (list as HTMLElement).parentElement as HTMLElement
  }

  it('shows no starter chips when the caller passes none, leaving the message list alone', () => {
    // Both shipped assistants spread a hook return that carries no `suggestions`;
    // if the guard ever inverted they would grow a stray chip row.
    const { container } = renderChat()

    const area = messagesArea(container)
    expect(area.querySelectorAll('button')).toHaveLength(0)
    expect(area.querySelector('.ant-bubble-list')).toBeInTheDocument()
  })

  it('renders the identical panel for an empty suggestion list and for no list at all', () => {
    const omitted = renderChat()
    const omittedHtml = messagesArea(omitted.container).innerHTML
    omitted.unmount()

    const empty = renderChat({ suggestions: [] })

    expect(messagesArea(empty.container).innerHTML).toBe(omittedHtml)
  })

  it('offers the starter chips only until the conversation has its first message', () => {
    const { container, rerender } = renderChat({ suggestions })

    expect(screen.getByRole('button', { name: 'Top pages' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Traffic sources' })).toBeInTheDocument()
    expect(messagesArea(container).querySelectorAll('button')).toHaveLength(suggestions.length)

    // Once the user has asked something the chips must get out of the way of the
    // transcript rather than sitting above every answer.
    rerender(
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <AIAssistantChat
              {...baseProps}
              suggestions={suggestions}
              bubbleItems={[{ key: 'm1', role: 'user', content: 'What are my top pages this week?' }]}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    )

    expect(screen.queryByRole('button', { name: 'Top pages' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Traffic sources' })).not.toBeInTheDocument()
  })

  it('hands the clicked chip prompt to the caller, not its label', () => {
    const onSuggestion = vi.fn()
    const setInputValue = vi.fn()
    renderChat({ suggestions, onSuggestion, setInputValue })

    fireEvent.click(screen.getByRole('button', { name: 'Traffic sources' }))

    expect(onSuggestion).toHaveBeenCalledTimes(1)
    expect(onSuggestion).toHaveBeenCalledWith('Where is my traffic coming from?')
    // The custom handler owns the click entirely; the composer must not also be filled.
    expect(setInputValue).not.toHaveBeenCalled()
  })

  it('fills the composer with the prompt when the caller gives no chip handler', () => {
    const setInputValue = vi.fn()
    renderChat({ suggestions, setInputValue })

    fireEvent.click(screen.getByRole('button', { name: 'Top pages' }))

    expect(setInputValue).toHaveBeenCalledWith('What are my top pages this week?')
  })

  it('blocks the starter chips while a response is streaming', () => {
    // Clicking a chip mid-stream would queue a second turn against a busy hook.
    const onSuggestion = vi.fn()
    renderChat({ suggestions, onSuggestion, isStreaming: true })

    const chip = screen.getByRole('button', { name: 'Top pages' })
    expect(chip).toBeDisabled()

    fireEvent.click(chip)
    expect(onSuggestion).not.toHaveBeenCalled()
  })
})

describe('AIAssistantChat panel width', () => {
  // The Blog and Email assistants render prose and pass no width; the analytics one
  // carries small metric tables and asks for more. The default is the fence that
  // keeps a change made for one of them off the other two.
  const panelOf = (container: HTMLElement) =>
    container.querySelector<HTMLElement>('div[style*="position: fixed"][style*="width"]')

  it('keeps the historical width when the caller asks for none', () => {
    const { container } = render(<AIAssistantChat {...baseProps} open />)
    expect(panelOf(container)?.style.width).toBe('420px')
  })

  it('widens the panel for a caller that asks for it', () => {
    const { container } = render(<AIAssistantChat {...baseProps} open width={520} />)
    expect(panelOf(container)?.style.width).toBe('520px')
  })
})
