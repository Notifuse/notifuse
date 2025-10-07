import { useState } from 'react'
import type { SetupConfig } from '../../types/setup'

interface EmailConfigStepProps {
  config: Partial<SetupConfig>
  onUpdate: (updates: Partial<SetupConfig>) => void
  onNext: () => void
  onBack: () => void
}

export default function EmailConfigStep({
  config,
  onUpdate,
  onNext,
  onBack
}: EmailConfigStepProps) {
  const [formData, setFormData] = useState({
    smtp_host: config.smtp_host || '',
    smtp_port: config.smtp_port || 587,
    smtp_username: config.smtp_username || '',
    smtp_password: config.smtp_password || '',
    smtp_from_email: config.smtp_from_email || '',
    smtp_from_name: config.smtp_from_name || 'Notifuse'
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [testing, setTesting] = useState(false)

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {}

    if (!formData.smtp_host) {
      newErrors.smtp_host = 'SMTP host is required'
    }

    if (!formData.smtp_port || formData.smtp_port < 1 || formData.smtp_port > 65535) {
      newErrors.smtp_port = 'Valid port number is required (1-65535)'
    }

    if (!formData.smtp_from_email) {
      newErrors.smtp_from_email = 'From email is required'
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.smtp_from_email)) {
      newErrors.smtp_from_email = 'Invalid email format'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleNext = () => {
    if (validate()) {
      onUpdate(formData)
      onNext()
    }
  }

  const handleTestConnection = async () => {
    if (!validate()) {
      return
    }

    setTesting(true)
    // Note: This is a placeholder. In a real implementation, you'd call an API endpoint to test the connection
    setTimeout(() => {
      setTesting(false)
      alert('Connection test is not implemented yet. Configuration will be validated during setup.')
    }, 1000)
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">Email Configuration</h2>
        <p className="mt-1 text-sm text-gray-600">Configure SMTP settings for sending emails</p>
      </div>

      <div className="space-y-4">
        <div>
          <label htmlFor="smtp_host" className="block text-sm font-medium text-gray-700 mb-1">
            SMTP Host <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            id="smtp_host"
            value={formData.smtp_host}
            onChange={(e) => setFormData({ ...formData, smtp_host: e.target.value })}
            className={`w-full px-3 py-2 border rounded-md ${
              errors.smtp_host ? 'border-red-300' : 'border-gray-300'
            }`}
            placeholder="smtp.example.com"
          />
          {errors.smtp_host && <p className="mt-1 text-sm text-red-600">{errors.smtp_host}</p>}
        </div>

        <div>
          <label htmlFor="smtp_port" className="block text-sm font-medium text-gray-700 mb-1">
            SMTP Port <span className="text-red-500">*</span>
          </label>
          <input
            type="number"
            id="smtp_port"
            value={formData.smtp_port}
            onChange={(e) =>
              setFormData({ ...formData, smtp_port: parseInt(e.target.value) || 587 })
            }
            className={`w-full px-3 py-2 border rounded-md ${
              errors.smtp_port ? 'border-red-300' : 'border-gray-300'
            }`}
            placeholder="587"
          />
          {errors.smtp_port && <p className="mt-1 text-sm text-red-600">{errors.smtp_port}</p>}
          <p className="mt-1 text-xs text-gray-500">
            Common ports: 587 (TLS), 465 (SSL), 25 (unencrypted)
          </p>
        </div>

        <div>
          <label htmlFor="smtp_username" className="block text-sm font-medium text-gray-700 mb-1">
            SMTP Username
          </label>
          <input
            type="text"
            id="smtp_username"
            value={formData.smtp_username}
            onChange={(e) => setFormData({ ...formData, smtp_username: e.target.value })}
            className="w-full px-3 py-2 border border-gray-300 rounded-md"
            placeholder="user@example.com"
          />
        </div>

        <div>
          <label htmlFor="smtp_password" className="block text-sm font-medium text-gray-700 mb-1">
            SMTP Password
          </label>
          <input
            type="password"
            id="smtp_password"
            value={formData.smtp_password}
            onChange={(e) => setFormData({ ...formData, smtp_password: e.target.value })}
            className="w-full px-3 py-2 border border-gray-300 rounded-md"
            placeholder="••••••••"
          />
        </div>

        <div>
          <label htmlFor="smtp_from_email" className="block text-sm font-medium text-gray-700 mb-1">
            From Email <span className="text-red-500">*</span>
          </label>
          <input
            type="email"
            id="smtp_from_email"
            value={formData.smtp_from_email}
            onChange={(e) => setFormData({ ...formData, smtp_from_email: e.target.value })}
            className={`w-full px-3 py-2 border rounded-md ${
              errors.smtp_from_email ? 'border-red-300' : 'border-gray-300'
            }`}
            placeholder="notifications@example.com"
          />
          {errors.smtp_from_email && (
            <p className="mt-1 text-sm text-red-600">{errors.smtp_from_email}</p>
          )}
        </div>

        <div>
          <label htmlFor="smtp_from_name" className="block text-sm font-medium text-gray-700 mb-1">
            From Name
          </label>
          <input
            type="text"
            id="smtp_from_name"
            value={formData.smtp_from_name}
            onChange={(e) => setFormData({ ...formData, smtp_from_name: e.target.value })}
            className="w-full px-3 py-2 border border-gray-300 rounded-md"
            placeholder="Notifuse"
          />
        </div>

        <div className="bg-blue-50 border border-blue-200 rounded-md p-4">
          <p className="text-sm text-blue-800">
            💡 <strong>Tip:</strong> You can configure multiple email providers later in workspace
            settings. This SMTP configuration will be used for system emails.
          </p>
        </div>
      </div>

      <div className="flex justify-between pt-5 border-t border-gray-200">
        <button
          type="button"
          onClick={onBack}
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
        >
          Back
        </button>
        <div className="flex space-x-3">
          <button
            type="button"
            onClick={handleTestConnection}
            disabled={testing}
            className="px-4 py-2 text-sm font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-md hover:bg-blue-100 disabled:opacity-50"
          >
            {testing ? 'Testing...' : 'Test Connection'}
          </button>
          <button
            type="button"
            onClick={handleNext}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 border border-transparent rounded-md hover:bg-blue-700"
          >
            Continue
          </button>
        </div>
      </div>
    </div>
  )
}
