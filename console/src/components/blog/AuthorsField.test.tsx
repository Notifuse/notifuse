import { describe, expect, it, vi } from 'vitest'
import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { BlogAuthor } from '../../services/api/blog'

// The avatar field needs the file manager; a plain input stands in for it.
vi.mock('../common/ImageURLInput', () => ({
  ImageURLInput: (props: { value?: string; onChange?: (v: string) => void }) => (
    <input aria-label="Avatar URL" value={props.value ?? ''} onChange={(e) => props.onChange?.(e.target.value)} />
  )
}))

import { AuthorsField } from './AuthorsField'

i18n.loadAndActivate({ locale: 'en', messages: {} })

function renderField(initial: BlogAuthor[]) {
  const changes: BlogAuthor[][] = []
  function Host() {
    const [value, setValue] = useState(initial)
    return (
      <AuthorsField
        value={value}
        onChange={(next) => {
          changes.push(next)
          setValue(next)
        }}
      />
    )
  }
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <Host />
      </App>
    </I18nProvider>
  )
  return changes
}

const cards = () => [...document.querySelectorAll('[data-author-card]')] as HTMLElement[]

describe('AuthorsField', () => {
  it('shows each author as a card, initials without a photo, then the card that adds one', () => {
    renderField([{ name: 'Aiko Tanaka', avatar_url: 'https://cdn.example.com/aiko.png' }, { name: 'Tom Becker' }])
    expect(cards()).toHaveLength(2)
    expect(cards()[0].querySelector('img')).toHaveAttribute('src', 'https://cdn.example.com/aiko.png')
    expect(cards()[1]).toHaveTextContent('TB')
    expect(screen.getByRole('button', { name: 'Add author' })).toBeInTheDocument()
  })

  it('adds an author from the last card', async () => {
    const user = userEvent.setup()
    const changes = renderField([])
    await user.click(screen.getByRole('button', { name: 'Add author' }))
    await user.type(await screen.findByLabelText('Name'), 'Maria Lopez')
    await user.click(screen.getByRole('button', { name: 'Add' }))
    await waitFor(() => expect(changes.at(-1)).toEqual([expect.objectContaining({ name: 'Maria Lopez' })]))
  })

  it('edits an author by clicking its card', async () => {
    const user = userEvent.setup()
    const changes = renderField([{ name: 'Tom Becker' }])
    await user.click(screen.getAllByRole('button', { name: 'Edit Tom Becker' })[0])
    const name = await screen.findByLabelText('Name')
    expect(name).toHaveValue('Tom Becker')
    await user.clear(name)
    await user.type(name, 'Thomas Becker')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(changes.at(-1)).toEqual([expect.objectContaining({ name: 'Thomas Becker' })]))
  })

  it('removes an author only once confirmed', async () => {
    const user = userEvent.setup()
    const changes = renderField([{ name: 'Aiko Tanaka' }, { name: 'Tom Becker' }])
    await user.click(screen.getByRole('button', { name: 'Remove Aiko Tanaka' }))
    expect(await screen.findByText('Remove this author?')).toBeInTheDocument()
    expect(changes).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Yes' }))
    await waitFor(() => expect(changes.at(-1)).toEqual([{ name: 'Tom Becker' }]))
  })

  it('shows the initials when the photo does not load', () => {
    renderField([{ name: 'Aiko Tanaka', avatar_url: 'https://cdn.example.com/missing.png' }])
    fireEvent.error(cards()[0].querySelector('img')!)
    expect(cards()[0].querySelector('img')).toBeNull()
    expect(cards()[0]).toHaveTextContent('AT')
  })
})
