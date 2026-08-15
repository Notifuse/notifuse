package service

import (
	"context"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// The templates the demo's automations send.
//
// They live apart from createSampleTemplates because the four templates there
// already carry ~600 lines of bespoke MJML builders between them, and these four
// are all the same shape — heading, paragraph, CTA button, footer. One builder
// driven by a content struct covers all of them, in all three languages.
//
// Two traps are load-bearing here, because CreateTemplate failures are only
// Warn-logged and a template that silently never exists leaves the automation
// referencing it with a dangling template_id:
//   - Template.Name is capped at 32 characters (much tighter than a list's 255).
//   - buildEmailTranslations must be handed only fr and es. "en" is the top-level
//     Email field, so including it would create a duplicate translation.

// The template IDs are the demoTemplate* constants declared alongside the
// automations that reference them: an email node naming a template that was never
// created renders a dangling template_id, and the two halves have to agree.

// automationEmailContents holds one language's copy for the shared automation
// email layout. It is welcomeContent plus buttonHref: the welcome builder
// hardcodes its destination, and these four each point somewhere different.
type automationEmailContents struct {
	lang        string
	title       string
	preview     string
	heading     string
	mainContent string
	buttonText  string
	buttonHref  string
	footerText  string
}

// createAutomationEmailMJMLStructure builds the MJML tree shared by every
// automation email: title/preview in the head, then heading, body copy, CTA
// button, divider and footer in a single column.
func (s *DemoService) createAutomationEmailMJMLStructure(c automationEmailContents) notifuse_mjml.EmailBlock {
	titleContent := c.title
	previewContent := c.preview
	headingContent := c.heading
	mainContentText := c.mainContent
	buttonContent := c.buttonText
	footerContent := c.footerText

	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	previewBase := notifuse_mjml.NewBaseBlock("preview", notifuse_mjml.MJMLComponentMjPreview)
	previewBase.Content = &previewContent
	preview := &notifuse_mjml.MJPreviewBlock{BaseBlock: previewBase}

	headingBase := notifuse_mjml.NewBaseBlock("heading-text", notifuse_mjml.MJMLComponentMjText)
	headingBase.Attributes["font-size"] = "24px"
	headingBase.Attributes["font-weight"] = "bold"
	headingBase.Attributes["color"] = "#1d1d1f"
	headingBase.Content = &headingContent
	heading := &notifuse_mjml.MJTextBlock{BaseBlock: headingBase}

	mainTextBase := notifuse_mjml.NewBaseBlock("main-text", notifuse_mjml.MJMLComponentMjText)
	mainTextBase.Attributes["font-size"] = "16px"
	mainTextBase.Attributes["color"] = "#3a3a3c"
	mainTextBase.Content = &mainContentText
	mainText := &notifuse_mjml.MJTextBlock{BaseBlock: mainTextBase}

	buttonBase := notifuse_mjml.NewBaseBlock("cta-button", notifuse_mjml.MJMLComponentMjButton)
	buttonBase.Attributes["background-color"] = "#0071e3"
	buttonBase.Attributes["color"] = "#ffffff"
	buttonBase.Attributes["font-size"] = "16px"
	buttonBase.Attributes["padding"] = "12px 24px"
	buttonBase.Attributes["border-radius"] = "24px"
	buttonBase.Attributes["href"] = c.buttonHref
	buttonBase.Content = &buttonContent
	button := &notifuse_mjml.MJButtonBlock{BaseBlock: buttonBase}

	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("divider", notifuse_mjml.MJMLComponentMjDivider),
	}

	footerTextBase := notifuse_mjml.NewBaseBlock("footer-text", notifuse_mjml.MJMLComponentMjText)
	footerTextBase.Attributes["font-size"] = "12px"
	footerTextBase.Attributes["color"] = "#86868b"
	footerTextBase.Content = &footerContent
	footerText := &notifuse_mjml.MJTextBlock{BaseBlock: footerTextBase}

	columnBase := notifuse_mjml.NewBaseBlock("main-column", notifuse_mjml.MJMLComponentMjColumn)
	columnBase.Children = []notifuse_mjml.EmailBlock{heading, mainText, button, divider, footerText}
	column := &notifuse_mjml.MJColumnBlock{BaseBlock: columnBase}

	sectionBase := notifuse_mjml.NewBaseBlock("main-section", notifuse_mjml.MJMLComponentMjSection)
	sectionBase.Children = []notifuse_mjml.EmailBlock{column}
	section := &notifuse_mjml.MJSectionBlock{BaseBlock: sectionBase}

	headBase := notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)
	headBase.Children = []notifuse_mjml.EmailBlock{title, preview}
	head := &notifuse_mjml.MJHeadBlock{BaseBlock: headBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Attributes["background-color"] = "#f5f5f7"
	bodyBase.Children = []notifuse_mjml.EmailBlock{section}
	body := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	rootBase := notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml)
	rootBase.Attributes["lang"] = c.lang
	rootBase.Children = []notifuse_mjml.EmailBlock{head, body}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

