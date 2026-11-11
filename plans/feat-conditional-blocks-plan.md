# Plan — show or hide a block based on a condition on template variables (issue #395)

## Need

Issue #395: in the visual email editor, hide a whole block (an image, a section) unless a
condition on the template data holds, typically a field sent to the transactional API
(`order.has_image`). Today the only way is a pair of `mj-liquid` blocks (`{% if %}` before,
`{% endif %}` after), which is fragile and invisible on the canvas.

Spec (validated by Pierre, with mock-ups): Claude Doc "Spec : afficher un bloc selon une
condition sur les variables (issue 395)", mock-ups in the project files under
`maquettes/issue-395/`. Plan review: `reviews/issue-395-plan-review-fable.md` in the project
files (independent review, effort xhigh); every finding below marked (R) comes from it and
was checked against the code.

## Decisions (taken by Pierre)

- Conditions test **variables only** (no segment or list membership, no free Liquid
  expression, no nested groups): one list of rules joined by "all" or "any".
- The condition is built in a **modal**; the editor's right sidebar only shows its summary,
  with a trash icon (`type=link` button, Popconfirm) and a "Modifier" button (no icon) on the
  title line. The modal reuses the **design** (not the code) of the segment condition editor.
- **No condition on columns or groups**: the editor gives them explicit widths, hiding one
  would leave a hole. Not on `mj-liquid` nor on anything inside `mj-head` either.
- A variable missing from the test data is a **warning only**, it never blocks "Appliquer".
- The compared value is **free text** (a string or a number typed by the user), never another
  variable.

No SQL schema change, no migration, no environment variable: the condition lives in the
block tree, already stored as JSON in `templates.email` / `templates.translations`, and for
saved blocks in the workspace settings JSON (`WorkspaceSettings.TemplateBlocks`).

## Engine: simplified Liquid (decided by Pierre)

As the spec says, a condition is **translated into Liquid and evaluated by liquidgo**, and the
same Liquid wraps the block in the MJML export: sending, export and a template moved to code
mode through Export/Import MJML (`template_service.go:420`) behave the same by construction.

The translation is restricted so that liquidgo's limits never bite (all reproduced with
throwaway tests on `dev`, and confirmed by the first review):

| liquidgo limit | How the translation avoids it |
| --- | --- |
| no parentheses, `and`/`or` evaluated right to left (`a == true and b == true or c == true` with a=b=false, c=true is false) | never mix `and` and `or` in one expression: "all" is nested `{% if %}`, "any" sets a flag with `assign`; inside a rule, only `or` or only `and` |
| `v > 100` with text renders the text `Liquid error: comparison of string with int failed`, `err == nil` | order operators run behind a numeric guard (below) |
| `v == 100` false when `v` is a JSON number (float64) | fixed in liquidgo first (separate PR, decided by Pierre: `plans/liquidgo-fix-numeric-equality-plan.md` in the project files); Notifuse bumps liquidgo before this feature |
| `false == blank`, `{} == blank` are false | `is_blank` is written `v == blank or v == false or v == empty` (Pierre: `blank` itself stays unchanged in liquidgo) |
| `&`, `<`, quotes, newlines in a value break the MJML (gomjml parses the export) or Liquid's string syntax | non-trivial values are passed through `assign … | url_decode` |

Safety net: when evaluating, the server renders `<open>1<close>`; any output other than `1`
or empty (for example a "Liquid error" text) counts as **false**. `pkg/notifuse_mjml` has no
logger, so this is not logged; instead the shared fixture covers every operator against every
value type, so such an output is a failing test, not a production event.

The console still needs a small TypeScript evaluator for the live results in the modal and the
sidebar (liquidjs disagrees with liquidgo on `blank`, coercions and comparisons). It implements
the semantics below and is held to the same fixture, whose expected results the Go test checks
**by rendering the generated Liquid with the real liquidgo**: the fixture is the contract, and
liquidgo is its reference.

## Semantics (what the generated Liquid does; mirrored by the fixture file)

Second review: `reviews/issue-395-plan-review-v2-fable.md`; its findings are marked (R2).

