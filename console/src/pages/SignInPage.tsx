import { Form, Input, Button, Card, App, Space, Divider, Alert } from 'antd'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useState, useEffect, useCallback, useRef } from 'react'
import { authService } from '../services/api/auth'
import { ApiUnreachableError } from '../services/api/errors'
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
  const [unreachable, setUnreachable] = useState<{ endpoint: string; origin: string } | null>(
    null
  )

  // The console is served by the server it talks to, so when this page loaded but a request
  // to API_ENDPOINT got no answer, API_ENDPOINT names some other host — typically the old
  // domain after an instance moved. The browser only says "Failed to fetch", and a toast is
  // gone before anyone can copy a URL out of it, so that case gets a lasting alert naming
  // both addresses. A same-origin failure is the server going away and keeps the toast.
  const showRequestError = useCallback(
    (error: unknown, fallback: string) => {
      if (error instanceof ApiUnreachableError) {
        let endpointOrigin = ''
        try {
          endpointOrigin = new URL(error.endpoint).origin
        } catch {
          // Not an absolute URL: it cannot point anywhere but this page's origin.
        }
        if (endpointOrigin && endpointOrigin !== window.location.origin) {
          setUnreachable({ endpoint: error.endpoint, origin: window.location.origin })
          return
        }
      }
      message.error(error instanceof Error ? error.message : fallback)
    },
    [message]
  )

  const handleCodeSubmit = useCallback(
    async (values: { code: string }, emailToUse?: string) => {
      try {
        setLoading(true)
        setUnreachable(null)
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
        showRequestError(error, t`Failed to verify code`)
      } finally {
        setLoading(false)
      }
    },
    [email, signin, message, navigate, t, showRequestError]
  )

  const handleEmailSubmit = useCallback(
    async (values: SignInRequest) => {
      try {
        setLoading(true)
        setUnreachable(null)
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
        showRequestError(error, t`Failed to send magic code`)
      } finally {
        setLoading(false)
      }
    },
    [handleCodeSubmit, message, t, showRequestError]
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
        // An invitee whose identity provider vouches for their address gets an account;
        // one whose provider does not is refused here and has to use the emailed link.
        case 'not_provisioned':
          return t`No Notifuse account is linked to that identity. If you were invited, open the link in your invitation email; otherwise ask an administrator to invite you.`
        case 'domain_not_allowed':
          return t`Single sign-on cannot create an account for your email domain. If you were invited, open the link in your invitation email; otherwise ask an administrator to invite you.`
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
      setUnreachable(null)
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
      showRequestError(error, t`Failed to resend magic code`)
    } finally {
      setResendLoading(false)
    }
  }

  const unreachableEndpoint = unreachable?.endpoint ?? ''
  const pageOrigin = unreachable?.origin ?? ''

  return (
    <MainLayout>
      <div className="flex items-center justify-center h-[calc(100vh-48px)]">
        <Card title={t`Sign In`} style={{ width: 400 }}>
          {unreachable && (
            <Alert
              type="error"
              showIcon
              style={{ marginBottom: 24 }}
              message={t`Cannot reach the configured API endpoint`}
              description={
                <>
                  <p style={{ marginBottom: 8, overflowWrap: 'anywhere' }}>
                    {t`This page is served from ${pageOrigin}, but this instance sends its requests to ${unreachableEndpoint}, which did not answer. If Notifuse has moved to ${pageOrigin}, set the API_ENDPOINT environment variable to that address and restart the server.`}
                  </p>
                  <a
                    href="https://docs.notifuse.com/self-hosting/changing-domain"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {t`Changing your domain`}
                  </a>
                </>
              }
            />
          )}
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
                    inputMode="numeric"
                    autoComplete="one-time-code"
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
