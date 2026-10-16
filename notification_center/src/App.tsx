import { useState, useEffect, useRef, useCallback } from 'react'
import {
  getContactPreferences,
  parseNotificationCenterParams,
  subscribeToLists,
  unsubscribeOneClick,
  updateContactPreferences
} from './api/notification_center'
import type { ContactPreferencesResponse, List } from './api/notification_center'
import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'sonner'
import { getTranslation, getLanguage } from './translations'
import type { Language } from './translations'
import languageIcon from './assets/language-icon.svg'

// Language display names for the selector
const languageNames: Record<Language, string> = {
  en: 'English',
  fr: 'Français',
  es: 'Español',
  de: 'Deutsch',
  zh: '中文',
  hi: 'हिन्दी',
  ar: 'العربية',
  pt: 'Português',
  ru: 'Русский',
  ja: '日本語',
  pl: 'Polski'
}

// The contact preference fields a write changes; absent fields are left as they are
interface PreferenceUpdate {
  language?: string
  timezone?: string
}

function App() {
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notificationData, setNotificationData] = useState<ContactPreferencesResponse | null>(null)
  const [subscriptions, setSubscriptions] = useState<Record<string, boolean>>({})
  const [processingLists, setProcessingLists] = useState<Record<string, boolean>>({})
  const [allLists, setAllLists] = useState<Array<List & { status?: string }>>([])
  const [language, setLanguage] = useState<Language>(getLanguage())
  const [showLanguageMenu, setShowLanguageMenu] = useState(false)
  const [confirmationResult, setConfirmationResult] = useState<{
    success: boolean
    messageKey: string
    listId?: string
  } | null>(null)
  const [isPreviewMode, setIsPreviewMode] = useState(false)
  // List the visitor is asked to confirm unsubscribing from (action=confirm_unsubscribe)
  const [pendingUnsubscribeListId, setPendingUnsubscribeListId] = useState<string | null>(null)
  const languageMenuRef = useRef<HTMLDivElement>(null)
  const confirmationResultRef = useRef<HTMLDivElement>(null)
  // Set when the result banner must take focus once shown: the confirmation card's button,
  // which had it, is gone
  const focusConfirmationResultRef = useRef(false)

  // Translation helper function
  const t = (key: string) => getTranslation(key, language)

  // Browser preferences detected on load, waiting for the visitor's first interaction
  const pendingPreferenceSyncRef = useRef<PreferenceUpdate | null>(null)
  const hasInteractedRef = useRef(false)
  // Preference writes are sent one after the other, so a language the visitor picks is
  // always saved after - and so overrides - the detected one
  const preferenceWritesRef = useRef<Promise<void>>(Promise.resolve())

  const savePreferences = useCallback((update: PreferenceUpdate) => {
    const params = parseNotificationCenterParams()
    if (!params) {
      return
    }

    preferenceWritesRef.current = preferenceWritesRef.current
      .then(() =>
        updateContactPreferences({
          workspace_id: params.wid,
          email: params.email,
          email_hmac: params.email_hmac,
          ...update
        })
      )
      .then(
        () => undefined,
        (err) => console.error('Failed to save preferences:', err)
      )
  }, [])

  // The browser's language and timezone are saved on the visitor's first click, tap or key
  // press, never on load: email security scanners open these links in real browsers and
  // would otherwise write their own timezone (often UTC) onto the contact.
  const syncPreferencesAfterInteraction = useCallback(
    (update: PreferenceUpdate) => {
      if (hasInteractedRef.current) {
        savePreferences(update)
      } else {
        pendingPreferenceSyncRef.current = update
      }
    },
    [savePreferences]
  )

  useEffect(() => {
    const onFirstInteraction = () => {
      document.removeEventListener('pointerdown', onFirstInteraction, true)
      document.removeEventListener('keydown', onFirstInteraction, true)
      hasInteractedRef.current = true
      if (pendingPreferenceSyncRef.current) {
        savePreferences(pendingPreferenceSyncRef.current)
        pendingPreferenceSyncRef.current = null
      }
    }

    // Capture phase, so the detected preferences are queued before anything the page's own
    // handlers save for the same interaction
    document.addEventListener('pointerdown', onFirstInteraction, true)
    document.addEventListener('keydown', onFirstInteraction, true)
    return () => {
      document.removeEventListener('pointerdown', onFirstInteraction, true)
      document.removeEventListener('keydown', onFirstInteraction, true)
    }
  }, [savePreferences])

  useEffect(() => {
    async function loadNotificationData() {
      ;(window as Window & { Consecration?: string }).Consecration =
        'Imperet illi Deus, supplices deprecamur: tuque, Princeps militiae caelestis, Satanam aliosque spiritus malignos, qui ad perditionem animarum pervagantur in mundo, divina virtute in infernum detrude. Amen.'

      try {
        // Parse parameters from URL
        const params = parseNotificationCenterParams()

        if (!params) {
          setError(t('missingParameters'))
          setLoading(false)
          return
        }

        // Detect preview mode (test email, preview message ID, or fake HMAC)
        const isPreview =
          params.mid === 'preview' ||
          params.email === 'john.doe@example.com' ||
          params.email_hmac === 'abc123' ||
          params.email_hmac === '...'

        if (isPreview) {
          setIsPreviewMode(true)
          setLoading(false)
          // Don't make API calls in preview mode
          return
        }

        // Handle confirmation action
        if (params.action === 'confirm' && params.lid) {
          try {
            // Use the existing API client to subscribe to lists
            const response = await subscribeToLists({
              workspace_id: params.wid,
              contact: {
                id: '', // Will be populated by the backend
                email: params.email,
                email_hmac: params.email_hmac
              },
              list_ids: [params.lid]
            })

            if (response.success) {
              setConfirmationResult({
                success: true,
                messageKey: 'confirmSubscriptionSuccess',
                listId: params.lid
              })
            } else {
              setConfirmationResult({
                success: false,
                messageKey: 'confirmSubscriptionFailed'
              })
            }
          } catch (err) {
            console.error('Failed to confirm subscription:', err)
            setConfirmationResult({
              success: false,
              messageKey: 'confirmSubscriptionFailed'
            })
          }
        }

        // Handle unsubscribe action
        if (params.action === 'unsubscribe' && params.lid) {
          try {
            // Automatically unsubscribe from the list
            const response = await unsubscribeOneClick({
              wid: params.wid,
              email: params.email,
              email_hmac: params.email_hmac,
              lids: [params.lid],
              mid: params.mid
            })

            if (response.success) {
              setConfirmationResult({
                success: true,
                messageKey: 'unsubscribeSuccess',
                listId: params.lid
              })
            } else {
              setConfirmationResult({
                success: false,
                messageKey: 'unsubscribeFailed'
              })
            }
          } catch (err) {
            console.error('Failed to unsubscribe:', err)
            setConfirmationResult({
              success: false,
              messageKey: 'unsubscribeFailed'
            })
          }
        }

        // Load notification center data
        const data = await getContactPreferences({
          workspace_id: params.wid,
          email: params.email,
          email_hmac: params.email_hmac
        })
        setNotificationData(data)

        // Auto-detect browser language and timezone, sync if different from contact
        const browserLang = navigator.language.split('-')[0].toLowerCase()
        const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone
        const contactLang = data.contact?.language || null
        const contactTz = data.contact?.timezone || null

        const langChanged = browserLang.length === 2 && !contactLang
        const tzChanged = !!browserTz && contactTz !== browserTz

        if (langChanged || tzChanged) {
          syncPreferencesAfterInteraction({
            ...(langChanged ? { language: browserLang } : {}),
            ...(tzChanged ? { timezone: browserTz } : {})
          })

          // Also update the widget UI language if the browser lang is supported
          if (langChanged && browserLang in languageNames) {
            setLanguage(browserLang as Language)
          }
        }

        // Initialize subscriptions state
        const initialSubscriptions: Record<string, boolean> = {}

        // Combine public lists and contact-specific lists
        const combinedLists: Array<List & { status?: string }> = []

        // Process all contact lists to get status
        if (data.contact_lists) {
          data.contact_lists.forEach((contactList) => {
            // Set subscription status based on contact list status
            initialSubscriptions[contactList.list_id] = contactList.status === 'active'

            // Try to find this list in public lists to get name and description
            const publicList = data.public_lists?.find((list) => list.id === contactList.list_id)

            if (publicList) {
              // For lists in both contact_lists and public_lists
              combinedLists.push({
                ...publicList,
                status: contactList.status
              })
            } else {
              // For lists only in contact_lists (private lists)
              combinedLists.push({
                id: contactList.list_id,
                name: contactList.list_name || `List ${contactList.list_id}`,
                status: contactList.status
              })
            }
          })
        }

        // Add public lists that aren't in contact_lists
        if (data.public_lists) {
          data.public_lists.forEach((list) => {
            const existingList = combinedLists.find((l) => l.id === list.id)
            if (!existingList) {
              combinedLists.push({
                ...list,
                status: 'unsubscribed' // Default status for public lists not in contact_lists
              })
              initialSubscriptions[list.id] = false
            }
          })
        }

        setAllLists(combinedLists)
        setSubscriptions(initialSubscriptions)

        // The List-Unsubscribe header link lands here when it is opened rather than
        // one-click POSTed (the server redirects the GET). Anti-spam scanners fetch header
        // links on their own, so nothing is unsubscribed until the visitor confirms.
        if (params.action === 'confirm_unsubscribe' && params.lid) {
          // Automations also email contacts who aren't on their list, with that list's
          // header link, so a contact with no row on the list is asked too, and so is one
          // whose double opt-in is still pending, whom marketing emails reach as well
          const row = data.contact_lists?.find((cl) => cl.list_id === params.lid)
          if (initialSubscriptions[params.lid] || !row || row.status === 'pending') {
            setPendingUnsubscribeListId(params.lid)
          } else if (
            combinedLists.find((list) => list.id === params.lid)?.status === 'unsubscribed'
          ) {
            setConfirmationResult({
              success: true,
              messageKey: 'alreadyUnsubscribed',
              listId: params.lid
            })
          }
        }

        setLoading(false)
      } catch (err) {
        console.error('Failed to load notification center data:', err)
        setError(err instanceof Error ? err.message : t('failedToLoad'))
        setLoading(false)
      }
    }

    loadNotificationData()
    // Runs exactly once, on purpose: the load performs the URL's one-shot actions
    // (action=unsubscribe, action=confirm), which must never repeat
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (confirmationResult && focusConfirmationResultRef.current) {
      focusConfirmationResultRef.current = false
      confirmationResultRef.current?.focus()
    }
  }, [confirmationResult])

  // Set favicon when logo is available
  useEffect(() => {
    if (notificationData?.logo_url) {
      const existingLink = document.querySelector("link[rel*='icon']") as HTMLLinkElement | null
      const link = existingLink || document.createElement('link')
      link.type = 'image/x-icon'
      link.rel = 'shortcut icon'
      link.href = notificationData.logo_url

      if (!existingLink) {
        document.head.appendChild(link)
      }
    }
  }, [notificationData?.logo_url])

  // Update page title with contact information
  useEffect(() => {
    if (notificationData?.contact) {
      const pageName = getTranslation('emailSubscriptions', language)
      document.title = `${notificationData.contact.email} | ${pageName}`
    }
  }, [notificationData?.contact, language])

  // Handle clicks outside language menu to close it
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        languageMenuRef.current &&
        !languageMenuRef.current.contains(event.target as Node) &&
        showLanguageMenu
      ) {
        setShowLanguageMenu(false)
      }
    }

    function handleEscapeKey(event: KeyboardEvent) {
      if (event.key === 'Escape' && showLanguageMenu) {
        setShowLanguageMenu(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscapeKey)

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscapeKey)
    }
  }, [showLanguageMenu])

  // Handle keyboard events for language menu
  const handleLanguageKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      setShowLanguageMenu(false)
    }
    if (e.key === 'Enter' || e.key === ' ') {
      setShowLanguageMenu(!showLanguageMenu)
    }
  }

  // Handle keyboard selection within dropdown
  const handleLanguageOptionKeyDown = (e: React.KeyboardEvent, code: string) => {
    if (e.key === 'Enter' || e.key === ' ') {
      setLanguage(code as Language)
      setShowLanguageMenu(false)
      e.preventDefault()
      if (!isPreviewMode) {
        savePreferences({ language: code })
      }
    }
  }

  const subscribe = async (listId: string) => {
    try {
      // Clear any previous confirmation result (e.g., from unsubscribe action)
      setConfirmationResult(null)

      // Set processing state
      setProcessingLists((prev) => ({ ...prev, [listId]: true }))

      // Update local state optimistically
      setSubscriptions((prev) => ({
        ...prev,
        [listId]: true
      }))

      // Also update list status if possible
      setAllLists((prev) =>
        prev.map((list) => (list.id === listId ? { ...list, status: 'active' } : list))
      )

      if (notificationData?.contact) {
        const params = parseNotificationCenterParams()

        if (!params) {
          throw new Error('Missing required parameters')
        }

        // Call API to subscribe to list
        // Include email_hmac from URL params for private list authentication
        await subscribeToLists({
          workspace_id: params.wid,
          contact: { ...notificationData.contact, email_hmac: params.email_hmac },
          list_ids: [listId]
        })

        toast.success(t('successSubscribed'), {
          style: { backgroundColor: '#f0fdf4', borderLeft: '4px solid #22c55e', color: '#166534' },
          duration: 3000
        })
      }
    } catch (err) {
      // Revert local state on error
      setSubscriptions((prev) => ({
        ...prev,
        [listId]: false
      }))

      // Revert list status
      setAllLists((prev) =>
        prev.map((list) => (list.id === listId ? { ...list, status: 'unsubscribed' } : list))
      )

      console.error('Failed to subscribe:', err)
      toast.error(t('failedSubscribe'))
    } finally {
      // Clear processing state
      setProcessingLists((prev) => ({ ...prev, [listId]: false }))
    }
  }

  const unsubscribe = async (listId: string) => {
    try {
      // The list's own toggle answers a pending unsubscribe confirmation for it
      setPendingUnsubscribeListId((pending) => (pending === listId ? null : pending))

      // Set processing state
      setProcessingLists((prev) => ({ ...prev, [listId]: true }))

      // Update local state optimistically
      setSubscriptions((prev) => ({
        ...prev,
        [listId]: false
      }))

      // Also update list status if possible
      setAllLists((prev) =>
        prev.map((list) => (list.id === listId ? { ...list, status: 'unsubscribed' } : list))
      )

      if (notificationData?.contact) {
        const params = parseNotificationCenterParams()

        if (!params) {
          throw new Error('Missing required parameters')
        }

        // Call API to unsubscribe from list
        await unsubscribeOneClick({
          wid: params.wid,
          email: params.email,
          email_hmac: params.email_hmac,
          lids: [listId],
          mid: params.mid
        })

        toast.success(t('successUnsubscribed'), {
          style: { backgroundColor: '#f0fdf4', borderLeft: '4px solid #22c55e', color: '#166534' },
          duration: 3000
        })
      }
    } catch (err) {
      // Revert local state on error
      setSubscriptions((prev) => ({
        ...prev,
        [listId]: true
      }))

      // Revert list status
      setAllLists((prev) =>
        prev.map((list) => (list.id === listId ? { ...list, status: 'active' } : list))
      )

      console.error('Failed to unsubscribe:', err)
      toast.error(t('failedUnsubscribe'))
    } finally {
      // Clear processing state
      setProcessingLists((prev) => ({ ...prev, [listId]: false }))
    }
  }

  const confirmUnsubscribe = async (listId: string) => {
    const params = parseNotificationCenterParams()
    if (!params) {
      return
    }

    setProcessingLists((prev) => ({ ...prev, [listId]: true }))

    try {
      const response = await unsubscribeOneClick({
        wid: params.wid,
        email: params.email,
        email_hmac: params.email_hmac,
        lids: [listId],
        mid: params.mid
      })

      if (!response.success) {
        throw new Error('Unsubscribe was not successful')
      }

      setSubscriptions((prev) => ({ ...prev, [listId]: false }))
      setAllLists((prev) =>
        prev.map((list) => (list.id === listId ? { ...list, status: 'unsubscribed' } : list))
      )
      setPendingUnsubscribeListId(null)
      focusConfirmationResultRef.current = true
      setConfirmationResult({ success: true, messageKey: 'unsubscribeSuccess', listId })
    } catch (err) {
      console.error('Failed to unsubscribe:', err)
      setConfirmationResult({ success: false, messageKey: 'unsubscribeFailed' })
    } finally {
      setProcessingLists((prev) => ({ ...prev, [listId]: false }))
    }
  }

  if (loading) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-white">
        <div className="p-6 max-w-sm mx-auto">
          <div className="text-center">
            <div className="text-xl font-medium text-black">{t('loading')}</div>
          </div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-white">
        <div className="p-6 max-w-sm mx-auto">
          <div className="text-center">
            <div className="text-xl font-medium text-red-500">{t('error')}</div>
            <p className="text-gray-700 mt-2">{error}</p>
          </div>
        </div>
      </div>
    )
  }

  if (isPreviewMode) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-50">
        <div className="p-8 max-w-2xl mx-auto">
          <div className="bg-white rounded-lg shadow-xl p-8 border-2 border-indigo-200">
            <div className="text-center">
              <div className="text-5xl mb-4">👁️</div>
              <div className="text-3xl font-bold text-indigo-600 mb-4">Preview Mode</div>
              <p className="text-gray-700 text-lg mb-6">
                This is a preview of your email template's notification center links.
              </p>
              <div className="bg-indigo-50 border border-indigo-200 rounded-lg p-6 text-left">
                <p className="text-sm text-gray-700 mb-3">
                  <strong className="text-indigo-700">What you're seeing:</strong>
                </p>
                <ul className="text-sm text-gray-600 space-y-2 list-disc list-inside">
                  <li>This preview shows how subscription/unsubscribe links work</li>
                  <li>In production, real contacts will see their actual preferences</li>
                  <li>Links in live emails use secure authentication</li>
                </ul>
              </div>
              <div className="mt-6 pt-6 border-t border-gray-200">
                <p className="text-sm text-gray-500">
                  To test with real functionality, send yourself a test email from the template
                  editor.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    )
  }

  const websiteUrl = notificationData?.website_url || '#'

  // The preferences leave out private lists the contact isn't on, so for those the page
  // only knows the list's ID, which means nothing to them: the card then goes unnamed
  const pendingUnsubscribeList: List | undefined = pendingUnsubscribeListId
    ? (allLists.find((list) => list.id === pendingUnsubscribeListId) ?? {
        id: pendingUnsubscribeListId,
        name: ''
      })
    : undefined

  return (
    <div className="min-h-screen flex flex-col bg-white">
      <Toaster />
      {/* Topbar with bottom border */}
      <div className="bg-white border-b border-gray-200 w-full">
        <div className="flex items-center h-16 px-4 max-w-[600px] mx-auto">
          <div className="flex-shrink-0 mr-4 md:mr-6">
            {notificationData?.logo_url ? (
              <a href={websiteUrl} target="_blank" rel="noopener noreferrer" title="Visit website">
                <img
                  src={notificationData.logo_url}
                  alt="Workspace Logo"
                  className="h-8 md:h-10 w-auto object-contain"
                />
              </a>
            ) : (
              <div className="w-8 md:w-10 h-8 md:h-10"></div> /* Empty space when no logo */
            )}
          </div>
          <div className="text-sm font-medium text-gray-800 flex-1 text-center">
            <a
              href={websiteUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="hover:underline"
            >
              {t('emailSubscriptions')}
            </a>
          </div>
          {/* Language selector */}
          <div className="flex-shrink-0">
            <div className="relative" ref={languageMenuRef}>
              <button
                className="flex items-center focus:outline-none rounded-sm p-1 transition-all border border-gray-300 hover:bg-gray-50 cursor-pointer"
                onClick={() => setShowLanguageMenu(!showLanguageMenu)}
                onKeyDown={handleLanguageKeyDown}
                aria-label="Select language"
                aria-expanded={showLanguageMenu}
                aria-haspopup="true"
              >
                <img
                  src={languageIcon}
                  alt="Language"
                  className="h-6 w-6 opacity-70 hover:opacity-100 transition-opacity"
                />
              </button>

              {showLanguageMenu && (
                <div
                  className="absolute right-0 mt-2 py-2 w-32 bg-white rounded-md shadow-lg border border-gray-200 z-10"
                  role="menu"
                >
                  {Object.entries(languageNames).map(([code, name]) => (
                    <button
                      key={code}
                      className="block w-full text-left px-4 py-1 text-sm hover:bg-gray-100 cursor-pointer"
                      onClick={() => {
                        setLanguage(code as Language)
                        setShowLanguageMenu(false)
                        if (!isPreviewMode) {
                          savePreferences({ language: code })
                        }
                      }}
                      onKeyDown={(e) => handleLanguageOptionKeyDown(e, code)}
                      role="menuitem"
                      tabIndex={0}
                    >
                      {name}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col items-center p-4">
        <div className="w-full max-w-[600px]">
          {notificationData && (
            <>
              <div className="mb-6 mt-4">
                <div className="text-md font-medium">
                  {t('welcome')}{' '}
                  {notificationData.contact.first_name || notificationData.contact.email}
                </div>
              </div>

              {/* Confirmation result */}
              {confirmationResult && (
                <div
                  ref={confirmationResultRef}
                  role={confirmationResult.success ? 'status' : 'alert'}
                  tabIndex={-1}
                  className={`mb-6 p-4 rounded-sm border ${
                    confirmationResult.success
                      ? 'bg-green-50 border-green-200 text-green-800'
                      : 'bg-red-50 border-red-200 text-red-800'
                  }`}
                >
                  <div className="font-medium">
                    {confirmationResult.success ? t('successHeader') : t('errorHeader')}
                  </div>
                  <div className="mt-1 text-sm">{t(confirmationResult.messageKey)}</div>
                </div>
              )}

              {/* Unsubscribe confirmation (List-Unsubscribe header link opened in a browser) */}
              {pendingUnsubscribeList && (
                <div
                  className="mb-6 p-4 rounded-sm border border-gray-300 bg-gray-50"
                  data-testid="confirm-unsubscribe"
                >
                  {pendingUnsubscribeList.name && (
                    <div className="font-medium">{pendingUnsubscribeList.name}</div>
                  )}
                  <p className="mt-1 text-sm text-gray-700">{t('confirmUnsubscribePrompt')}</p>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => confirmUnsubscribe(pendingUnsubscribeList.id)}
                    disabled={processingLists[pendingUnsubscribeList.id]}
                    // The list's own row has an "Unsubscribe" button too
                    aria-label={
                      pendingUnsubscribeList.name
                        ? `${t('unsubscribe')}: ${pendingUnsubscribeList.name}`
                        : t('unsubscribe')
                    }
                    className="mt-3 cursor-pointer border-red-500 text-red-500 hover:bg-red-50"
                  >
                    {processingLists[pendingUnsubscribeList.id]
                      ? t('processing')
                      : t('unsubscribe')}
                  </Button>
                </div>
              )}

              {/* All Lists section with toggles - showing both public and private lists */}
              {allLists.length > 0 && (
                <div className="mb-6">
                  <div className="space-y-3">
                    {allLists.map((list) => {
                      const isSubscribed = subscriptions[list.id] || false
                      const isActive = list.status === 'active'
                      const isPublic =
                        notificationData?.public_lists?.some((l) => l.id === list.id) ?? false
                      // The workspace decides who joins a private list: a contact can only
                      // leave one, and the server refuses a subscribe
                      const canToggle =
                        list.status !== 'bounced' &&
                        list.status !== 'complained' &&
                        (isPublic || isSubscribed)

                      return (
                        <div
                          key={list.id}
                          className={`p-4 border border-gray-300 rounded-sm ${
                            isActive ? 'bg-white' : 'bg-gray-50'
                          }`}
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="font-medium">
                                {list.name}
                                {list.status &&
                                  list.status !== 'active' &&
                                  list.status !== 'unsubscribed' && (
                                    <span className="ml-2 text-xs px-2 py-1 bg-gray-200 text-gray-700 rounded-full">
                                      {list.status}
                                    </span>
                                  )}
                              </div>
                              {list.description && (
                                <p className="text-sm text-gray-600 mt-1">{list.description}</p>
                              )}
                            </div>
                            <div className="ml-4">
                              <Button
                                variant="outline"
                                onClick={() =>
                                  isSubscribed ? unsubscribe(list.id) : subscribe(list.id)
                                }
                                size="sm"
                                disabled={processingLists[list.id] || !canToggle}
                                className={`cursor-pointer ${
                                  !canToggle
                                    ? 'border-gray-300 text-gray-400 cursor-not-allowed'
                                    : isSubscribed
                                    ? 'border-red-500 text-red-500 hover:bg-red-50'
                                    : 'border-blue-500 text-blue-500 hover:bg-blue-50'
                                }`}
                              >
                                {processingLists[list.id]
                                  ? t('processing')
                                  : !canToggle
                                  ? list.status === 'unsubscribed'
                                    ? t('unsubscribedStatus')
                                    : list.status
                                  : isSubscribed
                                  ? t('unsubscribe')
                                  : t('subscribe')}
                              </Button>
                            </div>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              {/* Empty state when no lists */}
              {allLists.length === 0 && (
                <p className="text-center text-gray-500 py-4">{t('noSubscriptions')}</p>
              )}
            </>
          )}
        </div>
      </div>

      {/* Footer */}
      <div className="border-t border-gray-200 py-4 text-center text-sm text-gray-500">
        <a
          href={websiteUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="hover:text-gray-700 hover:underline"
        >
          {t('visitWebsite')}
        </a>
      </div>
    </div>
  )
}

export default App