**Lookup** is liquidgo's (`liquid/variable_lookup.go`, R2), and the TS evaluator resolves each
segment in this order: object own key; list index when the segment is all digits (`Atoi`, so
`items.0.sku` and `items.007` index the list); then the commands `size` (length of a list;
on an object, nil unless it has a `size` key; on a string, UTF-8 bytes until the liquidgo audit
fix lands, then characters — the fixture follows the pinned liquidgo), `first`, `last`,
`empty`; `to_liquid` returns the value
itself. Anything else is absent (`nil`): `items.length`, `name.length`, `name.0`,
`order.__proto__`, `order.constructor`. Each of these is a fixture case.

Whitespace in `blank` is liquidgo's `strings.TrimSpace` (Unicode white space); the TS evaluator
uses the same explicit set (Go's `unicode.IsSpace`: U+00A0 and U+0085 are blank, U+200B and
U+FEFF are not), not `String.prototype.trim` (R).

| Operator | Generated Liquid (`v` the variable) | True when |
| --- | --- | --- |
| `is_true` | `v == true or v == "true"` | boolean `true` or string `"true"` |
| `is_blank` | `v == blank or v == false or v == empty` | absent, `null`, `false`, a blank string, an empty list or object |
| `is_present` | `v != blank and v != empty` | anything else, including `false` and `0` |
| `is_absent` | `v == blank or v == empty` | absent, `null`, a blank string, an empty list or object |
| `eq` text | `v == "x"` | string equal, exactly (no trim, case-sensitive; `"007"` is not `"7"`) |
| `eq` number literal value | `v == "100" or v == 100` | the string `"100"`, or a number equal to 100 (`100`, `100.0`) |
| `eq` value `true`/`false` | `v == "true" or v == true` | the string or the boolean |
| `neq` | same as `eq` with `!=` and `and` | not `eq` (true when absent) |
| `contains` | `v contains "x"` (number value: `v contains "1" or v contains 1`) | substring of a string; a list element equal to the value (after the liquidgo fix); key of an object |
| `not_contains` | `{% unless <contains> %}` | not `contains` (true when absent) |
| `gt` `gte` `lt` `lte` | guard, then `n > 100` … (`lt`/`lte` written `100 > n`, `100 >= n`, so no `<` in the MJML) | `v` is a number or a numeric string, and the comparison holds; false for text, absent, booleans, lists, objects |

**Numeric guard** (R2: the first version rejected `"19.90"`, `"100.0"`, `"+5"`, typical
transactional prices):
`{% assign n = v | plus: 0 %}{% assign s = n | append: "" %}{% assign vs = v | append: "" %}{% if n != 0 or s == vs %}`.
Accepts numbers, `"0"`, `"19.90"`, `"100.0"`, `" 150 "`, `"007"`, `"+5"`; rejects text,
`"1e2"`, booleans, lists, objects, absent; also rejects `"0.0"`, `"0.00"`, `"-0"` and the JSON
number `-0` (documented). The TS port reproduces liquidgo's `ToNumber` (Go `TrimSpace`, then
`^-?\d+\.\d+$` → float, else `Atoi`, int64 overflow → not a number, so
`"12345678901234567890"` is rejected) and `FormatFloat('f', -1)` (`"0.0000001"` numeric, where
JS prints `1e-7`) (R2). Fixture cases for each.

`contains` with a list: liquidgo today matches a **substring of an element**; the liquidgo PR
switches it to element equality like Shopify Liquid (decided by Pierre). That is also what
makes the TS mirror exact: today's behaviour stringifies objects in Go map order (R2).

**Constructions** (R2: constant nesting depth, liquidgo refuses more than 100 nested blocks,
`if.go:129`): both `all` and `any` use a flag.
`all`: `{% assign __dc = true %}` then per rule `{% unless rule %}{% assign __dc = false %}{% endunless %}`
(`{% if %}` for `not_contains`), then `{% if __dc %}`.
`any`: `{% assign __dc = false %}` then per rule `{% if rule %}{% assign __dc = true %}{% endif %}`,
then `{% if __dc %}`. `open` and `close` are each one line with no whitespace between tags, so
`Evaluate` renders exactly `1` or empty (R2: tags separated by newlines render `"\n\n\n1\n"`).
In `Evaluate` the names are constant (`__dc`, `__dc_n`, `__dc_s`, `__dc_vs`, `__dc_v<k>`), so
liquidgo's never-evicted parse caches (`variable_lookup.go:96,139`) do not grow per block (R2);
the export suffixes them with the sanitised block id (truncated to 16 characters; a collision
is harmless since each flag is assigned right before use).

