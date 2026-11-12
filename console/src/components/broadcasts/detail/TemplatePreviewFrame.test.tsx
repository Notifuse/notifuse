import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TemplatePreviewFrame } from './TemplatePreviewFrame'
import { compileTemplatePreview, TemplatePreviewError } from '../../templates/compileTemplatePreview'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../templates/compileTemplatePreview', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../templates/compileTemplatePreview')>()),
  compileTemplatePreview: vi.fn()
}))

const compile = compileTemplatePreview as Mock
const workspace = { id: 'ws1', name: 'Acme', settings: {} } as unknown as Workspace
const template = {
  id: 'tpl-a',
  name: 'Alpha',
  version: 3,
  email: { subject: 'Hi', compiled_preview: '', visual_editor_tree: { id: 'root' } },
  test_data: { first_name: 'Ada' }
} as unknown as Template

const renderFrame = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TemplatePreviewFrame workspace={workspace} template={template} />
    </QueryClientProvider>
  )

describe('TemplatePreviewFrame', () => {
  const original = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth')
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
  })
  afterEach(() => {
    if (original) Object.defineProperty(HTMLElement.prototype, 'clientWidth', original)
  })

  it('compiles the template with its test data and renders it scaled to the box, sandboxed', async () => {
    compile.mockResolvedValue({ html: '<p>Hello Ada</p>', mjml: '' })
    renderFrame()
    const frame = (await screen.findByTitle('Preview of Alpha')) as HTMLIFrameElement
    expect(compile).toHaveBeenCalledWith({ workspace, email: template.email, testData: { first_name: 'Ada' } })
    expect(frame.getAttribute('srcdoc')).toBe('<p>Hello Ada</p>')
    expect(frame.getAttribute('sandbox')).toBe('')
    expect(frame.style.transform).toBe('scale(0.5)')
    expect(screen.getByTestId('template-preview-frame').style.height).toBe('450px')
  })

  it('says so when the template has no content', async () => {
    compile.mockRejectedValue(new TemplatePreviewError('missing_content'))
    renderFrame()
    expect(await screen.findByText('No content yet')).toBeInTheDocument()
  })

  it('says so when the compile fails or returns an MJML error', async () => {
    compile.mockResolvedValue({ html: '', mjml: '', error: { message: 'bad', details: [] } })
    renderFrame()
    expect(await screen.findByText("Couldn't render the preview")).toBeInTheDocument()
  })
})
