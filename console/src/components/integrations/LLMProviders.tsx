import React from 'react'
import { IntegrationType, LLMProviderKind } from '../../services/api/types'
import { ANTHROPIC_DEFAULT_MODEL, GEMINI_DEFAULT_MODEL, OPENAI_DEFAULT_MODEL } from './llmModels'

export interface LLMProviderInfo {
  type: IntegrationType
  kind: LLMProviderKind
  name: string
  defaultModel: string
  getIcon: (className?: string, size?: 'small' | 'large' | number) => React.ReactNode
}

export const getLLMProviderName = (kind: string): string => {
  switch (kind) {
    case 'anthropic':
      return 'Anthropic'
    case 'openai':
      return 'OpenAI'
    case 'gemini':
      return 'Google Gemini'
    default:
      return kind
  }
}

export const getLLMProviderIcon = (
  source: string,
  size: 'small' | 'large' | number = 'small'
): React.ReactNode => {
  const provider = llmProviders.find((p) => p.kind === source)
  if (provider) {
    return provider.getIcon('', size)
  }
  return null
}

export const llmProviders: LLMProviderInfo[] = [
  {
    type: 'llm',
    kind: 'anthropic',
    name: 'Anthropic',
    defaultModel: ANTHROPIC_DEFAULT_MODEL,
    getIcon: (className = '', size: 'small' | 'large' | number = 'small') => {
      const height = typeof size === 'number' ? size : size === 'small' ? 12 : 18
      // Wide wordmark logo needs max-width to prevent it being too wide
      const maxWidth = height * 5 // Reasonable aspect ratio constraint
      return (
        <img
          src="/console/anthropic.png"
          alt="Anthropic"
          style={{ height, maxWidth, objectFit: 'contain', display: 'inline-block' }}
          className={className}
        />
      )
    }
  },
  {
    type: 'llm',
    kind: 'openai',
    name: 'OpenAI',
    defaultModel: OPENAI_DEFAULT_MODEL,
    getIcon: (className = '', size: 'small' | 'large' | number = 'small') => {
      const height = typeof size === 'number' ? size : size === 'small' ? 12 : 18
      const maxWidth = height * 5
      return (
        <img
          src="/console/openai.png"
          alt="OpenAI"
          style={{ height, maxWidth, objectFit: 'contain', display: 'inline-block' }}
          className={className}
        />
      )
    }
  },
  {
    type: 'llm',
    kind: 'gemini',
    name: 'Google Gemini',
    defaultModel: GEMINI_DEFAULT_MODEL,
    getIcon: (className = '', size: 'small' | 'large' | number = 'small') => {
      const height = typeof size === 'number' ? size : size === 'small' ? 12 : 18
      // Wide wordmark logo (the spark + "Gemini"); constrain width like the other providers
      const maxWidth = height * 5
      return (
        <img
          src="/console/gemini.svg"
          alt="Google Gemini"
          style={{ height, maxWidth, objectFit: 'contain', display: 'inline-block' }}
          className={className}
        />
      )
    }
  }
]
