import type { MJMLComponentType } from './types'

/**
 * Default attribute values for MJML components based on official documentation
 * @see https://documentation.mjml.io/
 */

// mj-body defaults
export const MJ_BODY_DEFAULTS = {
  width: '600px',
  backgroundColor: '#ffffff'
}

// mj-wrapper defaults
export const MJ_WRAPPER_DEFAULTS = {
  backgroundColor: 'transparent',
  fullWidthBackgroundColor: 'transparent',
  paddingTop: '0px',
  paddingRight: '0px',
  paddingBottom: '0px',
  paddingLeft: '0px',
  cssClass: ''
}

// mjml defaults (root element has no attributes)
export const MJML_DEFAULTS = {}

// mj-section defaults
export const MJ_SECTION_DEFAULTS = {
  backgroundColor: 'transparent',
  backgroundUrl: '',
  backgroundRepeat: 'no-repeat',
  backgroundSize: 'auto',
  backgroundPosition: 'top center',
  border: 'none',
  borderTop: 'none',
  borderRight: 'none',
  borderBottom: 'none',
  borderLeft: 'none',
  borderRadius: '0px',
  direction: 'ltr' as const,
  fullWidth: '' as const,
  paddingTop: '20px',
  paddingRight: '0px',
  paddingBottom: '20px',
  paddingLeft: '0px',
  textAlign: 'center' as const,
  cssClass: ''
}

// mj-column defaults
export const MJ_COLUMN_DEFAULTS = {
  width: '100%',
  verticalAlign: 'top' as const,
  backgroundColor: 'transparent',
  innerBackgroundColor: 'transparent',
  border: 'none',
  borderTop: 'none',
  borderRight: 'none',
  borderBottom: 'none',
  borderLeft: 'none',
  borderRadius: '0px',
  innerBorder: 'none',
  innerBorderTop: 'none',
  innerBorderRight: 'none',
  innerBorderBottom: 'none',
  innerBorderLeft: 'none',
  innerBorderRadius: '0px',
  paddingTop: '0px',
  paddingRight: '0px',
  paddingBottom: '0px',
  paddingLeft: '0px',
  cssClass: ''
}

// mj-text defaults
export const MJ_TEXT_DEFAULTS = {
  align: 'left' as const,
  color: '#000000',
  fontFamily: 'Arial, sans-serif',
  fontSize: '13px',
  fontStyle: 'normal' as const,
  fontWeight: 'normal' as const,
  lineHeight: '1.4',
  textDecoration: 'none',
  textTransform: 'none' as const,
  paddingTop: '10px',
  paddingRight: '25px',
  paddingBottom: '10px',
  paddingLeft: '25px',
  backgroundColor: 'transparent'
}

// mj-button defaults
export const MJ_BUTTON_DEFAULTS = {
  align: 'center' as const,
  backgroundColor: '#414141',
  borderRadius: '3px',
  border: 'none',
  color: '#ffffff',
  fontFamily: 'Arial, sans-serif',
  fontSize: '13px',
  fontStyle: 'normal' as const,
  fontWeight: 'normal' as const,
  href: '',
  innerPadding: '10px 25px',
  lineHeight: '120%',
  paddingTop: '10px',
  paddingRight: '25px',
  paddingBottom: '10px',
  paddingLeft: '25px',
  target: '_blank' as const,
  // textAlign: 'none' as const, // better undefined than none
  textDecoration: 'none',
  textTransform: 'none' as const,
  verticalAlign: 'middle' as const,
  rel: 'noopener noreferrer'
}

// mj-image defaults
export const MJ_IMAGE_DEFAULTS = {
  align: 'center' as const,
  alt: '',
  border: 'none',
  borderRadius: '0px',
  containerBackgroundColor: 'transparent',
  fluidOnMobile: 'false' as const,
  height: 'auto',
  href: '',
  paddingTop: '10px',
  paddingRight: '25px',
  paddingBottom: '10px',
  paddingLeft: '25px',
  rel: 'noopener noreferrer',
  sizes: '',
  src: '',
  srcset: '',
  target: '_blank' as const,
  title: '',
  usemap: '',
  width: '100%'
}

// mj-head defaults (no visual attributes)
export const MJ_HEAD_DEFAULTS = {}

// mj-attributes defaults (dynamic component attributes)
export const MJ_ATTRIBUTES_DEFAULTS = {}

// mj-breakpoint defaults
export const MJ_BREAKPOINT_DEFAULTS = {
  width: '480px'
}

// mj-font defaults
export const MJ_FONT_DEFAULTS = {
  name: '',
  href: ''
}

// mj-html-attributes defaults (dynamic HTML attributes)
export const MJ_HTML_ATTRIBUTES_DEFAULTS = {}

// mj-preview defaults
export const MJ_PREVIEW_DEFAULTS = {
  content: ''
}

// mj-style defaults
export const MJ_STYLE_DEFAULTS = {
  inline: undefined as 'inline' | undefined,
  content: ''
}

// mj-title defaults
export const MJ_TITLE_DEFAULTS = {
  content: ''
}

// mj-group defaults
export const MJ_GROUP_DEFAULTS = {
  width: '100%',
  verticalAlign: 'top' as const,
  backgroundColor: 'transparent',
  direction: 'ltr' as const,
  cssClass: ''
}

// mj-raw defaults
export const MJ_RAW_DEFAULTS = {
  cssClass: ''
}

// mj-divider defaults
export const MJ_DIVIDER_DEFAULTS = {
  align: 'center' as const,
  borderColor: '#000000',
  borderStyle: 'solid' as const,
  borderWidth: '4px',
  containerBackgroundColor: 'transparent',
  paddingTop: '10px',
  paddingRight: '25px',
  paddingBottom: '10px',
  paddingLeft: '25px',
  width: '100%'
}

