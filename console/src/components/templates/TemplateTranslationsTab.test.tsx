import React, { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react'
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
    library?: Partial<React.ComponentProps<typeof TemplateTranslationsTab>>
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
        {...options.library}
      />
    </LicenseContext.Provider>
  )

// The language list names each switch after its language, in the console's language
const NAMES: Record<string, string> = { fr: 'French', de: 'German' }
const switchFor = (language: string) => screen.getByRole('switch', { name: `Translate into ${NAMES[language]}` })
// A row of the language list, which shows its language in the detail
const pick = (language: string) => fireEvent.click(screen.getByRole('button', { name: new RegExp(NAMES[language]) }))

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

describe('TemplateTranslationsTab saved blocks', () => {
  it('hands the library management to the builder', async () => {
    const onRenameSavedBlock = vi.fn()
    const onDeleteSavedBlock = vi.fn()
    renderTab(
      ['template_i18n'],
      { fr: { enabled: true, subject: 'Bonjour', subjectPreview: '', visualEditorTree: tree } },
      { library: { canManageSavedBlocks: true, onRenameSavedBlock, onDeleteSavedBlock } }
    )
    fireEvent.click(screen.getByText('Open email editor'))
    await waitFor(() => expect(builder.props).toBeDefined())
    expect(builder.props).toMatchObject({
      canManageSavedBlocks: true,
      onRenameSavedBlock,
      onDeleteSavedBlock
    })
  })
})

describe('TemplateTranslationsTab import and export', () => {
  interface ImportExportProps {
    onImport: (tree: EmailBlock) => void
    tree: EmailBlock
    templateName?: string
  }
  const importExport = () => (builder.props?.toolbarActions as React.ReactElement<ImportExportProps>).props
  const frenchTree = { kind: 'french' } as unknown as EmailBlock
  const imported = { kind: 'imported' } as unknown as EmailBlock

  it("imports into the open language's tree only, leaving the default language and the others alone", async () => {
    const onTranslationsStateChange = vi.fn()
    const de: TranslationEditorState = { enabled: true, subject: 'Hallo', subjectPreview: '', visualEditorTree: tree }
    renderTab(
      ['template_i18n'],
      { fr: { enabled: true, subject: 'Bonjour', subjectPreview: '', visualEditorTree: frenchTree }, de },
      { onTranslationsStateChange, library: { templateName: 'Welcome' } }
    )
    fireEvent.click(screen.getAllByText('Open email editor')[0])
    await waitFor(() => expect(builder.props?.toolbarActions).toBeDefined())

    act(() => importExport().onImport(imported))

    const next = onTranslationsStateChange.mock.calls.at(-1)![0] as Record<string, TranslationEditorState>
    expect(next.fr).toMatchObject({ subject: 'Bonjour', visualEditorTree: imported })
    expect(next.de).toEqual(de)
    // The default language is not part of the translations: only its own editor changes it
    expect(Object.keys(next).sort()).toEqual(['de', 'fr'])
  })

  it("exports the translation's tree, named after the template and the language", async () => {
    renderTab(
      ['template_i18n'],
      { fr: { enabled: true, subject: 'Bonjour', subjectPreview: '', visualEditorTree: frenchTree } },
      { library: { templateName: 'Welcome' } }
    )
    fireEvent.click(screen.getByText('Open email editor'))
    await waitFor(() => expect(builder.props?.toolbarActions).toBeDefined())

    expect(importExport().tree).toBe(frenchTree)
    expect(importExport().templateName).toBe('Welcome (fr)')
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

    // Reset is in the header of the language's card, behind a confirmation
    fireEvent.click(screen.getByRole('button', { name: 'Reset' }))
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

    // One language at a time in the detail: fr first, the first switched on, then de
    expect(screen.getByText('Text version: Custom')).toBeInTheDocument()
    pick('de')
    expect(screen.getByText('Text version: Automatic')).toBeInTheDocument()
  })
})

describe('TemplateTranslationsTab change made from an earlier render', () => {
  it('keeps the edits made since, when a late text arrives', async () => {
    const changes = vi.fn()
    const Host = () => {
      const [state, setState] = useState<Record<string, TranslationEditorState>>({
        fr: { enabled: true, subject: 'Bonjour', subjectPreview: '', visualEditorTree: tree }
      })
      return (
        <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(['template_i18n']) }}>
          <TemplateTranslationsTab
            workspace={workspace}
            editorMode="visual"
            translationsState={state}
            onTranslationsStateChange={(next) => {
              changes(next)
              setState(next)
            }}
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
    }
    render(<Host />)
    fireEvent.click(screen.getByText('Open email editor'))
    await waitFor(() => expect(builder.props).toBeDefined())
    // A seed keeps the handlers of the render that started it.
    const startedWith = (builder.props?.textVersion as { custom: { onChange: (v?: string) => void } }).custom

    const changedTree = { kind: 'root', edited: true } as unknown as EmailBlock
    act(() => (builder.props?.onTreeChange as (t: EmailBlock) => void)(changedTree))
    act(() => startedWith.onChange('Late seed'))

    expect(changes.mock.calls.at(-1)![0].fr).toMatchObject({ text: 'Late seed', visualEditorTree: changedTree })
  })
})

