import type { EmailBlock } from '../../../email_builder/types'
import { withBrand } from '../buildStartingPoint'
import { GALLERY_CONTENT, type GalleryContent } from '../content'
import { logoWidthFor } from '../logoWidth'
import { galleryModel } from '../models'
import type { GalleryAssets, GalleryModelId } from '../types'

// The demo workspace's email templates, built from the gallery's models.
//
// The demo seed is Go (internal/service/demo_service.go): it cannot run the gallery's builders,
// so it embeds what they build. demoFixtures.test.ts writes one JSON per template into
// internal/service/demo_gallery (`npm run demo-fixtures`) and fails when a committed file differs
// from what the builders build today, so a gallery change cannot silently leave the demo behind.

/** Where the builders put the gallery's images: each one is then swapped for an Unsplash photo
 * (DEMO_PHOTOS). Not Liquid: one left behind must show, not render empty. */
export const DEMO_GALLERY_BASE_URL = '[[notifuse_gallery_base_url]]'

/** The fixtures' folder, from the repository root. */
export const DEMO_FIXTURES_DIR = 'internal/service/demo_gallery'

const DEMO_LANGUAGES = ['en', 'fr', 'es'] as const
type DemoLanguage = (typeof DEMO_LANGUAGES)[number]

/** The demo workspace, as createDemoWorkspace creates it; the Go tests check they agree. */
export const DEMO_BRAND = {
  name: 'Apple',
  logoUrl: 'https://www.apple.com/ac/structured-data/images/knowledge_graph_logo.png',
  // That logo is 302×302: the width that gives it the headers' height.
  logoWidth: logoWidthFor(302, 302),
  defaultLanguage: 'en' as DemoLanguage,
  languages: DEMO_LANGUAGES
}

/** An Unsplash photo (free to use under the Unsplash licence), cropped by its image CDN. */
export const unsplash = (photo: string, width: number, height: number) =>
  `https://images.unsplash.com/photo-${photo}?w=${width}&h=${height}&fit=crop&auto=format&q=80`

interface DemoPhoto {
  /** The Unsplash photo id, after `photo-`. */
  photo: string
  /** The gallery image's own size, so the photo fills the same slot. */
  width: number
  height: number
  /** What the photo shows, per language. */
  alt: Record<DemoLanguage, string>
}

/**
 * The photo that stands, in the demo, for each gallery image its templates show: the demo's emails
 * show real photos rather than the gallery's placeholder art, and do not depend on the instance
 * serving its console. A gallery image missing here fails the build.
 */
export const DEMO_PHOTOS: Record<string, DemoPhoto> = {
  'hero-newsletter.jpg': {
    photo: '1533986690673-c50390c01cfa',
    width: 1072,
    height: 480,
    alt: {
      en: 'A silver iMac with its keyboard and mouse on a white desk',
      fr: 'Un iMac argenté avec son clavier et sa souris sur un bureau blanc',
      es: 'Un iMac plateado con su teclado y su ratón sobre un escritorio blanco'
    }
  },
  'hero-announce.jpg': {
    photo: '1561634109-465ffe688b50',
    width: 1200,
    height: 500,
    alt: {
      en: 'A laptop and a phone on a grey desk mat by a window',
      fr: 'Un ordinateur portable et un téléphone sur un sous-main gris près d’une fenêtre',
      es: 'Un portátil y un teléfono sobre una alfombrilla gris junto a una ventana'
    }
  },
  'hero-welcome.jpg': {
    photo: '1532198528077-0d9e4ca9bb40',
    width: 1200,
    height: 460,
    alt: {
      en: 'A closed silver MacBook on a wooden table',
      fr: 'Un MacBook argenté fermé sur une table en bois',
      es: 'Un MacBook plateado cerrado sobre una mesa de madera'
    }
  },
  'article-guide.jpg': {
    photo: '1517971071642-34a2d3ecc9cd',
    width: 480,
    height: 360,
    alt: {
      en: 'A hand writing on a sheet of paper with a pen',
      fr: 'Une main qui écrit au stylo sur une feuille de papier',
      es: 'Una mano que escribe con un bolígrafo sobre una hoja de papel'
    }
  },
  'article-story.jpg': {
    photo: '1753351052363-53ce102830eb',
    width: 480,
    height: 360,
    alt: {
      en: 'A smiling shop owner in an apron behind her counter',
      fr: 'Une commerçante souriante en tablier derrière son comptoir',
      es: 'Una comerciante sonriente con delantal detrás de su mostrador'
    }
  },
  'product-beanie.jpg': {
    photo: '1630691650107-53dd500d2907',
    width: 600,
    height: 600,
    alt: {
      en: 'A mustard knit beanie on a yellow background',
      fr: 'Un bonnet en maille moutarde sur un fond jaune',
      es: 'Un gorro de punto mostaza sobre un fondo amarillo'
    }
  },
  'product-wallet.jpg': {
    photo: '1624538000860-24716b9050f2',
    width: 600,
    height: 600,
    alt: {
      en: 'A tan leather bifold wallet on a white table',
      fr: 'Un portefeuille en cuir fauve sur une table blanche',
      es: 'Una cartera de cuero marrón claro sobre una mesa blanca'
    }
  },
  'product-shirt.jpg': {
    photo: '1740711152088-88a009e877bb',
    width: 600,
    height: 600,
    alt: {
      en: 'A blue linen shirt on a hanger against a white wall',
      fr: 'Une chemise en lin bleue sur un cintre contre un mur blanc',
      es: 'Una camisa de lino azul en una percha contra una pared blanca'
    }
  },
  'product-tote.jpg': {
    photo: '1544816155-12df9643f363',
    width: 600,
    height: 600,
    alt: {
      en: 'A beige canvas tote bag against a grey wall',
      fr: 'Un tote bag en toile beige contre un mur gris',
      es: 'Una bolsa de tela beige contra una pared gris'
    }
  }
}

