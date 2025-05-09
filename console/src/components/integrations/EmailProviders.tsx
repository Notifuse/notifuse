import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faEnvelope } from '@fortawesome/free-solid-svg-icons'
import { IntegrationType, EmailProviderKind } from '../../services/api/types'
import React from 'react'

export interface ProviderInfo {
  type: IntegrationType
  kind: EmailProviderKind
  name: string
  getIcon: (className?: string, size?: 'small' | 'large' | number) => React.ReactNode
}

export const getProviderName = (kind: EmailProviderKind): string => {
  switch (kind) {
    case 'smtp':
      return 'SMTP'
    case 'ses':
      return 'Amazon SES'
    case 'sparkpost':
      return 'SparkPost'
    case 'postmark':
      return 'Postmark'
    case 'mailgun':
      return 'Mailgun'
    case 'mailjet':
      return 'Mailjet'
    default:
      return kind
  }
}

export const emailProviders: ProviderInfo[] = [
  {
    type: 'email',
    kind: 'smtp',
    name: 'SMTP',
    getIcon: (className = '', size = 'small') => (
      <FontAwesomeIcon
        icon={faEnvelope}
        className={`${size === 'small' ? 'w-5 h-5' : 'w-16'} ${className}`.trim()}
      />
    )
  },
  {
    type: 'email',
    kind: 'ses',
    name: 'Amazon SES',
    getIcon: (className = '', size = 'small') => (
      <img
        src="/amazonses.png"
        alt="Amazon SES"
        className={`${size === 'small' ? 'h-5 w-5 object-contain inline-block' : 'h-8 w-16 object-contain inline-block'} ${className}`.trim()}
      />
    )
  },
  {
    type: 'email',
    kind: 'sparkpost',
    name: 'SparkPost',
    getIcon: (className = '', size = 'small') => (
      <img
        src="/sparkpost.png"
        alt="SparkPost"
        className={`${size === 'small' ? 'h-5 w-5 object-contain inline-block' : 'h-8 w-16 object-contain inline-block'} ${className}`.trim()}
      />
    )
  },
  {
    type: 'email',
    kind: 'postmark',
    name: 'Postmark',
    getIcon: (className = '', size = 'small') => (
      <img
        src="/postmark.png"
        alt="Postmark"
        className={`${size === 'small' ? 'h-5 w-5 object-contain inline-block' : 'h-8 w-16 object-contain inline-block'} ${className}`.trim()}
      />
    )
  },
  {
    type: 'email',
    kind: 'mailgun',
    name: 'Mailgun',
    getIcon: (className = '', size = 'small') => (
      <img
        src="/mailgun.png"
        alt="Mailgun"
        className={`${size === 'small' ? 'h-5 w-5 object-contain inline-block' : 'h-8 w-16 object-contain inline-block'} ${className}`.trim()}
      />
    )
  },
  {
    type: 'email',
    kind: 'mailjet',
    name: 'Mailjet',
    getIcon: (className = '', size = 'small') => (
      <img
        src="/mailjet.png"
        alt="Mailjet"
        className={`${size === 'small' ? 'h-5 w-5 object-contain inline-block' : 'h-8 w-16 object-contain inline-block'} ${className}`.trim()}
      />
    )
  }
  // Future integration types can be added here
]
