import { useQuery } from '@tanstack/react-query'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import { compileTemplatePreview } from '../../templates/compileTemplatePreview'

/** The compiled preview of a template; the frame and the composer's rendered subject share it. */
export function useTemplatePreview(
  workspace: Workspace,
  template: Template | undefined,
  testData?: Record<string, unknown>
) {
  return useQuery({
    queryKey: ['template-preview', workspace.id, template?.id, template?.version, testData ?? null],
    queryFn: () =>
      compileTemplatePreview({
        workspace,
        email: template!.email,
        testData: testData ?? template!.test_data
      }),
    enabled: !!template,
    staleTime: 5 * 60_000,
    retry: false
  })
}
