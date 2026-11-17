// The gallery's email content in English, the reference the other languages translate.
//
// These are emails, not console strings: they are written in the workspace's language, not the
// member's, so they are not Lingui messages. Every language has the same keys (GalleryContent).
// Text in {{ }} or {% %} is Liquid and stays as it is; <b>, <br> and <a> are kept around the
// same words.
export const en = {
  common: {
    brand: 'Acme',
    logoAlt: 'Acme logo',
    address: 'Acme · 12 Market Street, San Francisco, CA 94103',
    signature: 'Thanks,<br><b>The Acme team</b>',
    subscribedTo: 'You are receiving this email because you subscribed to {{ list.name }}.',
    managePreferences: 'Manage preferences',
    unsubscribe: 'Unsubscribe',
    hasAccount: 'You are receiving this email because you have an account with us.',
    placedOrder: 'You are receiving this email because you placed an order with Acme.',
    invited: 'You are receiving this email because someone invited you to Acme.',
    questions: 'Questions? Reply to this email.',
    navShop: 'Shop',
    navBlog: 'Blog',
    navHelp: 'Help',
    linkFallback: 'Button not working? Copy this link into your browser:'
  },
  models: {
    blank: {
      name: 'New email',
      subject: 'A message from Acme',
      preheader: 'Write the line the inbox shows after the subject.',
      greeting: 'Hi {{ contact.first_name }},',
      body: 'Write your message here. Use the blocks on the left to add images, buttons and columns.'
    },
    welcome: {
      name: 'Welcome',
      subject: 'Welcome to Acme, {{ contact.first_name }}',
      preheader: 'Three things to do in your first week.',
      title: 'Welcome aboard, {{ contact.first_name }}',
      intro: 'We are glad you are here. Here is how to get the most out of Acme in your first week.',
      step1Title: 'Complete your profile',
      step1Text: 'Tell us what you are interested in.',
      step2Title: 'Explore the guides',
      step2Text: 'Short reads to get started fast.',
      step3Title: 'Say hello',
      step3Text: 'Reply to this email, we read every one.',
      button: 'Get started'
    },
    verify: {
      name: 'Verification code',
      subject: 'Your verification code: {{ code }}',
      preheader: 'It expires in {{ expires_in_minutes }} minutes.',
      title: 'Your verification code',
      intro: 'Enter this code to finish signing in. It expires in {{ expires_in_minutes }} minutes.',
      ignore: 'If you did not try to sign in, you can ignore this email. Someone may have typed your address by mistake.'
    },
    reset: {
      name: 'Password reset',
      subject: 'Reset your password',
      preheader: 'This link expires in 1 hour.',
      title: 'Reset your password',
      intro: 'Hi {{ contact.first_name }}, we received a request to reset your password. Click the button below to choose a new one. This link expires in 1 hour.',
      button: 'Choose a new password',
      ignore: 'If you did not ask for this, ignore this email. Your password will not change.'
    },
    confirm: {
      name: 'Subscription confirmation',
      subject: 'Please confirm your subscription',
      preheader: 'One click and you are in.',
      title: 'Confirm your subscription',
      intro: 'You asked to receive {{ list.name }}. Click the button below to confirm your address.',
      button: 'Yes, subscribe me',
      ignore: 'If you did not sign up, ignore this email: you will not be subscribed.'
    },
    invite: {
      name: 'Team invitation',
      subject: '{{ inviter_name }} invited you to join {{ team_name }}',
      preheader: 'Accept the invitation to get started.',
      title: '{{ inviter_name }} invited you to join {{ team_name }}',
      intro: 'Join the team to collaborate on projects and share files. The invitation expires in {{ expires_in_days }} days.',
      button: 'Accept the invitation'
    },
    receipt: {
      name: 'Payment receipt',
      subject: 'Your receipt from Acme #{{ invoice.number }}',
      preheader: 'Amount paid: {{ invoice.amount }}.',
      title: 'Thanks for your payment',
      intro: 'Here is your receipt. A PDF copy is available from your account.',
      invoice: 'Invoice',
      date: 'Date',
      description: 'Description',
      paymentMethod: 'Payment method',
      amountPaid: 'Amount paid',
      button: 'Download invoice'
    },
    order: {
      name: 'Order confirmation',
      subject: 'Your order #{{ order.number }} is confirmed',
      preheader: 'Thanks for your order. Here is your summary.',
      tag: 'Order #{{ order.number }}',
      title: 'Thanks for your order, {{ contact.first_name }}',
      intro: 'We are getting it ready. We will email you again when it ships.',
      quantity: 'Qty',
      subtotal: 'Subtotal',
      shipping: 'Shipping',
      total: 'Total',
      shippingTo: 'Shipping to',
      button: 'View my order'
    },
    shipping: {
      name: 'Order shipped',
      subject: 'Your order #{{ order.number }} is on its way',
      preheader: 'Estimated delivery: {{ shipment.estimated_delivery }}.',
      title: 'Your order is on its way',
      intro: 'Order #{{ order.number }} left our warehouse. Estimated delivery: <b>{{ shipment.estimated_delivery }}</b>.',
      carrier: 'Carrier',
      trackingNumber: 'Tracking number',
      button: 'Track my package'
    },
    cart: {
      name: 'Abandoned cart',
      subject: 'You left something in your cart',
      preheader: 'Your items are saved. Complete your order in one click.',
      title: 'You left something behind',
      intro: 'Hi {{ contact.first_name }}, your cart is saved. Complete your order before your items sell out.',
      quantity: 'Qty',
      button: 'Return to my cart',
      alsoLiked: 'Customers also loved',
      product1: 'Wool beanie',
      product2: 'Leather wallet',
      shopNow: 'Shop now'
    },
    promo: {
      name: 'Sale',
      subject: 'Winter sale: 30% off everything',
      preheader: 'Three days only. Use code WINTER30.',
      tag: 'Three days only',
      title: 'Winter sale: 30% off everything',
      intro: 'Our biggest sale of the year is here. Stock up on your favourites before Sunday midnight.',
      useCode: 'Use this code at checkout',
      ends: 'Ends November 30.',
      product1: 'Linen shirt',
      product2: 'Canvas tote bag',
      shopNow: 'Shop now',
      button: 'Shop the sale'
    },
    newsletter: {
      name: 'Newsletter',
      subject: 'The Acme digest',
      preheader: 'What we shipped, a guide and a customer story.',
      greeting: 'Hi {{ contact.first_name }}, here is what happened this month.',
      featuredTag: 'Product update',
      featuredTitle: 'Five things we shipped this month',
      featuredText: 'Faster search, a new mobile app and three features you asked for. Here is what changed and how to use it.',
      readArticle: 'Read the article →',
      article1Tag: 'Guide',
      article1Title: 'How to write a subject line people open',
      article1Text: 'Six patterns from our most-opened emails.',
      article2Tag: 'Customer story',
      article2Title: 'How a small shop doubled its repeat orders',
      article2Text: 'A welcome series and one well-timed reminder.',
      readMore: 'Read more →',
      quote: '“We moved our newsletters and receipts to one place and cut our sending costs by half.”',
      quoteAuthor: '<b>Maria Lopez</b> · Head of growth, Example Co.'
    },
    announce: {
      name: 'Product announcement',
      subject: 'Introducing Smart Reports',
      preheader: 'Your weekly numbers, written for you.',
      tag: 'New',
      title: 'Introducing Smart Reports',
      intro: 'Every Monday, a short report of what changed in your numbers and why, written in plain words.',
      feature1Title: 'Plain-language summaries',
      feature1Text: 'The three changes that matter, without a dashboard to read.',
      feature2Title: 'Alerts that explain themselves',
      feature2Text: 'When a number moves, the report says what moved it.',
      feature3Title: 'Share in one click',
      feature3Text: 'Send the report to your team as a link or a PDF.',
      button: 'Try Smart Reports'
    },
    event: {
      name: 'Event invitation',
      subject: 'You are invited: planning your holiday campaigns',
      preheader: 'Live webinar on December 4. Save your seat.',
      month: 'DEC',
      day: '04',
      title: 'Planning your holiday campaigns',
      when: 'Thursday, 5:00 PM CET · Online · 45 minutes',
      intro: 'Hi {{ contact.first_name }}, join our team for a live session on getting the most out of the busiest season of the year.',
      agenda1Time: '5:00 PM',
      agenda1: 'What worked last year',
      agenda2Time: '5:15 PM',
      agenda2: 'Building your calendar',
      agenda3Time: '5:35 PM',
      agenda3: 'Live questions',
      button: 'Save my seat',
      recording: 'Can’t make it? Register anyway and we will send you the recording.'
    },
    unsub: {
      name: 'Unsubscribe confirmation',
      subject: 'You have been unsubscribed',
      preheader: 'You will no longer receive {{ list.name }}.',
      title: 'You have been unsubscribed',
      intro: 'You will no longer receive {{ list.name }}. Your other subscriptions are unchanged.',
      button: 'Manage my subscriptions',
      mistake: 'Unsubscribed by mistake? You can subscribe again from the same page.'
    },
    winback: {
      name: 'We miss you',
      subject: 'We miss you, {{ contact.first_name }}',
      preheader: 'Here is 20% off to welcome you back.',
      title: 'We miss you, {{ contact.first_name }}',
      intro: 'It has been a while. A lot has changed since your last visit: new products, faster delivery and free returns.',
      tag: 'Welcome back',
      offer: '20% off your next order',
      valid: 'Valid for 14 days.',
      button: 'See what is new'
    }
  }
}

export type GalleryContent = typeof en
