import { useState, useRef, useEffect } from 'react'
import { Button, message, Popover } from 'antd'
import { CloseOutlined } from '@ant-design/icons'
import { Sparkles, User, Search, Globe } from 'lucide-react'
import { Bubble, Sender } from '@ant-design/x'
import { XMarkdown } from '@ant-design/x-markdown'
import '@ant-design/x-markdown/dist/x-markdown.css'
import { llmApi, LLMChatEvent, LLMMessage, LLMTool } from '../../services/api/llm'
import type { Workspace } from '../../services/api/workspace'

interface BlogMetadata {
  title?: string
  excerpt?: string
  meta_title?: string
  meta_description?: string
  keywords?: string[]
  og_title?: string
  og_description?: string
}

interface BlogAIAssistantProps {
  workspace: Workspace
  onUpdateContent: (json: Record<string, unknown>) => void
  onUpdateMetadata: (metadata: BlogMetadata) => void
  currentContent?: Record<string, unknown> | null
  currentMetadata?: BlogMetadata
}

interface ChatMessage {
  key: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  loading?: boolean
  toolName?: string
}

// Constants
const MAX_TOKENS = 4096
const TOOL_NAMES = {
  UPDATE_CONTENT: 'update_blog_content',
  UPDATE_METADATA: 'update_blog_metadata',
  SCRAPE_URL: 'scrape_url',
  SEARCH_WEB: 'search_web'
} as const

// Helper to extract plain text from Tiptap JSON
function extractTextFromTiptap(doc: Record<string, unknown>): string {
  const extractFromNode = (node: Record<string, unknown>): string => {
    if (node.type === 'text' && typeof node.text === 'string') {
      return node.text
    }
    if (Array.isArray(node.content)) {
      return node.content
        .map((child) => extractFromNode(child as Record<string, unknown>))
        .join('')
    }
    return ''
  }

  const processBlock = (node: Record<string, unknown>): string => {
    const text = extractFromNode(node)
    // Add newlines after block elements
    if (['paragraph', 'heading', 'listItem', 'blockquote'].includes(node.type as string)) {
      return text + '\n'
    }
    if (node.type === 'bulletList' || node.type === 'orderedList') {
      return (
        (Array.isArray(node.content)
          ? node.content.map((item) => '• ' + extractFromNode(item as Record<string, unknown>)).join('\n')
          : '') + '\n'
      )
    }
    return text
  }

  if (doc.type === 'doc' && Array.isArray(doc.content)) {
    return doc.content
      .map((node) => processBlock(node as Record<string, unknown>))
      .join('')
      .trim()
  }
  return ''
}

// Tool definition for updating blog content
const UPDATE_BLOG_TOOL: LLMTool = {
  name: 'update_blog_content',
  description:
    'Update the blog post content in the editor. Use this when you have generated or modified content for the user.',
  input_schema: {
    type: 'object',
    properties: {
      content: {
        type: 'object',
        description:
          'Tiptap JSON document with type "doc" and content array containing heading, paragraph, bulletList, etc.'
      },
      message: {
        type: 'string',
        description: 'Brief message to show the user about what was updated'
      }
    },
    required: ['content', 'message']
  }
}

// Tool definition for updating blog metadata (title, excerpt, SEO, Open Graph)
const UPDATE_METADATA_TOOL: LLMTool = {
  name: 'update_blog_metadata',
  description:
    'Update the blog post metadata including title, excerpt, SEO settings (meta title, meta description, keywords), and Open Graph settings (og_title, og_description). Use this when asked to generate or update titles, descriptions, SEO content, or social sharing metadata. Only include fields you want to update.',
  input_schema: {
    type: 'object',
    properties: {
      title: {
        type: 'string',
        description: 'The blog post title (max 500 characters)'
      },
      excerpt: {
        type: 'string',
        description: 'Brief summary shown in post listings and previews (max 500 characters)'
      },
      meta_title: {
        type: 'string',
        description: 'SEO meta title for search engines (recommended 50-60 characters)'
      },
      meta_description: {
        type: 'string',
        description: 'SEO meta description for search results (recommended 150-160 characters)'
      },
      keywords: {
        type: 'array',
        items: { type: 'string' },
        description: 'SEO keywords as an array of strings'
      },
      og_title: {
        type: 'string',
        description: 'Open Graph title for social media sharing (max 60 characters)'
      },
      og_description: {
        type: 'string',
        description: 'Open Graph description for social media sharing (max 160 characters)'
      },
      message: {
        type: 'string',
        description: 'Brief message to show the user about what was updated'
      }
    },
    required: ['message']
  }
}