**Value literals** (R2 blocker: liquidgo splits a condition on `and`/`or` with
`strings.Fields`, ignoring quotes, `liquid/tags/if.go:297-300`, so `v == "black and white"` is
false and `w == "x or 1"` is true when `w` is absent): only a value matching
`^[A-Za-z0-9_.:/@-]+$` (no space) is written inline; any other value goes through
`{% assign __dc_v<k> = "<url.QueryEscape>" | url_decode %}` (QueryEscape, not PathEscape, which
leaves `+`, R2). Fixture values: `a and b`, `a or b`, `a  b`, ` x`, `x `, `and`,
`x contains y`, `R&D <x>`, a newline, `+`, `%41`. The quote-blind split is also a liquidgo bug
for hand-written templates: added to the liquidgo audit list.

A number value is written as typed once normalised by `strconv.ParseFloat` +
`FormatFloat('f', -1)` (`1e2` → `100`, `+5` → `5`, `.5` → `0.5`); it must be finite (`1e400`
is refused, R2).

**Same by construction, with two caveats** (R2): the exported wrapper sees variables assigned
by `mj-liquid` blocks and `for` loop variables, which `Evaluate` does not; and in liquidgo a
data key wins over an `assign` of the same name, so a data key named `__dc…` would break the
export (not the send). Both go in the docs.

Visible differences against the spec's table, to tell Pierre: `"1e2"`, `"0.0"`, `"-0"` are not
numbers for the order operators; `is_present` is false for an empty list or object, and
`is_blank`/`is_present` overlap only on `false`.

**Totality.** Generation never fails on a stored condition that would not pass validation (the
compile endpoint takes unvalidated trees, `template_compilation.go:651-674`, R): unknown
`match` → `all`; a rule that cannot be generated (variable not matching the validation regex
— R2: `a b`, `and`, `%}` would be a parse error and `x | y` a truthiness test — unknown
operator, missing value, non-finite or non-number value on an order operator) → that rule is
false (`{% if false %}`); zero rules → shown. `Evaluate` accepts nil data (R2).

**Validation** (on save):

- `display_condition` only on `mj-wrapper`, `mj-section`, `mj-text`, `mj-image`, `mj-button`,
  `mj-divider`, `mj-spacer`, `mj-social`, `mj-raw`, **and only under `mj-body`** (the same types
  inside `mj-head` — `mj-raw`, the defaults under `mj-attributes`, `model.go:718-735` — are
  refused, R). Anywhere else: refused, naming the block id and type.
- `match` is `all` or `any`; 1 to 20 rules.
- `variable` matches `^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)*$`, at most 200 characters, its
  first segment is not a Liquid keyword (`true`, `false`, `nil`, `null`, `empty`, `blank`, `and`,
  `or`, `contains`) and does not start with `__dc` (reserved for the generated variables) (R).
- `operator` is one of the operators of the table.
- operators without a value: `value` absent; with a value: present, non-blank, at most 200
  Unicode code points (R2: 20 rules × 500 four-byte code points generate 121 KB, over
  `DefaultMaxTemplateSize`, `liquid_secure.go:15,75`, so every send would fail; 200 keeps the
  worst case under the cap, pinned by a test), the console trimming leading/trailing spaces on
  Confirm (Go `utf8.RuneCountInString`, TS `[...s].length`) (R), containing none of
  `{{`, `}}`, `{%`, `%}`. Order operators: the value matches
  `^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$` and is finite (R2).
- The validation cases live in the shared fixture too (`"valid": true|false`), so the modal's
  "Confirm" and the server agree (R).

## Implementation

### `pkg/notifuse_mjml/model.go`

- `BaseBlock` and `EmailBlockJSON` gain
  `DisplayCondition *DisplayCondition \`json:"display_condition,omitempty"\``.
  `UnmarshalEmailBlock` copies it into the `BaseBlock` (today it rebuilds the block from `ID`,
  `Type`, `Children`, `Attributes`, `Content` only and drops it — confirmed by the review's
  probe). `MarshalJSON` needs no change (typed blocks embed `*BaseBlock`).
