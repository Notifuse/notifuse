# Remove Visibility Feature from Email Editor

## Context

GitHub issue: Notifuse/notifuse#305

The email editor adds a `visibility` attribute (`"all"`, `"email_only"`, `"web_only"`) to `<mj-section>` tags. This is not a valid MJML attribute and causes compilation failures. The feature was an experiment for multi-channel (email/web) section filtering and is no longer needed.

**Important:** The `Channel` field on `CompileTemplateRequest` and `Template.Channel` are **kept** — they serve other purposes (template classification, tracking skip for web, future SMS support). Only the visibility filtering logic is removed.

## Strategy

Strip the `visibility` attribute from all block attributes **before MJML compilation** in the backend converter. This fixes the issue for both new and existing templates (no DB migration needed). Then remove the dead UI and filtering code.

---

## Steps

### Step 1: Backend — Strip `visibility` in MJML attribute formatter

**File:** `pkg/notifuse_mjml/converter.go`

In `formatAttributesWithLiquid()` (line 347), skip the `visibility` key before it reaches MJML output. Add it to `shouldIncludeAttribute` or filter by key name in the loop:

```go
for key, value := range attributes {
    // Skip editor-only attributes that are not valid MJML
    if key == "visibility" {
        continue
    }
    // ... existing logic
}
```

This is the **critical fix** — it prevents `visibility="all"` from appearing in MJML output regardless of what's stored in the DB.

### Step 2: Backend — Remove `FilterBlocksByChannel` and its call

**Files to delete:**
- `pkg/notifuse_mjml/filter.go` (entire file)
- `pkg/notifuse_mjml/filter_test.go` (entire file)

**File to edit:** `pkg/notifuse_mjml/template_compilation.go`

- **Lines 362-366:** Remove the channel filtering block:
  ```go
  // DELETE these lines:
  tree := req.VisualEditorTree
  if req.Channel != "" {
      tree = FilterBlocksByChannel(req.VisualEditorTree, req.Channel)
  }
  // REPLACE with:
  tree := req.VisualEditorTree
  ```

- **Line 242:** Update the `Channel` field comment to remove "filters blocks by visibility":
  ```go
  Channel string `json:"channel,omitempty"` // "email" or "web"
  ```

### Step 3: Frontend — Remove visibility UI from MjSectionBlock

**File:** `console/src/components/email_builder/blocks/MjSectionBlock.tsx`

- **Lines 24-50:** Delete the `hasLiquidTagsInSection()` helper function (only used by the visibility warning).
- **Lines 191-222:** Delete the visibility `<Select>` control and the Liquid tags warning `<Alert>` below it. This is the entire block from `{/* Visibility / Channel Selector */}` through the closing of the `<Alert>`.

### Step 4: Frontend — Clean TypeScript type (if needed)

**File:** `console/src/components/email_builder/types.ts`

`MJSectionAttributes` (line 463) does **not** have a `visibility` field — it's passed as a generic attribute via `Record<string, unknown>` cast. No type change needed.

### Step 5: Clean i18n strings

**Files:** `console/src/i18n/locales/*.po` (all locales)

Run `npm run lingui:extract` after removing the UI code. The unused translation keys for "Visibility", "All Channels", "Email Only", "Web Only", "Control which channels can see this section", "Personalization Not Available for Web", and the warning description will be flagged as obsolete by Lingui and can be removed.

### Step 6: Update OpenAPI spec

**File:** `openapi.json`

Find the `channel` field description (around line 6554) referencing "block visibility" and update to just `"email" or "web"`.

---

## Testing

### Backend

```bash
# Verify filter files are deleted and nothing breaks
make test-unit

# Specifically test MJML compilation still works
make test-pkg
```

After Step 1, any template with `visibility` in its JSON will compile cleanly — the attribute is silently stripped.

### Frontend

```bash
cd console && npm run build
```

Verify the section settings panel no longer shows the Visibility dropdown.

---

## What is NOT removed

- `CompileTemplateRequest.Channel` field — still used for web channel (skip tracking, skip personalization)
- `Template.Channel` — template classification ("email" / "web")
- `Broadcast.ChannelType` — delivery type tracking
- `MessageHistory.Channel` — analytics
- Web channel special-casing in `template_compilation.go` (lines 379-381, 419, 460-467) — legitimate behavior for web templates
