// Suggested Anthropic models. Free text is allowed (the API takes any model ID), so a model
// released after this console still works. Model IDs from:
// https://docs.anthropic.com/en/docs/about-claude/models/overview
export const anthropicModels = [
  { value: 'claude-opus-4-6' },
  { value: 'claude-sonnet-4-6' },
  { value: 'claude-haiku-4-5-20251001' }
]

// Suggested Gemini models. Free text is allowed, so any current/future model ID can be entered.
export const geminiModels = [{ value: 'gemini-3.1-pro-preview' }]

// The model a new OpenAI integration starts with, and the one a save without a model falls back to.
export const OPENAI_DEFAULT_MODEL = 'gpt-4.1'
