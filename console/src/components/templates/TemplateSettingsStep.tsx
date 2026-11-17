import { useMemo, useRef } from 'react'
import { ConfigProvider, Dropdown, Form, Input, Select, Tooltip } from 'antd'
import type { InputRef } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Braces, Check, Code, IdCard, Mail, Sparkles } from 'lucide-react'
import { Panel } from '../layout'
import { TEMPLATE_ID_MAX_LENGTH, TEMPLATE_NAME_MAX_LENGTH } from '../../constants/limits'
import { useLanguageName } from './useLanguageName'
import { TemplateCategoryTag } from './TemplateCategoryTag'
import { liquidVariablePaths } from './liquidVariablePaths'

// The fields sit in a grid whose gap spaces them, as in the newsletter composer's cards.
const FIELDS_THEME = { components: { Form: { itemMarginBottom: 0 } } }

const CATEGORY_OPTIONS = ['marketing', 'transactional', 'welcome', 'opt_in', 'unsubscribe', 'bounce', 'blocklist']

export type EditorMode = 'visual' | 'code'

export interface TemplateSettingsStepProps {
  /** Editing a saved template: its ID can no longer change. */
  isSaved: boolean
  /** The category is imposed by where the template is created (a newsletter, a list). */
  forceCategory?: string
  editorMode: EditorMode
  onEditorModeChange: (mode: EditorMode) => void
  /** The editor mode can't change: a saved template, or a copy keeping its source's mode. */
  editorModeLocked: boolean
  /** The workspace has other languages: subject and preview text name the default one. */
  showLanguage: boolean
  defaultLanguage?: string
  /** The senders of the provider the category sends with. */
  senders: { id: string; name: string; email: string }[]
  categoryValue?: string
  /** Rejects an ID that is already taken. */
  validateId: (value: string) => Promise<void>
  /** The template data, whose variables the subject's { } button inserts. */
  testData?: Record<string, unknown>
  /** Called when a field changes outside antd's own change events (a variable inserted). */
  onDirty: () => void
  /** The ID follows the name until the template is saved. */
  onNameChange: (name: string) => void
}

