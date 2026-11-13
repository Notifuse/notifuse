export const contactKey = (workspaceId: string, email: string) =>
  ['contact', workspaceId, email] as const