type Common = GalleryContent['common']
type Copy<M extends GalleryModelId> = { common?: Partial<Common>; model?: Partial<GalleryContent['models'][M]> }

interface DemoTemplateOf<M extends GalleryModelId> {
  /** The id the demo's broadcasts, automations and notifications send. */
  templateId: string
  model: M
  /** The demo's category when it is not the model's. */
  category?: string
  subject: Record<DemoLanguage, string>
  preheader: Record<DemoLanguage, string>
  /** The model's text replaced for this template, per language. */
  copy?: Partial<Record<DemoLanguage, Copy<M>>>
  /** Over the model's test data. */
  testData: Record<string, unknown>
  /** Photos of this template that differ from DEMO_PHOTOS, by gallery image. */
  photos?: Record<string, DemoPhoto>
}
type DemoTemplate = { [M in GalleryModelId]: DemoTemplateOf<M> }[GalleryModelId]

// The demo's seeded messages carry the contact only (and reset_url for the password reset): a
// footer naming {{ list.name }} would read "subscribed to ." in their previews. Every list email
// of the demo goes to its one public list, named Newsletter, which a real send names itself.
const subscribedTo: Record<DemoLanguage, Partial<Common>> = {
  en: { subscribedTo: "You are receiving this email because you subscribed to {{ list.name | default: 'Newsletter' }}." },
  fr: { subscribedTo: "Vous recevez cet e-mail car votre adresse est inscrite à {{ list.name | default: 'Newsletter' }}." },
  es: { subscribedTo: "Recibes este correo porque te suscribiste a {{ list.name | default: 'Newsletter' }}." }
}

// A transactional email's footer says why it came: the gallery's own words for an order.
const placedOrder = (language: DemoLanguage): Partial<Common> => ({
  subscribedTo: withBrand(GALLERY_CONTENT[language].common, DEMO_BRAND.name).placedOrder
})

const contact = (first_name: string, last_name: string) => ({
  contact: { first_name, last_name, email: `${first_name}.${last_name}@example.com`.toLowerCase() }
})

