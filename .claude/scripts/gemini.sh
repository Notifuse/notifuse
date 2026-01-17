#!/bin/bash
# Gemini API script for Claude Code agents
# Usage: ./gemini.sh "Your prompt here"
# Requires: GEMINI_API_KEY environment variable

set -e

MODEL="gemini-3-pro-preview"
API_URL="https://generativelanguage.googleapis.com/v1beta/models/${MODEL}:generateContent"

if [ -z "$GEMINI_API_KEY" ]; then
  echo "Error: GEMINI_API_KEY environment variable is not set" >&2
  exit 1
fi

if [ -z "$1" ]; then
  echo "Usage: $0 \"Your prompt here\"" >&2
  exit 1
fi

PROMPT="$1"

response=$(curl -s "${API_URL}?key=${GEMINI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"contents\": [{
      \"parts\": [{
        \"text\": $(echo "$PROMPT" | jq -Rs .)
      }]
    }],
    \"generationConfig\": {
      \"temperature\": 0.7,
      \"maxOutputTokens\": 8192
    }
  }")

# Check for errors
error=$(echo "$response" | jq -r '.error.message // empty')
if [ -n "$error" ]; then
  echo "API Error: $error" >&2
  exit 1
fi

# Extract and print the response text
echo "$response" | jq -r '.candidates[0].content.parts[0].text // "No response"'
