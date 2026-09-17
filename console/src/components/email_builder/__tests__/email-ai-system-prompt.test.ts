import { describe, expect, it } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import {
  EMAIL_AI_SYSTEM_PROMPT,
  EMAIL_CATEGORY_NOTES,
  UNKNOWN_CATEGORY_NOTE,
  buildEmailSystemPrompt,
  categoryNote
} from '../email-ai-system-prompt'
import { serializeEmailTree } from '../email-ai-tools'
import { TOOL_RESULT_PROTOCOL_PROMPT } from '../../ai-assistant/wire'
import type { EmailBlock } from '../types'

/**
 * A marketing email shipped without an unsubscribe link because the prompt never asked
 * for one — it named {{ unsubscribe_url }} in a variable list and nowhere else. These
 * tests hold the instruction in place, and hold the variable list to what the backend
 * actually populates: a link the send cannot fill renders an empty href, which is worse
 * than no link at all.
 */

/** The prompt text under one `## ` heading, up to the next one. */
function section(heading: string): string {
  const start = EMAIL_AI_SYSTEM_PROMPT.indexOf(heading)
  if (start === -1) throw new Error(`the prompt has no "${heading}" section`)
  const next = EMAIL_AI_SYSTEM_PROMPT.indexOf('\n## ', start + heading.length)
  return next === -1
    ? EMAIL_AI_SYSTEM_PROMPT.slice(start)
    : EMAIL_AI_SYSTEM_PROMPT.slice(start, next)
}

const liquid = section('## Liquid Templating')

/** One availability group of the Liquid section, from its label to the next one. */
function group(label: string, nextLabel: string): string {
  const start = liquid.indexOf(label)
  const end = liquid.indexOf(nextLabel)
  if (start === -1) throw new Error(`the Liquid section has no "${label}" group`)
  if (end === -1) throw new Error(`the Liquid section has no "${nextLabel}" group`)
  return liquid.slice(start, end)
}

const alwaysGroup = group('Always available:', 'When a contact is known')
const contactGroup = group('When a contact is known', 'Only when the send has a list')
const listGroup = group('Only when the send has a list', 'Broadcasts only:')
const broadcastGroup = group('Broadcasts only:', 'Transactional API sends:')
const availabilityGroups = [alwaysGroup, contactGroup, listGroup, broadcastGroup]

const makeTree = (): EmailBlock => ({
  id: 'mjml-1',
  type: 'mjml',
  children: [
    {
      id: 'body-1',
      type: 'mj-body',
      attributes: { width: '600px' },
      children: [
        {
          id: 'section-1',
          type: 'mj-section',
          children: [
            {
              id: 'column-1',
              type: 'mj-column',
              attributes: { width: '100%' },
              children: [
                {
                  id: 'text-1',
                  type: 'mj-text',
                  content: '<p>Hello World</p>'
                }
              ]
            }
          ]
        }
      ]
    }
  ]
})

describe('EMAIL_AI_SYSTEM_PROMPT compliance footer', () => {
  const footer = section('## Compliance Footer')

  it('requires a visible unsubscribe link in a marketing email before any category is chosen', () => {
    expect(footer).toMatch(/MUST end with a footer/)
    expect(footer).toContain('{{ unsubscribe_url }}')
    expect(footer).toContain('{{ notification_center_url }}')
  })

  it('shows the model a footer it can reproduce', () => {
    // Parsing it here is the only way a broken escape inside the template literal fails
    // loudly: the model would otherwise be handed a footer it cannot copy.
    const fenced = footer.slice(footer.indexOf('```json') + '```json'.length)
    const block: unknown = JSON.parse(fenced.slice(0, fenced.indexOf('```')))
    const sectionBlock = block as EmailBlock
    expect(sectionBlock.type).toBe('mj-section')
    const column = sectionBlock.children?.[0]
    expect(column?.type).toBe('mj-column')
    const text = column?.children?.[0]
    expect(text?.type).toBe('mj-text')
    expect(text?.content).toContain('{{ unsubscribe_url }}')
    expect(text?.content).toContain('{{ notification_center_url }}')
  })

  it('tells the model not to put an unsubscribe link in a transactional email', () => {
    expect(footer).toMatch(/do NOT add an unsubscribe link/)
  })

  it('repeats the footer rule in the best-practice and common-mistake lists', () => {
    expect(section('## Best Practices')).toContain('compliance footer')
    expect(section('## Common Mistakes to Avoid')).toContain('{{ unsubscribe_url }}')
  })
})

