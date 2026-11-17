import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { App as AntApp, Form } from 'antd'
import type { FormInstance } from 'antd'
import { TemplateSettingsStep, liquidTagProblem, type TemplateSettingsStepProps } from './TemplateSettingsStep'

function renderStep(props: Partial<TemplateSettingsStepProps> = {}) {
  const formRef: { current?: FormInstance } = {}
  const onDirty = vi.fn()
  const onEditorModeChange = vi.fn()
  function Harness({ onForm }: { onForm: (form: FormInstance) => void }) {
    const [form] = Form.useForm()
    useEffect(() => {
      onForm(form)
    }, [form, onForm])
    return (
      <AntApp>
        <Form form={form} layout="vertical">
          <TemplateSettingsStep
            isSaved={false}
            editorMode="visual"
            onEditorModeChange={onEditorModeChange}
            editorModeLocked={false}
            showLanguage={false}
            defaultLanguage="en"
            senders={[]}
            validateId={async () => undefined}
            onDirty={onDirty}
            onNameChange={vi.fn()}
            testData={{ contact: { first_name: 'John' }, order: { number: '1042' } }}
            {...props}
          />
        </Form>
      </AntApp>
    )
  }
  render(<Harness onForm={(form) => (formRef.current = form)} />)
  return { form: () => formRef.current!, onDirty, onEditorModeChange }
}

describe('TemplateSettingsStep', () => {
  it('groups the fields in an Identity card and an Envelope card', () => {
    renderStep()
    expect(screen.getByText('Identity')).toBeInTheDocument()
    expect(screen.getByText('Envelope')).toBeInTheDocument()
    expect(screen.getByText("Can't change after the template is created")).toBeInTheDocument()
  })

  it('locks the ID of a saved template and says so', () => {
    renderStep({ isSaved: true })
    expect(screen.getByPlaceholderText('i.e: welcome-email')).toBeDisabled()
    expect(screen.getByText('Can no longer change')).toBeInTheDocument()
  })

  it('picks the editor mode from its two cards, unless it is locked', async () => {
    const { onEditorModeChange } = renderStep()
    const code = screen.getByRole('radio', { name: /Code \(MJML\)/ })
    expect(screen.getByRole('radio', { name: /Visual/ })).toHaveAttribute('aria-checked', 'true')
    await userEvent.click(code)
    expect(onEditorModeChange).toHaveBeenCalledWith('code')
  })

  it('says why the editor mode of a saved template is locked', () => {
    // The other mode was only greyed out, with no reason next to it.
    renderStep({ editorModeLocked: true, isSaved: true })
    expect(screen.getByText('Fixed once the template is saved')).toBeInTheDocument()
  })

  it('says why the editor mode of a copy is locked', () => {
    renderStep({ editorModeLocked: true, isSaved: false })
    expect(screen.getByText('Kept from the template or model it starts from')).toBeInTheDocument()
  })

  it('does not change a locked editor mode', async () => {
    const { onEditorModeChange } = renderStep({ editorModeLocked: true })
    expect(screen.getByRole('radio', { name: /Code \(MJML\)/ })).toBeDisabled()
    await userEvent.click(screen.getByRole('radio', { name: /Code \(MJML\)/ }))
    expect(onEditorModeChange).not.toHaveBeenCalled()
  })

  it('names the default language on the subject and preview text when there are translations', () => {
    renderStep({ showLanguage: true, defaultLanguage: 'fr' })
    expect(screen.getByText('Subject (fr)')).toBeInTheDocument()
    expect(screen.getByText('Preview text (fr)')).toBeInTheDocument()
    expect(screen.getByText(/Default language:/)).toBeInTheDocument()
  })

  it('inserts a variable of the template data where the cursor is in the subject', async () => {
    const { form, onDirty } = renderStep()
    const subject = screen.getByRole('textbox', { name: 'Subject' }) as HTMLInputElement
    await userEvent.type(subject, 'Hello !')
    subject.setSelectionRange(6, 6)

    await userEvent.click(screen.getByRole('button', { name: 'Insert a variable' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: '{{ contact.first_name }}' }))

    await waitFor(() => expect(form().getFieldValue(['email', 'subject'])).toBe('Hello {{ contact.first_name }}!'))
    expect(onDirty).toHaveBeenCalled()
  })
})

describe('liquidTagProblem', () => {
  it('accepts balanced tags and an empty text', () => {
    expect(liquidTagProblem('Hi {{ contact.first_name }}')).toBeUndefined()
    expect(liquidTagProblem('')).toBeUndefined()
    expect(liquidTagProblem(undefined)).toBeUndefined()
  })

  it('counts the tags of a text where one is left open', () => {
    expect(liquidTagProblem('Hi {{ name')).toEqual({ kind: 'count', opening: 1, closing: 0 })
  })

  it('reports a tag closed before it opens', () => {
    expect(liquidTagProblem('}} a {{')).toEqual({ kind: 'stray-close' })
  })
})

it('explains an unbalanced subject in the form', async () => {
  const { form } = renderStep()
  await userEvent.type(screen.getByRole('textbox', { name: 'Subject' }), 'Hi {{{{ name') // user-event types {{ as {
  await expect(form().validateFields([['email', 'subject']])).rejects.toBeDefined()
  expect(await screen.findByText(/Unclosed Liquid tag: 1 opening tags \(\{\{\) but 0 closing tags \(\}\}\)/)).toBeInTheDocument()
})
