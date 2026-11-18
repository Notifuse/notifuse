// The root email the setup page hands to the sign-in page across the full reload that follows
// the setup. Kept in this tab's session storage rather than the URL: an address in a query
// string ends up in server logs and history, and the sign-in page sends a code for it at once.
const KEY = 'notifuse_setup_signin_email'

/** Called by the setup page just before it reloads into the sign-in page. */
export function handOffSignInEmail(email: string | undefined): void {
  try {
    if (email) sessionStorage.setItem(KEY, email)
  } catch {
    // No storage (private mode, blocked site data): the field just stays empty.
  }
}

/** Read once by the sign-in page: the address it prefills, without sending a code. */
export function takeSignInEmail(): string | undefined {
  try {
    const email = sessionStorage.getItem(KEY) ?? undefined
    sessionStorage.removeItem(KEY)
    return email || undefined
  } catch {
    return undefined
  }
}