const SYSTEM_PROMPT = `You are a helpful blog writing assistant. Have natural conversations and help create blog content.

You have two tools available:
1. update_blog_content - Updates the blog post body content with Tiptap JSON
2. update_blog_metadata - Updates title, excerpt, SEO settings, and Open Graph settings

## IMPORTANT: When to use which tool

**Writing a blog post / Creating content / "Write about X":**
- You MUST use update_blog_content to create the actual article body
- The content is the PRIMARY deliverable - always generate it first
- After creating content, optionally use update_blog_metadata for title/excerpt

**Only metadata requests (title, SEO, excerpt, etc.):**
- Use update_blog_metadata when ONLY asked about titles, SEO, or metadata
- Do NOT skip content creation just because metadata is easier

You have access to the current blog content and metadata below. Use this to answer questions, suggest improvements, or generate relevant SEO content.

## Tiptap JSON Quick Start

Simple blog post example:
{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Your Title"}]},{"type":"paragraph","content":[{"type":"text","text":"Your paragraph text here."}]}]}

## Tiptap JSON Rules

1. Root must be: { "type": "doc", "content": [...] }
2. Content array contains block nodes (paragraph, heading, bulletList, etc.)
3. Text nodes go inside block nodes: { "type": "text", "text": "..." }
4. Formatting uses marks array: { "type": "text", "text": "bold", "marks": [{ "type": "bold" }] }

## Block Node Types

### paragraph
{ "type": "paragraph", "attrs": { "textAlign": "left" }, "content": [text nodes] }
- textAlign: "left" | "center" | "right" | "justify" (optional, default: "left")

### heading
{ "type": "heading", "attrs": { "level": 2, "textAlign": "left" }, "content": [text nodes] }
- level: 1-6 (required)
- textAlign: "left" | "center" | "right" | "justify" (optional)

### bulletList / orderedList
{ "type": "bulletList", "content": [listItem nodes] }
{ "type": "orderedList", "attrs": { "start": 1 }, "content": [listItem nodes] }
- start: number (optional, for orderedList only)

### listItem
{ "type": "listItem", "content": [paragraph or other block nodes] }
- MUST contain at least one paragraph inside!

### blockquote
{ "type": "blockquote", "content": [paragraph nodes] }

### codeBlock
{ "type": "codeBlock", "attrs": { "language": "javascript" }, "content": [{ "type": "text", "text": "code here" }] }
- language: string (e.g., "javascript", "typescript", "python", "go", "json", "html", "css", "bash", "sql", "yaml", "markdown", "plaintext")

### horizontalRule
{ "type": "horizontalRule" }

### hardBreak (line break within paragraph)
{ "type": "hardBreak" }

### image
{ "type": "image", "attrs": { "src": "https://...", "alt": "description", "align": "center", "width": 600 } }
- src: string (required, must be a REAL image URL - do NOT use placeholder URLs)
- alt: string (optional, accessibility text)
- align: "left" | "center" | "right" (optional, default: "left")
- width: number (optional, pixels)
- IMPORTANT: Only include image nodes with real, working URLs. If you need images, use search_web to find images on Unsplash (e.g., "site:unsplash.com [topic]"). Never use placeholder URLs - they will show as broken.

### youtube
{ "type": "youtube", "attrs": { "src": "VIDEO_ID", "width": 640, "align": "center" } }
- src: string (required, YouTube video ID only, e.g., "dQw4w9WgXcQ")
- width: number (optional, default: 640)
- height: number (optional, default: 315)
- align: "left" | "center" | "right" (optional, default: "left")
- start: number (optional, start time in seconds)

## Text Marks (Inline Formatting)

Apply to text nodes via "marks" array:

- bold: { "type": "bold" }
- italic: { "type": "italic" }
- underline: { "type": "underline" }
- strike: { "type": "strike" }
- code: { "type": "code" }
- subscript: { "type": "subscript" }
- superscript: { "type": "superscript" }
- link: { "type": "link", "attrs": { "href": "https://...", "target": "_blank" } }
- textStyle (for color): { "type": "textStyle", "attrs": { "color": "#ff0000" } }
- highlight: { "type": "highlight", "attrs": { "color": "#ffff00" } }

Example with multiple marks:
{ "type": "text", "text": "bold red text", "marks": [{ "type": "bold" }, { "type": "textStyle", "attrs": { "color": "#ff0000" } }] }

## Common Mistakes to Avoid

- DON'T put text directly in doc.content - wrap in paragraph/heading
- DON'T forget "content" array for nodes that need children
- DON'T use "children" - always use "content"
- DON'T put listItem directly in doc - wrap in bulletList/orderedList
- DON'T forget paragraph inside listItem
- DON'T use full YouTube URLs - use only the video ID
- DON'T forget the "type": "text" wrapper for actual text content
- DON'T include image nodes with placeholder/fake URLs - search Unsplash for real images

Be conversational and helpful. Ask clarifying questions if needed.`

