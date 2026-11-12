import { describe, it, expect } from 'vitest'
import { templateUnsubscribeCheck } from './unsubscribeLink'
import type { Template } from '../services/api/template'

const code = (mjml: string) => ({ editor_mode: 'code', mjml_source: mjml }) as unknown as Template['email']
const visual = (content: string, asString = false) => {
  const tree = { id: 'root', type: 'mjml', children: [{ type: 'mj-text', content }] }
  return { visual_editor_tree: asString ? JSON.stringify(tree) : tree } as unknown as Template['email']
}

describe('templateUnsubscribeCheck', () => {
  it('reads the MJML source in code mode', () => {
    expect(templateUnsubscribeCheck({ email: code('<a href="{{ unsubscribe_url }}">Unsub</a>') })).toEqual([])
    expect(templateUnsubscribeCheck({ email: code('<p>Hello</p>') })).toEqual(['default'])
  })

  it('reads the visual tree, as an object or a string', () => {
    expect(templateUnsubscribeCheck({ email: visual('<a href="{{unsubscribe_url}}">x</a>') })).toEqual([])
    expect(templateUnsubscribeCheck({ email: visual('<a href="{{ unsubscribe_url }}">x</a>', true) })).toEqual([])
    expect(templateUnsubscribeCheck({ email: visual('no link') })).toEqual(['default'])
  })

  it.each(['unsubscribe_url', 'oneclick_unsubscribe_url', 'notification_center_url'])('accepts {{ %s }}', (name) => {
    expect(templateUnsubscribeCheck({ email: code(`<a href="{{ ${name} }}">x</a>`) })).toEqual([])
  })

  it('accepts a Liquid filter and whitespace control', () => {
    expect(templateUnsubscribeCheck({ email: code('<a href="{{ unsubscribe_url | escape }}">x</a>') })).toEqual([])
    expect(templateUnsubscribeCheck({ email: code('<a href="{{- unsubscribe_url -}}">x</a>') })).toEqual([])
  })

  it('reports each translation without a link by its language', () => {
    const template = {
      email: code('{{ unsubscribe_url }}'),
      translations: {
        fr: { email: code('<p>Bonjour</p>') },
        de: { email: code('{{ notification_center_url }}') },
        es: {}
      }
    } as unknown as Template
    expect(templateUnsubscribeCheck(template)).toEqual(['fr'])
  })

  it('reports the default content as "default"', () => {
    const template = {
      email: code('<p>Hi</p>'),
      translations: { fr: { email: code('{{ unsubscribe_url }}') } }
    } as unknown as Template
    expect(templateUnsubscribeCheck(template)).toEqual(['default'])
  })

  it('does not take a lookalike variable nor the name outside {{ }} for a link', () => {
    expect(templateUnsubscribeCheck({ email: code('{{ unsubscribe_url_text }}') })).toEqual(['default'])
    expect(templateUnsubscribeCheck({ email: code('Click unsubscribe_url to leave') })).toEqual(['default'])
  })
})