/** The Settings step of the template drawer: an Identity card and an Envelope card. */
export function TemplateSettingsStep({
  isSaved,
  forceCategory,
  editorMode,
  onEditorModeChange,
  editorModeLocked,
  showLanguage,
  defaultLanguage,
  senders,
  categoryValue,
  validateId,
  testData,
  onDirty,
  onNameChange
}: TemplateSettingsStepProps) {
  const { t } = useLingui()
  const form = Form.useFormInstance()
  const subjectRef = useRef<InputRef>(null)
  const variables = useMemo(() => liquidVariablePaths(testData), [testData])
  const nameOf = useLanguageName()
  const languageName = defaultLanguage ? nameOf(defaultLanguage) : ''
  const lang = defaultLanguage || ''

  const liquidRule = {
    validator: (_: unknown, value: string) => {
      const problem = liquidTagProblem(value)
      if (!problem) return Promise.resolve()
      let error: string
      if (problem.kind === 'count') {
        const { opening, closing } = problem
        error = t`Unclosed Liquid tag: ${opening} opening tags (${OPEN_TAG}) but ${closing} closing tags (${CLOSE_TAG})`
      } else if (problem.kind === 'stray-close') {
        error = t`A closing Liquid tag (${CLOSE_TAG}) comes before its opening tag (${OPEN_TAG})`
      } else {
        const { open } = problem
        error = t`${open} Liquid tags are not closed: each ${OPEN_TAG} needs a matching ${CLOSE_TAG}`
      }
      return Promise.reject(new Error(error))
    }
  }

  // Puts {{ path }} where the cursor is in the subject, or at its end.
  const insertVariable = (path: string) => {
    const current: string = form.getFieldValue(['email', 'subject']) || ''
    const input = subjectRef.current?.input
    const start = input?.selectionStart ?? current.length
    const end = input?.selectionEnd ?? current.length
    const token = `{{ ${path} }}`
    form.setFieldValue(['email', 'subject'], current.slice(0, start) + token + current.slice(end))
    void form.validateFields([['email', 'subject']])
    onDirty()
    requestAnimationFrame(() => {
      subjectRef.current?.focus()
      input?.setSelectionRange(start + token.length, start + token.length)
    })
  }

  const modes: { value: EditorMode; label: string; icon: React.ReactNode }[] = [
    { value: 'visual', label: t`Visual`, icon: <Sparkles size={14} aria-hidden="true" /> },
    { value: 'code', label: t`Code (MJML)`, icon: <Code size={14} aria-hidden="true" /> }
  ]

  return (
    <ConfigProvider theme={FIELDS_THEME}>
      <div className="flex flex-col gap-3">
        <Panel title={t`Identity`} icon={<IdCard size={14} />}>
          <div className="grid grid-cols-2 gap-3">
            <Form.Item
              name="name"
              label={t`Template name`}
              rules={[
                { required: true },
                {
                  max: TEMPLATE_NAME_MAX_LENGTH,
                  message: t`Template name must be at most ${TEMPLATE_NAME_MAX_LENGTH} characters long`
                }
              ]}
            >
              <Input
                placeholder={t`i.e: Welcome Email`}
                maxLength={TEMPLATE_NAME_MAX_LENGTH}
                showCount
                onChange={(e) => onNameChange(e.target.value)}
              />
            </Form.Item>
            <Form.Item
              name="id"
              label={t`Template ID (utm_content)`}
              tooltip={t`This is the ID that will be used as the utm_content parameter in the links URL to track the template`}
              extra={isSaved ? t`Can no longer change` : t`Can't change after the template is created`}
              rules={[
                {
                  required: true,
                  type: 'string',
                  pattern: /^[a-z0-9_-]+$/,
                  message: t`ID must contain only lowercase letters, numbers, underscores, and hyphens`
                },
                {
                  max: TEMPLATE_ID_MAX_LENGTH,
                  message: t`Template ID must be at most ${TEMPLATE_ID_MAX_LENGTH} characters long`
                },
                { validator: (_rule, value) => validateId(value) }
              ]}
            >
              <Input
                disabled={isSaved}
                placeholder={t`i.e: welcome-email`}
                maxLength={TEMPLATE_ID_MAX_LENGTH}
                showCount
                className="font-mono"
              />
            </Form.Item>
            <Form.Item name="category" label={t`Category`} rules={[{ required: true, type: 'string' }]}>
              <Select
                placeholder={t`Select category`}
                disabled={!!forceCategory}
                options={CATEGORY_OPTIONS.map((value) => ({ value, label: <TemplateCategoryTag category={value} /> }))}
              />
            </Form.Item>
            <Form.Item
              label={t`Editor mode`}
              tooltip={t`The editor mode can't be changed after the template is created. To use the other editor, create a new template and copy the content with Import MJML or Export MJML.`}
              // A locked mode says why under the cards: the other one is greyed out, not hidden.
              extra={
                editorModeLocked
                  ? isSaved
                    ? t`Fixed once the template is saved`
                    : t`Kept from the template or model it starts from`
                  : undefined
              }
            >
              <div role="radiogroup" aria-label={t`Editor mode`} className="grid grid-cols-2 gap-2">
                {modes.map((mode) => {
                  const selected = editorMode === mode.value
                  return (
                    <button
                      key={mode.value}
                      type="button"
                      role="radio"
                      aria-checked={selected}
                      disabled={editorModeLocked}
                      onClick={() => onEditorModeChange(mode.value)}
                      className={`flex h-7 items-center gap-2 rounded-md border px-3 text-left text-base font-medium transition-colors focus-visible:outline-2 focus-visible:outline-primary-500 disabled:cursor-not-allowed ${
                        selected
                          ? 'border-primary-500 bg-surface text-ink ring-2 ring-primary-100'
                          : 'border-line bg-surface text-ink-2 hover:border-primary-300'
                      } ${editorModeLocked && !selected ? 'opacity-50' : ''}`}
                    >
                      <span className={selected ? 'flex text-primary-600' : 'flex text-ink-3'}>{mode.icon}</span>
                      {mode.label}
                      {selected ? <Check size={14} aria-hidden="true" className="ml-auto text-primary-600" /> : null}
                    </button>
                  )
                })}
              </div>
            </Form.Item>
          </div>
        </Panel>

        <Panel
          title={t`Envelope`}
          icon={<Mail size={14} />}
          extra={
            showLanguage && languageName ? (
              <span className="text-sm text-ink-3">{t`Default language: ${languageName}`}</span>
            ) : null
          }
        >
          <div className="grid grid-cols-2 gap-3">
            <Form.Item
              className="col-span-2"
              name={['email', 'subject']}
              label={showLanguage ? t`Subject (${lang})` : t`Subject`}
              extra={
                <span>
                  {t`Liquid accepted, for example`} <code>{'{{ contact.first_name }}'}</code>
                </span>
              }
              rules={[{ required: true, type: 'string' }, liquidRule]}
            >
              <Input
                ref={subjectRef}
                placeholder={t`Templating markup allowed`}
                suffix={
                  <Dropdown
                    trigger={['click']}
                    disabled={variables.length === 0}
                    menu={{
                      items: variables.map((path) => ({ key: path, label: <code>{`{{ ${path} }}`}</code> })),
                      onClick: ({ key }) => insertVariable(key),
                      className: 'max-h-80 overflow-y-auto'
                    }}
                  >
                    <Tooltip title={t`Insert a variable`}>
                      <button
                        type="button"
                        aria-label={t`Insert a variable`}
                        className="flex cursor-pointer items-center text-ink-3 hover:text-ink"
                      >
                        <Braces size={14} aria-hidden="true" />
                      </button>
                    </Tooltip>
                  </Dropdown>
                }
              />
            </Form.Item>
            <Form.Item
              className="col-span-2"
              name={['email', 'subject_preview']}
              label={showLanguage ? t`Preview text (${lang})` : t`Preview text`}
              extra={t`Liquid accepted`}
              rules={[{ required: true, type: 'string' }, liquidRule]}
            >
              <Input placeholder={t`Templating markup allowed`} />
            </Form.Item>
            <Form.Item
              name={['email', 'sender_id']}
              label={t`Sender`}
              extra={
                categoryValue === 'marketing'
                  ? t`Custom sender, from the marketing email provider`
                  : t`Custom sender, from the transactional email provider`
              }
              rules={[{ required: false, type: 'string' }]}
            >
              <Select
                allowClear
                placeholder={t`Provider's default sender`}
                options={senders.map((sender) => ({ value: sender.id, label: `${sender.name} <${sender.email}>` }))}
              />
            </Form.Item>
            <Form.Item name={['email', 'reply_to']} label={t`Reply to`} rules={[{ required: false, type: 'email' }]}>
              <Input />
            </Form.Item>
          </div>
        </Panel>
      </div>
    </ConfigProvider>
  )
}

// The Liquid delimiters named in the error messages, outside the translated sentences: a brace
// inside a message is ICU syntax.
const OPEN_TAG = '{{'
const CLOSE_TAG = '}}'

/** What is wrong with a text's Liquid output tags: every {{ needs its }}, in order. */
export type LiquidTagProblem =
  | { kind: 'count'; opening: number; closing: number }
  | { kind: 'stray-close' }
  | { kind: 'unclosed'; open: number }

// eslint-disable-next-line react-refresh/only-export-components -- validation tested on its own
export function liquidTagProblem(text: string | undefined): LiquidTagProblem | undefined {
  if (!text) return undefined
  const opening = (text.match(/\{\{/g) || []).length
  const closing = (text.match(/\}\}/g) || []).length
  if (opening !== closing) return { kind: 'count', opening, closing }
  let open = 0
  for (let i = 0; i < text.length - 1; ) {
    const pair = text.slice(i, i + 2)
    if (pair === '{{') {
      open++
      i += 2
    } else if (pair === '}}') {
      if (open === 0) return { kind: 'stray-close' }
      open--
      i += 2
    } else {
      i++
    }
  }
  return open > 0 ? { kind: 'unclosed', open } : undefined
}