export function BlogAIAssistant({
  workspace,
  onUpdateContent,
  onUpdateMetadata,
  currentContent,
  currentMetadata
}: BlogAIAssistantProps) {
  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [costs, setCosts] = useState({ input: 0, output: 0, total: 0 })
  const abortControllerRef = useRef<AbortController | null>(null)
  const inputContainerRef = useRef<HTMLDivElement | null>(null)

  // Focus the input when opening
  useEffect(() => {
    if (open) {
      // Small delay to ensure the DOM is ready
      setTimeout(() => {
        const textarea = inputContainerRef.current?.querySelector('textarea')
        textarea?.focus()
      }, 100)
    }
  }, [open])

  const llmIntegration = workspace.integrations?.find((i) => i.type === 'llm')

  const handleCancel = () => {
    // Abort the current request
    abortControllerRef.current?.abort()
    setIsStreaming(false)
    // Remove loading state from any pending messages
    setMessages((prev) =>
      prev
        .map((m) => (m.loading ? { ...m, loading: false, content: m.content || '(Cancelled)' } : m))
        .filter((m) => m.content.trim()) // Remove empty messages
    )
  }

  // Helper to insert tool message before assistant message
  const insertToolMessage = (
    assistantKey: string,
    content: string,
    toolName: string,
    loading = false
  ) => {
    setMessages((prev) => {
      const assistantIndex = prev.findIndex((m) => m.key === assistantKey)
      const newToolMessage: ChatMessage = {
        key: `tool-${Date.now()}`,
        role: 'tool',
        content,
        toolName,
        loading
      }

      if (assistantIndex === -1) {
        return [...prev, newToolMessage]
      }

      const assistant = prev[assistantIndex]
      if (!assistant.content.trim()) {
        // Remove empty assistant, insert tool at its position
        return [...prev.slice(0, assistantIndex), newToolMessage, ...prev.slice(assistantIndex + 1)]
      }

      // Insert tool before assistant (which has content)
      return [
        ...prev.slice(0, assistantIndex),
        newToolMessage,
        { ...assistant, loading: false },
        ...prev.slice(assistantIndex + 1)
      ]
    })
  }

  // Event Handlers
  const handleTextEvent = (event: LLMChatEvent, assistantKey: string) => {
    if (!event.content) return
    setMessages((prev) =>
      prev.map((m) =>
        m.key === assistantKey ? { ...m, content: m.content + event.content, loading: false } : m
      )
    )
  }

  const handleContentToolUse = (event: LLMChatEvent, assistantKey: string) => {
    const input = event.tool_input as { content: Record<string, unknown>; message: string }
    if (!input?.content) return
    onUpdateContent(input.content)
    const toolMsg = input.message || 'Content updated'
    insertToolMessage(assistantKey, toolMsg, TOOL_NAMES.UPDATE_CONTENT)
    message.success(toolMsg)
  }

  const handleMetadataToolUse = (event: LLMChatEvent, assistantKey: string) => {
    const input = event.tool_input as BlogMetadata & { message: string }
    if (!input) return
    const metadata: BlogMetadata = {}
    if (input.title !== undefined) metadata.title = input.title
    if (input.excerpt !== undefined) metadata.excerpt = input.excerpt
    if (input.meta_title !== undefined) metadata.meta_title = input.meta_title
    if (input.meta_description !== undefined) metadata.meta_description = input.meta_description
    if (input.keywords !== undefined) metadata.keywords = input.keywords
    if (input.og_title !== undefined) metadata.og_title = input.og_title
    if (input.og_description !== undefined) metadata.og_description = input.og_description
    onUpdateMetadata(metadata)
    const toolMsg = input.message || 'Metadata updated'
    insertToolMessage(assistantKey, toolMsg, TOOL_NAMES.UPDATE_METADATA)
    message.success(toolMsg)
  }

  const handleServerToolStart = (event: LLMChatEvent, assistantKey: string) => {
    const toolInput = event.tool_input || {}
    let displayText = `Using ${event.tool_name}...`
    if (event.tool_name === TOOL_NAMES.SCRAPE_URL && toolInput.url) {
      displayText = `Fetching: ${toolInput.url}`
    } else if (event.tool_name === TOOL_NAMES.SEARCH_WEB && toolInput.query) {
      displayText = `Searching: "${toolInput.query}"`
    }
    insertToolMessage(assistantKey, displayText, event.tool_name || '', true)
  }

  const handleServerToolResult = (event: LLMChatEvent) => {
    setMessages((prev) => {
      const lastToolIndex = [...prev]
        .reverse()
        .findIndex((m) => m.role === 'tool' && m.toolName === event.tool_name && m.loading)
      if (lastToolIndex === -1) return prev
      const actualIndex = prev.length - 1 - lastToolIndex
      const currentMessage = prev[actualIndex]
      let statusText = currentMessage.content.replace('...', '')
      statusText += event.error ? ' - Failed' : ' - Done'
      return prev.map((m, i) =>
        i === actualIndex ? { ...m, content: statusText, loading: false } : m
      )
    })
  }

  const handleDoneEvent = (event: LLMChatEvent, assistantKey: string) => {
    if (event.input_cost !== undefined || event.output_cost !== undefined) {
      setCosts((prev) => ({
        input: prev.input + (event.input_cost || 0),
        output: prev.output + (event.output_cost || 0),
        total: prev.total + (event.total_cost || 0)
      }))
    }
    setMessages((prev) => prev.map((m) => (m.key === assistantKey ? { ...m, loading: false } : m)))
    setIsStreaming(false)
  }

  const handleErrorEvent = (event: LLMChatEvent, assistantKey: string) => {
    setMessages((prev) =>
      prev.map((m) =>
        m.key === assistantKey ? { ...m, content: `Error: ${event.error}`, loading: false } : m
      )
    )
    setIsStreaming(false)
  }

  const handleSend = async () => {
    if (!inputValue.trim() || !llmIntegration || isStreaming) return

    const userMessage: ChatMessage = {
      key: `user-${Date.now()}`,
      role: 'user',
      content: inputValue
    }

    const assistantKey = `assistant-${Date.now()}`
    const assistantMessage: ChatMessage = {
      key: assistantKey,
      role: 'assistant',
      content: '',
      loading: true
    }

    setMessages((prev) => [...prev, userMessage, assistantMessage])
    setInputValue('')
    setIsStreaming(true)

    // Build context-aware system prompt
    let systemPrompt = SYSTEM_PROMPT
    if (currentMetadata?.title) systemPrompt += `\n\nCurrent blog title: "${currentMetadata.title}"`
    if (currentMetadata?.excerpt) systemPrompt += `\nCurrent excerpt: "${currentMetadata.excerpt}"`
    if (currentMetadata?.meta_title) systemPrompt += `\nCurrent meta title: "${currentMetadata.meta_title}"`
    if (currentMetadata?.meta_description)
      systemPrompt += `\nCurrent meta description: "${currentMetadata.meta_description}"`
    if (currentMetadata?.keywords?.length)
      systemPrompt += `\nCurrent keywords: ${currentMetadata.keywords.join(', ')}`
    if (currentMetadata?.og_title) systemPrompt += `\nCurrent OG title: "${currentMetadata.og_title}"`
    if (currentMetadata?.og_description)
      systemPrompt += `\nCurrent OG description: "${currentMetadata.og_description}"`
    if (currentContent) {
      const contentText = extractTextFromTiptap(currentContent)
      if (contentText) {
        systemPrompt += `\n\n## Current Blog Content\n\n${contentText}`
      }
    }

    const apiMessages: LLMMessage[] = messages
      .filter((m) => m.role !== 'tool' && m.content.trim()) // Don't send tool messages or empty content
      .map((m) => ({ role: m.role as 'user' | 'assistant', content: m.content }))
    apiMessages.push({ role: 'user', content: inputValue })

    abortControllerRef.current = new AbortController()

    try {
      await llmApi.streamChat(
        {
          workspace_id: workspace.id,
          integration_id: llmIntegration.id,
          messages: apiMessages,
          system_prompt: systemPrompt,
          max_tokens: MAX_TOKENS,
          tools: [UPDATE_BLOG_TOOL, UPDATE_METADATA_TOOL]
        },
        (event: LLMChatEvent) => {
          switch (event.type) {
            case 'text':
              handleTextEvent(event, assistantKey)
              break
            case 'tool_use':
              if (event.tool_name === TOOL_NAMES.UPDATE_CONTENT) {
                handleContentToolUse(event, assistantKey)
              } else if (event.tool_name === TOOL_NAMES.UPDATE_METADATA) {
                handleMetadataToolUse(event, assistantKey)
              }
              break
            case 'server_tool_start':
              handleServerToolStart(event, assistantKey)
              break
            case 'server_tool_result':
              handleServerToolResult(event)
              break
            case 'done':
              handleDoneEvent(event, assistantKey)
              break
            case 'error':
              handleErrorEvent(event, assistantKey)
              break
          }
        },
        (error) => {
          console.error('LLM error:', error)
          setIsStreaming(false)
        },
        { signal: abortControllerRef.current.signal }
      )
    } catch (error) {
      console.error('Failed to stream:', error)
      setIsStreaming(false)
    }
  }

  const bubbleItems = messages.map((m) => {
    // Determine if this is a server-side tool (scrape_url, search_web) or client-side tool
    const isServerTool =
      m.toolName === TOOL_NAMES.SCRAPE_URL || m.toolName === TOOL_NAMES.SEARCH_WEB

    return {
      key: m.key,
      role: m.role === 'user' ? 'user' : m.role === 'tool' ? 'system' : 'ai',
      content: m.content,
      loading: m.loading,
      // Style tool messages: blue for server tools (no border), green for client tools
      ...(m.role === 'tool' && {
        styles: {
          content: isServerTool
            ? { background: '#e6f4ff' } // Blue for server tools, no border
            : { background: '#f6ffed', border: '1px solid #b7eb8f' } // Green for client tools
        }
      }),
      // Add icon for server tools
      ...(m.role === 'tool' && isServerTool && {
        avatar: {
          icon: m.toolName === 'search_web' ? <Search size={10} /> : <Globe size={10} />,
          size: 20,
          style: { background: '#1890ff', minWidth: 20, minHeight: 20 }
        }
      })
    }
  })

  if (!llmIntegration) return null

  return (
    <>
      {/* Floating trigger button */}
      {!open && (
        <Button
          type="primary"
          shape="circle"
          size="large"
          icon={<Sparkles size={24} />}
          onClick={() => setOpen(true)}
          style={{
            position: 'fixed',
            bottom: 24,
            right: 24,
            zIndex: 1000,
            width: 56,
            height: 56,
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
          }}
        />
      )}

      {/* Floating chat box */}
      {open && (
        <div
          style={{
            position: 'fixed',
            top: 66,
            bottom: 24,
            right: 24,
            width: 420,
            backgroundColor: '#fff',
            borderRadius: 12,
            boxShadow: '0 6px 24px rgba(0,0,0,0.15)',
            zIndex: 1000,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden'
          }}
        >
          {/* Header */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '12px 16px',
              borderBottom: '1px solid #f0f0f0',
              backgroundColor: '#fafafa'
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Sparkles size={18} color="#f59e0b" />
              <span style={{ fontWeight: 500 }}>AI Blog Assistant</span>
            </div>
            <Button
              type="text"
              size="small"
              icon={<CloseOutlined />}
              onClick={() => setOpen(false)}
            />
          </div>

          {/* Messages area */}
          <div style={{ flex: 1, overflow: 'hidden', padding: 12 }}>
            <Bubble.List
              autoScroll
              style={{ height: '100%' }}
              items={bubbleItems}
              roles={{
                user: {
                  placement: 'end',
                  avatar: {
                    icon: <User size={12} />,
                    style: { background: '#1890ff' }
                  }
                },
                ai: {
                  placement: 'start',
                  avatar: {
                    icon: <Sparkles size={12} />,
                    style: { background: '#7763F1' }
                  },
                  messageRender: (content) => (
                    <XMarkdown openLinksInNewTab>{content as string}</XMarkdown>
                  )
                },
                system: {
                  placement: 'start',
                  messageRender: (content) => {
                    const text = content as string
                    // Make URLs clickable
                    const urlRegex = /(https?:\/\/[^\s]+)/g
                    const parts = text.split(urlRegex)
                    return (
                      <span>
                        {parts.map((part, i) =>
                          urlRegex.test(part) ? (
                            <a
                              key={i}
                              href={part}
                              target="_blank"
                              rel="noopener noreferrer"
                              style={{ color: '#1890ff' }}
                            >
                              {part}
                            </a>
                          ) : (
                            part
                          )
                        )}
                      </span>
                    )
                  }
                }
              }}
            />
          </div>

          {/* Input area */}
          <div ref={inputContainerRef} style={{ padding: 12, borderTop: '1px solid #f0f0f0' }}>
            <Sender
              value={inputValue}
              onChange={setInputValue}
              onSubmit={handleSend}
              onCancel={handleCancel}
              loading={isStreaming}
              placeholder="Ask me to help write your blog..."
            />
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                fontSize: 11,
                color: '#8c8c8c',
                marginTop: 8
              }}
            >
              <Button
                type="link"
                size="small"
                style={{ fontSize: 11, padding: 0, height: 'auto' }}
                onClick={() => {
                  setMessages([])
                  setCosts({ input: 0, output: 0, total: 0 })
                }}
                disabled={isStreaming || messages.length === 0}
              >
                New conversation
              </Button>
              <Popover
                content={
                  <div style={{ fontSize: 12 }}>
                    <div>Input: ${costs.input.toFixed(4)}</div>
                    <div>Output: ${costs.output.toFixed(4)}</div>
                  </div>
                }
                trigger="hover"
                placement="top"
              >
                <span style={{ cursor: 'help' }}>Cost: ${costs.total.toFixed(4)}</span>
              </Popover>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
