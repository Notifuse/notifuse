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
		EnabledSubject:          "Two-factor authentication is on for your Notifuse account",
		EnabledBody:             "Two-factor authentication was turned on for your account. From now on, after the code we email you, Notifuse asks for a code from your authenticator app.",
		DisabledSubject:         "Two-factor authentication is off for your Notifuse account",
		DisabledBody:            "Two-factor authentication was turned off for your account. Notifuse now asks only for the code we email you.",
		ReplacedSubject:         "Your Notifuse authenticator app was replaced",
		ReplacedBody:            "The authenticator app of your account was replaced. From now on, Notifuse accepts only codes from the new app. Your recovery codes still work.",
		RecoveryCodeUsedSubject: "A recovery code was used to sign in to Notifuse",
		RecoveryCodeUsedBody:    "Someone signed in to your account with one of your recovery codes. You have %[1]d left.",
		RecoveryCodesNewSubject: "New recovery codes for your Notifuse account",
		RecoveryCodesNewBody:    "New recovery codes were created for your account. The previous ones no longer work.",
		ResetSubject:            "Two-factor authentication was reset on your Notifuse account",
		ResetBody:               "A Notifuse administrator reset two-factor authentication on your account and signed you out everywhere. If your Notifuse instance requires it, you will set it up again at your next sign-in.",
		FailuresSubject:         "Wrong authentication codes were entered on your Notifuse account",
		FailuresBody:            "Several wrong authentication codes were entered for your account, after a correct code sent to your email. Someone may be able to read your mailbox. If this wasn't you, change your email password and turn on two-factor authentication for your mailbox.",
		WhenLabel:               "When:",
		IPLabel:                 "IP address given by the request:",
		BrowserLabel:            "Browser:",
		NotYouNotice:            "If this wasn't you, contact your Notifuse administrator.",
		SignOff:                 "Thanks,",
		UnknownValue:            "unknown",
	},
	"fr": {
		EnabledSubject:          "La double authentification est activée sur votre compte Notifuse",
		EnabledBody:             "La double authentification a été activée sur votre compte. Désormais, après le code envoyé par email, Notifuse demande un code de votre application d'authentification.",
		DisabledSubject:         "La double authentification est désactivée sur votre compte Notifuse",
		DisabledBody:            "La double authentification a été désactivée sur votre compte. Notifuse ne demande plus que le code envoyé par email.",
		ReplacedSubject:         "L'application d'authentification de votre compte Notifuse a été remplacée",
		ReplacedBody:            "L'application d'authentification de votre compte a été remplacée. Notifuse n'accepte plus que les codes de la nouvelle application. Vos codes de récupération restent valables.",
		RecoveryCodeUsedSubject: "Un code de récupération a servi à se connecter à Notifuse",
		RecoveryCodeUsedBody:    "Quelqu'un s'est connecté à votre compte avec l'un de vos codes de récupération. Il vous en reste %[1]d.",
		RecoveryCodesNewSubject: "Nouveaux codes de récupération pour votre compte Notifuse",
		RecoveryCodesNewBody:    "De nouveaux codes de récupération ont été créés pour votre compte. Les anciens ne fonctionnent plus.",
		ResetSubject:            "La double authentification de votre compte Notifuse a été réinitialisée",
		ResetBody:               "Un administrateur Notifuse a réinitialisé la double authentification de votre compte et vous a déconnecté partout. Si votre instance Notifuse l'exige, vous la configurerez de nouveau à votre prochaine connexion.",
		FailuresSubject:         "Des codes d'authentification erronés ont été saisis sur votre compte Notifuse",
		FailuresBody:            "Plusieurs codes d'authentification erronés ont été saisis sur votre compte, après un code correct envoyé à votre adresse email. Quelqu'un peut peut-être lire votre boîte mail. Si ce n'était pas vous, changez le mot de passe de votre messagerie et activez sa double authentification.",
		WhenLabel:               "Quand :",
		IPLabel:                 "Adresse IP indiquée par la requête :",
		BrowserLabel:            "Navigateur :",
		NotYouNotice:            "Si ce n'était pas vous, contactez votre administrateur Notifuse.",
		SignOff:                 "Merci,",
		UnknownValue:            "inconnu",
	},
	"es": {
		EnabledSubject:          "La autenticación en dos pasos está activada en tu cuenta de Notifuse",
		EnabledBody:             "Se activó la autenticación en dos pasos en tu cuenta. A partir de ahora, después del código que te enviamos por email, Notifuse pide un código de tu aplicación de autenticación.",
		DisabledSubject:         "La autenticación en dos pasos está desactivada en tu cuenta de Notifuse",
		DisabledBody:            "Se desactivó la autenticación en dos pasos en tu cuenta. Notifuse ya solo pide el código que te enviamos por email.",
		ReplacedSubject:         "Se ha sustituido la aplicación de autenticación de tu cuenta de Notifuse",
		ReplacedBody:            "Se ha sustituido la aplicación de autenticación de tu cuenta. A partir de ahora, Notifuse solo acepta los códigos de la nueva aplicación. Tus códigos de recuperación siguen funcionando.",
		RecoveryCodeUsedSubject: "Se usó un código de recuperación para iniciar sesión en Notifuse",
		RecoveryCodeUsedBody:    "Alguien inició sesión en tu cuenta con uno de tus códigos de recuperación. Te quedan %[1]d.",
		RecoveryCodesNewSubject: "Nuevos códigos de recuperación para tu cuenta de Notifuse",
		RecoveryCodesNewBody:    "Se crearon nuevos códigos de recuperación para tu cuenta. Los anteriores ya no funcionan.",
		ResetSubject:            "Se restableció la autenticación en dos pasos de tu cuenta de Notifuse",
		ResetBody:               "Un administrador de Notifuse restableció la autenticación en dos pasos de tu cuenta y cerró todas tus sesiones. Si tu instancia de Notifuse la exige, la configurarás de nuevo en tu próximo inicio de sesión.",
		FailuresSubject:         "Se introdujeron códigos de autenticación incorrectos en tu cuenta de Notifuse",
		FailuresBody:            "Se introdujeron varios códigos de autenticación incorrectos en tu cuenta, después de un código correcto enviado a tu email. Es posible que alguien pueda leer tu buzón. Si no fuiste tú, cambia la contraseña de tu correo y activa su autenticación en dos pasos.",
		WhenLabel:               "Cuándo:",
		IPLabel:                 "Dirección IP indicada por la solicitud:",
		BrowserLabel:            "Navegador:",
		NotYouNotice:            "Si no fuiste tú, contacta con tu administrador de Notifuse.",
		SignOff:                 "Gracias,",
		UnknownValue:            "desconocido",
	},
	"de": {
		EnabledSubject:          "Die Zwei-Faktor-Authentifizierung ist für Ihr Notifuse-Konto aktiviert",
		EnabledBody:             "Für Ihr Konto wurde die Zwei-Faktor-Authentifizierung aktiviert. Ab jetzt fragt Notifuse nach dem Code aus der E-Mail zusätzlich einen Code aus Ihrer Authenticator-App ab.",
		DisabledSubject:         "Die Zwei-Faktor-Authentifizierung ist für Ihr Notifuse-Konto deaktiviert",
		DisabledBody:            "Für Ihr Konto wurde die Zwei-Faktor-Authentifizierung deaktiviert. Notifuse fragt nur noch den Code ab, den wir Ihnen per E-Mail senden.",
		ReplacedSubject:         "Die Authenticator-App Ihres Notifuse-Kontos wurde ersetzt",
		ReplacedBody:            "Die Authenticator-App Ihres Kontos wurde ersetzt. Ab jetzt akzeptiert Notifuse nur noch Codes der neuen App. Ihre Wiederherstellungscodes gelten weiter.",
		RecoveryCodeUsedSubject: "Ein Wiederherstellungscode wurde für die Anmeldung bei Notifuse verwendet",
		RecoveryCodeUsedBody:    "Jemand hat sich mit einem Ihrer Wiederherstellungscodes bei Ihrem Konto angemeldet. Sie haben noch %[1]d.",
		RecoveryCodesNewSubject: "Neue Wiederherstellungscodes für Ihr Notifuse-Konto",
		RecoveryCodesNewBody:    "Für Ihr Konto wurden neue Wiederherstellungscodes erstellt. Die bisherigen funktionieren nicht mehr.",
		ResetSubject:            "Die Zwei-Faktor-Authentifizierung Ihres Notifuse-Kontos wurde zurückgesetzt",
		ResetBody:               "Ein Notifuse-Administrator hat die Zwei-Faktor-Authentifizierung Ihres Kontos zurückgesetzt und Sie überall abgemeldet. Wenn Ihre Notifuse-Instanz sie verlangt, richten Sie sie bei der nächsten Anmeldung neu ein.",
		FailuresSubject:         "Für Ihr Notifuse-Konto wurden falsche Authentifizierungscodes eingegeben",
		FailuresBody:            "Für Ihr Konto wurden mehrere falsche Authentifizierungscodes eingegeben, nachdem ein richtiger Code aus Ihrer E-Mail verwendet wurde. Möglicherweise kann jemand Ihr Postfach lesen. Wenn Sie das nicht waren, ändern Sie das Passwort Ihres E-Mail-Kontos und aktivieren Sie dort die Zwei-Faktor-Authentifizierung.",
		WhenLabel:               "Wann:",
		IPLabel:                 "Von der Anfrage angegebene IP-Adresse:",
		BrowserLabel:            "Browser:",
		NotYouNotice:            "Wenn Sie das nicht waren, wenden Sie sich an Ihren Notifuse-Administrator.",
		SignOff:                 "Danke,",
		UnknownValue:            "unbekannt",
	},
	"ca": {
		EnabledSubject:          "L'autenticació en dos passos està activada al teu compte de Notifuse",
		EnabledBody:             "S'ha activat l'autenticació en dos passos al teu compte. A partir d'ara, després del codi que t'enviem per correu, Notifuse demana un codi de la teva aplicació d'autenticació.",
		DisabledSubject:         "L'autenticació en dos passos està desactivada al teu compte de Notifuse",
		DisabledBody:            "S'ha desactivat l'autenticació en dos passos al teu compte. Notifuse ja només demana el codi que t'enviem per correu.",
		ReplacedSubject:         "S'ha substituït l'aplicació d'autenticació del teu compte de Notifuse",
		ReplacedBody:            "S'ha substituït l'aplicació d'autenticació del teu compte. A partir d'ara, Notifuse només accepta els codis de la nova aplicació. Els teus codis de recuperació continuen funcionant.",
		RecoveryCodeUsedSubject: "S'ha fet servir un codi de recuperació per iniciar sessió a Notifuse",
		RecoveryCodeUsedBody:    "Algú ha iniciat sessió al teu compte amb un dels teus codis de recuperació. Te'n queden %[1]d.",
		RecoveryCodesNewSubject: "Codis de recuperació nous per al teu compte de Notifuse",
		RecoveryCodesNewBody:    "S'han creat codis de recuperació nous per al teu compte. Els anteriors ja no funcionen.",
		ResetSubject:            "S'ha restablert l'autenticació en dos passos del teu compte de Notifuse",
		ResetBody:               "Un administrador de Notifuse ha restablert l'autenticació en dos passos del teu compte i ha tancat totes les teves sessions. Si la teva instància de Notifuse l'exigeix, la tornaràs a configurar en el proper inici de sessió.",
		FailuresSubject:         "S'han introduït codis d'autenticació incorrectes al teu compte de Notifuse",
		FailuresBody:            "S'han introduït diversos codis d'autenticació incorrectes al teu compte, després d'un codi correcte enviat al teu correu. És possible que algú pugui llegir la teva bústia. Si no has estat tu, canvia la contrasenya del teu correu i activa'n l'autenticació en dos passos.",
		WhenLabel:               "Quan:",
		IPLabel:                 "Adreça IP indicada per la sol·licitud:",
		BrowserLabel:            "Navegador:",
		NotYouNotice:            "Si no has estat tu, posa't en contacte amb l'administrador de Notifuse.",
		SignOff:                 "Gràcies,",
		UnknownValue:            "desconegut",
	},
	"pt-br": {
		EnabledSubject:          "A autenticação em dois fatores está ativada na sua conta do Notifuse",
		EnabledBody:             "A autenticação em dois fatores foi ativada na sua conta. A partir de agora, depois do código que enviamos por email, o Notifuse pede um código do seu aplicativo autenticador.",
		DisabledSubject:         "A autenticação em dois fatores está desativada na sua conta do Notifuse",
		DisabledBody:            "A autenticação em dois fatores foi desativada na sua conta. O Notifuse agora pede apenas o código que enviamos por email.",
		ReplacedSubject:         "O app autenticador da sua conta do Notifuse foi substituído",
		ReplacedBody:            "O app autenticador da sua conta foi substituído. A partir de agora, o Notifuse só aceita os códigos do novo app. Seus códigos de recuperação continuam valendo.",
		RecoveryCodeUsedSubject: "Um código de recuperação foi usado para entrar no Notifuse",
		RecoveryCodeUsedBody:    "Alguém entrou na sua conta com um dos seus códigos de recuperação. Restam %[1]d.",
		RecoveryCodesNewSubject: "Novos códigos de recuperação para sua conta do Notifuse",
		RecoveryCodesNewBody:    "Novos códigos de recuperação foram criados para sua conta. Os anteriores não funcionam mais.",
		ResetSubject:            "A autenticação em dois fatores da sua conta do Notifuse foi redefinida",
		ResetBody:               "Um administrador do Notifuse redefiniu a autenticação em dois fatores da sua conta e encerrou todas as suas sessões. Se a sua instância do Notifuse exigir, você vai configurá-la de novo no próximo login.",
		FailuresSubject:         "Códigos de autenticação errados foram digitados na sua conta do Notifuse",
		FailuresBody:            "Vários códigos de autenticação errados foram digitados na sua conta, depois de um código correto enviado ao seu email. Alguém pode estar conseguindo ler sua caixa de entrada. Se não foi você, troque a senha do seu email e ative a autenticação em dois fatores dele.",
		WhenLabel:               "Quando:",
		IPLabel:                 "Endereço IP informado pela requisição:",
		BrowserLabel:            "Navegador:",
		NotYouNotice:            "Se não foi você, fale com o administrador do Notifuse.",
		SignOff:                 "Obrigado,",
		UnknownValue:            "desconhecido",
	},
	"ja": {
		EnabledSubject:          "Notifuse アカウントで二要素認証が有効になりました",
		EnabledBody:             "お使いのアカウントで二要素認証が有効になりました。今後は、メールで届くコードに加えて、認証アプリのコードの入力が求められます。",
		DisabledSubject:         "Notifuse アカウントで二要素認証が無効になりました",
		DisabledBody:            "お使いのアカウントで二要素認証が無効になりました。今後はメールで届くコードだけでサインインできます。",
		ReplacedSubject:         "Notifuse アカウントの認証アプリが置き換えられました",
		ReplacedBody:            "アカウントの認証アプリが置き換えられました。今後、Notifuse は新しいアプリのコードのみを受け付けます。リカバリーコードは引き続き使えます。",
		RecoveryCodeUsedSubject: "リカバリーコードで Notifuse にサインインしました",
		RecoveryCodeUsedBody:    "リカバリーコードの一つを使って、お使いのアカウントにサインインがありました。残りは %[1]d 個です。",
		RecoveryCodesNewSubject: "Notifuse アカウントの新しいリカバリーコード",
		RecoveryCodesNewBody:    "お使いのアカウントに新しいリカバリーコードが作成されました。以前のコードは使えなくなりました。",
		ResetSubject:            "Notifuse アカウントの二要素認証がリセットされました",
		ResetBody:               "Notifuse の管理者がお使いのアカウントの二要素認証をリセットし、すべてのセッションからサインアウトしました。Notifuse インスタンスで必須の場合は、次回のサインイン時に再設定してください。",
		FailuresSubject:         "Notifuse アカウントで誤った認証コードが入力されました",
		FailuresBody:            "メールで届いた正しいコードの後に、お使いのアカウントで誤った認証コードが何度も入力されました。第三者がメールボックスを読める状態にある可能性があります。心当たりがない場合は、メールのパスワードを変更し、メールアカウントの二要素認証を有効にしてください。",
		WhenLabel:               "日時:",
		IPLabel:                 "リクエストが示した IP アドレス:",
		BrowserLabel:            "ブラウザ:",
		NotYouNotice:            "心当たりがない場合は、Notifuse の管理者に連絡してください。",
		SignOff:                 "よろしくお願いいたします。",
		UnknownValue:            "不明",
	},
	"it": {
		EnabledSubject:          "L'autenticazione a due fattori è attiva sul tuo account Notifuse",
		EnabledBody:             "L'autenticazione a due fattori è stata attivata sul tuo account. D'ora in poi, dopo il codice che ti inviamo via email, Notifuse chiede un codice della tua app di autenticazione.",
		DisabledSubject:         "L'autenticazione a due fattori è disattivata sul tuo account Notifuse",
		DisabledBody:            "L'autenticazione a due fattori è stata disattivata sul tuo account. Ora Notifuse chiede solo il codice che ti inviamo via email.",
		ReplacedSubject:         "L'app di autenticazione del tuo account Notifuse è stata sostituita",
		ReplacedBody:            "L'app di autenticazione del tuo account è stata sostituita. Da ora Notifuse accetta solo i codici della nuova app. I tuoi codici di recupero restano validi.",
		RecoveryCodeUsedSubject: "Un codice di recupero è stato usato per accedere a Notifuse",
		RecoveryCodeUsedBody:    "Qualcuno ha effettuato l'accesso al tuo account con uno dei tuoi codici di recupero. Te ne restano %[1]d.",
		RecoveryCodesNewSubject: "Nuovi codici di recupero per il tuo account Notifuse",
		RecoveryCodesNewBody:    "Sono stati creati nuovi codici di recupero per il tuo account. Quelli precedenti non funzionano più.",
		ResetSubject:            "L'autenticazione a due fattori del tuo account Notifuse è stata reimpostata",
		ResetBody:               "Un amministratore di Notifuse ha reimpostato l'autenticazione a due fattori del tuo account e ha chiuso tutte le tue sessioni. Se la tua istanza Notifuse la richiede, la configurerai di nuovo al prossimo accesso.",
		FailuresSubject:         "Sono stati inseriti codici di autenticazione errati sul tuo account Notifuse",
		FailuresBody:            "Sono stati inseriti diversi codici di autenticazione errati sul tuo account, dopo un codice corretto inviato alla tua email. Qualcuno potrebbe riuscire a leggere la tua casella di posta. Se non sei stato tu, cambia la password della tua email e attiva l'autenticazione a due fattori anche lì.",
		WhenLabel:               "Quando:",
		IPLabel:                 "Indirizzo IP indicato dalla richiesta:",
		BrowserLabel:            "Browser:",
		NotYouNotice:            "Se non sei stato tu, contatta il tuo amministratore Notifuse.",
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