/** The demo's email templates, each one built from a gallery model. */
export const DEMO_TEMPLATES: DemoTemplate[] = [
  {
    templateId: 'newsletter-weekly',
    model: 'newsletter',
    subject: {
      en: '{{contact.first_name}}, Your Weekly Update is Here! 📧',
      fr: '{{contact.first_name}}, Votre mise à jour hebdomadaire est arrivée ! 📧',
      es: '{{contact.first_name}}, ¡Tu actualización semanal está aquí! 📧'
    },
    preheader: {
      en: 'Your weekly dose of updates and insights',
      fr: "Votre dose hebdomadaire de mises à jour et d'informations",
      es: 'Tu dosis semanal de novedades e información'
    },
    copy: {
      en: { common: subscribedTo.en, model: { greeting: 'Hi {{ contact.first_name }}, here is what happened this week.' } },
      fr: { common: subscribedTo.fr, model: { greeting: 'Bonjour {{ contact.first_name }}, voici ce qui s’est passé cette semaine.' } },
      es: { common: subscribedTo.es, model: { greeting: 'Hola, {{ contact.first_name }}. Esto es lo que pasó esta semana.' } }
    },
    testData: contact('John', 'Doe')
  },
  {
    // The other arm of the newsletter A/B test: a different layout, the digest's own words.
    templateId: 'newsletter-weekly-v2',
    model: 'announce',
    subject: {
      en: "🚀 {{contact.first_name}}, This Week's Top Stories & Updates!",
      fr: '🚀 {{contact.first_name}}, Les articles et nouveautés de la semaine !',
      es: '🚀 {{contact.first_name}}, ¡Las mejores historias y novedades de la semana!'
    },
    preheader: {
      en: 'Your personalized weekly roundup of insights and updates',
      fr: "Votre sélection hebdomadaire personnalisée d'informations et de mises à jour",
      es: 'Tu selección semanal personalizada de novedades y actualizaciones'
    },
    copy: {
      en: {
        common: subscribedTo.en,
        model: {
          tag: 'Weekly digest',
          title: 'Stay ahead of the curve',
          intro: 'Hey {{ contact.first_name }}, here is your weekly digest: the latest trends, insights and updates, picked for you.',
          feature1Title: 'Featured story',
          feature1Text: 'New industry standards are reshaping how we approach digital transformation. Here is what you need to know.',
          feature2Title: 'Quick tips',
          feature2Text: 'Five productivity habits successful professionals swear by. Simple changes, big impact.',
          feature3Title: 'Trending now',
          feature3Text: 'The tools and strategies everyone is talking about this week.',
          button: 'Explore more'
        }
      },
      fr: {
        common: subscribedTo.fr,
        model: {
          tag: 'Résumé hebdomadaire',
          title: 'Gardez une longueur d’avance',
          intro: 'Bonjour {{ contact.first_name }}, voici votre résumé de la semaine : les dernières tendances, informations et nouveautés, choisies pour vous.',
          feature1Title: 'Article vedette',
          feature1Text: 'De nouvelles normes redéfinissent notre approche de la transformation numérique. Voici ce qu’il faut savoir.',
          feature2Title: 'Astuces rapides',
          feature2Text: 'Cinq habitudes de productivité adoptées par les professionnels qui réussissent. De petits changements, un grand impact.',
          feature3Title: 'Tendances du moment',
          feature3Text: 'Les outils et stratégies dont tout le monde parle cette semaine.',
          button: 'En savoir plus'
        }
      },
      es: {
        common: subscribedTo.es,
        model: {
          tag: 'Resumen semanal',
          title: 'Mantente a la vanguardia',
          intro: 'Hola, {{ contact.first_name }}. Aquí tienes tu resumen semanal: las últimas tendencias, novedades y actualizaciones, elegidas para ti.',
          feature1Title: 'Artículo destacado',
          feature1Text: 'Nuevos estándares están transformando nuestra forma de abordar la transformación digital. Esto es lo que debes saber.',
          feature2Title: 'Consejos rápidos',
          feature2Text: 'Cinco hábitos de productividad que usan los profesionales de éxito. Cambios sencillos, gran impacto.',
          feature3Title: 'Tendencias del momento',
          feature3Text: 'Las herramientas y estrategias de las que todos hablan esta semana.',
          button: 'Explorar más'
        }
      }
    },
    testData: contact('Sarah', 'Wilson')
  },
  {
    templateId: 'welcome-email',
    model: 'welcome',
    subject: {
      en: 'Welcome to our community, {{contact.first_name}}! 🎉',
      fr: 'Bienvenue dans notre communauté, {{contact.first_name}} ! 🎉',
      es: '¡Bienvenido/a a nuestra comunidad, {{contact.first_name}}! 🎉'
    },
    preheader: {
      en: 'Thank you for joining us, {{contact.first_name}}!',
      fr: 'Merci de nous rejoindre, {{contact.first_name}} !',
      es: 'Gracias por unirte, {{contact.first_name}}!'
    },
    copy: { en: { common: subscribedTo.en }, fr: { common: subscribedTo.fr }, es: { common: subscribedTo.es } },
    testData: contact('Jane', 'Smith')
  },
  {
    templateId: 'password-reset',
    model: 'reset',
    subject: {
      en: 'Reset your password, {{contact.first_name}}',
      fr: 'Réinitialisez votre mot de passe, {{contact.first_name}}',
      es: 'Restablece tu contraseña, {{contact.first_name}}'
    },
    preheader: {
      en: 'You requested a password reset for your account',
      fr: 'Vous avez demandé la réinitialisation de votre mot de passe',
      es: 'Solicitaste restablecer la contraseña de tu cuenta'
    },
    // The URL the seeded password-reset messages carry.
    testData: { ...contact('Alex', 'Johnson'), reset_url: 'https://demo.notifuse.com/reset-password?token=demo_token_123' }
  },
  {
    // The product-led arm of the cart-recovery A/B test: the model as it is.
    templateId: 'cart-recovery-a',
    model: 'cart',
    subject: {
      en: '{{contact.first_name}}, your cart is still waiting 🛒',
      fr: '{{contact.first_name}}, votre panier vous attend toujours 🛒',
      es: '{{contact.first_name}}, tu carrito sigue esperando 🛒'
    },
    preheader: {
      en: '{{contact.first_name}}, the items you picked are still saved',
      fr: '{{contact.first_name}}, les articles que vous avez choisis sont toujours enregistrés',
      es: '{{contact.first_name}}, los artículos que elegiste siguen guardados'
    },
    copy: { en: { common: subscribedTo.en }, fr: { common: subscribedTo.fr }, es: { common: subscribedTo.es } },
    testData: contact('Emma', 'Clarke')
  },
  {
    // The discount-led arm: the same email, offering 10% off.
    templateId: 'cart-recovery-b',
    model: 'cart',
    subject: {
      en: "{{contact.first_name}}, here's 10% off your cart 🎁",
      fr: '{{contact.first_name}}, 10 % de remise sur votre panier 🎁',
      es: '{{contact.first_name}}, un 10 % de descuento en tu carrito 🎁'
    },
    preheader: {
      en: '{{contact.first_name}}, here is 10% off to finish your order',
      fr: '{{contact.first_name}}, voici 10 % de remise pour finaliser votre commande',
      es: '{{contact.first_name}}, aquí tienes un 10 % para terminar tu pedido'
    },
    copy: {
      en: {
        common: subscribedTo.en,
        model: {
          title: 'Here is 10% off to finish up',
          intro: 'Hi {{ contact.first_name }}, your cart is saved, and so is 10% off: use the code <b>APPLE10</b> at checkout. It is valid for 48 hours, accessories included.',
          button: 'Claim my 10% off'
        }
      },
      fr: {
        common: subscribedTo.fr,
        model: {
          title: '10 % de remise pour conclure',
          intro: 'Bonjour {{ contact.first_name }}, votre panier est sauvegardé, et 10 % de remise vous attendent : utilisez le code <b>APPLE10</b> au moment du paiement. Il est valable 48 heures, accessoires compris.',
          button: 'Profiter des 10 %'
        }
      },
      es: {
        common: subscribedTo.es,
        model: {
          title: 'Un 10 % de descuento para terminar',
          intro: 'Hola, {{ contact.first_name }}. Guardamos tu carrito, y también un 10 % de descuento: usa el código <b>APPLE10</b> al finalizar la compra. Es válido durante 48 horas, accesorios incluidos.',
          button: 'Aprovechar el 10 %'
        }
      }
    },
    testData: contact('Liam', 'Ferreira')
  },
  {
    templateId: 'winback-offer',
    model: 'winback',
    subject: {
      en: "We miss you, {{contact.first_name}} — here's $50 off 👋",
      fr: 'Vous nous manquez, {{contact.first_name}} — 50 € pour votre retour 👋',
      es: 'Te echamos de menos, {{contact.first_name}}: 50 € para tu vuelta 👋'
    },
    preheader: {
      en: '{{contact.first_name}}, a lot has changed since your last visit',
      fr: '{{contact.first_name}}, beaucoup de choses ont changé depuis votre dernière visite',
      es: '{{contact.first_name}}, han cambiado muchas cosas desde tu última visita'
    },
    // The offer the subject announces.
    copy: {
      en: { common: subscribedTo.en, model: { offer: '$50 off your next order over $499' } },
      fr: { common: subscribedTo.fr, model: { offer: '50 € de remise dès 499 € d’achat' } },
      es: { common: subscribedTo.es, model: { offer: '50 € de descuento en pedidos de más de 499 €' } }
    },
    testData: contact('Olivia', 'Moreau')
  },
  {
    // The post-purchase automation's thank-you, which welcomes the buyer to the VIP Club. Not the
    // order model: it reads the items, totals and address from {{ order }}, which an automation's
    // send does not carry (it holds the contact and the purchase event), so it would render an
    // empty order. Transactional, so the executor's marketing guard never holds it back.
    templateId: 'order-thank-you',
    model: 'welcome',
    category: 'transactional',
    subject: {
      en: 'Thank you for your order, {{contact.first_name}}! 🎉',
      fr: 'Merci pour votre commande, {{contact.first_name}} ! 🎉',
      es: '¡Gracias por tu pedido, {{contact.first_name}}! 🎉'
    },
    preheader: {
      en: "{{contact.first_name}}, your order is confirmed — and you're now a VIP",
      fr: '{{contact.first_name}}, votre commande est confirmée — et vous êtes désormais VIP',
      es: '{{contact.first_name}}, tu pedido está confirmado y ya eres VIP'
    },
    copy: {
      en: {
        common: placedOrder('en'),
        model: {
          title: 'Thank you for your order, {{ contact.first_name }}',
          intro: 'Your order is confirmed and already being prepared: you will get a tracking number as soon as it leaves our warehouse. And this purchase makes you a member of our <b>VIP Club</b>.',
          step1Title: 'Early access',
          step1Text: 'Be first in line for every new release.',
          step2Title: 'Priority support',
          step2Text: 'Our technical team answers you first.',
          step3Title: 'Personal setup',
          step3Text: 'Free one-on-one sessions, whenever you need them.',
          button: 'Discover your VIP perks'
        }
      },
      fr: {
        common: placedOrder('fr'),
        model: {
          title: 'Merci pour votre commande, {{ contact.first_name }}',
          intro: 'Votre commande est confirmée et déjà en préparation : vous recevrez un numéro de suivi dès son départ de notre entrepôt. Et cet achat vous fait entrer dans notre <b>Club VIP</b>.',
          step1Title: 'Accès anticipé',
          step1Text: 'Soyez parmi les premiers à chaque nouveauté.',
          step2Title: 'Assistance prioritaire',
          step2Text: 'Notre équipe technique vous répond en premier.',
          step3Title: 'Configuration personnalisée',
          step3Text: 'Des séances individuelles offertes, quand vous le souhaitez.',
          button: 'Découvrir vos avantages VIP'
        }
      },
      es: {
        common: placedOrder('es'),
        model: {
          title: 'Gracias por tu pedido, {{ contact.first_name }}',
          intro: 'Tu pedido está confirmado y ya lo estamos preparando: recibirás un número de seguimiento en cuanto salga de nuestro almacén. Y esta compra te da entrada a nuestro <b>Club VIP</b>.',
          step1Title: 'Acceso anticipado',
          step1Text: 'Sé de los primeros en cada novedad.',
          step2Title: 'Soporte prioritario',
          step2Text: 'Nuestro equipo técnico te atiende primero.',
          step3Title: 'Configuración personalizada',
          step3Text: 'Sesiones individuales sin coste, cuando las necesites.',
          button: 'Descubrir tus ventajas VIP'
        }
      }
    },
    testData: contact('Noah', 'Baptiste'),
    // A gift rather than the welcome email's laptop: the order is in.
    photos: {
      'hero-welcome.jpg': {
        photo: '1513201099705-a9746e1e201f',
        width: 1200,
        height: 460,
        alt: {
          en: 'A gift box in brown paper tied with a pink ribbon',
          fr: 'Une boîte cadeau en papier kraft nouée d’un ruban rose',
          es: 'Una caja de regalo en papel kraft atada con una cinta rosa'
        }
      }
    }
  }
]

