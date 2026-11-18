import { useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ConfigProvider, Form, Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { setupApi } from '../services/api/setup'
import { apiBaseUrl } from '../services/api/client'
import type { SetupStatus } from '../types/setup'
import { getBrowserTimezone } from '../lib/timezoneNormalizer'
import { AdministratorPanel } from '../components/setup/AdministratorPanel'
import { SmtpPanel } from '../components/setup/SmtpPanel'
import { UpdatesPanel } from '../components/setup/UpdatesPanel'
import { SsoPanel } from '../components/setup/SsoPanel'
import { BridgePanel } from '../components/setup/BridgePanel'
import { SetupRail } from '../components/setup/SetupRail'
import { initialSetupValues, usePanelSummaries, type PanelSummary } from '../components/setup/setupHooks'
import { SetupRestartView, type RestartPhase } from '../components/setup/SetupRestartView'
import { buildSetupConfig } from '../components/setup/buildSetupConfig'
import { handOffSignInEmail } from '../components/setup/signInHandoff'
import {
  isFieldLocked,
  setupPanelDomId,
  splitList,
  type SetupFormValues,
  type SetupKey,
  type SetupPanelId
} from '../components/setup/setupFields'

const EMPTY_STATUS: SetupStatus = {
  is_installed: false,
  smtp_configured: false,
  api_endpoint_configured: false,
  root_email_configured: false,
  smtp_bridge_configured: false,
  oidc_configured: false
}

// Fields inside a panel keep two thirds of antd's margin: the panel's padding closes each one.
const FORM_THEME = { components: { Form: { itemMarginBottom: 16 } } }

// Only without a boot_id to compare (a server older than the field): a pause before the
// first poll, so that the process shutting down is less likely to answer it.
const RESTART_PAUSE_MS = 2000
const RESTART_POLL_MS = 1000
const RESTART_ATTEMPTS = 60

/** Fails silently: the newsletter must never block the setup. */
async function subscribeToNewsletter(email: string, values: SetupFormValues) {
  try {
    const contact: Record<string, unknown> = { email }
    try {
      const timezone = getBrowserTimezone()
      if (timezone) contact.timezone = timezone
    } catch {
      // No timezone then.
    }
    const endpoint = values.api_endpoint || window.API_ENDPOINT
    if (endpoint) contact.custom_string_1 = endpoint
    contact.custom_string_2 = values.check_for_updates === false ? 'false' : 'true'
    contact.custom_string_3 = values.telemetry_enabled === false ? 'false' : 'true'
    await fetch('https://email.notifuse.com/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace_id: 'notifuse', contact, list_ids: ['newsletter'] })
    })
  } catch {
    // Fail silently.
  }
}

