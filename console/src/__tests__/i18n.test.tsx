import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider, useLingui } from '@lingui/react'
import { LocaleProvider, useLocale } from '../contexts/LocaleContext'
import { LanguageSwitcher } from '../components/LanguageSwitcher'
import {
  loadLocale,
  getInitialLocale,
  initI18n,
  locales,
  localeNames,
  type Locale,
} from '../i18n'

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key]
    }),
    clear: vi.fn(() => {
      store = {}
    }),
  }
})()

Object.defineProperty(window, 'localStorage', { value: localStorageMock })

// Sample translations for testing
const englishMessages = {
  Hello: 'Hello',
  Goodbye: 'Goodbye',
  'Welcome {name}': 'Welcome {name}',
}

const frenchMessages = {
  Hello: 'Bonjour',
  Goodbye: 'Au revoir',
  'Welcome {name}': 'Bienvenue {name}',
}

const spanishMessages = {
  Hello: 'Hola',
  Goodbye: 'Adiós',
  'Welcome {name}': 'Bienvenido {name}',
}

const germanMessages = {
  Hello: 'Hallo',
  Goodbye: 'Auf Wiedersehen',
  'Welcome {name}': 'Willkommen {name}',
}

const catalanMessages = {
  Hello: 'Hola',
  Goodbye: 'Adéu',
  'Welcome {name}': 'Benvingut {name}',
}

// Mock the dynamic imports for locale files
vi.mock('../i18n/locales/en.po', () => ({ messages: englishMessages }))
vi.mock('../i18n/locales/fr.po', () => ({ messages: frenchMessages }))
vi.mock('../i18n/locales/es.po', () => ({ messages: spanishMessages }))
vi.mock('../i18n/locales/de.po', () => ({ messages: germanMessages }))
vi.mock('../i18n/locales/ca.po', () => ({ messages: catalanMessages }))

describe('i18n utility functions', () => {
  beforeEach(() => {
    localStorageMock.clear()
    vi.clearAllMocks()
  })

  describe('getInitialLocale', () => {
    it('returns "en" when localStorage is empty', () => {
      expect(getInitialLocale()).toBe('en')
    })

    it('returns stored locale when valid', () => {
      localStorageMock.setItem('locale', 'fr')
      expect(getInitialLocale()).toBe('fr')
    })

    it('returns "en" when stored locale is invalid', () => {
      localStorageMock.setItem('locale', 'invalid')
      expect(getInitialLocale()).toBe('en')
    })

    it('returns stored locale for all valid locales', () => {
      for (const locale of locales) {
        localStorageMock.setItem('locale', locale)
        expect(getInitialLocale()).toBe(locale)
      }
    })
  })

  describe('loadLocale', () => {
    it('loads and activates English locale', async () => {
      await loadLocale('en')
      expect(i18n.locale).toBe('en')
      expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'en')
    })

    it('loads and activates French locale', async () => {
      await loadLocale('fr')
      expect(i18n.locale).toBe('fr')
      expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'fr')
    })

    it('loads and activates all supported locales', async () => {
      for (const locale of locales) {
        await loadLocale(locale)
        expect(i18n.locale).toBe(locale)
        expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', locale)
      }
    })

    it('persists locale to localStorage', async () => {
      await loadLocale('es')
      expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'es')
    })
  })

  describe('initI18n', () => {
    it('initializes with default locale when localStorage is empty', async () => {
      await initI18n()
      expect(i18n.locale).toBe('en')
    })

    it('initializes with stored locale from localStorage', async () => {
      localStorageMock.setItem('locale', 'de')
      await initI18n()
      expect(i18n.locale).toBe('de')
    })
  })

  describe('locales and localeNames', () => {
    it('exports all supported locales', () => {
      expect(locales).toEqual(['en', 'fr', 'es', 'de', 'ca'])
    })

    it('exports locale names for all locales', () => {
      expect(localeNames).toEqual({
        en: 'English',
        fr: 'Français',
        es: 'Español',
        de: 'Deutsch',
        ca: 'Català',
      })
    })

    it('has a name for every locale', () => {
      for (const locale of locales) {
        expect(localeNames[locale]).toBeDefined()
        expect(typeof localeNames[locale]).toBe('string')
        expect(localeNames[locale].length).toBeGreaterThan(0)
      }
    })
  })
})

