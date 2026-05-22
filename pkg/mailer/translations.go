package mailer

import "strings"

// DefaultEmailLanguage is the locale used for system emails when no language is
// configured or when the configured language is not supported.
const DefaultEmailLanguage = "en"

// Translations holds every localized string used by the system emails
// (magic code, workspace invitation and circuit-breaker alert).
type Translations struct {
	// Lang is the canonical locale code for this set (e.g. "en", "pt-BR"),
	// used for the HTML lang attribute.
	Lang           string
	Common         CommonStrings
	MagicCode      MagicCodeStrings
	Invitation     InvitationStrings
	CircuitBreaker CircuitBreakerStrings
}

// CommonStrings holds strings shared across every system email.
type CommonStrings struct {
	Greeting string // greeting line, e.g. "Hello,"
	TeamName string // signature team name, e.g. "The Notifuse Team"
}

// MagicCodeStrings holds the strings for the authentication code email.
type MagicCodeStrings struct {
	Subject      string // email subject
	Heading      string // HTML heading
	Intro        string // sentence introducing the code
	Expiry       string // code expiry notice
	IgnoreNotice string // notice shown when the code was not requested
	SignOff      string // closing line, e.g. "Thanks,"
}

// InvitationStrings holds the strings for the workspace invitation email.
// Subject takes one argument (workspace name). Body takes two indexed
// arguments (%[1]s inviter, %[2]s workspace) so translations may reorder them.
// PlainLink takes one argument (the invitation URL).
type InvitationStrings struct {
	Subject     string
	Heading     string
	Body        string
	ClickPrompt string
	LinkText    string
	FallbackURL string
	PlainLink   string
	Expiry      string
	SignOff     string
}

// CircuitBreakerStrings holds the strings for the broadcast-paused alert email.
// Subject takes one argument (broadcast name). Body takes two indexed arguments
// (%[1]s broadcast, %[2]s workspace) so translations may reorder them.
type CircuitBreakerStrings struct {
	Subject     string
	Heading     string
	Body        string
	ReasonLabel string
	SignOff     string
}

// systemEmailTranslations maps lowercased locale codes to their translation set.
// Keys are stored lowercased so that lookups are case-insensitive ("pt-BR" == "pt-br");
// each set's canonical-cased code lives in its Lang field. This registry is
// deliberately narrower than domain.SupportedLanguages: it lists only the locales
// for which a baked-in system-email translation actually exists.
var systemEmailTranslations = map[string]Translations{
	"en":    englishTranslations,
	"fr":    frenchTranslations,
	"es":    spanishTranslations,
	"de":    germanTranslations,
	"ca":    catalanTranslations,
	"pt-br": portugueseBRTranslations,
	"ja":    japaneseTranslations,
	"it":    italianTranslations,
}

// normalizeLocale lower-cases and trims a locale code for case-insensitive lookup.
func normalizeLocale(lang string) string {
	return strings.ToLower(strings.TrimSpace(lang))
}

// GetTranslations returns the translation set for the given locale, falling back
// to English when the locale is empty or unsupported. Lookup is case-insensitive.
func GetTranslations(lang string) Translations {
	if t, ok := systemEmailTranslations[normalizeLocale(lang)]; ok {
		return t
	}
	return systemEmailTranslations[DefaultEmailLanguage]
}

// IsSupportedLanguage reports whether a translation set exists for the locale.
func IsSupportedLanguage(lang string) bool {
	_, ok := systemEmailTranslations[normalizeLocale(lang)]
	return ok
}