describe('EMAIL_AI_SYSTEM_PROMPT single response contract', () => {
  const oneResponse = section('## One Response Per Request')

  // The assistant runs one round per message. A model that calls one tool and waits for
  // the result silently drops the rest of the job: it set the subject, left the template
  // empty, and only built it when the user asked whether it had finished.
  it('asks for the whole job in one response', () => {
    expect(oneResponse).toMatch(/whole job in ONE response/)
    expect(oneResponse).toContain('setEmailTree AND updateEmailMetadata in the same response')
  })

  it('puts the structure before the metadata so a cut-off response keeps the email', () => {
    expect(oneResponse).toMatch(/setEmailTree BEFORE updateEmailMetadata/)
  })

  // Gemini narrates inside its reasoning, which renders in a collapsed block and never as
  // an answer, so a response of reasoning plus tool calls leaves the thread with no reply.
  it('requires a user-facing reply as ordinary text, not as reasoning', () => {
    expect(oneResponse).toMatch(/ordinary text in that same response/)
    expect(oneResponse).toMatch(/reasoning is never shown as an answer/i)
  })

  // Gemini cannot obey the line above: its contract puts the answer after the tool
  // results, so the hook sends them back and this paragraph tells it what that message is.
  it('explains the continuation message and forbids redoing the work in it', () => {
    expect(oneResponse).toMatch(/sends the results of your tool calls back in one more message/)
    expect(oneResponse).toContain(TOOL_RESULT_PROTOCOL_PROMPT)
    expect(oneResponse).toMatch(/repeating them there is refused/)
  })
})

describe('EMAIL_AI_SYSTEM_PROMPT Liquid variables', () => {
  it('groups the variables by when the backend populates them', () => {
    expect(liquid).toContain('Always available:')
    expect(liquid).toContain('When a contact is known')
    expect(liquid).toContain('Only when the send has a list')
    expect(liquid).toContain('Broadcasts only:')
    expect(liquid).toContain('Transactional API sends:')
  })

  it('puts the list-only variables in the list-only group', () => {
    expect(listGroup).toContain('unsubscribe_url')
    expect(listGroup).toContain('confirm_subscription_url')
    expect(listGroup).toContain('list.name')
    expect(alwaysGroup).not.toContain('unsubscribe_url')
    expect(contactGroup).not.toContain('confirm_subscription_url')
  })

  it('puts notification_center_url with the contact, not with the list', () => {
    // It needs a contact and nothing more, so it is the one preferences link a
    // list-less send can still render.
    expect(contactGroup).toContain('notification_center_url')
    expect(listGroup).not.toContain('notification_center_url')
  })

  it('keeps the header-only urls out of every availability group', () => {
    for (const availabilityGroup of availabilityGroups) {
      expect(availabilityGroup).not.toContain('oneclick_unsubscribe_url')
      expect(availabilityGroup).not.toContain('tracking_opens_url')
    }
    expect(liquid).toMatch(/Never put \{\{ oneclick_unsubscribe_url \}\}/)
  })

  it('points application links at website_url, not the tracking endpoint', () => {
    expect(liquid).toContain('{{ workspace.website_url }}/path')
    expect(liquid).not.toContain('{{ workspace.base_url }}/path')
  })
})

