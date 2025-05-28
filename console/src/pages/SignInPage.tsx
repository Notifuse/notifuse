import { Form, Input, Button, Card, App, Space } from 'antd'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useState, useEffect, useCallback, useRef } from 'react'
import { authService } from '../services/api/auth'
import { SignInRequest, VerifyCodeRequest } from '../services/api/types'
import { MainLayout } from '../layouts/MainLayout'

export function SignInPage() {
  const { signin } = useAuth()
  const navigate = useNavigate()
  const search = useSearch({ from: '/signin' })
  const [email, setEmail] = useState('')
  const [showCodeInput, setShowCodeInput] = useState(false)
  const [loading, setLoading] = useState(false)
  const [resendLoading, setResendLoading] = useState(false)
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const hasAutoSubmitted = useRef(false)

  const handleCodeSubmit = useCallback(
    async (values: { code: string }, emailToUse?: string) => {
      try {
        setLoading(true)
        const data: VerifyCodeRequest = {
          email: emailToUse || email,
          code: values.code
        }

        const response = await authService.verifyCode(data)
        const { token } = response
        // Use the existing signin function for now
        // This might need to be updated in AuthContext
        await signin(token)
        message.success('Successfully signed in')

        // Add a small delay to ensure auth state is updated before navigation
        setTimeout(() => {
          navigate({ to: '/' })
        }, 100)
      } catch (error) {
        message.error('Failed to verify code')
      } finally {
        setLoading(false)
      }
    },
    [email, signin, message, navigate]
  )

  const handleEmailSubmit = useCallback(
    async (values: SignInRequest) => {
      try {
        setLoading(true)
        const response = await authService.signIn(values)

        // Log code if present (for development)
        if (response.code && response.code !== '') {
          console.log('Magic code for development:', response.code)

          // Auto-submit the code in development
          setEmail(values.email)
          await handleCodeSubmit({ code: response.code }, values.email)
          return
        }

        setEmail(values.email)
        setShowCodeInput(true)
        message.success('Magic code sent to your email')
      } catch (error) {
        message.error('Failed to send magic code')
      } finally {
        setLoading(false)
      }
    },
    [handleCodeSubmit, message]
  )

  // Initialize email from URL parameter or demo mode
  useEffect(() => {
    // Prevent multiple auto-submissions
    if (hasAutoSubmitted.current) return

    let emailToUse = ''

    if (search.email) {
      // URL parameter takes priority
      emailToUse = search.email
    } else if ((window as any).demo === true) {
      // Demo mode fallback
      emailToUse = 'demo@notifuse.com'
    }

    if (emailToUse) {
      hasAutoSubmitted.current = true
      setEmail(emailToUse)
      form.setFieldsValue({ email: emailToUse })
      // Automatically submit the form if email is determined
      handleEmailSubmit({ email: emailToUse })
    }
  }, [search.email, form, handleEmailSubmit])

  const handleResendCode = async () => {
    try {
      setResendLoading(true)
      const response = await authService.signIn({ email })

      // Log code if present (for development)
      if (response.code) {
        console.log('⚡ Magic code for development:', response.code)

        // Auto-submit the code in development
        await handleCodeSubmit({ code: response.code }, email)
        return
      }

      message.success('New magic code sent to your email')
    } catch (error) {
      message.error('Failed to resend magic code')
    } finally {
      setResendLoading(false)
    }
  }

  return (
    <MainLayout>
      <div className="flex items-center justify-center h-[calc(100vh-48px)]">
        <Card title="Sign In" style={{ width: 400 }}>
          {!showCodeInput ? (
            <Form
              form={form}
              name="email"
              onFinish={handleEmailSubmit}
              layout="vertical"
              initialValues={{ email }}
            >
              <Form.Item
                label="Email"
                name="email"
                rules={[
                  { required: true, message: 'Please input your email!' },
                  { type: 'email', message: 'Please enter a valid email!' }
                ]}
              >
                <Input placeholder="Email" type="email" />
              </Form.Item>

              <Form.Item>
                <Button type="primary" htmlType="submit" block loading={loading}>
                  Send Magic Code
                </Button>
              </Form.Item>
            </Form>
          ) : (
            <>
              <p style={{ marginBottom: 24 }}>Enter the 6-digit code sent to {email}</p>
              <Form name="code" onFinish={handleCodeSubmit} layout="vertical">
                <Form.Item
                  name="code"
                  rules={[
                    { required: true, message: 'Please input the magic code!' },
                    {
                      pattern: /^\d{6}$/,
                      message: 'Please enter a valid 6-digit code!'
                    }
                  ]}
                >
                  <Input
                    placeholder="000000"
                    maxLength={6}
                    style={{ textAlign: 'center', letterSpacing: '0.5em' }}
                  />
                </Form.Item>

                <Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    Verify Code
                  </Button>
                </Form.Item>

                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Button
                    type="link"
                    onClick={() => setShowCodeInput(false)}
                    style={{ padding: 0 }}
                  >
                    Use a different email
                  </Button>
                  <Button
                    type="link"
                    onClick={handleResendCode}
                    loading={resendLoading}
                    style={{ padding: 0 }}
                  >
                    Resend code
                  </Button>
                </Space>
              </Form>
            </>
          )}
        </Card>
      </div>
    </MainLayout>
  )
}