// Content per template. The demo workspace is an Apple storefront (see
// demo_apple_products.go), so the copy talks about carts of Apple hardware,
// trade-in and delivery rather than generic "products".

func getCartRecoveryAContents() map[string]automationEmailContents {
	return map[string]automationEmailContents{
		"en": {
			lang:        "en",
			title:       "Your cart is still waiting",
			preview:     "{{contact.first_name}}, the items you picked are still saved",
			heading:     "Still thinking it over? 🛒",
			mainContent: "Hi {{contact.first_name}},<br><br>Your cart is still saved, exactly as you left it. Every order ships free, arrives in two days, and comes with a one-year warranty plus 90 days of complimentary technical support.<br><br>Trading in your current device? You can apply its value at checkout and see the new price instantly.",
			buttonText:  "Return to My Cart",
			buttonHref:  "https://demo.notifuse.com/cart?utm_source=notifuse&utm_medium=email&utm_campaign=cart-recovery-a",
			footerText:  "You received this email because you started an order on our store.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com\">Visit the store</a>",
		},
		"fr": {
			lang:        "fr",
			title:       "Votre panier vous attend toujours",
			preview:     "{{contact.first_name}}, les articles que vous avez choisis sont toujours enregistrés",
			heading:     "Vous hésitez encore ? 🛒",
			mainContent: "Bonjour {{contact.first_name}},<br><br>Votre panier est toujours là, exactement comme vous l'avez laissé. Chaque commande bénéficie de la livraison gratuite en deux jours, d'une garantie d'un an et de 90 jours d'assistance technique offerte.<br><br>Vous souhaitez reprendre votre appareil actuel ? Sa valeur est déduite au moment du paiement et le nouveau prix s'affiche immédiatement.",
			buttonText:  "Revenir à mon panier",
			buttonHref:  "https://demo.notifuse.com/cart?utm_source=notifuse&utm_medium=email&utm_campaign=cart-recovery-a",
			footerText:  "Vous recevez cet e-mail car vous avez commencé une commande sur notre boutique.<br><a href=\"{{unsubscribe_url}}\">Se désabonner</a> | <a href=\"https://demo.notifuse.com\">Visiter la boutique</a>",
		},
		"es": {
			lang:        "es",
			title:       "Tu carrito sigue esperando",
			preview:     "{{contact.first_name}}, los artículos que elegiste siguen guardados",
			heading:     "¿Todavía lo estás pensando? 🛒",
			mainContent: "Hola {{contact.first_name}},<br><br>Tu carrito sigue guardado, tal y como lo dejaste. Todos los pedidos incluyen envío gratuito en dos días, un año de garantía y 90 días de soporte técnico sin coste.<br><br>¿Quieres entregar tu dispositivo actual? Puedes aplicar su valor al finalizar la compra y ver el nuevo precio al instante.",
			buttonText:  "Volver a mi carrito",
			buttonHref:  "https://demo.notifuse.com/cart?utm_source=notifuse&utm_medium=email&utm_campaign=cart-recovery-a",
			footerText:  "Recibes este correo porque iniciaste un pedido en nuestra tienda.<br><a href=\"{{unsubscribe_url}}\">Cancelar suscripción</a> | <a href=\"https://demo.notifuse.com\">Visitar la tienda</a>",
		},
	}
}

