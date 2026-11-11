import type { EmailBlock } from '../types'

// Small trees for the library's placement tests. `b` builds a block, children last.

export const b = (
  id: string,
  type: string,
  children?: EmailBlock[],
  extra: Partial<EmailBlock> = {}
): EmailBlock =>
  ({
    id,
    type,
    attributes: {},
    ...(children ? { children } : {}),
    ...extra
  }) as unknown as EmailBlock

export const text = (id: string, content = '<p>Text</p>') => b(id, 'mj-text', undefined, { content } as Partial<EmailBlock>)

export const email = (bodyChildren: EmailBlock[], headChildren: EmailBlock[] = []): EmailBlock =>
  b('root', 'mjml', [b('head', 'mj-head', headChildren), b('body', 'mj-body', bodyChildren)])

export const condition = {
  display_condition: {
    operator: 'and',
    conditions: [{ variable: 'contact.first_name', operator: 'is_set' }]
  }
} as unknown as Partial<EmailBlock>
