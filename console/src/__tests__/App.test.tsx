import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { App } from '../App'

describe('App', () => {
  it('renders without crashing', () => {
    render(<App />)
    expect(document.body).toBeDefined()
  })
})