func getCartRecoveryBContents() map[string]automationEmailContents {
	return map[string]automationEmailContents{
		"en": {
			lang:        "en",
			title:       "10% off the cart you left behind",
			preview:     "{{contact.first_name}}, here is 10% off to finish your order",
			heading:     "Here's 10% off to finish up 🎁",
			mainContent: "Hi {{contact.first_name}},<br><br>Your cart is still saved — and so is a little something extra. Use the code <strong>APPLE10</strong> at checkout for 10% off your order.<br><br>The code is valid for the next 48 hours and works on everything still in your cart, accessories included.",
			buttonText:  "Claim My 10% Off",
			buttonHref:  "https://demo.notifuse.com/cart?promo=APPLE10&utm_source=notifuse&utm_medium=email&utm_campaign=cart-recovery-b",
			footerText:  "You received this email because you started an order on our store.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com\">Visit the store</a>",
		},
		"fr": {
			lang:        "fr",
			title:       "10 % de remise sur le panier que vous avez laissé",
			preview:     "{{contact.first_name}}, voici 10 % de remise pour finaliser votre commande",
			heading:     "Voici 10 % de remise pour conclure 🎁",
			mainContent: "Bonjour {{contact.first_name}},<br><br>Votre panier est toujours enregistré — et un petit plus vous attend. Utilisez le code <strong>APPLE10</strong> au moment du paiement pour bénéficier de 10 % de remise.<br><br>Le code est valable 48 heures et s'applique à tout ce qui se trouve dans votre panier, accessoires compris.",
			buttonText:  "Profiter des 10 %",
			buttonHref:  "https://demo.notifuse.com/cart?promo=APPLE10&utm_source=notifuse&utm_medium=email&utm_campaign=cart-recovery-b",
			footerText:  "Vous recevez cet e-mail car vous avez commencé une commande sur notre boutique.<br><a href=\"{{unsubscribe_url}}\">Se désabonner</a> | <a href=\"https://demo.notifuse.com\">Visiter la boutique</a>",
		},
		"es": {
			lang:        "es",
			title:       "10 % de descuento en el carrito que dejaste",
			preview:     "{{contact.first_name}}, aquí tienes un 10 % para terminar tu pedido",
			heading:     "Un 10 % de descuento para terminar 🎁",
			mainContent: "Hola {{contact.first_name}},<br><br>Tu carrito sigue guardado, y además te hemos reservado algo. Usa el código <strong>APPLE10</strong> al finalizar la compra y obtén un 10 % de descuento.<br><br>El código es válido durante 48 horas y se aplica a todo lo que hay en tu carrito, accesorios incluidos.",
			buttonText:  "Aprovechar el 10 %",
			buttonHref:  "https://demo.notifuse.com/cart?promo=APPLE10&utm_source=notifuse&utm_medium=email&utm_campaign=cart-recovery-b",
			footerText:  "Recibes este correo porque iniciaste un pedido en nuestra tienda.<br><a href=\"{{unsubscribe_url}}\">Cancelar suscripción</a> | <a href=\"https://demo.notifuse.com\">Visitar la tienda</a>",
		},
	}
}

