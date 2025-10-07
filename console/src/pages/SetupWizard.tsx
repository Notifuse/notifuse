import { useState } from 'react'
import WelcomeStep from '../components/setup/WelcomeStep'
import PasetoKeysStep from '../components/setup/PasetoKeysStep'
import EmailConfigStep from '../components/setup/EmailConfigStep'
import AdminAccountStep from '../components/setup/AdminAccountStep'
import CompleteStep from '../components/setup/CompleteStep'
import type { SetupConfig } from '../types/setup'

type SetupStep = 'welcome' | 'keys' | 'email' | 'admin' | 'complete'

export default function SetupWizard() {
  const [currentStep, setCurrentStep] = useState<SetupStep>('welcome')
  const [config, setConfig] = useState<Partial<SetupConfig>>({
    generate_paseto_keys: true,
    smtp_port: 587,
    smtp_from_name: 'Notifuse'
  })
  const [authToken, setAuthToken] = useState<string>('')

  const nextStep = () => {
    const steps: SetupStep[] = ['welcome', 'keys', 'email', 'admin', 'complete']
    const currentIndex = steps.indexOf(currentStep)
    if (currentIndex < steps.length - 1) {
      setCurrentStep(steps[currentIndex + 1])
    }
  }

  const prevStep = () => {
    const steps: SetupStep[] = ['welcome', 'keys', 'email', 'admin', 'complete']
    const currentIndex = steps.indexOf(currentStep)
    if (currentIndex > 0) {
      setCurrentStep(steps[currentIndex - 1])
    }
  }

  const updateConfig = (updates: Partial<SetupConfig>) => {
    setConfig((prev) => ({ ...prev, ...updates }))
  }

  const handleComplete = (token: string) => {
    setAuthToken(token)
    setCurrentStep('complete')
  }

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8">
      <div className="sm:mx-auto sm:w-full sm:max-w-2xl">
        <div className="bg-white py-8 px-4 shadow sm:rounded-lg sm:px-10">
          {currentStep === 'welcome' && <WelcomeStep onNext={nextStep} />}
          {currentStep === 'keys' && (
            <PasetoKeysStep
              config={config}
              onUpdate={updateConfig}
              onNext={nextStep}
              onBack={prevStep}
            />
          )}
          {currentStep === 'email' && (
            <EmailConfigStep
              config={config}
              onUpdate={updateConfig}
              onNext={nextStep}
              onBack={prevStep}
            />
          )}
          {currentStep === 'admin' && (
            <AdminAccountStep
              config={config}
              onUpdate={updateConfig}
              onComplete={handleComplete}
              onBack={prevStep}
            />
          )}
          {currentStep === 'complete' && <CompleteStep token={authToken} />}
        </div>

        {/* Progress indicator */}
        {currentStep !== 'welcome' && currentStep !== 'complete' && (
          <div className="mt-8 flex justify-center">
            <div className="flex space-x-2">
              {['keys', 'email', 'admin'].map((step, index) => (
                <div
                  key={step}
                  className={`w-3 h-3 rounded-full ${
                    currentStep === step
                      ? 'bg-blue-600'
                      : ['keys', 'email', 'admin'].indexOf(currentStep) > index
                        ? 'bg-blue-400'
                        : 'bg-gray-300'
                  }`}
                />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
