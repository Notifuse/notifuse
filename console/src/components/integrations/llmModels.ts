// Suggested Anthropic models. Free text is allowed (the API takes any model ID), so a model
// released after this console still works. Model IDs from:
// https://platform.claude.com/docs/en/about-claude/models/overview
export const anthropicModels = [
  { value: 'claude-opus-5-5' },
  { value: 'claude-sonnet-5-5' },
  { value: 'claude-haiku-5-5' }
]
export const ANTHROPIC_DEFAULT_MODEL = 'claude-sonnet-5-5'

// Suggested Gemini models. Free text is allowed, so any current/future model ID can be entered.
// Model IDs from: https://aistudio.google.com/docs/models
export const geminiModels = [
  { value: 'gemini-4-carbon' },
  { value: 'gemini-4-argon' },
  { value: 'gemini-4-hydrogen' }
]
export const GEMINI_DEFAULT_MODEL = 'gemini-4-argon'

// The model a new OpenAI integration starts with, and the one a save without a model falls back to.
// Model IDs from: https://developers.openai.com/api/docs/models
export const OPENAI_DEFAULT_MODEL = 'gpt-6.1-sol'
