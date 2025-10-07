import { useState, useEffect } from 'react'
import { setupApi } from '../../services/api/setup'
import type { SetupConfig } from '../../types/setup'

interface PasetoKeysStepProps {
  config: Partial<SetupConfig>
  onUpdate: (updates: Partial<SetupConfig>) => void
  onNext: () => void
  onBack: () => void
}

export default function PasetoKeysStep({ config, onUpdate, onNext, onBack }: PasetoKeysStepProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')
  const [generateMode, setGenerateMode] = useState(config.generate_paseto_keys ?? true)
  const [keys, setKeys] = useState({
    publicKey: config.paseto_public_key || '',
    privateKey: config.paseto_private_key || ''
  })
  const [copied, setCopied] = useState({ public: false, private: false })

  useEffect(() => {
    if (generateMode && !keys.publicKey && !keys.privateKey) {
      generateKeys()
    }
  }, [])

  const generateKeys = async () => {
    setLoading(true)
    setError('')
    try {
      const result = await setupApi.generatePasetoKeys()
      setKeys({
        publicKey: result.public_key,
        privateKey: result.private_key
      })
      onUpdate({
        paseto_public_key: result.public_key,
        paseto_private_key: result.private_key,
        generate_paseto_keys: true
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate keys')
    } finally {
      setLoading(false)
    }
  }

  const handleCopy = async (type: 'public' | 'private', value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied({ ...copied, [type]: true })
      setTimeout(() => setCopied({ ...copied, [type]: false }), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  const handleDownload = () => {
    const content = `# Notifuse PASETO Keys
# Generated: ${new Date().toISOString()}
# IMPORTANT: Save these keys securely. You'll need them for deployment.

PASETO_PUBLIC_KEY=${keys.publicKey}
PASETO_PRIVATE_KEY=${keys.privateKey}
`
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'notifuse-paseto-keys.txt'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  const handleNext = () => {
    if (generateMode) {
      if (!keys.publicKey || !keys.privateKey) {
        setError('Please generate PASETO keys before continuing')
        return
      }
      onUpdate({
        paseto_public_key: keys.publicKey,
        paseto_private_key: keys.privateKey,
        generate_paseto_keys: true
      })
    } else {
      if (!keys.publicKey || !keys.privateKey) {
        setError('Please provide both PASETO keys')
        return
      }
      onUpdate({
        paseto_public_key: keys.publicKey,
        paseto_private_key: keys.privateKey,
        generate_paseto_keys: false
      })
    }
    onNext()
  }

  const handleModeChange = (mode: boolean) => {
    setGenerateMode(mode)
    setError('')
    if (mode && !keys.publicKey && !keys.privateKey) {
      generateKeys()
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">PASETO Keys</h2>
        <p className="mt-1 text-sm text-gray-600">
          Secure authentication keys for your Notifuse instance
        </p>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-md p-4">
          <p className="text-sm text-red-800">{error}</p>
        </div>
      )}

      <div className="space-y-4">
        <div className="flex space-x-4">
          <button
            type="button"
            onClick={() => handleModeChange(true)}
            className={`flex-1 py-3 px-4 border rounded-md text-sm font-medium ${
              generateMode
                ? 'border-blue-500 bg-blue-50 text-blue-700'
                : 'border-gray-300 bg-white text-gray-700 hover:bg-gray-50'
            }`}
          >
            Generate New Keys (Recommended)
          </button>
          <button
            type="button"
            onClick={() => handleModeChange(false)}
            className={`flex-1 py-3 px-4 border rounded-md text-sm font-medium ${
              !generateMode
                ? 'border-blue-500 bg-blue-50 text-blue-700'
                : 'border-gray-300 bg-white text-gray-700 hover:bg-gray-50'
            }`}
          >
            Use Existing Keys
          </button>
        </div>

        {generateMode ? (
          <div className="space-y-4">
            <div className="bg-yellow-50 border border-yellow-200 rounded-md p-4">
              <p className="text-sm text-yellow-800">
                <strong>Important:</strong> Save these keys securely. They are required for system
                authentication and cannot be recovered if lost.
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Public Key</label>
              <div className="relative">
                <textarea
                  readOnly
                  value={keys.publicKey}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-50 font-mono text-xs"
                  rows={3}
                />
                <button
                  type="button"
                  onClick={() => handleCopy('public', keys.publicKey)}
                  className="absolute top-2 right-2 px-3 py-1 text-xs bg-white border border-gray-300 rounded hover:bg-gray-50"
                >
                  {copied.public ? 'Copied!' : 'Copy'}
                </button>
              </div>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Private Key</label>
              <div className="relative">
                <textarea
                  readOnly
                  value={keys.privateKey}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-50 font-mono text-xs"
                  rows={3}
                />
                <button
                  type="button"
                  onClick={() => handleCopy('private', keys.privateKey)}
                  className="absolute top-2 right-2 px-3 py-1 text-xs bg-white border border-gray-300 rounded hover:bg-gray-50"
                >
                  {copied.private ? 'Copied!' : 'Copy'}
                </button>
              </div>
            </div>

            <div className="flex space-x-3">
              <button
                type="button"
                onClick={generateKeys}
                disabled={loading}
                className="px-4 py-2 text-sm font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-md hover:bg-blue-100 disabled:opacity-50"
              >
                {loading ? 'Generating...' : 'Regenerate Keys'}
              </button>
              <button
                type="button"
                onClick={handleDownload}
                className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
              >
                Download as File
              </button>
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Public Key <span className="text-red-500">*</span>
              </label>
              <textarea
                value={keys.publicKey}
                onChange={(e) => setKeys({ ...keys, publicKey: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-xs"
                rows={3}
                placeholder="Paste your base64-encoded PASETO public key here..."
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Private Key <span className="text-red-500">*</span>
              </label>
              <textarea
                value={keys.privateKey}
                onChange={(e) => setKeys({ ...keys, privateKey: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-xs"
                rows={3}
                placeholder="Paste your base64-encoded PASETO private key here..."
              />
            </div>
          </div>
        )}
      </div>

      <div className="flex justify-between pt-5 border-t border-gray-200">
        <button
          type="button"
          onClick={onBack}
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
        >
          Back
        </button>
        <button
          type="button"
          onClick={handleNext}
          disabled={loading}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 border border-transparent rounded-md hover:bg-blue-700 disabled:opacity-50"
        >
          Continue
        </button>
      </div>
    </div>
  )
}