describe('TemplateTranslationsTab master-detail', () => {
  const on = (subject: string): TranslationEditorState => ({ enabled: true, subject, subjectPreview: '' })
  const off: TranslationEditorState = { enabled: false, subject: '', subjectPreview: '' }

  it('lists the default language and each translation language with its named switch', () => {
    renderTab(['template_i18n'], { fr: on('Bonjour'), de: off })

    const list = screen.getByText('Workspace languages').parentElement as HTMLElement
    expect(list).toHaveTextContent('English')
    expect(list).toHaveTextContent('Default')
    expect(switchFor('fr')).toBeChecked()
    expect(switchFor('de')).not.toBeChecked()
  })

  it('shows the first language switched on, and another one once picked', () => {
    renderTab(['template_i18n'], { fr: off, de: on('Hallo') })

    expect(screen.getByText('German translation')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Hallo')).toBeInTheDocument()
    pick('fr')
    expect(screen.getByText('French translation')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('Hallo')).not.toBeInTheDocument()
  })

  it('puts the default language text beside each field that translates it', () => {
    renderTab(['template_i18n'], { fr: on('Bonjour') })

    const source = screen.getByText('Subject · English').parentElement as HTMLElement
    expect(source).toHaveTextContent('Hello')
    expect(screen.getByText('Subject preview · English').parentElement).toHaveTextContent('—')
  })

  it('switches a language on or off without changing the one shown', () => {
    const onChange = vi.fn()
    renderTab(['template_i18n'], { fr: on('Bonjour'), de: off }, { onTranslationsStateChange: onChange })

    fireEvent.click(switchFor('de'))
    expect(onChange.mock.calls[0][0].de.enabled).toBe(true)
    expect(screen.getByText('French translation')).toBeInTheDocument()
  })

  it('keeps the language shown when another one is switched on before any pick', () => {
    const Host = () => {
      const [state, setState] = useState<Record<string, TranslationEditorState>>({ fr: off, de: off })
      return (
        <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(['template_i18n']) }}>
          <TemplateTranslationsTab
            workspace={workspace}
            editorMode="visual"
            translationsState={state}
            onTranslationsStateChange={setState}
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
    }
    render(<Host />)

    expect(screen.getByText('French translation')).toBeInTheDocument()
    fireEvent.click(switchFor('de'))
    // de is now the first switched on, yet the detail stays on fr
    expect(screen.getByText('French translation')).toBeInTheDocument()
  })

  it('lists the default language once in a workspace that names none', () => {
    render(
      <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(['template_i18n']) }}>
        <TemplateTranslationsTab
          workspace={{ id: 'ws1', settings: { languages: ['en', 'fr'] } } as unknown as Workspace}
          editorMode="visual"
          translationsState={{ fr: on('Bonjour') }}
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

    expect(screen.queryByRole('switch', { name: 'Translate into English' })).not.toBeInTheDocument()
    expect(switchFor('fr')).toBeInTheDocument()
  })

  it('offers to enable a language switched off, from its detail', () => {
    const onChange = vi.fn()
    renderTab(['template_i18n'], { fr: on('Bonjour'), de: off }, { onTranslationsStateChange: onChange })

    pick('de')
    expect(screen.getByText('Enable this translation to edit it.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Enable' }))
    expect(onChange.mock.calls[0][0].de).toMatchObject({ enabled: true, subject: 'Hello' })
  })

  it('locks enabling, reset and the editor without the licence, and points to its notice', () => {
    renderTab(['rbac'], { fr: on('Bonjour'), de: off })

    expect(screen.getByRole('button', { name: 'Reset' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Open email editor' })).toBeDisabled()
    pick('de')
    expect(screen.getByText('Your licence does not cover translations: see the notice above.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Enable' })).toBeDisabled()
  })
})

describe('TemplateTranslationsTab after a save refused for a missing field', () => {
  const fr: TranslationEditorState = { enabled: true, subject: 'Bonjour', subjectPreview: 'Aperçu' }
  const de: TranslationEditorState = { enabled: true, subject: 'Hallo', subjectPreview: '' }

  it('shows the language the save was refused for, with its empty field marked', () => {
    // The save used to leave a red dot on the step and a short toast: nothing said which field
    // of which language was missing, and the detail stayed on another language.
    const { rerender } = renderTab(['template_i18n'], { fr, de })
    expect(screen.getByRole('heading', { name: 'French translation' })).toBeInTheDocument()
    expect(screen.queryByText('Enter the German preview text')).toBeNull()

    rerender(
      <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(['template_i18n']) }}>
        <TemplateTranslationsTab
          workspace={workspace}
          editorMode="visual"
          translationsState={{ fr, de }}
          onTranslationsStateChange={vi.fn()}
          defaultSubject="Hello"
          defaultSubjectPreview=""
          defaultVisualEditorTree={tree}
          defaultMjmlSource=""
          onTestDataChange={vi.fn()}
          savedBlocks={[]}
          onSaveBlock={vi.fn()}
          missing={{ lang: 'de', attempt: 1 }}
        />
      </LicenseContext.Provider>
    )

    expect(screen.getByRole('heading', { name: 'German translation' })).toBeInTheDocument()
    expect(screen.getByText('Enter the German preview text')).toBeInTheDocument()
    expect(screen.queryByText('Enter the German subject')).toBeNull()
    expect(screen.getByLabelText('Subject preview').closest('.ant-input-status-error, .ant-input-outlined')).toHaveClass('ant-input-status-error')
  })
})
