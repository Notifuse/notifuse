import React, { useState } from 'react'
import { Card, Space, Typography, Button, message } from 'antd'
import { BlogContentEditor } from '../components/blog_editor'
import type { JSONContent } from '@tiptap/react'

const { Title, Paragraph, Text } = Typography

// Sample initial content
const initialContent: JSONContent = {
  type: 'doc',
  content: [
    {
      type: 'heading',
      attrs: { level: 1 },
      content: [{ type: 'text', text: 'Blog Editor Debug Page' }]
    },
    {
      type: 'paragraph',
      content: [
        { type: 'text', text: 'This is a test paragraph. Try hovering over blocks to see the ' },
        { type: 'text', marks: [{ type: 'bold' }], text: 'drag handle' },
        { type: 'text', text: ' and ' },
        { type: 'text', marks: [{ type: 'bold' }], text: '+ button' },
        { type: 'text', text: '.' }
      ]
    },
    {
      type: 'heading',
      attrs: { level: 2 },
      content: [{ type: 'text', text: 'Features to Test' }]
    },
    {
      type: 'bulletList',
      content: [
        {
          type: 'listItem',
          content: [
            {
              type: 'paragraph',
              content: [{ type: 'text', text: 'Drag and drop blocks using the grip icon' }]
            }
          ]
        },
        {
          type: 'listItem',
          content: [
            {
              type: 'paragraph',
              content: [{ type: 'text', text: 'Click the + button to insert new blocks' }]
            }
          ]
        },
        {
          type: 'listItem',
          content: [
            {
              type: 'paragraph',
              content: [{ type: 'text', text: 'Type / to open the slash command menu' }]
            }
          ]
        },
        {
          type: 'listItem',
          content: [
            {
              type: 'paragraph',
              content: [{ type: 'text', text: 'Select text to see formatting options' }]
            }
          ]
        }
      ]
    },
    {
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Try some ' },
        { type: 'text', marks: [{ type: 'bold' }], text: 'bold' },
        { type: 'text', text: ', ' },
        { type: 'text', marks: [{ type: 'italic' }], text: 'italic' },
        { type: 'text', text: ', ' },
        { type: 'text', marks: [{ type: 'underline' }], text: 'underline' },
        { type: 'text', text: ', and ' },
        { type: 'text', marks: [{ type: 'code' }], text: 'inline code' },
        { type: 'text', text: '.' }
      ]
    },
    {
      type: 'codeBlock',
      content: [
        {
          type: 'text',
          text: 'function hello() {\n  console.log("Hello, world!");\n}'
        }
      ]
    },
    {
      type: 'blockquote',
      content: [
        {
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'This is a blockquote. Use it for important callouts or citations.'
            }
          ]
        }
      ]
    }
  ]
}

export const DebugEditorPage: React.FC = () => {
  const [content, setContent] = useState<JSONContent>(initialContent)
  const [isSaving, setIsSaving] = useState(false)
  const [lastSaved, setLastSaved] = useState<Date | null>(null)

  const handleChange = (newContent: JSONContent) => {
    setContent(newContent)
  }

  const handleSave = () => {
    setIsSaving(true)
    // Simulate save
    setTimeout(() => {
      setIsSaving(false)
      setLastSaved(new Date())
      message.success('Content saved (simulated)')
    }, 500)
  }

  const handleReset = () => {
    setContent(initialContent)
    message.info('Content reset to initial state')
  }

  const handleClear = () => {
    setContent({
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: []
        }
      ]
    })
    message.info('Editor cleared')
  }

  const handleCopyJSON = () => {
    navigator.clipboard.writeText(JSON.stringify(content, null, 2))
    message.success('JSON content copied to clipboard')
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1400px', margin: '0 auto' }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Card>
          <Space direction="vertical" size="small">
            <Title level={2} style={{ margin: 0 }}>
              Blog Editor Debug Page
            </Title>
            <Paragraph style={{ margin: 0 }}>
              Test the blog editor features including drag handles, floating menu, and slash commands.
            </Paragraph>
          </Space>
        </Card>

        <Card>
          <Space style={{ marginBottom: 16 }} wrap>
            <Button onClick={handleSave} type="primary" loading={isSaving}>
              Save (Simulated)
            </Button>
            <Button onClick={handleReset}>Reset to Initial Content</Button>
            <Button onClick={handleClear}>Clear Editor</Button>
            <Button onClick={handleCopyJSON}>Copy JSON to Clipboard</Button>
            {lastSaved && (
              <Text type="secondary">Last saved: {lastSaved.toLocaleTimeString()}</Text>
            )}
          </Space>

          <BlogContentEditor
            content={content}
            onChange={handleChange}
            placeholder="Start writing your blog post or type / to browse options..."
            autoFocus={false}
            minHeight="600px"
            isSaving={isSaving}
            lastSaved={lastSaved}
          />
        </Card>

        <Card title="Current Content (JSON)">
          <pre
            style={{
              maxHeight: '300px',
              overflow: 'auto',
              background: '#f5f5f5',
              padding: '12px',
              borderRadius: '4px',
              fontSize: '12px'
            }}
          >
            {JSON.stringify(content, null, 2)}
          </pre>
        </Card>
      </Space>
    </div>
  )
}

export default DebugEditorPage