// englishTranslations is the canonical reference; every other locale is
// translated from it. Strings are kept free of HTML markup — the mailer
// decorates the placeholder arguments (e.g. wrapping names in <strong>).
var englishTranslations = Translations{
	Lang: "en",
	Common: CommonStrings{
		Greeting: "Hello,",
		TeamName: "The Notifuse Team",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Your Notifuse authentication code",
		Heading:      "Your authentication code",
		Intro:        "Your authentication code for Notifuse is:",
		Expiry:       "The code will expire in 10 minutes.",
		IgnoreNotice: "If you did not request this code, please ignore this email.",
		SignOff:      "Thanks,",
	},
	Invitation: InvitationStrings{
		Subject:     "You've been invited to join %s on Notifuse",
		Heading:     "You've been invited to join Notifuse!",
		Body:        "%[1]s has invited you to join the %[2]s workspace on Notifuse.",
		ClickPrompt: "Click the link below to join:",
		LinkText:    "Accept invitation",
		FallbackURL: "If the link doesn't work, copy and paste this URL into your browser:",
		PlainLink:   "Use the following link to join: %s",
		Expiry:      "This invitation will expire in 7 days.",
		SignOff:     "Thanks,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Broadcast Paused - %s",
		Heading:     "🚨 Broadcast Automatically Paused",
		Body:        "Your broadcast %[1]s in workspace %[2]s has been automatically paused.",
		ReasonLabel: "Reason:",
		SignOff:     "Best regards,",
	},
}

// frenchTranslations holds the French (fr) system email strings.
var frenchTranslations = Translations{
	Lang: "fr",
	Common: CommonStrings{
		Greeting: "Bonjour,",
		TeamName: "L'équipe Notifuse",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Votre code d'authentification Notifuse",
		Heading:      "Votre code d'authentification",
		Intro:        "Votre code d'authentification pour Notifuse est :",
		Expiry:       "Ce code expirera dans 10 minutes.",
		IgnoreNotice: "Si vous n'avez pas demandé ce code, veuillez ignorer cet e-mail.",
		SignOff:      "Merci,",
	},
	Invitation: InvitationStrings{
		Subject:     "Vous avez été invité à rejoindre %s sur Notifuse",
		Heading:     "Vous avez été invité à rejoindre Notifuse !",
		Body:        "%[1]s vous a invité à rejoindre l'espace de travail %[2]s sur Notifuse.",
		ClickPrompt: "Cliquez sur le lien ci-dessous pour rejoindre :",
		LinkText:    "Accepter l'invitation",
		FallbackURL: "Si le lien ne fonctionne pas, copiez et collez cette URL dans votre navigateur :",
		PlainLink:   "Utilisez le lien suivant pour rejoindre : %s",
		Expiry:      "Cette invitation expirera dans 7 jours.",
		SignOff:     "Merci,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Diffusion en pause - %s",
		Heading:     "🚨 Diffusion automatiquement mise en pause",
		Body:        "Votre diffusion %[1]s dans l'espace de travail %[2]s a été automatiquement mise en pause.",
		ReasonLabel: "Raison :",
		SignOff:     "Cordialement,",
	},
}

// spanishTranslations holds the Spanish (es) system email strings.
var spanishTranslations = Translations{
	Lang: "es",
	Common: CommonStrings{
		Greeting: "Hola,",
		TeamName: "El equipo de Notifuse",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Tu código de autenticación de Notifuse",
		Heading:      "Tu código de autenticación",
		Intro:        "Tu código de autenticación para Notifuse es:",
		Expiry:       "El código caducará en 10 minutos.",
		IgnoreNotice: "Si no solicitaste este código, ignora este correo electrónico.",
		SignOff:      "Gracias,",
	},
	Invitation: InvitationStrings{
		Subject:     "Te han invitado a unirte a %s en Notifuse",
		Heading:     "¡Te han invitado a unirte a Notifuse!",
		Body:        "%[1]s te ha invitado a unirte al espacio de trabajo %[2]s en Notifuse.",
		ClickPrompt: "Haz clic en el siguiente enlace para unirte:",
		LinkText:    "Aceptar invitación",
		FallbackURL: "Si el enlace no funciona, copia y pega esta URL en tu navegador:",
		PlainLink:   "Usa el siguiente enlace para unirte: %s",
		Expiry:      "Esta invitación caducará en 7 días.",
		SignOff:     "Gracias,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Difusión en pausa - %s",
		Heading:     "🚨 Difusión pausada automáticamente",
		Body:        "Tu difusión %[1]s en el espacio de trabajo %[2]s se ha pausado automáticamente.",
		ReasonLabel: "Motivo:",
		SignOff:     "Un saludo,",
	},
}