/** One demo template, as the Go seed reads it. */
export interface DemoFixture {
  generated_by: string
  template_id: string
  model: GalleryModelId
  category: string
  default_language: DemoLanguage
  brand: { name: string; logo_url: string; logo_width: number }
  test_data: Record<string, unknown>
  languages: Record<DemoLanguage, { subject: string; subject_preview: string; visual_editor_tree: EmailBlock }>
}

const assets: GalleryAssets = { baseUrl: DEMO_GALLERY_BASE_URL, logoUrl: DEMO_BRAND.logoUrl, logoWidth: DEMO_BRAND.logoWidth }

/** The photo a gallery image URL stands for, undefined when the value is no gallery image. */
function demoPhoto(spec: DemoTemplate, value: unknown): DemoPhoto | undefined {
  if (typeof value !== 'string' || !value.startsWith(`${DEMO_GALLERY_BASE_URL}/`)) return undefined
  const file = value.slice(DEMO_GALLERY_BASE_URL.length + 1)
  const photo = spec.photos?.[file] ?? DEMO_PHOTOS[file]
  if (!photo) throw new Error(`${spec.templateId}: no demo photo for the gallery image ${file}`)
  return photo
}

/** The tree with each gallery image swapped for its photo, and the photo's description as its alt. */
function withPhotos(spec: DemoTemplate, block: EmailBlock, language: DemoLanguage): EmailBlock {
  const node = block as unknown as { attributes?: Record<string, unknown>; children?: EmailBlock[] }
  const photo = demoPhoto(spec, node.attributes?.src)
  return {
    ...block,
    ...(photo ? { attributes: { ...node.attributes, src: unsplash(photo.photo, photo.width, photo.height), alt: photo.alt[language] } } : {}),
    ...(node.children ? { children: node.children.map((child) => withPhotos(spec, child, language)) } : {})
  } as EmailBlock
}

