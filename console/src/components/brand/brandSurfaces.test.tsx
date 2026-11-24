import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import { versionLabel, withBrandName } from './instanceBranding'
import { getThemePresets } from '../blog/themePresets'
import { buildEmailSystemPrompt } from '../email_builder/email-ai-system-prompt'
import { UpdateCard } from '../updates/UpdateCard'
import { UpdateFooter } from '../updates/UpdateFooter'
import { RecoveryCodesList } from '../two_factor/RecoveryCodesList'
import { AccountMenu } from '../../layouts/app-shell/AccountMenu'

vi.mock('../../contexts/LocaleContext', () => ({
  useLocale: () => ({ locale: 'en', setLocale: vi.fn() })
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })
const wrap = (node: ReactNode) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>{node}</App>
    </I18nProvider>
  )

const brand = (name: string) => {
  window.INSTANCE_BRANDING = {
    custom: true,
    name,
    logo_url: '',
    icon_url: '',
    hide_help_menu: false,
    splash_logo_src: '',
    favicon_src: '/console/neutral-icon.png',
    revision: 'r1'
  }
}

afterEach(() => {
  delete window.INSTANCE_BRANDING
  vi.restoreAllMocks()
})

describe('versionLabel', () => {
  it('names Notifuse by default and no product under a brand', () => {
    expect(versionLabel('45.0')).toBe('Notifuse v45.0')
    brand('Acme')
    expect(versionLabel('45.0')).toBe('v45.0')
  })
})

describe('withBrandName', () => {
  it('leaves the text alone by default', () => {
    expect(withBrandName('You are the Notifuse assistant.')).toBe('You are the Notifuse assistant.')
  })

  it('puts the brand in place of every Notifuse, a "$" included', () => {
    brand('A$&B')
    expect(withBrandName('Notifuse helps. Ask Notifuse.')).toBe('A$&B helps. Ask A$&B.')
  })

  it('reaches the email assistant prompt', () => {
    expect(buildEmailSystemPrompt({ tree: null })).toContain('the Notifuse endpoint')
    brand('Acme')
    const prompt = buildEmailSystemPrompt({ tree: null })
    expect(prompt).toContain('the Acme endpoint')
    expect(prompt).not.toMatch(/\bNotifuse\b/)
  })
})

describe('blog theme presets', () => {
  const footer = () => getThemePresets()[0].files['footer.liquid']

  it('keeps the Notifuse footer by default', () => {
    expect(footer()).toContain('Powered by')
    expect(footer()).toContain('https://x.com/notifuse')
    expect(footer()).toContain('https://github.com/Notifuse/notifuse')
  })

  it('drops every Notifuse mark under a brand and keeps the rest', () => {
    const before = getThemePresets()[0]
    brand('Acme')
    const after = getThemePresets()[0]

    expect(after.files['footer.liquid']).not.toMatch(/notifuse|Powered by/i)
    expect(after.files['footer.liquid']).toContain('{{ base_url }}/terms')
    expect(after.files['footer.liquid']).toContain('{{ base_url }}/feed.xml')
    expect(after.files['footer.liquid']).toContain('All rights reserved')
    for (const name of Object.keys(before.files) as (keyof typeof before.files)[]) {
      if (name !== 'footer.liquid') expect(after.files[name], name).toBe(before.files[name])
    }
  })
})

describe('update notices', () => {
  const data = {
    current: '44.0',
    latest: '45.0',
    update_available: true,
    behind: 1,
    security: false,
    upgrade_url: 'https://docs.notifuse.com/self-hosting/installation',
    releases: [
      {
        version: '45.0',
        released_at: '2026-12-01',
        security: false,
        security_note: '',
        summary: 'Summary.',
        highlights: [],
        before_upgrade: [],
        changelog_url: 'https://github.com/Notifuse/notifuse/blob/main/CHANGELOG.md'
      }
    ]
  }

  it('names the brand in the card, which stays shown', () => {
    brand('Acme')
    wrap(<UpdateCard data={data} onOpen={() => {}} onIgnore={() => {}} />)
    expect(screen.getByText('Acme 45.0 is available')).toBeInTheDocument()
  })

  it('prints the version without a product name in the footer', () => {
    brand('Acme')
    wrap(<UpdateFooter version="44.0" dismissedLatest={null} onOpen={() => {}} />)
    expect(screen.getByText('v44.0')).toBeInTheDocument()
    expect(screen.queryByText(/Notifuse/)).toBeNull()
  })
})

describe('account menu version', () => {
  it.each([
    [false, 'Notifuse v45.0'],
    [true, 'v45.0']
  ])('custom brand %s shows %s', async (custom, label) => {
    window.VERSION = '45.0'
    if (custom) brand('Acme')
    wrap(<AccountMenu user={{ email: 'ann@example.com' }} collapsed={false} onSignout={vi.fn()} getPopupContainer={() => document.body} />)
    fireEvent.click(screen.getByRole('button'))
    expect(await screen.findByText(label)).toBeInTheDocument()
  })
})

describe('recovery codes', () => {
  const download = () => {
    const names: string[] = []
    const files: Blob[] = []
    vi.spyOn(URL, 'createObjectURL').mockImplementation((file) => {
      files.push(file as Blob)
      return 'blob:codes'
    })
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      names.push(this.download)
    })
    fireEvent.click(screen.getByText('Download .txt'))
    const text = () =>
      new Promise<string>((resolve) => {
        const reader = new FileReader()
        reader.onload = () => resolve(String(reader.result))
        reader.readAsText(files[0])
      })
    return { names, text }
  }

  it('keeps the Notifuse heading and file name by default', async () => {
    wrap(<RecoveryCodesList codes={['aaaa-bbbb']} email="ann@example.com" saved={false} onSavedChange={() => {}} />)
    const file = download()
    expect(file.names).toEqual(['notifuse-recovery-codes.txt'])
    expect(await file.text()).toMatch(/^Notifuse recovery codes for ann@example.com\n/)
  })

  it('names the brand in the heading and no product in the file name', async () => {
    brand('Acme')
    wrap(<RecoveryCodesList codes={['aaaa-bbbb']} email="ann@example.com" saved={false} onSavedChange={() => {}} />)
    const file = download()
    expect(file.names).toEqual(['recovery-codes.txt'])
    expect(await file.text()).toMatch(/^Acme recovery codes for ann@example.com\n/)
  })
})

describe('blog theme presets, read on every render', () => {
  it('hands back the same unbranded preset each time', () => {
    brand('Acme')
    expect(getThemePresets()[0]).toBe(getThemePresets()[0])
  })
})