- `EmailBlock` interface: add `GetDisplayCondition() *DisplayCondition` (nil-safe on
  `*BaseBlock`). The one other implementer, `dummyEmptyTypeBlock` in
  `internal/domain/template_block_test.go:30`, gets the method.

Tests — `pkg/notifuse_mjml/model_test.go`:
- JSON round trip (`UnmarshalEmailBlock` → `json.Marshal` → `UnmarshalEmailBlock`) keeps a
  condition on a nested `mj-image`, with `match`, rules and values intact.
- A block without condition marshals without a `display_condition` key.

### `pkg/notifuse_mjml/display_condition.go` (new)

```go
type DisplayCondition struct {
    Match string                 `json:"match"` // "all" | "any"
    Rules []DisplayConditionRule `json:"rules"`
}
type DisplayConditionRule struct {
    Variable string  `json:"variable"`
    Operator string  `json:"operator"`
    Value    *string `json:"value,omitempty"`
}
func (c *DisplayCondition) Validate() error
// ToLiquid generates the guarded Liquid (see "Semantics"); total, never fails.
func (c *DisplayCondition) ToLiquid(blockID string) (open, close string)
// Evaluate renders open+"1"+close with a pooled SecureLiquidEngine (liquidEnginePool,
// converter.go:20) against data; true only when the output is exactly "1".
// nil condition → true, without rendering.
func (c *DisplayCondition) Evaluate(blockID string, data map[string]interface{}) (bool, error)
func ValidateDisplayConditions(tree EmailBlock) error // walks the tree, tracks "inside mj-head"
func ValidateDisplayConditionsInBlock(block EmailBlock) error // saved blocks: no mj-head context
```

`Evaluate` returns an error only when the engine itself fails (timeout, render length), which
the walk reports like any block's Liquid error today. An unexpected output (not `1`, not empty)
is `false, nil`, as decided. Engines that errored are not returned to the pool, as in
`processLiquidContent` (`converter.go:333-340`). Cost: one small parse and render per
conditional block per recipient, negligible next to the MJML render; no cache unless a
benchmark says otherwise (`BenchmarkDisplayConditionEvaluate` in the test file gives the
number for the PR).

Tests — `pkg/notifuse_mjml/display_condition_test.go`:
- Loads `testdata/display_conditions.json`; asserts the file holds evaluation and validation
  cases, that every operator named in a literal list in the test appears with both a true and a
  false expected result (no count stated, R), then for each evaluation case asserts
  `Evaluate` (that is, real liquidgo) gives `expected`, and for each validation case `Validate`.
- No output other than `1` or empty for any fixture case (the safety net never fires on
  supported input). Errors inside an "any" rule are swallowed by liquidgo (blank `if`,
  `if.go:217`), so each rule of each fixture case is also rendered alone, outside the flag
  construction, and must render without error text (R2).
- Size cap: 20 rules with 200-code-point four-byte values on order operators evaluate without
  "template size exceeds maximum"; 20 `all` rules on a wrapper, a section and a text, exported
  and compiled in code mode, stay under the nesting limit (R2).
- `ValidateDisplayConditions`: refused on `mj-column`, `mj-group`, `mj-liquid`, on `mj-raw`
  under `mj-head`, on `mj-text` under `mj-attributes`, with the block id in the message;
  accepted on each eligible type under `mj-body`, and nested deep in a wrapper.
- `ToLiquid`: golden strings for one rule of each operator, `all` and `any`, an encoded value,
  a normalised number; and the totality cases.

### `pkg/notifuse_mjml/testdata/display_conditions.json` (new)

