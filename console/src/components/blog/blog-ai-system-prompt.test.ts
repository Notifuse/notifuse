import { describe, expect, it } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import { BLOG_AI_SYSTEM_PROMPT } from './blog-ai-system-prompt'
import { BLOG_TOOL_NAMES } from './blog-ai-tools'
import { TOOL_RESULT_PROTOCOL_PROMPT } from '../ai-assistant/wire'

/**
 * The assistant runs one round per message and tool results never reach the model, so a
 * model that calls one tool and waits for the result drops the rest of the job silently.
 * The email assistant showed the failure first: it set the subject, left the template
 * empty, and only finished when the user asked whether it had. This prompt used to invite
 * exactly that, telling the model to use the metadata tool "after" creating the content.
 */

/** The prompt text under one `## ` heading, up to the next one. */
function section(heading: string): string {
  const start = BLOG_AI_SYSTEM_PROMPT.indexOf(heading)
  if (start === -1) throw new Error(`the prompt has no "${heading}" section`)
  const next = BLOG_AI_SYSTEM_PROMPT.indexOf('\n## ', start + heading.length)
  return next === -1
    ? BLOG_AI_SYSTEM_PROMPT.slice(start)
    : BLOG_AI_SYSTEM_PROMPT.slice(start, next)
}

describe('BLOG_AI_SYSTEM_PROMPT single response contract', () => {
  const oneResponse = section('## One Response Per Request')

  it('asks for the whole job in one response, naming both tools', () => {
    expect(oneResponse).toMatch(/whole job in ONE response/)
    expect(oneResponse).toContain(
      `${BLOG_TOOL_NAMES.UPDATE_CONTENT} AND ${BLOG_TOOL_NAMES.UPDATE_METADATA} in the same response`
    )
  })

  it('puts the article before the metadata so a cut-off response keeps the post', () => {
    expect(oneResponse).toMatch(
      new RegExp(`${BLOG_TOOL_NAMES.UPDATE_CONTENT} BEFORE ${BLOG_TOOL_NAMES.UPDATE_METADATA}`)
    )
  })

  // Gemini narrates inside its reasoning, which renders in a collapsed block and never as
  // an answer, so a response of reasoning plus tool calls leaves the thread with no reply.
  it('requires a user-facing reply as ordinary text, not as reasoning', () => {
    expect(oneResponse).toMatch(/ordinary text in that same response/)
    expect(oneResponse).toMatch(/reasoning is never shown as an answer/i)
  })

  it('explains the continuation message and forbids redoing the work in it', () => {
    expect(oneResponse).toMatch(/sends the results of your tool calls back in one more message/)
    expect(oneResponse).toContain(TOOL_RESULT_PROTOCOL_PROMPT)
    expect(oneResponse).toMatch(/repeating them there is refused/)
  })

  it('no longer tells the model to write the metadata after the content', () => {
    const whichTool = section('## IMPORTANT: When to use which tool')
    expect(whichTool).not.toMatch(/After creating content/)
    expect(whichTool).toMatch(/SAME response/)
  })
})

describe('BLOG_AI_SYSTEM_PROMPT', () => {
  // Translating it would ship the operator's locale to the model.
  it('keeps the model-facing prompt out of the translation catalog', () => {
    const source = readFileSync(resolve(__dirname, 'blog-ai-system-prompt.ts'), 'utf8')
    expect(source).not.toMatch(/@lingui/)
    expect(source).not.toMatch(/\bt`/)
    expect(source).not.toMatch(/\b(?:msg|defineMessage|useLingui|Trans)\b/)
  })
})