// mj-spacer defaults
export const MJ_SPACER_DEFAULTS = {
  height: '20px',
  containerBackgroundColor: 'transparent',
  paddingTop: '0px',
  paddingRight: '0px',
  paddingBottom: '0px',
  paddingLeft: '0px'
}

// mj-social defaults
export const MJ_SOCIAL_DEFAULTS = {
  align: 'center' as const,
  borderRadius: '3px',
  containerBackgroundColor: 'transparent',
  iconHeight: '20px',
  iconSize: '20px',
  innerPadding: '4px',
  lineHeight: '22px',
  mode: 'horizontal' as const,
  paddingTop: '10px',
  paddingRight: '25px',
  paddingBottom: '10px',
  paddingLeft: '25px',
  tableLayout: 'auto' as const,
  textPadding: '4px 4px 4px 0px'
}

// mj-social-element component defaults
export const MJ_SOCIAL_ELEMENT_DEFAULTS = {
  align: 'center',
  alt: '',
  backgroundColor: 'transparent', // Transparent background for true color icons
  borderRadius: '3px',
  color: '#333333',
  cssClass: undefined,
  fontFamily: 'Ubuntu, Helvetica, Arial, sans-serif',
  fontSize: '13px',
  fontStyle: 'normal',
  fontWeight: 'normal',
  href: undefined,
  iconHeight: undefined, // defaults to icon-size
  iconSize: '20px',
  iconPadding: '0px',
  // iconPosition: 'right', // Not supported by MJML
  lineHeight: '22px',
  name: undefined,
  padding: '4px',
  paddingTop: undefined,
  paddingRight: undefined,
  paddingBottom: undefined,
  paddingLeft: undefined,
  rel: undefined,
  sizes: undefined,
  src: undefined, // Each social name has its own default
  srcset: undefined,
  target: '_blank',
  textDecoration: 'none',
  textPadding: '4px 4px 4px 0',
  title: undefined,
  verticalAlign: 'middle'
}

/**
 * Comprehensive defaults mapping for all MJML component types
 */
export const MJML_COMPONENT_DEFAULTS: Record<MJMLComponentType, Record<string, any>> = {
  mjml: MJML_DEFAULTS,
  'mj-body': MJ_BODY_DEFAULTS,
  'mj-wrapper': MJ_WRAPPER_DEFAULTS,
  'mj-section': MJ_SECTION_DEFAULTS,
  'mj-column': MJ_COLUMN_DEFAULTS,
  'mj-text': MJ_TEXT_DEFAULTS,
  'mj-button': MJ_BUTTON_DEFAULTS,
  'mj-image': MJ_IMAGE_DEFAULTS,
  'mj-head': MJ_HEAD_DEFAULTS,
  'mj-attributes': MJ_ATTRIBUTES_DEFAULTS,
  'mj-breakpoint': MJ_BREAKPOINT_DEFAULTS,
  'mj-font': MJ_FONT_DEFAULTS,
  'mj-html-attributes': MJ_HTML_ATTRIBUTES_DEFAULTS,
  'mj-preview': MJ_PREVIEW_DEFAULTS,
  'mj-style': MJ_STYLE_DEFAULTS,
  'mj-title': MJ_TITLE_DEFAULTS,
  'mj-group': MJ_GROUP_DEFAULTS,
  'mj-raw': MJ_RAW_DEFAULTS,
  'mj-divider': MJ_DIVIDER_DEFAULTS,
  'mj-spacer': MJ_SPACER_DEFAULTS,
  'mj-social': MJ_SOCIAL_DEFAULTS,
  'mj-social-element': MJ_SOCIAL_ELEMENT_DEFAULTS
}

/**
 * Get default attributes for a specific MJML component type
 */
export const getComponentDefaults = (componentType: MJMLComponentType): Record<string, any> => {
  return MJML_COMPONENT_DEFAULTS[componentType] || {}
}

/**
 * Merge component attributes with defaults, giving priority to provided attributes
 */
export const mergeWithDefaults = (
  componentType: MJMLComponentType,
  attributes: Record<string, any> = {}
): Record<string, any> => {
  const defaults = getComponentDefaults(componentType)
  return { ...defaults, ...attributes }
}

/**
 * Common MJML measurement units
 */
export const MJML_UNITS = {
  PIXELS: 'px',
  PERCENT: '%',
  AUTO: 'auto'
} as const

/**
 * Common MJML color values
 */
export const MJML_COLORS = {
  TRANSPARENT: 'transparent',
  WHITE: '#ffffff',
  BLACK: '#000000',
  GRAY: '#414141'
} as const

/**
 * Common MJML font families
 */
export const MJML_FONTS = {
  DEFAULT: 'Arial, sans-serif',
  ARIAL: 'Arial, sans-serif',
  HELVETICA: 'Helvetica, Arial, sans-serif',
  UBUNTU: 'Arial, sans-serif'
} as const

/**
 * Common MJML alignment values
 */
export const MJML_ALIGNMENTS = {
  LEFT: 'left',
  CENTER: 'center',
  RIGHT: 'right',
  JUSTIFY: 'justify'
} as const

/**
 * Common MJML vertical alignment values
 */
export const MJML_VERTICAL_ALIGNMENTS = {
  TOP: 'top',
  MIDDLE: 'middle',
  BOTTOM: 'bottom'
} as const

/**
 * @deprecated Use GetInitialTemplate() instead
 */
export const INITIAL_EMAIL_TEMPLATE = {
  id: 'mjml-1',
  type: 'mjml' as const,
  children: [],
  attributes: {}
}
