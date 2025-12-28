import React from 'react'
import { IntegrationType } from '../../services/api/types'

export interface FirecrawlProviderInfo {
  type: IntegrationType
  name: string
  getIcon: (className?: string, size?: 'small' | 'large' | number) => React.ReactNode
}

export const firecrawlProvider: FirecrawlProviderInfo = {
  type: 'firecrawl',
  name: 'Firecrawl',
  getIcon: (className = '', size: 'small' | 'large' | number = 'small') => {
    const height = typeof size === 'number' ? size : size === 'small' ? 12 : 18
    return (
      <img
        src="/console/firecrawl.svg"
        alt="Firecrawl"
        style={{ height, objectFit: 'contain', display: 'inline-block' }}
        className={className}
      />
    )
  }
}

export const getFirecrawlIcon = (
  size: 'small' | 'large' | number = 'small'
): React.ReactNode => {
  return firecrawlProvider.getIcon('', size)
}
