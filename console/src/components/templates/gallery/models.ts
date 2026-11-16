import type { EmailBlock } from '../../email_builder/types'
import { ACCENT, COLORS, STYLE, modelBuilder, type ModelBuilder } from './blocks'
import { UNKNOWN_LOGO_WIDTH } from './logoWidth'
import type { GalleryContent } from './content/en'
import type { GalleryAssets, GalleryEmail, GalleryModel } from './types'

type Common = GalleryContent['common']

// The gallery's images, served by the instance (console/public/gallery).
const img = (assets: GalleryAssets, file: string) => `${assets.baseUrl}/${file}`

// ---------------------------------------------------------------------------------------------
// Sections the models share
// ---------------------------------------------------------------------------------------------

function header(b: ModelBuilder, c: Common, assets: GalleryAssets, withNav = false): EmailBlock {
  // The workspace's logo when it has one, sized to the placeholder's 40px height from its own
  // proportions (MJML sizes an image by its width); the gallery's 120×40 placeholder otherwise.
  const logoWidth = assets.logoUrl ? (assets.logoWidth ?? UNKNOWN_LOGO_WIDTH) : 120
  const logo = b.image(assets.logoUrl || img(assets, 'logo.png'), c.logoAlt, { width: `${logoWidth}px`, align: withNav ? 'left' : 'center', href: '{{ workspace.website_url }}' })
  if (!withNav) return b.section([b.column([logo])], { paddingTop: '24px', paddingBottom: '8px', paddingLeft: '32px', paddingRight: '32px' })
  return b.section(
    [
      b.group([
        b.column([logo], { width: '40%', verticalAlign: 'middle' }),
        b.column(
          [b.text(`<a href="{{ workspace.website_url }}" style="color:${COLORS.ink};text-decoration:none">${c.navShop}</a> &nbsp; <a href="{{ workspace.website_url }}" style="color:${COLORS.ink};text-decoration:none">${c.navBlog}</a> &nbsp; <a href="{{ workspace.website_url }}" style="color:${COLORS.ink};text-decoration:none">${c.navHelp}</a>`, { align: 'right', fontSize: '14px', paddingLeft: '0px', paddingRight: '0px' })],
          { width: '60%', verticalAlign: 'middle' }
        )
      ])
    ],
    { paddingTop: '20px', paddingBottom: '8px', paddingLeft: '32px', paddingRight: '32px' }
  )
}

const hero = (b: ModelBuilder, assets: GalleryAssets, file: string, alt: string) =>
  b.section([b.column([b.image(img(assets, file), alt, { fluidOnMobile: 'true' })])], { paddingTop: '0px', paddingBottom: '0px' })

const signature = (b: ModelBuilder, c: Common) => b.section([b.column([b.text(c.signature)])], { paddingTop: '0px' })

const footerSection = (b: ModelBuilder, html: string) =>
  b.section([b.column([b.text(html, STYLE.footer)])], { backgroundColor: 'transparent', paddingTop: '20px', paddingBottom: '28px' })

/** Marketing and welcome emails: the list's preferences and unsubscribe links. */
const marketingFooter = (b: ModelBuilder, c: Common) =>
  footerSection(
    b,
    `${c.address}<br>${c.subscribedTo}<br><a href="{{ notification_center_url }}" style="color:${COLORS.soft}">${c.managePreferences}</a> · <a href="{{ unsubscribe_url }}" style="color:${COLORS.soft}">${c.unsubscribe}</a>`
  )

/** Transactional emails: never an unsubscribe link, which a send without a list leaves empty. */
const transactionalFooter = (b: ModelBuilder, c: Common, why: string) => footerSection(b, `${c.address}<br>${why}<br>${c.questions}`)

/** Opt-in and unsubscribe confirmations: the address only. */
const addressFooter = (b: ModelBuilder, c: Common) => footerSection(b, c.address)

/** Label/value lines, two columns kept side by side. */
function keyValues(b: ModelBuilder, rows: [string, string][], total?: [string, string]): EmailBlock {
  const lines = (pick: 0 | 1) =>
    rows.map((row) => row[pick]).join('<br>') + (total ? `<br><b style="font-size:16px;color:${COLORS.ink}">${total[pick]}</b>` : '')
  return b.section(
    [
      b.group([
        b.column([b.text(lines(0), { ...STYLE.soft, lineHeight: '2' })], { width: '55%' }),
        b.column([b.text(lines(1), { align: 'right', fontWeight: 'bold', lineHeight: '2' })], { width: '45%' })
      ])
    ],
    { paddingTop: '0px' }
  )
}

