export type Language = 'en' | 'fr' | 'es' | 'de' | 'zh' | 'hi' | 'ar' | 'pt' | 'ru' | 'ja'

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
    ja: '読み込み中...'
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
    ja: 'エラー'
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
    ja: '必要なパラメータが不足しています。URLを確認してください。'
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
    ja: 'メール配信登録'
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
    ja: 'ようこそ、'
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
    ja: '処理中...'
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
    ja: '登録する'
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
    ja: '登録解除'
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
    ja: '利用可能な購読設定はありません。'
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
    ja: 'ウェブサイトにアクセス'
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
    ja: '登録が完了しました'
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
    ja: '登録解除が完了しました'
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
    ja: '登録に失敗しました。もう一度お試しください。'
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
    ja: '登録解除に失敗しました。もう一度お試しください。'
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
    ja: '通知の読み込みに失敗しました'
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
    'ja'
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
