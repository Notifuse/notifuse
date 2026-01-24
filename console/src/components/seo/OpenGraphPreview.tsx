import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCircleCheck } from '@fortawesome/free-regular-svg-icons'
import type { SEOSettings } from '../../services/api/workspace'

interface OpenGraphPreviewProps {
  webPublicationSettings: SEOSettings
  broadcastName: string
  customEndpointUrl?: string
  width?: number
  fallbackIcon?: React.ReactNode
  defaultDescription?: string
}

export const OpenGraphPreview: React.FC<OpenGraphPreviewProps> = ({
  webPublicationSettings,
  broadcastName,
  customEndpointUrl,
  width = 350,
  fallbackIcon = <FontAwesomeIcon icon={faCircleCheck} className="text-blue-300" size="2x" />,
  defaultDescription
}) => {
  const { t } = useLingui()
  const resolvedDefaultDescription =
    defaultDescription ?? t`Read the latest post from this broadcast.`
  return (
    <div
      className="border border-gray-200 rounded-lg overflow-hidden bg-white flex"
      style={{ width }}
    >
      {/* OG Image - Square on the left */}
      {webPublicationSettings.og_image ? (
        <div className="w-24 h-24 flex-shrink-0 bg-gray-100 overflow-hidden">
          <img
            src={webPublicationSettings.og_image}
            alt={webPublicationSettings.og_title || broadcastName}
            className="w-full h-full object-cover"
          />
        </div>
      ) : (
        <div className="w-24 h-24 flex-shrink-0 bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
          {fallbackIcon}
        </div>
      )}

      {/* OG Content - Text on the right */}
      <div className="flex-1 p-3 flex flex-col justify-center min-w-0">
        {customEndpointUrl && (
          <div className="text-xs text-gray-500 mb-1 truncate">
            {customEndpointUrl.replace(/^https?:\/\//, '')}
          </div>
        )}
        <div className="text-sm font-semibold text-gray-900 mb-1 line-clamp-2">
          {webPublicationSettings.og_title || webPublicationSettings.meta_title || broadcastName}
        </div>
        <div className="text-xs text-gray-600 line-clamp-2">
          {webPublicationSettings.og_description ||
            webPublicationSettings.meta_description ||
            resolvedDefaultDescription}
        </div>
      </div>
    </div>
  )
}
