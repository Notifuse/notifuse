import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import TemplateTranslationsTab, { type TranslationEditorState } from './TemplateTranslationsTab'
import type { Entitlements } from '../../types/license'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'

// The editors are the heavy half of this tab and none of it is under test here.
// The builder stub records its last props, so a test can call the onCompile it is given.
const builder = vi.hoisted(() => ({
  props: undefined as Record<string, unknown> | undefined,
  compile: vi.fn()
}))
vi.mock('../email_builder/EmailBuilder', () => ({
  default: (props: Record<string, unknown>) => {
    builder.props = props
    return null
  }
}))
vi.mock('../email_builder/MjmlCodeEditor', () => ({ default: () => null }))
vi.mock('./PhonePreview', () => ({ default: () => null }))
vi.mock('../../services/api/template', () => ({ templatesApi: { compile: builder.compile } }))

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
  translationsState: Record<string, TranslationEditorState>,
  options: {
    onTranslationsStateChange?: (state: Record<string, TranslationEditorState>) => void
    category?: string
  } = {}
) =>
  render(
    <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(features) }}>
      <TemplateTranslationsTab
        workspace={workspace}
        editorMode="visual"
        translationsState={translationsState}
        onTranslationsStateChange={options.onTranslationsStateChange ?? vi.fn()}
        defaultSubject="Hello"
        defaultSubjectPreview=""
        defaultVisualEditorTree={tree}
        defaultMjmlSource=""
        onTestDataChange={vi.fn()}
        savedBlocks={[]}
        onSaveBlock={vi.fn()}
        category={options.category}
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

describe('TemplateTranslationsTab text version', () => {
  it('turns on the text version mode and hands the compiled text to the builder', async () => {
    builder.compile.mockResolvedValue({ html: '<p>Bonjour</p>', mjml: '<mjml></mjml>', text: 'Bonjour\n' })
    renderTab(['template_i18n'], {
      fr: { enabled: true, subject: 'Bonjour', subjectPreview: '', visualEditorTree: tree }
    })

    fireEvent.click(screen.getByText('Open email editor'))
    await waitFor(() => expect(builder.props).toBeDefined())

    expect(builder.props?.textVersion).toMatchObject({ enabled: true })
    const onCompile = builder.props?.onCompile as (tree: unknown) => Promise<{ text?: string }>
    await expect(onCompile(tree)).resolves.toMatchObject({ html: '<p>Bonjour</p>', text: 'Bonjour\n' })
  })
})

describe('TemplateTranslationsTab custom text version', () => {
  const french = (text?: string): TranslationEditorState => ({
    enabled: true,
    subject: 'Bonjour',
    subjectPreview: '',
    visualEditorTree: tree,
    text
  })

  type Custom = {
    value?: string
    onChange: (value?: string) => void
    category?: string
    isTranslation?: boolean
    disabled?: boolean
  }
  const custom = () => (builder.props?.textVersion as { custom: Custom }).custom

  const openEditor = async () => {
    fireEvent.click(screen.getByText('Open email editor'))
    await waitFor(() => expect(builder.props).toBeDefined())
  }

  it('hands the text of the translation to its editor, as a translation of its category', async () => {
    renderTab(['template_i18n'], { fr: french('Bonjour {{ contact.first_name }}') }, { category: 'marketing' })
    await openEditor()

    expect(custom()).toMatchObject({
      value: 'Bonjour {{ contact.first_name }}',
      category: 'marketing',
      isTranslation: true,
      disabled: false
    })
  })

  it('stores the text written in the editor on the translation', async () => {
    const onChange = vi.fn()
    renderTab(['template_i18n'], { fr: french(undefined) }, { onTranslationsStateChange: onChange })
    await openEditor()

    custom().onChange('Salut')
    expect(onChange).toHaveBeenLastCalledWith({ fr: expect.objectContaining({ text: 'Salut' }) })
  })

  it('does not copy any text when a language is switched on', () => {
    const onChange = vi.fn()
    renderTab(['template_i18n'], { fr: french(undefined) }, { onTranslationsStateChange: onChange })

    fireEvent.click(switchFor('de'))
    const de = onChange.mock.calls[0][0].de as TranslationEditorState
    expect(de.enabled).toBe(true)
    expect(de.text).toBeUndefined()
  })

  it('keeps the text of a language switched off and on again', () => {
    const onChange = vi.fn()
    renderTab(
      ['template_i18n'],
      { fr: french(undefined), de: { enabled: false, subject: '', subjectPreview: '', text: 'Hallo' } },
      { onTranslationsStateChange: onChange }
    )

    fireEvent.click(switchFor('de'))
    expect(onChange.mock.calls[0][0].de).toEqual({
      enabled: true,
      subject: '',
      subjectPreview: '',
      text: 'Hallo'
    })
  })

  it('removes the text when the translation is reset', async () => {
    const onChange = vi.fn()
    renderTab(['template_i18n'], { fr: french('Salut') }, { onTranslationsStateChange: onChange })

    const card = screen.getByText(/\(fr\)/).closest('.ant-card') as HTMLElement
    const buttons = card.querySelectorAll('.ant-space-compact button')
    fireEvent.click(buttons[buttons.length - 1])
    fireEvent.click(await screen.findByText('Reset'))
    const confirm = await screen.findAllByRole('button', { name: 'Reset' })
    fireEvent.click(confirm[confirm.length - 1])

    await waitFor(() => expect(onChange).toHaveBeenCalled())
    const fr = onChange.mock.calls[0][0].fr as TranslationEditorState
    expect(fr).toHaveProperty('text', undefined)
  })

  it('shows on the card which text version the language sends', () => {
    renderTab(['template_i18n'], {
      fr: french('Salut'),
      de: { enabled: true, subject: 'Hallo', subjectPreview: '' }
    })

    const card = (language: string) =>
      screen.getByText(new RegExp(`\\(${language}\\)`)).closest('.ant-card') as HTMLElement
    expect(card('fr')).toHaveTextContent('Text version: Custom')
    expect(card('de')).toHaveTextContent('Text version: Automatic')
  })
})