func getOrderThankYouContents() map[string]automationEmailContents {
	return map[string]automationEmailContents{
		"en": {
			lang:        "en",
			title:       "Thank you for your order",
			preview:     "{{contact.first_name}}, your order is confirmed — and you're now a VIP",
			heading:     "Thank you for your order! 🎉",
			mainContent: "Hi {{contact.first_name}},<br><br>Your order is confirmed and already being prepared. You'll get a tracking number as soon as it leaves our warehouse.<br><br>One more thing: this purchase moves you into our <strong>VIP Club</strong>. That means early access to new releases, priority technical support, and free personal setup sessions whenever you need them.",
			buttonText:  "Track My Order",
			buttonHref:  "https://demo.notifuse.com/orders?utm_source=notifuse&utm_medium=email&utm_campaign=order-thank-you",
			footerText:  "This is a transactional message about an order you placed.<br>Questions? Reply to this email and our team will help.",
		},
		"fr": {
			lang:        "fr",
			title:       "Merci pour votre commande",
			preview:     "{{contact.first_name}}, votre commande est confirmée — et vous êtes désormais VIP",
			heading:     "Merci pour votre commande ! 🎉",
			mainContent: "Bonjour {{contact.first_name}},<br><br>Votre commande est confirmée et déjà en préparation. Vous recevrez un numéro de suivi dès son départ de notre entrepôt.<br><br>Autre bonne nouvelle : cet achat vous fait entrer dans notre <strong>Club VIP</strong>. Vous bénéficiez d'un accès anticipé aux nouveautés, d'une assistance technique prioritaire et de séances de configuration personnalisées offertes.",
			buttonText:  "Suivre ma commande",
			buttonHref:  "https://demo.notifuse.com/orders?utm_source=notifuse&utm_medium=email&utm_campaign=order-thank-you",
			footerText:  "Ceci est un message transactionnel concernant une commande que vous avez passée.<br>Une question ? Répondez à cet e-mail et notre équipe vous aidera.",
		},
		"es": {
			lang:        "es",
			title:       "Gracias por tu pedido",
			preview:     "{{contact.first_name}}, tu pedido está confirmado y ya eres VIP",
			heading:     "¡Gracias por tu pedido! 🎉",
			mainContent: "Hola {{contact.first_name}},<br><br>Tu pedido está confirmado y ya lo estamos preparando. Recibirás un número de seguimiento en cuanto salga de nuestro almacén.<br><br>Una cosa más: esta compra te da entrada a nuestro <strong>Club VIP</strong>, con acceso anticipado a las novedades, soporte técnico prioritario y sesiones de configuración personalizadas sin coste.",
			buttonText:  "Seguir mi pedido",
			buttonHref:  "https://demo.notifuse.com/orders?utm_source=notifuse&utm_medium=email&utm_campaign=order-thank-you",
			footerText:  "Este es un mensaje transaccional sobre un pedido que has realizado.<br>¿Alguna duda? Responde a este correo y nuestro equipo te ayudará.",
		},
	}
}

func getWinbackOfferContents() map[string]automationEmailContents {
	return map[string]automationEmailContents{
		"en": {
			lang:        "en",
			title:       "We miss you",
			preview:     "{{contact.first_name}}, a lot has changed since your last visit",
			heading:     "We miss you, {{contact.first_name}} 👋",
			mainContent: "It's been a while, and quite a lot has landed since your last order: a new iPhone lineup, faster MacBooks and the latest Apple Watch.<br><br>To make coming back easy, here's <strong>$50 off</strong> your next order over $499, plus free two-day delivery. No code needed — the discount is already attached to your account.",
			buttonText:  "See What's New",
			buttonHref:  "https://demo.notifuse.com/store?utm_source=notifuse&utm_medium=email&utm_campaign=winback-offer",
			footerText:  "You received this email because you shopped with us before.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com\">Visit the store</a>",
		},
		"fr": {
			lang:        "fr",
			title:       "Vous nous manquez",
			preview:     "{{contact.first_name}}, beaucoup de choses ont changé depuis votre dernière visite",
			heading:     "Vous nous manquez, {{contact.first_name}} 👋",
			mainContent: "Cela fait un moment, et beaucoup de nouveautés sont arrivées depuis votre dernière commande : une nouvelle gamme d'iPhone, des MacBook plus rapides et la dernière Apple Watch.<br><br>Pour faciliter votre retour, voici <strong>50 € de remise</strong> dès 499 € d'achat, ainsi que la livraison offerte en deux jours. Aucun code n'est nécessaire : la remise est déjà associée à votre compte.",
			buttonText:  "Découvrir les nouveautés",
			buttonHref:  "https://demo.notifuse.com/store?utm_source=notifuse&utm_medium=email&utm_campaign=winback-offer",
			footerText:  "Vous recevez cet e-mail car vous avez déjà commandé chez nous.<br><a href=\"{{unsubscribe_url}}\">Se désabonner</a> | <a href=\"https://demo.notifuse.com\">Visiter la boutique</a>",
		},
		"es": {
			lang:        "es",
			title:       "Te echamos de menos",
			preview:     "{{contact.first_name}}, han cambiado muchas cosas desde tu última visita",
			heading:     "Te echamos de menos, {{contact.first_name}} 👋",
			mainContent: "Ha pasado un tiempo y han llegado muchas novedades desde tu último pedido: una nueva gama de iPhone, MacBook más rápidos y el último Apple Watch.<br><br>Para ponértelo fácil, aquí tienes <strong>50 € de descuento</strong> en pedidos superiores a 499 €, con envío gratuito en dos días. No necesitas ningún código: el descuento ya está asociado a tu cuenta.",
			buttonText:  "Ver las novedades",
			buttonHref:  "https://demo.notifuse.com/store?utm_source=notifuse&utm_medium=email&utm_campaign=winback-offer",
			footerText:  "Recibes este correo porque ya has comprado con nosotros.<br><a href=\"{{unsubscribe_url}}\">Cancelar suscripción</a> | <a href=\"https://demo.notifuse.com\">Visitar la tienda</a>",
		},
	}
}

