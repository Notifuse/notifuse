#!/bin/bash
# Ask Gemini for a second opinion. Called by the gemini subagent (.claude/agents/gemini.md).
#
# Usage: .claude/scripts/gemini.sh "prompt" [image_or_pdf_path ...]
# Needs curl and jq.
#
# API key, from either:
#   - GEMINI_API_KEY in the environment (a local shell, or a cloud environment variable).
#     It is sent as the x-goog-api-key header, written to a private temp file, so it is
#     never part of the URL or of curl's command line;
#   - a cloud environment network secret for generativelanguage.googleapis.com, with the
#     header name x-goog-api-key. The proxy adds the key on the way out, so leave
#     GEMINI_API_KEY unset in that case.
#
# Settings, all optional:
#   GEMINI_MODEL        model name (default gemini-4-argon)
#   GEMINI_MAX_TOKENS   output cap; thinking tokens count against it (default 65536)
#   GEMINI_TEMPERATURE  sampling temperature (default 0.7)
#   GEMINI_TIMEOUT      seconds to wait for the answer (default 540, under the 10-minute
#                       limit Claude Code puts on a single command)
#   GEMINI_USAGE_LOG    file to append one JSON line of token usage per call
#
# Exit codes:
#   0  complete answer on stdout
#   1  usage, setup or API error (details on stderr)
#   3  the answer was cut short (finishReason is not STOP); the partial text is still
#      printed, but it is not a verdict
#   4  no answer received (network failure or timeout)
#
# Every call writes modelVersion, finishReason and token usage to stderr. Thinking models
# return their reasoning as "thought" parts; only the other parts are printed.

set -e

MODEL="${GEMINI_MODEL:-gemini-4-argon}"
API_URL="https://generativelanguage.googleapis.com/v1beta/models/${MODEL}:generateContent"
MAX_TOKENS="${GEMINI_MAX_TOKENS:-65536}"
TEMPERATURE="${GEMINI_TEMPERATURE:-0.7}"
TIMEOUT="${GEMINI_TIMEOUT:-540}"

for tool in curl jq; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "gemini.sh: $tool is not installed" >&2
    exit 1
  fi
done

if [ -z "$1" ]; then
  echo "Usage: $0 \"prompt\" [image_or_pdf_path ...]" >&2
  exit 1
fi

PROMPT="$1"
shift

# All large data lives in files, never in argv, which has a size limit (ARG_MAX).
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# The parts array is kept in a file and read back with --slurpfile, because --argjson
# would have to fit in argv too.
jq -n --arg text "$PROMPT" '[{text: $text}]' > "$tmpdir/parts.json"

i=0
for file in "$@"; do
  if [ ! -f "$file" ]; then
    echo "gemini.sh: file not found: $file" >&2
    exit 1
  fi
  ext=$(echo "$file" | awk -F. '{print tolower($NF)}')
  case "$ext" in
    png)        mime="image/png" ;;
    jpg|jpeg)   mime="image/jpeg" ;;
    webp)       mime="image/webp" ;;
    gif)        mime="image/gif" ;;
    heic|heif)  mime="image/heic" ;;
    pdf)        mime="application/pdf" ;;
    *) echo "gemini.sh: unsupported file type: .$ext (png, jpg, webp, gif, heic or pdf)" >&2; exit 1 ;;
  esac
  i=$((i+1))
  base64 < "$file" | tr -d '\n' > "$tmpdir/data_$i.b64"
  jq --slurpfile cur "$tmpdir/parts.json" \
     --arg mime "$mime" \
     --rawfile data "$tmpdir/data_$i.b64" \
     -n '$cur[0] + [{inline_data: {mime_type: $mime, data: $data}}]' \
     > "$tmpdir/parts.next.json"
  mv "$tmpdir/parts.next.json" "$tmpdir/parts.json"
done

jq --slurpfile parts "$tmpdir/parts.json" \
   --argjson max "$MAX_TOKENS" --argjson temp "$TEMPERATURE" -n '{
  contents: [{parts: $parts[0]}],
  generationConfig: {temperature: $temp, maxOutputTokens: $max}
}' > "$tmpdir/payload.json"

# printf is a shell builtin, so the key never shows up in the process list either.
: > "$tmpdir/headers"
if [ -n "${GEMINI_API_KEY:-}" ]; then
  printf 'x-goog-api-key: %s\n' "$GEMINI_API_KEY" > "$tmpdir/headers"
fi

# A network failure must say so: with set -e, a failed curl used to end the script
# silently, and the caller could not tell a cut answer from no call at all.
set +e
curl -s --max-time "$TIMEOUT" "$API_URL" \
  -H "Content-Type: application/json" \
  -H @"$tmpdir/headers" \
  --data-binary @"$tmpdir/payload.json" > "$tmpdir/response.json"
rc=$?
set -e
if [ "$rc" -ne 0 ]; then
  echo "gemini.sh: curl failed (exit $rc) within ${TIMEOUT}s; no answer received. Request body: $(wc -c < "$tmpdir/payload.json" | tr -d ' ') bytes. Retry, run it in the background, or shorten the prompt." >&2
  exit 4
fi

# A cloud session's proxy answers a blocked host with a non-JSON 403.
if ! jq -e . "$tmpdir/response.json" >/dev/null 2>&1; then
  echo "gemini.sh: the response is not JSON (blocked by a network policy?): $(head -c 300 "$tmpdir/response.json")" >&2
  exit 1
fi

error=$(jq -r '.error.message // empty' "$tmpdir/response.json")
if [ -n "$error" ]; then
  echo "gemini.sh: API error: $error" >&2
  if [ -z "${GEMINI_API_KEY:-}" ]; then
    echo "gemini.sh: GEMINI_API_KEY is not set. In a cloud session, add it as an environment variable, or as a network secret for generativelanguage.googleapis.com with the header x-goog-api-key." >&2
  fi
  exit 1
fi

finish=$(jq -r '.candidates[0].finishReason // "ABSENT"' "$tmpdir/response.json")
version=$(jq -r '.modelVersion // "unknown"' "$tmpdir/response.json")
usage=$(jq -c '.usageMetadata // {}' "$tmpdir/response.json")
echo "gemini.sh: modelVersion=${version} finishReason=${finish} usage=${usage}" >&2

if [ -n "${GEMINI_USAGE_LOG:-}" ]; then
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  jq --arg ts "$ts" --arg model "$MODEL" \
    -c '{ts: $ts, model: $model, modelVersion: (.modelVersion // null), finishReason: (.candidates[0].finishReason // null), usage: (.usageMetadata // null)}' \
    "$tmpdir/response.json" >> "$GEMINI_USAGE_LOG"
fi

# Every part that is not a "thought", concatenated: on thinking models parts[0] may be
# reasoning rather than the answer.
jq -r '[.candidates[0].content.parts[]? | select(.thought != true) | .text // empty] | join("") | if . == "" then "No response" else . end' "$tmpdir/response.json"

if [ "$finish" != "STOP" ]; then
  echo "gemini.sh: finishReason=${finish}: the answer above is INCOMPLETE or empty. Retry with a larger GEMINI_MAX_TOKENS or a shorter prompt, and don't report this text as a verdict." >&2
  exit 3
fi