describe('EMAIL_CATEGORY_NOTES', () => {
  const categories = [
    'marketing',
    'transactional',
    'welcome',
    'opt_in',
    'unsubscribe',
    'bounce',
    'blocklist',
    'blog',
    'other'
  ]

  it('gives every category the drawer offers, and the backend extras, its own note', () => {
    for (const category of categories) {
      expect(Object.prototype.hasOwnProperty.call(EMAIL_CATEGORY_NOTES, category)).toBe(true)
    }
    const notes = Object.values(EMAIL_CATEGORY_NOTES)
    expect(new Set(notes).size).toBe(notes.length)
  })

  it('demands the footer for a marketing template and names the list variables', () => {
    expect(EMAIL_CATEGORY_NOTES.marketing).toMatch(/REQUIRED/)
    expect(EMAIL_CATEGORY_NOTES.marketing).toContain('{{ unsubscribe_url }}')
    expect(EMAIL_CATEGORY_NOTES.marketing).toContain('{{ list.name }}')
  })

  it('forbids the unsubscribe link for a transactional template and explains the flat payload', () => {
    expect(EMAIL_CATEGORY_NOTES.transactional).toMatch(/Do NOT add an unsubscribe link/)
    expect(EMAIL_CATEGORY_NOTES.transactional).toContain('{{ order_id }}')
  })

  it('requires a confirmation button for a double opt-in template', () => {
    expect(EMAIL_CATEGORY_NOTES.opt_in).toContain('{{ confirm_subscription_url }}')
    expect(EMAIL_CATEGORY_NOTES.opt_in).toMatch(/button/i)
  })

  it('asks for no unsubscribe link in the operational categories', () => {
    for (const category of ['unsubscribe', 'bounce', 'blocklist']) {
      expect(EMAIL_CATEGORY_NOTES[category]).toMatch(/No unsubscribe (link|footer)/i)
    }
  })

  it('says a blog template is not an email', () => {
    expect(EMAIL_CATEGORY_NOTES.blog).toMatch(/not sent as an email/)
  })
})

describe('categoryNote', () => {
  it('returns the keyed note for a known category', () => {
    expect(categoryNote('marketing')).toBe(EMAIL_CATEGORY_NOTES.marketing)
  })

  it('falls back when no category is set', () => {
    expect(categoryNote(undefined)).toBe(UNKNOWN_CATEGORY_NOTE)
    expect(categoryNote('')).toBe(UNKNOWN_CATEGORY_NOTE)
  })

  it('falls back for a category it does not know, including prototype names', () => {
    expect(categoryNote('newsletter')).toBe(UNKNOWN_CATEGORY_NOTE)
    // Without an own-property guard this would hand the model Object.prototype.constructor.
    expect(categoryNote('constructor')).toBe(UNKNOWN_CATEGORY_NOTE)
  })
})

describe('buildEmailSystemPrompt', () => {
  it('starts with the static prompt', () => {
    expect(buildEmailSystemPrompt({ tree: null }).startsWith(EMAIL_AI_SYSTEM_PROMPT)).toBe(true)
  })

  it('names the selected category and appends its note', () => {
    const built = buildEmailSystemPrompt({ tree: null, category: 'marketing' })
    expect(built).toContain('Category: marketing')
    expect(built).toContain(EMAIL_CATEGORY_NOTES.marketing)
  })

  it('says the category is not set and uses the fallback note', () => {
    const built = buildEmailSystemPrompt({ tree: null })
    expect(built).toContain('Category: not set')
    expect(built).toContain(UNKNOWN_CATEGORY_NOTE)
    for (const note of Object.values(EMAIL_CATEGORY_NOTES)) {
      expect(built).not.toContain(note)
    }
  })

  it('appends the serialized tree, the subject and the preview text', () => {
    const tree = makeTree()
    const built = buildEmailSystemPrompt({
      tree,
      subject: 'Hello',
      previewText: 'Preview',
      category: 'marketing'
    })
    expect(built).toContain('## Current Email Structure')
    expect(built).toContain(serializeEmailTree(tree))
    expect(built).toContain('Current subject: "Hello"')
    expect(built).toContain('Current preview text: "Preview"')
  })

  it('omits the structure, subject and preview when they are absent', () => {
    const built = buildEmailSystemPrompt({ tree: null })
    expect(built).not.toContain('## Current Email Structure')
    expect(built).not.toContain('Current subject:')
    expect(built).not.toContain('Current preview text:')
  })

  it('puts the category block before the current structure', () => {
    // The live tree stays the last thing the model reads.
    const built = buildEmailSystemPrompt({ tree: makeTree(), category: 'marketing' })
    expect(built.indexOf('## Template Category')).toBeLessThan(
      built.indexOf('## Current Email Structure')
    )
  })

  // Translating it would ship the operator's locale to the model and put the MJML
  // vocabulary the tools enforce into a translator's hands.
  it('keeps the model-facing prompt out of the translation catalog', () => {
    const source = readFileSync(resolve(__dirname, '../email-ai-system-prompt.ts'), 'utf8')
    expect(source).not.toMatch(/@lingui/)
    expect(source).not.toMatch(/\bt`/)
    expect(source).not.toMatch(/\b(?:msg|defineMessage|useLingui|Trans)\b/)
  })
})
