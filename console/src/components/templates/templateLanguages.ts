import type { Template } from '../../services/api/template'

/**
 * A template's languages, the main one first: a translation counts once it has content (a visual
 * tree or MJML), since a send in that language falls back to the main one otherwise and a preview
 * has nothing to show for it.
 */
export function templateLanguages(template: Template, main: string): string[] {
  return [
    main,
    ...Object.entries(template.translations ?? {})
      .filter(([code, translation]) => code !== main && (translation.email?.visual_editor_tree || translation.email?.mjml_source))
      .map(([code]) => code)
  ]
}