describe('LocaleContext', () => {
  beforeEach(async () => {
    localStorageMock.clear()
    vi.clearAllMocks()
    // Reset i18n to clean state
    i18n.load('en', englishMessages)
    i18n.activate('en')
  })

  // Test component that displays locale info
  function LocaleDisplay() {
    const { locale, locales: availableLocales, localeNames: names, isLoading } = useLocale()
    return (
      <div>
        <span data-testid="current-locale">{locale}</span>
        <span data-testid="locale-count">{availableLocales.length}</span>
        <span data-testid="locale-name">{names[locale]}</span>
        <span data-testid="is-loading">{isLoading ? 'loading' : 'ready'}</span>
      </div>
    )
  }

  // Test component for locale switching
  function LocaleSwitcher() {
    const { locale, setLocale } = useLocale()
    return (
      <div>
        <span data-testid="current-locale">{locale}</span>
        <button onClick={() => setLocale('fr')} data-testid="switch-to-fr">
          Switch to French
        </button>
        <button onClick={() => setLocale('es')} data-testid="switch-to-es">
          Switch to Spanish
        </button>
        <button onClick={() => setLocale('en')} data-testid="switch-to-en">
          Switch to English
        </button>
      </div>
    )
  }

  function renderWithProviders(ui: React.ReactElement) {
    return render(<LocaleProvider>{ui}</LocaleProvider>)
  }

  it('provides default locale', async () => {
    renderWithProviders(<LocaleDisplay />)

    await waitFor(() => {
      expect(screen.getByTestId('is-loading')).toHaveTextContent('ready')
    })

    expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
  })

  it('provides all available locales', async () => {
    renderWithProviders(<LocaleDisplay />)

    await waitFor(() => {
      expect(screen.getByTestId('is-loading')).toHaveTextContent('ready')
    })

    expect(screen.getByTestId('locale-count')).toHaveTextContent('5')
  })

  it('provides locale name for current locale', async () => {
    renderWithProviders(<LocaleDisplay />)

    await waitFor(() => {
      expect(screen.getByTestId('is-loading')).toHaveTextContent('ready')
    })

    expect(screen.getByTestId('locale-name')).toHaveTextContent('English')
  })

  it('switches locale when setLocale is called', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LocaleSwitcher />)

    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
    })

    await user.click(screen.getByTestId('switch-to-fr'))

    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('fr')
    })
  })

  it('persists locale change to localStorage', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LocaleSwitcher />)

    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
    })

    await user.click(screen.getByTestId('switch-to-es'))

    await waitFor(() => {
      expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'es')
    })
  })

  it('throws error when useLocale is used outside LocaleProvider', () => {
    // Suppress console.error for this test
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    expect(() => {
      render(<LocaleDisplay />)
    }).toThrow('useLocale must be used within a LocaleProvider')

    consoleSpy.mockRestore()
  })
})

describe('LanguageSwitcher component', () => {
  beforeEach(async () => {
    localStorageMock.clear()
    vi.clearAllMocks()
    i18n.load('en', englishMessages)
    i18n.activate('en')
  })

  function renderLanguageSwitcher() {
    return render(
      <LocaleProvider>
        <I18nProvider i18n={i18n}>
          <LanguageSwitcher />
        </I18nProvider>
      </LocaleProvider>
    )
  }

  it('renders current locale in button', async () => {
    renderLanguageSwitcher()

    await waitFor(() => {
      expect(screen.getByRole('button')).toHaveTextContent('EN')
    })
  })

  it('shows dropdown menu when clicked', async () => {
    const user = userEvent.setup()
    renderLanguageSwitcher()

    await waitFor(() => {
      expect(screen.getByRole('button')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('English')).toBeInTheDocument()
      expect(screen.getByText('Français')).toBeInTheDocument()
      expect(screen.getByText('Español')).toBeInTheDocument()
      expect(screen.getByText('Deutsch')).toBeInTheDocument()
      expect(screen.getByText('Català')).toBeInTheDocument()
    })
  })

  it('switches locale when menu item is clicked', async () => {
    const user = userEvent.setup()
    renderLanguageSwitcher()

    await waitFor(() => {
      expect(screen.getByRole('button')).toHaveTextContent('EN')
    })

    await user.click(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('Français')).toBeInTheDocument()
    })

    await user.click(screen.getByText('Français'))

    await waitFor(() => {
      expect(screen.getByRole('button')).toHaveTextContent('FR')
    })
  })
})