// germanTranslations holds the German (de) system email strings.
var germanTranslations = Translations{
	Lang: "de",
	Common: CommonStrings{
		Greeting: "Hallo,",
		TeamName: "Das Notifuse-Team",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Ihr Notifuse-Authentifizierungscode",
		Heading:      "Ihr Authentifizierungscode",
		Intro:        "Ihr Authentifizierungscode für Notifuse lautet:",
		Expiry:       "Der Code läuft in 10 Minuten ab.",
		IgnoreNotice: "Wenn Sie diesen Code nicht angefordert haben, ignorieren Sie diese E-Mail bitte.",
		SignOff:      "Danke,",
	},
	Invitation: InvitationStrings{
		Subject:     "Sie wurden eingeladen, %s auf Notifuse beizutreten",
		Heading:     "Sie wurden zu Notifuse eingeladen!",
		Body:        "%[1]s hat Sie eingeladen, dem Workspace %[2]s auf Notifuse beizutreten.",
		ClickPrompt: "Klicken Sie auf den folgenden Link, um beizutreten:",
		LinkText:    "Einladung annehmen",
		FallbackURL: "Wenn der Link nicht funktioniert, kopieren Sie diese URL und fügen Sie sie in Ihren Browser ein:",
		PlainLink:   "Verwenden Sie den folgenden Link, um beizutreten: %s",
		Expiry:      "Diese Einladung läuft in 7 Tagen ab.",
		SignOff:     "Danke,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Broadcast pausiert - %s",
		Heading:     "🚨 Broadcast automatisch pausiert",
		Body:        "Ihr Broadcast %[1]s im Workspace %[2]s wurde automatisch pausiert.",
		ReasonLabel: "Grund:",
		SignOff:     "Mit freundlichen Grüßen,",
	},
}

// catalanTranslations holds the Catalan (ca) system email strings.
var catalanTranslations = Translations{
	Lang: "ca",
	Common: CommonStrings{
		Greeting: "Hola,",
		TeamName: "L'equip de Notifuse",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "El teu codi d'autenticació de Notifuse",
		Heading:      "El teu codi d'autenticació",
		Intro:        "El teu codi d'autenticació per a Notifuse és:",
		Expiry:       "El codi caducarà en 10 minuts.",
		IgnoreNotice: "Si no has sol·licitat aquest codi, ignora aquest correu electrònic.",
		SignOff:      "Gràcies,",
	},
	Invitation: InvitationStrings{
		Subject:     "T'han convidat a unir-te a %s a Notifuse",
		Heading:     "T'han convidat a unir-te a Notifuse!",
		Body:        "%[1]s t'ha convidat a unir-te a l'espai de treball %[2]s a Notifuse.",
		ClickPrompt: "Fes clic a l'enllaç següent per unir-t'hi:",
		LinkText:    "Accepta la invitació",
		FallbackURL: "Si l'enllaç no funciona, copia i enganxa aquest URL al teu navegador:",
		PlainLink:   "Utilitza l'enllaç següent per unir-t'hi: %s",
		Expiry:      "Aquesta invitació caducarà en 7 dies.",
		SignOff:     "Gràcies,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Difusió en pausa - %s",
		Heading:     "🚨 Difusió pausada automàticament",
		Body:        "La teva difusió %[1]s a l'espai de treball %[2]s s'ha pausat automàticament.",
		ReasonLabel: "Motiu:",
		SignOff:     "Salutacions cordials,",
	},
}

// portugueseBRTranslations holds the Brazilian Portuguese (pt-BR) system email strings.
var portugueseBRTranslations = Translations{
	Lang: "pt-BR",
	Common: CommonStrings{
		Greeting: "Olá,",
		TeamName: "A equipe da Notifuse",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Seu código de autenticação da Notifuse",
		Heading:      "Seu código de autenticação",
		Intro:        "Seu código de autenticação para a Notifuse é:",
		Expiry:       "O código expirará em 10 minutos.",
		IgnoreNotice: "Se você não solicitou este código, ignore este e-mail.",
		SignOff:      "Obrigado,",
	},
	Invitation: InvitationStrings{
		Subject:     "Você foi convidado para participar de %s na Notifuse",
		Heading:     "Você foi convidado para participar da Notifuse!",
		Body:        "%[1]s convidou você para participar do espaço de trabalho %[2]s na Notifuse.",
		ClickPrompt: "Clique no link abaixo para participar:",
		LinkText:    "Aceitar convite",
		FallbackURL: "Se o link não funcionar, copie e cole este URL no seu navegador:",
		PlainLink:   "Use o link a seguir para participar: %s",
		Expiry:      "Este convite expirará em 7 dias.",
		SignOff:     "Obrigado,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Transmissão pausada - %s",
		Heading:     "🚨 Transmissão pausada automaticamente",
		Body:        "Sua transmissão %[1]s no espaço de trabalho %[2]s foi pausada automaticamente.",
		ReasonLabel: "Motivo:",
		SignOff:     "Atenciosamente,",
	},
}

