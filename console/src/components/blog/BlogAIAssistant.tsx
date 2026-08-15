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

/* ---------------------------------------------------------------------------
 * Tool arguments arrive as `Record<string, unknown>` - whatever the model chose to
 * send. These guards narrow a single value at a time, so a handler never has to
 * assert a shape it has not checked.
 * ------------------------------------------------------------------------- */

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function asString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined
}

function asStringArray(value: unknown): string[] | undefined {
  return Array.isArray(value) && value.every((item): item is string => typeof item === 'string')
    ? value
    : undefined
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
        const input = event.tool_input
        const content = input?.content
        // The model can put anything in tool_input, so the Tiptap document is checked
        // for real instead of asserted: a non-object `content` was already a no-op
        // downstream (the editor only accepts a node whose type is "doc").
        if (!isRecord(content)) return
        onUpdateContent(content)
        const toolMsg = asString(input?.message) || 'Content updated'
        insert(toolMsg, BLOG_TOOL_NAMES.UPDATE_CONTENT)
        message.success(toolMsg)
      }
    ],
    [
      BLOG_TOOL_NAMES.UPDATE_METADATA,
      (event, insert) => {
        const input = event.tool_input
        if (!input) return

        // Every field is validated against the shape the tool schema promises, so a
        // malformed value is dropped rather than written into the form.
        const metadata: BlogMetadata = {}
        const title = asString(input.title)
        if (title !== undefined) metadata.title = title
        const excerpt = asString(input.excerpt)
        if (excerpt !== undefined) metadata.excerpt = excerpt
        const metaTitle = asString(input.meta_title)
        if (metaTitle !== undefined) metadata.meta_title = metaTitle
        const metaDescription = asString(input.meta_description)
        if (metaDescription !== undefined) metadata.meta_description = metaDescription
        const keywords = asStringArray(input.keywords)
        if (keywords !== undefined) metadata.keywords = keywords
        const ogTitle = asString(input.og_title)
        if (ogTitle !== undefined) metadata.og_title = ogTitle
        const ogDescription = asString(input.og_description)
        if (ogDescription !== undefined) metadata.og_description = ogDescription

        onUpdateMetadata(metadata)
        const toolMsg = asString(input.message) || 'Metadata updated'
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