/** Test data with each gallery image URL swapped for its photo's. */
function dataWithPhotos(spec: DemoTemplate, value: unknown): unknown {
  const photo = demoPhoto(spec, value)
  if (photo) return unsplash(photo.photo, photo.width, photo.height)
  if (Array.isArray(value)) return value.map((item) => dataWithPhotos(spec, item))
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, child]) => [key, dataWithPhotos(spec, child)]))
  }
  return value
}

/** Builds one demo template with the gallery's own builders, in each of the demo's languages. */
export function buildDemoFixture(spec: DemoTemplate): DemoFixture {
  const model = galleryModel(spec.model)
  if (!model?.category) throw new Error(`${spec.templateId}: no gallery model ${spec.model} with a category`)

  const languages = {} as DemoFixture['languages']
  let testData: Record<string, unknown> = {}
  for (const language of DEMO_LANGUAGES) {
    const base = withBrand(GALLERY_CONTENT[language], DEMO_BRAND.name)
    const copy = (spec.copy?.[language] ?? {}) as Copy<GalleryModelId>
    const content: GalleryContent = {
      common: { ...base.common, ...copy.common },
      models: {
        ...base.models,
        [spec.model]: { ...base.models[spec.model], ...copy.model, subject: spec.subject[language], preheader: spec.preheader[language] }
      }
    }
    const built = model.build(content, assets)
    languages[language] = { subject: built.subject, subject_preview: built.preheader, visual_editor_tree: withPhotos(spec, built.tree, language) }
    if (language === DEMO_BRAND.defaultLanguage) testData = dataWithPhotos(spec, { ...built.testData, ...spec.testData }) as Record<string, unknown>
  }

  return {
    generated_by: 'console/src/components/templates/gallery/demo/demoFixtures.ts — do not edit, run `npm run demo-fixtures` in console/',
    template_id: spec.templateId,
    model: spec.model,
    category: spec.category ?? model.category,
    default_language: DEMO_BRAND.defaultLanguage,
    brand: { name: DEMO_BRAND.name, logo_url: DEMO_BRAND.logoUrl, logo_width: DEMO_BRAND.logoWidth },
    test_data: testData,
    languages
  }
}

/** The fixture files, by name: their content as committed. */
export function demoFixtureFiles(): Record<string, string> {
  return Object.fromEntries(DEMO_TEMPLATES.map((spec) => [`${spec.templateId}.json`, `${JSON.stringify(buildDemoFixture(spec), null, 2)}\n`]))
}
