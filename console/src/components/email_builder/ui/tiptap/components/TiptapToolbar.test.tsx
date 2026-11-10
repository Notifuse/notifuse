import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Editor } from '@tiptap/react'
import { LinkButton } from './TiptapToolbar'

// An editor whose selection sits in a link to currentHref (none when empty), and that
// records what the popover sets or removes.
const fakeEditor = (currentHref = '') => {
  const calls: string[] = []
  const chain = {
    focus: () => chain,
    setLink: ({ href }: { href: string }) => {
      calls.push(`set ${href}`)
      return chain
    },
    unsetLink: () => {
      calls.push('unset')
      return chain
    },
    run: () => true
  }
  const editor = {
    chain: () => chain,
    getAttributes: () => (currentHref ? { href: currentHref } : {}),
    isActive: (name: string) => name === 'link' && currentHref !== '',
    state: {
      selection: { from: 1, to: 1, empty: true },
      doc: { resolve: () => ({ marks: () => [], parent: { childCount: 0 } }) }
    }
  }
  return { editor: editor as unknown as Editor, calls }
}

const openPopover = async () => {
  await userEvent.click(screen.getByRole('button'))
  await screen.findByText('Link Type')
}

describe('LinkButton web version link', () => {
  it('inserts the web version variable without asking for a value', async () => {
    const { editor, calls } = fakeEditor()
    render(<LinkButton editor={editor} title="Link" />)
    await openPopover()

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Web version of the email'))

    const target = screen.getByDisplayValue('{{ view_in_browser_url }}')
    expect(target).toBeDisabled()
    expect(screen.getByText(/broadcasts and automations/)).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: 'Insert Link' }))
    expect(calls).toEqual(['set {{ view_in_browser_url }}'])
  })

  it('reopens an existing web version link as one', async () => {
    const { editor } = fakeEditor('{{view_in_browser_url}}')
    render(<LinkButton editor={editor} title="Link" />)
    await openPopover()

    await waitFor(() => expect(screen.getByDisplayValue('{{ view_in_browser_url }}')).toBeDisabled())
    expect(screen.getByRole('button', { name: 'Update Link' })).toBeTruthy()
  })

  it('still removes a URL link left empty', async () => {
    const { editor, calls } = fakeEditor()
    render(<LinkButton editor={editor} title="Link" />)
    await openPopover()

    await userEvent.click(screen.getByRole('button', { name: 'Insert Link' }))
    expect(calls).toEqual(['unset'])
  })
})
