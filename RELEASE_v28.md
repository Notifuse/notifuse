# Notifuse v28.0 Release Notes

**Release Date:** March 5, 2026

---

## What's New

### Visual or MJML — You Choose

When creating a new email template, you can now pick between the **visual drag-and-drop builder** or the **MJML code editor**. Use whichever workflow fits the task.

### Template Translation

Email templates can now be **translated into every language** configured in your workspace settings, making it easier to run multi-language campaigns from a single template.

### Configurable SMTP EHLO Hostname

Some SMTP servers reject the default `EHLO localhost` greeting. You can now set a **custom EHLO hostname** (e.g. your own domain) via:

- The `SMTP_EHLO_HOSTNAME` environment variable
- The setup wizard
- Workspace integration settings

When left empty, the hostname defaults to the SMTP host value.

---

## Bug Fixes

- **Contacts** — Removed the invalid "Blacklisted" status from the change-status dropdown and replaced it with the correct "Bounced" and "Complained" options.
- **Transactional Notifications** — Delivery stats (sent, delivered, failed, bounced) were always showing 0. Messages are now linked to their originating notification via a new `transactional_notification_id` column.
- **Email Builder** — `<mj-attributes>` global styles now apply correctly in both preview and sent emails.

---

## Upgrade Notes

This release includes a **workspace database migration** that adds a `transactional_notification_id` column to the message history table. The migration runs automatically on startup and is backward-compatible.

No manual action is required.