describe('Translation rendering', () => {
  beforeEach(async () => {
    localStorageMock.clear()
    vi.clearAllMocks()
  })

  // Component that uses translations via the non-macro useLingui
  function TranslatedComponent() {
    const { i18n: linguiI18n } = useLingui()
    const { locale, setLocale } = useLocale()

    // Use i18n._ for translations in tests (non-macro approach)
    const greeting = linguiI18n._('Hello')
    const farewell = linguiI18n._('Goodbye')

    return (
      <div>
        <span data-testid="greeting">{greeting}</span>
        <span data-testid="farewell">{farewell}</span>
        <span data-testid="current-locale">{locale}</span>
        <span data-testid="i18n-locale">{linguiI18n.locale}</span>
        <button onClick={() => setLocale('fr')} data-testid="switch-to-fr">
          French
        </button>
        <button onClick={() => setLocale('en')} data-testid="switch-to-en">
          English
        </button>
        <button onClick={() => setLocale('es')} data-testid="switch-to-es">
          Spanish
        </button>
      </div>
    )
  }

  function renderWithI18n() {
    return render(
      <LocaleProvider>
        <I18nProvider i18n={i18n}>
          <TranslatedComponent />
        </I18nProvider>
      </LocaleProvider>
    )
  }

  it('renders English translations by default', async () => {
    renderWithI18n()

    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
    })

    expect(screen.getByTestId('greeting')).toBeInTheDocument()
    expect(screen.getByTestId('farewell')).toBeInTheDocument()
    expect(screen.getByTestId('i18n-locale')).toHaveTextContent('en')
  })

  it('updates i18n locale when context locale changes', async () => {
    const user = userEvent.setup()
    renderWithI18n()

    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('en')
    })

    await user.click(screen.getByTestId('switch-to-fr'))

    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('fr')
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('fr')
    })

    // Verify global i18n locale changed
    expect(i18n.locale).toBe('fr')
  })

  it('can switch between multiple locales', async () => {
    const user = userEvent.setup()
    renderWithI18n()

    // Start with English
    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('en')
    })

    // Switch to French
    await user.click(screen.getByTestId('switch-to-fr'))
    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('fr')
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('fr')
    })

    // Switch to Spanish
    await user.click(screen.getByTestId('switch-to-es'))
    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('es')
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('es')
    })

    // Switch back to English
    await user.click(screen.getByTestId('switch-to-en'))
    await waitFor(() => {
      expect(screen.getByTestId('current-locale')).toHaveTextContent('en')
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('en')
    })
  })

  it('renders translated content based on active locale', async () => {
    const user = userEvent.setup()
    renderWithI18n()

    // Wait for initial load with English
    await waitFor(() => {
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('en')
    })
    expect(screen.getByTestId('greeting')).toHaveTextContent('Hello')
    expect(screen.getByTestId('farewell')).toHaveTextContent('Goodbye')

    // Switch to French and verify translations
    await user.click(screen.getByTestId('switch-to-fr'))
    await waitFor(() => {
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('fr')
    })
    expect(screen.getByTestId('greeting')).toHaveTextContent('Bonjour')
    expect(screen.getByTestId('farewell')).toHaveTextContent('Au revoir')

    // Switch to Spanish
    await user.click(screen.getByTestId('switch-to-es'))
    await waitFor(() => {
      expect(screen.getByTestId('i18n-locale')).toHaveTextContent('es')
    })
    expect(screen.getByTestId('greeting')).toHaveTextContent('Hola')
    expect(screen.getByTestId('farewell')).toHaveTextContent('Adiós')
  })
})

