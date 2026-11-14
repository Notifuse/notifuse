import { readFileSync } from 'fs'
import { resolve } from 'path'
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { NotifuseLogo, NotifuseMark } from './NotifuseLogo'

// brand/ holds the official drawing; the console copies its geometry. A retouch of one that
// forgets the other, or a colour variant left behind, fails here.
const brandFile = (name: string) => readFileSync(resolve(__dirname, '../../../../brand', name), 'utf8')
const pathsOf = (svg: string) => [...svg.matchAll(/<path d="([^"]+)"/g)].map((m) => m[1])
const renderedPaths = (container: HTMLElement) =>
  [...container.querySelectorAll('path')].map((p) => p.getAttribute('d'))

describe('brand geometry', () => {
  it.each(['horizontal', 'square'])('the %s variants share one drawing', (format) => {
    const color = pathsOf(brandFile(`notifuse-${format}-color.svg`))
    expect(color).toHaveLength(2)
    expect(pathsOf(brandFile(`notifuse-${format}-black.svg`))).toEqual(color)
    expect(pathsOf(brandFile(`notifuse-${format}-white.svg`))).toEqual(color)
  })

  it('the padded icon is the icon with a margin', () => {
    expect(pathsOf(brandFile('notifuse-square-color-padded.svg'))).toEqual(
      pathsOf(brandFile('notifuse-square-color.svg'))
    )
  })

  it('the console wordmark is the one of brand/', () => {
    const { container } = render(<NotifuseLogo />)
    expect(renderedPaths(container)).toEqual(pathsOf(brandFile('notifuse-horizontal-color.svg')))
  })

  it('the console symbol is the one of brand/', () => {
    const { container } = render(<NotifuseMark />)
    expect(renderedPaths(container)).toEqual(pathsOf(brandFile('notifuse-square-color.svg')))
  })
})
