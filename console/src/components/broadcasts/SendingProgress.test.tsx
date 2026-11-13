import { afterEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { SendingProgress } from './SendingProgress'

afterEach(() => {
  i18n.activate('en')
})

describe('SendingProgress', () => {
  it('writes the counts with the separators of the console language', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    render(<SendingProgress enqueuedCount={5678} sentCount={1234} failedCount={0} />)

    // French groups thousands with a narrow no-break space, which \s matches.
    expect(screen.getByText(/^1\s234 of 5\s678$/)).toBeInTheDocument()
  })
})
