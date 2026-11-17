import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { UpdateCheckResponse, UpdateRelease } from '../../services/updates'
import { UpdateCard } from './UpdateCard'
import { UpdateDrawer } from './UpdateDrawer'
import { UpdateFooter } from './UpdateFooter'

i18n.loadAndActivate({ locale: 'en', messages: {} })
const wrap = (node: ReactNode) => render(<I18nProvider i18n={i18n}>{node}</I18nProvider>)

const release = (over: Partial<UpdateRelease>): UpdateRelease => ({
  version: '44.0',
  released_at: '2026-11-12',
  security: false,
  security_note: '',
  summary: 'Summary.',
  highlights: [],
  before_upgrade: [],
  changelog_url: 'https://github.com/Notifuse/notifuse/blob/main/CHANGELOG.md',
  ...over
})

const answer = (releases: UpdateRelease[], over: Partial<UpdateCheckResponse> = {}): UpdateCheckResponse => ({
  current: '42.0',
  latest: releases[0]?.version ?? '42.0',
  update_available: releases.length > 0,
  behind: releases.length,
  security: releases.some((r) => r.security),
  upgrade_url: 'https://docs.notifuse.com/self-hosting/installation',
  releases,
  ...over
})

describe('UpdateCard', () => {
  const twoReleases = answer([
    release({ version: '44.0', highlights: ['One', 'Two', 'Three'], before_upgrade: ['Set A'] }),
    release({ version: '43.0', before_upgrade: ['Set B'] })
  ])

  it('names the latest release, how far behind, the first two highlights and every warning', () => {
    wrap(<UpdateCard data={twoReleases} onOpen={() => {}} onIgnore={() => {}} />)
    expect(screen.getByText('Notifuse 44.0 is available')).toBeInTheDocument()
    expect(screen.getByText('2 versions behind')).toBeInTheDocument()
    expect(screen.getByText('One')).toBeInTheDocument()
    expect(screen.getByText('Two')).toBeInTheDocument()
    expect(screen.queryByText('Three')).not.toBeInTheDocument()
    expect(screen.getByText('2 points to read before upgrading')).toBeInTheDocument()
  })

  it('opens the panel', () => {
    const onOpen = vi.fn()
    wrap(<UpdateCard data={twoReleases} onOpen={onOpen} onIgnore={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: "See what's new" }))
    expect(onOpen).toHaveBeenCalled()
  })

  it('ignores the latest release only once confirmed', async () => {
    const onIgnore = vi.fn()
    wrap(<UpdateCard data={twoReleases} onOpen={() => {}} onIgnore={onIgnore} />)

    fireEvent.click(screen.getByRole('button', { name: 'Ignore this version' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
    expect(onIgnore).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Ignore this version' }))
    expect(await screen.findByText('Ignore 44.0?')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Ignore' }))
    await waitFor(() => expect(onIgnore).toHaveBeenCalledWith('44.0'))
  })

  it('a missing security release cannot be dismissed and says which release fixes it', () => {
    const data = answer([release({ version: '44.0' }), release({ version: '43.1', security: true, security_note: 'Fix.' })])
    wrap(<UpdateCard data={data} onOpen={() => {}} onIgnore={() => {}} />)
    expect(screen.queryByRole('button', { name: 'Ignore this version' })).not.toBeInTheDocument()
    expect(screen.getByText('Includes a security fix (43.1)')).toBeInTheDocument()
  })
})

describe('UpdateDrawer', () => {
  it('lists the missing releases newest first, with warnings, security notes and https links only', () => {
    const data = answer([
      release({ version: '44.0', summary: 'Newest <b>bold</b>', before_upgrade: ['Do this first.'] }),
      release({ version: '43.1', security: true, security_note: 'A vulnerability is fixed.', changelog_url: 'javascript:alert(1)' })
    ])
    wrap(<UpdateDrawer open onClose={() => {}} data={data} />)

    const sections = screen.getAllByRole('region').filter((el) => ['44.0', '43.1'].includes(el.getAttribute('aria-label') ?? ''))
    expect(sections.map((s) => s.getAttribute('aria-label'))).toEqual(['44.0', '43.1'])

    // Texts from the update service are text, never HTML.
    expect(screen.getByText('Newest <b>bold</b>')).toBeInTheDocument()
    expect(within(sections[0]).getByText('Do this first.')).toBeInTheDocument()
    expect(within(sections[1]).getByText('A vulnerability is fixed.')).toBeInTheDocument()

    expect(within(sections[0]).getByRole('link', { name: /Full changelog/ })).toHaveAttribute('rel', 'noopener noreferrer')
    expect(within(sections[1]).queryByRole('link', { name: /Full changelog/ })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /How to upgrade/ })).toHaveAttribute(
      'href',
      'https://docs.notifuse.com/self-hosting/installation'
    )
    expect(screen.queryByRole('button', { name: /Ignore/ })).not.toBeInTheDocument()
  })
})

describe('UpdateFooter', () => {
  it('shows the version, and a way back once the card was dismissed', () => {
    const onOpen = vi.fn()
    const { rerender } = wrap(<UpdateFooter version="42.0" dismissedLatest={null} onOpen={onOpen} />)
    expect(screen.getByText('Notifuse v42.0')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()

    rerender(
      <I18nProvider i18n={i18n}>
        <UpdateFooter version="42.0" dismissedLatest="44.0" onOpen={onOpen} />
      </I18nProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: '44.0 available' }))
    expect(onOpen).toHaveBeenCalled()
  })
})