// demoAutomationTemplates builds the four templates the demo's automations
// reference. It is split from createAutomationTemplates so the definitions can be
// inspected without a template service: CreateTemplate only warns on failure, so
// the assertions that matter (name length, ID charset, category, translations)
// have to be made against the definitions themselves.
func (s *DemoService) demoAutomationTemplates(workspaceID string) []*domain.Template {
	now := time.Now()

	// Cart Recovery A — the product-led arm of the cart-recovery A/B test.
	craContents := getCartRecoveryAContents()
	craMJML := s.createAutomationEmailMJMLStructure(craContents["en"])
	craTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Emma",
			"last_name":  "Clarke",
			"email":      "emma.clarke@example.com",
		},
	}
	craHTML := s.compileTemplateToHTML(workspaceID, "cart-recovery-a-preview", craMJML, craTestData)
	craSubjects := map[string]string{
		"fr": "{{contact.first_name}}, votre panier vous attend toujours 🛒",
		"es": "{{contact.first_name}}, tu carrito sigue esperando 🛒",
	}
	craMJMLStructures := map[string]notifuse_mjml.EmailBlock{
		"fr": s.createAutomationEmailMJMLStructure(craContents["fr"]),
		"es": s.createAutomationEmailMJMLStructure(craContents["es"]),
	}
	cartRecoveryA := &domain.Template{
		ID:       demoTemplateCartRecoveryA,
		Name:     "Cart Recovery A",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryMarketing),
		Email: &domain.EmailTemplate{
			Subject:          "{{contact.first_name}}, your cart is still waiting 🛒",
			CompiledPreview:  craHTML,
			VisualEditorTree: craMJML,
		},
		TestData:     craTestData,
		Translations: s.buildEmailTranslations(workspaceID, "cart-recovery-a", craSubjects, craMJMLStructures, craTestData),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Cart Recovery B — the discount-led arm of the same test.
	crbContents := getCartRecoveryBContents()
	crbMJML := s.createAutomationEmailMJMLStructure(crbContents["en"])
	crbTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Liam",
			"last_name":  "Ferreira",
			"email":      "liam.ferreira@example.com",
		},
	}
	crbHTML := s.compileTemplateToHTML(workspaceID, "cart-recovery-b-preview", crbMJML, crbTestData)
	crbSubjects := map[string]string{
		"fr": "{{contact.first_name}}, 10 % de remise sur votre panier 🎁",
		"es": "{{contact.first_name}}, un 10 % de descuento en tu carrito 🎁",
	}
	crbMJMLStructures := map[string]notifuse_mjml.EmailBlock{
		"fr": s.createAutomationEmailMJMLStructure(crbContents["fr"]),
		"es": s.createAutomationEmailMJMLStructure(crbContents["es"]),
	}
	cartRecoveryB := &domain.Template{
		ID:       demoTemplateCartRecoveryB,
		Name:     "Cart Recovery B",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryMarketing),
		Email: &domain.EmailTemplate{
			Subject:          "{{contact.first_name}}, here's 10% off your cart 🎁",
			CompiledPreview:  crbHTML,
			VisualEditorTree: crbMJML,
		},
		TestData:     crbTestData,
		Translations: s.buildEmailTranslations(workspaceID, "cart-recovery-b", crbSubjects, crbMJMLStructures, crbTestData),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Order Thank You — genuinely transactional, so it is not gated by the
	// executor's marketing subscription guard.
	otyContents := getOrderThankYouContents()
	otyMJML := s.createAutomationEmailMJMLStructure(otyContents["en"])
	otyTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Noah",
			"last_name":  "Baptiste",
			"email":      "noah.baptiste@example.com",
		},
	}
	otyHTML := s.compileTemplateToHTML(workspaceID, "order-thank-you-preview", otyMJML, otyTestData)
	otySubjects := map[string]string{
		"fr": "Merci pour votre commande, {{contact.first_name}} ! 🎉",
		"es": "¡Gracias por tu pedido, {{contact.first_name}}! 🎉",
	}
	otyMJMLStructures := map[string]notifuse_mjml.EmailBlock{
		"fr": s.createAutomationEmailMJMLStructure(otyContents["fr"]),
		"es": s.createAutomationEmailMJMLStructure(otyContents["es"]),
	}
	orderThankYou := &domain.Template{
		ID:       demoTemplateOrderThankYou,
		Name:     "Order Thank You",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryTransactional),
		Email: &domain.EmailTemplate{
			Subject:          "Thank you for your order, {{contact.first_name}}! 🎉",
			CompiledPreview:  otyHTML,
			VisualEditorTree: otyMJML,
		},
		TestData:     otyTestData,
		Translations: s.buildEmailTranslations(workspaceID, "order-thank-you", otySubjects, otyMJMLStructures, otyTestData),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Win-back Offer — sent by winback-sunset and by the welcome series'
	// non-active branch.
	wboContents := getWinbackOfferContents()
	wboMJML := s.createAutomationEmailMJMLStructure(wboContents["en"])
	wboTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Olivia",
			"last_name":  "Moreau",
			"email":      "olivia.moreau@example.com",
		},
	}
	wboHTML := s.compileTemplateToHTML(workspaceID, "winback-offer-preview", wboMJML, wboTestData)
	wboSubjects := map[string]string{
		"fr": "Vous nous manquez, {{contact.first_name}} — 50 € pour votre retour 👋",
		"es": "Te echamos de menos, {{contact.first_name}}: 50 € para tu vuelta 👋",
	}
	wboMJMLStructures := map[string]notifuse_mjml.EmailBlock{
		"fr": s.createAutomationEmailMJMLStructure(wboContents["fr"]),
		"es": s.createAutomationEmailMJMLStructure(wboContents["es"]),
	}
	winbackOffer := &domain.Template{
		ID:       demoTemplateWinbackOffer,
		Name:     "Win-back Offer",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryMarketing),
		Email: &domain.EmailTemplate{
			Subject:          "We miss you, {{contact.first_name}} — here's $50 off 👋",
			CompiledPreview:  wboHTML,
			VisualEditorTree: wboMJML,
		},
		TestData:     wboTestData,
		Translations: s.buildEmailTranslations(workspaceID, "winback-offer", wboSubjects, wboMJMLStructures, wboTestData),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	return []*domain.Template{cartRecoveryA, cartRecoveryB, orderThankYou, winbackOffer}
}

// createAutomationTemplates creates the templates the demo's automations send.
//
// A failure is logged and skipped rather than returned, matching
// createSampleTemplates: one rejected template must not cost the demo the rest of
// its seed.
func (s *DemoService) createAutomationTemplates(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating automation templates")

	for _, template := range s.demoAutomationTemplates(workspaceID) {
		if err := s.templateService.CreateTemplate(ctx, workspaceID, template); err != nil {
			s.logger.WithField("template_id", template.ID).WithField("error", err.Error()).
				Warn("Failed to create automation template")
		}
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Automation templates created successfully")
	return nil
}
