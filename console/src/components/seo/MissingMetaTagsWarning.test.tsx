import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { MissingMetaTagsWarning } from './MissingMetaTagsWarning'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderWarning = (seo?: Parameters<typeof MissingMetaTagsWarning>[0]['seo'], featuredImage?: string) =>
  render(
    <I18nProvider i18n={i18n}>
      <MissingMetaTagsWarning seo={seo} featuredImage={featuredImage} />
    </I18nProvider>
  )

describe('MissingMetaTagsWarning', () => {
  it('warns while a tag of the shared rule is empty', () => {
    const { container } = renderWarning({ meta_title: 'Guides', meta_description: 'How-tos', og_title: 'Guides', og_description: '  ', og_image: 'https://cdn.example.com/a.png' })
    expect(container.querySelector('.anticon-warning')).not.toBeNull()
  })

  it('shows nothing once every tag is filled', () => {
    const { container } = renderWarning({ meta_title: 'Guides', meta_description: 'How-tos', og_title: 'Guides', og_description: 'All', og_image: 'https://cdn.example.com/a.png' })
    expect(container).toBeEmptyDOMElement()
  })

  it("takes a post's featured image for its share image, as the theme does", () => {
    const { container } = renderWarning(
      { meta_title: 'Guides', meta_description: 'How-tos', og_title: 'Guides', og_description: 'All' },
      'https://cdn.example.com/hero.jpg'
    )
    expect(container).toBeEmptyDOMElement()
  })
})
