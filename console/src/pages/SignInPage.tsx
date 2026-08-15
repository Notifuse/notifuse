import { Form, Input, Button, Card, App, Space, Divider } from 'antd'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useState, useEffect, useCallback, useRef } from 'react'
import { authService } from '../services/api/auth'
import { SignInRequest, VerifyCodeRequest } from '../services/api/types'
import { MainLayout } from '../layouts/MainLayout'
import { useLingui } from '@lingui/react/macro'

export function SignInPage() {
  const { t } = useLingui()
  const { signin } = useAuth()
  const navigate = useNavigate()
  const search = useSearch({ from: '/console/signin' })
  const [email, setEmail] = useState('')
  const [showCodeInput, setShowCodeInput] = useState(false)
  const [loading, setLoading] = useState(false)
  const [resendLoading, setResendLoading] = useState(false)
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const hasAutoSubmitted = useRef(false)
  const hasExchangedOidc = useRef(false)

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
        message.success(t`Successfully signed in`)

        // Add a small delay to ensure auth state is updated before navigation
        setTimeout(() => {
          navigate({ to: '/console' })
        }, 100)
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : t`Failed to verify code`
        message.error(errorMessage)
      } finally {
        setLoading(false)
      }
    },
    [email, signin, message, navigate, t]
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
        message.success(t`Magic code sent to your email`)
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : t`Failed to send magic code`
        message.error(errorMessage)
      } finally {
        setLoading(false)
      }
    },
    [handleCodeSubmit, message, t]
  )

  // mapOidcError translates a backend OIDC error code into a localized message. The
  // code set is a fixed enum — never a raw IdP error — produced by mapCallbackError in
  // internal/http/oidc_handler.go. A new case there needs a new case here, or the user
  // gets the generic fallback.
  //
  // It has to live inside the component so `t` resolves to the useLingui() binding —
  // nesting it in a callback is fine, shadowing it is not. Declared at module level
  // taking `t` as a parameter, the macro cannot rewrite these tagged templates, and what
  // that parameter receives at runtime is i18n._, which answers "" to a tagged-template
  // call. The toast then opened with no message in it at all, in every language.
  const mapOidcError = useCallback(
    (code: string): string => {
      switch (code) {
        case 'not_provisioned':
          return t`No Notifuse account is linked to that identity. Ask an administrator to invite you first.`
        case 'email_unverified':
          return t`Your identity provider has not verified your email address.`
        case 'link_conflict':
          return t`This account is already linked to a different single sign-on identity.`
        case 'provider_unavailable':
          return t`Single sign-on is temporarily unavailable. Please try a magic code.`
        case 'rate_limited':
          return t`Too many sign-in attempts. Please wait a moment and try again.`
        default:
          return t`Single sign-on failed. Please try again or use a magic code.`
      }
    },
    [t]
  )

  const handleOidcExchange = useCallback(
    async (code: string) => {
      try {
        setLoading(true)
        const { token } = await authService.oidcExchange(code) // code from URL fragment
        await signin(token) // UNCHANGED AuthContext path -> localStorage('auth_token')
        message.success(t`Successfully signed in`)
        setTimeout(() => {
          navigate({ to: '/console' })
        }, 100)
      } catch {
        message.error(t`Single sign-on failed. Please try again or use a magic code.`)
      } finally {
        setLoading(false)
      }
    },
    [signin, message, navigate, t]
  )

  // OIDC callback handoff: backend /callback 302s to
  //   /console/signin#oidc_code=<code>   (success)  or
  //   /console/signin?oidc_error=<code>  (failure).
  // The success code rides the URL FRAGMENT (never the query string, never logged,
  // stripped from Referer). Read it, IMMEDIATELY replaceState it away so a
  // refresh/back cannot replay it, then POST it in the exchange body.
  useEffect(() => {
    if (hasExchangedOidc.current) return

    if (search.oidc_error) {
      hasExchangedOidc.current = true
      message.error(mapOidcError(search.oidc_error))
      navigate({ to: '/console/signin', search: {}, replace: true })
      return
    }

    const hash = window.location.hash // e.g. "#oidc_code=AbC123..."
    const m = /[#&]oidc_code=([^&]+)/.exec(hash)
    if (m) {
      hasExchangedOidc.current = true
      const code = decodeURIComponent(m[1])
      // Strip the fragment BEFORE the network round-trip (single-use + replaceState).
      window.history.replaceState(null, '', window.location.pathname + window.location.search)
      void handleOidcExchange(code)
    }
  }, [search.oidc_error, navigate, message, mapOidcError, handleOidcExchange])

  // Initialize email from URL parameter or demo mode
  useEffect(() => {
    // Prevent multiple auto-submissions
    if (hasAutoSubmitted.current) return

    let emailToUse = ''

    if (search.email) {
      // URL parameter takes priority
      emailToUse = search.email
    } else if ((window as unknown as Record<string, unknown>).demo === true) {
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

      message.success(t`New magic code sent to your email`)
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : t`Failed to resend magic code`
      message.error(errorMessage)
    } finally {
      setResendLoading(false)
    }
  }

  return (
    <MainLayout>
      <div className="flex items-center justify-center h-[calc(100vh-48px)]">
        <Card title={t`Sign In`} style={{ width: 400 }}>
          {!showCodeInput ? (
            <Form
              form={form}
              name="email"
              onFinish={handleEmailSubmit}
              layout="vertical"
              initialValues={{ email }}
            >
              <Form.Item
                label={t`Email`}
                name="email"
                rules={[
                  { required: true, message: t`Please input your email!` },
                  { type: 'email', message: t`Please enter a valid email!` }
                ]}
              >
                <Input placeholder={t`Email`} type="email" />
              </Form.Item>

              <Form.Item>
                <Button type="primary" htmlType="submit" block loading={loading}>
                  {t`Send Magic Code`}
                </Button>
              </Form.Item>

              {window.OIDC_ENABLED && (
                <>
                  <Divider plain>{t`or`}</Divider>
                  <Button
                    block
                    onClick={() => {
                      const base =
                        window.API_ENDPOINT?.trim().replace(/\/+$/, '') || window.location.origin
                      // Full-page navigation: /api/user.oidc.start sets the AEAD flow
                      // cookie and 302s to the IdP, so it must be a real navigation.
                      window.location.assign(`${base}/api/user.oidc.start`)
                    }}
                  >
                    {window.OIDC_BUTTON_LABEL || t`Sign in with SSO`}
                  </Button>
                </>
              )}
            </Form>
          ) : (
            <>
              <p style={{ marginBottom: 24 }}>{t`Enter the 6-digit code sent to ${email}`}</p>
              <Form name="code" onFinish={handleCodeSubmit} layout="vertical">
                <Form.Item
                  name="code"
                  rules={[
                    { required: true, message: t`Please input the magic code!` },
                    {
                      pattern: /^\d{6}$/,
                      message: t`Please enter a valid 6-digit code!`
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
                    {t`Verify Code`}
                  </Button>
                </Form.Item>

                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Button
                    type="link"
                    onClick={() => setShowCodeInput(false)}
                    style={{ padding: 0 }}
                  >
                    {t`Use a different email`}
                  </Button>
                  <Button
                    type="link"
                    onClick={handleResendCode}
                    loading={resendLoading}
                    style={{ padding: 0 }}
                  >
                    {t`Resend code`}
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
