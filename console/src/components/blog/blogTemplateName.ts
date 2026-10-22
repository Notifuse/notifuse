import { TEMPLATE_NAME_MAX_LENGTH } from '../../constants/limits'

/**
 * Name of the template that holds a blog post's content. A title can be longer than a
 * template name, so the name is cut to fit, by code points as the API counts them, so
 * an emoji is never split in half.
 */
export function blogTemplateName(title: string): string {
  return Array.from(`Blog: ${title}`).slice(0, TEMPLATE_NAME_MAX_LENGTH).join('')
}