`{ "evaluate": [{ "name", "data", "condition", "expected" }], "validate": [{ "name",
"condition", "valid" }] }`. Evaluation cases, at least: each operator with the variable
absent, `null`, `true`, `false`, `"true"`, `"True"`, `0`, `""`, `"  "`, `" "`, `[]`, `{}`,
a number, numeric strings (`"150"`, `" 150 "`, `"1e2"`, `"100.5"`), a non-numeric string;
`eq "100"` against `100`, `100.0`, `"100"`, `"100.0"`; `"007"` vs `"7"`; `eq "true"` against
`true` and `"True"`; `contains` against a string, a list of strings (including `["vip_gold"]`
contains `"vip"`: false after the liquidgo fix), a list of numbers, an object (key);
case sensitivity; `gt "100"` against `"abc"`, `150`, `"150"`, absent, `true`, a list;
values with `&`, `<`, both quotes, a newline, `+`, `%`; nested paths (`order.customer.vip`),
`order.items.size`, `items.first`, a path through a string, `order.constructor`,
`order.__proto__`; `all` and `any` with mixed results; the totality cases; `contact.*` shaped
like `BuildTemplateData` output. Validation cases: each rule of the validation list, one failing
case each (bad match, 0 and 21 rules, `order..x`, `1abc`, `a-b`, `true.x`, `and`, `dc_x`,
201-character path, unknown operator, value on `is_true`, missing and blank value on `eq`, 500
and 501 code points of a 4-byte emoji, `{{` in value, non-numeric value on `gt`) and one valid
case per operator.

### `pkg/notifuse_mjml/converter.go`

In each of the three walks, the parent's child loop skips a child whose condition is false
(R: skipping in the loop, rather than returning `""`, avoids touching the existing
empty-string returns of `mj-liquid` with empty content and of empty-type blocks,
`converter.go:64-66, 162-165, 241-244`):

- `convertBlockToMJMLWithErrorAndParsedData` (data path: sends, previews with test data,
  `CompiledPreview`): skip when `Evaluate(child.GetID(), parsedData)` is false; return its error if any. This walk
  already skips nil children (`converter.go:122`); add the typed-nil and empty-type guard the
  other two walks have (R2).
- `convertBlockToMJMLWithParsedData` (no-data path: preview without test data): evaluate
  against an empty map, same skip (this walk has no error return: an engine failure keeps
  the block, as this walk already ignores Liquid errors, `converter.go:183-186`). So empty test data hides `is_present` blocks, as the modal
  says.
- `convertBlockToMJMLRaw` (`preserve_liquid`, the MJML export): no skip; wrap the child's output
  in `ToLiquid`'s open/close lines at the child's indent.
- The root `mjml` block cannot carry a condition (validation), so the root needs no check.

A hidden block is skipped before its content is touched, so a Liquid error in a hidden block
no longer fails the send.

Tests — `pkg/notifuse_mjml/converter_test.go` (new `TestDisplayCondition*` functions; golden
strings use blocks with at most one attribute, since attribute order follows map order,
`converter.go:636`, R):
- First, a condition-free golden test of a small tree on all three walks, written and passing
  **before** the change, pinning that trees without conditions are unchanged (no such exact
  test exists today, R).
- Data path: an `mj-image` with `is_true` on `order.has_image` present with
  `{"order":{"has_image":true}}`, absent with `false` and with `{}`; a hidden `mj-section`'s
  children absent; a hidden `mj-text` with unclosed `{% if %}` gives no error while the same
  block shown gives the existing error; `any` with one true rule shows, `all` with one false
  rule hides; every child of a column hidden leaves `<mj-column>` with no child and the MJML
  still renders; same for every child of an `mj-wrapper` and of `mj-body` (R2); no blank line where a block was skipped.
- No-data path: `is_present` block skipped, `is_absent` block kept.
- Raw path: golden output with the wrapper around the block.

### `pkg/notifuse_mjml/template_compilation.go`

No logic change: `CompileTemplate` calls the three walks. Tests —
`template_compilation_test.go`:
- `CompileTemplate` with `TemplateData` hides the block in the HTML (`assert.NotContains` on the
  image `src`), shows it when true; same with nested `MapOfAny` and Go `int` values in the data
  (R).
- `PreserveLiquid: true` returns `Success: true` and MJML holding the wrapper, including with a
  value `R&D <x> "a" 'b'` and a newline (R: the export still goes through gomjml,
  `template_compilation.go:930-946`, and the console treats an error as a failed export).
- A hidden block between an `mj-liquid` `{% if %}` pair still compiles.
- Code-mode round trip: for **every** fixture evaluation case (R2), the exported MJML, compiled as
  `MjmlSource` with the case's data, shows or hides the block as `expected` (R: the API
  tells users to move a template to code mode through Export/Import MJML,
  `template_service.go:420`, so the export is evaluated by liquidgo for real).

