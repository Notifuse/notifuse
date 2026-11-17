package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// The templates the demo's automations send.
//
// Three are built from the template gallery (demo_gallery_templates.go). Order
// Thank You is not: no gallery model fits the data its sends carry, so it keeps
// the one-column builder below — heading, paragraph, CTA button, footer — in all
// three languages.
//
// Two traps are load-bearing here, because a template that silently never exists
// leaves the automation referencing it with a dangling template_id:
//   - Template.Name is capped at 255 characters and Template.ID at 32.
//   - buildEmailTranslations must be handed only fr and es. "en" is the top-level
//     Email field, so including it would create a duplicate translation.

// The template IDs are the demoTemplate* constants declared alongside the
// automations that reference them: an email node naming a template that was never
// created renders a dangling template_id, and the two halves have to agree.

// automationEmailContents holds one language's copy for the one-column
// automation email layout.
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
	headingBase.Attributes["fontSize"] = "24px"
	headingBase.Attributes["fontWeight"] = "bold"
	headingBase.Attributes["color"] = "#1d1d1f"
	headingBase.Content = &headingContent
	heading := &notifuse_mjml.MJTextBlock{BaseBlock: headingBase}

	mainTextBase := notifuse_mjml.NewBaseBlock("main-text", notifuse_mjml.MJMLComponentMjText)
	mainTextBase.Attributes["fontSize"] = "16px"
	mainTextBase.Attributes["color"] = "#3a3a3c"
	mainTextBase.Content = &mainContentText
	mainText := &notifuse_mjml.MJTextBlock{BaseBlock: mainTextBase}

	buttonBase := notifuse_mjml.NewBaseBlock("cta-button", notifuse_mjml.MJMLComponentMjButton)
	buttonBase.Attributes["backgroundColor"] = "#0071e3"
	buttonBase.Attributes["color"] = "#ffffff"
	buttonBase.Attributes["fontSize"] = "16px"
	buttonBase.Attributes["innerPadding"] = "12px 24px"
	buttonBase.Attributes["borderRadius"] = "24px"
	buttonBase.Attributes["href"] = c.buttonHref
	buttonBase.Content = &buttonContent
	button := &notifuse_mjml.MJButtonBlock{BaseBlock: buttonBase}

	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("divider", notifuse_mjml.MJMLComponentMjDivider),
	}

	footerTextBase := notifuse_mjml.NewBaseBlock("footer-text", notifuse_mjml.MJMLComponentMjText)
	footerTextBase.Attributes["fontSize"] = "12px"
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
	bodyBase.Attributes["backgroundColor"] = "#f5f5f7"
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

// demoAutomationTemplates builds the four templates the demo's automations
// reference. It is split from createAutomationTemplates so the definitions can be
// inspected without a template service: the assertions that matter (name length,
// ID charset, category, translations) are made against the definitions themselves.
//
// Cart Recovery A and B (the two arms of the cart-recovery A/B test) and the
// Win-back Offer are built from the template gallery (demo_gallery_templates.go);
// Order Thank You keeps the shared builder above.
func (s *DemoService) demoAutomationTemplates(workspaceID string) ([]*domain.Template, error) {
	now := time.Now()

	templates := make([]*domain.Template, 0, len(demoGalleryAutomationTemplates)+1)
	for _, spec := range demoGalleryAutomationTemplates {
		template, err := s.buildDemoGalleryTemplate(workspaceID, spec)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
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

	return append(templates, orderThankYou), nil
}

// createAutomationTemplates creates the templates the demo's automations send.
//
// A failure is logged and skipped rather than returned, matching
// createSampleTemplates: one rejected template must not cost the demo the rest of
// its seed.
func (s *DemoService) createAutomationTemplates(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating automation templates")

	templates, err := s.demoAutomationTemplates(workspaceID)
	if err != nil {
		return fmt.Errorf("demo: automation templates: %w", err)
	}
	for _, template := range templates {
		if err := s.templateService.CreateTemplate(ctx, workspaceID, withSubjectPreviews(template)); err != nil {
			return fmt.Errorf("demo: automation template %s: %w", template.ID, err)
		}
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Automation templates created successfully")
	return nil
}

// mjPreviewText returns the content of the tree's mj-preview block, nil when it has none.
func mjPreviewText(block notifuse_mjml.EmailBlock) *string {
	if block == nil {
		return nil
	}
	if block.GetType() == notifuse_mjml.MJMLComponentMjPreview {
		if content := block.GetContent(); content != nil && *content != "" {
			text := *content
			return &text
		}
		return nil
	}
	for _, child := range block.GetChildren() {
		if text := mjPreviewText(child); text != nil {
			return text
		}
	}
	return nil
}

// withSubjectPreviews gives a demo template, and each of its translations, the preview text its
// mj-preview block already holds. The console requires a preview text in every language: without
// it, a duplicated demo template could not be saved until each one was typed in again.
func withSubjectPreviews(template *domain.Template) *domain.Template {
	fill := func(email *domain.EmailTemplate) {
		if email != nil && email.SubjectPreview == nil {
			email.SubjectPreview = mjPreviewText(email.VisualEditorTree)
		}
	}
	fill(template.Email)
	for lang, translation := range template.Translations {
		fill(translation.Email)
		template.Translations[lang] = translation
	}
	return template
}
