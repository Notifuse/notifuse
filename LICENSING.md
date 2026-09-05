# Licensing

Notifuse is published under two licences, and which one applies depends on which
part of the repository — and which version — you are looking at.

## Which licence applies to what

| | Licence | |
|---|---|---|
| **Notifuse v39.x and earlier** | AGPL-3.0-or-later | Permanently. This change is not retroactive and cannot be. |
| **Notifuse v40.0 and later** | [Business Source License 1.1](LICENSE) | Becomes AGPL-3.0-or-later four years after each version is published. |
| **`web_analytics_sdk/`** | AGPL-3.0-or-later | A separate work, in every version. See [`web_analytics_sdk/LICENSE`](web_analytics_sdk/LICENSE). |

The four-year clock runs **per version**. Notifuse v40.0 becomes AGPL four years
after v40.0 was published; v41.0 four years after v41.0 was published. The dates
do not move, and they do not depend on us still being here.

## What you may do

The Business Source License grants you the right to copy, modify, create
derivative works from, and redistribute Notifuse. On top of that, our Additional
Use Grant gives you the right to **run it in production, including making it
available to third parties** — you may host Notifuse for other people, and you
may charge them for it.

The one thing the grant does not cover is using a **Licensed Feature** in
production without a valid licence key. There are five, and they are listed in
the [LICENSE](LICENSE) file itself rather than on a web page, so that the scope
of the licence is fixed by the version you downloaded and cannot be changed
under you afterwards:

1. Creating more than three workspaces on a single deployment.
2. Creating or editing a granular permission set for a member or an API key.
3. Provisioning an Amazon SES tenant.
4. Signing in through Single Sign-On (OpenID Connect).
5. Creating or editing a multilingual variant of a template.

Everything else — unlimited emails, unlimited contacts, unlimited team members,
the whole automation engine, web analytics, the transactional API, every email
provider integration — needs no key, and
[notifuse.com/licence-features](https://notifuse.com/licence-features) lists what
a licence will never cover, with 90 days' notice before that list can change.

## What the licence key is

A signed file. You paste it into Settings › Licence as the root user, or set
`NOTIFUSE_LICENSE_KEY`. It is verified offline against a public key compiled into
the binary: nothing is sent anywhere, there is no account to create, and no part
of your installation is reported to us.

A refusal refuses one action, at the moment you take it. Nothing is deleted,
hidden, disabled or made read-only, no part of the console is walled off, and
**the send path contains no licence check of any kind** — scheduled broadcasts,
transactional email, webhooks and API keys keep working with no key at all.

An installation that already exceeds a limit keeps everything it has. Eight
workspaces stay eight; it simply cannot create a ninth.

## Every version becomes AGPL

This is the part that makes the rest of it bearable, so it is worth stating
plainly: **every version of Notifuse published under the BSL becomes AGPL-3.0-or-later
four years after it ships.** That is not a promise about a future decision. It is
written into the licence of every copy you download, it applies version by
version, and it applies whatever happens to this company or to the person behind
it.

The Business Source License is not itself an open-source licence — its own Notice
section says so — because it restricts one kind of production use. What it
guarantees is that the restriction expires.

## Contributions

We do not currently accept pull requests. If we ever invite one, it will be under
the [CLA](CLA.md).