### MJML export

The raw walk wraps the block in exactly the Liquid `Evaluate` renders (see "Semantics"), so the
export is the send semantics by construction; the code-mode round trip test above pins it.

Importing an exported file back into the visual editor does not restore conditions (the
importer drops text between elements, confirmed). Test —
`console/src/components/mjml-converter/__tests__/mjml-to-json-browser.test.ts`: a file with
the export's lines around a section imports without error and keeps the section, and the
importer shows a warning when it meets `__dc` wrappers ("conditions are not imported, the
blocks will show to everyone") (R2).

### `internal/domain`

- `template.go`, `EmailTemplate.Validate`, visual branch: call
  `notifuse_mjml.ValidateDisplayConditions(e.VisualEditorTree)` before computing
  `CompiledPreview`, returning `invalid email template: <reason>`. Translations go through the
  same `Validate` (`template.go:180`), so their trees are covered.
- `template_block.go`, `CreateTemplateBlockRequest.Validate` / `UpdateTemplateBlockRequest.Validate`
  and `workspace.go`, `WorkspaceSettings.Validate` (saved blocks are also written by
  `workspaces.update`, `workspace_service.go:546-561`, R): call
  `ValidateDisplayConditionsInBlock` on each block.
- Stored templates that predate the feature have no key: unchanged. A stored invalid condition
  (not possible through the API once this ships) still renders thanks to `Evaluate`'s totality.

Tests:
- `template_test.go`: valid condition accepted and kept through `Template` JSON round trip and
  `Scan`/`Value`; invalid operator refused; condition on `mj-column` refused; on `mj-raw` under
  `mj-head` refused; a translation's own condition kept and an invalid one in a translation
  refused; `CompiledPreview` computed with test data omits the hidden block.
- `template_block_test.go`: create/update request with a valid and an invalid condition; a
  `TemplateBlock` JSON round trip (`TemplateBlock.UnmarshalJSON`, `template_block.go:73`) keeps
  the condition (R: that is the saved-block path, not `UnmarshalEmailBlocks`); the dummy block
  gets `GetDisplayCondition`.
- `workspace_test.go`: `WorkspaceSettings.Validate` refuses a saved block with an invalid
  condition, and its JSON round trip keeps a valid one.

### `internal/service/template_service.go`

No change (create, update and compile go through the domain). Test —
`template_service_test.go`: `CreateTemplate` with an invalid condition returns a validation
error and never calls the repository; `CompileTemplate` with test data hides the block.

Paths checked (review confirmed): every send (broadcasts, automations, transactional, list
emails, test emails, demo) goes through `notifuse_mjml.CompileTemplate`; `CompiledPreview` is
never sent; `grep 'BaseBlock{'` finds only `UnmarshalEmailBlock` and the examples; the console
deep-copies with `JSON.parse(JSON.stringify(...))`.

### Integration tests — `tests/integration/`

New `display_condition_test.go`, real SMTP to Mailpit, following `tracking_consent_test.go`:
- Transactional: a visual template whose `mj-image` has `is_true` on `order.has_image`, created
  through `templates.create`; two `transactional.send` with `data.order.has_image` true and
  false; the delivered HTML contains the image `src` only the first time. An invalid condition
  is refused by `templates.create` with 400.
- Broadcast: a section with `eq` on `contact.custom_string_1 = "vip"`, two contacts in the list
  (one `vip`), broadcast sent through the API, following `broadcast_data_feed_email_test.go`;
  only the `vip` contact's email has the section text.
- `template_roundtrip_test.go`, `TestTemplateRoundtripSerialization`: a template with
  conditions, including in a translation, survives create → get → update → get unchanged.

### Console

`email_builder/types.ts`: `DisplayCondition`, `DisplayConditionRule`,
`DisplayConditionOperator`; `display_condition?: DisplayCondition` on `BaseBlock`;
`CONDITION_ELIGIBLE_TYPES`.

`email_builder/displayCondition.ts` (new): `evaluateDisplayCondition`, `resolveVariable`
(own-property lookup, Liquid's `size`/`first`/`last`), `trimGoSpace` (Go's `unicode.IsSpace` set), `validateCondition`, `isConditionEligible(tree,
block)` (type list and not `EmailBlockClass.isChildOf(tree, id, 'mj-head')`, R),
`collectVariableSuggestions(testData)`, `countHiddenBlocks(tree, data)`.
Test — `displayCondition.test.ts`: reads `../../../../pkg/notifuse_mjml/testdata/display_conditions.json`
with `readFileSync` (precedent: `__tests__/email-ai-system-prompt.test.ts:308`), asserts it is
non-empty and that the same literal operator list appears with both outcomes, then every
evaluation and validation case; eligibility (defaults block under `mj-attributes` not eligible);
suggestions from nested test data; `countHiddenBlocks` counts a hidden section once.

`email_builder/ui/DisplayConditionSummary.tsx` (new): the sidebar card per the mock-ups (title
without icon; "Always shown" + "Add a condition" when none; trash `Button type="link"` in a
`Popconfirm`, then "Edit", top right; rules read like segment leaves; result with the test
data). Test — `DisplayConditionSummary.test.tsx`: without and with condition; confirming the
Popconfirm calls back with `undefined`; Edit opens the modal; result follows the test data.

`email_builder/ui/DisplayConditionModal.tsx` (new): the segment-editor look, one level, "+
Condition" adding an editing row (variable `AutoComplete` with suggestions and sample values,
operator `Select`, value `Input` hidden for no-value operators, ✕ and "Confirm"), per-rule and
overall result with the test data, missing-variable warning, "Cancel" / "Apply" (disabled while
a row is being edited or there is no rule). Test — `DisplayConditionModal.test.tsx`: add a rule
and apply (callback receives the JSON); switching to `is_true` drops the value; Apply disabled
while editing; delete a rule; Cancel changes nothing; missing variable warns without disabling
Apply; `gt` with a non-number cannot be confirmed.

