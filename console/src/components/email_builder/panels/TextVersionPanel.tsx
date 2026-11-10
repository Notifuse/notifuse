import React, { useState } from 'react'
import { Alert, Button, Input, Popconfirm, Radio, Spin, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'

const { Text } = Typography

// Same checkered canvas as the edit panel.
const CANVAS_BACKGROUND =
  'url("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAAXNSR0IArs4c6QAAAERlWElmTU0AKgAAAAgAAYdpAAQAAAABAAAAGgAAAAAAA6ABAAMAAAABAAEAAKACAAQAAAABAAAACqADAAQAAAABAAAACgAAAAA7eLj1AAAAK0lEQVQYGWP8DwQMaODZs2doIgwMTBgiOAQGUCELNodLSUlhuHQA3Ui01QDcPgnEE5wAOwAAAABJRU5ErkJggg==")'

const MONOSPACE = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'

// Liquid variables shown as written, outside the translated sentences.
const UNSUBSCRIBE_VARIABLE = '{{ unsubscribe_url }}'
const CONFIRM_VARIABLE = '{{ confirm_subscription_url }}'

// What a "start again from the automatic version" compile returns.
export interface TextVersionSeed {
  // The automatic version with Liquid kept; undefined when it could not be generated.
  text?: string
  // Why it could not be generated.
  error?: string
  // A Liquid tag of the starting point that would be refused at save.
  textError?: string
  // Liquid tags of the email that the starting point does not hold (conditions and loops
  // written between blocks).
  lostLiquid?: boolean
}

export interface CustomTextVersion {
  // The custom text as written; undefined for the automatic version.
  value?: string
  onChange: (value?: string) => void
  onSeed: () => Promise<TextVersionSeed>
  // Template category: decides which links the text is reminded to keep.
  category?: string
  // The text of a translation rather than of the default language.
  isTranslation?: boolean
  disabled?: boolean
}

interface TextVersionPanelProps {
  // The plain-text version returned by the compile endpoint; "" or undefined when the
  // email holds no text and is sent as HTML only.
  text?: string
  errors?: Array<Record<string, unknown>>
  // The compile is running: the automatic version is not known yet.
  loading?: boolean
  // When set, the text can be written by hand instead of generated.
  custom?: CustomTextVersion
  // A narrow pane (the code editor): the explanation stacks above the text.
  compact?: boolean
}

// TextVersionPanel shows the plain-text version sent with the email, in the editor's own
// frame: an explanation on the left, the text on the canvas. With `custom`, it also lets
// the user write the text, starting from the automatic version.
export const TextVersionPanel: React.FC<TextVersionPanelProps> = ({
  text,
  errors,
  loading,
  custom,
  compact
}) => {
  const { t } = useLingui()
  const [seeding, setSeeding] = useState(false)
  const [seedResult, setSeedResult] = useState<TextVersionSeed | null>(null)

  const isCustom = custom?.value !== undefined
  const hasCustomContent = !!custom?.value && custom.value.trim() !== ''
  const disabled = !!custom?.disabled

  const seed = async () => {
    if (!custom) return
    setSeeding(true)
    setSeedResult(null)
    try {
      const result = await custom.onSeed()
      setSeedResult(result)
      if (result.text !== undefined && !result.error) {
        custom.onChange(result.text)
      }
    } catch (error) {
      setSeedResult({ error: (error as Error).message })
    } finally {
      setSeeding(false)
    }
  }

  const chooseCustom = () => {
    if (!custom || isCustom) return
    custom.onChange('')
    void seed()
  }

  const chooseAutomatic = () => {
    if (!custom || !isCustom) return
    setSeedResult(null)
    custom.onChange(undefined)
  }

  const intro = (
    <>
      <div className="pt-4 px-6 text-gray-900 text-sm font-bold">{t`Text version`}</div>
      <div className="px-6 pt-3 text-sm text-gray-600">
        {t`Every email is sent with an HTML version and a text version. Mail clients show the text version when HTML is blocked or not supported.`}
      </div>
      {!custom && (
        <div className="px-6 pt-3 text-sm text-gray-600">
          {t`It is generated from the HTML at every send, so there is nothing to maintain. Its links are the links of the HTML, tracked the same way.`}
        </div>
      )}
    </>
  )

  const modeCards = custom && (
    <div className="px-6 pt-4 flex flex-col gap-3">
      <Popconfirm
        title={t`Use the automatic version?`}
        description={t`The text you wrote will be removed.`}
        okText={t`Use the automatic version`}
        cancelText={t`Cancel`}
        onConfirm={chooseAutomatic}
        disabled={disabled || !isCustom || !hasCustomContent}
      >
        <label
          className={`block rounded-lg border bg-white px-4 py-3 ${!isCustom ? 'border-indigo-500' : 'border-gray-200'}`}
          onClick={(e) => {
            // Without text to lose there is nothing to confirm.
            if (!disabled && isCustom && !hasCustomContent) {
              e.preventDefault()
              chooseAutomatic()
            } else if (isCustom && hasCustomContent) {
              e.preventDefault()
            }
          }}
        >
          <Radio checked={!isCustom} disabled={disabled}>
            <span className="font-medium">{t`Automatic`}</span>
          </Radio>
          <div className="pl-6 pt-1 text-xs text-gray-500">
            {t`Generated from the HTML at every send. Nothing to maintain.`}
          </div>
        </label>
      </Popconfirm>
      <label
        className={`block rounded-lg border bg-white px-4 py-3 ${isCustom ? 'border-indigo-500' : 'border-gray-200'}`}
      >
        <Radio checked={isCustom} disabled={disabled} onChange={chooseCustom}>
          <span className="font-medium">{t`Custom`}</span>
        </Radio>
        <div className="pl-6 pt-1 text-xs text-gray-500">
          {t`Written by hand, with the templating markup.`}
        </div>
      </label>
    </div>
  )

  const customNotes = custom && (
    <div className="px-6 pt-4 pb-6 flex flex-col gap-3">
      {isCustom && (
        <Popconfirm
          title={t`Start again from the automatic version?`}
          description={t`The text you wrote will be replaced.`}
          okText={t`Replace`}
          cancelText={t`Cancel`}
          onConfirm={() => void seed()}
          disabled={disabled || seeding || !hasCustomContent}
        >
          <Button
            block
            disabled={disabled || seeding}
            loading={seeding}
            onClick={() => {
              if (!hasCustomContent) void seed()
            }}
          >
            {t`Start again from the automatic version`}
          </Button>
        </Popconfirm>
      )}
      {isCustom && (
        <Alert
          type="warning"
          showIcon
          message={
            <div className="text-sm flex flex-col gap-1">
              {custom.category === 'marketing' && (
                <span>{t`Keep the unsubscribe link: ${UNSUBSCRIBE_VARIABLE}.`}</span>
              )}
              {custom.category === 'opt_in' && (
                <span>{t`Keep the confirmation link: ${CONFIRM_VARIABLE}.`}</span>
              )}
              <span>{t`Links in a custom text are not tracked.`}</span>
              <span>{t`This text is not updated when you change the email.`}</span>
            </div>
          }
        />
      )}
      <Alert
        type="info"
        showIcon
        message={
          <span className="text-sm">
            {custom.isTranslation
              ? t`Without its own text, this language receives the automatic version.`
              : t`This text is for the default language. Each translation has its own text in the Translations tab, otherwise it receives the automatic version.`}
          </span>
        }
      />
    </div>
  )

  const seedNotices = seedResult && (
    <div className="flex flex-col gap-2 mb-3">
      {seedResult.error && (
        <Alert
          type="error"
          showIcon
          message={t`The automatic version could not be generated: ${seedResult.error}`}
        />
      )}
      {seedResult.lostLiquid && !seedResult.error && (
        <Alert
          type="warning"
          showIcon
          message={t`Conditions and loops placed between blocks are not carried over. Check the text before saving.`}
        />
      )}
      {seedResult.textError && !seedResult.error && (
        <Alert
          type="warning"
          showIcon
          message={t`Some templating markup would not be accepted: ${seedResult.textError}`}
        />
      )}
    </div>
  )

  const hasErrors = !!errors && errors.length > 0
  const hasText = !!text && text.trim() !== ''

  const automaticPreview = (
    <>
      <div className="mb-2">
        <Text type="secondary" className="text-xs">
          {t`Preview with test data`}
        </Text>
      </div>
      <div
        className="bg-white rounded border border-gray-200 shadow-sm"
        style={{ padding: '24px 28px', minHeight: 240 }}
      >
        {hasErrors ? (
          <div className="bg-amber-50 border border-amber-200 rounded-lg p-4">
            <h3 className="text-sm font-medium text-amber-800 mb-2">
              {t`The email could not be compiled`}
            </h3>
            <ul className="text-sm text-amber-700 space-y-1">
              {errors.map((error, index) => (
                <li key={index}>
                  {typeof error.message === 'string' ? error.message : String(error)}
                </li>
              ))}
            </ul>
          </div>
        ) : loading ? (
          <div className="flex justify-center py-12">
            <Spin />
          </div>
        ) : hasText ? (
          <pre
            data-testid="text-version"
            className="m-0 text-sm text-gray-800"
            style={{
              fontFamily: MONOSPACE,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              lineHeight: 1.6
            }}
          >
            {text}
          </pre>
        ) : (
          <Text type="secondary">{t`No text version: this email will be sent as HTML only.`}</Text>
        )}
      </div>
    </>
  )

  const customEditor = (
    <>
      <div className="mb-2 flex items-center justify-between">
        <Text type="secondary" className="text-xs">
          {t`Text sent, as you write it`}
        </Text>
        <Tag color="purple" className="mr-0">
          {t`Custom`}
        </Tag>
      </div>
      {seedNotices}
      <Input.TextArea
        data-testid="custom-text-input"
        value={custom?.value ?? ''}
        onChange={(e) => custom?.onChange(e.target.value)}
        disabled={disabled || seeding}
        autoSize={{ minRows: 16 }}
        spellCheck={false}
        style={{ fontFamily: MONOSPACE, fontSize: 13, lineHeight: 1.6, padding: '20px 24px' }}
      />
    </>
  )

  const canvas = (
    <div className="mx-auto my-8 w-full" style={{ maxWidth: 640 }}>
      {isCustom ? customEditor : automaticPreview}
    </div>
  )

  if (compact) {
    return (
      <div className="flex flex-col flex-1 min-h-0 overflow-auto bg-gray-50">
        {intro}
        {modeCards}
        <div className="px-6">{canvas}</div>
        {customNotes}
      </div>
    )
  }

  return (
    <div className="flex flex-1 min-h-0">
      <div className="w-80 bg-gray-50 border-r border-gray-200 flex flex-col overflow-auto">
        {intro}
        {modeCards}
        {customNotes}
      </div>

      <div
        className="flex-1 flex flex-col min-w-0 overflow-auto px-6"
        style={{ background: CANVAS_BACKGROUND }}
      >
        {canvas}
      </div>
    </div>
  )
}