/** The answer comes from another process than the one read on opening the page. */
async function isRestarted(response: Response, previousBootId: string | undefined): Promise<boolean> {
  if (!previousBootId) return true
  try {
    const body = (await response.json()) as Partial<SetupStatus>
    // A server without boot_id cannot say; take its answer as before.
    return !body.boot_id || body.boot_id !== previousBootId
  } catch {
    return false
  }
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

export default function SetupWizard() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [form] = Form.useForm<SetupFormValues>()

  const [status, setStatus] = useState<SetupStatus>(EMPTY_STATUS)
  const [statusLoading, setStatusLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()
  const [restart, setRestart] = useState<{ phase: RestartPhase; elapsed: number; rootEmail?: string }>()
  const unmounted = useRef(false)

  const values = (Form.useWatch((all) => all, { form, preserve: true }) ?? initialSetupValues()) as SetupFormValues
  // The fields the form's rules refuse, read from the form at each render (a field that unmounts
  // drops its errors with it); onFieldsChange only asks for that render when a field's errors change.
  const [, setFieldsVersion] = useState(0)
  const fieldErrors = [
    ...new Set(
      form
        .getFieldsError()
        .filter((field) => field.errors.length > 0)
        .map((field) => field.name[0] as SetupKey)
    )
  ]
  // The last SMTP test: the panel states say so too
  const [smtpTestFailed, setSmtpTestFailed] = useState(false)
  const summaries = usePanelSummaries(status, values, { fieldErrors, smtpTestFailed })
  const stateOf = (id: SetupPanelId) => summaries.find((summary) => summary.id === id)!

  useEffect(() => {
    unmounted.current = false
    const fetchStatus = async () => {
      try {
        const current = await setupApi.getStatus()
        if (current.is_installed) {
          navigate({ to: '/console/signin' })
          return
        }
        setStatus(current)
      } catch {
        setError(t`Failed to fetch setup status`)
      } finally {
        setStatusLoading(false)
      }
    }
    fetchStatus()
    return () => {
      unmounted.current = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  /**
   * Polls the address that just answered setup.initialize until the restarted server answers:
   * a boot_id other than the one of the process that handled setup.initialize, since that
   * process still answers 200 while it drains.
   */
  const waitForRestart = async (rootEmail: string | undefined, previousBootId: string | undefined) => {
    const started = Date.now()
    const tick = () => Math.round((Date.now() - started) / 1000)
    setRestart({ phase: 'waiting', elapsed: 0, rootEmail })
    if (!previousBootId) await sleep(RESTART_PAUSE_MS)
    for (let attempt = 0; attempt < RESTART_ATTEMPTS; attempt++) {
      if (unmounted.current) return
      setRestart({ phase: 'waiting', elapsed: tick(), rootEmail })
      try {
        // A plain GET without custom headers: no CORS preflight while the server comes back.
        const response = await fetch(`${apiBaseUrl()}/api/setup.status?t=${Date.now()}`, { method: 'GET' })
        if (response.ok && (await isRestarted(response, previousBootId))) {
          setRestart({ phase: 'ready', elapsed: tick(), rootEmail })
          return
        }
      } catch {
        // Expected while the server is down.
      }
      await sleep(RESTART_POLL_MS)
    }
    setRestart({ phase: 'timeout', elapsed: tick(), rootEmail })
  }

  const submit = async () => {
    try {
      await form.validateFields()
    } catch {
      return
    }
    // All values, mounted or not: a value hidden behind a closed section is still sent.
    const all = form.getFieldsValue(true) as SetupFormValues
    setSubmitting(true)
    setError(undefined)
    let initializedBy: string | undefined
    try {
      initializedBy = (await setupApi.initialize(buildSetupConfig(all, status))).boot_id
    } catch (err) {
      setError(err instanceof Error ? err.message : t`Failed to complete setup`)
      setSubmitting(false)
      return
    }
    const rootEmail = isFieldLocked(status, 'root_email') ? undefined : splitList(all.root_email)[0]
    if (all.subscribe_newsletter && rootEmail) await subscribeToNewsletter(rootEmail, all)
    setSubmitting(false)
    await waitForRestart(rootEmail, initializedBy)
  }

  const goTo = (summary: PanelSummary) => {
    const [firstMissing] = summary.missing
    if (firstMissing) {
      form.scrollToField(firstMissing, { block: 'center', behavior: 'smooth', focus: true })
      return
    }
    document.getElementById(setupPanelDomId(summary.id))?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  }

  // A full reload, so /config.js is read again with the installed configuration.
  // The sign-in page prefills the root email it is told to sign in with.
  const signIn = () => {
    handOffSignInEmail(restart?.rootEmail)
    window.location.href = '/console/signin'
  }

  if (statusLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-canvas" aria-busy="true">
        <Spin description={t`Loading setup...`}>
          <span className="block size-16" />
        </Spin>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-canvas px-4 py-8 sm:px-6">
      <div className="mx-auto w-full max-w-[1000px]">
        <img src="/console/logo.png" alt="Notifuse" width={120} />
        {restart ? (
          <div className="mt-8">
            <SetupRestartView
              phase={restart.phase}
              elapsed={restart.elapsed}
              rootEmail={restart.rootEmail}
              onSignIn={signIn}
              onReload={() => window.location.reload()}
            />
          </div>
        ) : (
          <>
            <h1 data-unreset className="mt-4 mb-0.5 text-title font-semibold text-ink">
              {t`Set up your Notifuse instance`}
            </h1>
            <p data-unreset className="mt-0 mb-5 text-ink-2">
              {t`Done once. Everything here can be changed later in System settings.`}
            </p>
            <ConfigProvider theme={FORM_THEME}>
              <Form
                form={form}
                layout="vertical"
                initialValues={initialSetupValues()}
                onFinish={submit}
                onFieldsChange={() => setFieldsVersion((version) => version + 1)}
                requiredMark
                scrollToFirstError={{ block: 'center', behavior: 'smooth' }}
              >
                <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
                  <div className="flex min-w-0 flex-col gap-3.5">
                    <AdministratorPanel status={status} state={stateOf('administrator')} />
                    <SmtpPanel status={status} state={stateOf('smtp')} onTestFailedChange={setSmtpTestFailed} />
                    <UpdatesPanel status={status} state={stateOf('updates')} />
                    <SsoPanel status={status} state={stateOf('sso')} />
                    <BridgePanel status={status} state={stateOf('bridge')} />
                  </div>
                  <aside className="lg:sticky lg:top-4" aria-label={t`Setup summary`}>
                    <SetupRail summaries={summaries} error={error} submitting={submitting} onGoTo={goTo} />
                  </aside>
                </div>
              </Form>
            </ConfigProvider>
          </>
        )}
      </div>
    </div>
  )
}
