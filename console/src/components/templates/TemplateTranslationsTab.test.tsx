import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import TemplateTranslationsTab, { type TranslationEditorState } from './TemplateTranslationsTab'
import type { Entitlements } from '../../types/license'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'

// The editors are the heavy half of this tab and none of it is under test here.
vi.mock('../email_builder/EmailBuilder', () => ({ default: () => null }))
vi.mock('../email_builder/MjmlCodeEditor', () => ({ default: () => null }))
vi.mock('./PhonePreview', () => ({ default: () => null }))
vi.mock('../../services/api/template', () => ({ templatesApi: {} }))

const workspace = {
  id: 'ws1',
  settings: { languages: ['en', 'fr', 'de'], default_language: 'en' }
} as unknown as Workspace

const tree = { kind: 'root' } as unknown as EmailBlock

const licensedFor = (features: Entitlements['features']): Entitlements => ({
  tier: 'Studio',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 5,
  features,
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z'
})

const renderTab = (
  features: Entitlements['features'],
  translationsState: Record<string, TranslationEditorState>
) =>
  render(
    <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(features) }}>
      <TemplateTranslationsTab
        workspace={workspace}
        editorMode="visual"
        translationsState={translationsState}
        onTranslationsStateChange={vi.fn()}
        defaultSubject="Hello"
        defaultSubjectPreview=""
        defaultVisualEditorTree={tree}
        defaultMjmlSource=""
        onTestDataChange={vi.fn()}
        savedBlocks={[]}
        onSaveBlock={vi.fn()}
      />
    </LicenseContext.Provider>
  )

const switchFor = (language: string) =>
  screen.getByText(new RegExp(`\\(${language}\\)`)).closest('.ant-card')!.querySelector('button[role="switch"]') as HTMLButtonElement

/**
 * What is locked mirrors what the server's TranslationsWiden check refuses, no more: switching a
 * language on and editing what a translation says. Switching one off stays open, because a
 * deployment whose licence lapsed still owns the translations it made and must be able to take
 * them back — the server never gates a removal.
 */
describe('TemplateTranslationsTab under an unlicensed deployment', () => {
  const saved: TranslationEditorState = { enabled: true, subject: 'Bonjour', subjectPreview: '' }
  const unsaved: TranslationEditorState = { enabled: false, subject: '', subjectPreview: '' }

  it('locks switching a language on, and says what to buy', () => {
    renderTab(['rbac'], { fr: saved, de: unsaved })

    expect(
      screen.getByText('Template translations require a Notifuse Studio licence.')
    ).toBeInTheDocument()
    expect(switchFor('de')).toBeDisabled()
  })

  it('keeps a saved translation removable, and its content read-only', () => {
    renderTab(['rbac'], { fr: saved, de: unsaved })

    expect(switchFor('fr')).toBeEnabled()
    expect(screen.getByDisplayValue('Bonjour')).toBeDisabled()
    expect(screen.getByRole('button', { name: /open email editor/i })).toBeDisabled()
  })

  it('locks nothing when the licence covers translations', () => {
    renderTab(['template_i18n'], { fr: saved, de: unsaved })

    expect(screen.queryByText(/requires? a Notifuse/i)).not.toBeInTheDocument()
    expect(switchFor('de')).toBeEnabled()
    expect(screen.getByDisplayValue('Bonjour')).toBeEnabled()
  })
})
