/**
 * The address of a Supabase hook, to paste into Supabase › Authentication › Hooks. It carries the
 * integration's id, so it only exists once the integration is saved.
 */
export const supabaseWebhookURL = (
  hookType: 'auth-email' | 'before-user-created',
  workspaceID: string,
  integrationID: string
): string => {
  let defaultOrigin = window.location.origin
  if (defaultOrigin.includes('notifusedev.com')) {
    defaultOrigin = 'https://localapi.notifuse.com:4000'
  }
  const apiEndpoint = window.API_ENDPOINT?.trim() || defaultOrigin

  return `${apiEndpoint}/webhooks/supabase/${hookType}?workspace_id=${workspaceID}&integration_id=${integrationID}`
}
