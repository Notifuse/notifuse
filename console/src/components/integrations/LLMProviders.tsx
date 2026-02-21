import React from 'react'
import { IntegrationType, LLMProviderKind } from '../../services/api/types'

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
    defaultModel: 'claude-sonnet-4-6',
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
  }
]
