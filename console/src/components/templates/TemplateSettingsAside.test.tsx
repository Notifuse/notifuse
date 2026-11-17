import '../../__tests__/resizeObserverMock'
import { describe, expect, it, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { compileTemplatePreview } from './compileTemplatePreview'
import type { Workspace } from '../../services/api/types'
import { TemplateSettingsAside, type TemplateSettingsAsideProps } from './TemplateSettingsAside'

vi.mock('./PhonePreview', () => ({
  default: ({ sender, subject }: { sender: string; subject: string }) => (
    <div data-testid="phone-preview">
      {sender} · {subject}
    </div>
  )
}))
// The API client imports the router, and the router every page: without this, loading the real
// module in the factory below reaches the preview hook before the mock is in place.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))
// The preview's compile, as TemplatePreviewFrame's own tests stub it.
vi.mock('./compileTemplatePreview', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./compileTemplatePreview')>()),
  compileTemplatePreview: vi.fn()
}))

const workspace = { id: 'ws1', settings: {} } as unknown as Workspace
const tree = { id: 'root', type: 'mjml', children: [] }

function renderAside(props: Partial<TemplateSettingsAsideProps> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <TemplateSettingsAside
        workspace={workspace}
        email={{ subject: 'Welcome, {{ contact.first_name }}', subject_preview: 'Three things', editor_mode: 'visual', visual_editor_tree: tree as never }}
        testData={{ contact: { first_name: 'Camille', last_name: 'Laurent' } }}
        senderName="Acme"
        onOpenContent={vi.fn()}
        {...props}
      />
    </QueryClientProvider>
  )
}

describe('TemplateSettingsAside', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(compileTemplatePreview as Mock).mockResolvedValue({
      html: '<p>Hi</p>',
      mjml: '<mjml/>',
      subject: 'Welcome, Camille',
      subject_preview: 'Three things'
    })
  })

  it('opens on the phone, the subject rendered from the template data', async () => {
    renderAside()
    expect(screen.queryByTestId('inbox-preview')).toBeNull()
    await waitFor(() => expect(screen.getByTestId('phone-preview')).toHaveTextContent('Acme · Welcome, Camille'), { timeout: 3000 })

    const input = (compileTemplatePreview as Mock).mock.calls[0][0]
    expect(input.email.subject).toBe('Welcome, {{ contact.first_name }}')
    expect(input.testData).toEqual({ contact: { first_name: 'Camille', last_name: 'Laurent' } })
  })

  it('lists the email in a desktop inbox on Desktop', async () => {
    renderAside()
    await userEvent.click(screen.getByText('Desktop'))
    expect(screen.queryByTestId('phone-preview')).toBeNull()
    await waitFor(() => expect(screen.getByTestId('inbox-subject')).toHaveTextContent('Welcome, Camille'), { timeout: 3000 })
  })

  it('opens the Content step from its card', async () => {
    const onOpenContent = vi.fn()
    renderAside({ onOpenContent })
    await userEvent.click(screen.getByRole('button', { name: /Edit/ }))
    expect(onOpenContent).toHaveBeenCalled()
  })

  it('says where the content came from, with a Change button, only when it came from somewhere', async () => {
    const onChange = vi.fn()
    const { unmount } = renderAside({ startingPoint: { name: 'Welcome', detail: 'Gallery · English', onChange } })
    expect(screen.getByTestId('starting-point')).toHaveTextContent('Started from “Welcome”')
    await userEvent.click(screen.getByRole('button', { name: 'Change' }))
    expect(onChange).toHaveBeenCalled()
    unmount()
    renderAside()
    expect(screen.queryByTestId('starting-point')).toBeNull()
  })

  it('compiles nothing without content', () => {
    renderAside({ email: { subject: 'Hi', editor_mode: 'code', mjml_source: '' } })
    expect(screen.getByText('No content yet')).toBeInTheDocument()
    expect(compileTemplatePreview).not.toHaveBeenCalled()
  })
})