/** A product card: image, name, price, button. */
const productCard = (b: ModelBuilder, assets: GalleryAssets, file: string, name: string, price: string, cta: string, was?: string) =>
  b.column(
    [
      b.image(img(assets, file), name, { borderRadius: '6px', paddingLeft: '12px', paddingRight: '12px' }),
      b.text(`<b>${name}</b><br><b>${price}</b>${was ? ` <s style="color:${COLORS.soft};font-weight:normal">${was}</s>` : ''}`, { paddingLeft: '12px', paddingRight: '12px' }),
      b.button(cta, '{{ workspace.website_url }}', { backgroundColor: '#ffffff', color: ACCENT, border: `2px solid ${ACCENT}`, paddingLeft: '12px', paddingRight: '12px', align: 'left' })
    ],
    { width: '50%' }
  )

/** A boxed offer: dashed border, code in a monospace face. */
const promoBox = (b: ModelBuilder, lines: EmailBlock[]) =>
  b.section(
    [
      b.column(lines, {
        backgroundColor: COLORS.accentSoft,
        borderTop: `2px dashed ${ACCENT}`,
        borderRight: `2px dashed ${ACCENT}`,
        borderBottom: `2px dashed ${ACCENT}`,
        borderLeft: `2px dashed ${ACCENT}`,
        borderRadius: '8px',
        paddingTop: '12px',
        paddingBottom: '12px'
      })
    ],
    { paddingLeft: '32px', paddingRight: '32px' }
  )

/**
 * Item lines from a list of the template data, one section each: a thumbnail, the name and
 * quantity, the price. MJML with Liquid, since the visual blocks cannot repeat.
 */
const itemLoop = (list: string, quantityLabel: string) => `{% for item in ${list} %}
<mj-section background-color="#ffffff" padding="0px 32px">
  <mj-group>
    <mj-column width="20%" vertical-align="middle">
      <mj-image src="{{ item.image_url }}" alt="{{ item.name }}" width="60px" border-radius="6px" padding="8px 0px" />
    </mj-column>
    <mj-column width="55%" vertical-align="middle">
      <mj-text padding="8px 12px" font-size="15px" color="${COLORS.ink}"><b>{{ item.name }}</b><br /><span style="color:${COLORS.soft};font-size:13px">${quantityLabel} {{ item.quantity }}</span></mj-text>
    </mj-column>
    <mj-column width="25%" vertical-align="middle">
      <mj-text padding="8px 0px" align="right" font-size="15px" font-weight="bold" color="${COLORS.ink}">{{ item.price }}</mj-text>
    </mj-column>
  </mj-group>
</mj-section>
{% endfor %}`

const sampleItems = (assets: GalleryAssets) => [
  { name: 'Linen shirt', quantity: 1, price: '$59.00', image_url: img(assets, 'product-shirt.jpg') },
  { name: 'Canvas tote bag', quantity: 2, price: '$36.00', image_url: img(assets, 'product-tote.jpg') }
]

const contactData = { first_name: 'John', last_name: 'Doe', email: 'john.doe@example.com' }

// ---------------------------------------------------------------------------------------------
// The models, in the order the gallery shows them
// ---------------------------------------------------------------------------------------------

const email = (b: ModelBuilder, subject: string, preheader: string, sections: EmailBlock[], testData: Record<string, unknown>): GalleryEmail => ({
  subject,
  preheader,
  tree: b.email(preheader, sections),
  testData
})

