import React from 'react'
import { IntegrationType } from '../../services/api/types'

/**
 * Zapier's own wordmark, as it is served on zapier.com: the orange leading dash plus "zapier".
 *
 * It is a wide mark (roughly 3.7:1), not a square one, which is why the icon below is sized by
 * height and left to find its own width — the same shape as Firecrawl's. Give it a fixed width
 * and it letterboxes.
 */
const ZAPIER_ICON_SRC = '/console/zapier.svg'

export interface ZapierProviderInfo {
  type: IntegrationType
  name: string
  getIcon: (className?: string, size?: 'small' | 'large' | number) => React.ReactNode
}

export const zapierProvider: ZapierProviderInfo = {
  type: 'zapier',
  name: 'Zapier',
  getIcon: (className = '', size: 'small' | 'large' | number = 'small') => {
    const height = typeof size === 'number' ? size : size === 'small' ? 12 : 18
    return (
      <img
        src={ZAPIER_ICON_SRC}
        alt="Zapier"
        style={{ height, objectFit: 'contain', display: 'inline-block' }}
        className={className}
      />
    )
  }
}

export const getZapierIcon = (size: 'small' | 'large' | number = 'small'): React.ReactNode => {
  return zapierProvider.getIcon('', size)
}
