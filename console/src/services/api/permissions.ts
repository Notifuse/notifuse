/**
 * The permission model, mirroring the backend's own definition of it.
 *
 * It lives on its own, with no runtime imports, because AuthContext and WorkspaceLayout build
 * permission sets at module scope and both sit in an import cycle with ./client (client → router
 * → RootLayout → AuthContext). Reaching the constructors through ./workspace would evaluate them
 * while that module is still in the temporal dead zone.
 *
 * The two imports below survive that rule: `MessageDescriptor` is a type and erases, and the
 * `msg` macro is compiled away into plain descriptor objects, so neither reaches the module graph.
 */
import { msg } from "@lingui/core/macro";
import type { MessageDescriptor } from "@lingui/core";

export interface ResourcePermissions {
  read: boolean;
  write: boolean;
}

export type PermissionResource =
  | "contacts"
  | "lists"
  | "templates"
  | "broadcasts"
  | "transactional"
  | "workspace"
  | "message_history"
  | "blog"
  | "automations"
  | "llm"
  | "web_analytics"
  | "segments"
  | "webhook_subscriptions"
  | "webhook_events";

// Mirrors domain.AllPermissionResources, in the same order. Anything that renders or builds a
// permission set iterates this rather than the keys of a stored map, which may be partial.
export const ALL_PERMISSION_RESOURCES: PermissionResource[] = [
  // Audience
  "contacts",
  "segments",
  "lists",
  // Content
  "templates",
  "blog",
  // Sending
  "broadcasts",
  "transactional",
  "automations",
  // Reporting
  "message_history",
  "web_analytics",
  // Integrations
  "webhook_subscriptions",
  "webhook_events",
  "llm",
  // Workspace
  "workspace",
];

export type PermissionType = "read" | "write";

/**
 * The verbs no gate can enforce as the API stands: `/api/llm.chat` is the only LLM route, and all
 * four message-history service methods are reads.
 *
 * The matrix renders them granted and locked rather than denied, because a stored `false` here is
 * permanent: every permission backfill only ever adds the keys a row is missing, so nothing would
 * widen it if a later release gives the verb a real gate.
 */
const UNENFORCED_PERMISSIONS: ReadonlyArray<
  readonly [PermissionResource, PermissionType]
> = [
  ["llm", "read"],
  ["message_history", "write"],
  ["webhook_events", "write"],
];

export function isPermissionEnforced(
  resource: PermissionResource,
  type: PermissionType,
): boolean {
  return !UNENFORCED_PERMISSIONS.some(([r, p]) => r === resource && p === type);
}

/**
 * One endpoint a verb gates, and what holding that verb lets the caller do with it.
 *
 * `endpoint` is an identifier and is never translated; only `action` is.
 */
export interface PermissionEndpoint {
  readonly endpoint: string;
  readonly action: MessageDescriptor;
}

/**
 * What one verb of one resource gates.
 *
 * `endpoints` is empty exactly when the verb gates nothing today, and `note` then says why in
 * plain words — an empty section would read as an omission, and for the two verbs the matrix
 * renders granted-and-locked the owner is owed the reason they cannot turn them off.
 */
export interface PermissionVerbDetail {
  readonly endpoints: readonly PermissionEndpoint[];
  readonly note?: MessageDescriptor;
}

/** The full explanation of one row of the permission matrix. */
export interface PermissionDescriptor {
  readonly scope: MessageDescriptor;
  readonly read: PermissionVerbDetail;
  readonly write: PermissionVerbDetail;
  // Present when the grant reaches further than its name suggests, or when something else
  // reaches what it names.
  readonly caveat?: MessageDescriptor;
}

/**
 * Keyed by PermissionResource on purpose: a resource added to the union without an entry here,
 * or a misspelt key, is a compile error rather than a row that silently explains nothing.
 *
 * Lazy descriptors, not `t`: this is built at module scope, before any catalog is active, and is
 * rendered through i18n._() at the moment the popover opens.
 */
export const PERMISSION_DESCRIPTORS: Record<
  PermissionResource,
  PermissionDescriptor
