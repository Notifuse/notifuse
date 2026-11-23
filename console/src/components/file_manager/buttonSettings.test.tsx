import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within, act } from '@testing-library/react'
import { App, ConfigProvider, message } from 'antd'
import ButtonFilesSettings from './buttonSettings'
import type { FileManagerSettings } from './interfaces'
import { fakeFiles } from './fakeFilesApi'

vi.mock('../../services/api/files', async () => ({
  filesApi: (await import('./fakeFilesApi')).fakeFiles.api
}))

// As the server sends them: the secret is never received, only that one is stored and its end.
const stored: FileManagerSettings = {
  provider: 'other',
  endpoint: 'https://s3.example.com',
  access_key: 'AKIA',
  bucket: 'media',
  has_secret_key: true,
  secret_key_hint: 'WXYZ'
}

const renderSettings = (settings: FileManagerSettings = stored) => {
  const onUpdateSettings = vi.fn<(next: FileManagerSettings) => Promise<void>>(async () => {})
  render(
    <ConfigProvider>
      <App>
        <ButtonFilesSettings workspaceId="ws" settings={settings} onUpdateSettings={onUpdateSettings}>
          <button type="button">Open settings</button>
        </ButtonFilesSettings>
      </App>
    </ConfigProvider>
  )
  return { onUpdateSettings }
}

const openForm = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'Open settings' }))
  const dialog = await screen.findByRole('dialog')
  await waitFor(() => expect(within(dialog).getByLabelText('S3 bucket')).toHaveValue('media'))
  return dialog
}

const save = (dialog: HTMLElement) =>
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))

describe('ButtonFilesSettings', () => {
  beforeEach(() => {
    fakeFiles.reset()
  })

  it('leaves the secret blank, with the end of the stored one as placeholder', async () => {
    renderSettings()
    const dialog = await openForm()

    const secret = within(dialog).getByLabelText('S3 secret key') as HTMLInputElement
    expect(secret.value).toBe('')
    expect(secret.placeholder).toBe('•••• WXYZ')
    expect(within(dialog).getByText('Blank keeps the current one.')).toBeTruthy()
  })

  it('tests then saves without a secret when the endpoint and access key are unchanged', async () => {
    const { onUpdateSettings } = renderSettings()
    const dialog = await openForm()

    save(dialog)

    await waitFor(() => expect(onUpdateSettings).toHaveBeenCalledTimes(1))
    expect(fakeFiles.api.testSettings).toHaveBeenCalledTimes(1)
    const [workspaceId, tested] = fakeFiles.api.testSettings.mock.calls[0]
    expect(workspaceId).toBe('ws')
    expect(tested).toMatchObject({
      endpoint: 'https://s3.example.com',
      access_key: 'AKIA',
      bucket: 'media'
    })
    expect(tested).not.toHaveProperty('secret_key')
    const saved = onUpdateSettings.mock.calls[0][0]
    expect(saved).toMatchObject({ endpoint: 'https://s3.example.com', access_key: 'AKIA', bucket: 'media' })
    // Neither the secret nor the fields only the server writes.
    expect(saved).not.toHaveProperty('secret_key')
    expect(saved).not.toHaveProperty('has_secret_key')
    expect(saved).not.toHaveProperty('secret_key_hint')
    // Tested before it is saved.
    expect(fakeFiles.api.testSettings.mock.invocationCallOrder[0]).toBeLessThan(
      onUpdateSettings.mock.invocationCallOrder[0]
    )
  })

  it('sends a typed secret to the test and the save', async () => {
    const { onUpdateSettings } = renderSettings()
    const dialog = await openForm()

    fireEvent.change(within(dialog).getByLabelText('S3 secret key'), { target: { value: 'new-secret' } })
    save(dialog)

    await waitFor(() => expect(onUpdateSettings).toHaveBeenCalledTimes(1))
    expect(fakeFiles.api.testSettings.mock.calls[0][1]).toMatchObject({ secret_key: 'new-secret' })
    expect(onUpdateSettings.mock.calls[0][0]).toMatchObject({ secret_key: 'new-secret' })
  })

  it('asks for the secret once the endpoint changes, and sends nothing', async () => {
    const { onUpdateSettings } = renderSettings()
    const dialog = await openForm()

    fireEvent.change(within(dialog).getByLabelText('S3 endpoint'), {
      target: { value: 'https://other.example.com' }
    })
    save(dialog)

    expect(await within(dialog).findByText('Enter the secret key')).toBeTruthy()
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(fakeFiles.api.testSettings).not.toHaveBeenCalled()
    expect(onUpdateSettings).not.toHaveBeenCalled()
  })

  it('asks for the secret when none is stored', async () => {
    const { onUpdateSettings } = renderSettings({ ...stored, has_secret_key: false, secret_key_hint: undefined })
    const dialog = await openForm()

    save(dialog)

    expect(await within(dialog).findByText('Enter the secret key')).toBeTruthy()
    expect(fakeFiles.api.testSettings).not.toHaveBeenCalled()
    expect(onUpdateSettings).not.toHaveBeenCalled()
  })

  it('shows why the test failed and saves nothing', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    // The form reports through antd's static message, which jsdom does not paint.
    const shown = vi.spyOn(message, 'error')
    fakeFiles.api.testSettings.mockRejectedValue(new Error('The bucket refused the access key'))
    const { onUpdateSettings } = renderSettings()
    const dialog = await openForm()

    save(dialog)

    await waitFor(() => expect(shown).toHaveBeenCalledWith('The bucket refused the access key'))
    expect(onUpdateSettings).not.toHaveBeenCalled()
    // The form stays open to fix the values.
    expect(screen.getByRole('dialog')).toBeTruthy()
    shown.mockRestore()
  })
})