// japaneseTranslations holds the Japanese (ja) system email strings.
var japaneseTranslations = Translations{
	Lang: "ja",
	Common: CommonStrings{
		Greeting: "こんにちは、",
		TeamName: "Notifuse チーム",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Notifuse 認証コード",
		Heading:      "認証コード",
		Intro:        "Notifuse の認証コードは次のとおりです:",
		Expiry:       "このコードは10分後に有効期限が切れます。",
		IgnoreNotice: "このコードをリクエストしていない場合は、このメールを無視してください。",
		SignOff:      "よろしくお願いいたします、",
	},
	Invitation: InvitationStrings{
		Subject:     "%s に参加するよう招待されました（Notifuse）",
		Heading:     "Notifuse への参加に招待されました！",
		Body:        "%[1]s さんが、Notifuse のワークスペース %[2]s への参加にあなたを招待しました。",
		ClickPrompt: "参加するには、以下のリンクをクリックしてください:",
		LinkText:    "招待を承認する",
		FallbackURL: "リンクが機能しない場合は、この URL をコピーしてブラウザに貼り付けてください:",
		PlainLink:   "参加するには次のリンクを使用してください: %s",
		Expiry:      "この招待は7日後に有効期限が切れます。",
		SignOff:     "よろしくお願いいたします、",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 ブロードキャストが一時停止されました - %s",
		Heading:     "🚨 ブロードキャストが自動的に一時停止されました",
		Body:        "ワークスペース %[2]s のブロードキャスト %[1]s が自動的に一時停止されました。",
		ReasonLabel: "理由:",
		SignOff:     "よろしくお願いいたします、",
	},
}

// italianTranslations holds the Italian (it) system email strings.
var italianTranslations = Translations{
	Lang: "it",
	Common: CommonStrings{
		Greeting: "Ciao,",
		TeamName: "Il team di Notifuse",
	},
	MagicCode: MagicCodeStrings{
		Subject:      "Il tuo codice di autenticazione Notifuse",
		Heading:      "Il tuo codice di autenticazione",
		Intro:        "Il tuo codice di autenticazione per Notifuse è:",
		Expiry:       "Il codice scadrà tra 10 minuti.",
		IgnoreNotice: "Se non hai richiesto questo codice, ignora questa email.",
		SignOff:      "Grazie,",
	},
	Invitation: InvitationStrings{
		Subject:     "Sei stato invitato a unirti a %s su Notifuse",
		Heading:     "Sei stato invitato a unirti a Notifuse!",
		Body:        "%[1]s ti ha invitato a unirti allo spazio di lavoro %[2]s su Notifuse.",
		ClickPrompt: "Fai clic sul link sottostante per unirti:",
		LinkText:    "Accetta l'invito",
		FallbackURL: "Se il link non funziona, copia e incolla questo URL nel tuo browser:",
		PlainLink:   "Usa il seguente link per unirti: %s",
		Expiry:      "Questo invito scadrà tra 7 giorni.",
		SignOff:     "Grazie,",
	},
	CircuitBreaker: CircuitBreakerStrings{
		Subject:     "🚨 Trasmissione in pausa - %s",
		Heading:     "🚨 Trasmissione messa in pausa automaticamente",
		Body:        "La tua trasmissione %[1]s nello spazio di lavoro %[2]s è stata messa in pausa automaticamente.",
		ReasonLabel: "Motivo:",
		SignOff:     "Cordiali saluti,",
	},
}
