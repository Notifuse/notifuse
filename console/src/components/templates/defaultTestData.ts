// The test data a new template starts with: example values of the variables a send
// fills in, so the preview renders them.
// Note: Preview links (mid=preview) show a "Preview Mode" message in the notification center
// Real emails use secure HMACs for authentication
// The `workspace` object (base_url / website_url) is intentionally omitted: the compile
// endpoint injects it server-side (single source of truth), so it renders in the preview
// and is echoed back without being hard-coded here.
export function buildDefaultTestData(endpoint: string, workspaceId: string) {
  return {
    contact: {
      first_name: 'John',
      last_name: 'Doe',
      email: 'john.doe@example.com'
    },
    list: {
      id: 'newsletter',
      name: 'Newsletter'
    },
    unsubscribe_url: `${endpoint}/notification-center?action=unsubscribe&email=john.doe@example.com&lid=newsletter&lname=Newsletter&wid=${workspaceId}&mid=preview&email_hmac=abc123`,
    confirm_subscription_url: `${endpoint}/notification-center?action=confirm&email=john.doe@example.com&lid=newsletter&lname=Newsletter&wid=${workspaceId}&mid=preview&email_hmac=abc123`,
    notification_center_url: `${endpoint}/notification-center?email=john.doe@example.com&email_hmac=abc123&wid=${workspaceId}`,
    tracking_preferences_url: `${endpoint}/notification-center?action=tracking&email=john.doe@example.com&email_hmac=abc123&wid=${workspaceId}`,
    tracking_optout_url: `${endpoint}/notification-center?action=tracking_optout&email=john.doe@example.com&email_hmac=abc123&wid=${workspaceId}`,
    // A preview has no sent message: this address opens a page saying so. A broadcast or
    // an automation fills in the real one.
    view_in_browser_url: `${endpoint}/v/preview`
  }
}