> = {
  contacts: {
    scope: msg`Contact records and their fields, contact timelines, and custom events recorded against contacts.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/contacts.list",
          action: msg`List contacts with their full profile fields`,
        },
        {
          endpoint: "/api/contacts.count",
          action: msg`Count every contact in the workspace`,
        },
        {
          endpoint: "/api/contacts.getByEmail",
          action: msg`Look up a contact's complete record by email address`,
        },
        {
          endpoint: "/api/contacts.getByExternalID",
          action: msg`Look up a contact's complete record by your own external ID`,
        },
        {
          endpoint: "/api/timeline.list",
          action: msg`Read one contact's full activity timeline`,
        },
        {
          endpoint: "/api/customEvents.get",
          action: msg`Read one custom event recorded against a contact`,
        },
        {
          endpoint: "/api/customEvents.list",
          action: msg`List custom events recorded against contacts`,
        },
        {
          endpoint: "/api/segments.preview",
          action: msg`Count the contacts matching an arbitrary segment definition (also needs Segments read)`,
        },
        {
          endpoint: "/api/segments.contacts",
          action: msg`List the email addresses inside a segment (also needs Segments read)`,
        },
        {
          endpoint: "/api/contactLists.getContactsByList",
          action: msg`Enumerate every subscriber address on a list (also needs Lists read)`,
        },
        {
          endpoint: "/api/contactLists.getListsByContact",
          action: msg`Confirm a contact exists and list everything it is subscribed to (also needs Lists read)`,
        },
        {
          endpoint: "/api/analytics.query",
          action: msg`Run aggregate queries over the \`contacts\` schema`,
        },
        {
          endpoint: "/api/analytics.schemas",
          action: msg`See the \`contacts\` schema listed in the queryable catalogue (the listing is filtered to what you may read, never refused)`,
        },
        {
          endpoint: "/api/transactional.send",
          action: msg`Set cc or bcc on a transactional send, which renders the subject against those recipients' whole records (also needs Transactional write)`,
        },
        {
          endpoint: "/api/transactional.testTemplate",
          action: msg`Set cc or bcc on a test send (also needs Transactional write)`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/contacts.upsert",
          action: msg`Create a contact or overwrite its fields`,
        },
        {
          endpoint: "/api/contacts.import",
          action: msg`Bulk create contacts and overwrite every field of every contact in the payload`,
        },
        {
          endpoint: "/api/contacts.delete",
          action: msg`Permanently delete any contact`,
        },
        {
          endpoint: "/api/customEvents.upsert",
          action: msg`Record or overwrite a custom event on a contact`,
        },
        {
          endpoint: "/api/customEvents.import",
          action: msg`Bulk record custom events against contacts`,
        },
        {
          endpoint: "/api/contactLists.updateStatus",
          action: msg`Change a contact's subscription status on a list (also needs Lists write)`,
        },
        {
          endpoint: "/api/contactLists.removeContact",
          action: msg`Remove a contact from a list (also needs Lists write)`,
        },
        {
          endpoint: "/api/transactional.send",
          action: msg`Write contact fields beyond the recipient's email address; without it the send still creates the recipient but carries only the email (also needs Transactional write)`,
        },
      ],
    },
    caveat: msg`Write includes permanent deletion and bulk overwriting: \`/api/contacts.import\` overwrites every field each row carries, so a single call can rewrite your whole contact database. This is not "can add contacts". Read also covers custom events and the contact timeline, which the name does not suggest. Two other grants reach contact data without this one: Lists write on its own lets anyone create or overwrite a complete contact record through \`/api/lists.subscribe\`, and Automations write can send the complete contact record to any URL through a webhook node.`,
  },

  segments: {
    scope: msg`Dynamic segment definitions, their compiled queries, and the background tasks that build them.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/segments.list",
          action: msg`List segments with their full definition trees`,
        },
        {
          endpoint: "/api/segments.get",
          action: msg`Read one segment's definition and generated SQL`,
        },
        {
          endpoint: "/api/segments.preview",
          action: msg`Count the contacts an arbitrary, unsaved segment tree would match (also needs Contacts read)`,
        },
        {
          endpoint: "/api/segments.contacts",
          action: msg`List the email addresses currently in a segment (also needs Contacts read)`,
        },
        {
          endpoint: "/api/tasks.list",
          action: msg`See segment build and recompute tasks; a listing that names no type is narrowed to the types you may read`,
        },
        {
          endpoint: "/api/tasks.get",
          action: msg`Read one segment build/recompute task's state and progress`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/segments.create",
          action: msg`Create a segment and start its first build`,
        },
        {
          endpoint: "/api/segments.update",
          action: msg`Change a segment's definition`,
        },
        {
          endpoint: "/api/segments.delete",
          action: msg`Delete a segment`,
        },
        {
          endpoint: "/api/segments.rebuild",
          action: msg`Force a full rebuild of a segment`,
        },
        {
          endpoint: "/api/tasks.trigger",
          action: msg`Run a segment build/recompute task immediately`,
        },
        {
          endpoint: "/api/tasks.reset",
          action: msg`Reset a segment task's progress so it re-runs from the start`,
        },
        {
          endpoint: "/api/tasks.delete",
          action: msg`Delete a segment build/recompute task`,
        },
      ],
    },
    caveat: msg`Preview accepts any definition sent to it, not just a saved segment, so anyone holding it together with Contacts read can ask counting questions about contact attributes, list membership, custom events and timeline — one count at a time, on any condition they like. Segments read alone cannot reach it. Segment permissions also quietly decide who may run three kinds of background task on the shared /api/tasks.* endpoints.`,
  },

  lists: {
    scope: msg`Mailing lists, their settings and statistics, and contact-to-list membership rows.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/lists.list",
          action: msg`List every list and its settings, including double opt-in and template configuration`,
        },
        {
          endpoint: "/api/lists.get",
          action: msg`Read one list's full settings`,
        },
        {
          endpoint: "/api/lists.stats",
          action: msg`Read a list's subscriber counts by status`,
        },
        {
          endpoint: "/api/contactLists.getByIDs",
          action: msg`Read one contact's membership row on one list`,
        },
        {
          endpoint: "/api/contactLists.getContactsByList",
          action: msg`Enumerate every subscriber address on a list (also needs Contacts read)`,
        },
        {
          endpoint: "/api/contactLists.getListsByContact",
          action: msg`List everything one contact is subscribed to (also needs Contacts read)`,
        },
      ],
    },
    write: {
      endpoints: [
        { endpoint: "/api/lists.create", action: msg`Create a list` },
        {
          endpoint: "/api/lists.update",
          action: msg`Change a list's settings, including its double opt-in and public flag`,
        },
        { endpoint: "/api/lists.delete", action: msg`Delete a list` },
        {
          endpoint: "/api/lists.subscribe",
          action: msg`Subscribe an address to any list, public or not — and create or overwrite that contact's record in the same call`,
        },
        {
          endpoint: "/api/contactLists.updateStatus",
          action: msg`Change a contact's subscription status on a list (also needs Contacts write)`,
        },
        {
          endpoint: "/api/contactLists.removeContact",
          action: msg`Remove a contact from a list (also needs Contacts write)`,
        },
        {
          endpoint: "/api/contacts.import",
          action: msg`Subscribe imported contacts to lists — required only when the import names lists (also needs Contacts write)`,
        },
      ],
    },
    caveat: msg`Lists write also grants contact writing. Subscribing an address through \`/api/lists.subscribe\` creates or overwrites that contact's whole record, with every field the request carries and without needing Contacts write — so anyone holding only Lists write can still rewrite contact data. It can also subscribe someone to a list that is not public. Read is not confined to list settings either: through /api/contactLists.* it lists the addresses of a list's members, which is why those two endpoints additionally require Contacts read.`,
  },

  templates: {
    scope: msg`Email templates in every language, and the reusable template blocks stored on the workspace.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/templates.list",
          action: msg`List templates with their full MJML bodies`,
        },
        {
          endpoint: "/api/templates.get",
          action: msg`Read one template, at any stored version`,
        },
        {
          endpoint: "/api/templates.compile",
          action: msg`Compile MJML to HTML with test data`,
        },
        {
          endpoint: "/api/templateBlocks.list",
          action: msg`List the workspace's reusable template blocks`,
        },
        {
          endpoint: "/api/templateBlocks.get",
          action: msg`Read one reusable template block`,
        },
      ],
    },
    write: {
      endpoints: [
        { endpoint: "/api/templates.create", action: msg`Create a template` },
        {
          endpoint: "/api/templates.update",
          action: msg`Change a template's content, in any language`,
        },
        { endpoint: "/api/templates.delete", action: msg`Delete a template` },
        {
          endpoint: "/api/templateBlocks.create",
          action: msg`Create a reusable template block`,
        },
        {
          endpoint: "/api/templateBlocks.update",
          action: msg`Change a reusable template block, which changes every template that embeds it`,
        },
        {
          endpoint: "/api/templateBlocks.delete",
          action: msg`Delete a reusable template block`,
        },
      ],
    },
    caveat: msg`Covers more than templates: reusable blocks answer to the same switch, and editing a block edits every template that embeds it. Blocks are kept with the workspace's own settings, so their content also comes back from /api/workspaces.get and from /api/workspaces.list, which needs no permission at all — Templates read is not the only way to see block content. And /api/templates.compile only checks this grant for calls arriving over the API; when the workspace compiles a template for itself, nothing is checked.`,
  },

  blog: {
    scope: msg`Blog categories, posts and themes, plus the workspace-level blog configuration.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/blogCategories.list",
          action: msg`List blog categories`,
        },
        {
          endpoint: "/api/blogCategories.get",
          action: msg`Read one category by ID or slug`,
        },
        {
          endpoint: "/api/blogPosts.list",
          action: msg`List blog posts, including unpublished drafts`,
        },
        {
          endpoint: "/api/blogPosts.get",
          action: msg`Read one post, including unpublished drafts`,
        },
        {
          endpoint: "/api/blogThemes.list",
          action: msg`List theme versions`,
        },
        {
          endpoint: "/api/blogThemes.get",
          action: msg`Read one theme version, including its scripts.js`,
        },
        {
          endpoint: "/api/blogThemes.getPublished",
          action: msg`Read the currently published theme`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/blogCategories.create",
          action: msg`Create a blog category`,
        },
        {
          endpoint: "/api/blogCategories.update",
          action: msg`Rename or re-slug a category`,
        },
        {
          endpoint: "/api/blogCategories.delete",
          action: msg`Delete a category`,
        },
        { endpoint: "/api/blogPosts.create", action: msg`Create a post` },
        { endpoint: "/api/blogPosts.update", action: msg`Edit any post` },
        { endpoint: "/api/blogPosts.delete", action: msg`Delete a post` },
        {
          endpoint: "/api/blogPosts.publish",
          action: msg`Publish a post to the live blog`,
        },
        {
          endpoint: "/api/blogPosts.unpublish",
          action: msg`Take a published post off the live blog`,
        },
        {
          endpoint: "/api/blogThemes.create",
          action: msg`Create a theme version`,
        },
        {
          endpoint: "/api/blogThemes.update",
          action: msg`Edit a theme version, including its scripts.js`,
        },
        {
          endpoint: "/api/blogThemes.publish",
          action: msg`Make a theme version live on the blog's public domain, replacing the current one`,
        },
        {
          endpoint: "/api/workspaces.setBlogSettings",
          action: msg`Turn the blog on or off and change its title, SEO, pagination and feed settings`,
        },
      ],
    },
    caveat: msg`Blog write publishes JavaScript on your own domain. A theme carries its own \`scripts.js\`, and /api/blogThemes.publish makes it live for every visitor of the blog's custom domain — so this switch is the right to run any script on your site, not just to edit content. It also reaches workspace configuration: /api/workspaces.setBlogSettings answers to Blog write rather than Workspace write, so whoever you hand the blog to can also switch the blog feature itself on or off.`,
  },

  broadcasts: {
    scope: msg`Marketing campaigns: their drafts, A/B variations, data feeds, schedules and send lifecycle.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/broadcasts.list",
          action: msg`List broadcasts with their audience and schedule`,
        },
        {
          endpoint: "/api/broadcasts.get",
          action: msg`Read one broadcast's full configuration`,
        },
        {
          endpoint: "/api/broadcasts.getTestResults",
          action: msg`Read A/B test results for a broadcast`,
        },
        {
          endpoint: "/api/analytics.query",
          action: msg`Run aggregate queries over the \`broadcasts\` schema`,
        },
        {
          endpoint: "/api/analytics.schemas",
          action: msg`See the \`broadcasts\` schema listed in the queryable catalogue`,
        },
        {
          endpoint: "/api/tasks.list",
          action: msg`See broadcast send tasks; a listing that names no type is narrowed to the types you may read`,
        },
        {
          endpoint: "/api/tasks.get",
          action: msg`Read a send task's progress`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/broadcasts.create",
          action: msg`Create a broadcast`,
        },
        {
          endpoint: "/api/broadcasts.update",
          action: msg`Edit a broadcast, including its audience and templates`,
        },
        {
          endpoint: "/api/broadcasts.schedule",
          action: msg`Schedule or immediately send a broadcast to its entire audience`,
        },
        {
          endpoint: "/api/broadcasts.pause",
          action: msg`Pause a broadcast mid-send`,
        },
        {
          endpoint: "/api/broadcasts.resume",
          action: msg`Resume a paused broadcast, continuing the send`,
        },
        {
          endpoint: "/api/broadcasts.cancel",
          action: msg`Cancel a broadcast`,
        },
        {
          endpoint: "/api/broadcasts.delete",
          action: msg`Delete a broadcast`,
        },
        {
          endpoint: "/api/broadcasts.sendToIndividual",
          action: msg`Send a broadcast to one named address`,
        },
        {
          endpoint: "/api/broadcasts.selectWinner",
          action: msg`Pick the winning A/B variation and send it to the remaining audience`,
        },
        {
          endpoint: "/api/broadcasts.refreshGlobalFeed",
          action: msg`Refresh the broadcast's data feed by calling the configured external URL`,
        },
        {
          endpoint: "/api/broadcasts.testRecipientFeed",
          action: msg`Fire a test data-feed request for a named recipient`,
        },
        {
          endpoint: "/api/tasks.trigger",
          action: msg`Start a broadcast send task immediately`,
        },
        {
          endpoint: "/api/tasks.reset",
          action: msg`Reset a send task, so a broadcast re-sends from the start`,
        },
        { endpoint: "/api/tasks.delete", action: msg`Delete a send task` },
      ],
    },
    caveat: msg`Broadcasts write sends. The same switch that lets someone draft a campaign lets them push it to the entire audience — through /api/broadcasts.schedule, and again through /api/tasks.trigger and /api/tasks.reset on the underlying send task. Note that Broadcasts read does NOT give delivery statistics: open/click/bounce numbers come from /api/messages.broadcastStats and siblings, which are gated on Message History read. It also does not grant the recipient list — that is Contacts read.`,
  },

  transactional: {
    scope: msg`Transactional notification definitions and every path that actually sends an email through the workspace's provider credentials.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/transactional.list",
          action: msg`List transactional notification definitions`,
        },
        {
          endpoint: "/api/transactional.get",
          action: msg`Read one transactional notification's channel and template configuration`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/transactional.create",
          action: msg`Create a transactional notification`,
        },
        {
          endpoint: "/api/transactional.update",
          action: msg`Edit a transactional notification`,
        },
        {
          endpoint: "/api/transactional.delete",
          action: msg`Delete a transactional notification`,
        },
        {
          endpoint: "/api/transactional.send",
          action: msg`Send a real transactional email to any address; adding cc or bcc additionally needs Contacts read, and writing contact fields beyond the recipient's email additionally needs Contacts write`,
        },
        {
          endpoint: "/api/transactional.testTemplate",
          action: msg`Send a real test email through a chosen integration and sender; cc or bcc additionally needs Contacts read`,
        },
        {
          endpoint: "/api/email.testProvider",
          action: msg`Send a real test email through the workspace's stored provider credentials`,
        },
        {
          endpoint: "SMTP bridge (SMTP submission, not an HTTP route)",
          action: msg`Send a transactional notification by SMTP using the key's API email address as the username — the same gate as /api/transactional.send`,
        },
      ],
    },
    caveat: msg`A send-only key really is send-only, and this is new in 39.0: /api/transactional.send still creates the recipient contact, but without Contacts write it stores nothing beyond the email address, and without Contacts read it refuses cc and bcc outright — the subject line is rendered against each recipient's whole record. The SMTP bridge is a second front door to the same switch, so this is also exactly what the SMTP username can do. Note that the write verb sends real mail from three separate endpoints, including one that only claims to "test" a provider.`,
  },

  automations: {
    scope: msg`Automation flows — their triggers, nodes and per-contact execution history.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/automations.list",
          action: msg`List automations with their complete node graphs`,
        },
        {
          endpoint: "/api/automations.get",
          action: msg`Read one automation's full node configuration, including webhook node URLs and bearer secrets`,
        },
        {
          endpoint: "/api/automations.nodeExecutions",
          action: msg`Read one named contact's journey through an automation, node by node`,
        },
        {
          endpoint: "/api/analytics.query",
          action: msg`Run aggregate queries over the \`automation_node_executions\` schema`,
        },
        {
          endpoint: "/api/analytics.schemas",
          action: msg`See the \`automation_node_executions\` schema listed in the queryable catalogue`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/automations.create",
          action: msg`Create an automation, including webhook, email and list nodes`,
        },
        {
          endpoint: "/api/automations.update",
          action: msg`Edit any automation's nodes, including where its webhook nodes post`,
        },
        {
          endpoint: "/api/automations.delete",
          action: msg`Delete an automation and exit every contact currently in it`,
        },
        {
          endpoint: "/api/automations.activate",
          action: msg`Make an automation live and install its database trigger, so contacts begin flowing through it`,
        },
        {
          endpoint: "/api/automations.pause",
          action: msg`Pause a live automation`,
        },
      ],
    },
    caveat: msg`Automations write is an indirect contact read that Contacts read cannot restrain: a webhook node sends the complete contact record to any URL the automation names, and nothing checks permissions while an automation runs. The same nodes add contacts to and remove them from lists, and send email, so this switch changes list membership and sends mail with no Lists or Transactional grant at all. Automations read returns a webhook node's full configuration, including the bearer secret saved on it.`,
  },

  message_history: {
    scope: msg`The outbound message log: every email the workspace sent, its delivery status, and the engagement counted against it.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/messages.list",
          action: msg`List sent messages with their recipient address, delivery and engagement timestamps, and the data each was rendered with`,
        },
        {
          endpoint: "/api/messages.broadcastStats",
          action: msg`Read a broadcast's delivered, open, click, bounce and complaint counts`,
        },
        {
          endpoint: "/api/messages.broadcastVariationStats",
          action: msg`Read the same counts for one A/B variation`,
        },
        {
          endpoint: "/api/messages.broadcastLinkStats",
          action: msg`Read per-link click counts for one broadcast template`,
        },
        {
          endpoint: "/api/analytics.query",
          action: msg`Run aggregate queries over the \`message_history\` and \`email_queue\` schemas`,
        },
        {
          endpoint: "/api/analytics.schemas",
          action: msg`See the \`message_history\` and \`email_queue\` schemas listed in the queryable catalogue`,
        },
      ],
    },
    write: {
      endpoints: [],
      note: msg`This verb gates nothing today. Every message history call the API offers is a read, and rows are written by the sending pipeline itself, never by anything you can call. The switch is granted and locked rather than denied, because storing it as "off" would be permanent: filling in a missing permission only ever adds what a grant lacks, so nothing would widen it if a later release gives this verb something real to gate.`,
    },
    caveat: msg`This is where delivery statistics live, not under Broadcasts: a key that may create and send a broadcast still cannot read its open, click or bounce numbers without Message History read. The log carries recipient addresses and the rendered message data, so it is contact data by another name.`,
  },

  web_analytics: {
    scope: msg`Website visitor analytics — sessions, page views and goals — the annotations drawn on those charts, and the historical backfill.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/analytics.query",
          action: msg`Run aggregate queries over the \`web_sessions\`, \`web_pages\` and \`web_goals\` schemas`,
        },
        {
          endpoint: "/api/analytics.schemas",
          action: msg`See the web analytics schemas listed in the queryable catalogue`,
        },
        {
          endpoint: "/api/annotations.list",
          action: msg`List the annotations drawn on the analytics timeline`,
        },
        {
          endpoint: "/api/annotations.get",
          action: msg`Read one annotation`,
        },
        {
          endpoint: "/api/webAnalytics.backfillStatus",
          action: msg`Read the progress of the historical backfill`,
        },
        {
          endpoint: "/api/tasks.list",
          action: msg`See web analytics backfill tasks; a listing that names no type is narrowed to the types you may read`,
        },
        {
          endpoint: "/api/tasks.get",
          action: msg`Read a backfill task's state and progress`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/webAnalytics.backfillStart",
          action: msg`Start a historical backfill of web analytics data`,
        },
        {
          endpoint: "/api/webAnalytics.backfillCancel",
          action: msg`Cancel a running backfill`,
        },
        {
          endpoint: "/api/annotations.create",
          action: msg`Add an annotation to the analytics timeline`,
        },
        {
          endpoint: "/api/annotations.update",
          action: msg`Edit any annotation`,
        },
        {
          endpoint: "/api/annotations.delete",
          action: msg`Delete an annotation`,
        },
        {
          endpoint: "/api/workspaces.setWebAnalyticsSettings",
          action: msg`Turn web analytics on or off and change its allowed domains, filters, custom dimension labels and email-link identification`,
        },
        {
          endpoint: "/api/tasks.trigger",
          action: msg`Run a backfill task immediately`,
        },
        {
          endpoint: "/api/tasks.reset",
          action: msg`Reset a backfill task so it re-runs from the start`,
        },
        {
          endpoint: "/api/tasks.delete",
          action: msg`Delete a backfill task`,
        },
      ],
    },
    caveat: msg`Annotations have no permission of their own: they deliberately answer to this switch, so a Web Analytics grant is also the right to write on the shared timeline everyone reads. Write reaches workspace configuration too — /api/workspaces.setWebAnalyticsSettings answers to Web Analytics write rather than Workspace write, including the email-link identification setting that ties recipients to their browsing. The public /track collector is governed by nothing at all: it accepts data without any authentication, and no grant here changes that.`,
  },

  webhook_subscriptions: {
    scope: msg`Outbound webhook subscriptions — where the workspace POSTs its own events — and their delivery log.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/webhookSubscriptions.list",
          action: msg`List subscriptions with their target URL, event types and filters`,
        },
        {
          endpoint: "/api/webhookSubscriptions.get",
          action: msg`Read one subscription; its signing secret is redacted unless you own the workspace`,
        },
        {
          endpoint: "/api/webhookSubscriptions.deliveries",
          action: msg`Read the delivery log, including each event payload and the endpoint's response status and body`,
        },
        {
          endpoint: "/api/analytics.query",
          action: msg`Run aggregate queries over the \`webhook_deliveries\` schema`,
        },
        {
          endpoint: "/api/analytics.schemas",
          action: msg`See the \`webhook_deliveries\` schema listed in the queryable catalogue`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/webhookSubscriptions.create",
          action: msg`Create a subscription that POSTs the workspace's events to any URL`,
        },
        {
          endpoint: "/api/webhookSubscriptions.update",
          action: msg`Change a subscription, including where it posts and which events it carries`,
        },
        {
          endpoint: "/api/webhookSubscriptions.delete",
          action: msg`Delete a subscription`,
        },
        {
          endpoint: "/api/webhookSubscriptions.toggle",
          action: msg`Enable or disable a subscription`,
        },
        {
          endpoint: "/api/webhookSubscriptions.test",
          action: msg`Fire a test delivery at the subscription's URL — write, not read, because it makes the server call out`,
        },
      ],
    },
    caveat: msg`Write decides where the workspace's event stream goes: create and update both accept any URL, and the delivery log that read opens carries the event payloads themselves. Two neighbours on the same routes answer to something else — /api/webhookSubscriptions.regenerateSecret is owner-only whatever this switch says, and /api/webhookSubscriptions.eventTypes is a fixed catalogue that checks nothing at all. Reading a subscription hides its signing secret from everyone but an owner.`,
  },

  webhook_events: {
    scope: msg`The inbound events email providers send back — bounces, complaints, deliveries and replies — as the raw rows the provider posted.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/inboundWebhookEvents.list",
          action: msg`List the raw provider callbacks, including recipient addresses and provider diagnostics`,
        },
      ],
    },
    write: {
      endpoints: [],
      note: msg`This verb gates nothing today. Exactly one inbound webhook event call is permission-checked and it is a read; the rows themselves arrive on the provider callback routes /webhooks/email and /webhooks/email/inbound, which carry no workspace session and so consult no grant. Unlike the two verbs the matrix locks on, this one is still yours to switch — turning it off simply changes nothing today.`,
    },
    caveat: msg`Read is not a summary: these are the provider's own payloads, so the rows carry recipient addresses and provider diagnostics. That is why membership alone is not enough for them.`,
  },

  llm: {
    scope: msg`The AI assistant: chat over the workspace's configured LLM integration, and the server-side tools it may call.`,
    read: {
      endpoints: [],
      note: msg`This verb gates nothing today. /api/llm.chat is the only LLM endpoint and it answers to LLM write, so there is nothing left for read to govern. The switch is granted and locked rather than denied, because storing it as "off" would be permanent: filling in a missing permission only ever adds what a grant lacks, so nothing would widen it if a later release gives this verb something real to gate.`,
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/llm.chat",
          action: msg`Hold a streaming conversation with the workspace's configured LLM provider, spending its credits`,
        },
      ],
    },
    caveat: msg`One endpoint, but it is the whole assistant: /api/llm.chat streams through the workspace's own provider integration, on the workspace's own billing. When a Firecrawl integration is configured it also lets the assistant run the scrape and search tools from the server, so this grant reaches the open web through your credentials.`,
  },

  workspace: {
    scope: msg`The workspace record itself — its settings and its member roster — plus the custom contact field labels.`,
    read: {
      endpoints: [
        {
          endpoint: "/api/workspaces.get",
          action: msg`Read the workspace record and its settings; Workspace write alone also passes, so someone who may only edit can still load what they are editing`,
        },
        {
          endpoint: "/api/workspaces.members",
          action: msg`List the whole team — members, owners, API keys and pending invitations; without it the answer is degraded to your own row rather than refused`,
        },
        {
          endpoint: "/api/tasks.list",
          action: msg`See integration sync tasks; a listing that names no type is narrowed to the types you may read`,
        },
        {
          endpoint: "/api/tasks.get",
          action: msg`Read an integration sync task's state and progress`,
        },
      ],
    },
    write: {
      endpoints: [
        {
          endpoint: "/api/workspaces.setCustomFieldLabels",
          action: msg`Rename the workspace's custom contact field labels`,
        },
        {
          endpoint: "/api/workspaces.get",
          action: msg`Load the workspace record, which read or write both admit`,
        },
        {
          endpoint: "/api/tasks.trigger",
          action: msg`Run an integration sync task immediately`,
        },
        {
          endpoint: "/api/tasks.reset",
          action: msg`Reset an integration sync task so it re-runs from the start`,
        },
        {
          endpoint: "/api/tasks.delete",
          action: msg`Delete an integration sync task`,
        },
      ],
    },
    caveat: msg`Workspace write is far narrower than its name: the only workspace endpoint it gates is /api/workspaces.setCustomFieldLabels. Renaming or deleting the workspace, and creating, updating or deleting an integration, are owner-only and consult no permission at all — granting this does not confer them, and an owner has them without it. /api/workspaces.list is not gated either: it returns the workspaces you belong to whatever this switch says. Two settings endpoints that look like workspace configuration answer elsewhere — /api/workspaces.setBlogSettings to Blog write, and /api/workspaces.setWebAnalyticsSettings to Web Analytics write.`,
  },
};