export const GALLERY_MODELS: GalleryModel[] = [
  {
    id: 'blank',
    sentBy: 'any',
    build: ({ common: c, models: { blank: m } }, assets) => {
      const b = modelBuilder('blank')
      return email(b, m.subject, m.preheader, [header(b, c, assets), b.section([b.column([b.text(m.greeting), b.text(m.body)])]), signature(b, c), marketingFooter(b, c)], { contact: contactData })
    }
  },
  {
    id: 'verify',
    useCase: 'account',
    category: 'transactional',
    sentBy: 'api',
    build: ({ common: c, models: { verify: m } }, assets) => {
      const b = modelBuilder('verify')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([
            b.column([
              b.text(m.title, { ...STYLE.h2, ...STYLE.center }),
              b.text(m.intro, { ...STYLE.soft, ...STYLE.center }),
              b.text('{{ code }}', { ...STYLE.code, containerBackgroundColor: COLORS.accentSoft, paddingTop: '14px', paddingBottom: '14px' }),
              b.text(m.ignore, { ...STYLE.small, ...STYLE.center })
            ])
          ]),
          transactionalFooter(b, c, c.hasAccount)
        ],
        { code: '482913', expires_in_minutes: 10 }
      )
    }
  },
  {
    id: 'reset',
    useCase: 'account',
    category: 'transactional',
    sentBy: 'api',
    build: ({ common: c, models: { reset: m } }, assets) => {
      const b = modelBuilder('reset')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([
            b.column([
              b.text(m.title, { ...STYLE.h2, ...STYLE.center }),
              b.text(m.intro, { ...STYLE.soft, ...STYLE.center }),
              b.button(m.button, '{{ reset_url }}'),
              b.text(`${c.linkFallback}<br><a href="{{ reset_url }}" style="color:${ACCENT}">{{ reset_url }}</a>`, { ...STYLE.small, ...STYLE.center }),
              b.text(m.ignore, { ...STYLE.small, ...STYLE.center })
            ])
          ]),
          transactionalFooter(b, c, c.hasAccount)
        ],
        { contact: contactData, reset_url: 'https://acme.com/reset?token=7f3a9c' }
      )
    }
  },
  {
    id: 'confirm',
    useCase: 'account',
    category: 'opt_in',
    sentBy: 'list',
    build: ({ common: c, models: { confirm: m } }, assets) => {
      const b = modelBuilder('confirm')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([
            b.column([
              b.text(m.title, { ...STYLE.h2, ...STYLE.center }),
              b.text(m.intro, { ...STYLE.soft, ...STYLE.center }),
              b.button(m.button, '{{ confirm_subscription_url }}'),
              b.text(m.ignore, { ...STYLE.small, ...STYLE.center })
            ])
          ]),
          addressFooter(b, c)
        ],
        { contact: contactData }
      )
    }
  },
  {
    id: 'invite',
    useCase: 'account',
    category: 'transactional',
    sentBy: 'api',
    build: ({ common: c, models: { invite: m } }, assets) => {
      const b = modelBuilder('invite')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([
            b.column([
              b.text(m.title, { ...STYLE.h2, ...STYLE.center }),
              b.text(m.intro, { ...STYLE.soft, ...STYLE.center }),
              b.button(m.button, '{{ invite_url }}'),
              b.text(`${c.linkFallback}<br><a href="{{ invite_url }}" style="color:${ACCENT}">{{ invite_url }}</a>`, { ...STYLE.small, ...STYLE.center })
            ])
          ]),
          transactionalFooter(b, c, c.invited)
        ],
        { inviter_name: 'Maria Lopez', team_name: 'Acme Marketing', invite_url: 'https://acme.com/invite/9b2c', expires_in_days: 7 }
      )
    }
  },
  {
    id: 'receipt',
    useCase: 'billing',
    category: 'transactional',
    sentBy: 'api',
    build: ({ common: c, models: { receipt: m } }, assets) => {
      const b = modelBuilder('receipt')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([b.column([b.text(m.title, STYLE.h2), b.text(m.intro, STYLE.soft)])], { paddingBottom: '0px' }),
          keyValues(
            b,
            [
              [m.invoice, '{{ invoice.number }}'],
              [m.date, '{{ invoice.date }}'],
              [m.description, '{{ invoice.description }}'],
              [m.paymentMethod, '{{ invoice.payment_method }}']
            ],
            [m.amountPaid, '{{ invoice.amount }}']
          ),
          b.section([b.column([b.button(m.button, '{{ invoice.url }}', { align: 'left', paddingLeft: '32px' })])], { paddingTop: '0px' }),
          transactionalFooter(b, c, c.hasAccount)
        ],
        {
          invoice: {
            number: 'INV-2026-0142',
            date: 'November 15, 2026',
            description: 'Pro plan, monthly',
            payment_method: 'Visa ending 4242',
            amount: '$49.00',
            url: 'https://acme.com/invoices/0142'
          }
        }
      )
    }
  },
  {
    id: 'order',
    useCase: 'shop',
    category: 'transactional',
    sentBy: 'api',
    build: ({ common: c, models: { order: m } }, assets) => {
      const b = modelBuilder('order')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([b.column([b.text(m.tag, STYLE.tag), b.text(m.title, STYLE.h2), b.text(m.intro, STYLE.soft)])], { paddingBottom: '4px' }),
          b.liquid(itemLoop('order.items', m.quantity)),
          keyValues(b, [[m.subtotal, '{{ order.subtotal }}'], [m.shipping, '{{ order.shipping }}']], [m.total, '{{ order.total }}']),
          b.section([
            b.column([
              b.text(`<span style="font-size:11px;letter-spacing:1px;text-transform:uppercase;font-weight:bold;color:${ACCENT}">${m.shippingTo}</span><br>{{ order.address }}`),
              b.button(m.button, '{{ order.url }}')
            ])
          ], { paddingTop: '0px' }),
          transactionalFooter(b, c, c.placedOrder)
        ],
        {
          contact: contactData,
          order: {
            number: '1042',
            url: 'https://acme.com/orders/1042',
            items: sampleItems(assets),
            subtotal: '$95.00',
            shipping: '$5.00',
            total: '$100.00',
            address: '12 Market Street, San Francisco'
          }
        }
      )
    }
  },
  {
    id: 'shipping',
    useCase: 'shop',
    category: 'transactional',
    sentBy: 'api',
    build: ({ common: c, models: { shipping: m } }, assets) => {
      const b = modelBuilder('shipping')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          hero(b, assets, 'hero-shipping.jpg', ''),
          b.section([b.column([b.text(m.title, STYLE.h2), b.text(m.intro, STYLE.soft)])], { paddingBottom: '0px' }),
          keyValues(b, [[m.carrier, '{{ shipment.carrier }}'], [m.trackingNumber, '{{ shipment.tracking_number }}']]),
          b.section([b.column([b.button(m.button, '{{ shipment.tracking_url }}')])], { paddingTop: '0px' }),
          transactionalFooter(b, c, c.placedOrder)
        ],
        {
          order: { number: '1042' },
          shipment: { carrier: 'UPS', tracking_number: '1Z999AA10123456784', estimated_delivery: 'Tuesday, November 18', tracking_url: 'https://www.ups.com/track' }
        }
      )
    }
  },
  {
    id: 'cart',
    useCase: 'shop',
    category: 'marketing',
    sentBy: 'automation',
    build: ({ common: c, models: { cart: m } }, assets) => {
      const b = modelBuilder('cart')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets, true),
          b.section([b.column([b.text(m.title, { ...STYLE.h1, ...STYLE.center }), b.text(m.intro, { ...STYLE.soft, ...STYLE.center })])], { paddingBottom: '4px' }),
          // The cart's items come from the event that started the automation.
          b.liquid(`{% if trigger_event.properties.items %}\n${itemLoop('trigger_event.properties.items', m.quantity)}\n{% endif %}`),
          b.section([b.column([b.button(m.button, '{{ trigger_event.properties.cart_url | default: workspace.website_url }}')])]),
          b.section([b.column([b.text(m.alsoLiked, { ...STYLE.h2, ...STYLE.center, fontSize: '17px' })])], { paddingBottom: '0px' }),
          b.section([productCard(b, assets, 'product-beanie.jpg', m.product1, '$24.00', m.shopNow), productCard(b, assets, 'product-wallet.jpg', m.product2, '$45.00', m.shopNow)], { paddingLeft: '20px', paddingRight: '20px' }),
          marketingFooter(b, c)
        ],
        {
          contact: contactData,
          list: { id: 'newsletter', name: 'Newsletter' },
          trigger_event: {
            kind: 'custom_event.cart_abandoned',
            name: 'cart_abandoned',
            external_id: 'cart-42',
            properties: { items: sampleItems(assets), cart_url: 'https://acme.com/cart/42' }
          }
        }
      )
    }
  },
  {
    id: 'promo',
    useCase: 'shop',
    category: 'marketing',
    sentBy: 'broadcast',
    build: ({ common: c, models: { promo: m } }, assets) => {
      const b = modelBuilder('promo')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets, true),
          hero(b, assets, 'hero-sale.jpg', ''),
          b.section([b.column([b.text(m.tag, { ...STYLE.tag, ...STYLE.center }), b.text(m.title, { ...STYLE.h1, ...STYLE.center }), b.text(m.intro, { ...STYLE.soft, ...STYLE.center })])]),
          promoBox(b, [b.text(m.useCode, { ...STYLE.small, ...STYLE.center }), b.text('WINTER30', { ...STYLE.code, fontSize: '24px', letterSpacing: '4px' }), b.text(m.ends, { ...STYLE.small, ...STYLE.center })]),
          b.section([productCard(b, assets, 'product-shirt.jpg', m.product1, '$41.30', m.shopNow, '$59.00'), productCard(b, assets, 'product-tote.jpg', m.product2, '$12.60', m.shopNow, '$18.00')], { paddingLeft: '20px', paddingRight: '20px' }),
          b.section([b.column([b.button(m.button, '{{ workspace.website_url }}')])]),
          marketingFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  },
  {
    id: 'newsletter',
    useCase: 'news',
    category: 'marketing',
    sentBy: 'broadcast',
    build: ({ common: c, models: { newsletter: m } }, assets) => {
      const b = modelBuilder('newsletter')
      const article = (file: string, tag: string, title: string, text: string) =>
        b.section(
          [
            b.column([b.image(img(assets, file), title, { borderRadius: '6px', paddingLeft: '32px', paddingRight: '8px' })], { width: '38%' }),
            b.column(
              [b.text(`<span style="font-size:11px;letter-spacing:1px;text-transform:uppercase;font-weight:bold;color:${ACCENT}">${tag}</span><br><b style="font-size:16px">${title}</b><br><span style="color:${COLORS.soft};font-size:13px">${text}</span><br><a href="{{ workspace.website_url }}" style="color:${ACCENT};font-weight:bold;text-decoration:none">${m.readMore}</a>`, { paddingLeft: '8px' })],
              { width: '62%' }
            )
          ],
          { paddingTop: '8px', paddingBottom: '8px' }
        )
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets, true),
          b.section([b.column([b.text(m.greeting, STYLE.small)])], { paddingBottom: '0px' }),
          b.section([
            b.column([
              b.image(img(assets, 'hero-newsletter.jpg'), m.featuredTitle, { borderRadius: '6px', paddingLeft: '32px', paddingRight: '32px' }),
              b.text(m.featuredTag, STYLE.tag),
              b.text(m.featuredTitle, STYLE.h2),
              b.text(m.featuredText, STYLE.soft),
              b.text(`<a href="{{ workspace.website_url }}" style="color:${ACCENT};font-weight:bold;text-decoration:none">${m.readArticle}</a>`)
            ])
          ]),
          b.divider(),
          article('article-guide.jpg', m.article1Tag, m.article1Title, m.article1Text),
          article('article-story.jpg', m.article2Tag, m.article2Title, m.article2Text),
          b.divider(),
          b.section([b.column([b.text(m.quote, { fontSize: '17px', fontStyle: 'italic' }), b.text(m.quoteAuthor, STYLE.small)])]),
          marketingFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  },
  {
    id: 'announce',
    useCase: 'news',
    category: 'marketing',
    sentBy: 'broadcast',
    build: ({ common: c, models: { announce: m } }, assets) => {
      const b = modelBuilder('announce')
      const feature = (title: string, text: string) => b.text(`<span style="color:${ACCENT};font-weight:bold">✓</span> <b>${title}</b><br><span style="color:${COLORS.soft};font-size:14px">${text}</span>`)
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets, true),
          hero(b, assets, 'hero-announce.jpg', m.title),
          b.section([b.column([b.text(m.tag, { ...STYLE.tag, ...STYLE.center }), b.text(m.title, { ...STYLE.h1, ...STYLE.center }), b.text(m.intro, { ...STYLE.soft, ...STYLE.center })])]),
          b.section([b.column([feature(m.feature1Title, m.feature1Text), feature(m.feature2Title, m.feature2Text), feature(m.feature3Title, m.feature3Text)])], { paddingTop: '0px' }),
          b.section([b.column([b.button(m.button, '{{ workspace.website_url }}')])]),
          marketingFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  },
  {
    id: 'event',
    useCase: 'news',
    category: 'marketing',
    sentBy: 'broadcast',
    build: ({ common: c, models: { event: m } }, assets) => {
      const b = modelBuilder('event')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          hero(b, assets, 'hero-event.jpg', m.title),
          b.section(
            [
              b.group([
                b.column(
                  [
                    b.text(m.month, { align: 'center', color: '#ffffff', containerBackgroundColor: ACCENT, fontSize: '12px', fontWeight: 'bold', letterSpacing: '2px', paddingTop: '4px', paddingBottom: '4px', paddingLeft: '0px', paddingRight: '0px' }),
                    b.text(m.day, { align: 'center', fontSize: '28px', fontWeight: 'bold', paddingLeft: '0px', paddingRight: '0px' })
                  ],
                  { width: '18%', borderTop: `1px solid ${COLORS.line}`, borderRight: `1px solid ${COLORS.line}`, borderBottom: `1px solid ${COLORS.line}`, borderLeft: `1px solid ${COLORS.line}`, borderRadius: '8px', verticalAlign: 'middle' }
                ),
                b.column([b.text(m.title, { ...STYLE.h2, paddingLeft: '16px' }), b.text(m.when, { ...STYLE.small, paddingLeft: '16px' })], { width: '82%', verticalAlign: 'middle' })
              ])
            ],
            { paddingLeft: '32px', paddingRight: '32px' }
          ),
          b.section([b.column([b.text(m.intro, STYLE.soft)])], { paddingTop: '0px', paddingBottom: '0px' }),
          keyValues(b, [[m.agenda1Time, m.agenda1], [m.agenda2Time, m.agenda2], [m.agenda3Time, m.agenda3]]),
          b.section([b.column([b.button(m.button, '{{ workspace.website_url }}', { align: 'left', paddingLeft: '32px' }), b.text(m.recording, STYLE.small)])], { paddingTop: '0px' }),
          marketingFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  },
  {
    id: 'welcome',
    useCase: 'lifecycle',
    category: 'welcome',
    sentBy: 'list',
    build: ({ common: c, models: { welcome: m } }, assets) => {
      const b = modelBuilder('welcome')
      const step = (n: string, title: string, text: string) =>
        b.column([b.text(`<span style="display:inline-block;width:28px;height:28px;line-height:28px;border-radius:14px;background:${COLORS.accentSoft};color:${ACCENT};text-align:center;font-weight:bold">${n}</span><br><b>${title}</b><br><span style="color:${COLORS.soft};font-size:13px">${text}</span>`, { paddingLeft: '12px', paddingRight: '12px' })], { width: '33.33%' })
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          hero(b, assets, 'hero-welcome.jpg', ''),
          b.section([b.column([b.text(m.title, { ...STYLE.h1, ...STYLE.center }), b.text(m.intro, { ...STYLE.soft, ...STYLE.center })])]),
          b.section([step('1', m.step1Title, m.step1Text), step('2', m.step2Title, m.step2Text), step('3', m.step3Title, m.step3Text)], { paddingLeft: '20px', paddingRight: '20px', paddingTop: '0px' }),
          b.section([b.column([b.button(m.button, '{{ workspace.website_url }}')])], { paddingTop: '0px' }),
          signature(b, c),
          marketingFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  },
  {
    id: 'winback',
    useCase: 'lifecycle',
    category: 'marketing',
    sentBy: 'automation',
    build: ({ common: c, models: { winback: m } }, assets) => {
      const b = modelBuilder('winback')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([b.column([b.text(m.title, { ...STYLE.h1, ...STYLE.center }), b.text(m.intro, { ...STYLE.soft, ...STYLE.center })])]),
          promoBox(b, [b.text(m.tag, { ...STYLE.tag, ...STYLE.center }), b.text(m.offer, { ...STYLE.h2, ...STYLE.center }), b.text('COMEBACK20', { ...STYLE.code, fontSize: '22px', letterSpacing: '4px' }), b.text(m.valid, { ...STYLE.small, ...STYLE.center })]),
          b.section([b.column([b.button(m.button, '{{ workspace.website_url }}')])]),
          signature(b, c),
          marketingFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  },
  {
    id: 'unsub',
    useCase: 'lifecycle',
    category: 'unsubscribe',
    sentBy: 'list',
    build: ({ common: c, models: { unsub: m } }, assets) => {
      const b = modelBuilder('unsub')
      return email(
        b,
        m.subject,
        m.preheader,
        [
          header(b, c, assets),
          b.section([
            b.column([
              b.text(m.title, { ...STYLE.h2, ...STYLE.center }),
              b.text(m.intro, { ...STYLE.soft, ...STYLE.center }),
              b.button(m.button, '{{ notification_center_url }}', { backgroundColor: '#ffffff', color: ACCENT, border: `2px solid ${ACCENT}` }),
              b.text(m.mistake, { ...STYLE.small, ...STYLE.center })
            ])
          ]),
          addressFooter(b, c)
        ],
        { contact: contactData, list: { id: 'newsletter', name: 'Newsletter' } }
      )
    }
  }
]

export const galleryModel = (id: string) => GALLERY_MODELS.find((model) => model.id === id)
