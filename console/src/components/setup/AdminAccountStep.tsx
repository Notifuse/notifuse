import { useState } from 'react'
import { setupApi } from '../../services/api/setup'
import type { SetupConfig } from '../../types/setup'

interface AdminAccountStepProps {
  config: Partial<SetupConfig>
  onUpdate: (updates: Partial<SetupConfig>) => void
  onComplete: (token: string) => void
  onBack: () => void
}

export default function AdminAccountStep({
  config,
  onUpdate,
  onComplete,
  onBack
}: AdminAccountStepProps) {
  const [formData, setFormData] = useState({
    root_email: config.root_email || '',
    api_endpoint: config.api_endpoint || window.location.origin
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [confirmed, setConfirmed] = useState(false)

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {}

    if (!formData.root_email) {
      newErrors.root_email = 'Admin email is required'
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.root_email)) {
      newErrors.root_email = 'Invalid email format'
    }

    if (!formData.api_endpoint) {
      newErrors.api_endpoint = 'API endpoint is required'
    } else {
      try {
        new URL(formData.api_endpoint)
      } catch {
        newErrors.api_endpoint = 'Invalid URL format'
      }
    }

    if (!confirmed) {
      newErrors.confirmed = 'Please confirm that you have saved your PASETO keys'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleComplete = async () => {
    if (!validate()) {
      return
    }

    setLoading(true)

    try {
      const setupConfig: SetupConfig = {
        root_email: formData.root_email,
        api_endpoint: formData.api_endpoint,
        generate_paseto_keys: config.generate_paseto_keys ?? true,
        paseto_public_key: config.paseto_public_key,
        paseto_private_key: config.paseto_private_key,
        smtp_host: config.smtp_host!,
        smtp_port: config.smtp_port!,
        smtp_username: config.smtp_username || '',
        smtp_password: config.smtp_password || '',
        smtp_from_email: config.smtp_from_email!,
        smtp_from_name: config.smtp_from_name || 'Notifuse'
      }

      const response = await setupApi.initialize(setupConfig)

      // Save token to localStorage for immediate login
      localStorage.setItem('auth_token', response.token)

      onComplete(response.token)
    } catch (err) {
      setErrors({
        submit: err instanceof Error ? err.message : 'Failed to complete setup'
      })
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">Admin Account</h2>
        <p className="mt-1 text-sm text-gray-600">Set up the root administrator account</p>
      </div>

      {errors.submit && (
        <div className="bg-red-50 border border-red-200 rounded-md p-4">
          <p className="text-sm text-red-800">{errors.submit}</p>
        </div>
      )}

      <div className="space-y-4">
        <div>
          <label htmlFor="root_email" className="block text-sm font-medium text-gray-700 mb-1">
            Admin Email <span className="text-red-500">*</span>
          </label>
          <input
            type="email"
            id="root_email"
            value={formData.root_email}
            onChange={(e) => setFormData({ ...formData, root_email: e.target.value })}
            className={`w-full px-3 py-2 border rounded-md ${
              errors.root_email ? 'border-red-300' : 'border-gray-300'
            }`}
            placeholder="admin@example.com"
          />
          {errors.root_email && <p className="mt-1 text-sm text-red-600">{errors.root_email}</p>}
          <p className="mt-1 text-xs text-gray-500">
            This email will be used for the root administrator account
          </p>
        </div>

        <div>
          <label htmlFor="api_endpoint" className="block text-sm font-medium text-gray-700 mb-1">
            API Endpoint <span className="text-red-500">*</span>
          </label>
          <input
            type="url"
            id="api_endpoint"
            value={formData.api_endpoint}
            onChange={(e) => setFormData({ ...formData, api_endpoint: e.target.value })}
            className={`w-full px-3 py-2 border rounded-md ${
              errors.api_endpoint ? 'border-red-300' : 'border-gray-300'
            }`}
            placeholder="https://notifuse.example.com"
          />
          {errors.api_endpoint && (
            <p className="mt-1 text-sm text-red-600">{errors.api_endpoint}</p>
          )}
          <p className="mt-1 text-xs text-gray-500">
            Public URL where this Notifuse instance is accessible. Used for webhooks and
            documentation examples.
          </p>
        </div>

        <div className="bg-yellow-50 border border-yellow-200 rounded-md p-4">
          <div className="flex items-start">
            <input
              type="checkbox"
              id="confirmed"
              checked={confirmed}
              onChange={(e) => setConfirmed(e.target.checked)}
              className="mt-1 mr-3 h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
            />
            <label htmlFor="confirmed" className="text-sm text-yellow-900">
              <strong>I confirm that I have securely saved the PASETO keys.</strong> These keys are
              required for system authentication and cannot be recovered if lost.
            </label>
          </div>
          {errors.confirmed && <p className="mt-2 text-sm text-red-600 ml-7">{errors.confirmed}</p>}
        </div>

        <div className="bg-blue-50 border border-blue-200 rounded-md p-4">
          <h3 className="text-sm font-medium text-blue-900 mb-2">What happens next?</h3>
          <ul className="text-sm text-blue-800 space-y-1 list-disc list-inside">
            <li>Your configuration will be saved to the database</li>
            <li>The root administrator account will be created</li>
            <li>You'll be automatically logged in</li>
            <li>You can create workspaces and start using Notifuse</li>
          </ul>
        </div>
      </div>

      <div className="flex justify-between pt-5 border-t border-gray-200">
        <button
          type="button"
          onClick={onBack}
          disabled={loading}
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
        >
          Back
        </button>
        <button
          type="button"
          onClick={handleComplete}
          disabled={loading}
          className="px-6 py-2 text-sm font-medium text-white bg-green-600 border border-transparent rounded-md hover:bg-green-700 disabled:opacity-50"
        >
          {loading ? 'Completing Setup...' : 'Complete Setup'}
        </button>
      </div>
    </div>
  )
}