`panels/SettingsPanel.tsx`: the summary at the top for eligible blocks only, using `testData`
(already a prop, unused today); `handleDisplayConditionChange` deep-copies the block, sets or
deletes `display_condition`, calls `onUpdateBlock` (undo/redo through the existing history).
`EmailBuilder.tsx:947`: key the panel by block id instead of block type, so the summary/modal
state never carries over to another block of the same type (R). Test — new
`SettingsPanel.test.tsx`: summary present for `mj-image`, absent for `mj-column`, `mj-liquid`
and a defaults `mj-text` under `mj-attributes`; applying calls `onUpdateBlock` with the
condition and unchanged attributes; selecting another `mj-image` shows that block's condition.

`panels/TreePanel.tsx`: an icon with a tooltip ("Conditional") in `TreeNodeTitle` when the
block has a condition. Canvas: a `data-display-condition` attribute next to the existing
`data-block-id` on each eligible block's root, and a CSS badge in `editor.css`; the block stays
visible in edit mode. Test — extend `blocks/canvasRendering.test.tsx`.

`panels/Preview.tsx`: a line "N blocks hidden with this test data" when `countHiddenBlocks > 0`,
computed on the compile response's echoed `test_data` (it includes the injected `workspace`
object, `template_service.go:592-597`, R), falling back to the typed test data. Test — extend
`Preview.test.tsx`.

AI assistant (R): today `set_email_tree` replaces the whole tree
(`EmailAIAssistant.tsx:141-168`) and the model never sees conditions, so any rebuild drops them.
- `email-ai-system-prompt.ts`: document `display_condition` (shape, operators, eligible blocks)
  and the rule "keep every existing `display_condition` when you rebuild the tree".
- `email-ai-tools.ts`: `serializeEmailTree` prints the condition as JSON on the block's line.
- `set_email_tree` handler: validate conditions with `validateCondition`/`isConditionEligible`
  and refuse the tree with the error text, like `validateStructure` errors.
- Extract the inline `onUpdateBlock` merge of `CreateTemplateDrawer.tsx:1405-1421` into a helper
  (`EmailBlockClass.mergeBlockUpdate`) and test that it keeps `display_condition`.
Tests: `__tests__/email-ai-tools.test.ts` (serialisation), `__tests__/email-ai-system-prompt.test.ts`
(prompt mentions the field), `__tests__/EmailAIAssistant.handlers.test.tsx` (invalid condition
refused), `EmailBlockClass` merge helper test.

