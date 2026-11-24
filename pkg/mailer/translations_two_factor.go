package mailer

// TwoFactorStrings holds the strings of the security notices sent about two-factor
// authentication. Each notice has a subject and a body; RecoveryCodeUsedBody takes one
// indexed argument (%[1]d, the codes left). The labels introduce the details added under
// the body. Strings carry no HTML: the mailer escapes and decorates them.
type TwoFactorStrings struct {
	EnabledSubject          string
	EnabledBody             string
	DisabledSubject         string
	DisabledBody            string
	ReplacedSubject         string
	ReplacedBody            string
	RecoveryCodeUsedSubject string
	RecoveryCodeUsedBody    string
	RecoveryCodesNewSubject string
	RecoveryCodesNewBody    string
	ResetSubject            string
	ResetBody               string
	FailuresSubject         string
	FailuresBody            string
	WhenLabel               string
	IPLabel                 string
	BrowserLabel            string
	NotYouNotice            string
	SignOff                 string
	UnknownValue            string
}

// twoFactorTranslations maps lowercased locale codes to their security notice strings,
// for the locales of systemEmailTranslations. init copies each set into its Translations.
var twoFactorTranslations = map[string]TwoFactorStrings{
	"en": {
		EnabledSubject:          "Two-factor authentication is on for your {brand} account",
		EnabledBody:             "Two-factor authentication was turned on for your account. From now on, after the code we email you, {brand} asks for a code from your authenticator app.",
		DisabledSubject:         "Two-factor authentication is off for your {brand} account",
		DisabledBody:            "Two-factor authentication was turned off for your account. {brand} now asks only for the code we email you.",
		ReplacedSubject:         "Your {brand} authenticator app was replaced",
		ReplacedBody:            "The authenticator app of your account was replaced. From now on, {brand} accepts only codes from the new app. Your recovery codes still work.",
		RecoveryCodeUsedSubject: "A recovery code was used to sign in to {brand}",
		RecoveryCodeUsedBody:    "Someone signed in to your account with one of your recovery codes. You have %[1]d left.",
		RecoveryCodesNewSubject: "New recovery codes for your {brand} account",
		RecoveryCodesNewBody:    "New recovery codes were created for your account. The previous ones no longer work.",
		ResetSubject:            "Two-factor authentication was reset on your {brand} account",
		ResetBody:               "A {brand} administrator reset two-factor authentication on your account and signed you out everywhere. If your {brand} instance requires it, you will set it up again at your next sign-in.",
		FailuresSubject:         "Wrong authentication codes were entered on your {brand} account",
		FailuresBody:            "Several wrong authentication codes were entered for your account, after a correct code sent to your email. Someone may be able to read your mailbox. If this wasn't you, change your email password and turn on two-factor authentication for your mailbox.",
		WhenLabel:               "When:",
		IPLabel:                 "IP address given by the request:",
		BrowserLabel:            "Browser:",
		NotYouNotice:            "If this wasn't you, contact your {brand} administrator.",
		SignOff:                 "Thanks,",
		UnknownValue:            "unknown",
	},
	"fr": {
		EnabledSubject:          "La double authentification est activée sur votre compte {brand}",
		EnabledBody:             "La double authentification a été activée sur votre compte. Désormais, après le code envoyé par email, {brand} demande un code de votre application d'authentification.",
		DisabledSubject:         "La double authentification est désactivée sur votre compte {brand}",
		DisabledBody:            "La double authentification a été désactivée sur votre compte. {brand} ne demande plus que le code envoyé par email.",
		ReplacedSubject:         "L'application d'authentification de votre compte {brand} a été remplacée",
		ReplacedBody:            "L'application d'authentification de votre compte a été remplacée. {brand} n'accepte plus que les codes de la nouvelle application. Vos codes de récupération restent valables.",
		RecoveryCodeUsedSubject: "Un code de récupération a servi à se connecter à {brand}",
		RecoveryCodeUsedBody:    "Quelqu'un s'est connecté à votre compte avec l'un de vos codes de récupération. Il vous en reste %[1]d.",
		RecoveryCodesNewSubject: "Nouveaux codes de récupération pour votre compte {brand}",
		RecoveryCodesNewBody:    "De nouveaux codes de récupération ont été créés pour votre compte. Les anciens ne fonctionnent plus.",
		ResetSubject:            "La double authentification de votre compte {brand} a été réinitialisée",
		ResetBody:               "Un administrateur {brand} a réinitialisé la double authentification de votre compte et vous a déconnecté partout. Si votre instance {brand} l'exige, vous la configurerez de nouveau à votre prochaine connexion.",
		FailuresSubject:         "Des codes d'authentification erronés ont été saisis sur votre compte {brand}",
		FailuresBody:            "Plusieurs codes d'authentification erronés ont été saisis sur votre compte, après un code correct envoyé à votre adresse email. Quelqu'un peut peut-être lire votre boîte mail. Si ce n'était pas vous, changez le mot de passe de votre messagerie et activez sa double authentification.",
		WhenLabel:               "Quand :",
		IPLabel:                 "Adresse IP indiquée par la requête :",
		BrowserLabel:            "Navigateur :",
		NotYouNotice:            "Si ce n'était pas vous, contactez votre administrateur {brand}.",
		SignOff:                 "Merci,",
		UnknownValue:            "inconnu",
	},
	"es": {
		EnabledSubject:          "La autenticación en dos pasos está activada en tu cuenta de {brand}",
		EnabledBody:             "Se activó la autenticación en dos pasos en tu cuenta. A partir de ahora, después del código que te enviamos por email, {brand} pide un código de tu aplicación de autenticación.",
		DisabledSubject:         "La autenticación en dos pasos está desactivada en tu cuenta de {brand}",
		DisabledBody:            "Se desactivó la autenticación en dos pasos en tu cuenta. {brand} ya solo pide el código que te enviamos por email.",
		ReplacedSubject:         "Se ha sustituido la aplicación de autenticación de tu cuenta de {brand}",
		ReplacedBody:            "Se ha sustituido la aplicación de autenticación de tu cuenta. A partir de ahora, {brand} solo acepta los códigos de la nueva aplicación. Tus códigos de recuperación siguen funcionando.",
		RecoveryCodeUsedSubject: "Se usó un código de recuperación para iniciar sesión en {brand}",
		RecoveryCodeUsedBody:    "Alguien inició sesión en tu cuenta con uno de tus códigos de recuperación. Te quedan %[1]d.",
		RecoveryCodesNewSubject: "Nuevos códigos de recuperación para tu cuenta de {brand}",
		RecoveryCodesNewBody:    "Se crearon nuevos códigos de recuperación para tu cuenta. Los anteriores ya no funcionan.",
		ResetSubject:            "Se restableció la autenticación en dos pasos de tu cuenta de {brand}",
		ResetBody:               "Un administrador de {brand} restableció la autenticación en dos pasos de tu cuenta y cerró todas tus sesiones. Si tu instancia de {brand} la exige, la configurarás de nuevo en tu próximo inicio de sesión.",
		FailuresSubject:         "Se introdujeron códigos de autenticación incorrectos en tu cuenta de {brand}",
		FailuresBody:            "Se introdujeron varios códigos de autenticación incorrectos en tu cuenta, después de un código correcto enviado a tu email. Es posible que alguien pueda leer tu buzón. Si no fuiste tú, cambia la contraseña de tu correo y activa su autenticación en dos pasos.",
		WhenLabel:               "Cuándo:",
		IPLabel:                 "Dirección IP indicada por la solicitud:",
		BrowserLabel:            "Navegador:",
		NotYouNotice:            "Si no fuiste tú, contacta con tu administrador de {brand}.",
		SignOff:                 "Gracias,",
		UnknownValue:            "desconocido",
	},
	"de": {
		EnabledSubject:          "Die Zwei-Faktor-Authentifizierung ist für Ihr {brand}-Konto aktiviert",
		EnabledBody:             "Für Ihr Konto wurde die Zwei-Faktor-Authentifizierung aktiviert. Ab jetzt fragt {brand} nach dem Code aus der E-Mail zusätzlich einen Code aus Ihrer Authenticator-App ab.",
		DisabledSubject:         "Die Zwei-Faktor-Authentifizierung ist für Ihr {brand}-Konto deaktiviert",
		DisabledBody:            "Für Ihr Konto wurde die Zwei-Faktor-Authentifizierung deaktiviert. {brand} fragt nur noch den Code ab, den wir Ihnen per E-Mail senden.",
		ReplacedSubject:         "Die Authenticator-App Ihres {brand}-Kontos wurde ersetzt",
		ReplacedBody:            "Die Authenticator-App Ihres Kontos wurde ersetzt. Ab jetzt akzeptiert {brand} nur noch Codes der neuen App. Ihre Wiederherstellungscodes gelten weiter.",
		RecoveryCodeUsedSubject: "Ein Wiederherstellungscode wurde für die Anmeldung bei {brand} verwendet",
		RecoveryCodeUsedBody:    "Jemand hat sich mit einem Ihrer Wiederherstellungscodes bei Ihrem Konto angemeldet. Sie haben noch %[1]d.",
		RecoveryCodesNewSubject: "Neue Wiederherstellungscodes für Ihr {brand}-Konto",
		RecoveryCodesNewBody:    "Für Ihr Konto wurden neue Wiederherstellungscodes erstellt. Die bisherigen funktionieren nicht mehr.",
		ResetSubject:            "Die Zwei-Faktor-Authentifizierung Ihres {brand}-Kontos wurde zurückgesetzt",
		ResetBody:               "Ein {brand}-Administrator hat die Zwei-Faktor-Authentifizierung Ihres Kontos zurückgesetzt und Sie überall abgemeldet. Wenn Ihre {brand}-Instanz sie verlangt, richten Sie sie bei der nächsten Anmeldung neu ein.",
		FailuresSubject:         "Für Ihr {brand}-Konto wurden falsche Authentifizierungscodes eingegeben",
		FailuresBody:            "Für Ihr Konto wurden mehrere falsche Authentifizierungscodes eingegeben, nachdem ein richtiger Code aus Ihrer E-Mail verwendet wurde. Möglicherweise kann jemand Ihr Postfach lesen. Wenn Sie das nicht waren, ändern Sie das Passwort Ihres E-Mail-Kontos und aktivieren Sie dort die Zwei-Faktor-Authentifizierung.",
		WhenLabel:               "Wann:",
		IPLabel:                 "Von der Anfrage angegebene IP-Adresse:",
		BrowserLabel:            "Browser:",
		NotYouNotice:            "Wenn Sie das nicht waren, wenden Sie sich an Ihren {brand}-Administrator.",
		SignOff:                 "Danke,",
		UnknownValue:            "unbekannt",
	},
	"ca": {
		EnabledSubject:          "L'autenticació en dos passos està activada al teu compte de {brand}",
		EnabledBody:             "S'ha activat l'autenticació en dos passos al teu compte. A partir d'ara, després del codi que t'enviem per correu, {brand} demana un codi de la teva aplicació d'autenticació.",
		DisabledSubject:         "L'autenticació en dos passos està desactivada al teu compte de {brand}",
		DisabledBody:            "S'ha desactivat l'autenticació en dos passos al teu compte. {brand} ja només demana el codi que t'enviem per correu.",
		ReplacedSubject:         "S'ha substituït l'aplicació d'autenticació del teu compte de {brand}",
		ReplacedBody:            "S'ha substituït l'aplicació d'autenticació del teu compte. A partir d'ara, {brand} només accepta els codis de la nova aplicació. Els teus codis de recuperació continuen funcionant.",
		RecoveryCodeUsedSubject: "S'ha fet servir un codi de recuperació per iniciar sessió a {brand}",
		RecoveryCodeUsedBody:    "Algú ha iniciat sessió al teu compte amb un dels teus codis de recuperació. Te'n queden %[1]d.",
		RecoveryCodesNewSubject: "Codis de recuperació nous per al teu compte de {brand}",
		RecoveryCodesNewBody:    "S'han creat codis de recuperació nous per al teu compte. Els anteriors ja no funcionen.",
		ResetSubject:            "S'ha restablert l'autenticació en dos passos del teu compte de {brand}",
		ResetBody:               "Un administrador de {brand} ha restablert l'autenticació en dos passos del teu compte i ha tancat totes les teves sessions. Si la teva instància de {brand} l'exigeix, la tornaràs a configurar en el proper inici de sessió.",
		FailuresSubject:         "S'han introduït codis d'autenticació incorrectes al teu compte de {brand}",
		FailuresBody:            "S'han introduït diversos codis d'autenticació incorrectes al teu compte, després d'un codi correcte enviat al teu correu. És possible que algú pugui llegir la teva bústia. Si no has estat tu, canvia la contrasenya del teu correu i activa'n l'autenticació en dos passos.",
		WhenLabel:               "Quan:",
		IPLabel:                 "Adreça IP indicada per la sol·licitud:",
		BrowserLabel:            "Navegador:",
		NotYouNotice:            "Si no has estat tu, posa't en contacte amb l'administrador de {brand}.",
		SignOff:                 "Gràcies,",
		UnknownValue:            "desconegut",
	},
	"pt-br": {
		EnabledSubject:          "A autenticação em dois fatores está ativada na sua conta em {brand}",
		EnabledBody:             "A autenticação em dois fatores foi ativada na sua conta. A partir de agora, depois do código que enviamos por email, {brand} pede um código do seu aplicativo autenticador.",
		DisabledSubject:         "A autenticação em dois fatores está desativada na sua conta em {brand}",
		DisabledBody:            "A autenticação em dois fatores foi desativada na sua conta. {brand} agora pede apenas o código que enviamos por email.",
		ReplacedSubject:         "O app autenticador da sua conta em {brand} foi substituído",
		ReplacedBody:            "O app autenticador da sua conta foi substituído. A partir de agora, {brand} só aceita os códigos do novo app. Seus códigos de recuperação continuam valendo.",
		RecoveryCodeUsedSubject: "Um código de recuperação foi usado para entrar em {brand}",
		RecoveryCodeUsedBody:    "Alguém entrou na sua conta com um dos seus códigos de recuperação. Restam %[1]d.",
		RecoveryCodesNewSubject: "Novos códigos de recuperação para sua conta em {brand}",
		RecoveryCodesNewBody:    "Novos códigos de recuperação foram criados para sua conta. Os anteriores não funcionam mais.",
		ResetSubject:            "A autenticação em dois fatores da sua conta em {brand} foi redefinida",
		ResetBody:               "Um administrador de {brand} redefiniu a autenticação em dois fatores da sua conta e encerrou todas as suas sessões. Se a sua instância de {brand} exigir, você vai configurá-la de novo no próximo login.",
		FailuresSubject:         "Códigos de autenticação errados foram digitados na sua conta em {brand}",
		FailuresBody:            "Vários códigos de autenticação errados foram digitados na sua conta, depois de um código correto enviado ao seu email. Alguém pode estar conseguindo ler sua caixa de entrada. Se não foi você, troque a senha do seu email e ative a autenticação em dois fatores dele.",
		WhenLabel:               "Quando:",
		IPLabel:                 "Endereço IP informado pela requisição:",
		BrowserLabel:            "Navegador:",
		NotYouNotice:            "Se não foi você, fale com o administrador de {brand}.",
		SignOff:                 "Obrigado,",
		UnknownValue:            "desconhecido",
	},
	"ja": {
		EnabledSubject:          "{brand} アカウントで二要素認証が有効になりました",
		EnabledBody:             "お使いのアカウントで二要素認証が有効になりました。今後は、メールで届くコードに加えて、認証アプリのコードの入力が求められます。",
		DisabledSubject:         "{brand} アカウントで二要素認証が無効になりました",
		DisabledBody:            "お使いのアカウントで二要素認証が無効になりました。今後はメールで届くコードだけでサインインできます。",
		ReplacedSubject:         "{brand} アカウントの認証アプリが置き換えられました",
		ReplacedBody:            "アカウントの認証アプリが置き換えられました。今後、{brand} は新しいアプリのコードのみを受け付けます。リカバリーコードは引き続き使えます。",
		RecoveryCodeUsedSubject: "リカバリーコードで {brand} にサインインしました",
		RecoveryCodeUsedBody:    "リカバリーコードの一つを使って、お使いのアカウントにサインインがありました。残りは %[1]d 個です。",
		RecoveryCodesNewSubject: "{brand} アカウントの新しいリカバリーコード",
		RecoveryCodesNewBody:    "お使いのアカウントに新しいリカバリーコードが作成されました。以前のコードは使えなくなりました。",
		ResetSubject:            "{brand} アカウントの二要素認証がリセットされました",
		ResetBody:               "{brand} の管理者がお使いのアカウントの二要素認証をリセットし、すべてのセッションからサインアウトしました。{brand} インスタンスで必須の場合は、次回のサインイン時に再設定してください。",
		FailuresSubject:         "{brand} アカウントで誤った認証コードが入力されました",
		FailuresBody:            "メールで届いた正しいコードの後に、お使いのアカウントで誤った認証コードが何度も入力されました。第三者がメールボックスを読める状態にある可能性があります。心当たりがない場合は、メールのパスワードを変更し、メールアカウントの二要素認証を有効にしてください。",
		WhenLabel:               "日時:",
		IPLabel:                 "リクエストが示した IP アドレス:",
		BrowserLabel:            "ブラウザ:",
		NotYouNotice:            "心当たりがない場合は、{brand} の管理者に連絡してください。",
		SignOff:                 "よろしくお願いいたします。",
		UnknownValue:            "不明",
	},
	"it": {
		EnabledSubject:          "L'autenticazione a due fattori è attiva sul tuo account {brand}",
		EnabledBody:             "L'autenticazione a due fattori è stata attivata sul tuo account. D'ora in poi, dopo il codice che ti inviamo via email, {brand} chiede un codice della tua app di autenticazione.",
		DisabledSubject:         "L'autenticazione a due fattori è disattivata sul tuo account {brand}",
		DisabledBody:            "L'autenticazione a due fattori è stata disattivata sul tuo account. Ora {brand} chiede solo il codice che ti inviamo via email.",
		ReplacedSubject:         "L'app di autenticazione del tuo account {brand} è stata sostituita",
		ReplacedBody:            "L'app di autenticazione del tuo account è stata sostituita. Da ora {brand} accetta solo i codici della nuova app. I tuoi codici di recupero restano validi.",
		RecoveryCodeUsedSubject: "Un codice di recupero è stato usato per accedere a {brand}",
		RecoveryCodeUsedBody:    "Qualcuno ha effettuato l'accesso al tuo account con uno dei tuoi codici di recupero. Te ne restano %[1]d.",
		RecoveryCodesNewSubject: "Nuovi codici di recupero per il tuo account {brand}",
		RecoveryCodesNewBody:    "Sono stati creati nuovi codici di recupero per il tuo account. Quelli precedenti non funzionano più.",
		ResetSubject:            "L'autenticazione a due fattori del tuo account {brand} è stata reimpostata",
		ResetBody:               "Un amministratore di {brand} ha reimpostato l'autenticazione a due fattori del tuo account e ha chiuso tutte le tue sessioni. Se la tua istanza {brand} la richiede, la configurerai di nuovo al prossimo accesso.",
		FailuresSubject:         "Sono stati inseriti codici di autenticazione errati sul tuo account {brand}",
		FailuresBody:            "Sono stati inseriti diversi codici di autenticazione errati sul tuo account, dopo un codice corretto inviato alla tua email. Qualcuno potrebbe riuscire a leggere la tua casella di posta. Se non sei stato tu, cambia la password della tua email e attiva l'autenticazione a due fattori anche lì.",
		WhenLabel:               "Quando:",
		IPLabel:                 "Indirizzo IP indicato dalla richiesta:",
		BrowserLabel:            "Browser:",
		NotYouNotice:            "Se non sei stato tu, contatta il tuo amministratore {brand}.",
		SignOff:                 "Grazie,",
		UnknownValue:            "sconosciuto",
	},
}

func init() {
	for locale, set := range systemEmailTranslations {
		set.TwoFactor = twoFactorTranslations[locale]
		systemEmailTranslations[locale] = set
	}
}
