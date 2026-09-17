# Security Policy

We take the security of Notifuse and its self-hosted users seriously. Thank you
for helping keep Notifuse and its community safe.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
pull requests, or discussions.**

Instead, use one of these private channels:

1. **GitHub Private Vulnerability Reporting (preferred).** Go to the
   [Security tab](https://github.com/Notifuse/notifuse/security/advisories/new)
   of this repository and open a private advisory. This keeps the report
   confidential and lets us collaborate on a fix and a CVE in one place.
2. **Email.** Send the details to **hello@notifuse.com**.

If you wish to encrypt your report, mention it in an initial message and we will
share a key.

### What to include

To help us triage quickly, please include as much of the following as you can:

- A description of the vulnerability and its impact.
- The affected component, endpoint, or file (and version / commit if known).
- Step-by-step reproduction instructions or a proof-of-concept.
- Any relevant logs, requests, or configuration (please redact secrets).
- Whether the issue is already public or known to third parties.

## Our Commitment

- We will acknowledge your report within **3 business days**.
- We will provide an initial assessment and a remediation plan within
  **10 business days**.
- We will keep you informed of progress and coordinate on a disclosure timeline.
- With your permission, we will credit you in the release notes and any advisory.
- We can request a CVE on your behalf, or coordinate if you prefer to request
  one yourself.

We aim to release fixes for confirmed high-severity issues as quickly as is
practical, and to publish a security advisory when a fix ships.

## Coordinated Disclosure

We follow a coordinated-disclosure model. Please give us a reasonable
opportunity to release a fix before any public disclosure. We will work with you
on timing and are happy to disclose promptly once a fix is available and users
have had a chance to upgrade.

## Safe Harbor

We consider security research and vulnerability disclosure conducted in good
faith and in accordance with this policy to be authorized. We will not pursue or
support legal action against researchers who:

- Make a good-faith effort to avoid privacy violations, data destruction, and
  service disruption.
- Only interact with accounts and data they own or have explicit permission to
  access.
- Give us a reasonable time to remediate before public disclosure.

## Supported Versions

Security fixes are applied to the latest released version. Self-hosted operators
are strongly encouraged to track the latest release. See
[CHANGELOG.md](CHANGELOG.md) for version history.

## Hardening Notes for Self-Hosted Operators

- Run Notifuse behind a network egress policy where possible. Several features
  make outbound HTTP requests to addresses a workspace member supplies —
  broadcast data feeds, outgoing webhook deliveries, and link/favicon previews.
  All of them are SSRF-protected by default: private, loopback, link-local and
  reserved targets are refused at dial time, on the first hop and on every
  redirect. Two settings relax this, each scoped to one feature and both off by
  default — `BROADCAST_DATA_FEED_ALLOW_PRIVATE_HOSTS` for data feeds and
  `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` for webhook delivery. Leave them
  disabled unless you deliberately target your own internal network.
- Grant workspace members the least privilege necessary; many privileged actions
  require resource-specific write permissions.