Translations (R): a translation is its own tree, cloned when the language is enabled
(`TemplateTranslationsTab.tsx:92-99`); a condition added later to the default language is not
copied. Documented in the docs page; no UI change in this PR.

i18n: msgids in English (`sourceLocale: "en"`, `lingui.config.ts:5`), French in `fr.po`, every
locale filled (en, fr, es, de, ca, pt-BR, ja, it), `npm run lingui:extract`;
`catalogues.test.ts` checks them; catalogs are compiled by `npm run build` (R).

### CI

`.github/workflows/console.yml` (push and pull_request path lists) and the Go workflow if it
filters paths: add `pkg/notifuse_mjml/testdata/**`, so a change to the fixture runs both suites
(precedent: `web-analytics-sdk.yml:14-17`) (R).

### OpenAPI, docs, VERSION, CHANGELOG

- `openapi/components/schemas/template.yaml`: the `visual_editor_tree` description mentions
  `display_condition` on blocks with a short example; `make openapi-bundle`, `make openapi-lint`.
- `Notifuse/docs`, branch `docs/conditional-blocks` from `dev`, own PR to `dev`: a "Conditional
  blocks" section in `features/templates.mdx` (operators, absent variables, eligible blocks,
  conditions not seeing variables assigned in `mj-liquid`, translations having their own
  conditions, the exported Liquid), a line in `features/transactional-api.mdx`, `openapi.json`
  mirrored.
- `config/config.go`: `VERSION` 43.0 → 43.1 (minor: no schema change; skip if the liquidgo bump
  PR already did it, R2), then `make sdk-build`
  on Node 24 and commit `web_analytics_sdk/dist/*` and `package.json` (VERSION is injected into
  the SDK bundle and CI diffs it, `Makefile:68-75`) (R).
- `CHANGELOG.md`, `## [43.1]`, one bullet:
  - **Feature**: In the visual email editor, a block can now be shown only when a condition on
    the template data holds, for example only when `order.has_image` is true in the data sent
    to the transactional API, or only to contacts whose `custom_string_1` is "vip". Conditions
    apply to wrappers, sections and content blocks, are set in the block settings, and the
    preview hides blocks according to the test data.

## Risks

- Rollback to 43.0: the older version shows conditional blocks to everyone, and drops the key
  on the next save of the template (review probe). Rare; not worth a CHANGELOG line unless
  Pierre wants one.
- `EmailBlock` interface change: out-of-tree implementers break at compile time; only one test
  dummy in this repo.
- Trees without conditions render byte for byte as before: pinned by the golden test written
  before the change.
- Console evaluator vs liquidgo: drift is caught by the shared fixture, checked against real
  liquidgo in Go and against the TS evaluator in the console, and both CI workflows run on a
  fixture change.
- A future liquidgo upgrade that changes a semantic used by the translation fails the fixture
  test in Notifuse before it ships.
- Dependency: this feature needs the liquidgo equality fix and its bump in Notifuse first
  (`plans/liquidgo-fix-numeric-equality-plan.md`).

## Steps

0. Prerequisite merged: liquidgo equality fix and its bump in Notifuse (own plan and PRs).
1. Golden test of the current converter output, passing on `dev`.
2. Tests first: fixture file, `display_condition_test.go`, model round trip, converter,
   compilation, domain (template, template block, workspace), service; see them fail.
3. `display_condition.go` (generation + evaluation), model fields, converter loops, domain
   validation.
4. Integration tests (transactional, broadcast, round trip).
5. Console: types, `displayCondition.ts` + tests, summary, modal, settings panel (keyed by id),
   tree icon, canvas badge, preview line, AI assistant; Lingui extract and translations.
6. CI path lists; OpenAPI yaml + bundle; VERSION 43.1 + `make sdk-build`; CHANGELOG; docs PR.
7. Run: `make test-pkg`, `make test-domain`, `make test-service`,
   `INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestDisplayCondition|TestTemplateRoundtripSerialization'`,
   `go vet ./...`, `cd console && npx vitest run && npx eslint . && npm run build`,
   `make openapi-lint`; then the full `make test-unit` once before the code review.