describe('Locale switching updates translations', () => {
  beforeEach(async () => {
    localStorageMock.clear()
    vi.clearAllMocks()
    // Load actual translations
    i18n.load('en', englishMessages)
    i18n.load('fr', frenchMessages)
    i18n.activate('en')
  })

  /**
   * This test simulates the real app structure where:
   * 1. LocaleProvider wraps the app
   * 2. AppContent (inner component) uses useLocale() to get locale
   * 3. I18nProvider is inside AppContent with a static i18n reference
   * 4. Child components use useLingui() to translate
   *
   * The bug: When locale changes, AppContent re-renders, but the I18nProvider
   * doesn't force its children to re-render because the i18n object reference
   * is the same (only its internal state changed via i18n.activate()).
   */
  it('translations should update when locale changes (simulating real app structure)', async () => {
    const user = userEvent.setup()

    // Simulates a deeply nested component that uses translations
    // This component does NOT have access to locale context directly
    function DeepChildComponent() {
      const { i18n: linguiI18n } = useLingui()
      // This translation is computed once when component mounts
      // and may not update when i18n.activate() is called
      return <span data-testid="deep-translated">{linguiI18n._('Hello')}</span>
    }

    // Simulates AppContent from App.tsx
    function AppContent() {
      const { locale } = useLocale()
      // Note: I18nProvider receives the same i18n object reference
      // When locale changes, this component re-renders, but...
      return (
        <I18nProvider i18n={i18n}>
          <div>
            <span data-testid="app-locale">{locale}</span>
            <DeepChildComponent />
          </div>
        </I18nProvider>
      )
    }

    // Separate component for the language switcher (like LanguageSwitcher.tsx)
    function LanguageSwitcherTest() {
      const { setLocale } = useLocale()
      return (
        <button onClick={() => setLocale('fr')} data-testid="switch-fr">
          Switch to French
        </button>
      )
    }

    // Full app structure
    render(
      <LocaleProvider>
        <AppContent />
        <LanguageSwitcherTest />
      </LocaleProvider>
    )

    // Initial state: English
    await waitFor(() => {
      expect(screen.getByTestId('app-locale')).toHaveTextContent('en')
    })
    expect(screen.getByTestId('deep-translated')).toHaveTextContent('Hello')

    // Switch to French
    await user.click(screen.getByTestId('switch-fr'))

    // Context locale updates
    await waitFor(() => {
      expect(screen.getByTestId('app-locale')).toHaveTextContent('fr')
    })

    // i18n internal locale also updates
    expect(i18n.locale).toBe('fr')

    // The deep child should now show French translation
    // If this fails (still shows "Hello"), the bug exists
    expect(screen.getByTestId('deep-translated')).toHaveTextContent('Bonjour')
  })

  /**
   * Test that demonstrates the fix: adding key={locale} to I18nProvider
   * forces a remount when locale changes, ensuring translations update.
   */
  it('FIX: adding key to I18nProvider forces re-render on locale change', async () => {
    const user = userEvent.setup()

    function DeepChildComponent() {
      const { i18n: linguiI18n } = useLingui()
      return <span data-testid="deep-translated">{linguiI18n._('Hello')}</span>
    }

    // Fixed AppContent: adds key={locale} to I18nProvider
    function AppContentFixed() {
      const { locale } = useLocale()
      return (
        // KEY FIX: Adding key={locale} forces I18nProvider to remount
        // when locale changes, causing all children to re-render
        <I18nProvider i18n={i18n} key={locale}>
          <div>
            <span data-testid="app-locale">{locale}</span>
            <DeepChildComponent />
          </div>
        </I18nProvider>
      )
    }

    function LanguageSwitcherTest() {
      const { setLocale } = useLocale()
      return (
        <button onClick={() => setLocale('fr')} data-testid="switch-fr">
          Switch to French
        </button>
      )
    }

    render(
      <LocaleProvider>
        <AppContentFixed />
        <LanguageSwitcherTest />
      </LocaleProvider>
    )

    // Initial state: English
    await waitFor(() => {
      expect(screen.getByTestId('app-locale')).toHaveTextContent('en')
    })
    expect(screen.getByTestId('deep-translated')).toHaveTextContent('Hello')

    // Switch to French
    await user.click(screen.getByTestId('switch-fr'))

    // Context locale updates
    await waitFor(() => {
      expect(screen.getByTestId('app-locale')).toHaveTextContent('fr')
    })

    // With the key fix, translations should update
    expect(screen.getByTestId('deep-translated')).toHaveTextContent('Bonjour')
  })
})

describe('Locale persistence across sessions', () => {
  beforeEach(() => {
    localStorageMock.clear()
    vi.clearAllMocks()
  })

  it('restores locale from localStorage on mount', async () => {
    // Pre-set locale in localStorage
    localStorageMock.getItem.mockReturnValue('de')

    function LocaleDisplay() {
      const { locale } = useLocale()
      return <span data-testid="locale">{locale}</span>
    }

    render(
      <LocaleProvider>
        <LocaleDisplay />
      </LocaleProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('locale')).toHaveTextContent('de')
    })
  })

  it('saves locale to localStorage when changed', async () => {
    const user = userEvent.setup()

    function LocaleSwitcher() {
      const { setLocale } = useLocale()
      return (
        <button onClick={() => setLocale('ca')} data-testid="switch">
          Switch
        </button>
      )
    }

    render(
      <LocaleProvider>
        <LocaleSwitcher />
      </LocaleProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('switch')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('switch'))

    await waitFor(() => {
      expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'ca')
    })
  })
})