// Every resource is required on purpose: this annotates the sets the console CONSTRUCTS and
// SENDS, so the compiler rejects them the moment a resource is added to the union, instead of
// leaving that resource silently absent — which every permission gate reads as "denied".
export type UserPermissions = Record<PermissionResource, ResourcePermissions>;

// What the server actually returns: the stored map may hold any subset of the resources, and
// for synthesised invitation rows it may be null.
export type StoredPermissions = Partial<
  Record<PermissionResource, ResourcePermissions>
>;

export function createFullPermissions(): UserPermissions {
  return buildPermissions(true);
}

export function createEmptyPermissions(): UserPermissions {
  return buildPermissions(false);
}

// Applied to any set the console is about to send, so an unenforceable verb never persists as a
// `false` that no future backfill would widen.
export function grantUnenforcedPermissions(
  permissions: UserPermissions,
): UserPermissions {
  const granted = { ...permissions };
  for (const [resource, type] of UNENFORCED_PERMISSIONS) {
    granted[resource] = { ...granted[resource], [type]: true };
  }
  return granted;
}

function buildPermissions(granted: boolean): UserPermissions {
  const permissions = {} as UserPermissions;
  for (const resource of ALL_PERMISSION_RESOURCES) {
    permissions[resource] = { read: granted, write: granted };
  }
  return permissions;
}
