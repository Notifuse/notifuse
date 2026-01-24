import { useLingui } from '@lingui/react/macro'
import { message } from 'antd'
import { Sparkles } from 'lucide-react'
import { useAIAssistant, AIAssistantChat } from '../ai-assistant'
import type { AIAssistantConfig, ToolHandler } from '../ai-assistant'
import type { Workspace } from '../../services/api/workspace'
import { BLOG_AI_SYSTEM_PROMPT } from './blog-ai-system-prompt'
import {
  BLOG_AI_TOOLS,
  BLOG_TOOL_NAMES,
  extractTextFromTiptap,
  type BlogMetadata
} from './blog-ai-tools'

interface BlogAIAssistantProps {
  workspace: Workspace
  onUpdateContent: (json: Record<string, unknown>) => void
  onUpdateMetadata: (metadata: BlogMetadata) => void
  currentContent?: Record<string, unknown> | null
  currentMetadata?: BlogMetadata
}

// Note: Config is created inside component to access t() for translations

export function BlogAIAssistant({
  workspace,
  onUpdateContent,
  onUpdateMetadata,
  currentContent,
  currentMetadata
}: BlogAIAssistantProps) {
  const { t } = useLingui()

  const config: AIAssistantConfig = {
    title: t`AI Blog Assistant`,
    icon: <Sparkles size={18} />,
    iconButton: <Sparkles size={24} />,
    iconLarge: <Sparkles size={32} />,
    iconColor: '#764ba2',
    avatarColor: '#764ba2',
    placeholder: t`Ask me to help write your blog...`,
    maxTokens: 4096,
    notConfiguredGradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)'
  }

  const buildSystemPrompt = () => {
    let systemPrompt = BLOG_AI_SYSTEM_PROMPT

    if (currentMetadata?.title) {
      systemPrompt += `\n\nCurrent blog title: "${currentMetadata.title}"`
    }
    if (currentMetadata?.excerpt) {
      systemPrompt += `\nCurrent excerpt: "${currentMetadata.excerpt}"`
    }
    if (currentMetadata?.meta_title) {
      systemPrompt += `\nCurrent meta title: "${currentMetadata.meta_title}"`
    }
    if (currentMetadata?.meta_description) {
      systemPrompt += `\nCurrent meta description: "${currentMetadata.meta_description}"`
    }
    if (currentMetadata?.keywords?.length) {
      systemPrompt += `\nCurrent keywords: ${currentMetadata.keywords.join(', ')}`
    }
    if (currentMetadata?.og_title) {
      systemPrompt += `\nCurrent OG title: "${currentMetadata.og_title}"`
    }
    if (currentMetadata?.og_description) {
      systemPrompt += `\nCurrent OG description: "${currentMetadata.og_description}"`
    }
    if (currentContent) {
      const contentText = extractTextFromTiptap(currentContent)
      if (contentText) {
        systemPrompt += `\n\n## Current Blog Content\n\n${contentText}`
      }
    }

    return systemPrompt
  }

  const toolHandlers = new Map<string, ToolHandler>([
    [
      BLOG_TOOL_NAMES.UPDATE_CONTENT,
      (event, insert) => {
        const input = event.tool_input as { content: Record<string, unknown>; message: string }
        if (!input?.content) return
        onUpdateContent(input.content)
        const toolMsg = input.message || 'Content updated'
        insert(toolMsg, BLOG_TOOL_NAMES.UPDATE_CONTENT)
        message.success(toolMsg)
      }
    ],
    [
      BLOG_TOOL_NAMES.UPDATE_METADATA,
      (event, insert) => {
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
        insert(toolMsg, BLOG_TOOL_NAMES.UPDATE_METADATA)
        message.success(toolMsg)
      }
    ]
  ])

  const assistant = useAIAssistant({
    workspace,
    config,
    tools: BLOG_AI_TOOLS,
    toolHandlers,
    buildSystemPrompt
  })

  return <AIAssistantChat {...assistant} workspace={workspace} config={config} />
}
