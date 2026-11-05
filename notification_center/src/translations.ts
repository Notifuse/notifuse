export type Language = 'en' | 'fr' | 'es' | 'de' | 'zh' | 'hi' | 'ar' | 'pt' | 'ru' | 'ja' | 'pl'

export interface TranslationObject {
  [key: string]: {
    [key in Language]?: string
  }
}

export const translations: TranslationObject = {
  loading: {
    en: 'Loading...',
    fr: 'Chargement...',
    es: 'Cargando...',
    de: 'Wird geladen...',
    zh: '加载中...',
    hi: 'लोड हो रहा है...',
    ar: 'جاري التحميل...',
    pt: 'Carregando...',
    ru: 'Загрузка...',
    ja: '読み込み中...',
    pl: 'Ładowanie...'
  },
  error: {
    en: 'Error',
    fr: 'Erreur',
    es: 'Error',
    de: 'Fehler',
    zh: '错误',
    hi: 'त्रुटि',
    ar: 'خطأ',
    pt: 'Erro',
    ru: 'Ошибка',
    ja: 'エラー',
    pl: 'Błąd'
  },
  missingParameters: {
    en: 'Missing required parameters. Please check the URL.',
    fr: "Paramètres requis manquants. Veuillez vérifier l'URL.",
    es: 'Faltan parámetros requeridos. Por favor, compruebe la URL.',
    de: 'Erforderliche Parameter fehlen. Bitte überprüfen Sie die URL.',
    zh: '缺少必需的参数。请检查URL。',
    hi: 'आवश्यक पैरामीटर गायब हैं। कृपया URL जांचें।',
    ar: 'معلمات مطلوبة مفقودة. يرجى التحقق من عنوان URL.',
    pt: 'Parâmetros necessários ausentes. Por favor, verifique o URL.',
    ru: 'Отсутствуют необходимые параметры. Пожалуйста, проверьте URL.',
    ja: '必要なパラメータが不足しています。URLを確認してください。',
    pl: 'Brakuje wymaganych parametrów. Sprawdź adres URL.'
  },
  emailSubscriptions: {
    en: 'Email Subscriptions',
    fr: 'Abonnements par e-mail',
    es: 'Suscripciones de correo electrónico',
    de: 'E-Mail-Abonnements',
    zh: '电子邮件订阅',
    hi: 'ईमेल सदस्यता',
    ar: 'اشتراكات البريد الإلكتروني',
    pt: 'Assinaturas de e-mail',
    ru: 'Подписки на электронную почту',
    ja: 'メール配信登録',
    pl: 'Subskrypcje e-mail'
  },
  welcome: {
    en: 'Welcome,',
    fr: 'Bienvenue,',
    es: 'Bienvenido,',
    de: 'Willkommen,',
    zh: '欢迎，',
    hi: 'स्वागत है,',
    ar: 'مرحبًا،',
    pt: 'Bem-vindo,',
    ru: 'Добро пожаловать,',
    ja: 'ようこそ、',
    pl: 'Witaj,'
  },
  processing: {
    en: 'Processing...',
    fr: 'Traitement...',
    es: 'Procesando...',
    de: 'Verarbeitung...',
    zh: '处理中...',
    hi: 'प्रोसेसिंग...',
    ar: 'جاري المعالجة...',
    pt: 'Processando...',
    ru: 'Обработка...',
    ja: '処理中...',
    pl: 'Przetwarzanie...'
  },
  subscribe: {
    en: 'Subscribe',
    fr: "S'abonner",
    es: 'Suscribirse',
    de: 'Abonnieren',
    zh: '订阅',
    hi: 'सदस्यता लें',
    ar: 'اشترك',
    pt: 'Assinar',
    ru: 'Подписаться',
    ja: '登録する',
    pl: 'Subskrybuj'
  },
  unsubscribe: {
    en: 'Unsubscribe',
    fr: 'Se désabonner',
    es: 'Darse de baja',
    de: 'Abbestellen',
    zh: '取消订阅',
    hi: 'सदस्यता समाप्त करें',
    ar: 'إلغاء الاشتراك',
    pt: 'Cancelar assinatura',
    ru: 'Отписаться',
    ja: '登録解除',
    pl: 'Anuluj subskrypcję'
  },
  noSubscriptions: {
    en: 'No subscriptions settings available.',
    fr: "Aucun paramètre d'abonnement disponible.",
    es: 'No hay configuraciones de suscripción disponibles.',
    de: 'Keine Abonnement-Einstellungen verfügbar.',
    zh: '没有可用的订阅设置。',
    hi: 'कोई सदस्यता सेटिंग उपलब्ध नहीं है।',
    ar: 'لا توجد إعدادات اشتراكات متاحة.',
    pt: 'Nenhuma configuração de assinatura disponível.',
    ru: 'Настройки подписок недоступны.',
    ja: '利用可能な購読設定はありません。',
    pl: 'Brak dostępnych ustawień subskrypcji.'
  },
  visitWebsite: {
    en: 'Visit our website',
    fr: 'Visitez notre site web',
    es: 'Visite nuestro sitio web',
    de: 'Besuchen Sie unsere Website',
    zh: '访问我们的网站',
    hi: 'हमारी वेबसाइट पर जाएँ',
    ar: 'زيارة موقعنا',
    pt: 'Visite nosso site',
    ru: 'Посетите наш сайт',
    ja: 'ウェブサイトにアクセス',
    pl: 'Odwiedź naszą stronę'
  },
  successSubscribed: {
    en: 'Successfully subscribed',
    fr: 'Abonnement réussi',
    es: 'Suscripción exitosa',
    de: 'Erfolgreich abonniert',
    zh: '订阅成功',
    hi: 'सफलतापूर्वक सदस्यता ली गई',
    ar: 'تم الاشتراك بنجاح',
    pt: 'Assinatura bem-sucedida',
    ru: 'Успешная подписка',
    ja: '登録が完了しました',
    pl: 'Pomyślnie zasubskrybowano'
  },
  successUnsubscribed: {
    en: 'Successfully unsubscribed',
    fr: 'Désabonnement réussi',
    es: 'Cancelación exitosa',
    de: 'Erfolgreich abbestellt',
    zh: '已成功取消订阅',
    hi: 'सफलतापूर्वक सदस्यता समाप्त की गई',
    ar: 'تم إلغاء الاشتراك بنجاح',
    pt: 'Cancelamento bem-sucedido',
    ru: 'Успешная отписка',
    ja: '登録解除が完了しました',
    pl: 'Pomyślnie anulowano subskrypcję'
  },
  failedSubscribe: {
    en: 'Failed to subscribe. Please try again.',
    fr: "Échec de l'abonnement. Veuillez réessayer.",
    es: 'Error al suscribirse. Por favor, inténtelo de nuevo.',
    de: 'Abonnement fehlgeschlagen. Bitte versuchen Sie es erneut.',
    zh: '订阅失败。请重试。',
    hi: 'सदस्यता लेने में विफल। कृपया पुन: प्रयास करें।',
    ar: 'فشل الاشتراك. يرجى المحاولة مرة أخرى.',
    pt: 'Falha na assinatura. Por favor, tente novamente.',
    ru: 'Не удалось подписаться. Пожалуйста, попробуйте снова.',
    ja: '登録に失敗しました。もう一度お試しください。',
    pl: 'Nie udało się zasubskrybować. Spróbuj ponownie.'
  },
  failedUnsubscribe: {
    en: 'Failed to unsubscribe. Please try again.',
    fr: 'Échec du désabonnement. Veuillez réessayer.',
    es: 'Error al darse de baja. Por favor, inténtelo de nuevo.',
    de: 'Abbestellung fehlgeschlagen. Bitte versuchen Sie es erneut.',
    zh: '取消订阅失败。请重试。',
    hi: 'सदस्यता समाप्त करने में विफल। कृपया पुन: प्रयास करें।',
    ar: 'فشل إلغاء الاشتراك. يرجى المحاولة مرة أخرى.',
    pt: 'Falha no cancelamento. Por favor, tente novamente.',
    ru: 'Не удалось отписаться. Пожалуйста, попробуйте снова.',
    ja: '登録解除に失敗しました。もう一度お試しください。',
    pl: 'Nie udało się anulować subskrypcji. Spróbuj ponownie.'
  },
  failedToLoad: {
    en: 'Failed to load notifications',
    fr: 'Échec du chargement des notifications',
    es: 'Error al cargar las notificaciones',
    de: 'Benachrichtigungen konnten nicht geladen werden',
    zh: '无法加载通知',
    hi: 'सूचनाओं को लोड करने में विफल',
    ar: 'فشل في تحميل الإشعارات',
    pt: 'Falha ao carregar notificações',
    ru: 'Не удалось загрузить уведомления',
    ja: '通知の読み込みに失敗しました',
    pl: 'Nie udało się załadować powiadomień'
  },
  successHeader: {
    en: '✓ Success!',
    fr: '✓ Succès !',
    es: '✓ ¡Éxito!',
    de: '✓ Erfolg!',
    zh: '✓ 成功！',
    hi: '✓ सफल!',
    ar: '✓ نجاح!',
    pt: '✓ Sucesso!',
    ru: '✓ Успех!',
    ja: '✓ 成功！',
    pl: '✓ Sukces!'
  },
  errorHeader: {
    en: '✗ Error',
    fr: '✗ Erreur',
    es: '✗ Error',
    de: '✗ Fehler',
    zh: '✗ 错误',
    hi: '✗ त्रुटि',
    ar: '✗ خطأ',
    pt: '✗ Erro',
    ru: '✗ Ошибка',
    ja: '✗ エラー',
    pl: '✗ Błąd'
  },
  confirmSubscriptionSuccess: {
    en: 'Subscription confirmed successfully!',
    fr: 'Abonnement confirmé avec succès !',
    es: '¡Suscripción confirmada con éxito!',
    de: 'Abonnement erfolgreich bestätigt!',
    zh: '订阅确认成功！',
    hi: 'सदस्यता सफलतापूर्वक पुष्टि की गई!',
    ar: 'تم تأكيد الاشتراك بنجاح!',
    pt: 'Assinatura confirmada com sucesso!',
    ru: 'Подписка успешно подтверждена!',
    ja: '登録が正常に確認されました！',
    pl: 'Subskrypcja potwierdzona pomyślnie!'
  },
  confirmSubscriptionFailed: {
    en: 'Failed to confirm subscription',
    fr: "Échec de la confirmation de l'abonnement",
    es: 'Error al confirmar la suscripción',
    de: 'Abonnement konnte nicht bestätigt werden',
    zh: '无法确认订阅',
    hi: 'सदस्यता की पुष्टि करने में विफल',
    ar: 'فشل تأكيد الاشتراك',
    pt: 'Falha ao confirmar a assinatura',
    ru: 'Не удалось подтвердить подписку',
    ja: '登録の確認に失敗しました',
    pl: 'Nie udało się potwierdzić subskrypcji'
  },
  unsubscribeSuccess: {
    en: 'You have been unsubscribed successfully.',
    fr: 'Vous avez été désabonné avec succès.',
    es: 'Se ha dado de baja correctamente.',
    de: 'Sie wurden erfolgreich abgemeldet.',
    zh: '您已成功取消订阅。',
    hi: 'आपकी सदस्यता सफलतापूर्वक समाप्त कर दी गई है।',
    ar: 'تم إلغاء اشتراكك بنجاح.',
    pt: 'Você foi cancelado com sucesso.',
    ru: 'Вы успешно отписаны.',
    ja: '正常に登録解除されました。',
    pl: 'Zostałeś pomyślnie wypisany.'
  },
  unsubscribeFailed: {
    en: 'Failed to unsubscribe',
    fr: 'Échec du désabonnement',
    es: 'Error al darse de baja',
    de: 'Abmeldung fehlgeschlagen',
    zh: '取消订阅失败',
    hi: 'सदस्यता समाप्त करने में विफल',
    ar: 'فشل إلغاء الاشتراك',
    pt: 'Falha ao cancelar a assinatura',
    ru: 'Не удалось отписаться',
    ja: '登録解除に失敗しました',
    pl: 'Nie udało się wypisać'
  },
  confirmUnsubscribePrompt: {
    en: 'Do you want to stop receiving emails from this list?',
    fr: 'Voulez-vous ne plus recevoir les e-mails de cette liste ?',
    es: '¿Desea dejar de recibir correos electrónicos de esta lista?',
    de: 'Möchten Sie keine E-Mails mehr von dieser Liste erhalten?',
    zh: '您要停止接收此列表的电子邮件吗？',
    hi: 'क्या आप इस सूची से ईमेल प्राप्त करना बंद करना चाहते हैं?',
    ar: 'هل تريد التوقف عن تلقي رسائل البريد الإلكتروني من هذه القائمة؟',
    pt: 'Deseja deixar de receber e-mails desta lista?',
    ru: 'Вы хотите перестать получать письма из этого списка?',
    ja: 'このリストからのメールの受信を停止しますか？',
    pl: 'Czy chcesz przestać otrzymywać e-maile z tej listy?'
  },
  alreadyUnsubscribed: {
    en: 'You are already unsubscribed from this list.',
    fr: 'Vous êtes déjà désabonné de cette liste.',
    es: 'Ya se ha dado de baja de esta lista.',
    de: 'Sie sind von dieser Liste bereits abgemeldet.',
    zh: '您已取消订阅此列表。',
    hi: 'आप इस सूची से पहले ही सदस्यता समाप्त कर चुके हैं।',
    ar: 'لقد ألغيت اشتراكك في هذه القائمة بالفعل.',
    pt: 'Você já cancelou a assinatura desta lista.',
    ru: 'Вы уже отписаны от этого списка.',
    ja: 'このリストの登録はすでに解除されています。',
    pl: 'Jesteś już wypisany z tej listy.'
  },
  unsubscribedStatus: {
    en: 'Unsubscribed',
    fr: 'Désabonné',
    es: 'Dado de baja',
    de: 'Abgemeldet',
    zh: '已取消订阅',
    hi: 'सदस्यता समाप्त',
    ar: 'تم إلغاء الاشتراك',
    pt: 'Inscrição cancelada',
    ru: 'Подписка отменена',
    ja: '登録解除済み',
    pl: 'Wypisano'
  },
  emailTracking: {
    en: 'Email tracking',
    fr: 'Tracking des e-mails',
    es: 'Seguimiento de correos',
    de: 'E-Mail-Tracking',
    zh: '邮件跟踪',
    hi: 'ईमेल ट्रैकिंग',
    ar: 'تتبع البريد الإلكتروني',
    pt: 'Rastreamento de e-mails',
    ru: 'Отслеживание писем',
    ja: 'メールのトラッキング',
    pl: 'Śledzenie e-maili'
  },
  emailTrackingIntro: {
    en: 'Choose whether we may record when you open our emails and click their links.',
    fr: 'Choisissez si nous pouvons enregistrer quand vous ouvrez nos e-mails et cliquez sur leurs liens.',
    es: 'Elija si podemos registrar cuándo abre nuestros correos y hace clic en sus enlaces.',
    de: 'Wählen Sie, ob wir erfassen dürfen, wann Sie unsere E-Mails öffnen und auf deren Links klicken.',
    zh: '选择我们是否可以记录您何时打开我们的邮件并点击其中的链接。',
    hi: 'चुनें कि क्या हम रिकॉर्ड कर सकते हैं कि आप कब हमारे ईमेल खोलते हैं और उनके लिंक पर क्लिक करते हैं।',
    ar: 'اختر ما إذا كان بإمكاننا تسجيل وقت فتحك لرسائلنا والنقر على روابطها.',
    pt: 'Escolha se podemos registrar quando você abre nossos e-mails e clica em seus links.',
    ru: 'Выберите, можем ли мы фиксировать, когда вы открываете наши письма и переходите по ссылкам в них.',
    ja: 'メールの開封やリンクのクリックを記録してよいかを選択してください。',
    pl: 'Wybierz, czy możemy rejestrować, kiedy otwierasz nasze e-maile i klikasz zawarte w nich linki.'
  },
  openTracking: {
    en: 'Open tracking',
    fr: 'Tracking des ouvertures',
    es: 'Seguimiento de aperturas',
    de: 'Öffnungs-Tracking',
    zh: '打开跟踪',
    hi: 'ओपन ट्रैकिंग',
    ar: 'تتبع الفتح',
    pt: 'Rastreamento de aberturas',
    ru: 'Отслеживание открытий',
    ja: '開封のトラッキング',
    pl: 'Śledzenie otwarć'
  },
  openTrackingDescription: {
    en: 'An invisible image in our emails records when you open them.',
    fr: 'Une image invisible dans nos e-mails enregistre quand vous les ouvrez.',
    es: 'Una imagen invisible en nuestros correos registra cuándo los abre.',
    de: 'Ein unsichtbares Bild in unseren E-Mails erfasst, wann Sie sie öffnen.',
    zh: '我们邮件中的一个不可见图像会记录您何时打开邮件。',
    hi: 'हमारे ईमेल में एक अदृश्य छवि रिकॉर्ड करती है कि आप उन्हें कब खोलते हैं।',
    ar: 'صورة غير مرئية في رسائلنا تسجل وقت فتحك لها.',
    pt: 'Uma imagem invisível em nossos e-mails registra quando você os abre.',
    ru: 'Невидимое изображение в наших письмах фиксирует, когда вы их открываете.',
    ja: 'メール内の見えない画像が、メールを開いた時刻を記録します。',
    pl: 'Niewidoczny obraz w naszych e-mailach rejestruje, kiedy je otwierasz.'
  },
  clickTracking: {
    en: 'Click tracking',
    fr: 'Tracking des clics',
    es: 'Seguimiento de clics',
    de: 'Klick-Tracking',
    zh: '点击跟踪',
    hi: 'क्लिक ट्रैकिंग',
    ar: 'تتبع النقرات',
    pt: 'Rastreamento de cliques',
    ru: 'Отслеживание кликов',
    ja: 'クリックのトラッキング',
    pl: 'Śledzenie kliknięć'
  },
  clickTrackingDescription: {
    en: 'Links in our emails go through a redirect that records each click.',
    fr: 'Les liens de nos e-mails passent par une redirection qui enregistre chaque clic.',
    es: 'Los enlaces de nuestros correos pasan por una redirección que registra cada clic.',
    de: 'Links in unseren E-Mails laufen über eine Weiterleitung, die jeden Klick erfasst.',
    zh: '我们邮件中的链接会经过一个记录每次点击的重定向。',
    hi: 'हमारे ईमेल के लिंक एक रीडायरेक्ट से होकर जाते हैं जो हर क्लिक रिकॉर्ड करता है।',
    ar: 'تمر الروابط في رسائلنا عبر إعادة توجيه تسجل كل نقرة.',
    pt: 'Os links em nossos e-mails passam por um redirecionamento que registra cada clique.',
    ru: 'Ссылки в наших письмах проходят через перенаправление, которое фиксирует каждый клик.',
    ja: 'メール内のリンクは、クリックを記録するリダイレクトを経由します。',
    pl: 'Linki w naszych e-mailach przechodzą przez przekierowanie, które rejestruje każde kliknięcie.'
  },
  trackingOn: {
    en: 'On',
    fr: 'Activé',
    es: 'Activado',
    de: 'An',
    zh: '开启',
    hi: 'चालू',
    ar: 'مفعّل',
    pt: 'Ativado',
    ru: 'Включено',
    ja: 'オン',
    pl: 'Włączone'
  },
  trackingOff: {
    en: 'Off',
    fr: 'Désactivé',
    es: 'Desactivado',
    de: 'Aus',
    zh: '关闭',
    hi: 'बंद',
    ar: 'معطّل',
    pt: 'Desativado',
    ru: 'Выключено',
    ja: 'オフ',
    pl: 'Wyłączone'
  },
  trackingAllow: {
    en: 'Allow',
    fr: 'Autoriser',
    es: 'Permitir',
    de: 'Erlauben',
    zh: '允许',
    hi: 'अनुमति दें',
    ar: 'السماح',
    pt: 'Permitir',
    ru: 'Разрешить',
    ja: '許可する',
    pl: 'Zezwól'
  },
  trackingStop: {
    en: 'Stop',
    fr: 'Arrêter',
    es: 'Detener',
    de: 'Beenden',
    zh: '停止',
    hi: 'बंद करें',
    ar: 'إيقاف',
    pt: 'Parar',
    ru: 'Остановить',
    ja: '停止する',
    pl: 'Zatrzymaj'
  },
  trackingSaved: {
    en: 'Your tracking choice has been saved',
    fr: 'Votre choix de tracking a été enregistré',
    es: 'Se ha guardado su elección de seguimiento',
    de: 'Ihre Tracking-Auswahl wurde gespeichert',
    zh: '您的跟踪选择已保存',
    hi: 'आपकी ट्रैकिंग पसंद सहेज ली गई है',
    ar: 'تم حفظ اختيارك للتتبع',
    pt: 'Sua escolha de rastreamento foi salva',
    ru: 'Ваш выбор отслеживания сохранён',
    ja: 'トラッキングの選択を保存しました',
    pl: 'Twój wybór dotyczący śledzenia został zapisany'
  },
  trackingSaveFailed: {
    en: 'We could not save your tracking choice. Please try again.',
    fr: 'Nous n\'avons pas pu enregistrer votre choix de tracking. Veuillez réessayer.',
    es: 'No pudimos guardar su elección de seguimiento. Inténtelo de nuevo.',
    de: 'Wir konnten Ihre Tracking-Auswahl nicht speichern. Bitte versuchen Sie es erneut.',
    zh: '我们无法保存您的跟踪选择，请重试。',
    hi: 'हम आपकी ट्रैकिंग पसंद सहेज नहीं सके। कृपया फिर से प्रयास करें।',
    ar: 'تعذر حفظ اختيارك للتتبع. يرجى المحاولة مرة أخرى.',
    pt: 'Não foi possível salvar sua escolha de rastreamento. Tente novamente.',
    ru: 'Не удалось сохранить ваш выбор отслеживания. Попробуйте ещё раз.',
    ja: 'トラッキングの選択を保存できませんでした。もう一度お試しください。',
    pl: 'Nie udało się zapisać Twojego wyboru dotyczącego śledzenia. Spróbuj ponownie.'
  },
  trackingOptoutPrompt: {
    en: 'Do you want us to stop recording when you open our emails and click their links?',
    fr: 'Voulez-vous que nous cessions d\'enregistrer quand vous ouvrez nos e-mails et cliquez sur leurs liens ?',
    es: '¿Desea que dejemos de registrar cuándo abre nuestros correos y hace clic en sus enlaces?',
    de: 'Möchten Sie, dass wir nicht mehr erfassen, wann Sie unsere E-Mails öffnen und auf deren Links klicken?',
    zh: '您希望我们停止记录您何时打开我们的邮件并点击其中的链接吗？',
    hi: 'क्या आप चाहते हैं कि हम यह रिकॉर्ड करना बंद करें कि आप कब हमारे ईमेल खोलते हैं और उनके लिंक पर क्लिक करते हैं?',
    ar: 'هل تريد أن نتوقف عن تسجيل وقت فتحك لرسائلنا والنقر على روابطها؟',
    pt: 'Deseja que deixemos de registrar quando você abre nossos e-mails e clica em seus links?',
    ru: 'Хотите, чтобы мы перестали фиксировать, когда вы открываете наши письма и переходите по ссылкам в них?',
    ja: 'メールの開封やリンクのクリックの記録を停止しますか？',
    pl: 'Czy chcesz, abyśmy przestali rejestrować, kiedy otwierasz nasze e-maile i klikasz zawarte w nich linki?'
  },
  trackingOptoutConfirm: {
    en: 'Stop tracking',
    fr: 'Arrêter le tracking',
    es: 'Detener el seguimiento',
    de: 'Tracking beenden',
    zh: '停止跟踪',
    hi: 'ट्रैकिंग बंद करें',
    ar: 'إيقاف التتبع',
    pt: 'Parar o rastreamento',
    ru: 'Остановить отслеживание',
    ja: 'トラッキングを停止',
    pl: 'Zatrzymaj śledzenie'
  },
  trackingOptoutSuccess: {
    en: 'Opens and clicks of our emails are no longer tracked for you.',
    fr: 'Les ouvertures et les clics de nos e-mails ne sont plus trackés pour vous.',
    es: 'Las aperturas y los clics de nuestros correos ya no se siguen para usted.',
    de: 'Öffnungen und Klicks unserer E-Mails werden für Sie nicht mehr erfasst.',
    zh: '我们将不再跟踪您对我们邮件的打开和点击。',
    hi: 'हमारे ईमेल के ओपन और क्लिक अब आपके लिए ट्रैक नहीं किए जाते।',
    ar: 'لم يعد يتم تتبع فتحك لرسائلنا ونقرك على روابطها.',
    pt: 'As aberturas e os cliques dos nossos e-mails não são mais rastreados para você.',
    ru: 'Открытия и клики по нашим письмам больше не отслеживаются для вас.',
    ja: '今後、メールの開封とクリックはトラッキングされません。',
    pl: 'Otwarcia i kliknięcia naszych e-maili nie są już dla Ciebie śledzone.'
  }
}

export function getLanguage(): Language {
  // Get browser language
  const browserLang = navigator.language.split('-')[0].toLowerCase()

  // Check if browser language is supported
  const supportedLanguages: Language[] = [
    'en',
    'fr',
    'es',
    'de',
    'zh',
    'hi',
    'ar',
    'pt',
    'ru',
    'ja',
    'pl'
  ]

  if (supportedLanguages.includes(browserLang as Language)) {
    return browserLang as Language
  }

  // Default to English
  return 'en'
}

export function getTranslation(key: string, language: Language = getLanguage()): string {
  if (translations[key] && translations[key][language]) {
    return translations[key][language] as string
  }

  // Fallback to English
  if (translations[key] && translations[key]['en']) {
    return translations[key]['en'] as string
  }

  // Return key if no translation found
  return key
}
